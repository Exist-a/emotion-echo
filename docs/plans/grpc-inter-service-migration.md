---
status: planned
priority: medium
owner: TBD
created: 2026-09-07
updated: 2026-09-11
related-stages:
  - stage-10-grpc-migration.md
  - stage-19-ai-svc-grpc-server.md
  - stage-34-multimodal-fusion.md
  - stage-36-fixes-roadmap.md
  - stage-58-q3-followups.md（chat-svc gRPC 6 RPC 实测 Unimplemented）
  - stage-63-bff-grpc-wiring.md（收口发现 #25 #26 #27）
related-adrs:
  - docs/architecture/decisions.md（决策 4：跨服务调用 = gRPC + .proto）
  - docs/architecture/decisions.md（决策 5：Python LLM 服务 = 独立微服务 + gRPC server）
  - docs/architecture/adr/adr-2026-09-doc-drift-registry.md（#25 #26 #27）
---

## 🆕 2026-09-11 增补：Stage 63 收口发现 3 条链路 bug

Stage 63 端到端验证发现 BFF→4 svc gRPC 链路上有 3 层叠错（决策 18 #25 #26 #27）。
**Sprint C（解 #26 ctxkey 重构）已落地** —— 见 [legacy-plans/landed/sprint-c-ctxkey-refactor.md](../legacy-plans/landed/sprint-c-ctxkey-refactor.md)。
**Sprint D（解 #27 chat-svc 4 RPC 实现）已落地** —— 见 [legacy-plans/landed/sprint-d-chat-grpc-implementation.md](../legacy-plans/landed/sprint-d-chat-grpc-implementation.md)。
**剩余 #32 chat-svc HTTP 端 /api/v1/conversations 500 bug** —— BFF 默认 grpc 后被规避，但根因未查，记后续。

# Plan — 后端微服务间调用 HTTP → gRPC 改造

## 一、现状（与代码事实对齐）

### 1.1 决策背景

`docs/architecture/decisions.md` 决策 4 明确：

> 外部 API（浏览器→svc）= HTTP REST + JSON + APISIX
> 内部 svc-to-svc = **gRPC + .proto**（待实施）
> 异步事件 = Kafka + JSON

决策 5：Python LLM 服务 = 独立微服务 + gRPC server。

### 1.2 已落地 gRPC 的调用链（3 条）

| 调用链 | 协议 | proto / 接口 | 证据 |
|---|---|---|---|
| **chat-svc → ai-svc** | gRPC | `emotion_query.proto` · `UpsertNeutralEmotion`（dev fallback 用） | `chat-svc/internal/grpcclient/ai_client.go` + `ai-svc/internal/grpcserver/server.go` |
| **ai-svc → emotion-llm-service** | gRPC | `emotion_llm.proto` · 文本情绪分析 | `ai-svc/internal/analyzer/grpc_analyzer.go` + Python `emotion-llm-service/` |
| **BFF → ai-svc** | gRPC | `emotion_query.proto` · `GetEmotionByMessage` / `GetEmotionByConversation` / `GetFusedEmotion` | `web-bff/main.go:202` `grpc.NewClient` + `web-bff/internal/emotion_query.go` |

ai-svc gRPC server（`grpcserver/server.go`）已挂完整拦截器链：user ID metadata 拦截器 + SkyWalking tracing + logging + recovery，并有 grpc_health_integration_test。

### 1.3 仍是 HTTP 的内部调用（5 条）

| 调用链 | 协议 | 证据 |
|---|---|---|
| **BFF → user-svc** | HTTP REST | `web-bff/internal/user.go` + config `UserService.BaseURL=http://emotion-echo-user-svc:8888` |
| **BFF → chat-svc** | HTTP REST | `web-bff/internal/chat.go` + config `ChatService.BaseURL=http://emotion-echo-chat-svc:8890` |
| **BFF → assessment-svc** | HTTP REST | `web-bff/internal/assessment.go` + config `AssessmentService.BaseURL=http://emotion-echo-assessment-svc:8889` |
| **BFF → analytics-svc** | HTTP REST | `web-bff/internal/analytics.go` + config `AnalyticsService.BaseURL=http://emotion-echo-analytics-svc:8904` |
| **ai-svc → FER / SenseVoice / XTTS** | HTTP REST | `ai-svc/internal/aiclient/{fer,sensevoice,xtts}.go`（FastAPI 模型服务）|

