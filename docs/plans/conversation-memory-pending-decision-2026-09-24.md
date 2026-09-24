---
status: planned
priority: high
owner: TBD
created: 2026-09-24
type: pending-decision
source: 用户 2026-09-24 会话中口头反馈（「会话中的记忆系统似乎没有，这是巨大的问题」→ 指令：有对应阶段记阶段下，无阶段先放待决策文档）
depends-on: []
related-plans:
  - on-device-hybrid-inference-2026-09-23.md（v0.2 §6.2 分层记忆 = **端侧上下文管理**，与本条云端对话记忆是不同问题，决策时应互通但互不替代）
related-stages:
  - e2e-10-chat-core（收发链路，不含历史注入）
  - e2e-14-personality-ai-prompt（画像注入 system prompt，不是对话记忆）
related-adrs: []   # 全仓 ADR 无「会话记忆/对话历史」决策记录（2026-09-24 grep 核实）
---

# 待决策 — 会话记忆系统缺失（conversation memory）

## §A 上下文与假设

- 用户 2026-09-24 反馈：会话中的记忆系统似乎没有，判定为**巨大问题**。
- 本文假设（与现状对比见 §B）：
  1. 假设「AI 在同一会话多轮对话中能记得之前轮次说过什么」——**现状：不能**；
  2. 假设「跨会话存在某种用户记忆持久化」——**现状：不存在**（仅有人格画像，来自问卷不来自对话）；
  3. 假设「该缺口有 ADR/决策记录（有意设计为无记忆）」——**现状：全仓 ADR / decisions.md / e2e decisions.md 均无相关决策记录**，因此**不能**把它当"有意设计"，只能当缺口。
- 归属核查：`docs/e2e-roadmap/roadmap.md` 30 个阶段无一覆盖本主题；账本 `discovered-unresolved.md` 的 "memory" 命中全部是 `InMemory*` 测试替身，与会话记忆无关。按用户指示 → 无对应阶段，落本文档。

## §B 现状核实（2026-09-24 代码证据链，逐条可复现）

**结论：AI 每一轮只看到「system prompt + 当前这一条用户消息」，对同会话此前轮次零记忆；跨会话亦无任何记忆存储。**

| # | 环节 | 证据（文件:行号） | 说明 |
|---|------|------------------|------|
| 1 | 前端 OpenAI 格式 | `emotion-echo-web/app/composables/useAIStream.ts:52` | `messages: [{ role: 'user', content: prompt }]` —— **数组里只有当前一条** |
| 2 | 前端 sender 格式 | `emotion-echo-web/app/composables/useConversationSender.ts:141` | 请求体 `{ message: content, ... }` —— 单条消息，无历史字段 |
| 3 | BFF 提取 | `emotion-echo-web-bff/internal/handler/ai_stream_handler.go:364-367` | 从 `req.Messages` **只取最后一条** user 消息作 `userContent`，其余丢弃 |
| 4 | BFF 组装 LLM 请求 | `emotion-echo-web-bff/internal/handler/ai_stream_handler.go:409-413` | `llmMessages = [system prompt, {role:"user", content:userContent}]` —— **恰好 2 条，无任何历史** |
| 5 | BFF 拉过历史但只用于文件 | 同文件 `:290-320` `collectFileAttachments` | `ListMessages(limit=50)` 只挑 `ContentType=="file"` 的附件，**文本历史被丢弃** |
| 6 | llm-service 不补历史 | `emotion-llm-service/grpc_server.py:267` | 原样透传收到的 `messages`；`file_context.py` 只注入文件上下文；**无任何 history 拉取** |
| 7 | 已有的"近似记忆"均非对话内容 | `ai_stream_handler.go:95-170`（D-14 情绪段）；E2E-14 画像段 | 情绪段 = 最近情绪**标签**模式；画像段 = 问卷人格。**都不含用户说过的话** |

**测试现状**：`emotion-echo-web-bff/internal/handler/ai_stream_handler_test.go` 现有 5 个测试（SSE 头 / OpenAI 格式 delta / 缺 messages 400 / 悲伤回复 / 默认回复）—— **无一断言历史注入**，即"无记忆"从未被测试钉住（既没被验证为设计，也没被验证为缺陷）。

