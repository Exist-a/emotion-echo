"""CL-04 · Phase 5（T-10）：多附件必须**全部**出现在上下文块里（修缩进 bug）。

为什么补这条：既有 `test_file_context.py` 只覆盖**单附件**场景，而
`file_context.py:189-196` 的 `for` 循环体只含 `name = ...` 与 `fetch_and_extract(...)` 两行，
`if text is not None:` 及其 `sections.append(...)` **缩进在循环之外** ⇒
**N 个附件只输出最后一条**，且 `i` / `name` 泄漏为循环末值。

契约（plan G5 / §3.5「修 file_context.py 缩进 bug（多附件全输出）」）：
    附件数 = section 数；每条都带自己的文件名与自己的正文；防注入前缀仍在最前。
"""

import pytest

from file_context import FetchConfig, build_file_context_text


def _cfg(**kw) -> FetchConfig:
    defaults = dict(
        allowlist={"emotion-echo-minio:9000"},
        timeout=2.0,
        max_bytes=1024 * 1024,
        max_chars=100,
    )
    defaults.update(kw)
    return FetchConfig(**defaults)


@pytest.fixture()
def stub_fetch(monkeypatch):
    """按 URL 返回可区分的正文，便于断言"每条附件都在且内容对应"。"""
    from file_context import build_file_context_text as fn

    def fake(url, name, cfg=None):
        return (f"正文-{name}", "")

    monkeypatch.setitem(fn.__globals__, "fetch_and_extract", fake)
    return fn


class TestMultiAttachmentAllIncluded:
    def test_three_attachments_produce_three_sections(self, stub_fetch):
        atts = [
            {"url": "http://emotion-echo-minio:9000/avatars/uploads/a.txt", "name": "a.txt"},
            {"url": "http://emotion-echo-minio:9000/avatars/uploads/b.txt", "name": "b.txt"},
            {"url": "http://emotion-echo-minio:9000/avatars/uploads/c.txt", "name": "c.txt"},
        ]
        out = stub_fetch(atts, _cfg())

        assert out.count("--- 附件 ") == 3, f"3 个附件必须产出 3 个段，实际：{out!r}"
        for name in ("a.txt", "b.txt", "c.txt"):
            assert name in out, f"附件 {name} 丢失（缩进 bug：只有最后一条被输出）"
        # 每条附件的正文必须与自己的名字对应（防"内容错位/共用末值"）
        for name in ("a.txt", "b.txt", "c.txt"):
            assert f"正文-{name}" in out, f"附件 {name} 的正文缺失或错位"
        # 段落编号不得重复/跳号（i 曾在循环外泄漏成末值）
        assert "--- 附件 1：" in out and "--- 附件 2：" in out and "--- 附件 3：" in out

    def test_guard_prefix_still_first(self, stub_fetch):
        atts = [
            {"url": "http://emotion-echo-minio:9000/avatars/uploads/a.txt", "name": "a.txt"},
            {"url": "http://emotion-echo-minio:9000/avatars/uploads/b.txt", "name": "b.txt"},
        ]
        out = stub_fetch(atts, _cfg())
        assert "不可信" in out, "防注入前缀必须仍在"
        assert out.index("不可信") < out.index("--- 附件 1："), "防注入前缀必须在附件段之前"

    def test_mixed_success_and_failure_keeps_all(self, monkeypatch):
        from file_context import build_file_context_text as fn

        def fake(url, name, cfg=None):
            if name == "bad.txt":
                return (None, "抽取失败")
            return (f"正文-{name}", "")

        monkeypatch.setitem(fn.__globals__, "fetch_and_extract", fake)
        atts = [
            {"url": "http://emotion-echo-minio:9000/avatars/uploads/ok1.txt", "name": "ok1.txt"},
            {"url": "http://emotion-echo-minio:9000/avatars/uploads/bad.txt", "name": "bad.txt"},
            {"url": "http://emotion-echo-minio:9000/avatars/uploads/ok2.txt", "name": "ok2.txt"},
        ]
        out = fn(atts, _cfg())

        assert out.count("--- 附件 ") == 3, "成功/失败混合时也必须逐条产出（失败条给注记）"
        assert "正文-ok1.txt" in out and "正文-ok2.txt" in out
        assert "未能读取" in out and "bad.txt" in out
