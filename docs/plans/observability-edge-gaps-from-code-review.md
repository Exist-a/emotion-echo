---
status: in-progress
priority: medium
owner: TBD
created: 2026-09-13
source: external code review (会话 2026-09-13, not project-internal discovery)
depends-on: []
related-stages:
  - stage-44-observability-sprint-b.md
  - stage-50-e2e-validation.md
  - stage-86-outbox-dead-alert-2026-09-13.md
  - stage-92-kafka-sw8-propagation-2026-09-14.md
related-adrs: []
landed-parts:
  - §A Kafka sw8 透传（chat-svc + ai-svc）—— Stage 92 全 GREEN（2026-09-14）
  - §A-extension analytics-svc consumer sw8 透传 —— Stage 93 候选（无 Tracer 集成，需先加）
---

# Plan - observability edge gaps from code review (non-sprint scope)

## 0. 来源说明

本计划不是项目内部演进识别出来的, 是 **2026-09-13 会话** 中对可观测链路相关代码做细致审查时发现的问题汇总.

来源会话做的是: 基于用户请求把"可观测链路"细致讲清楚(SkyWalking / Prometheus / Loki / Kafka-exporter / alertmanager 五件套), 过程中把相关源码全部读了一遍:
- `emotion-echo-shared/pkg/skywalking/{skywalking.go, tracing.go, gorm_tracing.go, redis_tracing.go}`
- `emotion-echo-shared/pkg/middleware/{gin_skywalking.go, gin_auth.go, limiter.go}`
- `emotion-echo-shared/pkg/grpcinterceptor/{tracing.go, tracing_go2sky.go}`
- `shared/pkg/{metrics, logging, messaging}/`
- `emotion-echo-ai-svc/{main.go, internal/consumer/consumer.go}`
- `emotion-echo-chat-svc/internal/{events, outbox}/`
- `deploy/prometheus/`, `deploy/grafana/`, `deploy/loki/`

下面 6 个问题是**代码事实**(不是文档宣称, 也不是"应该改进"的建议), 按严重度排序.

---

## A. Kafka 异步链路在 SkyWalking 上是断的 (P1, 最值得修复)

> **🟢 2026-09-14 已 landed — Stage 92**（详见
> [`legacy-plans/landed/stage-92-kafka-sw8-propagation.md`](../legacy-plans/landed/stage-92-kafka-sw8-propagation.md)
> 与 [`stages/stage-92-kafka-sw8-propagation-2026-09-14.md`](../stages/stage-92-kafka-sw8-propagation-2026-09-14.md)）。
> Stage 92 PR-1+PR-2 全 GREEN + docker e2e 实证 chat-svc producer 写入完整 sw8（221 chars）；
> ai-svc:v0.1.6 consumer 用 CreateEntrySpan 重建父 trace。
> **唯一残余**：analytics-svc consumer sw8 透传（§A-extension，Stage 93 候选）。

### A.1 问题

chat-svc 用 `kafka_publisher.go:Publish` 发消息时, **只写了 `content-type: application/x-protobuf` header, 没写 SkyWalking sw8 trace header**:

```go
// emotion-echo-chat-svc/internal/events/kafka_publisher.go:48
msg := &sarama.ProducerMessage{
    Topic: topic,
    Key:   sarama.StringEncoder(e.ID),
    Value: sarama.ByteEncoder(body),
    Headers: []sarama.RecordHeader{
        {Key: []byte("content-type"), Value: []byte(ContentTypeHeaderProto)},
    },
}
```

ai-svc 消费时(`internal/consumer/consumer.go:118`)调 `h.Tracer.CreateLocalSpan(sess.Context(), "kafka-consume")` —— **这是新的 trace tree, 没有父 span**.

### A.2 影响

- SkyWalking UI 上: 用户在浏览器发消息 -> 走 chat-svc 完成 -> 异步链路 ai-svc 消费情绪分析是**独立的 trace tree**, 看不到上游 chat-svc 的耗时与上下文
- 跨服务全链路图(chat -> ai-svc -> llm-service)画不出来
- 排查"消息发出去了但情绪分析迟迟不返回"这种跨进程问题只能靠 grep 日志 + 手动对齐 timestamp

### A.3 代码事实佐证

全仓 `grep sw8 / HeaderCarrier / Set` (通过 PowerShell `Select-String -Pattern 'sw8|HeaderCarrier'`) **零命中**. 项目里**没有任何 sw8 跨 Kafka 透传的实现**.

### A.4 修复方案(与项目惯例对齐)

按 SkyWalking 官方 sw8 协议 + Kafka header, chat-svc producer 端加:

