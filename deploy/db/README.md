# Emotion-Echo · 数据库 README

> 微服务拆分的"铁律 #1"：**每张表归属唯一一个业务域 svc**。

## 当前状态（2026-09-18，E2E-06）

✅ **5 个 schema 已创建**，15+ 张表已分布到各自业务域  
✅ **迁移版本追踪**：`schema_migrations` 表记录已应用的迁移文件 + checksum  
✅ **软删除统一**：users / conversations / messages 均使用 `deleted_at` + GORM 自动过滤  
✅ **密保问题**：`user_security_answers` 表支持 1~2 个密保问题（供找回密码）  
✅ **死字段清理**：users 表已删除 phone / email / status 三个零读写字段

## Schema → Service 映射表

| Schema | 拥有者 svc | 主要职责 |
|--------|----------|---------|
| `emotion_echo_user` | user-svc | 用户、token、密保问题、文件元数据 |
| `emotion_echo_chat` | chat-svc | 会话、消息、outbox 事件 |
| `emotion_echo_ai` | ai-svc | 情绪分析、语音转写、人脸检测、融合情绪 |
| `emotion_echo_assessment` | assessment-svc | 量表、结果、心理健康评估、报告 |
| `emotion_echo_analytics` | analytics-svc | 用户行为事件、统计视图 |

## 文件清单

```
deploy/db/
├── README.md                              ← 本文件
├── 01-create-schemas.sql                  ← 5 个 schema 创建
├── 02-create-tables-in-schemas.sql        ← 15 张表分布到 schema
├── 03-seed-default-users.sql              ← 默认测试用户种子（echo/smoke_user）
├── 04-create-views.sql                    ← 跨 schema VIEWs + analytics_reader 角色
├── 05-drop-user-oauth.sql                 ← 删除 user_oauth 表（ADR 21）
├── 06-create-schema-migrations.sql        ← E2E-06: 迁移版本追踪表
├── migrate.sh                             ← 迁移自动执行脚本（幂等，带版本追踪）
├── seed-demo-account.sh                   ← E2E-06: 可重跑的演示账号种子（带密保）
├── cleanup-demo-account.sh                ← E2E-06: 演示账号清理脚本
├── test_migrations_contract.sh            ← 迁移契约测试
└── test_migrations_no_service_order.sh    ← 验证 glob 自动发现模式
```

## 迁移机制（migrate.sh）

### 设计

- **不使用 initdb.d**：Postgres 的 docker-entrypoint-initdb.d 只在数据卷为空时执行一次，无法表达"持续演进"
- **自动发现**：glob `*/migrations` 自动发现所有带 migrations 目录的服务
- **幂等执行**：每个迁移必须可重复执行（IF NOT EXISTS / OR REPLACE）
- **版本追踪**（E2E-06）：`schema_migrations` 表记录已应用的迁移文件 + checksum

### 执行顺序

```
PRIORITY_ORDER: chat → ai → analytics
理由：analytics 视图依赖 chat/ai 表；ai 视图依赖自己建的表
```

### 版本追踪行为

| 场景 | 行为 |
|------|------|
| 已应用且 checksum 一致 | 跳过（日志 SKIP） |
| checksum 不一致 | 报错（迁移文件被修改） |
| 未应用 | 执行并记录 |

### 使用方式

```bash
# 容器内（compose 自动执行）
docker exec emotion-echo-postgres psql -U postgres -d emotion_echo < migrate.sh

# 宿主机（契约测试复用）
bash deploy/db/migrate.sh
```

## 各 svc 迁移文件

| svc | 迁移目录 | 文件数 | 说明 |
|-----|---------|--------|------|
| chat-svc | `emotion-echo-chat-svc/migrations/` | 9 | outbox、intent、**软删除** |
| ai-svc | `emotion-echo-ai-svc/migrations/` | 10 | 情绪分析、软删除、视图 |
| analytics-svc | `emotion-echo-analytics-svc/migrations/` | 9 | 视图、索引 |
| user-svc | `emotion-echo-user-svc/migrations/` | 2 | **死字段删除**、**密保表** |

## 演示账号管理

### 创建/更新

```bash
bash deploy/db/seed-demo-account.sh
# 环境变量：DEMO_USERNAME、DEMO_PASSWORD、DEMO_NICKNAME
# 默认：echo / echo123 / Echo User
# 自带 2 个密保问题（供 E2E-07 找回密码演示）
```

### 清理

```bash
bash deploy/db/cleanup-demo-account.sh
# 删除演示账号及其关联数据（密保、聊天记录等）
```

### 幂等性

两个脚本均可重复执行，不会产生重复数据。

## 软删除约定

| 表 | 字段 | GORM 类型 | 自动过滤 |
|----|------|----------|---------|
| users | `deleted_at` | `gorm.DeletedAt` | ✅ 是 |
| conversations | `deleted_at` | `*time.Time` | 手动 WHERE |
| messages | `deleted_at` | `*time.Time` | 手动 WHERE |
| emotion_analysis | `deleted_at` | `gorm.DeletedAt` | ✅ 是 |
| face/voice/fused | `deleted_at` | `gorm.DeletedAt` | ✅ 是 |

## 跨域查询约定

**禁止**：跨 schema JOIN  
**允许**：

| 场景 | 通信方式 |
|------|---------|
| chat-svc 需要 user 信息 | gRPC `UserService.GetUser(id)` |
| chat-svc 触发 AI 分析 | Kafka 事件（outbox 模式） |
| analytics-svc 订阅事件 | Kafka consumer |

## 验证命令

```sql
-- 看各 schema 表数量
SELECT schemaname, COUNT(*)
FROM pg_tables
WHERE schemaname LIKE 'emotion_echo_%'
GROUP BY schemaname;

-- 看迁移版本
SELECT version, applied_at FROM emotion_echo_user.schema_migrations ORDER BY applied_at;

-- 看演示账号密保
SELECT u.username, sa.question_order, sa.question
FROM emotion_echo_user.users u
JOIN emotion_echo_user.user_security_answers sa ON u.id = sa.user_id
WHERE u.username = 'echo';
```

---

> 最后更新：2026-09-18 by E2E-06 数据库改造阶段