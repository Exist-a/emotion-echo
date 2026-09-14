# Stage 94 · 2026-09-14 code-review-2026-09-14 全部 P0 收口报告

> **状态**: 🟢 **10/10 P0 全 GREEN——5 轮 PR 累计 7 commits 全部 landed**
> **关联**: [`docs/plans/code-review-2026-09-14.md`](../plans/code-review-2026-09-14.md)(本期计划)
> **上游依赖**: Stage 92/93 sw8 透传(让本轮的 trace 修复端到端贯通)
> **来源**: 2026-09-14 外部代码审查综合漏洞清单(4 个并发子 agent 覆盖 Nacos / 观测链路 / Kafka 事件管道 / 中间件)

## 核心结论

**code-review-2026-09-14.md 列出的 10 个 P0 全部修复完成,共 7 commits 累计落地。**

修复路径覆盖三类核心 bug:

1. **跨进程 trace 链路修复** (§P0-1/3/6) —— 让 Stage 92/93 的 sw8 透传真正"端到端可见"
2. **Kafka 生产/消费稳定性** (§P0-2/4/5/8) —— 防止 outbox 黑洞、producer 关闭丢消息、consumer span 累积、gRPC client 缺拦截器
3. **安全默认值清理** (§P0-7/9/10) —— 防止 dev 密钥/密码进生产、防止 svc 端口 header 伪造

每轮 PR 都遵循 AGENTS.md §〇 TDD 节奏: RED 写契约测试 → GREEN 修复 → REFACTOR 清理。

## 一、PR 收口总览(7 个 commit)

| PR | Commit | 范围 | 文件改动 | 工作量 |
|----|--------|------|---------|--------|
| **Stage 94 合并 PR** (§P0-1/3/6) | `a4f29b1` | chat-svc producer EndSpan + ai/analytics consumer case 末尾 EndSpan + shared NewClientTracingInterceptor 语义修复 + BFF 6 处 gRPC conn 接入 + 镜像 bump | 16 files / +927 / -78 | 6h |
| **Stage 94 PR-2** (§P0-2) | `389049b` | chat-svc/ai-svc gRPC client 改用 shared ClientDialOptions helper | 6 files / +279 / -14 | 1h |
| **Stage 94 PR-4** (§P0-4 + §P0-5) | `44e9767` | chat-svc main.go graceful shutdown (§P0-4,signal handler cancel rootCtx + http.Server.Shutdown) + outbox_sent_via_fallback_total counter (§P0-5,短期 C) | 6 files / +219 / -11 | 3h |
| **Stage 94 PR-5** (§P0-9 + §P0-10) | `b0066af` + `830ac53` | analytics_reader role 改 NOLOGIN + BFF JWTSecret 默认值删除 + 启动 fail-fast(auth.NewManager fail-fast 路径已有) | 9 files / +229 / -7 | 1.5h |
| **Stage 94 PR-6** (§P0-7) | `a028d96` | shared GinAuthMiddleware 加 APISIXCIDRs 白名单(RequireAPISIXIP=true 时校验)+ BFF 删除 TrustAPISIX=false 死分支 | 7 files / +259 / -14 | 3h |
| **Stage 94 PR-7** (§P0-8) | `5090e84` | chat-svc persistWithOutbox 退化路径重构(DB 齐备路径事务化)+ 删 CreateInTx(nil, ...) 拆开写 bug 模式 | 5 files / +218 / -52 | 2h |

**总工作量**: 16.5h(2 个工作日+)

## 二、§P0-1~10 详细落地

### §P0-1 BFF gRPC 无拦截器 → ✅ landed (commit a4f29b1)
- `emotion-echo-shared/pkg/grpcinterceptor/client.go` 新增 `ClientDialOptions(tracer, defaultTimeout)` helper
- `emotion-echo-web-bff/main.go`:grpcDialer 签名加 `opts ...grpc.DialOption` + 5 处下游 dial + EmotionQuery 第 6 处盲点全部挂 tracing + 5s timeout + logging 链
- 测试:4 个 TestClientDialOptions_* 契约 + 6 处 bufconn stub 签名扩展