```go
// 在 chat-svc/internal/events/kafka_publisher.go Publish() 末尾注入 sw8
// 实际: 从 ctx 抽 sw8 编码(go2sky 自带 TraceContext.Encode()), 写 Headers
// ai-svc consumer: 从 msg.Headers 解 sw8 -> 调 tracer.SetSkyWalkingContext()
//                  然后 CreateExitSpan 或在 CreateLocalSpan 注入父 ID
```

### A.5 工作量

- chat-svc producer 改造 + 测试: 2h
- ai-svc consumer 改造 + 测试: 2h
- analytics-svc consumer 同样模式改造: 1h
- docker 端到端验证: 1h

**总计**: 半天 (5-6h)

### A.6 DoD

1. docker 端到端: 用户发消息 -> SkyWalking UI 上能看到一条跨 chat-svc + ai-svc + llm-service 的完整 trace
2. ai-svc 消费侧的 span 上能看到 `parent_span_id` 标签, 对应 chat-svc producer 侧的 span
3. 没有 sw8 header 的旧消息降级(不破坏)

---

## B. consumer.attempts map 无并发保护 (P2, 理论 race)

### B.1 问题

`ai-svc/internal/consumer/consumer.go:51` 定义:

```go
type ConsumerGroupHandler struct {
    ...
    attempts map[string]int
}
```

在 `ConsumeClaim` 的循环里读写 `h.attempts`:

```go
// consumer.go:107 读
if key := attemptKey(msg); key != "" {
    delete(h.attempts, key)
}
sess.MarkMessage(msg, "")

// consumer.go:113 写
h.attempts[key]++  // handleFailure 里
```

`sarama.ConsumerGroupHandler.ConsumeClaim` 是**单 goroutine 调用**(sarama 内部保证), 所以**目前**没有 race. 但:

- 同一 `ConsumerGroupHandler` 实例的 `ConsumeClaim` 被 sarama 在 partition rebalance 时**顺序执行**, 没问题
- 但如果未来重构(多 goroutine 处理 / 加 worker pool), `attempts` map 会变 race
- 实际生产案例: sarama 在某些版本下 partition 分派会跨 goroutine

### B.2 修复

把 `attempts` 换成 `sync.Map` 或加 `sync.Mutex` 保护.

### B.3 工作量

30 分钟(机械改造 + 测试)

---

## C. metrics path "unmatched" 兜底造成潜在指标污染 (P2)

### C.1 问题

`shared/pkg/metrics/metrics.go:107`:

```go
path := c.FullPath() // 路由模板
if path == "" {
    path = "unmatched"  // <- 所有未匹配请求都打这个 label
}
```

任何未匹配请求(404, 扫描请求, 攻击探测)都会累积 `path="unmatched"` 这个 series. **短期内 series 数不变**(因为 label 都是固定值 "unmatched"), 但**绝对计数会持续上涨** —— Prometheus 长期存储里这块数据没业务价值, 且攻击者扫端口会刷高 counter 让指标"看起来很忙".

### C.2 修复

两种方案:

**方案 1(推荐)**: 跳过 4xx 路由不匹配的情况

```go
path := c.FullPath()
if path == "" {
    // 未匹配路由(404 攻击/扫描): 不计入业务指标, 避免污染
    return
}
```

**方案 2**: 把 `status` 也加进 label, 让 404 / 405 / 503 区分开:

```go
if path == "" {
    path = "unmatched-" + strconv.Itoa(c.Writer.Status())
}
```

### C.3 工作量

30 分钟 + 回归测试 1h

---

## D. GinSkywalkingMiddleware 跳过路径硬编码 (P3)

### D.1 问题

`middleware/gin_skywalking.go:56`:

```go
if path == "/health" || strings.HasPrefix(path, "/internal/") {
    c.Next()
    return
}
```

- `/health`: 跳过合理(k8s liveness probe 不应该产 trace)
- `/internal/`: 跳过 OK 但**硬编码** —— 未来如果加 `/admin/` 或 `/debug/` 还得改这里

### D.2 修复

抽出一个配置项(如 `SkipPathPrefixes []string`)注入到中间件构造函数. 当前阶段可不动.

### D.3 工作量

30 分钟 + 验证

---

## E. AI 模型客户端 env 静默失败 (P2)

### E.1 问题

`ai-svc/main.go:288`:

```go
if svcCtx.FER != nil {
    logging.Printf("[ai] FER client active: %s", c.FER.BaseURL)
} else {
    logging.Printf("[ai] FER disabled (FER.BaseURL empty)")
}
```

`FER.BaseURL == ""` -> `NewFERClient(...)` 不调用 -> client 是 nil. 这是**正确的快速失败** —— 但:

