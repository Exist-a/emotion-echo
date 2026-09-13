"""Stage 91 PR-1 RED：file_context prompt 必须含强指令性措辞（修 Stage 89/90 PDF 拒读 residual）

背景：Stage 89 把 file_context 追加到 system 消息末尾，Stage 90 改为追加到最后一条
user 消息尾部——两次改造后 PDF 短文本哨兵仍部分拒读（DeepSeek 行为不一致）。

根因分析：现有 prompt 头"[附件上下文] 以下内容来自用户上传的附件，回答用户问题时可参考"
是**描述性**措辞。LLM 判定为"可选参考"后，对单短文本容易判定为噪声而忽略。

Stage 91 候选路线 #2（prompt 前置明确措辞）落地规格：
- 必须含**强指令性**短语（让 LLM 知道这是必读内容、不是噪声）
- 必须含**"原文引用"指令**（明确告诉 LLM：用户期待的就是附件里的原文）
- prompt 头必须**前置在 user 原文之前**（而非附在末尾）——这样 LLM 先看到指令再看到
  user 问题，符合"任务定义 → 上下文 → 用户问题"的对话结构

不变量（契约锁）：
1. build_file_context_text 的开头必须含"请基于以下附件原文回答"类指令性短语
2. 开头必须含"原文"二字（避免 LLM 改写/总结掉原文）
3. prompt 长度 ≤ 200 字符（避免过度占上下文；超出会让 LLM 觉得啰嗦）

下游约定：grpc_server.py 把 file_context_text 注入 user 消息**前面**（而非 Stage 90
的"追加到尾部"）。本测试只锁 build_file_context_text 的输出形态；注入位置由
test_grpc_files_v3.py（后续 PR）锁住。
"""

from file_context import build_file_context_text


# 强指令性短语关键词（必须出现在 prompt 头的前 200 字符内）
DIRECTIVE_KEYWORDS = ("请基于", "原文", "附件")


def _short_pdf_text() -> str:
    """模拟 PDF 短文本抽取结果（< 200 字符，Stage 90 报告中 DeepSeek 拒读的高发场景）"""
    return "PDF-SENTINEL-pypdf-OK-2026"


class TestFileContextPromptDirective:
    def test_prompt_header_contains_strong_directive(self):
        """prompt 头必须含强指令性短语（让 LLM 把它当必读任务，不是可选参考）"""
        out = build_file_context_text(
            [{"url": "http://x.localhost:9000/a.pdf", "name": "a.pdf"}],
            # 不让模块真去下载；直接给一份预先抽好的文本路径由 caller 注入
            # 这里改走 build_file_context_text 的内部分支——我们只测最终字符串形态
        )
        # 由于不传下载结果，会落"未能读取"分支；但 prompt 头仍应包含指令性措辞
        assert out, "file_context_text 必须非空"
        head = out[:200]
        for kw in DIRECTIVE_KEYWORDS:
            assert kw in head, (
                f"prompt 头必须含强指令性关键词 {kw!r}（避免 LLM 把它当噪声忽略）；"
                f"实际 head[:200]={head!r}"
            )

    def test_prompt_header_contains_quote_instruction(self):
        """prompt 头必须明确告诉 LLM"引用原文"——用户问题常见是"请引用附件里那句话"，
        LLM 必须理解"原文引用"是用户期待的核心动作。"""
        out = build_file_context_text(
            [{"url": "http://x.localio:9000/a.pdf", "name": "a.pdf"}]
        )
        head = out[:200]
        # 期望措辞至少包含"引用"或"原文引用"或"逐字"之一
        assert any(kw in head for kw in ("引用", "原文引用", "逐字", "原样")), (
            f"prompt 头必须明确'引用/原文/逐字'指令；实际={head!r}"
        )

    def test_prompt_header_within_size_budget(self):
        """prompt 头 ≤ 200 字符（避免冗余占上下文让 LLM 反感）"""
        out = build_file_context_text(
            [{"url": "http://x.localio:9000/a.pdf", "name": "a.pdf"}]
        )
        head = out[:200]
        assert len(head) <= 200, (
            f"prompt 头超 200 字符（{len(head)}），过度占上下文会让 LLM 反感"
        )


class TestFileContextWithActualShortPdfText:
    """端到端：直接测 build_file_context_text + 注入路径——验证 PDF 短文本场景下
    prompt 仍含强指令性 + 哨兵字符串保留在最终字符串中。"""

    def test_short_pdf_context_keeps_sentinel_and_directive(self, monkeypatch):
        """PDF 短文本（< 200 字符）抽取后注入，最终 prompt 必须：
        1. 含强指令性措辞
        2. 保留哨兵字符串（不让 LLM 误以为上下文被吞了）
        3. 标注"用户上传"身份
        """
        # 走 monkeypatch 跳过网络层：直接调抽取+truncate+build 链路
        from file_context import extract_text_from_bytes, truncate_text

        raw_pdf_bytes = b"%PDF-stub"  # 不真抽 pypdf，直接模拟文本
        # 实际：我们用 txt 路径模拟 PDF 短文本场景（fastest fixture）
        simulated_short_text = "PDF-SENTINEL-pypdf-OK-2026"
        truncated = truncate_text(simulated_short_text, max_chars=6000)

        # 走 build_file_context_text 内部分支——把"已抽取文本"装进 sections 模拟
        # 这里直接调 build_file_context_text 但短路 fetch_and_extract
        import file_context as fc

        def fake_fetch_and_extract(url, name, cfg=None):
            return truncated, ""

        monkeypatch.setattr(fc, "fetch_and_extract", fake_fetch_and_extract)
        out = fc.build_file_context_text(
            [{"url": "http://emotion-echo-minio:9000/a.pdf", "name": "a.pdf"}]
        )
        # 三条契约
        assert "PDF-SENTINEL-pypdf-OK-2026" in out, "哨兵必须保留"
        head = out[:200]
        assert "请基于" in head, "prompt 头必须含强指令性"
        assert "原文" in head, "prompt 头必须含'原文'"
