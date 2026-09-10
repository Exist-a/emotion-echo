---
status: landed
landed: 2026-09-11
owner: User
original-plan: docs/plans/grpc-inter-service-migration.md
related-stages:
  - stage-63-bff-grpc-wiring.md
related-adrs:
  - docs/architecture/adr/adr-2026-09-doc-drift-registry.md #34 #35
---

# Sprint F1 — user-svc gRPC ResetPassword/Logout 实现

## 一、问题

Sprint E（2026-09-11）补 Login/Register 后，user-svc 仍有 ResetPassword 和 Logout 走 HTTP fallback：
- `ResetPassword` — `user_grpc.go:75-77` 注释 "走 HTTP fallback（user-svc proto 未扩）"
- `Logout` — BFF 端 Logout handler 之前未实现，本次顺手加

## 二、范围

Sprint F1 加 2 RPC + 实现 + BFF client 接入。**Sprint F1 = user-svc 7/7 方法全 gRPC**。

## 三、修法

### 3.1 proto 扩

`proto/user.proto` 加 2 RPC + 4 message：

```proto
rpc ResetPassword (ResetPasswordRequest) returns (ResetPasswordResponse);
rpc Logout (LogoutRequest) returns (LogoutResponse);

message ResetPasswordRequest {
  string username = 1;
  string verification_code = 2;
  string new_password = 3;
}
message ResetPasswordResponse { UserInfo user = 1; }
message LogoutRequest {}
message LogoutResponse { bool success = 1; }
```

### 3.2 user-svc gRPC server 实现

`emotion-echo-user-svc/internal/grpcserver/user_server.go`:
- `ResetPassword`:复用 `logic.NewAuthLogic(ctx, s.svcCtx).ResetPassword(&types.ResetPasswordReq{...})`
- `Logout`:mock auth 无服务端黑名单，仅返 success=true
- `mapAuthError` 加 `ErrInvalidVerifyCode → codes.PermissionDenied`

### 3.3 拦截器跳过清单更新

`emotion-echo-user-svc/internal/grpcserver/server.go`:
- `anonMethods` 加 `ResetPassword`（匿名：用户未登录）
- `Logout` 保留鉴权（userid 拦截器需 x-user-id）

### 3.4 BFF client 接入

`emotion-echo-web-bff/internal/downstream/user_grpc.go`:
- `ResetPassword` 改走 gRPC（不传 x-user-id metadata）
- `Logout` 改走 gRPC（用 withUserID(ctx) 注入 user_id）
- 注释刷新："**所有 7 个 UserClient 方法（Login/Register/ResetPassword/Logout/GetMe/UpdateProfile/GetUserById）全走 gRPC**"

### 3.5 测试 (RED → GREEN)

`emotion-echo-user-svc/internal/grpcserver/user_server_sprint_e_test.go`(扩展, +3 测试):
- `TestUserServer_ResetPassword_Success` — ResetPassword 不再 Unimplemented
- `TestUserServer_ResetPassword_ThenLogin` — 完整流程：重置后用新密码登录成功
- `TestUserServer_Logout_Success` — 鉴权带 x-user-id metadata，返 success=true

## 四、DoD 验证

| # | 项 | 结果 |
|---|---|---|
| 1 | proto 扩 2 RPC + 生成 stub | ✅ |
| 2 | user-svc gRPC server 实现 + 拦截器跳过 ResetPassword | ✅ |
| 3 | 3 RED 测试转 GREEN | ✅ |
| 4 | user-svc `go test ./...` 全绿 | ✅ |
| 5 | BFF 全包 `go test ./...` 全绿 | ✅ |
| 6 | docker 端到端 register → verification-code → reset-password → login with new password 全 200 | ✅ |
| 7 | user-svc v0.1.3 + BFF v0.1.6 rebuild + 重启 | ✅ |
| 8 | 决策 18 #35 登记 | ✅ |

## 五、变更清单

| 文件 | 变更 |
|---|---|
| `proto/user.proto` | + ResetPassword/Logout rpc + 4 message |
| `emotion-echo-shared/pkg/emotionuser/user.pb.go` | gen.sh 重新生成 |
| `emotion-echo-shared/pkg/emotionuser/user_grpc.pb.go` | gen.sh 重新生成 |
| `emotion-echo-user-svc/internal/grpcserver/user_server.go` | + ResetPassword/Logout 实现 + mapAuthError 加 ErrInvalidVerifyCode |
| `emotion-echo-user-svc/internal/grpcserver/server.go` | anonMethods 加 ResetPassword |
| `emotion-echo-user-svc/internal/grpcserver/user_server_sprint_e_test.go` | + 3 测试 |
| `emotion-echo-web-bff/internal/downstream/user_grpc.go` | ResetPassword/Logout 改 gRPC + 注释刷新 |
| `deploy/docker-compose.apps.yml` | user-svc v0.1.2→v0.1.3, BFF v0.1.5→v0.1.6 |

## 六、决策 4 覆盖度最终刷新（2026-09-11 Sprint F1 后）

| 维度 | Sprint E 后 | **Sprint F1 后** | 评注 |
|---|---|---|---|
| 核心业务路径（BFF→4 svc handler 调用） | 21/21 = 100% | **21/21 = 100%** | 不变 |
| BFF→4 svc 全方法 | ~76% | **100%** | user-svc 7/7 全 gRPC（Logout 算上）|
| 所有内部 svc-to-svc（决策 4 全文范围） | ~88% | **~90%** | +ResetPassword/Logout |

**BFF→4 svc 全方法 100% gRPC 覆盖** ✅。

剩余故意不做：
- ai-svc 业务方法 gRPC 化（ai.go MultiModalAnalyze）：Sprint F2 评估
- ai-svc → FER/SenseVoice/XTTS：plan §决策 A 明确不做（FastAPI 模型服务）
- BFF → llm-service（DeepSeek）：外部 API，本就 HTTP
- chat-svc PinConversation/StreamMessages：chat-svc 缺底层功能，留业务触发
- #32 chat-svc HTTP `/api/v1/conversations` 500 bug：根因未查

## 七、调研依据

- `proto/user.proto`（Sprint E 后再加 2 RPC）
- `emotion-echo-user-svc/internal/logic/authlogic.go:152`（ResetPassword 已存在）
- `emotion-echo-web-bff/internal/downstream/user_grpc.go`（HTTP fallback 注释）
- 决策 18 #34 #35 全文
