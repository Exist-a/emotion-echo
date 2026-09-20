---
status: accepted
date: 2026-09-20
deciders: [e2e-13]
---

# ADR: assessment-svc survey 端点走 HTTP 绕过 gRPC

## 上下文

E2E-13 修复心理测验链路时发现：BFF 通过 gRPC 调 assessment-svc 的 `GetSurvey` / `SubmitSurvey` 端点时，proto 转换会丢失数据：

1. **GetSurvey**: questions JSONB（`map[string]Question`）被转为 proto `SurveyQuestion`（字段为 `prompt`/`options: []string`/`scaleMin` 等），丢失了前端需要的 `title`/`options: [{id,text,score}]` 结构
2. **SubmitSurvey**: answers key（`"q1"`）被 `parseInt64` 转为数字 `1`，再 `fmt.Sprintf` 转回 `"1"`，scorer 期望 `"q1"` 格式

根因：proto `SurveyQuestion` 消息结构无法表达灵活的 JSONB 格式（`options` 是 `repeated string` 而非 `repeated Message`）。

## 决策

BFF 的 survey handler 5 个端点（list/detail/submit/listResults/getResult）**走 HTTP 直调 assessment-svc**，绕过 gRPC 转换层。

- HTTP 路径保留原始 JSONB 格式（`questions` 为 `map[string]any`，BFF 层做 map→数组转换）
- HTTP 路径保留 answers key 原始格式（`"q1"` 不被转为数字）
- 其他 BFF→assessment-svc 的交互（如 ListSurveys 列表）仍走 gRPC（列表不含 questions，无数据丢失）

## 后果

- **正面**: 心理测验链路端到端跑通（Playwright 18/18 PASS），JSONB 数据完整性得到保证
- **负面**: survey 端点的 gRPC 路径变成死代码（`assessmentGRPCClient.GetSurvey` / `SubmitSurvey` / `GetResult` / `ListResults` 不再被调用）
- **后续**: 若需统一走 gRPC，需扩展 proto `SurveyQuestion` 消息支持完整 JSONB（或添加 `raw_questions` string 字段携带原始 JSON）
