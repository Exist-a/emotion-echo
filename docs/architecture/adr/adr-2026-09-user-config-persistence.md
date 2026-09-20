# ADR · 2026-09 · 用户个性化配置（config）服务端持久化

> **决策状态**：✅ **Accepted**（2026-09-19 用户决议 D-09；2026-09-20 E2E-12 实施落地）
> **登记位置**：[decisions.md](../decisions.md) D-09
> **实施证据**：PR #28；`emotion-echo-user-svc/migrations/u003_add_user_config.sql`；`proto/user.proto:112-118,225-239`

---

## 一、背景

设置页（`/chat/setting`）的**字号**与**主题**切换存在"操作看似成功、刷新即失效"的缺陷，登记为账本 `E2E-F-82`。

建档实测（E2E-12 plan §1）确认：**持久化在当时的架构上不可能实现**——四层契约各缺一环，任一缺失都会让 `PATCH /api/v1/users/me {"config":{...}}` 被静默丢弃：

| 层 | 缺失 | 证据（改前） |
|----|------|-------------|
| 数据库 | `emotion_echo_user.users` 无 config 列 | `deploy/db/02-create-tables-in-schemas.sql:7-18` 列清单止于 `birthday` |
| proto | `UpdateProfileRequest` 只有 nickname / avatar_url / gender | `proto/user.proto:112-118` |
| BFF 请求体 | `UpdateProfileReq` 只有 Nickname / Gender / Birthday / AvatarURL | `emotion-echo-web-bff/internal/downstream/user.go:36-41` |
| BFF 响应 | `toProfileVM` 硬编码 `Config: map[string]any{}` | `emotion-echo-web-bff/internal/handler/viewmodel.go:106` |

Go `json.Unmarshal` 忽略未知字段 ⇒ 前端拿到 200、`result.isOk` 为真、提示"成功"，而服务端什么都没存。

**关键事实**：前端**早已按服务端持久化写好**（`app/stores/user.ts:59-95` 的 `setFontSize`/`setTheme` 都是"先调 API 成功再更新本地"）。缺陷不在前端设计，而在契约缺失。

## 二、决策

**采用服务端持久化（方案 A）**：`users` 表新增 `config JSONB` 列，经 proto → BFF → user-svc 全链路透传。

| 层 | 改动 |
|----|------|
| DB | `emotion_echo_user.users.config JSONB`（`NULL` = 未设置，区别于 `{}` = 主动清空） |
| 迁移 | `emotion-echo-user-svc/migrations/u003_add_user_config.sql`（`ADD COLUMN IF NOT EXISTS` 保幂等）+ 权威 DDL `deploy/db/02-create-tables-in-schemas.sql` 同批同步 |
| proto | `UpdateProfileRequest.config`（optional string，JSON）+ `UserInfo.config`（optional string，JSON） |
| user-svc | `model.JSONMap`（`map[string]any` 的 GORM JSONB 别名，含 `Scan`/`Value`）→ types → repository → logic → grpcserver 全链路 |
| BFF | `downstream.UpdateProfileReq.Config` / `UserInfo.Config` + `toProfileVM` 映射（nil → 空 map）+ gRPC 客户端序列化 |

**传输形态**：proto 侧用 `optional string`（JSON 文本）而非 `google.protobuf.Struct`——与本仓库既有 proto 风格一致（无 `Struct` 依赖），且序列化边界显式可控。

### 备选方案（未选）

| 方案 | 做法 | 未选原因 |
|------|------|---------|
| B. 仅本地存储 | localStorage/cookie 镜像，删掉前端那段服务端写入 | 换浏览器/清缓存即丢；等于把「已实现却失效」改为「主动降级」，与页面既有设计意图相反，且不满足阶段标题的"持久化" |

## 三、后果

**正向**

- `E2E-F-82` 关闭：字号/主题跨刷新、跨会话保持。
- 前端既有实现无需改动设计，只是"补齐契约让已有代码真正生效"。
- `config JSONB` 为后续个性化设置（字体族、消息密度等）留出扩展位，无需再改 schema。

**代价与约束**

- **schema + proto 双变更**：新增字段属契约扩展，须走 ADR（本文件）+ 迁移幂等 + `test_migrations_contract.sh` 一致。
- **`NULL` vs `{}` 语义必须区分**：未设置用 `NULL`，不用 `{}` 默认值——否则"从未设置"与"主动清空"不可分（E2E-12 plan 风险 R4）。
- **值域不做服务端强校验**：`config` 是开放 JSONB，服务端只保证存取往返；未知值（如 `theme: "neon"`）由前端 `getUserConfig` 的 `|| 'light'` 兜底回落，**不得崩溃**（E2E-12 测试点 #12）。
- **proto 重生成须复现原方式**：`bash proto/gen.sh user.proto`（R 系列教训：改 proto 必须复现原生成方式，不得换生成器版本）。
