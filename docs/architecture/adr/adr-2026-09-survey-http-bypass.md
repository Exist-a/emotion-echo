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
- ~~其他 BFF→assessment-svc 的交互（如 ListSurveys 列表）仍走 gRPC（列表不含 questions，无数据丢失）~~
  → **该行已过时（2026-10-08 更正）**：E2E-14 实测 `SurveyItem` proto 无 `description` 字段、走 gRPC 时 3 个量表描述全空，遂把 **ListSurveys 也并入 HTTP 路径**（该端点是 E2E-13 当时漏掉的第 5 个，见 `survey_handler.go:68-72` 注释）。**现状 = 5 个端点全部走 HTTP，BFF 侧 gRPC 客户端零调用**。

## 后果

- **正面**: 心理测验链路端到端跑通（Playwright 18/18 PASS），JSONB 数据完整性得到保证
- **负面**: survey 端点的 gRPC 路径变成死代码（`assessmentGRPCClient.GetSurvey` / `SubmitSurvey` / `GetResult` / `ListResults` 不再被调用）
- **后续**: 若需统一走 gRPC，需扩展 proto `SurveyQuestion` 消息支持完整 JSONB（或添加 `raw_questions` string 字段携带原始 JSON）

## 更正记录（2026-10-08，E2E-F-208 只读排查）

本节及 §决策 原文存在**两处与实际代码不符**，本次仅更正文档、不改代码：

| # | 原文断言 | 实际代码 | 性质 |
|---|---------|---------|------|
| 1 | §决策「其他 BFF→assessment-svc 的交互（如 ListSurveys 列表）仍走 gRPC」 | `survey_handler.go:72` 的 `if h.assessmentBase != ""` **恒真** ⇒ ListSurveys 走 HTTP | **决策写定时即自相矛盾**（§决策首行已把 `list` 列入 5 个 HTTP 端点）+ E2E-14 后彻底过时 |
| 2 | §后果「survey 端点的 gRPC 路径变成死代码」，范围暗示仅 4 个方法 | 实际是 **BFF→assessment-svc 整条 gRPC 通道**（5 客户端方法 + 服务端 251 行 + `:8886` 监听 + 一条白 dial 的 `*grpc.ClientConn`），且 `ListResults`/`GetResult` **客户端从未实现**（stub 直接返 error） | 低估范围 |

**与「决策 4」（跨服务调用 = gRPC + .proto，2026-09-11 已收口）的关系**：决策 4 清单 §四 #10 把「BFF → assessment-svc（5 RPC 全）」列为**已 gRPC**；本 ADR 使其在该链路上**失效**。二者张力是既存事实，**消解方向（补全 proto 后切回 gRPC）待裁**，见账本 `E2E-F-208`。

