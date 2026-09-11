---
status: in-progress
priority: high
owner: User
created: 2026-09-12
related-adrs:
  - adr-2026-09-decision-4-closure.md §八（后续 sprint backlog）
  - adr-2026-09-doc-drift-registry.md（决策 18）
related-plans:
  - docs/plans/kafka-reliability-gaps.md §1.4/§1.5（Kafka P2 细则）
  - docs/plans/nacos-enablement-dev.md（Nacos 全链路细则）
  - docs/plans/todo-pile-2026-09-04.md §D6（Helm 对齐细则）
related-stages:
  - stage-71-c8-routes-alignment-2026-09-11.md §六（本批未做清单）
---

# Plan — 2026-09-12 backlog 排序（4 项，按推荐执行顺序）

> 来源：Stage 71 收口报告 §六 + 决策 4 ADR §八。本计划把散在各处的 4 项 open
> backlog 收拢为一个执行序列，按「立刻可做 / 影响面 / 链路耦合」排序。
> 每项落地后在本文件勾销 + 迁 stage 收口报告。

## 执行顺序

| # | 项 | 来源 | 工作量 | 状态 |
|---|---|---|---|---|
| 1 | chat-svc PinConversation + UpdateConversation 两个 RPC（含 schema migration + BFF 接通 + 前端 store 恢复真实 API） | 决策 4 ADR §八 + Stage 71 §六 | 1~2 天 | ✅ **已落地（Stage 72）** |
| 2 | Kafka P2：consumer lag 监控 + 事件 Protobuf 迁移 | kafka-reliability-gaps.md §1.4/§1.5 | 3~4 天 | 🔍 已调查（§1.4 骨架已落地需纠偏，§1.5 确实未做） |
| 3 | Nacos dev 模式全链路启用（ephemeral 实例 30s 被踢 / Heartbeat 未真正触发） | nacos-enablement-dev.md | 1~2 天 | 🔧 **部分落地（Stage 72）**：PR-1 验收达成 |
| 4 | Helm chart ↔ compose dev 全面对齐 | todo-pile §D6 | 1 天 | 🔍 已调查（偏差清单见 §项4；configmap 端口错值已修） |

---

## 项 1 · chat-svc PinConversation + UpdateConversation（本轮实施）

### 上下文（假设清单）

本文假设（与代码核实一致）：
- proto `chat.proto` 已有 `PinConversation`（占位返 Unimplemented），**没有** `UpdateConversation` → 需加 proto + regen
- chat-svc `model.Conversation` 无 `pinned` 字段；DDL（`deploy/db/02-create-tables-in-schemas.sql:62-73`）无 `pinned` 列 → 需 migration `003_add_pinned_to_conversations.sql`（db-migrate 容器自动应用）+ DDL 同步
- BFF `chat_handler.go:53-62` PATCH 端点临时返 501；前端 `stores/conversation.ts` `updateConversationTitle` 降级为本地更新，`togglePinConversation` 调 knownOrphans 路径
- BFF→chat-svc 走 gRPC（CHAT_TRANSPORT=grpc 默认），chat-svc HTTP 端点不在本批范围

### 契约

- `UpdateConversation`：PATCH 语义，当前仅支持改 `title`（前端唯一用途）；owner 校验复用 DeleteConversation 模式；NotFound → codes.NotFound（grpcerr 已注册）
- `PinConversation`：请求/响应 proto 已定义；补 `pinned BOOLEAN DEFAULT FALSE` 列；repo 加 `SetPinned`（InMemory + Postgres）；logic 做 owner 校验
- BFF：PATCH /api/v1/conversations/:id 接真实 gRPC；新增 POST /api/v1/conversations/:id/pin 路由；ChatClient interface 加 `UpdateConversation`
- 前端：`updateConversationTitle` 恢复真实 `patch()` 调用；`togglePinConversation` 从 knownOrphans 移回正式路由

### TDD

RED（grpcserver + BFF handler 测试先行，断言真实行为而非 Unimplemented）
→ GREEN（proto regen → repo → logic → grpcserver → BFF → 前端）
→ REFACTOR。

### 合并门槛

- `go test ./...`（chat-svc + web-bff + shared）+ `go vet ./...` 全绿
- §契约 5：schema 与写入端一致性（pinned 列 DDL ↔ migration ↔ GORM model 三方一致；布尔列不涉 VARCHAR enum）
- dev 模式 §契约 1-6 smoke：docker 全停状态，项 1 落地后按需 `docker compose -f deploy/docker-compose.apps.yml up -d` 拉起复跑（本批不改事件发布链 / analytics SQL / 报表端点，仅加列 + 2 RPC）

---

## 项 2 · Kafka P2（下轮）

细则见 `kafka-reliability-gaps.md` §1.4（consumer lag 监控）与 §1.5（outbox payload JSON→Protobuf 迁移）。

