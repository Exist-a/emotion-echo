# ADR — 决策 4（跨服务调用 = gRPC + .proto）收口

| 维度 | 选择 |
|------|------|
| 状态 | ✅ **Accepted**（从原"未来/待实施"翻"实施完成"） |
| 日期 | 2026-09-11 |
| 形式化触发 | Stage 63 端到端收口 → Sprint C/D/E/F1 全部落地 |

---

## 一、决策 4 原文（[`docs/architecture/decisions.md`](../decisions.md) 决策 4）

> **跨服务调用 = gRPC + .proto**
>
> | 维度 | 选择 |
> |------|------|
> | 外部 API（浏览器→svc） | HTTP REST + JSON + APISIX |
> | 内部 svc-to-svc | **gRPC + .proto**（待实施） |
> | 异步事件 | Kafka + JSON |

## 二、收口结论（2026-09-11 实测）

| 维度 | 覆盖率 | 评注 |
|---|---|---|
| 核心业务路径（BFF→4 svc handler 调用） | **21/21 = 100%** | 全部走 gRPC |
| BFF→4 svc 全方法 | **100%** | user-svc 7/7（含 Logout）、chat-svc 4/4 handler、assessment-svc 5/5、analytics-svc 6/6 handler |
| 所有内部 svc-to-svc（决策 4 全文范围） | **~90%** | 12 条 gRPC + 4 条故意不做 |

**决策 4 核心意图已 100% 生效**。

## 三、实施里程碑

| Sprint | 落地内容 | commit | 解决决策 18 |
|---|---|---|---|
| Sprint C | shared/pkg/ctxkey 重构 + 拦截器/middleware 两侧类型别名指向 ctxkey.UserID | `72c599d` | #26（grpcinterceptor/middleware 两套 ctx key 不通） |
| Sprint D | chat-svc gRPC 4 RPC 真实实现（ListConversations/SendMessage/ListMessages/DeleteConversation） | `3829154` | #27（chat-svc gRPC 6 RPC 全 Unimplemented） |
| Sprint E | user.proto 扩 Login/Register RPC + user-svc gRPC server 实现 + BFF client 接入 | `f755b71` | #33（user.proto 半残缺） |
| Sprint F1 | user.proto 扩 ResetPassword/Logout RPC + user-svc gRPC server 实现 + BFF client 接入 | `fa324df` | #35（Sprint E 报告"还差 ResetPassword/Logout"延迟收口） |
| Sprint F2 | emotion_query.proto 扩 MultiModalAnalyze/SynthesizeSpeech/AIHealth 3 RPC + ai-svc gRPC server 实现 + BFF aiGRPCClient 接入 + `/api/v1/ai/health` 路由补 | 本次 commit | 完成决策 4 "全文范围"目标 |
| Sprint G | BFF 全局 gRPC error → HTTP code 映射（`MapGRPCError` helper + 5 client 24 处 `fmt.Errorf` → `wrapGRPCError` 包装 + handler 侧零改动自动生效） | `f874d3f` | 解决 tts/synthesize gRPC Unavailable 误返 502 问题（Sprint F2 V3 暴露） |

## 四、内部 svc-to-svc gRPC 化全清单

### ✅ 已 gRPC（15 条调用链）

