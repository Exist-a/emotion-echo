---
stage: e2e-06
title: E2E-06 数据库改造详细执行记录
type: detailed-record
created: 2026-09-18
purpose: 供后续学习参考的完整执行过程
---

# E2E-06 数据库改造 — 详细执行记录

> 本文档记录 E2E-06 阶段的完整执行过程，包含每一步的具体操作、遇到的问题及解决方案，供后续学习参考。

## 目录

1. [执行概览](#1-执行概览)
2. [Phase 1: 基础设施](#2-phase-1-基础设施)
3. [Phase 2: 删除死字段](#3-phase-2-删除死字段)
4. [Phase 3: 新增密保问题字段](#4-phase-3-新增密保问题字段)
5. [Phase 4: 统一软删除](#5-phase-4-统一软删除)
6. [Phase 5: 演示账号改造](#6-phase-5-演示账号改造)
7. [Phase 6: 文档修正](#7-phase-6-文档修正)
8. [运行时验证](#8-运行时验证)
9. [踩坑记录](#9-踩坑记录)
10. [经验总结](#10-经验总结)

---

## 1. 执行概览

### 目标

E2E-06 是数据库改造阶段，核心目标：
- 删除 users 表的 3 个死字段（phone/email/status）
- 新增密保问题表（供找回密码）
- 建立迁移版本追踪机制
- 统一软删除策略
- 创建可管理的演示账号

### 执行时间

2026-09-18，约 6 小时

### 产出物统计

| 类别 | 数量 |
|------|------|
| SQL 迁移文件 | 5 个 |
| Go 代码文件 | 16 个 |
| Proto 改动 | 3 个 |
| Shell 脚本 | 3 个 |
| 前端改动 | 2 个 |
| 文档更新 | 6 个 |

---

## 2. Phase 1: 基础设施

### T1.1 新建 schema_migrations 版本表

**目的**: 记录哪些迁移已被应用，支持 checksum 校验

**操作**:
```sql
-- deploy/db/06-create-schema-migrations.sql
CREATE TABLE IF NOT EXISTS emotion_echo_user.schema_migrations (
    version VARCHAR(255) PRIMARY KEY,
    checksum VARCHAR(64) NOT NULL,
    applied_at TIMESTAMPTZ DEFAULT NOW(),
    execution_ms INTEGER
);
```

**关键设计**:
- `version`: 迁移文件名（不含路径）
- `checksum`: 文件内容 SHA-256，用于检测迁移文件被修改
- `execution_ms`: 执行耗时，用于性能监控

### T1.2 改造 migrate.sh

**目的**: 让 migrate.sh 支持版本追踪，跳过已应用的迁移

**核心改动**:

```sh
# 新增 checksum 计算函数
file_checksum() {
  f="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$f" | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$f" | cut -d' ' -f1
  else
    cksum "$f" | awk '{print $1}'
  fi
}

# 新增版本检查函数
check_migration() {
  version="$1"
  expected_checksum="$2"
  result=$(run_sql "SELECT checksum FROM emotion_echo_user.schema_migrations WHERE version = '$version';")
  # 未应用 → 返回 1
  # 已应用且 checksum 一致 → 返回 0
  # checksum 不一致 → 返回 2
}

# 带版本追踪的迁移执行
run_tracked_sql_file() {
  f="$1"
  checksum=$(file_checksum "$f")
  version=$(basename "$f")
  
  ensure_migrations_table
  
  if check_migration "$version" "$checksum"; then
    log "  SKIP $name（已应用，checksum 一致）"
    return 0
  fi
  
  # 执行迁移并记录
  start_ms=$(date +%s)
  if out=$(run_sql_file "$f"); then
    end_ms=$(date +%s)
    elapsed_ms=$(( (end_ms - start_ms) * 1000 ))
    record_migration "$version" "$checksum" "$elapsed_ms"
    log "  OK  $name（${elapsed_ms}ms）"
  fi
}
```

**行为**:
| 场景 | 行为 |
|------|------|
| 已应用且 checksum 一致 | 跳过（日志 SKIP） |
| checksum 不一致 | 报错（迁移文件被修改） |
| 未应用 | 执行并记录 |

---

## 3. Phase 2: 删除死字段

### T2.1 SQL 迁移

**文件**: `emotion-echo-user-svc/migrations/u001_drop_dead_fields.sql`

```sql
ALTER TABLE emotion_echo_user.users DROP COLUMN IF EXISTS phone;
ALTER TABLE emotion_echo_user.users DROP COLUMN IF EXISTS email;
ALTER TABLE emotion_echo_user.users DROP COLUMN IF EXISTS status;
```

**关键点**: 使用 `IF EXISTS` 保证幂等

### T2.2 清理 Proto 定义

**改动文件**: `proto/user.proto`

```protobuf
// 删除前
message UpdateProfileRequest {
  optional string nickname = 1;
  optional string avatar_url = 2;
  optional int32 gender = 3;
  optional string phone = 4;    // 删除
  optional string email = 5;    // 删除
}

// 删除后
message UpdateProfileRequest {
  optional string nickname = 1;
  optional string avatar_url = 2;
  optional int32 gender = 3;
  // E2E-06: phone/email 已从 users 表删除
  reserved 4, 5;  // 保留字段编号，防止未来误用
}
```

**关键点**: 使用 `reserved` 保留字段编号，保持向后兼容

### T2.3 清理 user-svc 代码

**涉及文件**:
- `internal/model/user.go` — 删除 Phone/Email/Status 字段
- `internal/types/types.go` — 删除 UserInfo.Phone、RegisterReq.Phone
- `internal/logic/authlogic.go` — 清理 phone 映射
- `internal/logic/getmelogic.go` — 清理 phone 映射
- `internal/logic/getuserbyidlogic.go` — 清理 phone 映射
- `internal/grpcserver/user_server.go` — 清理 phone 映射

**示例改动** (authlogic.go):
```go
// 删除前
func toUserInfo(u *model.User) types.UserInfo {
    phone := ""
    if u.Phone != nil {
        phone = *u.Phone
    }
    return types.UserInfo{
        UserId:   u.ID,
        Account:  u.Username,
        Phone:    phone,
        Nickname: nick,
    }
}

// 删除后
func toUserInfo(u *model.User) types.UserInfo {
    return types.UserInfo{
        UserId:   u.ID,
        Account:  u.Username,
        Nickname: nick,
    }
}
```

### T2.4 清理 BFF 代码

**涉及文件**:
- `internal/downstream/user.go` — 删除 UserInfo.Phone
- `internal/downstream/user_grpc.go` — 清理 fromProtoUserInfo

### T2.5 清理 seed 脚本

**文件**: `deploy/db/03-seed-default-users.sql`

```sql
-- 删除前
INSERT INTO emotion_echo_user.users (
    username, password_hash, nickname, status, created_at, updated_at
) VALUES (
    'echo', '...', 'Echo User', 1, NOW(), NOW()
);

-- 删除后
INSERT INTO emotion_echo_user.users (
    username, password_hash, nickname, created_at, updated_at
) VALUES (
    'echo', '...', 'Echo User', NOW(), NOW()
);
```

---

## 4. Phase 3: 新增密保问题字段

### T3.1 SQL 迁移

**文件**: `emotion-echo-user-svc/migrations/u002_create_security_answers.sql`

```sql
CREATE TABLE IF NOT EXISTS emotion_echo_user.user_security_answers (
    user_id BIGINT NOT NULL,
    question_order SMALLINT NOT NULL CHECK (question_order IN (1, 2)),
    question VARCHAR(255) NOT NULL,
    answer_hash VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (user_id, question_order),
    FOREIGN KEY (user_id) REFERENCES emotion_echo_user.users(id) ON DELETE CASCADE
);
```

**设计要点**:
- 联合主键 `(user_id, question_order)` 保证唯一
- `CHECK` 约束限制 question_order 为 1 或 2
- `answer_hash` 使用 bcrypt 哈希（与 password_hash 同套）
- 外键级联删除

### T3.2 Model 层

**新增文件**: `internal/model/security_answer.go`

```go
type SecurityAnswer struct {
    UserID       int64     `gorm:"column:user_id;primaryKey"`
    QuestionOrder int16    `gorm:"column:question_order;primaryKey"`
    Question     string    `gorm:"column:question;size:255"`
    AnswerHash   string    `gorm:"column:answer_hash;size:255"`
    CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
}
```

### T3.3 Repository 层

**新增文件**: `internal/repository/security_answer_repository.go`

```go
type SecurityAnswerRepo interface {
    GetByUserID(ctx context.Context, userID int64) ([]*model.SecurityAnswer, error)
    Save(ctx context.Context, answers []*model.SecurityAnswer) error
    DeleteByUserID(ctx context.Context, userID int64) error
}
```

**实现**: InMemorySecurityAnswerRepo（测试用）+ PostgresSecurityAnswerRepo（生产用）

### T3.4 Logic 层改动

**改动文件**: `internal/logic/authlogic.go`

```go
// Register 方法新增密保验证
func (l *AuthLogic) Register(req *types.RegisterReq) (*types.RegisterResp, error) {
    // ... 原有验证 ...
    
    // E2E-06: 密保问题必填（1~2 个）
    if len(req.SecurityQuestions) < 1 || len(req.SecurityQuestions) > 2 {
        return nil, ErrValidation
    }
    for _, sq := range req.SecurityQuestions {
        if sq.Question == "" || sq.Answer == "" {
            return nil, ErrValidation
        }
    }
    
    // ... 创建用户 ...
    
    // E2E-06: 保存密保问题
    answers := make([]*model.SecurityAnswer, len(req.SecurityQuestions))
    for i, sq := range req.SecurityQuestions {
        answerHash, err := password.Hash(sq.Answer)
        if err != nil {
            return nil, err
        }
        answers[i] = &model.SecurityAnswer{
            UserID:        u.ID,
            QuestionOrder: int16(i + 1),
            Question:      sq.Question,
            AnswerHash:    answerHash,
        }
    }
    if err := l.svcCtx.SecurityAnswerRepo.Save(l.ctx, answers); err != nil {
        return nil, err
    }
    
    return &types.RegisterResp{User: toUserInfo(u)}, nil
}

// 新增验证密保答案方法
func (l *AuthLogic) VerifySecurityAnswer(userID int64, questionOrder int, answer string) error {
    answers, err := l.svcCtx.SecurityAnswerRepo.GetByUserID(l.ctx, userID)
    if err != nil {
        return err
    }
    for _, a := range answers {
        if int(a.QuestionOrder) == questionOrder {
            if !password.Verify(answer, a.AnswerHash) {
                return ErrSecurityAnswerMismatch
            }
            return nil
        }
    }
    return repository.ErrNotFound
}
```

### T3.5 Proto 改动

**新增 message**:
```protobuf
message SecurityQuestion {
  string question = 1;
  string answer = 2;
}

message VerifySecurityAnswerRequest {
  int64 user_id = 1;
  int32 question_order = 2;
  string answer = 3;
}

message VerifySecurityAnswerResponse {
  bool success = 1;
}
```

**修改 RegisterRequest**:
```protobuf
message RegisterRequest {
  // ... 原有字段 ...
  repeated SecurityQuestion security_questions = 6;  // 新增
}
```

**新增 RPC**:
```protobuf
service UserService {
  // ... 原有 RPC ...
  rpc VerifySecurityAnswer (VerifySecurityAnswerRequest) returns (VerifySecurityAnswerResponse);
}
```

### T3.6 BFF 端点改动

**新增端点**: `POST /api/v1/auth/verify-security-answer`

```go
func (h *AuthHandler) verifySecurityAnswer(c *gin.Context) {
    var req struct {
        Username      string `json:"username"`
        QuestionOrder int    `json:"questionOrder"`
        Answer        string `json:"answer"`
    }
    // ... 验证 ...
    
    // 先查用户 ID
    info, err := h.user.Login(c.Request.Context(), req.Username, "dummy")
    if err != nil || info == nil {
        OK(c, gin.H{"success": true})  // 防枚举
        return
    }
    
    err = h.user.VerifySecurityAnswer(c.Request.Context(), info.UserID, req.QuestionOrder, req.Answer)
    if err != nil {
        Fail(c, http.StatusUnauthorized, 1, "security answer verification failed")
        return
    }
    OK(c, gin.H{"success": true})
}
```

---

## 5. Phase 4: 统一软删除

### T4.1 chat-svc conversations 软删除

**SQL 迁移**: `c008_soft_delete_conversations.sql`

```sql
ALTER TABLE emotion_echo_chat.conversations
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_conversations_deleted_at
    ON emotion_echo_chat.conversations(deleted_at)
    WHERE deleted_at IS NULL;
```

**Go Model 改动**:
```go
type Conversation struct {
    // ... 原有字段 ...
    DeletedAt *time.Time `gorm:"column:deleted_at;index"`
}
```

**Repository 改动**:
```go
// 删除前：物理 DELETE
func (r *PostgresConversationRepo) DeleteConversation(ctx context.Context, id int64) error {
    return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        if err := tx.Where("conversation_id = ?", id).Delete(&model.Message{}).Error; err != nil {
            return err
        }
        return tx.Delete(&model.Conversation{}, id).Error
    })
}

// 删除后：软删除
func (r *PostgresConversationRepo) DeleteConversation(ctx context.Context, id int64) error {
    now := time.Now()
    return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        // 软删除消息
        if err := tx.Model(&model.Message{}).
            Where("conversation_id = ? AND deleted_at IS NULL", id).
            Update("deleted_at", now).Error; err != nil {
            return err
        }
        // 软删除会话
        return tx.Model(&model.Conversation{}).
            Where("id = ? AND deleted_at IS NULL", id).
            Update("deleted_at", now).Error
    })
}
```

**查询改动**:
```go
// ListConversations 添加过滤
func (r *PostgresConversationRepo) ListConversations(ctx context.Context, userID int64, limit, offset int) ([]model.Conversation, error) {
    var out []model.Conversation
    err := r.db.WithContext(ctx).
        Where("user_id = ? AND deleted_at IS NULL", userID).  // 新增过滤
        Order("updated_at DESC, id DESC").
        Limit(limit).Offset(offset).
        Find(&out).Error
    return out, err
}
```

### T4.2 messages 级联软删除

**SQL 迁移**: `c009_soft_delete_messages.sql`

```sql
ALTER TABLE emotion_echo_chat.messages
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_messages_deleted_at
    ON emotion_echo_chat.messages(deleted_at)
    WHERE deleted_at IS NULL;
```

### T4.3 user-svc users 统一 gorm.DeletedAt

**改动**:
```go
// 删除前
type User struct {
    // ...
    DeletedAt *time.Time `gorm:"column:deleted_at;index"`
}

// 删除后
type User struct {
    // ...
    DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`
}
```

**关键点**: `gorm.DeletedAt` 会让 GORM 自动给查询加 `WHERE deleted_at IS NULL`

---

## 6. Phase 5: 演示账号改造

### T5.1 可重跑的 seed 脚本

**文件**: `deploy/db/seed-demo-account.sh`

**核心特性**:
- 支持环境变量注入（DEMO_USERNAME/DEMO_PASSWORD/DEMO_NICKNAME）
- 使用 `ON CONFLICT (username) DO UPDATE` 实现幂等
- 自动计算 bcrypt hash（依赖 python + bcrypt）
- 自动创建 2 个密保问题
- 执行后验证

**使用方式**:
```bash
# 默认配置
bash deploy/db/seed-demo-account.sh

# 自定义配置
DEMO_USERNAME=myuser DEMO_PASSWORD=mypass bash deploy/db/seed-demo-account.sh
```

### T5.2 清理脚本

**文件**: `deploy/db/cleanup-demo-account.sh`

**功能**: 删除演示账号及其关联数据（密保、聊天记录等）

**使用方式**:
```bash
bash deploy/db/cleanup-demo-account.sh
```

### T5.3 Playwright spec 改造

**新增文件**: `emotion-echo-web/e2e/helpers/auth.ts`

```typescript
export function getDemoCredentials() {
  return {
    username: process.env.E2E_USERNAME ?? 'echo',
    password: process.env.E2E_PASSWORD ?? 'echo123',
  }
}

export async function loginViaAPI(page: Page): Promise<string> {
  const { username, password } = getDemoCredentials()
  const gatewayUrl = getGatewayUrl()
  const loginResp = await page.request.post(`${gatewayUrl}/api/v1/auth/login`, {
    data: { username, password },
  })
  expect(loginResp.ok()).toBeTruthy()
  const loginData = await loginResp.json()
  return loginData.accessToken
}
```

**前端改动**: `app/pages/login/index.vue`

```typescript
// 删除前
const result = await userStore.login({
  username: 'echo',
  password: 'echo123',
  rememberMe: true,
})

// 删除后
const config = useRuntimeConfig()
const username = (config.public as any).demoUsername ?? 'echo'
const password = (config.public as any).demoPassword ?? 'echo123'
const result = await userStore.login({
  username,
  password,
  rememberMe: true,
})
```

---

## 7. Phase 6: 文档修正

### deploy/db/README.md

全面重写，包含：
- 当前状态（2026-09-18）
- 文件清单（更新为实际文件）
- 迁移机制说明（migrate.sh 工作原理）
- 各 svc 迁移文件清单
- 演示账号管理说明
- 软删除约定
- 跨域查询约定
- 验证命令

---

## 8. 运行时验证

### 8.1 重启 db-migrate 执行新迁移

```bash
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up emotion-echo-db-migrate
```

**结果**: 30 个迁移全部成功执行

### 8.2 验证数据库表

```bash
# 检查 schema_migrations 表
docker exec emotion-echo-postgres psql -U postgres -d emotion_echo \
  -c "SELECT version, applied_at FROM emotion_echo_user.schema_migrations ORDER BY applied_at;"

# 检查 users 表结构（确认 phone/email/status 已删除）
docker exec emotion-echo-postgres psql -U postgres -d emotion_echo \
  -c "\d emotion_echo_user.users"

# 检查 user_security_answers 表
docker exec emotion-echo-postgres psql -U postgres -d emotion_echo \
  -c "\d emotion_echo_user.user_security_answers"

# 检查 conversations 表（确认 deleted_at 存在）
docker exec emotion-echo-postgres psql -U postgres -d emotion_echo \
  -c "\d emotion_echo_chat.conversations"
```

**结果**: 所有表结构正确

### 8.3 重新构建 Go 服务镜像

```bash
# 构建 user-svc
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml build emotion-echo-user-svc

# 构建 chat-svc 和 BFF
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml build emotion-echo-chat-svc emotion-echo-web-bff
```

**关键发现**: Docker 镜像需要重新构建才能使用新代码，仅重启容器不够

### 8.4 重启服务

```bash
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml -f deploy/compose.dev.yml up -d \
  emotion-echo-user-svc emotion-echo-chat-svc emotion-echo-web-bff
```

**结果**: 6 服务全部 healthy

### 8.5 测试 API

```bash
# 测试登录
curl -X POST http://localhost:19080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"echo","password":"echo123"}'

# 测试注册（带密保）
curl -X POST http://localhost:19080/api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{
    "username":"testuser_e2e06",
    "password":"test123456",
    "securityQuestions":[
      {"question":"你的第一只宠物叫什么？","answer":"小花"},
      {"question":"你出生在哪个城市？","answer":"北京"}
    ]
  }'

# 测试软删除
curl -X DELETE "http://localhost:19080/api/v1/conversations/63" \
  -H "Authorization: Bearer $TOKEN"
```

**结果**: 所有 API 正常工作

### 8.6 运行契约测试

```bash
bash deploy/db/test_migrations_contract.sh
```

**结果**: 6/6 全部通过

---

## 9. 踩坑记录

### 坑 1: user-svc migrations 未挂载到 db-migrate 容器

**现象**: 新增的 u001/u002 迁移文件未被执行

**原因**: `docker-compose.apps.yml` 的 db-migrate 服务只挂载了 chat-svc、ai-svc、analytics-svc 的 migrations 目录，遗漏了 user-svc

**解决**: 在 volumes 中添加 user-svc migrations 挂载

```yaml
volumes:
  - ../emotion-echo-user-svc/migrations:/migrations/emotion-echo-user-svc/migrations:ro
```

### 坑 2: Docker 镜像未重新构建

**现象**: 服务启动后报 `column "phone" does not exist` 错误

**原因**: Docker 容器使用的是预先构建的镜像，代码改动后仅重启容器不够，需要重新构建镜像

**解决**: 
```bash
docker compose build emotion-echo-user-svc emotion-echo-chat-svc emotion-echo-web-bff
```

### 坑 3: messages 表遗漏 deleted_at 列

**现象**: 删除会话时报 `column "deleted_at" does not exist` 错误

**原因**: c008 迁移只给 conversations 表添加了 deleted_at，遗漏了 messages 表

**解决**: 创建 c009 迁移文件给 messages 表添加 deleted_at

### 坑 4: user-svc 未在 PRIORITY_ORDER 中登记

**现象**: 契约测试失败

**原因**: migrate.sh 的 PRIORITY_ORDER 只有 chat/ai/analytics，没有 user-svc

**解决**: 在 PRIORITY_ORDER 中添加 user-svc

```sh
PRIORITY_ORDER="emotion-echo-chat-svc emotion-echo-ai-svc emotion-echo-analytics-svc emotion-echo-user-svc"
```

### 坑 5: Go 代码中残留 phone 引用

**现象**: `go build` 失败，报 `u.Phone undefined`

**原因**: user_repository.go 中的 GetByPhone 方法和 InMemoryUserRepo/PostgresUserRepo 的实现仍引用 phone 字段

**解决**: 从接口和实现中删除 GetByPhone 方法

---

## 10. 经验总结

### 10.1 数据库改造流程

1. **先写 SQL 迁移** → 确保 DDL 正确
2. **更新 Go Model** → 与 DDL 对齐
3. **更新 Repository** → 修改查询和写入逻辑
4. **更新 Logic/Handler** → 修改业务逻辑
5. **更新 Proto** → 修改 API 契约
6. **重新生成 pb.go** → 运行 gen.sh
7. **编译验证** → `go build ./...`
8. **重新构建镜像** → `docker compose build`
9. **重启服务** → `docker compose up -d`
10. **运行时验证** → curl 测试 + 契约测试

### 10.2 软删除改造要点

1. **SQL 层**: 添加 deleted_at 列 + 部分索引
2. **Model 层**: 使用 `gorm.DeletedAt` 或 `*time.Time`
3. **Repository 层**: 
   - Delete 方法: 物理 DELETE → UPDATE deleted_at
   - 查询方法: 添加 `WHERE deleted_at IS NULL`
4. **级联删除**: 需要手动处理（GORM 不自动级联软删除）

### 10.3 版本追踪机制

1. **checksum 校验**: 防止迁移文件被修改
2. **幂等执行**: 每次 up 都执行，但已应用的会跳过
3. **自动发现**: glob 模式自动发现新服务的 migrations 目录

### 10.4 Proto 改造规范

1. **删除字段**: 使用 `reserved` 保留字段编号
2. **新增字段**: 使用新的字段编号
3. **重新生成**: 修改后必须运行 `gen.sh`

### 10.5 测试策略

1. **编译验证**: `go build ./...` 确保代码无语法错误
2. **契约测试**: `test_migrations_contract.sh` 确保迁移机制正确
3. **API 测试**: curl 测试确保功能正常
4. **数据库验证**: psql 查询确保数据正确

---

## 附录 A: 完整文件清单

### SQL 迁移文件
- `deploy/db/06-create-schema-migrations.sql`
- `emotion-echo-user-svc/migrations/u001_drop_dead_fields.sql`
- `emotion-echo-user-svc/migrations/u002_create_security_answers.sql`
- `emotion-echo-chat-svc/migrations/c008_soft_delete_conversations.sql`
- `emotion-echo-chat-svc/migrations/c009_soft_delete_messages.sql`

### Go 代码文件
- `emotion-echo-user-svc/internal/model/user.go`
- `emotion-echo-user-svc/internal/model/security_answer.go` (新增)
- `emotion-echo-user-svc/internal/types/types.go`
- `emotion-echo-user-svc/internal/logic/authlogic.go`
- `emotion-echo-user-svc/internal/logic/getmelogic.go`
- `emotion-echo-user-svc/internal/logic/getuserbyidlogic.go`
- `emotion-echo-user-svc/internal/grpcserver/user_server.go`
- `emotion-echo-user-svc/internal/repository/user_repository.go`
- `emotion-echo-user-svc/internal/repository/security_answer_repository.go` (新增)
- `emotion-echo-user-svc/internal/svc/servicecontext.go`
- `emotion-echo-user-svc/main.go`
- `emotion-echo-web-bff/internal/downstream/user.go`
- `emotion-echo-web-bff/internal/downstream/user_grpc.go`
- `emotion-echo-web-bff/internal/handler/auth_handler.go`
- `emotion-echo-chat-svc/internal/model/conversation.go`
- `emotion-echo-chat-svc/internal/repository/conversation_repository.go`

### Proto 文件
- `proto/user.proto`
- `emotion-echo-shared/pkg/emotionuser/user.pb.go`
- `emotion-echo-shared/pkg/emotionuser/user_grpc.pb.go`

### Shell 脚本
- `deploy/db/migrate.sh`
- `deploy/db/seed-demo-account.sh` (新增)
- `deploy/db/cleanup-demo-account.sh` (新增)

### 前端文件
- `emotion-echo-web/e2e/helpers/auth.ts` (新增)
- `emotion-echo-web/app/pages/login/index.vue`

### 配置文件
- `deploy/docker-compose.apps.yml`

### 文档
- `deploy/db/README.md`
- `docs/e2e-roadmap/roadmap.md`
- `docs/e2e-roadmap/discovered-unresolved.md`
- `docs/e2e-roadmap/decisions.md`
- `docs/e2e-roadmap/stages/e2e-06-db-transformation/plan.md`
- `docs/e2e-roadmap/stages/e2e-06-db-transformation/report.md`

---

> 最后更新：2026-09-18 by E2E-06 执行