# Stage 30-A SQL 落地报告（关闭 defer 项 1–5）

> **生成时间**: 2026-08-30
> **对应 commit**: `f1b64d4..e5dd4b6`（10 个 commits，8 个 TDD cycle + 2 个收尾）
> **对应规划**: `docs/stage-30-A-analytics-business.md` §九 defer + `docs/stage-30-A-landing.md` §六

---

## 一、目标回顾

Stage 30-A 完成了 9 个业务端点的 handler/logic/路由，但 **Postgres 仓库层 8 个方法全部是占位**（返 `"Round 5 实现未落地"` error），TriggerQueue worker body 是 no-op（只打日志）。本次把它们全部落地为**真实 SQL + 真实 worker**，并用 testcontainers 真 Postgres 集成测试验证。

按 `stage-30-A-landing.md` §六 的 defer 清单，本次关闭：

| # | defer 项 | 状态 |
|---|---------|------|
| 1 | PostgresReportRepo.GetDailyReport 真 SQL | ✅ 跨 schema 标量子查询聚合 |
| 2 | PostgresReportRepo.GetTrendReport 真 SQL | ✅ SQL DATE 域切桶 + Go 连续桶补零 |
| 3 | PostgresMentalHealthRepo 3 个真 SQL 方法 | ✅ assessment / history(cursor) / trend |
| 4 | PostgresEventRepo 3 个 query 方法真 SQL | ✅ day-night / depth / frequency |
| 5 | TriggerQueue worker body 真实 logic | ✅ MentalHealthRunner + migration 005 job 状态机 |

---

## 二、探查发现并修复的前置问题

1. **migration 001 VIEW 与生产 schema 不一致**（`ec71987`）：
   - `msg_summary_v` 引用 `send_time` —— 该列只存在于 legacy gin 单体；新 chat-svc 微服务 `messages` 用 `created_at`。→ VIEW 改为 `created_at AS send_time`（保持 VIEW 外部契约不变）。
   - `assessment_v` 引用 `risk_level` —— `mental_health_assessments` 表无此列（risk_level 在 `survey_results` 上）。→ VIEW 去掉该列，Go 侧 `riskFromScore(overall_score)` 推导。
   - 该不一致会导致 migration 001 在真库上创建 VIEW 失败；Stage 30-A 声称"文件结构正确，实际执行待 CI"时未被捕获。

2. **`PostgresMentalHealthRepo` 是 `struct{}`**（无 db 字段，main.go 也未传 db）→ 补 `*gorm.DB` 字段 + main.go 接线（`9fdcbf3` / `0b6ef5a`）。

3. **main.go 缺少规划中声明的启动时 `REFRESH MATERIALIZED VIEW mv_daily_emotion`** → 已补（非致命，失败不影响实时 VIEW 查询，`0b6ef5a`）。

---

## 三、实现明细

### 3.1 数据层（8 个 Postgres 方法）

| 方法 | SQL 策略 |
|------|---------|
| `ReportRepo.GetDailyReport` | 4 个跨 schema 标量子查询（msg_summary_v / user_behavior_events / assessment_v / daily_emotion_v）+ emotion counts GROUP BY；日期以 `YYYY-MM-DD` 字符串传 `$n::date`，**不依赖数据库会话时区** |
| `ReportRepo.GetTrendReport` | SQL 内 `start + ((day-start)/bucket_days)*bucket_days` 在 **DATE 域整数运算**切桶（weekly=7 / monthly=30 / yearly=365），Go `buildTrendPoints` 补 [start,end] 连续桶（空桶 Count=0、主导情绪取桶内 max count、avg 按 count 加权） |
| `MentalHealthRepo.GetLatestAssessment` | 窗口参数（daily=24h / weekly=7d / comprehensive=无）传 `$2::timestamptz`，`ORDER BY created_at DESC LIMIT 1`；无结果 → `(nil, nil)`；RiskLevel 由 `riskFromScore` 推导；`dimensions` JSONB → `[]DimensionScore`（宽松解析对象/数值两种形状，按 Name 排序） |
| `MentalHealthRepo.ListAssessmentHistory` | id DESC **keyset 分页**（`id < $3`，`LIMIT $4+1` 判 hasMore，nextCursor=本页末条 id）；assessment_type 可选过滤 |
| `MentalHealthRepo.GetTrendData` | 按日 `AVG(overall_score)` 聚合 → 复用 `buildTrendPoints` 切 weekly/monthly 连续桶；AvgSentiment=桶均分、PrimaryEmotion=桶均分风险等级 |
| `EventRepo.GetDayNightPattern` | `EXTRACT(HOUR FROM occurred_at)` GROUP BY，窗口 `[start, end+1day)`；稀疏 map，logic 层补 24 桶 |
| `EventRepo.GetInteractionDepth` | CTE `windowed` + `session_agg`：total=事件数、convs=DISTINCT session_id、avg 保留 2 位小数（除零保护）、longest=按 session 首末事件跨度最大值（ms） |
| `EventRepo.GetFrequencyTrend` | `occurred_at::date` GROUP BY ORDER BY 升序 |

### 3.2 TriggerQueue worker（`0b6ef5a`）

