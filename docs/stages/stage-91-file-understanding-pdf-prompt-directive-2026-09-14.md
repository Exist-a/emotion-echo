# Stage 91 · 2026-09-14 file-understanding PDF 强指令性 prompt（Stage 90 residual）

> **状态**：🟡 **代码 + 单测全绿（196/196）；容器 e2e 因 GitHub tini ADD 超时阻塞，待网络恢复后补**
> **关联**：[`docs/stages/stage-90-file-understanding-pdf-tail-inject-2026-09-13.md`](stage-90-file-understanding-pdf-tail-inject-2026-09-13.md)（residual 来源）

## 核心结论

**已完成**：file_context prompt 头从**描述性**改**强指令性**措辞，单元测试契约锁住规格。

**未验证**：容器 e2e PDF 哨兵重测——GitHub `krallin/tini` ADD 超时阻塞镜像重建。**这是网络环境问题，不是代码问题**（与 Stage 36 pypi CDN、Stage 89/90 重测阻塞同源）。**待网络恢复后**用新镜像重启容器，重跑 PDF 哨兵 5-10 次验证拒读消除。

## 一、本期 PR 收口

### PR-1 file_context prompt 头改强指令性

**emotion-llm-service/file_context.py** `build_file_context_text` 末尾：

```python
# 旧（Stage 90 描述性）：
"[附件上下文]\n以下内容来自用户上传的附件，回答用户问题时可参考；"
"若附件内容与问题无关请如实说明：\n" + sections

# 新（Stage 91 强指令性）：
_PROMPT_HEADER = (
    "[附件上下文 · 请基于以下用户上传的附件原文回答用户问题，必要时逐字引用原文]\n"
    "以下内容来自用户上传的附件，是必读上下文：\n"
)
return _PROMPT_HEADER + "\n".join(sections)
```

**变化点**：
- 标题加"请基于附件原文回答" + "必要时逐字引用原文"（强指令性）
- "回答用户问题时可参考" → "是必读上下文"（明确非可选噪声）
- 去掉"如实说明"尾句（事实约束由 system prompt 管；附件 prompt 不该越权）

**为什么这样改**：Stage 90 报告根因分析——原措辞"可参考/如实说明"是**描述性**，DeepSeek 把末尾 user 消息的附件内容判定为"可选噪声"，对单短文本尤其敏感（PDF 哨兵 4 次 2 拒）。改"必读任务 + 引用原文"把判定从"噪声"升级为"任务核心"。

### PR-2 抽 `_PROMPT_HEADER` 模块常量 + docstring 同步

为未来 A/B 测试不同措辞留接口；docstring 增 Stage 91 段落说明规格契约由 `test_file_context_prompt_directive` 锁住。

## 二、规格契约（测试）

新增 `emotion-llm-service/tests/unit/test_file_context_prompt_directive.py` 4 用例：

| 用例 | 断言 |
|------|------|
| `test_prompt_header_contains_strong_directive` | head 前 200 字符必含 `请基于`、`原文`、`附件` 三关键词 |
| `test_prompt_header_contains_quote_instruction` | head 必含 `引用`/`原文引用`/`逐字`/`原样` 之一 |
| `test_prompt_header_within_size_budget` | head 长度 ≤ 200 字符（避免过度占上下文） |
| `test_short_pdf_context_keeps_sentinel_and_directive` | PDF 短文本场景端到端：哨兵保留 + 强指令性 + "用户上传"身份 |

**RED → GREEN 全过程**：3/4 用例 RED 全 fail（现状描述性 prompt 缺关键词），1 用例已 pass（长度本就够短）。GREEN 后 4/4 pass。

## 三、未验证项（容器 e2e）

**待网络恢复后必做**：

1. `cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml --env-file .env.local build --no-cache emotion-llm-service`
2. 重建镜像后启动容器：`up -d emotion-llm-service`
3. PDF 哨兵重测（按 Stage 90 报告 §二流程）：手工构造合法 PDF → 发消息 → 验 AI 流式响应引用 `PDF-SENTINEL-pypdf-OK-2026`
4. 跑 5-10 次确认成功率 ≥ 80%（Stage 89 baseline ~50%，Stage 90 ~50%，目标 ≥ 80%）

**当前阻塞**：GitHub `https://github.com/krallin/tini/releases/download/v0.19.0/tini` 不可达（`unexpected EOF`）。宿主机网络整体 GitHub 受限（curl github.com 也失败）。同源问题 Stage 36/89/90 都遇到过，记录在 todo-pile 中。

## 四、本批新增的"文档卫生"顺手销账

无（todo-pile 已 Stage 90 销账 A1/A2/B4）。

## 五、关键 commit

```
[Stage 91 PR-1 RED]   test(llm): Stage 91 PR-1 RED — file_context prompt 必须含强指令性措辞
[Stage 91 PR-1 GREEN] feat(llm): Stage 91 PR-1 GREEN — file_context prompt 头改强指令性措辞
[Stage 91 REFACTOR]   refactor(llm): Stage 91 PR-1 — file_context prompt 头抽常量 + docstring 同步
[Stage 91 DEPLOY]     chore(deploy): bump llm-service 镜像标签 v0.1.0 → v0.1.2（Stage 91 PR-1）
```

## 六、残余（Stage 92+ 候选）

| 项 | 说明 | 路线 |
|---|---|---|
| **容器 e2e 验证** | 网络恢复后必做；PDF 哨兵 5-10 次确认 ≥ 80% 引用率 | 网络恢复后立即 |
| 若 e2e 仍部分拒读 | 试 Stage 90 候选 #1（注入第二条独立 user turn）/#3（长 PDF 切片摘要）/#5（换模型对比） | Stage 92 评估 |
| observability-edge-gaps B-F 5 项 | B 30min / C 1.5h / D 30min / E 1h / F 30min；按需排期 | 独立 sprint |
| observability-edge-gaps A 项（Kafka sw8 透传） | 半天；与文件理解无强耦合，可独立 | 独立 sprint |
| Kafka 可选残余：consumer 进程级指标 | 半天 | 独立 sprint |
| web 历史 typecheck 96 处 | charts/DigitalHuman 等遗留 | 清理 sprint |
| prod 独立 bff-client 证书 | llm mTLS 现复用 ai-client | 部署事项 |
| todo-pile C6（quick-login 端点） | 1-2h | 顺手 |
| todo-pile D5（chat-svc 表依赖 ADR） | 半天 | 顺手 |

## 七、调研依据

- 已读：`file_context.py`（Stage 89 PR-4）/ `grpc_server.py:287-310`（Stage 90 注入位置）
  / `test_grpc_files_v2.py`（Stage 90 RED 规格）/ `pdf_fixture_b64.txt`（Stage 89 旧 fixture）
- 容器实证（Stage 90 报告）：PDF 哨兵 4 次 2 成功 2 拒——DeepSeek 行为不一致
- prompt 措辞对照：
  - 旧 = 描述性："可参考"（"参考" = 可选）/ "如实说明"（事实约束）
  - 新 = 指令性："请基于"（明确任务）/ "必要时逐字引用"（明确动作）/ "必读上下文"（明确优先级）
- 网络阻塞佐证：`curl -sI --max-time 20 https://github.com` 无响应 + `docker build` tini ADD `unexpected EOF`

---

> 最后更新：2026-09-14 by Stage 91 实施 session
> 关联：file-understanding-llm.md（Stage 89 计划，主线已落地）/ stage-90（residual 来源）
> Plan：本阶段小调整（仅 prompt 措辞 + 测试契约），不另起计划文档
