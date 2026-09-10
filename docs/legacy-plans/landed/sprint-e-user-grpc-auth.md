---
status: landed
landed: 2026-09-11
owner: User
original-plan: docs/plans/grpc-inter-service-migration.md
related-stages:
  - stage-63-bff-grpc-wiring.md
related-adrs:
  - docs/architecture/adr/adr-2026-09-doc-drift-registry.md #33 #34
---

# Sprint E — user-svc gRPC Login/Register 实现（解决策 18 #33）

## 一、问题

`proto/user.proto`（Stage 62 PR-3.1 commit）只定义 3 RPC：
- `GetMe`
- `UpdateProfile`
- `GetUserById`

**完全没有 `Login` / `Register` / `ResetPassword` / `Logout` RPC**。

后果：
- BFF `userGRPCClient.Login/Register` 仍走 HTTP fallback（user_grpc.go:79-87 注释明写）
- 用户注册/登录高频路径与"决策 4 内部 svc-to-svc = gRPC"不符
- 与 `emotion-echo-user-svc/internal/grpcserver/user_server.go` 注释 "GetMe/UpdateProfile/GetUserById" 一致——确实只有 3 RPC

## 二、范围

Sprint E 实现 **Login + Register gRPC**（高频核心路径）。**ResetPassword / Logout 留 Sprint F**（ResetPassword 是 Sprint 1 PR-4c-4 之后才有，频率低；Logout 当前 BFF 端未实现，无紧迫性）。

## 三、修法

### 3.1 proto 扩

`proto/user.proto` 加 2 RPC + 4 message：

```proto
rpc Login (LoginRequest) returns (LoginResponse);
rpc Register (RegisterRequest) returns (RegisterResponse);

message LoginRequest { string username = 1; string password = 2; }
message LoginResponse { UserInfo user = 1; }
message RegisterRequest {
  string username = 1;
  string password = 2;
  optional string verification_code = 3;
  optional string phone = 4;
  optional string nickname = 5;
}
message RegisterResponse { UserInfo user = 1; }
```

**字段编号续号**（不重用旧编号，保证旧 client 兼容）。

### 3.2 user-svc gRPC server 实现

`emotion-echo-user-svc/internal/grpcserver/user_server.go`:
- `Login(ctx, req)`:复用 `logic.NewAuthLogic(ctx, s.svcCtx).Login(&types.LoginReq{...})`
- `Register(ctx, req)`:复用 `logic.NewAuthLogic(ctx, s.svcCtx).Register(&types.RegisterReq{...})`
- `mapAuthError` 把 logic 错误映射到 gRPC status code:
  - `ErrInvalidCredentials` → `codes.Unauthenticated`
  - `ErrUsernameTaken` → `codes.AlreadyExists`
  - `ErrValidation` → `codes.InvalidArgument`

### 3.3 拦截器跳过 Login/Register（匿名调用）

`emotion-echo-user-svc/internal/grpcserver/server.go`:
- `newServiceAwareUserIDInterceptor` 改签名加 `anonMethods ...string` 变参
- 调用方传 `emotionuser.UserService_Login_FullMethodName` + `UserService_Register_FullMethodName` 进跳过清单
- 用 method name 精确匹配（map 查），避免 HasPrefix 误伤

### 3.4 BFF client 接入

`emotion-echo-web-bff/internal/downstream/user_grpc.go`:
- `Login` / `Register` 改走 gRPC（调 proto stub）
- **不传** metadata x-user-id（Login/Register 匿名调用，`withUserID(ctx)` 不能用）
- `ResetPassword` 仍走 HTTP fallback（Sprint F）
- 注释刷新："5 个 gRPC 方法 + 1 个 HTTP fallback"

### 3.5 测试 (RED → GREEN)

`emotion-echo-user-svc/internal/grpcserver/user_server_sprint_e_test.go`(新建):
- `TestUserServer_Register_Success` — Register 创建用户返 UserInfo
- `TestUserServer_Login_AfterRegister` — Register → Login 完整流程
- `TestUserServer_Login_WrongPassword` — 错误密码返 Unauthenticated 而非 OK