- 配置错误时(env 没注入 / yaml 写错)**只打一行 INFO log**, 不会触发告警
- 多模态分析接口(`/api/v1/multimodal/analyze`)请求到达时返回 503, 但运维不会立刻知道是"FER 不可用"还是"FER 配置错"

### E.2 修复

加 metrics: 每个模型 client 启停时递增 counter, **配置错误也算 init failed**:

```go
// 在 NewFERClient 失败或 nil 时:
metrics.IncModelClientInitFailed("fer")
```

参考已有 `metrics.SkyWalkingInitFailedTotal` 的实现(`shared/pkg/metrics/metrics.go:64`).

### E.3 工作量

1 小时

---

## F. consumer.go 文件职责混杂 (P3, 轻度)

### F.1 问题

`ai-svc/internal/consumer/consumer.go` 共 244 行, 包含:
- `ConsumerGroupHandler`(消费者 handler)
- `MessageHandler` 类型定义
- `KafkaConsumer`(sarama 包装)
- `handleFailure` / `attemptKey` 私有 helper
- 大量"未来演进"注释(Stage 25-F / 30-C A2 / ADR-19 / PR-OBS-17 / Stage 73)

DLQ 已经拆分到 `dlq.go`, 但 `KafkaConsumer` 仍在本文件.

### F.2 修复

把 `KafkaConsumer` + `NewKafkaConsumer` + `Close` 抽到独立文件(如 `kafka_consumer.go`), `consumer.go` 只留 `ConsumerGroupHandler`.

### F.3 工作量

纯文件移动 + import 调整, 30 分钟

---

## G. 风险与缓解(汇总)

| 风险 | 等级 | 缓解 |
|---|---|---|
| 修 A 时漏改一处 consumer | 中 | 三方 ai-svc / analytics-svc 共用一套 helper, 统一 PR |
| 修 B 时测试覆盖不全 | 低 | 现 `consumer_test.go` 已覆盖 attempt 计数路径, 机械加锁不会破坏 |
| 修 C 后业务监控漏报 | 低 | 现状 `path="unmatched"` series 没业务价值; 如担心可保留 1m counter |
| 修 E 引入新 metric 引发 cardinality 焦虑 | 低 | 只在 init failed 时递增, series 数极小 |
| 修 F 是纯重构 | 低 | 无业务行为变化, 只调整文件位置 |

---

## H. 不在本计划范围

- **Kafka sw8 透传的客户端库选型**(go2sky 原生 vs 自实现)—— 应由 A 修复的 PR 自带选型 ADR
- **SkyWalking OAP 升级到 9.7+ 拿真正的 EntrySpan** —— 与 go2sky v1.5 -> v2 升级是另一个独立工程
- **trace 采样率配置**(dev 全量 -> prod 抽样)—— 业务侧独立计划
- **APM -> 日志联动**(SkyWalking UI 跳 Loki)—— 部署侧增强, 不在代码层

---

## I. 工作量估算(汇总)

| 项 | 工作量 | 优先级 |
|---|---|---|
| A. Kafka sw8 透传(chat-svc + ai-svc + analytics-svc) | 半天 | P1 |
| B. consumer.attempts 加锁 | 0.5h | P2 |
| C. metrics unmatched 路径处理 | 0.5h+测试 1h | P2 |
| D. GinSkywalking 跳过路径配置化 | 0.5h | P3 |
| E. AI model init failed metric | 1h | P2 |
| F. consumer.go 文件拆分 | 0.5h | P3 |

**总: 约 1 人天(紧凑做 0.5 人天)**

---

## J. 调研依据

| 文件 | 调研内容 |
|---|---|
| `emotion-echo-chat-svc/internal/events/kafka_publisher.go:48` | ProducerMessage.Headers 实际写入内容(仅 content-type) |
| `emotion-echo-chat-svc/internal/events/proto_marshal.go` | 无 sw8 注入逻辑 |
| `emotion-echo-ai-svc/internal/consumer/consumer.go:51,107,113` | attempts map 读写位置 |
| `emotion-echo-shared/pkg/middleware/gin_skywalking.go:56` | 跳过路径硬编码 |
| `emotion-echo-shared/pkg/metrics/metrics.go:107` | path 兜底逻辑 |
| `emotion-echo-ai-svc/main.go:288` | AI model client 启停日志位置 |
| `emotion-echo-ai-svc/internal/consumer/consumer.go`(244 行) | 文件职责混杂 |
| 全仓 `Select-String -Pattern 'sw8\|HeaderCarrier'` | 零命中 -> 验证 A.3 |
| `deploy/loki/promtail-config.yaml` | 业务 svc 日志未采集(不在本计划范围, 参考用) |
| `shared/pkg/metrics/metrics.go:64`(SkyWalkingInitFailedTotal) | E 项的参考实现 |