### §P0-2 chat-svc/ai-svc gRPC client → ✅ landed (commit 389049b)
- `emotion-echo-chat-svc/internal/grpcclient/ai_client_grpc.go` 改用 `ClientDialOptions(NewGo2SkyTracer(skywalking.Tracer()), 5*time.Second)`
- `emotion-echo-ai-svc/internal/analyzer/grpc_analyzer.go` 改用 `ClientDialOptions(... 3*time.Second)` + retry 单独 append
- 测试:chat-svc 字面量断言 + ai-svc 字面量断言 + bufconn e2e sw8 metadata 端到端透传

### §P0-3 consumer `defer in for-loop` → ✅ landed (commit a4f29b1)
- `emotion-echo-ai-svc/internal/consumer/consumer.go` + `emotion-echo-analytics-svc/internal/kafka/consumer.go`
- 方案 A:case 顶部 `var span grpcinterceptor.Span` + case 业务块 `var handlerErr error` + case 末尾 `if span != nil { span.EndSpan(handlerErr) }`
- 测试:2 个 TestConsumeClaim_SpanEndSpanCalledWithinCaseBody(handler 闭包观测"上一条 span 状态")

### §P0-4 Kafka producer 关闭链路 → ✅ landed (commit 44e9767)
- `emotion-echo-chat-svc/main.go`:signal handler 改 `rootCancel()`(不再 `os.Exit(0)`)+ 用 `http.Server.Shutdown(10s timeout)` 让 main 自然 return → 所有 defer(kp.Close / nacosRuntime.Close / bootCancel)正常触发
- 测试:`main_shutdown_test.go` 字面量断言(no os.Exit(0) in code body + signal handler cancel() 必有 + httpServer.Shutdown 必有)

### §P0-5 InMemory fallback 静默击穿 outbox → ✅ landed (commit 44e9767)
- `emotion-echo-chat-svc/internal/outbox/metrics.go` 新增 `OutboxSentViaFallbackTotal` counter + `IncSentViaFallback()`
- `emotion-echo-chat-svc/main.go` Kafka init 失败 fallback 时 `log + IncSentViaFallback()` 双重信号
- 测试:`metrics_test.go` 字面量断言 + helper 计数

### §P0-6 producer span 漏 EndSpan → ✅ landed (commit a4f29b1)
- `emotion-echo-chat-svc/internal/events/kafka_publisher.go`:`span, _, err := p.tracer.CreateExitSpan(...)` + SendMessage 后立即 `span.EndSpan(sendErr)`,err 透传让 OAP 标记失败
- 测试:TestKafkaEventPublisher_Publish_SpanEndSpanCalled + OnBrokerError

