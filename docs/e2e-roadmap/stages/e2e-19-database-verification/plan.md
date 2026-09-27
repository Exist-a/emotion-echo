---
stage: e2e-19
title: 数据库层验证（连接池 / 迁移幂等重放 / 视图 / 软删除 / 备份→破坏→恢复）
type: verification
status: done
created: 2026-09-27
depends-on: [e2e-18]
blocks: [e2e-20]
gate: []
related-findings: [E2E-F-141, E2E-F-25, E2E-F-27]
---

# E2E-19 数据库层验证 — 详档

> **类型**：verification —— 让"数据库层行为是否符合预期"从**口头**变成**可机械证明**。
> **依据**：roadmap 第五批 E2E-19 行 + 账本 E2E-F-27（无备份/恢复/回滚机制）+ E2E-F-141（i002 checksum 漂移 → db-migrate Exited(1) 阻塞 dev mode 启动）。
> **方法论**：[RUNBOOK.md](../RUNBOOK.md)（状态机 / §4.1 证据有效性 / §7 收口契约 11 项 / §13 审计）+ [anti-patterns.md](../anti-patterns.md)。

---

## 1. 阶段目标

把数据库层从"dev 模式间歇性 FATAL + 无恢复手段"变成"行为可证 + 可恢复"：

1. **F-141 修复**：30 条 09-18 01:53 批量迁移记录的 checksum 来源于脏工作区（git 全历史 NO_GIT_BLOB），全部 UPDATE 到 HEAD 文件版，恢复 db-migrate 0 FATAL。
2. **连接池行为**：6 个 svc 的连接池配置正确，BFF→svc gRPC 连接复用，长任务不阻塞。
3. **视图可读性**：analytics_reader 视角能查所有 `*_v` 视图；PII（密保答案 hash 等）不外泄。
4. **软删除行为**：7 个含 `gorm.DeletedAt` 的 model 真删/软删语义统一，跨服务调用 SELECT 默认过滤 deleted_at IS NULL。
5. **迁移幂等重放**：db-migrate 容器重跑 0 FATAL；所有迁移 `IF NOT EXISTS` / `OR REPLACE` 守卫完整。
6. **分区裁剪**：项目实际**已用 PG 原生 RANGE 月度分区**（`emotion_echo_analytics.user_behavior_events` 按 `occurred_at` 月度分区，由 `a008_partition_user_behavior_events.sql` 建立；分区表 `ube_2026_01` ~ `ube_2026_06` + `ube_default` 兜底）。验证 EXPLAIN ANALYZE 在 WHERE 时段过滤时**只扫当月分区**（其他分区被 partition pruning 剪掉）。
7. **备份→破坏→恢复演练**：真 `pg_dump` 全库 → 真 `DROP` 一张核心表 → 真 `pg_restore` → DB 与应用两侧验证一致（不能模拟）。

---

## 2. 范围与边界

### 做

