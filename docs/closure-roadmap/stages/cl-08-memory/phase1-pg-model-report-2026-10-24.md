---
status: done
type: phase-report
stage: CL-08c
phase: 1
created: 2026-10-24
related:
  - plan.md（§3 Phase 1 / §2 T-1~T-3）
  - phase0-spike-report-2026-10-22.md（前置）
  - ../../../architecture/adr/adr-2026-10-agent-memory-graph.md（决策 45：PG=系统记录 / 决策 8：图可重建）
  - ../../discovered-unresolved.md（CL-F-72 / CL-F-73）
  - evidence/phase1-t1-red-contract-2026-10-24.txt
  - evidence/phase1-t1-migrate-twice-2026-10-24.txt
  - evidence/phase1-integration-pg-2026-10-24.txt
  - evidence/phase1-go-test-all-2026-10-24.txt
---

# CL-08c Phase 1 · PG 数据模型报告（2026-10-24）

> **落地载体**：阶段 PR [#273](https://github.com/Exist-a/emotion-echo/pull/273)（draft → 收口时转 ready）。
> **登记**：`parallel-contract.md §三`（CL-08c Phase 1 行，随登记 PR #274 → main `651b4a9`）。
> **纪律**：全程未取 dev 栈锁（dev 栈 2026-10-24 实测仅剩 3 容器，未动它）；迁移验证用一次性 postgres 容器。

---

## §1 范围校正（相对 plan.md §3 Phase 1 原文）

plan.md §3 Phase 1 所列四表 `memory_episode / memory_entity / memory_fact / memory_extract_run`，
**后两张（以及额外的 `memory_note`）已由 CL-08a 以 `i011_create_memory_facts.sql` 落地**
（含 `valid_from/valid_to/source_kind/source_id/user_id/status` 全套双时间轴列与幂等唯一索引）。

⇒ 本轮只补两块（已在 roadmap.md 2026-10-24 开工注里声明，登记 PR #274）：

| 新增 | 语义（ADR 决策 45 / 决策 8） |
|---|---|
| `memory_episode` | 情节 = 对来源（message/note）的一次**可重建存档**；图丢了从它重放重建（"从 PG 重建图"的原料） |
| `memory_entity` | 实体注册表 = 图节点在 PG 的**镜像**；同用户内同名唯一（消解锚点）；管理页读取 / 图重建依据 |

## §2 交付物

| 文件 | 内容 |
|---|---|
| `emotion-echo-ai-svc/migrations/i012_create_memory_episode_entity.sql` | 两表 + 5 索引；`IF NOT EXISTS` 幂等；CHECK 约束锁枚举（status: active/deleted；source_kind: message/note）；归属 `emotion_echo_ai` |
| `internal/model/memory_episode.go` | `MemoryEpisode` / `MemoryEntity`（GORM）+ `MemoryNodeStatusActive/Deleted` 常量 |
| `internal/repository/memory_episode_repository.go` | `MemoryGraphRepo` 接口 + InMemory 替身 + Postgres 实现（`ON CONFLICT` 幂等、实体摘要刷新、软删复活） |
| `migrations/i012_..._test.go` | 迁移字面量契约测试（先剥离 SQL 注释再断言，i010/i011 同款纪律） |
| `internal/repository/memory_episode_repository_test.go` | 7 组语义契约（往返 / 幂等不覆盖 / 非法输入 / 用户隔离 / 摘要刷新 / 软删复活 / 跨用户独立行） |
| `integration_test/memory_episode_entity_repo_integration_test.go` | 真库 4 测试（含 **§契约 5**：CHECK 枚举列写错值必须被真库拦下 + 合法值对照可写） |

**关键设计裁决（本轮内定，依据 ADR 既有裁决推导）**：
1. **episode 幂等键 = `(source_kind, source_id)`** —— 与 `memory_extract_run` 复合主键同键，Phase 2 抽取 worker 直接复用（plan.md §7.1 T9 已定"幂等键 = message_id"）。
2. **episode/entity 无 `superseded` 状态** —— 情节是对"说过什么"的不可变存档；实体的"最新"语义落在 summary 刷新。生命周期只有 active/deleted。
3. **实体唯一锚点 = `(user_id, name)` 全量唯一 + 软删复活** —— 用户删掉实体后再次说起新证据 ⇒ 复活该行（`status→active`、`deleted_at→NULL`），不新建（保持唯一锚点）。删除入口**不在本轮**仓储里（删除闭环 = PG+图+Redis 三联动，归 Phase 5 / CL-08d 一起做，防半套删除先落地）。
4. **双时间轴**：episode 用 `valid_at`（事件发生）+ `created_at`（摄入）承载 Graphiti 双时间轴，与 fact 的 `valid_from/valid_to` 同族不同列（情节不可变，无需关区间）。

## §3 测试点结论（本阶段 21 点中与本 Phase 相关者）

| # | 测试点 | 判定 | 证据 |
|---|---|---|---|
| T-1 | 迁移**幂等可重跑** + checksum 追踪 | **PASS**（事实表部分由 CL-08a 取得；本轮补 i012） | `phase1-t1-migrate-twice-2026-10-24.txt`：一次性 postgres:15-alpine 容器，migrate.sh 第 1 次 `OK i012（2000ms）` EXIT=0，第 2 次**全 SKIP**（checksum 一致）EXIT=0；`schema_migrations` 含 `i012|a468c874…`；两表 + 5 索引实查齐全 |
| T-2 | 系统记录**写入读回一致** | **PASS**（本轮覆盖 episode/entity） | `phase1-integration-pg-2026-10-24.txt`：真 PG 往返 4/4 PASS（含字段逐一比对）；fact 表部分沿用 CL-08a 证据 |
| T-3 | **双时间轴语义**（旧行关区间不删除） | **PASS（沿用 CL-08a）** —— fact 侧 i011 + `ApplyFacts` supersede 已有集成测试；episode 侧语义为 `valid_at`+`created_at`（不可变存档，无需关区间），由契约测试钉住 | CL-08a `report.md` §T-3 + 本轮 `TestPostgresMemoryGraphRepo_UpsertEpisode_RoundTripAndIdempotent` |

**§契约 5（schema 与写入端一致性）**：`TestPostgresMemoryGraphRepo_CheckConstraints_RejectBadEnumValues` —— `status`/`source_kind` 写非法值被真库 CHECK 拦下（报错指名约束）+ 合法值对照可写。

## §4 全量验证

- `go test ./... -count=1`（ai-svc）：**17/17 包 ok**，EXIT=0（`phase1-go-test-all-2026-10-24.txt`）
- `go vet ./...`：干净
- TDD：RED（`phase1-t1-red-contract-2026-10-24.txt`：契约测试先跑红）→ GREEN（实现 + 测试同 commit）

## §5 范围外发现（只记账，本轮不修）

| 账号 | 摘要 | 归属 |
|---|---|---|
| `CL-F-72` | Docker Desktop 29.x 容器化引擎下 testcontainers-go v0.44 ryuk reaper 起不来 ⇒ integration test 全红；临时解法 `TESTCONTAINERS_RYUK_DISABLED=true`（已验证 4/4 绿） | CL-09 |
| `CL-F-73` | 全新空库 migrate.sh 首跑死在 chat/c005（GRANT `analytics_reader` 早于 analytics/a004 建角色）；仅影响"从零 bootstrap"路径，dev/prod 现行路径不触发 | CL-12 |

另：2026-10-24 实测 dev 栈仅剩 3 容器（ai-svc crash-loop 中，llm-service/xtts healthy）——属上一会话遗留的运行态，与本轮无关，未动它；下一位需要 dev 栈者需先整栈重建。

## §6 未做 / 下一步（如实声明）

- 未做 dev 栈上的 db-migrate 实跑（dev 栈 postgres 容器已不在；i012 将随下次 compose up 自动应用——迁移幂等已在一次性容器按同机制验证）。
- 仓储尚未接线到 ai-svc 的装配面（`svc`/`main.go`）：那是 **Phase 2 抽取 worker** 的事，本轮刻意不碰 `main.go`（spine 文件，无并发者也没必要提前占）。
- T-4（消费幂等端到端）/ T-5（LLM 挂不阻塞）属 Phase 2；T-13~T-17 属 Phase 4/5。
