# Stage 63 · 2026-09-10 BFF gRPC 接线启用（修复 PR-3 "建好了但没上线" bug）

> **状态**：🟡 **代码已 landed + 单测全绿 · 待 docker 端到端验证**
> **关联**：[`docs/stages/stage-62-landing.md`](stage-62-landing.md) §五.4（明确点名此遗留）·
> [`docs/plans/grpc-inter-service-migration.md`](../plans/grpc-inter-service-migration.md) ·
> [`docs/architecture/decisions.md`](../architecture/decisions.md) 决策 4（跨服务调用 = gRPC）
> **Commit**：`3d3f0ea`

---

## 一、问题（bug 根因）

### 1.1 现象

Stage 62 PR-3（4 子 PR）落地了：
- 3 svc gRPC server（user :8887 / assessment :8886 / analytics :8885）
- chat-svc gRPC server（:8892，Stage 58 PR-GRPC-3 已落地）
- BFF 4 个 gRPC client 实现（`internal/downstream/{user,chat,assessment,analytics}_grpc.go`）
- docker 冒烟 7/7 PASS（直连 svc gRPC 端口）

**但 BFF 生产路径仍全走 HTTP。**

### 1.2 根因链

`emotion-echo-web-bff/main.go` `buildServiceContext`（修复前 `:179-192`）构造 4 个下游 client 时：

```go
svcCtx.SetUser(downstream.NewUserClient(downstream.UserClientOptions{
    BaseURL: c.UserService.BaseURL, TimeoutMs: c.UserService.TimeoutMs,
    // ← 缺 Transport，缺 GRPCConn
}))
```

而下游工厂逻辑（以 `user.go:111-135` 为例）：

```go
func NewUserClient(opts UserClientOptions) UserClient {
    if opts.Transport == "" || opts.Transport == UserTransportGRPC {
        if opts.GRPCConn != nil {       // ← nil，进不来
            return &userGRPCClient{...}
        }
    }
    // 静默 fallback 到 HTTP
    return &userHTTPClient{...}
}
```

**结论**：`Transport` 默空 → 工厂按 "grpc" 处理 → 但 `GRPCConn==nil` → 静默降级 HTTP。
gRPC server 在跑、client 实现存在、冒烟直连通过——**但 BFF 从来没 dial 过任何一个 gRPC 连接**。
决策 4（跨服务调用 = gRPC）在 BFF→4 svc 这条链路上等于没落地。

### 1.3 为什么 Stage 62 没发现

Stage 62 PR-3.4 的 docker 冒烟（`scripts/grpc_smoke/`）是**独立 gRPC 客户端直连 svc gRPC 端口**，
不经过 BFF。所以冒烟 7/7 只能证明 "svc gRPC server 可达"，不能证明 "BFF 实际走 gRPC"。
Stage 62 报告 §五.4 自己也登记了这个遗留，但标为 "下一 sprint"。

---

## 二、修复内容

### 2.1 config 层（`internal/config/config.go`）

`HTTPService` 结构体新增两个字段：

```go
type HTTPService struct {
    BaseURL   string
    TimeoutMs int
    GRPCAddr  string   // 新增：gRPC server 地址（host:port）
    Transport string   // 新增："grpc"(默认空) | "http"(强制回滚)
}
```

**默认值**（`SetDefaults`）：

| 服务 | GRPCAddr | 依据 |
|---|---|---|
| user-svc | `localhost:8887` | `deploy/docker-compose.apps.yml:95` |
| chat-svc | `localhost:8892` | `deploy/docker-compose.apps.yml:150`（Stage 58 PR-GRPC-3） |
| assessment-svc | `localhost:8886` | `deploy/docker-compose.apps.yml:266` |
| analytics-svc | `localhost:8885` | `deploy/docker-compose.apps.yml:211` |

**env 覆盖**（`ApplyEnvOverrides`，共 8 个新 env）：

```
USER_SVC_GRPC_ADDR / CHAT_SVC_GRPC_ADDR / ASSESSMENT_SVC_GRPC_ADDR / ANALYTICS_SVC_GRPC_ADDR
USER_TRANSPORT / CHAT_TRANSPORT / ASSESSMENT_TRANSPORT / ANALYTICS_TRANSPORT
```

`*_TRANSPORT=http` 是**回滚开关**：设了之后即使 gRPC 地址配置了也强制走 HTTP，无需改代码重新部署。

### 2.2 main.go 接线（`main.go`）

新增两个可测试组件：

```go
// grpcDialer 包级变量，测试可覆盖为 bufconn dialer
var grpcDialer = func(addr string) (*grpc.ClientConn, error) {
    return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

// dialGRPC dial 下游 gRPC；addr 空或 dial 失败 → 返 nil（调用方降级 HTTP）
func dialGRPC(addr, name string) *grpc.ClientConn
```