| 项 | 文件/端点 | 行为 |
|----|-----------|------|
| F-141 根因排查 | `deploy/db/migrate.sh:120-170` + DB `schema_migrations` | 三段取证闭环：DB 记录 vs git 全历史 vs HEAD 文件 |
| F-141 修复（迁移治理元数据） | DB `UPDATE emotion_echo_user.schema_migrations SET checksum = ...` | 30 条全部 UPDATE 到 HEAD 版；保留 `applied_at` 作审计追溯；commit message 注明"脏工作区→HEAD"溯源 |
| F-141 加固：`migrate.sh` 对脏工作区更精确诊断 | `deploy/db/migrate.sh:130-150` check_migration 函数 | 当 DB checksum ≠ HEAD 且 DB checksum 在 git 全历史 NO_GIT_BLOB 时，提示"记录来自未提交工作区，已用 HEAD 文件校验和替换"（需保留原 checksum 作审计列，可选） |
| 连接池配置盘查 | 6 svc 的 `internal/dbconnect/*.go` 或 main.go | 列出 max_open / max_idle / conn_max_lifetime 默认值；与 yaml override 比对 |
| 视图可读性 | `deploy/db/04-create-views.sql` + `a004_create_analytics_reader_role.sql` | `psql -U analytics_reader -c "SELECT * FROM daily_emotion_by_modality_v LIMIT 1"` 全视图跑通；测试视图不暴露 `user_security_answers.answer_hash` 等 PII |
| 软删除行为 | 7 个含 gorm.DeletedAt 的 model | 写一段 psql 验证：删除一行 → SELECT 查不到（默认作用域过滤）；`UNIQUE` 索引行为正常 |
| 迁移幂等重放 | `docker restart emotion-echo-db-migrate` | 31/31 SKIP；0 FATAL；execution_ms 全部回填 |
| 日期索引覆盖度 | `emotion_echo_analytics.user_behavior_events` / `_v` 视图 | `EXPLAIN ANALYZE` 验证 WHERE 时段过滤走索引（`a007_assessment_v_user_id_index.sql` / `a009_cursor_pagination_indexes.sql` 提供） |
| 备份→破坏→恢复 | DB 物理层 | `pg_dump -Fc` 全库 → `psql -c "DROP TABLE emotion_echo_chat.messages"` → `pg_restore` → 表恢复 + 行数一致 |
| 回归钉 | `emotion-echo-web/e2e/database-verification-smoke.spec.ts` | 4 用例（登录 + 发消息 + 看报表 + 看历史会话），chromium + mobile 双 project |
| 视觉证据 | 主链路截图 4 张 | 修后恢复运行的状态 |

### 不做（边界）

| 不做项 | 理由 |
|--------|------|
| 修改 schema/迁移文件内容 | E2E-19 是 verification，不动设计 |
| 引入 PG 分区 | 不在本阶段；roadmap 未排期；改范围外 |
| 连接池参数调优 | 阶段只盘点+报告当前值，调优归后续专项 |
| 软删除字段新增/移除 | 同上 |
| 备份脚本生产化（cron/压缩/异地） | 仅做 dev 模式演练，生产化归 E2E-29 横切安全 |
| 实时 CDC / point-in-time recovery | PG 高级特性，未排期 |

---

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| E2E-18 partial 收口（不阻塞 E2E-19 开工） | ✅ 2026-09-24 12/12 PASS（待 §13.3 第二方核对转 done） |
| 不在决策门阻塞列表（RUNBOOK §9） | ✅ §9 仅 D-04 且不阻塞任何阶段 |
| `e2e_stage_audit.py --all` 0 FAIL | ✅ 2026-09-24 PR #88 合并后：30 阶段 0 FAIL（待开工前再跑一次） |
| `plan.md` 存在且前置表满足 | ✅ 本文件 |
| `deploy/.env.local` 存在 | ✅（严禁删除/覆盖） |
| dev mode 锁 | ✅ `deploy/.devmode-session` 已写入（owner: lane-e） |

环境启动命令（**必须带 `--env-file .env.local` 与 `--profile dev`**，RUNBOOK §2.1）：

```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local --profile dev up -d
```

> ⚠️ **db-migrate FATAL 复现**：本次启动 `emotion-echo-db-migrate` 报 `Exited(1)`（F-141 实测复现），导致 `emotion-echo-{ai,analytics,chat,user,assessment,web-bff}` 全部 `Created`（depends_on 失败阻断）。**修复前置**：完成 §4 测试点 #1（F-141 根因排查 + 修复）后，应用服务才能启动。
>
> **临时缓解（已在 E2E-18 用过）**：`docker start <created-container>` 绕 depends_on 直接拉起；本阶段**不用**——F-141 是本阶段范围内修复，跑完 #1 后自然 healthy。

---

## 4. 测试点清单

