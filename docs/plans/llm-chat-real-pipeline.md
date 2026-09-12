---
status: planned
priority: high
owner: TBD
target-stage: Stage 80（候选）
created: 2026-09-12
related-plans:
  - docs/plans/ai-response-structured.md（阶段 2 依赖本计划）
  - docs/plans/intent-classification-6-types.md（依赖本计划）
related-stages:
  - docs/stages/stage-78-business-plans-audit-2026-09-12.md（三计划核查）
related-adrs:
  - docs/architecture/decisions.md 决策 4（gRPC）
---

# Plan — 真实 LLM 对话链路（打通 mock → llm-service）

> 来源：2026-09-12 三份业务计划排期核查（stage-78）。核查发现 ai-response-structured
> 阶段 2 与 intent-classification-6-types 的共同前置是"存在真实 LLM 对话/分析链路"——
> 而当前 AI 聊天回复是 **BFF 硬编码 mock**，该链路在 docs/plans 里从未立过计划，本文件补上。

> **Stage 81 状态注记（2026-09-12）**：PR-2 已落地（stage-81 报告）。上游优先级链 =
> gRPC（llm-service）→ Phase D HTTP 直连（BFF_LLM_API_KEY 有 key 时保留）→ mock；
> llm-service 未注册 Nacos，BFF 用 `LLM_SVC_GRPC_ADDR` env 直连 + mTLS（复用 ai-client 证书）。

> **Stage 80 状态注记（2026-09-12）**：PR-1 已落地（stage-80 报告）。两处实施修正：
> ① 上游 env 沿用仓库既有 `LLM_API_KEY/LLM_BASE_URL/LLM_MODEL`（DeepSeek 等 OpenAI
> 兼容端点），非本文初稿所写的 Moonshot；② llm-service 对内已有 Stage 17/18 的
> x-internal-api-key + mTLS 约定，PR-2 的 BFF 客户端需对齐（Go 侧复用 ai-svc 调
> llm-service 的既有 TLS/auth 客户端模式）。PR-2 前置核查：llm-service 的 Nacos 注册
> 是否带 metadata.grpc_port（Stage 75 模式）。

## 一、现状（与代码事实对齐）

| 组件 | 现状 | 证据 |
|---|---|---|
| 前端发送 | `useAIStream.ts` POST `/api/v1/ai/stream`（OpenAI chat.completions 兼容 SSE） | `ai_stream_handler.go` 头注释 |
| BFF | **mock**：关键词共情回复 + 情绪标签，SSE 逐字 30ms；头注释自证"真实 LLM 对话流后续接 llm-service / ai-svc（保留 OpenAI 兼容格式，前端不变）" | `web-bff/internal/handler/ai_stream_handler.go:22` |
| llm-service（Python） | 仅文本**情绪分析** gRPC（规则式情感词命中，非 LLM）：`emotion + sentimentScore`；无 chat completion 能力 | `emotion-llm-service/main.py` |
| ai-svc aiclient | sensevoice（ASR）/ fer（表情）/ xtts（TTS）客户端；**无 LLM chat 客户端** | `ai-svc/internal/aiclient/` |
| Markdown 渲染 | 前端已就绪（marked + vue-dompurify-html） | stage-78 核查 |

## 二、目标架构

```
web ──SSE──> BFF /api/v1/ai/stream ──gRPC streaming──> llm-service（新增 ChatCompletion RPC）
                （保留 OpenAI 兼容格式，前端零改动）          │ Kimi/Moonshot API（chat.completions 流式）
                                                          └─ 系统人设 prompt（情感陪伴）
```

选 llm-service 而非 ai-svc 作 LLM 宿主的理由：Python 生态对 OpenAI 兼容 SDK 最顺；
ai-svc 保持 Go 侧 ASR/FER/TTS 编排职责；BFF→llm-service 走 gRPC（决策 4），复用
Nacos 发现（Stage 75 模式，metadata 带不带 grpc_port 由 llm-service 注册端补齐）。

## 三、分期（每期一个 TDD 循环，可独立收口）

### PR-1（llm-service）：gRPC 新增 ChatCompletion 流式 RPC

- proto 扩展：`emotion_llm.proto` 加 `ChatCompletion(stream)`（消息数组入、chunk 流出，
  兼容映射 OpenAI delta 格式由 BFF 侧做）
- Python 端：Moonshot/Kimi SDK（OpenAI 兼容 base_url）真流式转发；`MOONSHOT_API_KEY`
  env 注入 compose + Nacos 配置双通道；无 key 时降级返回现有 mock 文案（保 dev 可跑）
- 契约测试：proto_decode 对照 + fake key 降级路径；§契约 7 注册表 smoke 纳入 llm-service

### PR-2（BFF）：AIStreamHandler 从 mock 切 gRPC 上游

- RED：`ai_stream_handler_test.go` 加"上游 gRPC 可用时透传 chunk / 不可用时回落 mock"契约
- GREEN：downstream 新增 llm-service gRPC client（复用 `resolveGRPCAddr` Nacos 优先模式，
  Stage 75 打样）；SSE chunk 透传；mock 保留为 fallback（env 开关 `BFF_AI_MOCK=true`）
- 验收：dev 栈真 key 端到端——聊天页提问，流式返回真实 LLM 回复，SkyWalking 出
  BFF→llm-service 跨服务 trace

### PR-3（后续，解锁另两份计划）

- 意图分类：llm-service 在 chat completion 前置一次轻量分类调用（或独立 RPC），
  输出 6 类 intent → 重写 `intent-classification-6-types.md` 按新架构落地
- 结构化回复：按 intent 注入 system prompt 风格指令 → 落地 `ai-response-structured.md` 阶段 2

## 四、风险

- API key 管理：dev 走 .env，不进仓库；prod 走 Nacos 配置（PR-4 HotReload 模式复用）
- 限流/费用：BFF 现有 limit-count 在 APISIX 层已挡一层；llm-service 侧加并发上限
- mock 兜底不能删：无 key 环境（CI/离线 demo）必须仍可跑通全链路（§契约 6 同款哲学）

## 调研依据

- 已读：web-bff/internal/handler/ai_stream_handler.go（+test）、emotion-llm-service/main.py、
  ai-svc/internal/aiclient/{ai_client,interfaces}.go、ai-svc/internal/consumer/consumer.go、
  emotion-echo-web/app/{pages/chat/conversation/[id].vue,stores/message.ts}
- 已查：docs/plans/{ai-response-structured,intent-classification-6-types}.md（含本次核查注记）、
  decisions.md 决策 4、stage-75（BFF gRPC Nacos 拨号模式）、stage-78 核查报告
- grep 实证：kimi/moonshot/LLM chat 在 ai-svc/chat-svc/BFF 零命中；llm-service 仅情绪分析