- **migration `005_create_assessment_jobs.sql`**（新）：`emotion_echo_analytics.assessment_jobs`（analytics 自有 schema，不违反跨 schema 只读边界），记录任务状态机 `running → done(result JSONB) | failed(error)`。
- **`internal/trigger/runner.go`**：`MentalHealthRunner.Run(ctx, req)` — `InsertJob(running)` → `GetLatestAssessment`（只读跨 schema 查询）→ `CompleteJob(done, result)` / `FailJob(failed, err)`。对评估仓库依赖窄接口 `assessmentGetter`（仅 GetLatestAssessment），单测可用 stub。
- **`internal/trigger/postgres_job_store.go`**：`JobStore` 的 Postgres 实现（INSERT / UPDATE assessment_jobs）。
- **main.go**：worker body 由 no-op 日志替换为 `runner.Run`；无 db 时降级为 skip + warn。

---

## 四、提交链（10 commits，每步单测绿 + vet 绿）

| # | Commit | 类型 | 内容 |
|---|--------|------|------|
| 1 | `f1b64d4` | 🔴 test | EventRepo 3 查询方法集成测试（seed helper + 窗口/隔离/空数据） |
| 2 | `df8a055` | 🟢 feat | PostgresEventRepo 真 SQL（day-night/depth/frequency） |
| 3 | `ec71987` | 🔧 fix | migration 001 VIEW 列对齐（send_time→created_at、drop risk_level） |
| 4 | `1734be6` | 🔴 test | ReportRepo 集成测试（完整 harness：4 schema + 底层表 + migrations 001-004 + 种子） |
| 5 | `a496bac` | 🟢 feat | PostgresReportRepo 真 SQL + buildTrendPoints 连续桶 |
| 6 | `d117b57` | 🔴 test | MentalHealthRepo 集成测试 + riskFromScore 单测 |
| 7 | `9fdcbf3` | 🟢 feat | PostgresMentalHealthRepo 真 SQL + risk 推导 + cursor 分页 |
| 8 | `d05d300` | 🔴 test | migration 005 + MentalHealthRunner 单测/集成测试 |
| 9 | `0b6ef5a` | 🟢 feat | MentalHealthRunner + PostgresJobStore + main.go 接线 + MV refresh |
| 10 | `e5dd4b6` | ♻️ refactor | 清理过时的 "Round 5 pending" 注释 |

---

## 五、验证矩阵

| 命令 | 结果 |
|------|------|
| `go test ./...`（8 packages，≤5s） | ✅ 全绿 |
| `go vet ./...` | ✅ 无 warning |
| `go vet -tags integration ./...` | ✅ 无 warning |
| `go build -tags integration ./...` | ✅ OK |
| `go test -tags integration -timeout 8m ./integration_test/`（Docker, 全部 20+ 用例） | ✅ 全绿 |
| `grep t.Skip`（除 `testing.Short()` 环境守卫） | ✅ 0 命中 |
| `grep "Round 5 实现未落地"` | ✅ 0 命中（占位 error 全部移除） |

新增测试统计：**19 个集成测试**（EventRepo 6 + ReportRepo 4 + MentalHealthRepo 6 + Runner 2 + 既有 harness）+ **5 个 runner 单测** + **1 个 riskFromScore 单测**。

---

## 六、设计要点与说明

- **时区确定性**：所有日期边界以 `YYYY-MM-DD` 字符串传 SQL（`$n::date`），SQL 内的 DATE 域运算（`day - start`、`start + k*bucket_days`）不依赖数据库/Go 会话时区 —— 测试在任何机器 TZ 下可重现（避免 `timestamptz::date` 在 UTC+8 与 UTC 机器上相差一天）。
- **跨 schema 只读边界**：所有查询走 `*_v` VIEW + `user_behavior_events`；唯一写入在 analytics 自有 `assessment_jobs`（migration 005）。
- **reports 端点走实时 VIEW `daily_emotion_v`**（非 MV）：查询永远最新，MV 保留用于未来加速；启动时 REFRESH 已补（v1 无 pg_cron）。
- **`GetInteractionDepth` 语义**：按现有接口契约（total=窗口内事件数、convs=DISTINCT session_id），与 InMemory 测试替身"最简近似"一致；`session_id` 实际由 Kafka consumer 写入 topic 名（`chat-events`），语义局限已在 consumer 注释中记录，不在本次范围。

---

## 七、剩余 defer（显式保留）

| 项 | 原因 | 后续 |
|----|------|------|
| worker 跨服务调用 assessment-svc 触发评估（`mental_health_service.TriggerAssessment`） | 需 assessment-svc 新增端点 + 鉴权透传 | landing §六"长期"路径，独立 PR |
| pg_cron `*/15 * * * *` REFRESH CONCURRENTLY | v1 启动时 REFRESH 足够 | Stage-2 |
| ClickHouse / StarRocks 替换 Postgres | v1 规模不需要 | Stage-2 |
| 实时事件聚合（5 秒延迟） | v1 batch query 即可 | Stage-2 |
| `analytics_reader` 密码 `CHANGE_ME_AT_DEPLOY` | 生产由 secret manager 覆盖 | 部署时 |

---

## 八、结论

**Stage 30-A 的 SQL defer 项 1–5 全部落地并验证**：9 个业务端点现在对接真实 Postgres 跨 schema 只读聚合，TriggerQueue worker 执行真实评估任务并持久化状态机。加上前置发现并修复的 migration 001 schema 不一致，analytics-svc 的数据层已具备在真库上部署运行的条件。

---

> 生成者: ZCode agent
> 生成时间: 2026-08-30
> 对应 git tag: main @ e5dd4b6
> 前一个 commit: 7f68c94（Stage 30-A completion report）
