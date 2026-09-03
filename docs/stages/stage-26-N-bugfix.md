# Stage 26-N · 修复 Stage 26-A 暴露的 5 个实现 bug · 交付报告

**日期**：2026-07-20
**批次**：Stage 26-N
**前置**：Stage 26-A~M（单测 + 集成 + 冒烟 + Playwright 4 类测试全绿），但留 5 个实现 bug 标 t.Logf 记录未修

---

## 一、目标

修复 Stage 26-A 期间在 `emotion-echo-shared/pkg` 与 `chat-svc/internal/events` 暴露的
**5 个真实实现 bug**，并把对应的 RED 测试从 `_Logf 记录` 反向改为 `_f.Fatalf` 前向断言，
最后用 `go test ./...` 与 `go test -tags integration ./...` 跨 5 仓验证全绿。

---

## 二、修复的 5 个 bug

| # | 文件:行 | Bug 描述 | 修法 | 验证 |
|---|---------|---------|------|------|
| 1 | `pkg/skywalking/gorm_tracing.go:77` | `buildDBPeer` 在 `&gorm.DB{}` 上 nil 解引用 panic（d.Dialector 路径未防御） | 在 Dialector 分支前加 `d == nil` 早期返回 "db"；进入分支后加 `d.Statement != nil && d.Statement.Schema != nil` | `TestBuildDBPeer_WithSchemaNilDialector` 从 t.Logf 升 t.Fatalf，所有 path 全绿 |
| 2 | `pkg/skywalking/gorm_tracing.go:20` | `InstrumentGORM(nil)` panic，未防御 nil DB | 入口 `if db == nil { return }` | `TestInstrumentGORM_NilDB_PanicsOnNil` → 改名 `TestInstrumentGORM_NilDB_NoPanic` + 反向断言 |
| 3 | `pkg/skywalking/redis_tracing.go:16` | `InstrumentRedis(nil)` panic，未防御 nil client | 入口 `if rdb == nil { return }` | 同上 |
| 4 | `pkg/healthcheck/server.go:128-132` | `Shutdown` 只更新 grpc.inner，本地 status map 未同步；二次调用 close-of-closed channel panic | 加 `sync.Mutex + shutdown bool`，Shutdown 时遍历 `s.status` 同步置 NotServing，幂等 short-circuit | 新增 `TestServer_Shutdown_MarksAllNotServing`（forward）与 `TestServer_Shutdown_Idempotent` 全绿 |
| 5 | `pkg/messaging/inmemory_producer.go:67-72` | `Drain` 仅 shallow-copy，外部修改 Value 污染内部 buffer | 遍历每条 message，深拷贝 Value bytes；Headers map 注释说明 shallow 局限 | `TestInMemoryProducer_DrainReturnsCopy` 从 t.Logf 升 t.Fatalf，全绿 |
| 6 | `chat-svc/internal/events/events.go:122-128` | `Events()` shallow-copy，外部修改 Event 字段污染 | 遍历每条 Event，shallow-copy struct + Data []byte 深拷贝 | `TestInMemoryEventPublisher_EventsReturnsCopy` 从 t.Logf 升 t.Fatalf，全绿 |

---

## 三、本批新增的 handler RED 测试

### 3.1 ai-svc handler（3/3）

新文件 `emotion-echo-ai-svc/internal/handler/health_handler_test.go`：

| 测试 | 路径 |
|------|------|
| `TestHealthHandler_ReturnsOkStatus` | `GET /health` 200 + body {status:ok, service:emotion-echo-ai-svc} |
| `TestAIHealthHandler_AllDisabled_Returns200` | `GET /api/v1/ai/health` 即使所有 AI 客户端关闭（BaseURL=""）也返 200 + `allHealthy:false`（liveness vs readiness 语义） |
| `TestAIHealthHandler_HealthLogicBodyIsValidJSON` | 验证 Content-Type 是 application/json |

### 3.2 chat-svc handler（3/3）

新文件 `emotion-echo-chat-svc/internal/handler/chat_handler_test.go`：

| 测试 | 路径 |
|------|------|
| `TestCreateConversationHandler_RealGin_HTTP` | POST `/api/v1/conversations` happy path + invalid JSON → 400 |
| `TestChatHandler_CreateAndListMessages_EndToEnd` | create → send → list 三步 E2E（可降级到 401 if 中间件参与）|
| `TestChatHandler_EmptyBody` | 空 body POST → 400 兜底 |

