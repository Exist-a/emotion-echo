# Emotion-Echo · Stage 8a 用户资料修改功能（已完成 ✅）

> 2026-07-14：user-svc 业务能力增强，新增 PATCH /api/v1/users/me。
> 这是 legacy handler 迁移的最小切片（Stage 8 第一步）。

## 🏆 战果

| 维度 | 数据 |
|------|------|
| 新增端点 | `PATCH /api/v1/users/me` |
| Logic | 1 个（UpdateProfileLogic） |
| Repo 新方法 | 1 个（UpdateProfile，nil-safe partial update） |
| TDD 测试 | **15 个**（3 repo + 7 logic + 5 已有）全部 PASS |
| 编译 | ✅ 通过 |
| e2e 验证 | ✅ 端到端通过（修改前→修改后真的变了） |

## 🟢🟢🟢 端到端验证证据

```
=== 测试 1：无 JWT 鉴权拦截 ===
PATCH /api/v1/users/me (无 Authorization header)
→ HTTP 401 ✅

=== 测试 2：修改前 ===
GET /api/v1/users/me (JWT, user_id=1)
→ nickname="Stage 8 测试"

=== 测试 3：PATCH 修改 ===
PATCH /api/v1/users/me
Body: {"nickname":"Alice 新昵称","gender":2}
→ 200 OK + 更新后的用户信息

=== 测试 4：修改后 ===
GET /api/v1/users/me
→ nickname="Alice 新昵称"  ✅ 真的变了！

=== 测试 5：业务校验 ===
PATCH Body: {"nickname":"abcdefghijklmnopqrstuvwxyz012345678"}  (33字符)
→ {"error":"validation: nickname length must be 1-32"}  ✅
```

## 📁 改动文件清单

### 修改
- `emotion-echo-user-svc/internal/repository/user_repository.go`
  - 接口加 `UpdateProfile(ctx, id, nickname, gender, birthday, avatarURL) error`
  - InMemoryUserRepo.UpdateProfile 实现
  - PostgresUserRepo.UpdateProfile 实现（用 `Updates(map)` 做局部更新）
- `emotion-echo-user-svc/internal/types/types.go`
  - 新增 `UpdateProfileReq`（所有字段 optional）
  - 新增 `UpdateProfileResp`（= GetMeResp 别名）
- `emotion-echo-user-svc/internal/handler/user_handler.go`
  - 新增 `UpdateProfileHandler`（Gin HandlerFunc）
- `emotion-echo-user-svc/main.go`
  - 注册 `r.PATCH("/api/v1/users/me", ...)`

### 新增
- `emotion-echo-user-svc/internal/logic/updateprofilelogic.go`
- `emotion-echo-user-svc/internal/logic/updateprofilelogic_test.go`（7 个测试）
- `emotion-echo-user-svc/internal/repository/user_repository_test.go`（追加 3 个）
- `deploy/apisix/route-user-update.json`（APISIX 路由配置，已 PUT 到 etcd）

## 🎯 API 设计

```
PATCH /api/v1/users/me
Authorization: Bearer <JWT>     ← 由 APISIX jwt-auth 验证
Content-Type: application/json

Body（所有字段 optional）：
{
  "nickname": "New Alice",      // 1-32 字符
  "gender": 1,                   // 0=unknown 1=male 2=female
  "birthday": "1990-05-15",     // YYYY-MM-DD
  "avatarUrl": "https://..."    // URL
}

200 OK
{
  "user": {
    "userId": 1,
    "account": "alice",
    "phone": "13800138000",
    "nickname": "New Alice"
  }
}
```

### nil-safe partial update（关键设计）

| 调用方式 | 行为 |
|---------|------|
| `{"nickname":"X"}` | 只改 nickname，其他不动 |
| `{"nickname":"X","gender":1}` | 改 nickname + gender |
| `{}` 空对象 | 不报错，无更新 |
| `{"birthday":"1990-05-15"}` | 解析为 time.Time 写库 |
| `{"birthday":"bad"}` | 400 校验失败 |

## 🎯 APISIX 路由配置（r-user-update）

```json
{
  "id": "r-user-update",
  "uri": "/api/v1/users/me",
  "methods": ["PATCH"],
  "upstream_id": "user-svc",
  "plugins": {
    "jwt-auth":      { "key": "user_jwt" },
    "limit-count":   { "count": 30, "time_window": 60, "key": "X-User-Id" },
    "api-breaker":   { "unhealthy": {...}, "healthy": {...} }
  }
}
```

PATCH 比 GET 更敏感（30/min 而非 60/min）。

## 🎯 TDD 测试覆盖（全部 PASS）

### repo 层（追加 3 个测试）
- ✅ `TestUserRepo_InMemory_UpdateProfile_Existing_UpdatesFields`
- ✅ `TestUserRepo_InMemory_UpdateProfile_NotFound_ReturnsErrNotFound`
- ✅ `TestUserRepo_InMemory_UpdateProfile_NilFields_DoesNotTouch`

