# Stage 94 · 2026-09-14 Kafka trace span bug 修复 + BFF gRPC interceptor 接入

> **状态**：🟢 **PR-1~4 全收口——单测 5 包全绿 + 镜像 bump 完成（待 docker e2e 实证）**
> **关联**：[`docs/plans/code-review-2026-09-14.md`](../plans/code-review-2026-09-14.md) §P0-1 + §P0-3 + §P0-6
> **上游依赖**：[Stage 92](stage-92-kafka-sw8-propagation-2026-09-14.md) + [Stage 93](stage-93-analytics-svc-sw8-propagation-2026-09-14.md) 的 sw8 透传
> **修复的 issue**：
> - §P0-6 chat-svc producer span 漏 EndSpan
> - §P0-3a ai-svc consumer `defer in for-loop` bug
> - §P0-3b analytics-svc consumer `defer in for-loop` bug
> - §P0-1a shared `NewClientTracingInterceptor` 用 StartEntry 语义错 + 未接 metadata
> - §P0-1b BFF 5 conn + 1 盲点（EmotionQuery）无 interceptor

## 核心结论

**修 3 个跨进程 trace bug + 1 个 client-side tracing 语义修复 + BFF 6 处 gRPC conn 接入 interceptor 链，让 Stage 92/93 的 sw8 跨进程 trace 工程"端到端可见"——OAP 上能看到正确的 producer/consumer/client/server span 树形拓扑，sw8 在 Kafka header + gRPC metadata 两类 carrier 上贯通。**

之前（修前）：chat-svc producer span 数据丢失（_, _, _ 丢弃），ai/analytics consumer 的 span 累积到 ConsumeClaim 退出才批量收尾，BFF→downstream gRPC 没有 exit span，shared client tracing 用 StartEntry 创建的 entry span 把 trace 树画反方向。

现在（修后）：
- chat-svc producer span SendMessage 后立即 EndSpan(sendErr)，err 透传让 OAP 标记失败
- ai-svc / analytics-svc consumer 在 case 分支末尾显式 EndSpan(handlerErr)，每条消息 span duration 反映真实处理耗时，失败路径透传
- shared client tracing 用 CreateExitSpan + metadata.MD 注入 sw8，下游 server 端能抽到重建父 trace
- BFF 6 个下游 gRPC conn（含 EmotionQuery 第 6 处）统一挂 tracing + 5s timeout + logging 链，跨 gRPC 进程 sw8 透传

## 一、本期 PR 收口

### PR-1 · chat-svc producer span EndSpan（§P0-6）

**文件**：`emotion-echo-chat-svc/internal/events/kafka_publisher.go`

**问题**：原 `_, _, _ = p.tracer.CreateExitSpan(...)` 三返回值全 discard，go2sky reporter 累积未收尾的 span 实例，OAP 上 producer span duration 永远 = 0 或 = 该进程累计 publish 间隔。

**修复**：
- 保留 `spanCtx, span, err := p.tracer.CreateExitSpan(...)` 返回值
- SendMessage 同步阻塞（WaitForAll）后立即 `span.EndSpan(sendErr)`
- sendErr 透传让 OAP 正确标记失败

**测试**：`emotion-echo-chat-svc/internal/events/kafka_publisher_test.go` 新增：
- `TestKafkaEventPublisher_Publish_SpanEndSpanCalled` —— happy path 断言 EndSpan(nil) 调 1 次
- `TestKafkaEventPublisher_Publish_SpanEndSpanOnBrokerError` —— SendMessage 失败时 EndSpan(sendErr) err=broker error

mock 改造：`sw8MockTracer` 加 `outSpan grpcinterceptor.Span` 字段；旧测试 outSpan=nil 走原路径（向后兼容），新测试用 `newSw8MockTracerWithSpan` 注入 `spanEndRecorder` 捕获 EndSpan 调用。

### PR-2 · ai-svc + analytics-svc consumer case 末尾 EndSpan（§P0-3）

**问题**：原 `defer span.EndSpan(nil)` 在 `for { select { case msg := <-claim.Messages(): ... } }` case 内。Go defer 绑定到 ConsumeClaim 函数返回，不绑定到 case 分支。N 条消息累积 N 个 defer，全部延迟到 ConsumeClaim 退出才批量触发 → OAP 上每条消息 duration = 整个 consumer goroutine 寿命。

