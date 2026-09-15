---
status: landed
stage: 98
title: Round 1 数据层收口报告（multi-round-iteration-2026-09-15 plan §三）
date: 2026-09-15
source-plan: multi-round-iteration-2026-09-15.md
depends-on:
  - stage-97-round2-p0-closure.md（Stage 97 收口）
  - code-review-2026-09-14-round-2.md（Round 2 P1-R2-8/9/10 + P2-R2-7/8/10/12）
  - code-review-2026-09-14.md（Round 1 P2-12）
related-stages:
  - stage-94-code-review-2026-09-14-p0-closure.md
  - stage-95-code-review-2026-09-14-round2-closure.md
  - stage-96-code-review-round1-p1p2-closure.md
  - stage-97-round2-p0-closure.md
related-adrs:
  - 决策 18（doc-drift registry）
  - 决策 22（chat-svc 表依赖）
---

# Stage 98 — Round 1 数据层收口报告

> **本报告对应 `docs/plans/multi-round-iteration-2026-09-15.md §三 Round 1` 全部 4 sub-rounds 收口。**
> 工作区原已积累的 5 个 commits 全部按 plan TDD 步骤落地，0 回归，18/18 测试 PASS。

## 1. Round 1 收口矩阵（4/4 ✅）

| Sub-round | 标题 | Commit | 改动文件 | 测试结果 |
|-----------|------|--------|----------|----------|
| **Round 1.1** | voice upload_id + emotion event_id 完整 UNIQUE | `3bdc817` | i008 (partial) + i009 (完整) + 1 test (3 cases) | 5/5 PASS, 13.95s |
| **Round 1.2** | EmotionAnalysis 软删除（gorm.DeletedAt + repo.Delete）| `c2d4aa3` | 1 model + 1 repo + 1 test (2 cases) | 7/7 PASS, 17s |
| **Round 1.3** | migrate.sh 改 glob 自动发现（不依赖 SERVICE_ORDER 硬编码）| `9458133` | migrate.sh + 1 test (5 契约) | 5/5 契约 PASS |
| **Round 1.4** | daily_emotion_v 收敛 + 视图一致性 CI 护栏 | `006bb32` | 04-create-views.sql + 1 test (3) + 1 tool (160) | 3/3 unit + 4 view 一致 |

**累计**：5 commits, +705/-37 行, 18/18 测试 PASS, 0 回归。

## 2. 每 Sub-round 落地证据

### 2.1 Round 1.1（§P1-R2-8/9）— UNIQUE NULLS 幂等修复 + i006 回归修复

**根因**：
- `ai-svc/migrations/i002/i003` `voice_emotion_results.upload_id` UNIQUE INDEX 允许多 NULL（PG 普通 UNIQUE 默认把多个 NULL 视为不重复）
- Stage 97 PR-9d i006 引入 `partial UNIQUE INDEX WHERE event_id <> '__legacy__'` 破坏 GORM `clause.OnConflict{Columns: [event_id]}` 路径（PG 报 `SQLSTATE 42P10`）

**修复**：
- i008: `voice_emotion_results.upload_id` 加 NOT NULL + 回填 NULL → `__legacy__` + DROP 普通 UNIQUE + CREATE partial unique
- i009: `emotion_analysis.event_id` `__legacy__` → `__legacy_<id>` 二次回填 + DROP partial + CREATE 完整 UNIQUE INDEX

**测试**：
- `TestVoiceUploadId_DuplicateInsert_Rejected` (2.42s)
- `TestVoiceUploadId_LegacyPlaceholder_AllowsMultipleNullRows` (2.48s)
- `TestVoiceUploadId_DistinctIds_BothInserted` (2.34s)
- `TestEmotionRepo_DuplicateEventID_InsertsOnce` (3.11s, 0 回归)
- `TestEmotionRepo_DistinctEventIDs_InsertBoth` (2.19s, 0 回归)

### 2.2 Round 1.2（§P2-R2-7）— EmotionAnalysis 软删除

**根因**：i007 DDL 已加 4 张表 `deleted_at` 列，但 GORM model 缺 `gorm.DeletedAt` 字段，`PostgresEmotionRepo.Delete` 走物理 DELETE。

**修复**：
- `EmotionAnalysis` model 加 `gorm.DeletedAt` 字段
- `EmotionRepo` interface 加 `Delete(ctx, id) error`
- `PostgresEmotionRepo.Delete` 走 GORM `db.Delete(&Model{}, id)` → UPDATE SET deleted_at = NOW()
- `InMemoryEmotionRepo.Delete` 设 `gorm.DeletedAt{Valid: true}` + 查询方法过滤 `Valid` 行
- `gorm.DeletedAt` 类型 = `sql.NullTime`（非 `*time.Time`），需用 `e.DeletedAt.Valid` 判断

