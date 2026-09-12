# Stage 72 — chat-svc Pin/Update RPC + Nacos discovery 修复 + 4 项 backlog 调查收口

> 日期：2026-09-12
> 来源：会话目标"按推荐顺序将 4 项 backlog 加入计划，然后调查并修复"
> 计划：[docs/legacy-plans/landed/backlog-order-2026-09-12.md](../legacy-plans/landed/backlog-order-2026-09-12.md)

## 一、本轮落地汇总

| 项 | 结果 | commits |
|---|---|---|
| 项 1 chat-svc PinConversation + UpdateConversation | ✅ 全链路落地（proto→chat-svc→BFF→前端） | f56ad8f / 0507596 / 42ab6d6 / bb494fb |
| 项 1 集成测试（§契约 5） | ✅ testcontainers 真实 Postgres + migration 幂等 | (见 integration commit) |
| 项 2 Kafka P2 调查 | ✅ 纠偏：§1.4 骨架已落地；§1.5 确实未做 | docs |
| 项 3 Nacos | ✅ PR-1 验收达成：3 集成用例首次全绿 + 4 处代码修复 | (见 shared commit) |
| 项 4 Helm 对齐 | ✅ 机械对齐批次落地（tags/NACOS/KAFKA/gRPC 端口/错值），helm lint+template 断言全过 | docs + charts |

## 二、项 1：PinConversation / UpdateConversation（决策 4 ADR §八 收口）

### 2.1 TDD 时间线

- **RED**：`chat_server_stage72_test.go` 表驱动 10 用例（pin/unpin/改名/空标题/越权/NotFound/缺鉴权）。PinConversation 原为 Unimplemented 占位；UpdateConversation proto 不存在。
- **GREEN**：proto 加 `UpdateConversation` RPC + `PinConversationRequest.IsPinned` + `Conversation.IsPinned` → regen；model.Conversation + Pinned；repo SetPinned/UpdateTitle（InMemory+Postgres，均刷 updated_at）；logic 两文件（owner 校验，不发 outbox——置顶/改名非用户行为事件）；grpcserver 两方法真实实现；BFF ChatClient 接口 + gRPC client + handler PATCH 替换 Stage 71 的 501 暂存 + 新增 POST /:id/pin；前端 updateConversationTitle 恢复真实 patch、pin 从 knownOrphans 转正。

### 2.2 schema（§契约 5）

- migration `003_add_pinned_to_conversations.sql`（ALTER TABLE ADD COLUMN IF NOT EXISTS，幂等）——既有库 db-migrate 容器自动应用
- `deploy/db/02-create-tables-in-schemas.sql` DDL 同步（全新安装）
- 集成测试：旧结构上连跑两次 migration SQL 幂等不报错 + SetPinned/UpdateTitle 真实持久化验证

### 2.3 顺手修复

- **Stage 71 遗留语法错误**：`conversation.ts:201` 多余 `};`（esbuild 实测编译失败）——Stage 71 收口报告未发现
- 既有集成测试 messages 内联 DDL 缺 `client_msg_id` 列（模型有列测试表没有 → GORM 插入 42703），修复后 chat-svc 全集成套件 147s 全绿

### 2.4 验证

- chat-svc `go test ./...` 全绿（新增 10 用例）+ `go vet` 绿
- web-bff `go test ./...` 全绿（新增 5 handler 用例 + 路由契约 32→33 条）+ `go vet` 绿
- shared `go test ./...` 全绿
- chat-svc `go test -tags integration ./integration_test/` 全绿（147s）
- 前端 `vitest apiRoutes.test.ts` 3/3 绿（pin 路由转正后契约测试通过）
- dev 模式 §契约 1-6 smoke 未跑（docker 全停，本批不改事件链/analytics SQL/报表端点）；pinned 布尔列不涉 VARCHAR enum，§契约 5 由集成测试覆盖