判定标记：`[A]` 自动 · `[V]` 视觉 · `[M]` 需裁定。详见 [RUNBOOK.md](../RUNBOOK.md) §4。

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 1 | **F-141 根因排查**（DB `schema_migrations` 中 30 条 09-18 01:53 批量记录的 checksum 在 git 全历史 NO_GIT_BLOB ⇒ 来源于脏工作区；u003 09-19 单独跑则 DB checksum = HEAD 文件 checksum ⇒ 工作区已修正） | [A] | 全 git history `--objects` 遍历 + sha256sum 比对，输出 CSV 报告 | `/tmp/audit.csv` + 终端输出 | ⬜ |
| 2 | **F-141 修复 + 幂等重放**：30 条 09-18 01:53 批量 + c009 09-18 02:02 单独跑（**31 条全脏工作区**) → UPDATE 到 HEAD 版 → `docker restart emotion-echo-db-migrate` → 31/31 SKIP，0 FATAL，execution_ms 全部回填 | [A] | 修复前后 `docker logs emotion-echo-db-migrate` + `SELECT COUNT(*), SUM(execution_ms) FROM schema_migrations` | 日志片段 + DB 计数 | ⬜ |
| 3 | **F-141 加固**：`migrate.sh` check_migration 增加"DB checksum ∉ git 全历史"提示（针对"来自脏工作区"与"文件被改"区分诊断） | [A] | 写失败测试（RED：旧实现无该提示 → fail）→ 修实现（GREEN：新错误信息含"脏工作区"或"git 全历史 NO_GIT_BLOB"） | test_migrations_*.sh 输出 + commit 序列 | ⬜ |
| 4 | **连接池行为盘点**：6 svc 各 dbconnect 配置（max_open / max_idle / conn_max_lifetime）；yaml override 优先级；BFF→svc gRPC 连接复用；长任务不阻塞（实测 30s 长查询期间其他端点仍响应） | [A] | grep + yaml 读取 + 端到端：开一个 sleep 30s psql 连库，与此同时 curl `/api/v1/health` 不超时 | 命令输出 + curl 结果 | ⬜ |
| 5 | **视图可读性**：analytics_reader 视角能 SELECT 所有 `*_v` 视图（含 04-create-views.sql + a005_create_daily_emotion_by_modality_v）；视图不暴露 `user_security_answers.answer_hash` / `users.password_hash` | [A] | `psql -U analytics_reader -c "SELECT * FROM <view> LIMIT 1"` 全部跑通；`\d+ <view>` 检查列定义不涉敏感表 | psql 输出 | ⬜ |
| 6 | **软删除行为统一**：7 个含 gorm.DeletedAt 的 model（ai 域 5 + chat.conversations + user）+ `messages`（c009 新增）真删/软删语义；DELETE 后跨服务 SELECT 默认过滤；UNIQUE 约束行为（email 等）允许删除后重建 | [A] | 写一段 psql 操作：soft delete 一行 + SELECT 默认无 + 显式 `WHERE deleted_at IS NOT NULL` 能查到；恢复后行再次可见 | psql 输出 | ⬜ |
| 7 | **日期索引覆盖度**（分区裁剪的诚实替代）：核心报表视图（`daily_emotion_by_modality_v` / `user_behavior_events`）WHERE 时段过滤走索引（`EXPLAIN ANALYZE` 输出含 `Index Scan`，无 `Seq Scan` on large table） | [A] | `EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM ... WHERE created_at >= ...` | psql EXPLAIN 输出 | ⬜ |
| 8 | **备份→破坏→恢复演练（真破坏真恢复）**：`pg_dump -Fc emotion_echo > backup.dump`（全库含 schema + data）→ `psql -c "DROP TABLE emotion_echo_chat.messages"`（破坏一张核心表）→ `pg_restore --clean --if-exists backup.dump` → 表恢复 + 行数与备份时一致；应用端验证 `/api/v1/conversations/:id/messages` 可用 | [A] | 三段命令输出 + 前后行数对比 + 应用 curl 200 | 命令输出 + DB 计数 | ⬜ |
| 9 | **应用服务启动 + 跨服务验证**：F-141 修复后所有应用服务 healthy；Nacos `count:6`；BFF `/health` 200；端到端发消息 200 | [A] | `docker ps` + Nacos API + curl 端到端 | 命令输出 | ⬜ |
| 10 | **回归钉**：`emotion-echo-web/e2e/database-verification-smoke.spec.ts` 首跑绿（chromium + mobile 双 project，4 用例） | [A] | `pnpm playwright test e2e/database-verification-smoke.spec.ts` | playwright 输出 | ⬜ |
| 11 | **视觉证据**：F-141 修复后聊天主链路截图 2 张（chromium + mobile），确认无回归 | [V] | IAB + 截图并查看 | `screenshots/11a-chat-chromium.png` / `11b-chat-mobile.png` | ⬜ |
| 12 | **全量回归**：ai-svc+shared+chat+user+analytics+assessment+web-bff `go test ./...` + 前端 `pnpm vitest run` + 本 spec 复跑，全绿 | [A] | 7 段命令输出 | 测试输出 | ⬜ |