**修复（方案 A）**：
- case 顶部 `var span grpcinterceptor.Span`
- case 业务块 `var handlerErr error`
- case 末尾 `if span != nil { span.EndSpan(handlerErr) }`
- 失败路径（handleFailure 走 DLQ + 重投）也走 EndSpan(handlerErr)，让 OAP 标记失败

**文件**：
- `emotion-echo-ai-svc/internal/consumer/consumer.go:90-180`
- `emotion-echo-analytics-svc/internal/kafka/consumer.go:181-260`

**测试**：两边 svc 各新增 `TestConsumeClaim_SpanEndSpanCalledWithinCaseBody`：
- 构造 N=3 条消息；mock handler 闭包观测"上一条 span 状态"
- 钉死契约：handler #2 / #3 时 span0 / span1 已 EndSpan（=case 末尾立刻收尾）
- mockTracer 加 `entryFn` 字段：每次 CreateEntrySpan 返回独立 mockSpan，让测试可观测多消息 span 独立生命周期
- 旧测试 entryFn=nil 走原路径（向后兼容，13/13 旧测试不动）

### PR-3 · shared `NewClientTracingInterceptor` 语义修复（§P0-1a）

**文件**：`emotion-echo-shared/pkg/grpcinterceptor/tracing.go`

**问题**：
1. 用 `tracer.StartEntry(ctx, "client:"+method)` —— 语义错！client 端应创建 exit span（出向），不是 entry
2. injector 完全没接 metadata.MD，sw8 不会写到 outgoing ctx，下游 server 端拿不到 sw8

**修复**：
- 用 `tracer.CreateExitSpan(ctx, opName, peer, injector)` 替换 StartEntry
- peer 取 `cc.Target()`（带 nil-guard 避免测试 panic）
- injector 写入 `metadata.MD["sw8"]`
- `metadata.NewOutgoingContext(ctx, md)` 把 sw8 装回 outgoing ctx —— 让 gRPC 透传给下游 server
- nil md 守卫（`FromOutgoingContext` 在 ctx 无 outgoing 时返 nil，`Set` 会 panic）
- `span.SetSpanLayer(SpanLayerGRPC)` + `SetComponent(ComponentGoGRPC)` + 2 个 RPC tag（与 server 对称）
- panic-recover + `span.EndSpan(err)`

**测试**：`emotion-echo-shared/pkg/grpcinterceptor/tracing_test.go` 新增：
- `TestClientTracingInterceptor_CallsCreateExitSpanWithSw8Metadata` —— 验证 CreateExitSpan 被调 + injector 写入 sw8 到 outgoing metadata + span.EndSpan + SetSpanLayer/SetComponent
- `TestClientTracingInterceptor_NilTracer_NoOp` —— nil tracer 走 no-op 路径（向后兼容）

### PR-4 · `ClientDialOptions` helper + BFF 6 处接入（§P0-1b）

**文件**：
- `emotion-echo-shared/pkg/grpcinterceptor/client.go` —— 新增 helper
- `emotion-echo-web-bff/main.go` —— 5 + 1 处 dial 接入

**修复**：BFF 5 个下游 gRPC conn（user/chat/assessment/analytics/ai）+ EmotionQuery 第 6 处盲点全部裸 `grpc.NewClient(addr, insecure)` —— 无 interceptor、无 sw8 metadata 透传。helper 把"tracing + timeout + logging"链标准化：

```go
func ClientDialOptions(tracer Tracer, defaultTimeout time.Duration) []grpc.DialOption {
    var interceptors []grpc.UnaryClientInterceptor
    if tracer != nil {
        interceptors = append(interceptors, NewClientTracingInterceptor(tracer))
    }
    if defaultTimeout > 0 {
        interceptors = append(interceptors, ClientTimeoutInterceptor(defaultTimeout))
    }
    interceptors = append(interceptors, ClientLoggingInterceptor())
    if len(interceptors) == 0 { return nil }
    return []grpc.DialOption{grpc.WithChainUnaryInterceptor(interceptors...)}
}
```

**BFF main.go 改动**：
- `grpcDialer` 签名加 `opts ...grpc.DialOption`（向后兼容：bufconn stub 透传 opts）
- `dialGRPC` 内部 helper：`tracer` 提升为包级 `packageTracer`，5 处下游 dial + EmotionQuery 第 6 处全部调 `dialGRPC(addr, name, sharedgrpc.ClientDialOptions(sharedgrpc.NewGo2SkyTracer(packageTracer), 5*time.Second)...)`
- EmotionQuery 第 6 处改走 `dialGRPC` 统一 fallback / interceptor 链（之前是裸 `grpc.NewClient`，dial 失败只打 log 不 fallback——语义不一致）