## 三、项 3：Nacos discovery（PR-1 验收达成）

### 3.1 根因链（实测）

1. **fixture bug（最根）**：testcontainers 把 8848 映射到随机宿主端口，SDK v2 按「服务端口+1000」拨 gRPC → 拨到无映射的宿主端口 → 连接永远 STARTING → Register 必报 `client not connected`。容器间互拨不受影响，故 dev compose 正常而集成测试从未通过。
2. **Register STARTING race**：SDK gRPC 通道异步建立，NewNamingClient 返回后立即 Register 有竞态（1 的显性表现）。
3. 历史 bug（0.0.0.0 Heartbeat 覆盖）已由 Stage 62 PR-3.4 修复，非当前问题——调查中核实的文档过期点。

### 3.2 修复（emotion-echo-shared）

- fixture 固定绑定 18848/19848 保持 +1000 偏移
- Register 对 `client not connected` 做 500ms×30 退避重试
- Discover：SDK 空 hosts error → 空 slice（契约对齐）；convertInstances 剥 `GROUP@@` 前缀
- Unregister 集成测试改用 HTTP open API 断言服务端真相

### 3.3 已登记残余（新发现）

SDK SelectInstances 读 serviceInfoHolder 本地缓存（push 刷新），服务端对空列表 push 有延迟保护 → 注销后 Discover 陈旧 30s+。PR-2（BFF 走 Discover）前需评估。

## 四、项 2 / 项 4 调查结论

见 [backlog-order-2026-09-12.md](../legacy-plans/landed/backlog-order-2026-09-12.md) §项2（Kafka P2 纠偏后残余）/ §项4（Helm 偏差清单表格）。

## 五、调研依据

- 已读：proto/chat.proto、chat-svc {grpcserver/logic/repository/model/types/migrations}、web-bff {downstream/chat*.go, handler/chat_handler.go, viewmodel.go, main_test.go}、web {stores/conversation.ts, lib/apiRoutes.ts}、shared {pkg/discovery/nacos_register.go, internal/integrationtest/nacos.go}、deploy {apps/infra compose, db DDL, prometheus}
- 已查：ADR 决策 4 §八、decisions.md、stage-71 §六、kafka-reliability-gaps.md、nacos-enablement-dev.md、todo-pile §D6、charts/ 23 subcharts 与 compose 三方 diff
- smoke：docker 20 容器全 Exited（未拉起）；替代验证 = testcontainers Postgres/Nacos 集成测试全绿 + 全量单测/vet 绿
- 命令证据：`go test -tags integration ./integration_test/` ok 147s；`go test -tags=integration ./pkg/discovery/ -run Integration` 3/3 PASS；`npx vitest run app/lib/apiRoutes.test.ts` 3 passed

## 六、本批未做（open）

| 项 | 说明 |
|---|---|
| chat-svc StreamMessages | 业务未触发（架构判断见 chat_server.go 注释），维持 Unimplemented |
| chat-svc/bff 容器 rebuild | 镜像仍是 v0.1.8/v0.1.11，dev 栈拉起前需 rebuild 才能让 PATCH 端到端生效 |
| dev 模式全量 §契约 1-6 | 待 docker 栈拉起后补跑（含 PATCH/pin 容器端到端） |
| Kafka P2 §1.5 Protobuf 迁移 | 范围已明确，3~4 天独立批次 |
| Helm 残余 | nacos 安装 ns 假定 / web apiBaseUrl 走网关 / xtts 镜像源 / MinIO+db-migrate+apisix-seed chart 等价物 / values-prod 同步（见 backlog-order §项4 残余） |
| SDK Discover 缓存陈旧性 | PR-2 前评估（push 空列表延迟保护） |

---

> 最后更新：2026-09-12 by Stage 72 实施 session
> 关联：决策 4 ADR §八、nacos-enablement-dev PR-1、kafka-reliability-gaps §1.4/§1.5、todo-pile §D6