### logic 层（新增 7 个测试）
- ✅ `TestUpdateProfileLogic_HappyPath_UpdatesAndReturnsLatest`
- ✅ `TestUpdateProfileLogic_NoUserID_Unauthorized`
- ✅ `TestUpdateProfileLogic_NicknameTooLong_ValidationError`
- ✅ `TestUpdateProfileLogic_GenderOutOfRange_ValidationError`
- ✅ `TestUpdateProfileLogic_BirthdayInvalidFormat_ValidationError`
- ✅ `TestUpdateProfileLogic_BirthdayValid_Parsed`
- ✅ `TestUpdateProfileLogic_UserNotFound_ReturnsErrNotFound`

**共 10 个新测试**，覆盖：
- ✅ happy path（修改前 → 修改后真的变了）
- ✅ 鉴权（无 user_id）
- ✅ 字段校验（昵称长度、性别范围、生日格式）
- ✅ 不存在的 user
- ✅ nil-safe partial update
- ✅ 端到端业务流（PATCH → GET 看到新值）

## 🎓 关键设计洞察

### 1. **不走 legacy service 层**

| 选项 | 评价 |
|------|------|
| A. 搬整个 `service.UserService` 进来 | ❌ 需要 refactor 单体 AppContext、改所有依赖 |
| **B. 直接在 svc 内写 logic** | ✅ 工作量可控，复用已有 repository |

我们选 B。Stage 8 不应该一次完成所有 14 个 handler 迁移。

### 2. **nil-safe partial update 模式**

```go
// 关键代码
if nickname != nil {
    updates["nickname"] = *nickname
}
if len(updates) == 0 {
    return nil  // 无字段需更新
}
```

让客户端可以只传想改的字段，不需要重发所有字段。

### 3. **复用 getmelogic 的 toGetMeResp**

```go
return toGetMeResp(u), nil // 复用现有转换函数
```

零重复代码。

### 4. **校验集中在 logic 层**

| 层 | 责任 |
|----|------|
| handler | 解析 JSON、调 logic |
| logic | 业务校验 + 数据组装 |
| repo | 纯 DB 操作 |

## 🚦 部署动作（已完成）

| 步骤 | 命令 | 状态 |
|------|------|------|
| 1. 编译 | `go build -o user-svc.exe .` | ✅ |
| 2. 启动 svc | `Start-Process -FilePath ".\user-svc.exe"` | ✅ |
| 3. 健康检查 | `curl /user-health` | ✅ dbOk:true |
| 4. APISIX 注册路由 | `curl PUT .../apisix/admin/routes/r-user-update` | ✅ |
| 5. 端到端业务 | PATCH → GET 验证 DB 写入 | ✅ |

## ⚠️ 过程中的坑

1. **etcd 容器退出 2 小时**
   - APISIX 配置中心断了，所有路由无法读取
   - 重启 `docker start emotion-echo-etcd` 恢复
2. **用户 handler 文件 import 块被破坏**
   - SearchReplace 在中文注释场景下偶发丢失换行
   - 解决：用 Write 重写整个文件
3. **PowerShell `curl` 是 Invoke-WebRequest 别名**
   - 必须用 `curl.exe` 显式指定

## 📊 项目进度

```
Phase 0 基础设施       ████████████████████ 100% ✅
Phase 1 微服务拆分      ████████████████████ 100% ✅
Phase 2 Kafka          ████████████████████ 100% ✅
Phase 3 LLM 接入       ████████████████████ 100% ✅
Phase 4 业务深化        █████████████░░░░░  75%
Phase 5 Nacos 删除     ████████████████████ 100% ✅
Phase 6 APISIX 安全    ████████████████████ 100% ✅
Phase 7 Gin 化迁移     ████████████████████ 100% ✅
Phase 8 legacy 迁移    ███░░░░░░░░░░░░░░░░  15%
                       user-svc: UpdateProfile 完成 ✅
                       e2e 验证通过 ✅
                       assessment-svc ⏳
                       其他 svc ⏳
Phase 9 K8s           ░░░░░░░░░░░░░░░░░░░░   0%
Phase 10 gRPC          ░░░░░░░░░░░░░░░░░░░░   0%
```

## 🎯 Stage 8b 下一步候选

| handler | svc | 工作量 | 价值 |
|---------|-----|--------|------|
| **ListSurveys** | assessment-svc | 30min | 量表列表（核心） |
| **SubmitSurveyResponse** | assessment-svc | 1h | 提交答案（核心） |
| GetMentalHealthReport | assessment-svc | 2h | 心理报告 |
| GetBehaviorReport | analytics-svc | 1.5h | 行为报告 |
| StreamChat | ai-svc | 2h | AI 流式对话 |
| Login | user-svc | 4h | 真实登录（password + JWT + refresh_token） |

**推荐顺序**：ListSurveys → SubmitSurveyResponse → StreamChat（按 ROI）

---

**关键洞察**：Stage 8 不应该是"14 个 handler 一次迁移"，而应该是**最小可交付切片**。本次只做了 user-svc 的 UpdateProfile 一个端点，但完整覆盖了 TDD → 编译 → 部署 → e2e 全流程，证明了迁移模式成立。后续每个端点都可以用相同模式：扩 repo → 写 logic + 测试 → 写 handler → 注册 main → APISIX 路由 → e2e。