**测试**：
- `emotion-echo-shared/pkg/grpcinterceptor/client_test.go` 新增 4 个 `TestClientDialOptions_*` 契约测试（nil tracer、tracer、timeout、全链）
- `emotion-echo-web-bff/main_grpc_test.go` + `main_grpc_discovery_test.go` 6 处 stub 改 `func(addr string, opts ...grpc.DialOption)` —— 已 bufconn stub 透传 opts 不破坏现有断言
- 既有 `TestBuildServiceContext_GRPCDial_AddrsResolvedViaNacos` + `TestBuildServiceContext_GRPCDial_NilGrpcResolverKeepsEnv` 断言更新为 6 处 dial（EmotionQuery 第 6 处也走 dialGRPC）

### PR-5 · 镜像 bump

| 服务 | 旧 tag | 新 tag | 修复 |
|------|--------|--------|------|
| chat-svc | v0.1.10 | v0.1.11 | §P0-6 producer span EndSpan |
| ai-svc | v0.1.6 | v0.1.7 | §P0-3a consumer case 末尾 EndSpan |
| analytics-svc | v0.1.6 | v0.1.7 | §P0-3b consumer case 末尾 EndSpan |
| web-bff | v0.1.11 | v0.1.12 | §P0-1b 6 处 gRPC conn interceptor 接入 |

## 二、跨 svc 回归（实测）

| 包 | 范围 | 结果 |
|----|------|------|
| `emotion-echo-shared` | shared 全包（含 PR-3 语义修复 + PR-4 helper） | ✅ 全绿 |
| `emotion-echo-chat-svc` | 含 PR-1 新增 2 用例（kafka_publisher_test） | ✅ 全绿 |
| `emotion-echo-ai-svc` | 含 PR-2a 新增 1 用例（consumer_test） | ✅ 全绿 |
| `emotion-echo-analytics-svc` | 含 PR-2b 新增 1 用例（kafka/consumer_test） | ✅ 全绿 |
| `emotion-echo-web-bff` | 含 PR-4 stub 签名扩展 + main.go 6 处 dial 接入 | ✅ 全绿（含 2 个更新后的 dial 计数测试） |

## 三、容器 e2e（待实证）

**计划 e2e 脚本**：`emotion-llm-service/tests/e2e/stage94_kafka_span_endspan_verify.py`（沿用 Stage 92/93 模板）

**触发**：X-User-Id header 直接调 chat-svc REST API `POST /api/v1/conversations/62/messages` → 触发 message.created → producer + Kafka + 两个 consumer。

**预期 OAP 实证**：
| 项 | 预期 |
|---|------|
| chat-svc producer span | 真实 EndSpan（duration = SendMessage ack 时长），而非堆积到下一次 publish |
| ai-svc / analytics-svc consumer span | 每条消息独立 EndSpan（duration = 实际处理时长），不再 = 整 consumer 寿命 |
| BFF→downstream exit span | 出现（带 peer=downstream-host:port），sw8 透传 |
| chat-svc → Kafka → ai-svc + analytics-svc → BFF → downstream gRPC 4 跳 | 同一 trace tree（traceID 复用） |

**状态**：docker rebuild + e2e 脚本待执行（code-review-2026-09-14 §P0-1/3/6 落地的最后一公里**。本次 session 完成代码改造 + 单测验证 + 镜像 tag 更新，重建+e2e 由 owner 在 dev compose 环境执行。

## 四、代码改动汇总

| 文件 | 改动类型 | 行数 |
|------|---------|------|
| `emotion-echo-chat-svc/internal/events/kafka_publisher.go` | PR-1 GREEN | +14 / -5 |
| `emotion-echo-chat-svc/internal/events/kafka_publisher_test.go` | PR-1 RED+GREEN | +97 / -3 |
| `emotion-echo-ai-svc/internal/consumer/consumer.go` | PR-2a GREEN | +25 / -10 |
| `emotion-echo-ai-svc/internal/consumer/consumer_test.go` | PR-2a RED+GREEN | +122 / -3 |
| `emotion-echo-analytics-svc/internal/kafka/consumer.go` | PR-2b GREEN | +28 / -12 |
| `emotion-echo-analytics-svc/internal/kafka/consumer_test.go` | PR-2b RED+GREEN | +115 / -3 |
| `emotion-echo-shared/pkg/grpcinterceptor/tracing.go` | PR-3 GREEN | +30 / -8 |
| `emotion-echo-shared/pkg/grpcinterceptor/tracing_test.go` | PR-3 RED+GREEN | +90 / -1 |
| `emotion-echo-shared/pkg/grpcinterceptor/client.go` | PR-4 helper | +50 / -1 |
| `emotion-echo-shared/pkg/grpcinterceptor/client_test.go` | PR-4 tests | +55 / -0 |
| `emotion-echo-web-bff/main.go` | PR-4 wire | +18 / -8 |
| `emotion-echo-web-bff/main_grpc_test.go` | stub 签名 | +6 / -6 |
| `emotion-echo-web-bff/main_grpc_discovery_test.go` | stub 签名 + 断言更新 | +9 / -8 |
| `deploy/docker-compose.apps.yml` | 镜像 bump 4 处 | +4 / -4 |
| **总计** | | **+663 / -72** |