**Stage 72 调查结论（2026-09-12）**：
- §1.4 lag 监控**骨架已落地**（kafka-exporter + Prometheus scrape + lag 告警规则均已存在，计划文档原记录过期，已在 §1.4 纠偏）。残余：Grafana lag 面板确认 + 可选的 consumer 进程级指标。
- §1.5 Protobuf 迁移**确实未做**：链路仍全 JSON（producer `kafka_publisher.go:42` / consumer `consumer.go:101,221`），`chat_events.proto` 不存在；但 `shared/pkg/eventrow` 已统一事件→DB 行映射，消解部分镜像风险。迁移范围：proto 定义 → producer/consumer 双写切换 → event_type 命名统一（eventrow 头注释登记的 chat-svc `message.created` vs analytics-svc 规范化值不一致，属数据迁移 PR）。
- DLQ（Stage 70）topic=`chat-events-dlq`，依赖 Kafka auto-create；infra compose 无 KAFKA_CREATE_TOPICS，可留意。

## 项 3 · Nacos dev 全链路（部分落地）

细则见 `nacos-enablement-dev.md`。

**Stage 72 调查 + 修复（2026-09-12）**：
- 原"30s 被踢"归因中，**0.0.0.0 Heartbeat 覆盖 bug 已在 Stage 62 PR-3.4 修复**（Register/Unregister/Heartbeat 三处统一 registerHost()，nacos_register.go:360 注释自证）——计划文档对此的"待修"描述过期。
- **集成测试 fixture 从未跑通过**：testcontainers 随机端口映射 vs SDK「服务端口+1000」gRPC 拨号矛盾 → 永远 `client not connected, current status:STARTING`。已修：固定绑定 18848/19848 保持偏移。
- **Register STARTING race 已修**：SDK gRPC 通道异步建立，Register 立即调用报瞬时错误 → 加 500ms×30 退避重试。
- **Discover 两处契约修复**：SDK 空 hosts 返 error → 映射空列表；服务名剥 `GROUP@@` 前缀。
- **实测**：RegisterAndDiscover / HeartbeatKeepsInstanceAlive（30s 存活 = PR-1 验收）/ UnregisterRemoves 三集成用例**首次全绿**。
- **已登记残余**：SDK SelectInstances 读本地缓存，服务端空列表 push 有延迟保护 → 注销后 Discover 陈旧 30s+（Unregister 测试改用 HTTP API 断言服务端真相）。BFF 走 Discover（PR-2）时需注意优雅退出短窗口仍可发现已停实例。

## 项 4 · Helm chart 对齐（已出偏差清单，机械修批次待排期）

**Stage 72 调查结论（2026-09-12）**：chart 自 Stage 32 后未跟 compose 演进，23 个 subchart 零处 NACOS_*，业务 svc tag 全停 v0.1.0（compose 已 v0.1.2~v0.1.11）。已修：web-bff configmap analytics 端口 8904→8893。其余偏差清单：

| 类别 | 偏差 |
|---|---|
| image tag | user v0.1.4 / chat v0.1.8 / analytics v0.1.5 / assessment v0.1.2 / ai v0.1.5 / bff v0.1.11（chart 全 v0.1.0） |
| Stage 70 env | KAFKA_ENABLED / KAFKA_DLQ_TOPIC chat+analytics+ai 缺失（K8s 下会走 DevEventPublisher 而非 Kafka） |
| gRPC 端口 | 5 个 svc 的 gRPC 端口（8887/8892/8885/8886 + bff 4 个 *_SVC_GRPC_ADDR）未暴露 |
| NACOS_* | 23 个 subchart 全缺 |
| 錯值 | analytics-svc chart 5 个 env 名代码不读（臆造）；web 的 apiBaseUrl 绕过网关；fer repository 应为 fer-tflite；apisix chart 引用不存在的 dashboard 镜像 |
| 平台件 | 无 MinIO chart；无 db-migrate Job；无 apisix-seed Job（etcd emptyDir 重启即清） |
| 文档失真 | compose.dev.yml 覆盖项大部分已被 apps.yml 基线内联（冗余非失效） |

---

## 调研依据

- 已读：proto/chat.proto、emotion-echo-chat-svc/{internal/grpcserver/chat_server.go, internal/logic/deleteconversationlogic.go, internal/repository/conversation_repository.go, internal/model/conversation.go, internal/types/types.go, migrations/}、emotion-echo-web-bff/internal/{handler/chat_handler.go, downstream/chat_grpc.go}、emotion-echo-web/app/stores/conversation.ts、emotion-echo-web/app/lib/apiRoutes.ts、deploy/db/02-create-tables-in-schemas.sql、deploy/docker-compose.apps.yml
- 已查：ADR 决策 4 §八、decisions.md 决策记录尾表、stage-71 §六、kafka-reliability-gaps.md、nacos-enablement-dev.md、todo-pile §D6
- smoke 现状：docker 20 容器全 Exited（9 小时前），本批单测不依赖；dev 模式契约项 1 落地后复跑