## 四、DoD 验证

| # | 项 | 结果 |
|---|---|---|
| 1 | proto 扩 Login/Register + 生成 stub | ✅ (`bash proto/gen.sh user.proto`) |
| 2 | user-svc gRPC server Login/Register 实现 + 拦截器跳过 auth | ✅ |
| 3 | 3 RED 测试转 GREEN | ✅ |
| 4 | user-svc `go test ./...` 全绿 | ✅ |
| 5 | BFF 全包 `go test ./...` 全绿 | ✅ |
| 6 | user-svc v0.1.2 + BFF v0.1.5 rebuild + docker 重启 | ✅ |
| 7 | docker 端到端 register 走 gRPC 200 | ✅ |
| 8 | docker 端到端 login 走 gRPC 200 + 返 accessToken | ✅ |
| 9 | docker 端到端 users/me 走 gRPC 200 (新注册用户 id=6) | ✅ |
| 10 | 决策 18 #33 close | ✅ |

## 五、变更清单

| 文件 | 变更 |
|---|---|
| `proto/user.proto` | + Login/Register rpc + 4 message + 注释更新 |
| `emotion-echo-shared/pkg/emotionuser/user.pb.go` | gen.sh 重新生成 |
| `emotion-echo-shared/pkg/emotionuser/user_grpc.pb.go` | gen.sh 重新生成 |
| `emotion-echo-user-svc/internal/grpcserver/user_server.go` | + Login/Register 实现 + mapAuthError + 注释 |
| `emotion-echo-user-svc/internal/grpcserver/server.go` | newServiceAwareUserIDInterceptor 签名加 anonMethods + 传 Login/Register |
| `emotion-echo-user-svc/internal/grpcserver/user_server_sprint_e_test.go` | 新建 (3 测试) |
| `emotion-echo-web-bff/internal/downstream/user_grpc.go` | Login/Register 改 gRPC + 注释 |
| `deploy/docker-compose.apps.yml` | user-svc v0.1.1→v0.1.2, BFF v0.1.4→v0.1.5 |

## 六、未做（移 Sprint F）

- **ResetPassword gRPC 化**：proto 缺 + user-svc gRPC server 缺 + BFF client 缺
- **Logout gRPC 化**：BFF 端 Logout handler 未实现，无紧迫性
- **#32** chat-svc HTTP 端 `/api/v1/conversations` 500 bug：仍 open
- **ai-svc 业务方法 gRPC 化**（ai.go 的 MultiModalAnalyze 等）：proto 是否扩待评估
- **chat-svc PinConversation / StreamMessages**：chat-svc 缺底层功能，留业务触发后

## 七、决策 4 覆盖度刷新（2026-09-11 Sprint E 收口）

| 维度 | Sprint C/D 后 | Sprint E 后 | 评注 |
|---|---|---|---|
| 核心业务路径（BFF→4 svc handler 调用） | 19/19 = 100% | **21/21 = 100%** | +2 (Login/Register) |
| BFF→4 svc 全方法 | ~70% | **~76%** | ResetPassword + Logout 仍 HTTP |
| 所有内部 svc-to-svc（决策 4 全文范围） | ~85% | **~88%** | |

## 八、调研依据

- `proto/user.proto`（Stage 62 PR-3.1 commit, 只 3 RPC）
- `emotion-echo-user-svc/internal/grpcserver/user_server.go`（现有 3 RPC 实现）
- `emotion-echo-user-svc/internal/logic/authlogic.go`（Login/Register/ResetPassword 已存在）
- `emotion-echo-web-bff/internal/downstream/user_grpc.go`（HTTP fallback 注释）
- `emotion-echo-web-bff/internal/handler/auth_handler.go`（accessToken 由 BFF jwt.Sign 签发）
- 决策 18 #33 #34 全文