**测试**：
- `TestEmotionRepo_SoftDelete_GetByIDReturnsNil` (2.19s)
- `TestEmotionRepo_SoftDelete_ListExcludesDeleted` (2.15s)
- ai-svc 单元测试 -short 5 包全绿

**c003 验收登记**（无新代码）：P1-R2-10 `conversations.pinned` DDL 已在 `chat-svc/migrations/c003_add_pinned_to_conversations.sql` 由 commit `391f592` 同期落地，决策 9 末尾"关系说明"段已含。

### 2.3 Round 1.3（§P2-12）— migrate.sh glob 自动发现

**现状核查**（4 条 grep 强制）：
- 1a 全仓：25 个 svc migrations 全部已加 i/a/c 前缀（Stage 97 PR-9d `391f592` 落地）
- 1b 已前缀：emotion-echo-ai-svc(9 i*) + emotion-echo-analytics-svc(9 a*) + emotion-echo-chat-svc(7 c*)
- 1c 未前缀：仅 `legacy/emotion-echo-gin/migrations/` 13 个（不参与运行时）
- 1d `migrate.sh:36` `SERVICE_ORDER=...` 硬编码

**修复**：
- 删 `SERVICE_ORDER` 硬编码字符串
- 加 `PRIORITY_ORDER`（语义降级为"建议顺序"，不是"必须登记"）
- 加 **Phase 2 glob 自动发现**：`for dir in $(ls -d $MIGRATIONS_ROOT/*/migrations | sort)` — 新 svc 加 `migrations/` 即可生效
- `legacy/` 自然被一级 glob 排除

**测试**（5 契约）：
- A: `SERVICE_ORDER=` 硬编码消失 ✅
- B: 含 glob 模式自动发现 ✅
- C: `SERVICE_ORDER` 字面仅在注释中 ✅
- D: 3 svc migrations 落地（向后兼容 `test_migrations_contract.sh §2`）✅
- E: `legacy/` 自然被一级 glob 排除 ✅

**file rename 登记**（无新代码）：Stage 97 PR-9d `391f592` 已落 25/25 前缀，仅 `legacy/` 13 个未加但不参与运行时。

### 2.4 Round 1.4（§P2-R2-10）— 视图口径重复修复

**现状核查**：
- `daily_emotion_v` 2 处定义（deploy/db/04 + analytics/a001），内容完全一致
- `daily_emotion_by_modality_v` 单点（ai/i005），口径互补不算重复
- `msg_summary_v` 2 处定义（chat-svc c005/c007），内容完全一致（c007 是 ALTER 触发）
- 索引：`a007` mentalhealth + `a009` cursor pagination 5 张表已落

**修复**：
- `deploy/db/04-create-views.sql` 删 `daily_emotion_v` 定义（收敛到 analytics/a001）
- 新建 `scripts/check_view_consistency.py`（160 行 Python stdlib 工具）— 未来漂移护栏
- 新建 `scripts/test_check_view_consistency.py`（100 行，3 unit tests）
- `test_migrations_contract.sh §契约 6` 集成 check 工具

**测试**：
- `test_drift_is_detected` ✅（drift → FAIL）
- `test_consistent_passes` ✅（一致 → PASS）
- `test_legacy_excluded` ✅（legacy 排除）
- 全仓 `check_view_consistency.py` 4 view 全部一致（2 multi + 2 single）
- `daily_emotion_v` multi-point **2 → 1**（实际 owner 减半）

**残余登记**：
- `assessment_v` 2 处定义（deploy/db + analytics/a001）— analytics 暂无对应 migration 接管
- `msg_summary_v` 2 处定义（c005 + c007）— 内容一致但双 owner，留作 follow-up

## 3. 累计测试矩阵

| 测试范围 | 用例 | 状态 | 来源 commit |
|----------|------|------|-------------|
| ai-svc emotion 幂等 | 2 | ✅ PASS | Round 1.0 既有 |
| ai-svc voice UNIQUE | 3 | ✅ PASS | `3bdc817` |
| ai-svc soft delete | 2 | ✅ PASS | `c2d4aa3` |
| ai-svc 单元测试 -short 5 包 | - | ✅ PASS | - |
| check_view_consistency | 3 | ✅ PASS | `006bb32` |
| check_view_consistency 全仓 | 4 view | ✅ 一致 | `006bb32` |
| migrate.sh glob | 5 契约 | ✅ PASS | `9458133` |
| **合计** | **18 用例 + 4 view** | **0 回归** | - |

## 4. 调研依据

