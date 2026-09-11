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
| 1 | chat-svc PinConversation + UpdateConversation 两个 RPC（含 schema migration + BFF 接通 + 前端 store 恢复真实 API） | 决策 4 ADR §八 + Stage 71 §六 | 1~2 天 | 🔨 本轮实施 |
| 2 | Kafka P2：consumer lag 监控 + 事件 Protobuf 迁移 | kafka-reliability-gaps.md §1.4/§1.5 | 3~4 天 | planned |
| 3 | Nacos dev 模式全链路启用（ephemeral 实例 30s 被踢 / Heartbeat 未真正触发） | nacos-enablement-dev.md | 1~2 天 | planned |
| 4 | Helm chart ↔ compose dev 全面对齐 | todo-pile §D6 | 1 天 | planned |

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

细则见 `kafka-reliability-gaps.md` §1.4（consumer lag 监控：Burrow/prometheus kafka_exporter 选型 +lag 告警阈值）与 §1.5（outbox payload JSON→Protobuf 迁移：双写窗口 + consumer 兼容期）。前置：项 1 落地后 chat-svc 事件链稳定。

## 项 3 · Nacos dev 全链路（下下轮）

细则见 `nacos-enablement-dev.md` §一.1.1：SDK ephemeral 实例 30s 被踢的三个候选根因（单节点 Derby 启动慢 / Heartbeat 未真正触发 BeatRequest / namespace 配置），PR-1 先修心跳。前置：docker 栈拉起复现。

## 项 4 · Helm chart 对齐（收尾）

细则见 `todo-pile §D6`：charts/ 与 deploy/compose*.yml 的镜像 tag、env、端口三方 diff。纯比对+修 chart，无代码改动。

---

## 调研依据

- 已读：proto/chat.proto、emotion-echo-chat-svc/{internal/grpcserver/chat_server.go, internal/logic/deleteconversationlogic.go, internal/repository/conversation_repository.go, internal/model/conversation.go, internal/types/types.go, migrations/}、emotion-echo-web-bff/internal/{handler/chat_handler.go, downstream/chat_grpc.go}、emotion-echo-web/app/stores/conversation.ts、emotion-echo-web/app/lib/apiRoutes.ts、deploy/db/02-create-tables-in-schemas.sql、deploy/docker-compose.apps.yml
- 已查：ADR 决策 4 §八、decisions.md 决策记录尾表、stage-71 §六、kafka-reliability-gaps.md、nacos-enablement-dev.md、todo-pile §D6
- smoke 现状：docker 20 容器全 Exited（9 小时前），本批单测不依赖；dev 模式契约项 1 落地后复跑