| # | 调用链 | proto / RPC | 关键文件 |
|---|---|---|---|
| 1 | chat-svc → ai-svc | `emotion_query.proto` · UpsertNeutralEmotion | `chat-svc/internal/grpcclient/ai_client.go` + `ai-svc/internal/grpcserver/server.go` |
| 2 | ai-svc → emotion-llm-service | `emotion_llm.proto` · 文本情绪分析 | `ai-svc/internal/analyzer/grpc_analyzer.go` + Python `emotion-llm-service/` |
| 3 | BFF → ai-svc (emotion_query) | `emotion_query.proto` · GetEmotionByMessage/GetEmotionByConversation/GetFusedEmotion | `web-bff/main.go:253` `grpc.NewClient` + `web-bff/internal/emotion_query.go` |
| 4 | BFF → user-svc (Login) | `user.proto` · Login | Sprint E |
| 5 | BFF → user-svc (Register) | `user.proto` · Register | Sprint E |
| 6 | BFF → user-svc (ResetPassword) | `user.proto` · ResetPassword | Sprint F1 |
| 7 | BFF → user-svc (Logout) | `user.proto` · Logout | Sprint F1 |
| 8 | BFF → user-svc (GetMe/UpdateProfile/GetUserById) | `user.proto` · GetMe/UpdateProfile/GetUserById | Stage 62 PR-3.3 |
| 9 | BFF → chat-svc (ListConversations/SendMessage/ListMessages/DeleteConversation) | `chat.proto` · 4 RPC | Sprint D |
| 10 | BFF → assessment-svc (5 RPC 全) | `agent.proto` · ListSurveys/GetSurvey/SubmitSurvey/ListMyResults/GetSurveyResult | Stage 62 PR-3.3 |
| 11 | BFF → analytics-svc (6 handler 触发的 RPC) | `metric.proto` · ReportsDaily/ReportsTrend/UserBehavior{3}/MentalHealthAssessment | Stage 62 PR-3.3 |
| 12 | BFF → ai-svc (MultiModalAnalyze) | `emotion_query.proto` · MultiModalAnalyze | Sprint F2 |
| 13 | BFF → ai-svc (SynthesizeSpeech) | `emotion_query.proto` · SynthesizeSpeech | Sprint F2 |
| 14 | BFF → ai-svc (AIHealth) | `emotion_query.proto` · AIHealth | Sprint F2 |

### ❌ 故意不做（3 条 + 1 边界）

| # | 调用链 | 不做原因 | 出处 |
|---|---|---|---|
| 1 | ai-svc → FER/SenseVoice/XTTS | FastAPI 模型服务（Python），改 gRPC 成本高；AI profile 按需启用，调用量低 | [`grpc-inter-service-migration.md`](../../plans/grpc-inter-service-migration.md) §决策 A |
| 2 | BFF → llm-service (DeepSeek 外部 API) | **外部 API，决策 4 明文走 HTTP** | 决策 4 |
| 3 | chat-svc gRPC PinConversation/StreamMessages | chat-svc 缺底层功能（无 pin 字段、无流式业务）；proto 留接口，业务未触发 | Sprint D 决策 |
| 4 | chat-svc HTTP `/api/v1/conversations` 500 | BFF 默认 grpc 规避；根因待查（决策 18 #32） | 决策 18 #32 |

## 五、proto 契约全清单

| proto 文件 | package | go_package | RPC 数 | 状态 |
|---|---|---|---|---|
| `proto/emotion_llm.proto` | `emotion_llm.v1` | `emotionllm` | 1 | ✅ 已用（ai-svc → llm） |
| `proto/emotion_query.proto` | `emotion_query.v1` | `emotionquery` | **7** | ✅ 已用（chat→ai UpsertNeutralEmotion；BFF→ai 6 RPC: 3 emotion_query + 3 业务） |
| `proto/chat.proto` | `emotion_chat.v1` | `emotionchat` | 7 | ✅ 4 已实现（Pin/Stream 留 Unimplemented） |
| `proto/agent.proto` | `emotion_assessment.v1` | `emotionassessment` | 5 | ✅ 5 全实现 |
| `proto/metric.proto` | `emotion_analytics.v1` | `emotionanalytics` | 9 | ✅ 9 全 server 实现 + 6 BFF client |
| `proto/user.proto` | `emotion_user.v1` | `emotionuser` | **7** | ✅ 7 全实现（Sprint E/F1 扩 Login/Register/ResetPassword/Logout） |

## 六、gRPC 基础设施完整度