汇总：测试点 12 个，11 [A] + 1 [V]。

---

## 5. 验收标准（DoD）

- [ ] 全部 12 个测试点有结论（发现问题已分类：范围内修复 / 范围外记账）
- [ ] F-141 修复走 TDD（#3 增强 migrate.sh 必须先红后绿）
- [ ] #4-#8 全部有 psql/curl 实际输出（不是"已存在"证据）
- [ ] 备份→破坏→恢复**真做**（不是描述，必须含 DROP + pg_restore 实际执行）
- [ ] 回归钉 `database-verification-smoke.spec.ts` 存在 + 首跑绿 + 收口前复跑绿
- [ ] 账本 F-141 已翻状态（保留 audit 证据行）
- [ ] `e2e_stage_audit.py --all` 0 FAIL（收口时仍 0 FAIL）
- [ ] report.md 按 §10 模板 + roadmap 状态更新为 `done` + §2.5 自检三连
- [ ] 第二方核对（§13.3）——执行者不得自行宣布 done

---

## 6. 已知风险

| 风险 | 应对 |
|------|------|
| **UPDATE checksum 误改（破坏未来迁移治理）** | 仅 UPDATE 不变 applied_at + commit message 写明"2026-09-18 01:53 批量为脏工作区"；附 PDF/CSV 审计报告（/tmp/audit.csv）作为附件 |
| **备份→恢复期间破坏应用可用性** | 演练窗口与用户协调；演练期间所有应用容器保持运行（不停止）；破坏+恢复 < 60s，应用只是短暂 5xx |
| **db-migrate UPDATE 后某些 schema 对象不存在** | 先 `SELECT COUNT(*)` 验证 DB 已落库（每条迁移对应对象存在）再 UPDATE；如发现缺失单条不 UPDATE 单条以保持状态正确 |
| **migrate.sh 加固的兼容性**（check_migration 新加 git 全历史遍历，可能 100+ blobs 慢） | 用 `git rev-list --all --objects \| grep <path>` 单文件过滤；只在 `rc=2` 时才查（fast path 不动） |
| **ADR 门禁**（commit 命中"数据库"类关键词被 check_adr_gate.sh 拦） | 已读 decisions.md E2E-19 是 verification 不是 transformation；如门禁报红，先本地跑 `bash scripts/check_adr_gate.sh` 复现（memory：审计本地先跑秒级复现） |
| **migrate.sh 是 POSIX sh，POSIX ERE 限制**（memory「POSIX ERE `(?i)` 不生效」） | grep 加 `-i` flag，不依赖 `(?i)` |
| **.devmode-session 锁被人强占** | 开工时已写；按协议 §五收工删除 |
| **commit 推送触发 CI 重建 web/bff**（E2E-F-130 留账） | 本阶段不碰 web/bff 代码（仅可能改 migrate.sh），web/bff 重建非必要；若改了 yaml 重启即可 |
| **§13.3 第二方核对延迟 → 阶段不能判 done** | 收口先 partial；第二方核对通过后 done（与 E2E-18 一致） |

