# Stage 84 — chat-svc events 包 3 个存量测试失败修复（测试适配 Stage 73 Protobuf 契约）

> 日期：2026-09-12
> 类型：test（无生产代码改动）
> 关联：Stage 73（Kafka §1.5 Protobuf 迁移）、Stage 83 §五 open 表第 3 项

---

## 一、背景

Stage 73 把 chat-svc 事件 payload 从 JSON 换成 Protobuf（`MarshalChatEvent` +
`ChatEventEnvelope` oneof），并立契约：**Data 必须是 typed payload，按 Type 恰好
设置 oneof 其一；未知/nil Data 在 marshal 阶段报错（不静默丢载荷）**。
`kafka_publisher_test.go` 有 3 个测试未跟上该契约，自 Stage 73 起持续红：

| 失败测试 | 根因 |
|---|---|
| `Publish_TopicIsForwarded` | Event 无 Data → marshal 报 `unsupported Data type <nil>`，根本没到 SendMessage |
| `Publish_SendMessageError_Propagates` | 同上；且本测试要验证的是 broker 错误透传，构造的 Event 却过不了 marshal |
| `Publish_ValueIsValidJSON` | 仍在断言旧 JSON payload 契约 |

## 二、修复内容（仅测试文件）

1. `Publish_ValueIsValidJSON` → 重命名 `Publish_ValueIsValidProtoEnvelope`：
   断言 payload 可 `proto.Unmarshal` 为 `ChatEventEnvelope`（id/type/source +
   oneof message_created 已设置），且 `content-type` header =
   `application/x-protobuf`（consumer 双写窗口依赖 header 识别新旧格式）。
2. `Publish_SendMessageError_Propagates` / `Publish_TopicIsForwarded`：Event 补
   `Data: MessageCreatedData{...}`，使其通过 marshal、真正触达 mock broker。
3. 新增回归锁 `Publish_MarshalError_NotSentToBroker`：nil Data 时 Publish 必须
   在触达 broker **之前**返回 marshal 错误（checker 断言 SendMessage 未被调用）——
   正是本批排查中"错误发生在哪一层"的混淆点，落成永久断言。
4. 文件头 coverage matrix 注释同步更新。

**设计判断**：不改生产代码——nil Data 报错是 `proto_marshal.go` §契约明文
（"不静默丢载荷"），且 `proto_marshal_test.go` 已锁定该行为；红的原因是测试
过期，不是实现 bug。

## 三、验收

- `go test ./internal/events/...` → ok（含新增回归锁）
- `go vet ./...` → 0 err
- `go test ./...`（chat-svc 全量）→ **全部 ok**，合并门槛 `go test ./...` 恢复绿色

## 四、本批未做（open）

| 项 | 说明 |
|---|---|
| 趋势报告意图维度 | Stage 85 推进中（本期主目标） |
| 其余 open 项 | 与 Stage 83 §五一致，不变 |

## 五、调研依据

- 已读：`emotion-echo-chat-svc/internal/events/{kafka_publisher.go, kafka_publisher_test.go,
  proto_marshal.go, events.go, proto_marshal_test.go}`、`emotion-echo-shared/pkg/chatevents/chat_events.pb.go`
- 已查：Stage 83 §五 open 表、Stage 73 迁移报告（Protobuf 契约与 content-type header）
- 命令证据：修复前后 `go test ./internal/events/...` 输出（3 FAIL → ok）

---

> 最后更新：2026-09-12 by Stage 84 实施 session
