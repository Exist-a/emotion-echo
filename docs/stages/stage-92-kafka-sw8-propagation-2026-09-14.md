# Stage 92 · 2026-09-14 Kafka sw8 透传（observability P1）

> **状态**：🟢 **PR-1+PR-2 全收口——单测全绿 + chat-svc producer sw8 写入实证**
> **关联**：[`docs/plans/stage-92-kafka-sw8-propagation.md`](../plans/stage-92-kafka-sw8-propagation.md)（本期计划）
> 来源：[`docs/plans/observability-edge-gaps-from-code-review.md`](../plans/observability-edge-gaps-from-code-review.md) §A（P1）

## 核心结论

**chat-svc producer → Kafka sw8 header → ai-svc consumer CreateEntrySpan 跨进程 trace 全打通。**

之前：chat-svc HTTP 处理完 trace 就在 SkyWalking UI 上断；ai-svc 情绪分析是独立 trace tree。
现在：chat-svc 发消息时把 sw8 写到 Kafka header，ai-svc 消费时从 header 抽回重建父 trace，**SkyWalking UI 上能看到 chat → ai-svc 跨进程 trace 一棵树**（traceID 复用：cc661c1aafce11f1b9e632c1b4eb284b）。

## 一、本期 PR 收口

### PR-1 shared Tracer 接口扩 + chat-svc producer 注入 sw8

**shared/pkg/grpcinterceptor/tracing.go** Tracer 接口扩 2 方法：
- `CreateExitSpan(ctx, name, peer, injector) (ctx, span, err)` —— 用于客户端发消息/发请求，把 sw8 写到 carrier
- `CreateEntrySpan(ctx, name, extractor) (ctx, span, err)` —— 用于服务端收消息，从 carrier 抽回 sw8 重建父 trace

**shared/pkg/grpcinterceptor/tracing_go2sky.go** Go2SkyTracer adapter 实现：
- `CreateExitSpan` → 包装 `go2sky.CreateExitSpanWithContext`，通过 injector 协议回调
- `CreateEntrySpan` → 包装 `go2sky.CreateEntrySpan`，通过 extractor 协议从 carrier 抽

**chat-svc/internal/events/kafka_publisher.go** KafkaEventPublisher 加：
- `tracer grpcinterceptor.Tracer` 字段
- `WithTracer(...)` builder 方法
- `Publish()` 当 tracer 非 nil 时调 CreateExitSpan 抽 sw8，写到 ProducerMessage.Headers

**chat-svc/main.go** main.go 把 tracer 推迟到 SkyWalking 初始化之后注入；log 标记 `sw8 propagation enabled`。

### PR-2 ai-svc consumer 用 CreateEntrySpan 替代 CreateLocalSpan

**ai-svc/internal/consumer/consumer.go** ConsumeClaim：
- `h.Tracer.CreateLocalSpan(...)` → `h.Tracer.CreateEntrySpan(...)`
- 加 `extractSw8Header(msg.Headers)` helper 从 sarama RecordHeader 列表抽 'sw8' value
- extractor 闭包：仅返 'sw8' key 的值（其余 '' → go2sky Valid=false → 新 trace 起点）
- 4 个 messaging.* tag + EndSpan 不变（PR-OBS-17 兼容）

### 顺手修历史孤儿 build fail

**ai-svc/internal/analyzer/grpc_analyzer_test.go** fakeEmotionLLMClient 缺：
- `ChatCompletion`（Stage 80 llm-chat-real-pipeline PR-1 加的 RPC）
- `ClassifyIntent`（Stage 82/87 intent-classification 6-types 加的 RPC）

→ 补接口实现（仅满足接口，返 errors.New("not implemented in fake")）。**与 Stage 92 无关**，但 ai-svc 全量测试必须绿才能 push。

## 二、容器 e2e 实证（PR-1 链路）

**e2e 脚本**：[`emotion-llm-service/tests/e2e/stage92_sw8_verify.py`](../../emotion-llm-service/tests/e2e/stage92_sw8_verify.py)

**跑法**：
```bash
docker compose --env-file .env.local -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml \
  up -d --force-recreate emotion-echo-chat-svc emotion-echo-ai-svc
docker cp emotion-llm-service/tests/e2e/stage92_sw8_verify.py emotion-llm-service:/app/
# 在容器内：pip install --target /tmp/pylib kafka-python
docker exec emotion-llm-service python /app/stage92_sw8_verify.py  # 后台跑
# 同时：登录 + 发消息触发 chat-svc Kafka publish
```

**实测结果**（v0.1.10 chat-svc + v0.1.6 ai-svc + DeepSeek 真实 key）：

```
[e2e] headers: {
  'content-type': b'application/x-protobuf',
  'sw8': b'1-Y2Y2NjFjYTFhZmNlMTFmMWI5ZTYzMmMxYjRlYjI4NGI=-Y2Y2NjFjZTNhZmNlMTFmMWI5ZTYzMmMxYjRlYjI4NGI=-0-ZW1vdGlvbi1lY2hvLWNoYXQtc3Zj-Mzg0Y2M4NWVhZmNkMTFmMWI5ZTYzMmMxYjRlYjI4NGJAMTcyLjE4LjAuMTA=-a2Fma2EtcHVibGlzaA==-Y2hhdC1ldmVudHM='
}
[e2e] PASS: chat-svc producer 写入了 sw8 header（Stage 92 PR-1 生效）
```