BFF 的 HTTP downstream client 在 `internal/downstream/`（user/chat/analytics/assessment/ai/xtts/llm/minio），支持 Nacos 服务发现（`internal/discovery/resolver.go`），但传输层仍是 HTTP。

### 1.4 gRPC 基础设施就绪度

- **proto 契约**：`proto/` 下 2 个文件（emotion_llm.proto / emotion_query.proto），生成代码在 `shared/pkg/emotionllm/` + `shared/pkg/emotionquery/`
- **拦截器套件**：`shared/pkg/grpcinterceptor/` 已完整实现 auth / client / retry / server / stream / tracing / userid / logging / recovery，全部有单元测试
- **gRPC 版本**：`google.golang.org/grpc v1.80.0`（BFF go.mod）
- **健康检查**：ai-svc gRPC server 已挂 `grpc.health.v1.Health`，有集成测试

---

## 二、关键决策点（需用户拍板）

### 决策 A：BFF → 4 个下游 svc 是否全量改 gRPC？

**支持全量 gRPC 的理由**：
- 决策 4 明确"内部 svc-to-svc = gRPC"，BFF→下游属于内部调用
- gRPC 优势：强类型契约（.proto 单一事实源）、二进制序列化（性能/带宽）、流式支持（SSE/聊天流可走 gRPC stream）、拦截器统一（tracing/auth/retry 已在 shared 包就绪）
- 当前 HTTP downstream client 手写 JSON 序列化 + 错误处理，每个 svc 一份，易漂移

**反对/需谨慎的理由**：
- BFF 是**边界聚合层**，职责是协议转换（外部 REST ↔ 内部），HTTP 聚合有其合理性：REST 语义直观、缓存友好、降级简单、与 APISIX 网关链路一致
- BFF→下游是"边缘→内部"，不是"内部→内部"纯服务间调用；决策 4 的"内部 svc-to-svc"是否包含 BFF 有歧义
- 全量改造工作量大（4 个 svc × 各自接口定义 proto + 生成代码 + server 实现 + client 改写 + 测试），且 BFF 的 HTTP downstream 已有 Nacos 发现 + 重试，稳定性尚可
- ai-svc 已经同时暴露 HTTP（:8891 Gin）和 gRPC，BFF 对 ai-svc 是"HTTP downstream + gRPC emotion query"双轨——说明混合模式是可接受的

**建议方案（分阶段，不一次性全量）**：

| 阶段 | 范围 | 理由 |
|---|---|---|
| **Phase 1** | chat-svc 全接口 gRPC 化（BFF→chat-svc 改 gRPC） | 聊天是最高频路径，且 chat-svc→ai-svc 已有 gRPC 基础，chat-svc 加 gRPC server 成本最低；SSE 聊天流可走 gRPC server stream |
| **Phase 2** | user-svc gRPC 化（BFF→user-svc 改 gRPC） | 认证/用户信息调用频繁，强类型契约收益大 |
| **Phase 3** | assessment-svc + analytics-svc gRPC 化 | 读多写少，优先级低；可保留 HTTP 作为兼容 |
| **不做** | ai-svc → FER/SenseVoice/XTTS | FastAPI 模型服务改 gRPC 成本高收益低，保持 HTTP；AI profile 按需启用，调用量低 |

每个 svc 改 gRPC 时保留 HTTP 端口（兼容 dev 调试 / Postman），gRPC 端口独立（如 chat-svc HTTP :8890 + gRPC :8890g），BFF 配置切换。

### 决策 B：gRPC 端口分配规范

当前 ai-svc HTTP :8891 + gRPC 同进程不同端口（需确认 gRPC 端口）。建议统一规范：