`buildServiceContext` 在构造 client 前 dial 4 个连接：

```go
userGRPCConn := dialGRPC(c.UserService.GRPCAddr, "user-svc")
chatGRPCConn := dialGRPC(c.ChatService.GRPCAddr, "chat-svc")
assessmentGRPCConn := dialGRPC(c.AssessmentService.GRPCAddr, "assessment-svc")
analyticsGRPCConn := dialGRPC(c.AnalyticsService.GRPCAddr, "analytics-svc")

svcCtx.SetUser(downstream.NewUserClient(downstream.UserClientOptions{
    BaseURL: ..., TimeoutMs: ...,
    Transport: downstream.UserTransport(c.UserService.Transport),
    GRPCConn:  userGRPCConn,    // ← 修复前缺失
}))
// (chat / assessment / analytics 同理)
```

**容错设计**：
- `GRPCAddr` 为空 → 不 dial，直接 HTTP
- dial 失败 → 打 `[grpc] xxx dial failed (fallback to HTTP)` 日志，返 nil，不阻塞启动
- `Transport="http"` → 即使 conn 非 nil 也走 HTTP（回滚开关）

### 2.3 配置文件同步

| 文件 | 改动 |
|---|---|
| `etc/web-bff.yaml` | 4 个下游加 `GRPCAddr` 字段 + 端口注释 |
| `deploy/docker-compose.apps.yml` | BFF environment 加 4 个 `*_SVC_GRPC_ADDR`（容器 DNS） |

---

## 三、TDD 过程

### 3.1 RED（先写失败测试）

**config 测试**（`internal/config/config_test.go`，+5）：

```
TestConfig_DownstreamGRPCAddr_Defaults          ← RED FAIL（GRPCAddr 字段不存在）
TestConfig_ApplyEnvOverrides_GRPCAddr
TestConfig_ApplyEnvOverrides_GRPCAddr_Empty
TestConfig_Transport_DefaultEmpty
TestConfig_ApplyEnvOverrides_Transport
```

**接线测试**（`main_grpc_test.go`，新文件，+6）：

```
TestBuildServiceContext_GRPCWiring_AllFourClients     ← RED FAIL（grpcDialer/dialGRPC 不存在）
TestBuildServiceContext_TransportHTTP_ForcesHTTPFallback
TestBuildServiceContext_GRPCAddrEmpty_HTTPFallback
TestBuildServiceContext_DialFailure_HTTPFallback
TestDialGRPC_EmptyAddr_ReturnsNil
TestDialGRPC_InvalidAddr_ReturnsNil
```

核心测试 `TestBuildServiceContext_GRPCWiring_AllFourClients` 用 `bufconn` 启动 4 个
in-process gRPC server，覆盖 `grpcDialer`，调用 `buildServiceContext`，用反射断言
返回的 4 个 client 类型名含 "GRPC"（而非 "HTTP"）。

### 3.2 GREEN（最小实现）

1. config 加字段 + 默认值 + env 覆盖 → config 5 测试转绿
2. main.go 加 `grpcDialer` + `dialGRPC` + buildServiceContext 接线 → 6 接线测试转绿

### 3.3 REFACTOR

- 删除测试文件中未使用的 `startBufGRPCServer` helper
- 确认 `go vet ./...` 无警告

---

## 四、验证结果

### 4.1 已验证（单测层）

```
$ go test ./emotion-echo-web-bff/...
ok  emotion-echo-web-bff              0.972s   (含 6 新接线测试 + 原有路由契约测试)
ok  emotion-echo-web-bff/internal/config   0.751s   (含 5 新 config 测试)
ok  emotion-echo-web-bff/internal/downstream (cached)
ok  emotion-echo-web-bff/internal/handler   0.737s
... (其余包 cached / no test files)

$ go vet ./emotion-echo-web-bff/...
(无输出 = 干净)
```

### 4.2 待验证（docker 端到端）🟡

以下项目需在 `docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d --build` 后验证：

| # | 验证项 | 方法 | 预期 |
|---|---|---|---|
| V1 | BFF 启动日志含 4 条 gRPC connected | `docker logs emotion-echo-web-bff \| grep "\[grpc\]"` | `user-svc/chat-svc/assessment-svc/analytics-svc connected` 各 1 条 |
| V2 | gRPC 冒烟仍 7/7 | `bash scripts/grpc_smoke/run.sh` | 7 PASS / 0 FAIL |
| V3 | 经 APISIX 真实登录 + 业务调用 | `curl` login → users/me → conversations → surveys → reports/daily | 全 HTTP 200 + 数据正常 |
| V4 | 回滚开关生效 | BFF env 加 `USER_TRANSPORT=http` → 重启 → 调 users/me | 仍 200（走 HTTP fallback） |
| V5 | gRPC server 不可用时降级 | 停 user-svc 容器 → 调 users/me | 返 502/错误（不 panic；BFF 启动时已 dial 成功，运行时断连由 gRPC client 重试/报错） |