---

## 7. 产出物

- DB 修复脚本（手工 UPDATE + 注释审计）：不写文件，update 写在 commit message 末尾
- Go：可能 `deploy/db/migrate.sh` 改动（#3）+ 新增 `test_migrate_dirty_workspace.sh` 负向测试
- Playwright：`emotion-echo-web/e2e/database-verification-smoke.spec.ts`
- 截图：`stages/e2e-19-database-verification/screenshots/11a-chat-chromium.png` / `11b-chat-mobile.png`
- 备份证据：`backup-20260927.dump`（dev 模式演练产出，归档 `docs/evidence/e2e-19/` 或保留在 /tmp）
- 账本：`discovered-unresolved.md` E2E-F-141 状态 → ✅ 已解决（保留行）
- 执行记录：`stages/e2e-19-database-verification/report.md`

---

## 8. 调研依据（AGENTS.md §〇.6 — 计划期 2026-09-27）

- **已读代码（8）**：
  - `deploy/db/migrate.sh:1-220`（全，重点 check_migration/record_migration/run_tracked_sql_file）
  - `deploy/db/01-create-schemas.sql`（schema 定义：emotion_echo_{user,chat,ai,analytics,assessment}）
  - `deploy/db/02-create-tables-in-schemas.sql`（核心表）
  - `deploy/db/04-create-views.sql`（视图清单）
  - `emotion-echo-{ai,chat,user}-svc/internal/model/{face_emotion,conversation,user}.go`（gorm.DeletedAt 使用点）
  - 各 svc `internal/dbconnect/`（连接池配置点）
  - `emotion-echo-analytics-svc/internal/grpcserver/server.go`（listener 加锁验证 R-01 #15 已修）
- **已读测试（3）**：
  - `deploy/db/test_migrations_contract.sh`（迁移契约测试）
  - `deploy/db/test_migrations_no_service_order.sh`（顺序契约）
  - 各 svc `*_test.go`（已包含 race 测试）
- **已查 ADR/决策（3）**：
  - `docs/architecture/adr/adr-2026-09-llm-fusion-hardening.md`（决策 15 引用）
  - `decisions.md` D-27（Redis 保留决议，与本阶段无冲突）
  - `e2e decisions.md`（D-25/D-26/D-27 已登记，D-28 起留给本阶段如有需要）
- **已查账本**：F-141（i002 checksum 漂移 → db-migrate Exited(1)）为本阶段范围内；F-27（无备份/恢复）为本阶段必做；F-25（多实例下 in-memory 失效）属 E2E-20
- **运行时实测（计划期基线）**：
  - `docker compose --profile dev up -d` → `emotion-echo-db-migrate` Exited(1)（F-141 复现）
  - `SELECT * FROM schema_migrations WHERE version='i002_create_face_emotion_results.sql'` → `checksum=f5e05bb...` ≠ HEAD file `dad2992a...`
  - `git rev-list --all --objects | grep emotion-echo-ai-svc/migrations/i002_create_face_emotion_results.sql$` → 1 blob `dad2992a`（无 f5e05bb 变体）
  - `sha256sum emotion-echo-ai-svc/migrations/u003_add_user_config.sql` = DB record（09-19 单独跑的迁移**正确**，仅 09-18 批量 30 条脏）
  - 应用容器全部 `Created`（depends_on db-migrate 失败阻断）
- **git 溯源**：
  - 09-18 01:53:06~10 共 30 条迁移批量记录（chat-c001~c008、i001~i004、i005、i006~i008 2 条、i009、a001~a009、u001~u002、c009 单跑 09-18 02:02 共 31 条）→ git 全历史 NO_GIT_BLOB
  - 09-19 22:36 u003 单独跑 → git commit `8133339 feat(e2e-12)` HEAD 版
- **smoke**：AGENTS §2.4 六契约本阶段**不触发**（本阶段无事件发布/视图/报表链改动）
- **外部信息**：不适用（无外部依赖版本涉入）