## §C 影响面

- **同会话多轮**：聊到第 N 轮，AI 对前 N-1 轮内容完全失忆，只剩人设 + 画像 + 情绪标签。用户复述过的信息会被要求再讲一遍。
- **跨会话**：无任何用户事实记忆（姓名/偏好/既往倾诉内容均不落盘）；唯一持久化的是人格画像（E2E-14，来源是量表问卷**不是对话**）。
- **产品定位伤害**：情绪疏导陪伴场景下"记得我说过什么"是信任基础；且 F-134 明确对标豆包（有记忆的产品）。
- **不阻塞当前任何 E2E 阶段**（E2E-10/14 的既有测试点不受影响），故不构成 roadmap 依赖断裂——属于**新功能缺口**，不是回归。

## §D 待决策项（需用户拍板，本文档不擅自决议）

| 选项 | 内容 | 代价/备注 |
|------|------|-----------|
| **D-a 要不要** | 产品层面是否需要会话记忆（同会话多轮历史 + 跨会话持久记忆可分开决策） | 一切的前提 |
| **D-b 同会话历史注入的形态** | ① 滑动窗口（近 N 轮全量拼进 `llmMessages`，BFF `ai_stream_handler.go:409` 处改造，改动面小）/ ② 摘要压缩（每轮后异步摘要，token 稳定，需新链路）/ ③ 维持现状 | ①最快，`ListMessages` 已现成（证据 #5 拉 50 条只差文本过滤） |
| **D-c 跨会话记忆的形态** | 无 / 用户事实抽取落库 / RAG 检索既往对话 | 涉 schema 变更（E2E-19 边界外）+ 隐私定位，建议与 v0.2 §十二决策 5「摘要存储」**一并**拍板 |
| **D-d 排期归属** | ① 新增 roadmap 阶段（如 E2E-31 会话记忆验证，先立功能 plan 再验）/ ② 并入既有阶段 / ③ 明确"暂不做"并补一条 ADR 记录"有意无记忆" | 若选 ③，必须落 ADR，否则本文档本身就成了新的文档-代码漂移 |

**推荐顺序**：先 D-a（要不要）→ 要则 D-b 选 ①（改动面最小）→ D-c 与 v0.2 §十二决策 5 联动 → 最后 D-d 排期。

## §E 与 Lane O / 双轨协议的边界

- Lane O 的端侧「分层记忆」（v0.2 §6.2：端云共用组装层、token 预算）解决的是**端侧推理时上下文怎么装**；本文档解决的是**云端对话历史根本没进上下文**。两者在 D-b/D-c 拍板时应互通（若端侧将来要记忆，组装层入口同样要吃历史），但**互不替代**。
- 本文档为 `docs/plans/` 非 on-device 文件，不触碰 Lane O 独占列；`README.md` 索引与本行握手登记见 `docs/_meta/parallel-tracks.md` §六。

## §F 调研依据（AGENTS.md §〇）

- **已读实现文件（6）**：`useAIStream.ts`、`useConversationSender.ts`、`ai_stream_handler.go`（关键段全读）、`emotion-llm-service/grpc_server.py`、`file_context.py`、`chat_completion.py`。
- **已读测试文件（1）**：`ai_stream_handler_test.go`（5 个 Test 函数，无历史断言）。
- **已查 ADR/决策（3 处）**：`docs/architecture/adr/*.md` grep 记忆/memory/历史 → 无相关决策；`docs/architecture/decisions.md` → 无；`docs/e2e-roadmap/decisions.md` D-01~D-25 → 无。
- **已查归属（2）**：roadmap 30 阶段 grep 记忆/memory/上下文 → 无覆盖阶段；账本 grep → 命中均为 `InMemory*` 替身。
- **smoke**：不适用（本发现不触及 §2.4 数据契约的任一管道；证据为静态代码链而非运行态契约）。
- **外部信息**：不适用（无外部依赖版本涉入）。