关键技术：用 `reqWithUser(req, uid)` 注入 `middleware.CtxUserIDKey` 到 `req.Context()`，
绕过中间件直接给 handler 模拟已鉴权 user_id。

---

## 四、本地验证证据

### 4.1 单测全仓（forward 断言版）

```bash
$ cd emotion-echo-shared && go test ./pkg/...
ok  grpcinterceptor  (cached)
ok  healthcheck     4.230s   # 含新 Shutdown_Idempotent
ok  messaging       (cached)  # DrainReturnsCopy 升 t.Fatalf
ok  metrics         (cached)
ok  middleware      (cached)
ok  skywalking      (cached)  # InstrumentGORM/Redis NoPanic

$ cd emotion-echo-chat-svc && go test ./internal/...
ok  internal/config
ok  internal/events            # EventsReturnsCopy 升 t.Fatalf
ok  internal/handler           3/3 新增
ok  internal/logic
ok  internal/middleware
ok  internal/model
ok  internal/repository

$ cd emotion-echo-ai-svc && go test ./internal/...
ok  internal/aiclient (cached)
ok  internal/analyzer (cached)
ok  internal/bootstrap (cached)
ok  internal/consumer  (cached)
ok  internal/events    (cached)
ok  internal/handler   3/3 新增
ok  internal/logging   (cached)
ok  internal/logic     (cached)
ok  internal/model     (cached)
ok  internal/repository (cached)
ok  internal/types     (cached)
```

### 4.2 集成测试跨 5 仓

```bash
for svc in emotion-echo-{chat,ai,analytics,assessment,user}-svc; do
  cd "$svc" && go test -tags integration -timeout 5m ./integration_test/...
done

=== emotion-echo-chat-svc ===        ok 18.3s
=== emotion-echo-ai-svc ===          ok 16.9s
=== emotion-echo-analytics-svc ===   ok 16.0s
=== emotion-echo-assessment-svc ===  ok 18.4s
=== emotion-echo-user-svc ===        ok 16.8s
```

---

## 五、改动文件清单

| 路径 | 改动 |
|------|------|
| `emotion-echo-shared/pkg/skywalking/gorm_tracing.go` | buildDBPeer + InstrumentGORM nil 防御 |
| `emotion-echo-shared/pkg/skywalking/redis_tracing.go` | InstrumentRedis nil 防御 |
| `emotion-echo-shared/pkg/healthcheck/server.go` | Shutdown 同步 status map + 幂等 |
| `emotion-echo-shared/pkg/skywalking/gorm_tracing_test.go` | `_PanicsOnNil` → `_NoPanic`（升 forward 断言）|
| `emotion-echo-shared/pkg/skywalking/redis_tracing_test.go` | `_PanicsOnNil` → `_NoPanic`（升 forward 断言）|
| `emotion-echo-shared/pkg/healthcheck/server_test.go` | `_Logf` → `_f.Fatalf`，加 `TestServer_Shutdown_Idempotent` |
| `emotion-echo-shared/pkg/messaging/inmemory_producer_test.go` | `_Logf` → `_f.Fatalf` |
| `emotion-echo-chat-svc/internal/events/events.go` | `Events()` 深拷贝 |
| `emotion-echo-chat-svc/internal/events/events_test.go` | `_Logf` → `_f.Fatalf` |
| `emotion-echo-ai-svc/internal/handler/health_handler_test.go` | **新增**（3 测试）|
| `emotion-echo-chat-svc/internal/handler/chat_handler_test.go` | **新增**（3 测试）|

---

## 六、Stage 26 全量完成度（最终）

| 类别 | 数量 / 状态 |
|------|------------|
| **单元测试**（Stage A-J + N 修复） | ~280 函数 / 全绿 |
| **集成测试**（Stage K + M）| 5 仓 × 3 测试 = 15 / 全绿 |
| **冒烟测试**（Stage L）| 5 脚本 × 24/24 子测 / 全绿 |
| **E2E**（Stage M）| Playwright 2/2 / 全绿 |
| **handler 测试**（Stage N）| 6 测试跨 2 仓 / 全绿 |
| **5 个真实实现 bug** | 全部修复 + forward 断言 |

---

> 最后更新：2026-07-20 · Stage 26-N 收尾 · 与 AGENTS.md § 一 / 二 强约束全绿