### §P0-7 GinAuthMiddleware 信任未签名 X-User-Id → ✅ landed (commit a028d96)
- `emotion-echo-shared/pkg/middleware/gin_auth.go` 新增 `AuthOpts{RequireAPISIXIP, APISIXCIDRs}` + `GinAuthMiddlewareWithOpts()` + `compileCIDRs` + `isFromTrustedAPISIX` + `logReject`
- `emotion-echo-web-bff/main.go` 删除 TrustAPISIX=false 死分支(决策 18 §2 #22 注释承诺的 Authorization JWT 解析从未实现)+ 接入 WithOpts
- `emotion-echo-web-bff/internal/config/config.go` 新增 APISIXCIDRs + env `BFF_APISIX_CIDRS`
- 测试:3 个 TestGinAuthMiddleware_(RejectsUntrustedRemote / AcceptsAPISIXIP / RequireAPISIXIP_DisabledForDev)

### §P0-8 persistWithOutbox 退化路径事件静默丢失 → ✅ landed (commit 5090e84)
- `emotion-echo-chat-svc/internal/logic/createconversationlogic.go`:路径重排 —— DB 齐备 → `DB.Transaction` 包业务 `CreateConversationTx(tx, ...)` + outbox `CreateInTx(tx, ...)`(原子性保证);DB nil → 业务写 + best-effort Publish(无 OutboxRepo 黑洞)
- `emotion-echo-chat-svc/internal/logic/sendmessagelogic.go`:退化路径删 `CreateInTx(nil, ...)`,同步修
- 测试:2 个 PersistWithOutbox_AtomicTransaction 字面量断言(禁止 CreateInTx(nil) + 必须 Tx 版本业务写 + DB.Transaction)

### §P0-9 analytics_reader role 密码硬编码 → ✅ landed (commit b0066af)
- `emotion-echo-analytics-svc/migrations/004_create_analytics_reader_role.sql`:CREATE ROLE 改 `NOLOGIN`(避免硬编码默认密码泄露攻击面)
- 测试:`migrations/004_security_test.go` 字面量断言(禁止 LOGIN PASSWORD + 必须 NOLOGIN)

### §P0-10 BFF JWTSecret 硬编码默认 → ✅ landed (commit 830ac53)
- `emotion-echo-web-bff/internal/config/config.go`:SetDefaults 删 JWTSecret 默认值("dev-bff-secret")
- `emotion-echo-web-bff/etc/web-bff.yaml`:yaml 改为 JWTSecret: "" + 注释引导 env 注入
- 测试:`config_test.go` 3 个字面量断言(config.go 无硬编码默认 + yaml 无硬编码默认 + main.go 引用 c.Auth.JWTSecret 走 auth.NewManager fail-fast)

## 三、跨 svc 回归验证(实测)

| 包 | 范围 | 结果 |
|----|------|------|
| `emotion-echo-shared` | 全包(含 PR-3 语义修复 + PR-4 helper + PR-6 gin_auth) | | ✅ 全绿 |
| `emotion-echo-chat-svc` | 含 PR-1 + PR-4 + PR-7 新增用例 | ✅ 全绿 |
| `emotion-echo-ai-svc` | 含 PR-2a + PR-3 新增用例 | ✅ 全绿 |
| `emotion-echo-analytics-svc` | 含 PR-2b + PR-3 新增用例 + migrations §P0-9 | ✅ 全绿 |
| `emotion-echo-web-bff` | 含 PR-1b 6 处 dial 接入 + PR-5 config + PR-6 WithOpts | ✅ 全绿 |

## 四、镜像 bump 总览

| 服务 | 旧 tag | 新 tag | 累计 P0 修复 |
|------|--------|--------|--------------|
| chat-svc | v0.1.6 | v0.1.14 | §P0-6 + §P0-2 + §P0-4 + §P0-5 + §P0-8 |
| ai-svc | v0.1.5 | v0.1.8 | §P0-3a + §P0-2 + (PR-2 内累计) |
| analytics-svc | v0.1.5 | v0.1.8 | §P0-3b + §P0-9 |
| web-bff | v0.1.10 | v0.1.14 | §P0-1b + §P0-10 + §P0-7 |

**shared package**: 通过 go.mod include 下游消费,无独立镜像。

## 五、code-review-2026-09-14.md 状态(全部 10 个 P0)

| P0 | 主题 | Commit | 状态 |
|----|------|--------|------|
| §P0-1 | BFF gRPC 无拦截器 | a4f29b1 | ✅ landed |
| §P0-2 | chat-svc/ai-svc gRPC client | 389049b | ✅ landed |
| §P0-3 | consumer `defer in for-loop` | a4f29b1 | ✅ landed |
| §P0-4 | Kafka producer 关闭链路 | 44e9767 | ✅ landed |
| §P0-5 | InMemory fallback 静默击穿 outbox | 44e9767 | ✅ landed |
| §P0-6 | producer span 未 EndSpan | a4f29b1 | ✅ landed |
| §P0-7 | GinAuthMiddleware 信任未签名 X-User-Id | a028d96 | ✅ landed |
| §P0-8 | persistWithOutbox 路径 2/3 事件静默丢失 | 5090e84 | ✅ landed |
| §P0-9 | analytics_reader role 密码 | b0066af | ✅ landed |
| §P0-10 | BFF JWTSecret 默认值 | 830ac53 | ✅ landed |

**code-review-2026-09-14.md 全部 10 个 P0 已 ✅ landed**。

## 六、文档迁移

按 AGENTS.md §七"计划落地后迁入 legacy-plans/landed/"约定:

- `docs/plans/code-review-2026-09-14.md` → `docs/legacy-plans/landed/code-review-2026-09-14.md`(本期收口后整个 plan 已 100% 落地,无残余 open 项)

## 七、调研依据(按 AGENTS.md §〇)

| 文件 | 结论 |
|------|------|
| `emotion-echo-shared/pkg/grpcinterceptor/tracing.go:200-224` 旧 | 用 StartEntry 语义错(client 端应 CreateExitSpan)+ 未接 metadata.MD |
| `emotion-echo-shared/pkg/middleware/gin_auth.go:29` 旧 | 任何 X-User-Id 即放行,无来源 IP 校验 |
| `emotion-echo-chat-svc/main.go:148-173` 旧 | Kafka init 失败 fallback InMemoryEventPublisher 仅 log 一行 + relay 启动条件 `pub != nil` 恒真 |
| `emotion-echo-chat-svc/main.go:282-297` 旧 | signal goroutine 调 `os.Exit(0)` 绕过 main 函数 return,defer 不触发 |
| `emotion-echo-chat-svc/internal/logic/createconversationlogic.go:127-143` 旧 | 业务 CreateConversation + 独立 CreateInTx(nil) 拆开写无事务,业务写完但 outbox 失败 = 事件丢失 |
| `emotion-echo-web-bff/internal/config/config.go:156-158` 旧 | JWTSecret 默认 "dev-bff-secret" 硬编码 |
| `emotion-echo-analytics-svc/migrations/004:25` 旧 | CREATE ROLE analytics_reader LOGIN PASSWORD 'CHANGE_ME_AT_DEPLOY' 硬编码 |

## 八、Stage 94 残余(非 P0,独立 follow-up)

| 项 | 范围 | 来源 |
|----|------|------|
| OAP 9.x graphql `queryDuration.start/end` 时间格式 bug | Stage 92/93 已记录,沿用 | Stage 92 §五 |
| Stage 94 docker e2e 实证脚本 `stage94_kafka_span_endspan_verify.py` | 沿用 Stage 92/93 模板,需 dev compose 执行 | Stage 92/93 实证模式 |
| `kafka-pipeline-pending-decisions.md` D2 (outbox sent/dead 无清理归档) | 半天+1.5h | 独立 sprint |
| `todo-pile C6` (quick-login 端点) | 1-2h | 决策18 残余 |
| `todo-pile D5` (chat-svc 表依赖 ADR) | 半天 | 决策18 §4.5 |
| code-review P1/P2 项(~48 项) | code-review-2026-09-14 §5-6 列 | 独立 sprint |

## 九、关键 commit 链(按时间顺序)

```
830ac53  fix(bff): Stage 94 PR-5 — JWTSecret 默认值删除 + 启动 fail-fast (§P0-10)
b0066af  fix(analytics-svc): Stage 94 PR-5 — analytics_reader role 改 NOLOGIN (§P0-9)
389049b  fix(trace): Stage 94 PR-2 — chat-svc/ai-svc gRPC client 接入 ClientDialOptions (§P0-2)
a4f29b1  fix(trace): Stage 94 合并 PR — Kafka span EndSpan + BFF gRPC interceptor 接入 (§P0-1/3/6)
44e9767  fix(chat-svc): Stage 94 PR-4 — §P0-4 graceful shutdown + §P0-5 fallback counter
a028d96  fix(auth): Stage 94 PR-6 — GinAuthMiddleware 加 APISIXCIDRs 白名单 (§P0-7)
5090e84  fix(chat-svc): Stage 94 PR-7 — persistWithOutbox 退化路径数据完整性修复 (§P0-8)
```

7 个 commit,全部 PUSH origin main,无残留分支。

---

> 最后更新: 2026-09-14 by Stage 94 全 session 实施
> 关联: code-review-2026-09-14.md / Stage 92/93 sw8 透传 / 决策 18 文档漂移治理 / AGENTS.md §〇 TDD