| svc | HTTP | gRPC（建议） |
|---|---|---|
| user-svc | :8888 | :8889?（与 assessment 冲突，需重新规划）|
| chat-svc | :8890 | :8990 |
| assessment-svc | :8889 | :8989 |
| analytics-svc | :8893 | :8993 |
| ai-svc | :8891 | :8991（当前可能用其他端口，需确认）|

建议 gRPC 端口 = HTTP 端口百位 +1（88xx → 89xx），compose / Nacos 同步注册。

---

## 三、Phase 1 详细方案（chat-svc gRPC 化，作为模板）

### 3.1 任务拆分（TDD 循环）

| PR | 范围 | RED 测试 | GREEN 改动 |
|---|---|---|---|
| **PR-1** | proto 契约定义 | `proto/chat_service.proto` 定义 Conversation / Message 服务 + 生成代码 + `shared/pkg/chatservice/` 类型测试 | 写 proto + `gen.sh` 生成 |
| **PR-2** | chat-svc gRPC server 骨架 | `grpcserver/server_test.go` 断言服务注册 + 拦截器链 + health check | 复用 ai-svc grpcserver 模式，挂 shared grpcinterceptor |
| **PR-3** | CreateConversation / SendMessage gRPC 实现 | 集成测试：真 gRPC client 调 server → 断言 DB 写入 + Kafka 事件 | handler 逻辑复用现有 logic 层，gRPC handler 调 logic |
| **PR-4** | GetConversations / GetMessages / DeleteConversation gRPC 实现 | 同上 | 同上 |
| **PR-5** | SendMessage 改 gRPC server stream（SSE 流） | 集成测试：client 接收 stream → 断言 token 序列 | 现有 SSE 逻辑迁移到 gRPC stream |
| **PR-6** | BFF chat downstream 改 gRPC client | BFF 集成测试：gRPC client 调 chat-svc → 断言响应 | `web-bff/internal/chat.go` 改 gRPC，保留 HTTP 作为 fallback |
| **PR-7** | compose / Nacos 注册 gRPC 端口 + 文档 | smoke 断言 gRPC 端口可达 + health check 通过 | compose 加端口映射 + Nacos 注册 gRPC metadata |

### 3.2 兼容性策略

- chat-svc **同时保留 HTTP 和 gRPC**：HTTP 用于 dev 调试 / Postman / 旧客户端；gRPC 用于 BFF 生产路径
- BFF 配置 `ChatService.Transport = "grpc" | "http"`，默认 grpc，可降级 http
- proto 字段编号严格管理，加字段不破坏旧 client
- 错误映射：gRPC status code → BFF HTTP status code（统一在 shared 包做映射 helper）

### 3.3 验收条件（DoD）

| # | 项 | 验证方式 |
|---|---|---|
| 1 | chat-svc gRPC server 启动 + health check 通过 | `grpcurl -plaintext localhost:8990 grpc.health.v1.Health/Check` |
| 2 | BFF 经 gRPC 调 chat-svc 全接口通 | e2e：登录 → 创建会话 → 发消息 → 拉消息列表 → 删除会话 |
| 3 | SSE 聊天流经 gRPC server stream 正常 | e2e：发消息 → 前端收到流式 token |
| 4 | HTTP 端口仍可用（兼容） | `curl http://localhost:8890/api/v1/conversations` 200 |
| 5 | SkyWalking trace 跨 BFF→chat-svc gRPC 连续 | SkyWalking UI 拓扑出现 gRPC 边 |
| 6 | `go test ./...` 全绿 + 新测试覆盖 gRPC handler | CI |
| 7 | Nacos 同时注册 HTTP + gRPC 端口 | Nacos console 查看实例 metadata |

---

## 四、Phase 2/3 概要

### Phase 2 — user-svc gRPC 化

- proto：`user_service.proto`（Login / Register / GetProfile / UpdateProfile / RefreshToken / Logout）
- 注意：JWT 签发逻辑在 user-svc，gRPC 调用需透传 X-User-Id metadata（shared grpcinterceptor userid 已支持）
- BFF auth handler 改 gRPC client
- 工作量：约 Phase 1 的 60%（无 stream，接口简单）

