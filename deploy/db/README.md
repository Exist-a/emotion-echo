# Emotion-Echo · 数据库拆分 README

> 微服务拆分的"铁律 #1"：**每张表归属唯一一个业务域 svc**。

## 当前状态（2026-07-13）

✅ **5 个 schema 已创建**，15 张表已分布到各自业务域  
🟡 **数据尚未迁移**（public 旧表保留 11 张，可对照数据）  
🟡 **旧 Gin 应用尚未切换**连接串

## Schema → Service 映射表

| Schema | 拥有者 svc | gRPC 服务名 | 主要职责 |
|--------|----------|-----------|---------|
| `emotion_echo_user` | user-svc | `UserRpc` | 用户、token、OAuth、文件元数据 |
| `emotion_echo_chat` | chat-svc | `ChatRpc` | 会话、消息 |
| `emotion_echo_ai` | ai-svc | `AiRpc` | 情绪分析、语音转写、人脸检测 |
| `emotion_echo_assessment` | assessment-svc | `AssessmentRpc` | 量表、结果、心理健康评估、报告 |
| `emotion_echo_analytics` | analytics-svc | `AnalyticsRpc` | 用户行为事件 |

## 文件清单

```
deploy/db/
├── README.md                              ← 本文件
├── 01-create-schemas.sql                  ← 5 个 schema 创建（已完成）
├── 02-create-tables-in-schemas.sql        ← 15 张表分布到 schema（已完成）
├── 03-migrate-data.sql                    ← TODO: 从 public 迁移数据
└── 04-drop-old-public-tables.sql          ← TODO: 数据校验后删除旧表
```

## 各 svc 的连接串

```yaml
# user-svc
postgres://postgres:postgres@localhost:5432/emotion_echo?search_path=emotion_echo_user

# chat-svc
postgres://postgres:postgres@localhost:5432/emotion_echo?search_path=emotion_echo_chat

# ai-svc
postgres://postgres:postgres@localhost:5432/emotion_echo?search_path=emotion_echo_ai

# assessment-svc
postgres://postgres:postgres@localhost:5432/emotion_echo?search_path=emotion_echo_assessment

# analytics-svc
postgres://postgres:postgres@localhost:5432/emotion_echo?search_path=emotion_echo_analytics
```

## 跨域查询约定

**禁止**：跨 schema JOIN  
**允许**：

| 场景 | 通信方式 |
|------|---------|
| chat-svc 需要 user 信息 | RPC `UserRpc.GetUser(id)` |
| chat-svc 触发 AI 分析 | RPC `AiRpc.AnalyzeEmotion()` 或 Kafka 事件 |
| assessment-svc 触发 AI 推理 | RPC `AiRpc.DetectFace()` |
| analytics-svc 订阅事件 | Kafka consumer |
| 任何 svc 验证 token | RPC `UserRpc.ValidateToken(token)` |

## 迁移步骤（待执行）

### 1. 准备：双重写入期

旧 Gin 仍连 `public`，新 svc 连各自 schema。

### 2. 数据迁移（一次性）

```bash
# 03-migrate-data.sql 还没写，模板如下：
INSERT INTO emotion_echo_user.users SELECT * FROM public.users;
INSERT INTO emotion_echo_user.refresh_tokens SELECT * FROM public.refresh_tokens;
-- ... 以此类推
```

### 3. 双写验证（两周）

旧 Gin 写 `public`，新 svc 写各自 schema。
读路径优先走新 svc，对照两边数据是否一致。

### 4. 切换读路径

把所有读请求切到新 svc。如果数据稳定，旧 Gin 不再被读。

### 5. 删除旧 public 表

```bash
# 04-drop-old-public-tables.sql
DROP TABLE public.users;
DROP TABLE public.refresh_tokens;
-- ...
```

## 验证命令

```sql
-- 看各 schema 表数量
SELECT schemaname, COUNT(*)
FROM pg_tables
WHERE schemaname LIKE 'emotion_echo_%'
GROUP BY schemaname;

-- 看 user schema 的所有表
\dt emotion_echo_user.*

-- 跨域查询尝试（应该被审核拦截）
SELECT * FROM emotion_echo_chat.messages m
JOIN emotion_echo_user.users u ON m.user_id = u.id;  -- ❌ 不允许
```