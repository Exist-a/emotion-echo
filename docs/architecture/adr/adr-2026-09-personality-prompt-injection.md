---
status: accepted
date: 2026-09-20
deciders: [e2e-14]
---

# ADR: 人格画像注入 AI 提示词的落点选在 BFF 侧

## 上下文

D-02 决议要求「人格量表产出心理画像 → 驱动 AI 提示词定制」。E2E-14 落地时面临一个落点选择：画像数据从哪里进入 LLM 请求？

现状（2026-09-20 实测）：

| 层 | 现状 | 证据 |
|----|------|------|
| system prompt | 两处硬编码静态字符串，内容完全相同 | `emotion-echo-web-bff/internal/handler/ai_stream_handler.go:226,284`（改造前行号） |
| proto | `ChatCompletionRequest` 无画像字段 | `proto/emotion_llm.proto:61-72` |
| llm-service | 只做 key 管理与上游降级，不读用户数据 | `emotion-llm-service` |
| BFF → assessment-svc | 已有 `AssessmentClient`（gRPC + HTTP 双实现） | `emotion-echo-web-bff/internal/downstream/assessment.go` |

即 prompt 组装目前完全在 BFF（两条上游路径——llm-service gRPC 与 HTTP 直连外部 LLM——各自组装一次，内容靠人工保持一致）。

## 决策

**在 BFF 侧组装画像并拼进 system prompt**，不改 proto、不改 llm-service。

具体：

1. 新增 `downstream.PersonalityProfileSource`，从 `GET /api/v1/surveys/results` 里挑出最新的人格量表结果（判据 `riskLevel == "dimension_profile"`，取 `factorScores`）
2. BFF `AIStreamHandler` 新增 `personalitySource` 依赖；`buildSystemPrompt(ctx)` 把基础人设 + 画像文本拼成最终 prompt
3. **两条上游路径共用同一个 `buildSystemPrompt`**（改造前是两份独立硬编码，本次收敛为一处）
4. 画像取数走 **HTTP**（gRPC `ListResults` 未实现，与 [adr-2026-09-survey-http-bypass](adr-2026-09-survey-http-bypass.md) 同因）
5. 缺画像（未测评 / 脏数据 / 上游报错）一律回落基础人设 prompt，**不阻断对话**

## 备选方案

| 方案 | 做法 | 未采纳原因 |
|------|------|-----------|
| **proto 层透传** | `ChatCompletionRequest` 加画像字段，llm-service 侧拼 prompt | 需要改 proto + 重新生成双端桩 + 改 llm-service（Python）；而 llm-service 目前刻意不碰用户数据（只做 key 管理/降级），让 BFF 查询反而更贴合既有边界 |
| 前端拼 prompt | 前端把画像拼进 messages 后下发 | 画像属服务端数据，前端拼装等于把 system prompt 的组装权交给不可信输入 |
| 改 llm-service 查库 | llm-service 直连 assessment-svc | llm-service 是 Python 轻服务，接入 Go 侧的数据访问栈会显著扩大其职责 |

## 后果

- **正面**：改动集中在 BFF 一层；两条上游路径的 prompt 组装由两份硬编码收敛为一处（原本已存在漂移风险）；proto 不动，无桩代码重生成成本
- **负面**：每次 AI 对话多一次 assessment-svc 查询（本地服务调用，未做缓存——刻意不加，避免"刚做完测评却因缓存读不到画像"的陈旧读）
- **负面**：BFF 的 `AIStreamHandler` 依赖数量增至 5 个（cfg / llm / files / chat / personality），构造函数已有 4 个变体；若再加依赖需考虑改 options struct
- **后续**：若画像查询成为延迟热点，可在 `PersonalityProfileSource` 内加短 TTL 缓存（须同时给出失效策略）；若 proto 日后需要承载画像，本 ADR 的落点选择可复议

## 关联

- 决议来源：`docs/e2e-roadmap/decisions.md` D-02（两种量表并存）
- 阶段详档：`docs/e2e-roadmap/stages/e2e-14-personality-ai-prompt/plan.md`
- 评分器：`emotion-echo-assessment-svc/internal/scoring/scorer.go` `BigFiveScorer`（写 `riskLevel = "dimension_profile"` 标记）
- 前置 ADR：[adr-2026-09-survey-http-bypass](adr-2026-09-survey-http-bypass.md)（survey 端点走 HTTP 的原因）
