---
status: landed
landed: 2026-09-11
owner: User
original-plan: docs/plans/grpc-inter-service-migration.md
related-stages:
  - stage-63-bff-grpc-wiring.md
related-adrs:
  - docs/architecture/adr/adr-2026-09-doc-drift-registry.md #26
---

# Sprint C — shared/pkg/ctxkey 重构（解决策 18 #26）

## 一、问题

`grpcinterceptor.CtxUserIDKeyType{}` 与 `middleware.CtxUserIDKey{}` 是两个不同 struct{} 类型，
注释"同义"是错的（`grpcinterceptor/userid.go:28-30` 与 `middleware/jwt_auth.go:33`）。

**实际后果**：4 svc gRPC server 的 userid 拦截器注入 user_id 到 ctx (类型 A)，但 4 svc
logic 读 ctx 用的类型 B → **ctx.Value 永远 miss → 业务 401 "missing user id in context"**。

13 处 logic 受影响：user-svc 4 / chat-svc 5 / assessment-svc 4。

## 二、修法

1. **新建 `emotion-echo-shared/pkg/ctxkey/userid.go`**:
   - `type UserID struct{}` — 唯一 ctx key
   - `WithUserID(ctx, uid) ctx` — 写入
   - `UserIDFromContext(ctx) (int64, bool)` — 读取
   - 包本身不引 grpcinterceptor 或 middleware（避免循环依赖）

2. **`emotion-echo-shared/pkg/middleware/jwt_auth.go`**:
   - `type CtxUserIDKey struct{}` → `type CtxUserIDKey = ctxkey.UserID`

3. **`emotion-echo-shared/pkg/grpcinterceptor/userid.go`**:
   - `type CtxUserIDKeyType struct{}` → `type CtxUserIDKeyType = ctxkey.UserID`

4. 全仓 `grep -rn "CtxUserIDKey\|CtxUserIDKeyType" emotion-echo-*/internal/` 无须改
   （类型别名透明，调用面零变化）。

## 三、测试 (RED → GREEN)

**BFF 跨包等价测试** (`emotion-echo-web-bff/ctxkey_equivalence_test.go`):

```
TestCtxUserIDKey_TypeAlias          ← reflect.TypeOf 比对两类型
TestCtxUserIDKey_CrossReadWrite     ← interceptor 写入 → middleware 读
TestCtxUserIDKey_ReverseDirection   ← middleware 写入 → interceptor 读
```

**ctxkey 包自测** (`emotion-echo-shared/pkg/ctxkey/userid_test.go`):

```
TestWithUserID_RoundTrip
TestUserIDFromContext_Empty
TestWithUserID_OverwriteLast
TestUserID_TypeEmptyStruct         ← 防御性断言零字节 struct{}
```

**3/3 BFF 跨包 RED → GREEN**。

## 四、DoD 验证

| # | 项 | 结果 |
|---|---|---|
| 1 | 5 svc `go test ./...` 全绿 | ✅ |
| 2 | BFF `Transport=""` 默认走 grpc | ✅ (改回) |
| 3 | docker 端到端 users/me 200 | ✅ (原 401 "missing user id in context") |
| 4 | docker 端到端 surveys 200 | ✅ |
| 5 | docker 端到端 reports/daily 200 | ✅ |
| 6 | docker 端到端 conversations 200 | ✅ (CHAT_TRANSPORT=http 绕 #27) |
| 7 | 决策 18 #26 关闭 | ✅ |

## 五、风险

类型别名 (`type A = B`) 编译期等价，运行时同 ctx key，**零行为变化风险**。
唯一约束是 ctxkey 包不引 grpcinterceptor 或 middleware（已遵守）。

## 六、变更清单

| 文件 | 变更 |
|---|---|
| `emotion-echo-shared/pkg/ctxkey/userid.go` | 新建 (54 行) |
| `emotion-echo-shared/pkg/ctxkey/userid_test.go` | 新建 (50 行) |
| `emotion-echo-shared/pkg/middleware/jwt_auth.go` | CtxUserIDKey 改别名 + 引 ctxkey |
| `emotion-echo-shared/pkg/grpcinterceptor/userid.go` | CtxUserIDKeyType 改别名 + 引 ctxkey |
| `emotion-echo-web-bff/ctxkey_equivalence_test.go` | 新建 (3 跨包测试) |
| `emotion-echo-web-bff/internal/config/config.go` | 默认 Transport=http 改回空(grpc) |
| `emotion-echo-web-bff/internal/config/config_test.go` | TestConfig_Transport_DefaultHTTP 改回 DefaultEmpty |
| `emotion-echo-analytics-svc/internal/logic/mentalhealth_trigger_logic_test.go` | 顺手清理 #31 merge 冲突 |
| `emotion-echo-analytics-svc/internal/trigger/trigger_queue_test.go` | 顺手清理 #31 merge 冲突 |
| `deploy/docker-compose.apps.yml` | 5 svc tag v0.1.0 → v0.1.1, BFF v0.1.3 → v0.1.4 |
| `deploy/compose.sprint-c-override.yml` | 新建 (CHAT_TRANSPORT=http 测试用) |

## 七、未做（移出 Sprint C，剩 Sprint D）

- **#27** chat-svc gRPC 6 RPC 全 Unimplemented（SendMessage / ListMessages /
  ListConversations / DeleteConversation / PinConversation / StreamMessages）
- **#32** chat-svc HTTP 端 `/api/v1/conversations` 500（chat-svc logic 本身的问题）
- 两个都是 chat-svc conversations 链路,**根因不同**,Sprint D 合并修

## 八、调研依据

- `emotion-echo-shared/pkg/grpcinterceptor/userid.go`（拦截器注入 ctx 行为）
- `emotion-echo-shared/pkg/middleware/jwt_auth.go`（中间件注入 ctx 行为）
- `emotion-echo-user-svc/internal/logic/getmelogic.go:38`（logic 读 ctx 行为，典型）
- `emotion-echo-chat-svc/internal/logic/*logic.go`（5 处 logic 读 ctx）
- `emotion-echo-assessment-svc/internal/logic/*logic.go`（4 处 logic 读 ctx）
- 决策 18 #25 #26 #27 #31 #32 全文
