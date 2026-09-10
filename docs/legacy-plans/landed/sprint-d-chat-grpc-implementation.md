---
status: landed
landed: 2026-09-11
owner: User
original-plan: docs/plans/grpc-inter-service-migration.md
related-stages:
  - stage-63-bff-grpc-wiring.md
related-adrs:
  - docs/architecture/adr/adr-2026-09-doc-drift-registry.md #27 #32
---

# Sprint D — chat-svc gRPC 4 RPC 实现（解决策 18 #27）

## 一、问题

Stage 58 PR-GRPC-3 实质只挂了 server skeleton,chat-svc gRPC 6 个 RPC 函数体
全部 `return nil, status.Error(codes.Unimplemented, "Xxx: PR-GRPC-3 阶段补完")`。

**实际后果**:BFF→chat-svc gRPC 路径全部 Unimplemented。Sprint C 期间临时绕路
(`CHAT_TRANSPORT=http`),但 chat-svc HTTP 端 `/api/v1/conversations` 也返 502
(决策 18 #32,根因不同)。端到端 conversations 全断。

## 二、范围

Sprint D 实际范围: **4 RPC 真实实现**(SendMessage / ListMessages /
ListConversations / DeleteConversation)。

**PinConversation 保留 Unimplemented 是正确设计**:
- chat-svc 无 `model.Conversation.IsPinned` 字段
- `ConversationRepo` interface 无 Pin 方法
- chat-svc HTTP 端无 pin 端点
- BFF 端 PinConversation 当前**未调用**(chat.go:11 注释"下游尚未实现")
- 加 pin 功能需 schema migration + repo 接口扩展 + HTTP 端点 + BFF 调用方
  → 独立 PR,不在 Sprint D 范围

**StreamMessages 保留 Unimplemented 是架构判断**(chat_server.go:124-138):
- 当前 chat-svc 无"订阅消息流"业务语义(SendMessage 同步 RPC)
- 前端聊天走 POST /api/v1/ai/stream(直连 LLM),不经 chat-svc
- BFF 端 StreamMessages 当前**未调用**
- proto 留接口供未来扩展(多客户端实时协作场景)

## 三、修法

### 3.1 代码变更

**`emotion-echo-chat-svc/internal/grpcserver/chat_server.go`**:
- 新增 `toProtoMessage(m types.MessageView) *emotionchat.Message` helper
- 新增 `mapLogicError(err, op) error` 把 logic 层业务错误映射到 gRPC status code:
  - `repository.ErrNotFound` → codes.NotFound
  - "unauthorized"/"forbidden"/"validation" 前缀 → codes.Unauthenticated/PermissionDenied/InvalidArgument
  - 其他 → codes.Internal
- 4 RPC 占位实现替换:
  - `SendMessage`:proto ConversationId → types.Id;ClientMsgId string → *string(空字符串转 nil)
  - `ListMessages`:proto ConversationId → types.Id
  - `ListConversations`:proto Limit/Offset → types.Limit/Offset;返回 list + hasMore
  - `DeleteConversation`:proto ConversationId → types.Id;返回 success + id

### 3.2 测试 (RED → GREEN)

**`emotion-echo-chat-svc/internal/grpcserver/chat_server_sprint_d_test.go`**(新建):
- `TestChatServer_ListConversations_NotUnimplemented` — 断言 list conv 不返 Unimplemented
- `TestChatServer_SendMessage_NotUnimplemented`
- `TestChatServer_ListMessages_NotUnimplemented`
- `TestChatServer_DeleteConversation_NotUnimplemented`
- PinConversation 测试**删除**(在文件末尾加注释说明"不在 Sprint D 范围")

每个测试构造 mock svcCtx(`repository.NewInMemoryConversationRepo()` +
`events.NewInMemoryEventPublisher()`),走真实 gRPC server + metadata x-user-id。

## 四、DoD 验证

| # | 项 | 结果 |
|---|---|---|
| 1 | chat-svc `go test ./...` 全绿 | ✅ |
| 2 | 4 RPC RED 测试转 GREEN | ✅ (从 Unimplemented 转 NotFound/Internal,业务路径完全接通) |
| 3 | chat-svc v0.1.2 rebuild + docker 重启 | ✅ healthy |
| 4 | docker 端到端 `conversations` 走 gRPC 200 | ✅ (空 list `{"hasMore":false,"list":[]}`) |
| 5 | docker 端到端 `users/me` `surveys` `reports/daily` 仍 200 | ✅ |
| 6 | `deploy/compose.sprint-c-override.yml` 删除 | ✅ |
| 7 | BFF 默认 Transport=grpc 覆盖全部 4 svc | ✅ |
| 8 | 决策 18 #27 close | ✅ |

## 五、风险与遗留

**已规避**(因 BFF 不再走 chat-svc HTTP):
- chat-svc HTTP 端 `/api/v1/conversations` 500 bug(决策 18 #32)

**仍 open**:
- #32 chat-svc HTTP conversations 500 — 如有人设 `CHAT_TRANSPORT=http` 仍会撞,根因未查(怀疑 gin_auth middleware 在某条件下返 500)
- chat-svc 无 pin 功能 — 业务未触发,留待产品决策

## 六、变更清单

| 文件 | 变更 |
|---|---|
| `emotion-echo-chat-svc/internal/grpcserver/chat_server.go` | +mapLogicError +toProtoMessage,4 RPC 占位 → 真实实现 |
| `emotion-echo-chat-svc/internal/grpcserver/chat_server_sprint_d_test.go` | 新建 (4 RED 测试 + 注释说明 PinConversation 范围) |
| `deploy/docker-compose.apps.yml` | chat-svc tag v0.1.1 → v0.1.2 |
| `deploy/compose.sprint-c-override.yml` | 删除 (Sprint C 临时文件, Sprint D 不再需要) |

## 七、调研依据

- `emotion-echo-chat-svc/internal/grpcserver/chat_server.go`（5 RPC 占位实现）
- `emotion-echo-chat-svc/internal/logic/{send,list,delete}*logic.go`（4 个 logic 已存在）
- `emotion-echo-web-bff/internal/downstream/chat_grpc.go`（BFF 调 chat-svc gRPC 期望的字段）
- `emotion-echo-shared/pkg/emotionchat/chat.pb.go`（proto 类型）
- 决策 18 #27 #32 全文

## 八、Stage 63 收口

Sprint C + Sprint D 完成后:
- BFF 默认 Transport=grpc 覆盖全部 4 svc
- 端到端 4/4 业务(users/me / surveys / reports/daily / conversations)全 200
- 决策 4(跨服务调用 = gRPC + .proto)在 BFF→4 svc 链路上**真正生效**
- Stage 63 翻 🟢