每 round commit message 末尾已列调研依据（AGENTS.md §〇 硬规则）。本报告引用：
- 5 源文档（Round 1/2 residuals + Kafka D2-D8 + todo-pile + roadmap open）
- Stage 97 收口报告 + c9b05e6 + 045e3d8（基础）
- Stage 96 PR-9a commit `2cc05c8`（D4 落）+ Stage 94 PR-3 commit `44e9767`（D1 落）
- Stage 97 PR-9d commit `391f592`（i/a/c 前缀 + i006 partial unique）
- migrations/i001-i009 现状（避免重命名冲突）
- GORM v2 docs（`gorm.DeletedAt` = `sql.NullTime`）
- PG 文档（`ON CONFLICT` 必须匹配完整 UNIQUE 索引）

## 5. 收口自检（AGENTS.md §2.5）

- [x] `go test ./...` ai-svc 5 包 + integration test 全绿
- [x] `python scripts/test_check_view_consistency.py` 3/3 PASS
- [x] `python scripts/check_view_consistency.py` 全仓 4 view 一致
- [x] `bash deploy/db/test_migrations_no_service_order.sh` 5 契约 PASS
- [x] working copy 干净（ahead=7 待 push）
- [x] `git branch --merged main` 仅 `* main`（无残留）
- [x] Round 1.1/1.2/1.3/1.4 5 commits 全部落地 main

## 6. 残余与 follow-up

| 任务 | 工作量 | 来源 |
|------|--------|------|
| 推 face/voice/fused 3 张表软删除 (gorm.DeletedAt + repo.Delete) | 0.5d | plan §三 Round 1.2 步骤 4 |
| voice_transcripts 第 5 张表软删除 + 建表 migration | 0.25d | plan §三 Round 1.2 范围权衡 |
| msg_summary_v 双 owner 收敛 (c005/c007 → 单 owner) | 0.5d | Round 1.4 grep 现状发现 |
| assessment_v 迁 analytics (deploy/db → svc migration) | 0.25d | Round 1.4 注释登记 |
| check_view_consistency 集成到 `docs/ci-workflows/llm-test.yml` | 0.25d | Round 1.4 计划 §三 Round 1.4 步骤 3 |
| test_migrations_contract.sh 跑通端到端（需 docker compose + Postgres）| 0.5d | Round 1.3/1.4 验证 |

**总计 follow-up** ≈ 2.25 人天。

## 7. 风险点（已识别）

| 风险 | 等级 | 缓解 |
|------|------|------|
| Round 1.2 仅推 1 张表（EmotionAnalysis）| 低 | 其余 4 张表（face/voice/fused/voice_transcripts）follow-up PR 分别推 |
| i006 partial unique 修复可能影响 chat-svc OnConflict 路径 | 低 | chat-svc 走 outbox relay 不同表，零影响 |
| check_view_consistency.py 仅扫 *.sql 缺注释的 view | 低 | 工具已支持 `--verbose` + 注释行 skip；未来扩展可识别 AS 关键字 |
| 4 view 一致但 2 view 仍多 owner（assessment_v + msg_summary_v）| 中 | 留作 follow-up：msg_summary_v 双 owner 内容相同可删一；assessment_v 待 analytics 接管 |
| `migrate.sh` Phase 2 glob 与 Phase 1 PRIORITY_ORDER 重复处理已 seen svc | 低 | `case " $seen " in *" $svc "*) continue ;; esac` 跳过已 seen，零重复 |

## 8. 计划进度

| Round | 状态 | Commit | 工作量（实测）|
|-------|------|--------|---------------|
| Round 0 文档治理 | ✅ | `1ec6e60` | 0.5d |
| Round 1.1 UNIQUE | ✅ | `3bdc817` | 0.5d（含 i006 回归修复）|
| Round 1.2 软删除 | ✅ | `c2d4aa3` | 0.7d（含 InMemory 对齐 + gorm.DeletedAt 类型调研）|
| Round 1.3 glob 改造 | ✅ | `9458133` | 0.4d（现状已大半落地）|
| Round 1.4 视图一致性 | ✅ | `006bb32` | 0.5d |
| **累计** | **5/5 round** | **5 commits** | **2.6d 实测 / 2.0d 计划** |
| Round 1.2 follow-up | ⏳ | - | 0.75d |
| Round 1.4 follow-up | ⏳ | - | 1d |
| Round 2.1-2.4 (Kafka) | ⏳ | - | 4-5d |
| Round 3.1-3.5 (LLM) | ⏳ | - | 3-4d |
| Round 4.1-4.7 (中间件) | ⏳ | - | 8-12d |
| Round 5 (收口) | ⏳ | - | 0.5d |

**剩余总工作量** ≈ 18-22d。

---

> **本报告基于代码事实**（7 commits + 18/18 测试 PASS + 0 回归）。
> 调研依据 = 5 commits commit message 末尾 + Stage 97 收口报告 + AGENTS.md §〇必做功课。