### Phase 3 — assessment + analytics gRPC 化

- assessment：`survey_service.proto`（ListSurveys / GetSurvey / SubmitSurvey）
- analytics：`analytics_service.proto`（GetDailyReport / GetTrend / GetUserBehavior）
- 这两个 svc 读多写少，可最后做；也可保留 HTTP 作为最终决策（如果用户认为 BFF→读服务 HTTP 足够）
- 工作量：各约 Phase 1 的 40%

---

## 五、风险与缓解

| 风险 | 缓解 |
|---|---|
| 全量改造期间 HTTP + gRPC 双轨维护成本 | 每个 svc 改造周期控制在 1 个 sprint；改造完成后 HTTP 端口标记为 deprecated，下一个大版本移除 |
| gRPC 错误码与 HTTP status 映射不一致 | shared 包加 `grpcstatus.ToHTTPStatus` helper，BFF 统一使用；加映射表单元测试 |
| proto 契约变更影响多服务 | proto 文件放 `proto/` 仓库根目录，生成代码放 shared；变更走 PR + 兼容性测试（加字段不破坏旧 client）|
| gRPC 端口与现有端口冲突 | 统一 89xx 段，compose / Nacos / firewall 同步更新；启动时端口冲突 fail-fast |
| Nacos 服务发现同时支持 HTTP + gRPC | Nacos 实例 metadata 加 `grpc_port` 字段，BFF resolver 按 transport 选择 |
| 模型服务（FER/SenseVoice/XTTS）改 gRPC 成本高 | 明确不做，保持 HTTP；AI profile 调用量低，HTTP 足够 |
| BFF 是边界层，全量 gRPC 是否过度设计 | Phase 1 先做 chat-svc 验证收益，再决定是否推进 Phase 2/3；用户可在 Phase 1 后拍板停止 |

---

## 六、不在本计划范围

- APISIX → BFF（HTTP，网关到 upstream，不改）
- 前端 → APISIX（HTTP REST，外部 API，不改）
- ai-svc → FER/SenseVoice/XTTS（HTTP FastAPI，明确不做）
- Kafka 事件序列化改 Protobuf（见 kafka-reliability-gaps.md §3.5，独立计划）
- gRPC mTLS（决策 18 已记录，当前 dev 用 insecure，prod 再做）
- 服务网格（Istio / Linkerd），超出当前架构范围

---

## 七、调研依据

> 满足 AGENTS.md §〇 "写文档前先调研"

| 文件 | 调研内容 |
|---|---|
| `proto/emotion_llm.proto` + `proto/emotion_query.proto` | 现有 proto 契约（2 个）|
| `emotion-echo-ai-svc/internal/grpcserver/server.go:1-80` | ai-svc gRPC server 实现模板（拦截器链 + health + user ID）|
| `emotion-echo-chat-svc/internal/grpcclient/ai_client.go` | chat-svc gRPC client（UpsertNeutralEmotion，dev fallback）|
| `emotion-echo-web-bff/main.go:192-206` | BFF gRPC dial ai-svc（emotion query）|
| `emotion-echo-web-bff/internal/config/config.go:17,116-125` | BFF 4 个 downstream BaseURL 配置（全 HTTP）|
| `emotion-echo-web-bff/internal/downstream/` | BFF HTTP downstream client（user/chat/analytics/assessment）|
| `emotion-echo-web-bff/internal/discovery/resolver.go` | BFF Nacos 服务发现（当前只支持 HTTP BaseURL）|
| `emotion-echo-shared/pkg/grpcinterceptor/` | 完整拦截器套件（auth/client/retry/server/stream/tracing/userid），全部有测试 |
| `emotion-echo-ai-svc/integration_test/grpc_health_integration_test.go` | gRPC health 集成测试模板 |
| `docs/architecture/decisions.md` 决策 4/5 | gRPC 架构决策原文 |
| `docs/stages/stage-10-grpc-migration.md` | gRPC 迁移早期 stage |
| `docs/stages/stage-19-ai-svc-grpc-server.md` | ai-svc gRPC server 落地记录 |