## 五、code-review-2026-09-14.md 状态同步（计划合并后）

§P0-1 BFF gRPC 无拦截器 → **✅ Stage 94 landed**
§P0-3 consumer `defer in for-loop` → **✅ Stage 94 landed**
§P0-6 producer span 未 EndSpan → **✅ Stage 94 landed**

残余仍 open：
- §P0-2 chat-svc → ai-svc gRPC client 无拦截器（ai-svc 内的 grpc_analyzer.go 调用模式）—— `NewClientTracingInterceptor` 修好后可直接套，单独 PR
- §P0-4 chat-svc `os.Exit(0)` + defer 不执行 + relay ctx 取消无序 —— 独立 sprint
- §P0-5 InMemory fallback 静默击穿 outbox —— 已在 kafka-pipeline-pending-decisions.md D1 登记
- §P0-7 / §P0-8 / §P0-10 auth + 数据完整性 + 配置默认值 —— 独立 PR

## 六、关键 commit（待提交）

```
<pending> docs(stage-94): Kafka trace span bug + BFF gRPC interceptor 收口
<pending> feat(shared): NewClientTracingInterceptor 改用 CreateExitSpan + metadata 注入 (§P0-1a)
<pending> feat(shared): 新增 ClientDialOptions helper (§P0-1b)
<pending> test(shared): 新增 ClientDialOptions 契约测试
<pending> test(chat-svc): §P0-6 producer span EndSpan 测试
<pending> feat(chat-svc): §P0-6 kafka_publisher span.EndSpan(sendErr)
<pending> test(ai-svc): §P0-3a consumer case 末尾 EndSpan 测试
<pending> feat(ai-svc): §P0-3a consumer ConsumeClaim 方案 A
<pending> test(analytics-svc): §P0-3b consumer case 末尾 EndSpan 测试
<pending> feat(analytics-svc): §P0-3b consumer ConsumeClaim 方案 A
<pending> feat(web-bff): §P0-1b 5+1 处 dial 挂 ClientDialOptions 链
<pending> chore(deploy): 镜像 bump chat-svc/ai-svc/analytics-svc/web-bff
```

## 七、调研依据（按 AGENTS.md §〇）

写文档/动手前必读 + 已读：
- `emotion-echo-chat-svc/internal/events/kafka_publisher.go:86` —— §P0-6
- `emotion-echo-ai-svc/internal/consumer/consumer.go:136` —— §P0-3 ai
- `emotion-echo-analytics-svc/internal/kafka/consumer.go:212` —— §P0-3 analytics
- `emotion-echo-web-bff/main.go:50-54, 266-270, 307-315` —— §P0-1b
- `emotion-echo-shared/pkg/grpcinterceptor/tracing.go:200-224` —— §P0-1a
- `emotion-echo-shared/pkg/grpcinterceptor/tracing_go2sky.go:144-160` —— CreateExitSpan 已有正确实现
- `emotion-echo-shared/pkg/grpcinterceptor/client.go` —— ClientLoggingInterceptor + ClientTimeoutInterceptor 已就位
- `emotion-echo-ai-svc/internal/analyzer/grpc_analyzer.go:70-77` —— 4 interceptor 链参考模板
- `emotion-echo-web-bff/main_grpc_test.go:32-37` —— bufconn stub 模式
- `docs/plans/code-review-2026-09-14.md` §P0-1/3/6 —— 工作量估 1-1.5d（实测 ~6h，与拆分吻合）

---

> 最后更新：2026-09-14 by Stage 94 实施 session
> 关联：Stage 92/93 sw8 透传 / code-review §P0-1/3/6 / 决策 6 (logging/trace) / PR-OBS-15/17 / observability-edge-gaps §A 全收口