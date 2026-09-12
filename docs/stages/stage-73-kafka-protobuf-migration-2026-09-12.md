# Stage 73 — Kafka 事件 Protobuf 迁移（§1.5）+ dev 栈 e2e 三 bug 收口

> 日期：2026-09-12
> 来源：Stage 72 收口后验证器指示：实施 kafka-reliability-gaps.md §1.5 批次 + 拉起 dev 栈补跑 §契约 smoke 与 PATCH/pin 端到端
> 计划：[docs/plans/backlog-order-2026-09-12.md](../plans/backlog-order-2026-09-12.md) 项 2

## 一、§1.5 Protobuf 迁移本体

### 1.1 schema 单一事实源

- `proto/chat_events.proto`：`ChatEventEnvelope`（id/type/source/time_unix_milli + **oneof** 3 种 Data：MessageCreatedData / ConversationCreatedData / ConversationClosedData）
- gen.sh 扩展 → `shared/pkg/chatevents` 生成代码
- event_type 契约：type 恒为带点原值（message.created / conversation.created / conversation.closed）

### 1.2 双写窗口设计

- **producer**（chat-svc `KafkaEventPublisher.Publish`）：`MarshalChatEvent` Protobuf 编码 + Kafka header `content-type: application/x-protobuf`；outbox DB 内 payload 保持 JSONB（relay 出站时才转 Protobuf）
- **consumer**（analytics `internal/kafka/proto_decode.go`、ai `internal/consumer/proto_decode.go`）：`DecodeChatEvent` 三级识别——header → 首字节嗅探（`{`=JSON）→ 旧 JSON fallback；迁移窗口内旧消息继续可消费
- **TDD**：RED（MarshalChatEvent / 两端 DecodeChatEvent 函数不存在，编译失败）→ GREEN；每端表驱动含 header/嗅探/JSON fallback/oneof 缺失 4 类用例

### 1.3 event_type 命名统一（核查结论）

analytics consumer 自 PR-A1.4 起已用 `ev.Type` 原值（不再 normalize），与 chat-svc DevEventPublisher 一致——**命名差异实际已收口**，eventrow 包注释的"留作后续数据迁移 PR"描述过期，已修正为留档说明。

## 二、dev 栈端到端暴露并修复的 3 个 bug

| # | bug | 根因 | 修复 | 测试 |
|---|---|---|---|---|
| 1 | outbox 行重试 100 次后 dead（smoke run2 的 3 事件全灭） | relay `publishOne` 裸 `json.Unmarshal` → Data 落 `map[string]interface{}`，`MarshalChatEvent` 拒绝 map | 新增 `events.UnmarshalChatEventJSON`（按 Type 反序列化到具体 struct），relay 改用之 | `proto_marshal_relay_test.go`（3 类型 + protobuf 往返） |
| 2 | pin 置顶状态永远 false | 前端 store 发 `{"isTop":bool}`（conversation.ts:217），BFF handler 误读 `isPinned` | BFF handler 改读 `isTop` 传下游 | handler 测试同步 |
| 3 | （非 bug）Windows shell 发 UTF-8 中文标题乱码 | git-bash curl 编码假象 | 用 python urllib 复测：`改名成功✅` 全链路往返正确 | — |

修复后 rebuild chat-svc/web-bff 镜像并重启，手工把 6 条 dead 行重置 pending → **全部转 sent**。

## 三、端到端验证证据（docker dev 栈）

```
# 容器：infra 9 + app 6 全 healthy；db-migrate Exited(0)（migration 003 pinned 列已应用）
# 镜像 rebuild：chat-svc / analytics-svc / ai-svc / web-bff（build_dev_images.sh ALL OK）

$ python scripts/smoke_data_layer.py
§契约 1 行数=52（≥1 OK）· outbox_sent=52 events=52 正常
§契约 2 distinct_types=6（含 message.created + conversation.created/closed）
§契约 3 analytics_reader 4 视图全 OK
§契约 4 /reports/daily summary 非空 + emotionDistribution.len=2
§契约 5 SKIP（Stage 72 testcontainers 集成测试已覆盖）
§契约 6 SKIP（§1 PASS 证明 Kafka 路径活）
汇总: 11/11 PASS, 0 FAIL

# PATCH/pin 端到端（经 BFF → chat-svc gRPC → Postgres）
create → PATCH title=改名成功✅ (200) → DB title 改名成功✅
pin  {"isTop":true}  → {"isPinned":true} (200)
unpin{"isTop":false} → {"isPinned":false} (200) · DB pinned=f
越权 PATCH user 999 → 403 · 不存在 → 404 · 空标题 → 400

# outbox 终态
sent=52, pending=0, dead=0
```

Protobuf 链路证明：smoke 新事件由新 chat-svc（Protobuf+header）发布、新 analytics/ai consumer 解码落库，outbox 无堆积。

## 四、调研依据

- 已读：kafka-reliability-gaps.md §1.5/§3.5、chat-svc {events/*.go, outbox/relay.go}、analytics {kafka/consumer.go, events/events.go}、ai {consumer/consumer.go, events/events.go}、shared eventrow/mapper.go
- 已查：ADR-19（DevEventPublisher/eventrow/PR-A1.4）、scripts/smoke_data_layer.py 契约清单、scripts/build_dev_images.sh
- 命令证据：4 仓 go build/vet/test 全绿；`python scripts/smoke_data_layer.py` 11/11；curl/python e2e 状态码表；outbox 终态 SQL

## 五、本批未做（open）

| 项 | 说明 |
|---|---|
| §1.4 Grafana lag 面板确认 | kafka-exporter+scrape+告警已在，面板 provisioning 待查 |
| Kafka P3（outbox relay 重试上限已有 dead 机制；告警走 alertmanager） | 见计划 §3.6 |
| Helm 残余（nacos 安装 ns / web apiBaseUrl / xtts / MinIO chart） | backlog-order §项4 残余 |
| dev 栈 web 前端容器 | node:20-alpine 拉取受限未起；不影响契约链路 |
| eventrow 命名段留档 vs 实际删除 | 已改为留档说明，历史 normalize 数据迁移 SQL 见 migrations/002 |

---

> 最后更新：2026-09-12 by Stage 73 实施 session
> 关联：kafka-reliability-gaps.md §1.5、ADR-19、stage-72（项 1/3/4 收口）