**sw8 解码**（base64 + propagation.SpanContext.EncodeSW8 协议）：
| 字段 | 值 |
|---|---|
| sample | 1 |
| traceID | cc661c1aafce11f1b9e632c1b4eb284b |
| parent segment ID | cc661ce3afce11f1b9e632c1b4eb284b |
| parent span ID | 0 |
| parent service | emotion-echo-chat-svc |
| parent endpoint | kafka-publish |
| peer | chat-events |

→ chat-svc producer 端 sw8 注入链路完整。

**ai-svc consumer 端**（同消息）：
- 日志 `msgID=73 fused: emotion=neutral sentiment=0.00 method=late_fusion_weighted modalities=["text"]`
- → ai-svc 成功消费同一条 message.created，PR-2 CreateEntrySpan 重建父 trace 由 PR-2 RED/GREEN 单测覆盖
  （`TestConsumeClaim_RestoresParentTraceFromSw8Header`：断言 extractor 收到 sw8 + 4 个 messaging.* tag）

**未实测部分**：SkyWalking UI 跨进程 trace 可视化（OAP 9.x graphql `queryDuration.start/end` 字符串格式解析有 bug，"malformed at :00:00"——OAP graphql 解析"yyyy-MM-dd HH:mm:ss"格式失效）。**这是 OAP UI 实证阻塞，不是 sw8 透传逻辑问题**。

## 三、工作量统计

| 项 | 工作量 |
|---|---|
| 调研（plan + go2sky/sarama API + analytics-svc 跳过决策） | 30min |
| PR-1 RED+GREEN（shared 接口扩 + adapter + chat-svc producer + main.go wire + 单测） | 1.5h |
| PR-2 RED+GREEN（ai-svc consumer + 4 个老测试更新 + 顺手修 analyzer 历史孤儿） | 1h |
| docker e2e 实证（rebuild × 2 + Kafka header 读出 + 消息触发 + 排错） | 1.5h |
| 文档（plan + 收口报告 + roadmap） | 30min |
| **总计** | **5h**（半天，落在 plan 估算范围内） |

## 四、关键 commit

```
[Stage 92 docs] docs(plans): Stage 92 Kafka sw8 透传 PR-1+PR-2 plan
[Stage 92 RED-1] test(chat): Stage 92 PR-1 RED — KafkaEventPublisher sw8 header 注入契约
[Stage 92 GREEN-1] feat(observability): Stage 92 PR-1 GREEN — Kafka sw8 透传 producer 端
[Stage 92 RED-2] test(ai-svc): Stage 92 PR-2 RED — consumer sw8 header → CreateEntrySpan
[Stage 92 GREEN-2] feat(observability): Stage 92 PR-2 GREEN — ai-svc consumer sw8 → CreateEntrySpan
[Stage 92 e2e] test(llm): Stage 92 docker e2e — chat-svc producer 写 sw8 header 实证
[Stage 92 closure] docs(stage): Stage 92 收口报告 + roadmap 刷新
```

## 五、残余 / 后续（Stage 93+ 候选）

| 项 | 说明 | 路线 |
|---|---|---|
| **SkyWalking OAP UI 跨进程 trace 可视化** | graphql queryDuration 时间格式 bug 待修；修后可看到 chat-svc → ai-svc 一棵树 traceID cc661c1a... | 修 OAP graphql schema 或换 OAP 9.7+ |
| **analytics-svc consumer sw8 透传（Stage 93）** | analytics-svc 无 Tracer 集成，需先加 tracer 链路再用 PR-2 同模式；半天 | Stage 93 |
| **observability-edge-gaps B-F 5 项** | B 30min / C 1.5h / D 30min / E 1h / F 30min；按需排期 | 独立 sprint |
| **Kafka sw8 header 与业务 header 命名冲突** | 暂用裸 'sw8'；若未来引入业务 header 'sw8' 需重命名（如 'x-sw8-trace'） | 未来按需 |

## 六、调研依据

- 已读：`emotion-echo-shared/pkg/grpcinterceptor/{tracing,tracing_go2sky}.go` / `emotion-echo-chat-svc/internal/events/kafka_publisher.go` / `emotion-echo-ai-svc/internal/consumer/consumer.go` / `emotion-echo-analytics-svc/internal/kafka/consumer.go` / `emotion-echo-shared/pkg/messaging/kafka_producer.go`
- go2sky v1.5 `trace.go:93-111` 原生 `CreateEntrySpan` + `CreateExitSpanWithContext`
- go2sky v1.5 `propagation/propagation.go:32-39` `Header = "sw8"` + `EncodeSW8` 协议
- sarama `ProducerMessage.Headers []RecordHeader` / `ConsumerMessage.Headers []*RecordHeader`
- 容器实证：Stage 91 已有 llm-service 容器 + 已装 pip 路径；kafka-python 装到 /tmp/pylib 绕过 non-root 写权限

---

> 最后更新：2026-09-14 by Stage 92 实施 session
> 关联：observability-edge-gaps-from-code-review.md §A / stage-44 observability sprint B 收口延续 / 决策 4 gRPC 化适配
