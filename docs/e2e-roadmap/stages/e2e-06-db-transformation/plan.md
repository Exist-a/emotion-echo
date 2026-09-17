---
stage: e2e-06
title: 数据库改造
type: transformation
status: pending
created: 2026-09-17
depends-on: [e2e-03]
blocks: [e2e-07, e2e-09, e2e-19]
related-findings: [E2E-F-09, E2E-F-19]
---

# E2E-06 🔧 数据库改造

## 1. 阶段目标

一次把数据库该动的地方动完：**删死字段、加密保字段（供 D-01）、建迁移版本表、统一软删除、修文档**。本阶段是 E2E-07（找回密码）的硬前置——密保问题字段必须先就位。

## 2. 范围与边界

### 做 (a)：删除死字段

核实方法：对 `emotion_echo_user.users` 每列 grep 全部非测试 Go 代码的读写点（结论已核）。

| 字段 | 读取点 | 写入点 | 结论 |
|------|--------|--------|------|
| `email VARCHAR(128)` | ❌ 无（仅 `user-svc/internal/model/user.go:15` tag） | ❌ 无 | **删** |
| `phone VARCHAR(20)` | ✅ 仅 API 响应回显（`getmelogic.go:67`、`getuserbyidlogic.go:42`、`user_server.go:60`、`authlogic.go:113,136`） | ❌ **无**→恒 NULL | **删**（连带清理上述 4 处回显与 proto/DTO 字段） |
| `status SMALLINT` | ❌ 无 | ❌ 无（仅 `model/user.go:21` `default:1`） | **待确认**：若确认无软禁用规划则删 |

> 注意：删 `phone` 会连带影响 proto/DTO 与 BFF 响应体，需一并清理（否则留下悬空字段，是新的文档-代码漂移源）。

### 做 (b)：新增密保问题字段（供 D-01=C）

- 方案：`users` 表加 `security_question VARCHAR(128)` + `security_answer_hash VARCHAR(255)`，答案用与 `password_hash` 同套 bcrypt；**或**独立 `user_security_answers` 表（支持多问题，扩展性更好）
- 需同步改：`user-svc` model / repository / logic、BFF 端点、proto 契约

### 做 (c)：迁移治理

- 新增 `schema_migrations` 版本表（记录已应用的迁移名 + 校验和 + 应用时间）
- 现模型是"幂等 + 每次 compose up 重放"（`deploy/db/migrate.sh`），**无法回答"某环境跑过哪些迁移"**

### 做 (d)：软删除统一

- chat 的 `deleteconversation` 是**物理删**，与 users/ai 域的 `deleted_at` 软删除不一致
- 统一为软删除（需评估对现有查询的影响：列表/详情/统计是否都要加 `deleted_at IS NULL`）

### 做 (e)：文档修正

- `deploy/db/README.md` 仍列**不存在**的 `03-migrate-data.sql`

### 不做（边界）

- 连接池调优（归 E2E-19）
- 备份/恢复演练（归 E2E-19）
- 分区表改造（现有 a008 已按月分区，归 E2E-19 验证）
- 数据迁移/清洗（无历史数据负担）

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| E2E-03（CI，schema 变更要有回归门槛） | ⏳ |
| dev 模式 postgres 可连 | ✅ |
| 确认 `status` 字段的去留（需用户/代码考古确认） | ⚠️ 待确认 |
| 确认 `phone` 删除的连带影响面（proto/BFF/前端） | 需先 grep |

## 4. 测试点清单

| # | 测试点 | 验证方式 | 证据 | 结果 |
|---|--------|---------|------|------|
| 1 | 死字段删除后无残留引用 | 全仓 grep `\bphone\b` / `\bemail\b`（非测试），断言无业务引用 | grep 输出 | ⬜ |
| 2 | 迁移可从空库跑通 | 删 volume → `docker compose up` → 断言 schema 完整 | 输出 | ⬜ |
| 3 | 迁移可从既有库幂等重放 | 不删 volume 再跑一次，断言无报错、无重复数据 | 输出 | ⬜ |
| 4 | `schema_migrations` 表正确记录 | `psql` 查询该表，断言列出全部迁移 | 查询输出 | ⬜ |
| 5 | 密保字段可写可读 | 集成测试：写入问题+哈希 → 读回 → bcrypt 校验通过 | 测试输出 | ⬜ |
| 6 | 软删除统一后行为正确 | 删会话 → 断言 DB 行仍在且 `deleted_at` 有值；列表/详情/统计均不返回该会话 | 断言 | ⬜ |
| 7 | 视图不受影响 | 跑 `scripts/check_view_consistency.py`，断言绿 | 输出 | ⬜ |
| 8 | 契约测试全绿 | `deploy/db/test_migrations_contract.sh` + `test_migrations_no_service_order.sh` | 输出 | ⬜ |
| 9 | 服务启动无回归 | compose up 后 6 服务 healthy | `docker ps` | ⬜ |
| 10 | 文档已更正 | `deploy/db/README.md` 无悬空文件引用 | diff | ⬜ |

## 5. 验收标准（DoD）

- [ ] 10 个测试点通过
- [ ] 死字段删除后 proto/BFF/前端/DTO 无悬空引用
- [ ] `schema_migrations` 生效且被 CI 覆盖
- [ ] 密保字段就位（解锁 E2E-07）
- [ ] 按 AGENTS.md §2.4 契约要求：schema 变更补 integration test 断言写入值合法

## 6. 已知风险

| 风险 | 应对 |
|------|------|
| 删 `phone` 牵连 proto 生成代码（改动面可能超预期） | 先 grep 出完整影响面清单，再决定是否拆两步（先停写、后删列） |
| `status` 字段去留不明，误删可能破坏未实现的规划 | 标"待确认"；无明确依据时**保留并加注释**而非删 |
| 软删除统一会改变 chat 现有查询语义，可能引入"会话还在但要读过滤"类 bug | 逐查询点排查；补集成测试覆盖列表/详情/统计 |
| 迁移改动在既有 dev 库上重放的幂等性 | 测试点 2/3 双向验证（空库 + 既有库） |
| `05-drop-user-oauth.sql` 是**单向破坏性**迁移，无回滚 | 加 `schema_migrations` 后同步评估是否补 down 脚本（若补，归 E2E-19） |

## 7. 产出物

- 新迁移文件：`chat-svc|ai-svc|analytics-svc/migrations/*`（删列/加列/版本表）
- `deploy/db/README.md` 更正
- 集成测试：密保字段读写 + 软删除行为
- 执行记录：`stages/e2e-06-db-transformation/report.md`