- **拦截器套件**（`shared/pkg/grpcinterceptor/`）：auth / client / retry / server / stream / **tracing** / **userid** / **logging** / **recovery** — 9 个全有单测
- **ctx key 唯一**：`shared/pkg/ctxkey.UserID`（Sprint C 收口，middleware + grpcinterceptor 双侧别名指向）
- **健康检查**：ai-svc / chat-svc / user-svc / assessment-svc / analytics-svc 5 svc 全挂 `grpc.health.v1.Health`
- **错误映射**：chat-svc `mapLogicError` / user-svc `mapAuthError` / analytics-svc / assessment-svc 各自定义（统一收口是后续 sprint backlog）
- **proto 生成脚本**：`proto/gen.sh`（protoc v32.1）一键生成 Go + Python stub
- **gRPC 版本**：`google.golang.org/grpc v1.80.0`（BFF + 4 svc 一致）

## 七、决策 4 收口判定标准（self-audit checklist）

- [x] 决策 4 原文"内部 svc-to-svc = gRPC + .proto" 100% 覆盖（BFF→4 svc handler 调用 24/24 = 100% + BFF→ai-svc 业务 3 RPC 走 gRPC）
- [x] proto 契约有单一事实源（`proto/*.proto` 6 个文件，gen.sh 一键生成）
- [x] gRPC 拦截器套件完整（userid/tracing/logging/recovery 全 9 个）
- [x] ctx key 跨包一致（ctxkey 重构后 middleware + grpcinterceptor 互通）
- [x] 故意不做的边界有 plan §决策 A 明确划定（不静默不写）
- [x] 决策 18 doc-drift registry 中相关失真 #25-#35 已关闭或登记
- [x] docker 端到端验证：Stage 63 V1-V4 + Sprint E/F1/F2 register/login/reset-password/users-me/conversations/surveys/reports/daily/ai-health/multimodal-analyze/tts-synthesize 全 200（除 tts 因 XTTS 容器未启 503 预期错误流）

## 八、不在本 ADR 范围（后续 sprint backlog）

| 项 | 工作量 | 优先级 |
|---|---|---|
| ~~**#32** chat-svc HTTP `/api/v1/conversations` 500 bug 根因排查~~ | ~~1-2 小时~~ | 🟢 **已关闭（PR-3 `3e07571`）** — chat-svc v0.1.3 rebuild 后实测 4 路径（/health、/metrics、无 auth、有 auth）全 200/401；InMemory repo + Postgres 真库均无 500。bug 在镜像升级（v0.1.3 含 PR-2 分层 + Sprint D chat-svc gRPC 4 RPC 实现）中已被规避 |
| **chat-svc PinConversation gRPC** | 1 天（需 schema migration） | 🟢 低（业务未触发） |
| **chat-svc StreamMessages gRPC** | 1 天（需重新评估流式业务场景） | 🟢 低 |
| **错误码统一映射**（chat-svc mapLogicError / user-svc mapAuthError / analytics-svc / assessment-svc 各自分散） | 1 天 | 🟡 中 |
| ~~BFF 全局 gRPC error → HTTP code 映射~~（Sprint F2 V3 暴露：BFF 把 gRPC codes.Unavailable 统一标 502，与 HTTP handler 503 行为不符） | 半天 | 🟢 **Sprint G 已完成（`f874d3f`）** — tts/synthesize 502→503；7 路径 Final E2E 全 200/503 |
| **gRPC mTLS**（dev 用 insecure，prod mTLS） | 1 周 | 🟡 中（决策 18 已记录 prod 必做） |

## 九、调研依据

- `docs/architecture/decisions.md` 决策 4 区块
- `docs/plans/grpc-inter-service-migration.md` §一.2/§一.3/§决策 A/B
- `docs/legacy-plans/landed/sprint-{c,d,e,f1}-*.md`（4 份 sprint landed 文档）
- `docs/architecture/adr/adr-2026-09-doc-drift-registry.md` #25-#35
- `docs/stages/stage-63-bff-grpc-wiring.md`（V1-V4 端到端实测表）
- `proto/*.proto`（6 个 proto 文件 + gen.sh 脚本）
- `emotion-echo-shared/pkg/grpcinterceptor/`（9 个拦截器套件）