> **注意 V5**：当前 gRPC client 方法（如 `userGRPCClient.GetMe`）在 gRPC 调用失败时**直接返 error**，
> 不自动 fallback 到 HTTP。这是现有 gRPC client 实现的设计（Stage 62 PR-3.3），不在本 bug 修复范围。
> 如需运行时自动降级，需单独开 sprint 改 `*_grpc.go` 的每个方法。

---

## 五、与决策栈的关系

| 决策 | 关系 |
|---|---|
| **决策 4**（跨服务调用 = gRPC + .proto） | 本修复让决策 4 在 BFF→user/chat/assessment/analytics 4 条链路上**真正生效**（之前是 "代码存在但运行时走 HTTP"） |
| **决策 18**（doc-drift registry） | 本 bug 是决策 18 §三 类型 5（自报告失真）的变体：Stage 62 报告 "PR-3 全落地" 但实际 BFF 没接线。**建议在决策 18 台账登记新实例**（待 owner 确认编号） |
| **决策 12**（BFF = 纯聚合层） | 无冲突；BFF 仍只做聚合，只是传输层从 HTTP 切到 gRPC |

---

## 六、未做（不在本修复范围）

1. **运行时 gRPC→HTTP 自动降级**：当前 gRPC 方法调用失败直接返 error，不 fallback。需单独 sprint 改 `*_grpc.go`。
2. **chat-svc gRPC 全覆盖**：chat-svc gRPC server（Stage 58）和 BFF chatGRPCClient 已存在，但本修复只做接线，不扩展 RPC 覆盖范围。
3. **ai-svc → FER/SenseVoice/XTTS 的 HTTP→gRPC 改造**：这 3 个模型服务是 FastAPI，仍走 HTTP（`grpc-inter-service-migration.md` §1.3 第 5 条）。
4. **决策 18 台账登记**：建议登记但需 owner 确认编号，本文不擅自写入 `adr-2026-09-doc-drift-registry.md`。
5. **`/apisix-health` 路由 503**：Stage 62 §五.4 遗留的另一个小 bug，与本修复无关，留待后续。

---

## 七、调研依据（AGENTS.md §〇 文档功课）

**已读代码**（≥3 实现 + ≥1 测试）：

| 文件 | 关键发现 |
|---|---|
| `emotion-echo-web-bff/main.go:167-238` | `buildServiceContext` 构造 4 client 只传 BaseURL，不传 GRPCConn |
| `emotion-echo-web-bff/internal/downstream/user.go:111-135` | 工厂逻辑：Transport=grpc + GRPCConn==nil → 静默 HTTP |
| `emotion-echo-web-bff/internal/downstream/user_grpc.go:26-37` | gRPC client 实现存在，需非 nil conn |
| `emotion-echo-web-bff/internal/config/config.go:19-22` | `HTTPService` 原只有 BaseURL + TimeoutMs |
| `emotion-echo-web-bff/internal/config/config_test.go` | 现有测试模式（env 覆盖 + 默认值断言） |
| `deploy/docker-compose.apps.yml:95/150/211/266` | 4 svc gRPC 端口：8887/8892/8885/8886 |
| `emotion-echo-web-bff/etc/web-bff.yaml` | 原配置无 GRPCAddr 字段 |

**已查 ADR / stage**：
- `docs/architecture/decisions.md` 决策 4（`:85-93`）
- `docs/stages/stage-62-landing.md` §五.4（明确点名 "BFF main.go 尚未接线 gRPC 连接"）
- `docs/plans/grpc-inter-service-migration.md` §1.3（仍是 HTTP 的内部调用清单）

**架构假设清单**（本文假设）：
1. 假设 4 svc gRPC server 在 docker compose 中正常启动（Stage 62 PR-3.2 已落地 + 冒烟 7/7）
2. 假设 gRPC 调用不需要 mTLS（内部服务间，compose 网络隔离；与现有 ai-svc gRPC 一致用 insecure）
3. 假设 `grpc.NewClient` 懒连接不影响启动（dial 阶段不建真实连接，首次 RPC 时才建）
4. 假设 BFF 单实例（gRPC conn 无需连接池管理；多实例时每个 BFF 实例独立 dial）

---

> 最后更新：2026-09-10 by Stage 63 session
> 状态：🟡 代码 landed + 单测全绿 · 待 docker 端到端验证（§四.2 V1-V5）
> 下一步：重建 BFF 镜像 → docker compose up → 验证 V1-V5 → 全过后翻 🟢 + 登记决策 18 台账
