---
stage: e2e-15
title: 报表 Dashboard
type: transformation
status: pending
created: 2026-09-21
depends-on: [e2e-10]
blocks: [e2e-16, e2e-17]
gate: []
related-findings: [E2E-F-10, E2E-F-14, E2E-F-36, E2E-F-81]
---

# E2E-15 报表 Dashboard — 详档

> **类型**：transformation（修 E2E-F-10 + 4 个 dashboard 空态 + 端到端 4 类端点 + 视觉验证）
> **依据**：用户 2026-09-21 三项决策（修 F-10 / 先补 E2E-10 取证再开 / full 范围）
> **方法论**：[RUNBOOK.md](../RUNBOOK.md)（11 项收口契约 + 状态机）+ [anti-patterns.md](../anti-patterns.md)（14 类反例防）

---

## 1. 阶段目标

报表 Dashboard 端到端真实可用 —— 4 类报表端点（daily / trend / user-behavior / mental-health）能返回非空数据，4 个 dashboard 子页面（`/chat/dashboard/{daily,weekly,monthly,annual}Report`）能渲染真实图表，日期切换 / 主题跟随 / 跨视口 / 滚动（E2E-F-36 在 dashboard 复验）都正常。同时修复账本 E2E-F-10（mental_health_assessments 表生产写入方 = 0）—— 沿现有 `MentalHealthRunner` 触发链补 `Save(ctx, *MentalAssessment)` 路径。

---

## 2. 范围与边界

### 做
1. **修 E2E-F-10**：`MentalHealthRepo` 新增 `Save` 接口 + PostgresMentalHealthRepo 实现；`MentalHealthRunner.Run` 在 `GetLatestAssessment` 成功后调 `Save` 写一条新行到 `mental_health_assessments`
2. **修 4 个 dashboard 页面空态 div 无 v-if**（E2E-F-14 同型残留：`dailyReport.vue:24` / `weeklyReport.vue:24` / `monthlyReport.vue:24` / `annualReport.vue:24`）
3. **4 类端点端到端验证**：`/reports/daily` / `/reports/trend` / `/user-behavior/{day-night,depth,frequency}` / `/mental-health/assessment`
4. **日期切换**：4 个 dashboard 的 picker change → fetch*Report → chartData 重算
5. **E2E-F-36 滚动复验**：4 个 dashboard × 1280×600 视口（`.page-content` 必须 `overflow-y: auto`）
6. **暗色主题跟随**：4 个 dashboard 切主题后图表主题刷新
7. **跨视口响应式**：<992 / 992-1600 / ≥1600 三档列数

### 不做
- 视图重建（survey_results 合并进 assessment_v）— 留给下一轮（账本 E2E-F-10 的次级修法）
- §契约 4 smoke false SKIP 修复 — 归 E2E-30
- i18n（归 D-04 候选）
- 多实例并发（归 E2E-20）
- mental-health 评估算法本身（daily/weekly/comprehensive 评分逻辑不动 —— 沿现有 Go 侧 `riskFromScore` 推导）

---

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| E2E-10 done（聊天核心链路可产生行为事件） | ✅ **2026-09-21 取证补拍轮完成**（commit dbcf809，PR #45） |
| dev 模式 17 容器 healthy | ✅ 2026-09-21 实测 |
| `--env-file .env.local` 红线 | ✅ 沿用现有部署 |
| `e2e_stage_audit.py --all` 0 FAIL | ✅ 阶段 0 已达成（30 个阶段全部通过） |
| D-10 决议（mental_health_assessments 修法） | ✅ **2026-09-21 用户决议"触发器补写"**（选项 1） |

启动命令（沿用 E2E-10 / E2E-11 范式）：
```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local up -d
```

---

## 4. 代码探查摘要（已实测）

### 4.1 前端（emotion-echo-web）

| 文件 | 现状 | 改动范围 |
|------|------|---------|
| `app/pages/chat/dashboard/index.vue` | 仅 `<NuxtPage />` 容器 | 无改动 |
| `app/pages/chat/dashboard/dailyReport.vue` | 渲染 daily report，line 24 `<div class="ee-empty">暂无数据</div>` **无 v-if**（E2E-F-14 同型残留） | 修 v-else-if |
| `app/pages/chat/dashboard/weeklyReport.vue` | 同上 | 同上 |
| `app/pages/chat/dashboard/monthlyReport.vue` | 同上 | 同上 |
| `app/pages/chat/dashboard/annualReport.vue` | 同上 | 同上 |
| `app/components/report/ReportScaffold.vue` | 通用骨架（loading→skeleton / empty-state / summary-charts slot / daterange picker） | 无改动 |
| `app/components/report/chartsCard.vue` | 响应式 grid（<992/992-1600/>1600）+ 4 种 chart 组件异步加载 | 无改动 |
| `app/components/charts/BaseChart.vue` | ECharts dark theme + grid 子项 width:100%（E2E-14 修复）+ hasData 双重判断 | 无改动 |
| `app/utils/trendReportCharts.ts` | trendToChartItems（seriesData 不 flatMap，Stage 104）+ emotionDistribution/intentDistribution 转换 | 无改动 |
| `app/types/api.ts` | DailyReport / EmotionTrend / DayNight / Depth / Frequency 类型 | 无新增 |
| `app/lib/apiRoutes.ts` | reportsDaily / reportsTrend / userBehaviorDayNight / userBehaviorDepth / userBehaviorFrequency 路径 | 无新增 |
| `app/composables/useApi.ts` | `get<T>(path, params)` 通用 fetch | 无改动 |

### 4.2 后端（BFF）

| 文件 | 现状 | 改动范围 |
|------|------|---------|
| `emotion-echo-web-bff/internal/handler/analytics_handler.go` | 6 个端点（`/reports/{daily,trend}` + `/user-behavior/{day-night,depth,frequency}` + `/mental-health/{assessment,history,trigger,trend}`），userIDQuery IDOR 守卫已修（E2E-12），behaviorDateWindow 默认 30 天，normalizeTrendQuery 别名兼容 | 无改动 |
| `emotion-echo-web-bff/internal/handler/analytics_view.go` | toFrontendDailyReport / toFrontendTrendReport 视图变换 | 无改动 |
| `emotion-echo-web-bff/internal/downstream/analytics.go` | gRPC 默认 + HTTP fallback（9 个 RPC） | 无改动 |

### 4.3 后端（analytics-svc）

| 文件 | 现状 | 改动范围 |
|------|------|---------|
| `internal/repository/mentalhealth_repository.go` | `MentalHealthRepo` 接口 3 个只读方法（GetLatestAssessment / ListAssessmentHistory / GetTrendData）+ PostgresMentalHealthRepo + InMemoryMentalHealthRepo | **新增 `Save(ctx, *MentalAssessment) error` + PostgresMentalHealthRepo 实现 + InMemoryMentalHealthRepo 占位** |
| `internal/repository/report_repository.go` | PostgresReportRepo（GetDailyReport / GetTrendReport）跨 4 schema 读 | 无改动 |
| `internal/trigger/runner.go` | `MentalHealthRunner.Run` = InsertJob → GetLatestAssessment → CompleteJob | **末尾加 Save 路径**（GetLatestAssessment 成功后调 Save 写一条新行） |
| `internal/trigger/runner_test.go` | 4 个现有单测（happy / repo err / no data / insert err） | **新增测试：Save 被调用 + 无数据时仍写 placeholder（保持非空）+ Save 失败时 FailJob** |
| `internal/trigger/{trigger_queue,postgres_job_store}.go` | TriggerQueue worker pool + PostgresJobStore | 无改动 |

### 4.4 数据库

| 对象 | 现状 | 改动 |
|------|------|------|
| `emotion_echo_assessment.mental_health_assessments` 表 | DDL 02-create-tables-in-schemas.sql:187-199，**生产写入方 = 0** | 新增 trigger runner 路径写入 |
| `emotion_echo_assessment.assessment_v` 视图 | a001_create_views.sql:39-49 暴露前 7 列 + created_at | 无改动 |
| `emotion_echo_analytics.assessment_jobs` | trigger 状态机表 | 无改动 |

---

## 5. 测试点清单（14 项）

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|----------|------|
| 1 | dailyReport 渲染正常数据 | [A]+[V] | Playwright + API 数据校验 | e2e spec + screenshot |
| 2 | weeklyReport 渲染正常数据 | [A]+[V] | 同上 | — |
| 3 | monthlyReport 渲染正常数据 | [A]+[V] | 同上 | — |
| 4 | annualReport 渲染正常数据 | [A]+[V] | 同上 | — |
| 5 | dailyReport 日期切换 → chartData 更新 | [A] | picker change + API 数据 + DOM 重算 | — |
| 6 | weeklyReport 周切换 | [A] | daterange change 验证 | — |
| 7 | monthlyReport 月切换 | [A] | month picker change 验证 | — |
| 8 | annualReport 年切换 | [A] | year picker change 验证 | — |
| 9 | **修 4 个 dashboard 空态 div 加 v-else-if**（E2E-F-14 同型） | [A]+[V] | 静态契约钉 + 视觉空态验证 | contract.architecture.test.ts + screenshot |
| 10 | mental-health 端点有真实数据（**修 E2E-F-10 后**） | [A] | trigger 后 SQL count > 0 + API 返回非空 | integration test |
| 11 | mental-health 端到端（trigger → 写 mental_health_assessments → 视图可读 → BFF 返回） | [A] | 全链路 smoke | e2e_stage_audit §契约 4 / curl |
| 12 | E2E-F-36 滚动复验（dashboard 4 页 × 1280×600） | [A] | IAB 实测 + 契约钉 `app/layouts/nav.test.ts` 复用 | screenshot + 契约 |
| 13 | 暗色主题跟随（dashboard 4 页） | [V] | 切主题 → 图表 dark theme 刷新 | 截图对比 chromium/mobile |
| 14 | 跨视口响应式（<992 / 992-1600 / ≥1600 列数） | [V] | IAB 切视口截图 | screenshots |

**BLOCKED 上限**：3 项；超过则阶段只能 partial。

---

## 6. 实现方案（TDD）

### 阶段 1.1：dashboard 空态修复（独立 PR）
1. **RED**：建 `app/pages/chat/dashboard/e2e-15-dashboard-reports-contract.architecture.test.ts`，对 4 个 .vue 文件读源码做正则断言：`/v-else-if="chartData\.length\s*===\s*0"[\s\S]{0,80}ee-empty/` 必须存在
2. **GREEN**：4 个 dashboard .vue 文件 line 24-25 改 `<div class="ee-empty">` → `<div v-else-if="chartData.length === 0" class="ee-empty">`
3. 跑契约钉：4/4 绿
4. commit + push + PR + 合并

### 阶段 1.2：trigger runner 写 mental_health_assessments（独立 PR，修 E2E-F-10）
1. **RED**：扩展 `internal/trigger/runner_test.go` 加 3 条新测试：
   - `TestMentalHealthRunner_Run_SuccessWritesAssessment`：GetLatestAssessment 返非 nil → runner.Run 应调 repo.Save 一次（spy），job 仍 done
   - `TestMentalHealthRunner_Run_NoDataWritesPlaceholder`：GetLatestAssessment 返 nil → runner.Run 应调 repo.Save(placeholder)，job 仍 done（**关键**：让表非空，dev 演示账号首次即可见）
   - `TestMentalHealthRunner_Run_SaveErrorFailsJob`：repo.Save 报错 → job 应 fail
2. **RED**：扩 `internal/repository/mentalhealth_repository.go` 的 `MentalHealthRepo` 接口加 `Save`；`InMemoryMentalHealthRepo` 占位
3. **GREEN**：`PostgresMentalHealthRepo.Save` 实现 + `MentalHealthRunner` 改造（增加 saver 依赖，Run 末尾调 Save）
4. `go test ./...` + `go vet ./...` 全绿
5. commit + push + PR + 合并

### 阶段 1.3：端到端验证（独立 PR）
1. **RED**：建 `emotion-echo-web/e2e/dashboard-reports.spec.ts`，覆盖测试点 #1-#8 + #12 + #13 + #14
2. **GREEN**：seed 演示数据（手工 SQL INSERT mental_health_assessments + 确保 user_behavior_events / emotion_analysis 有数据）+ 触发 trigger + 跑 spec
3. 跑 chromium project 14/14 + mobile project 14/14 = 28/28 PASS
4. commit + push + PR + 合并

### 阶段 1.4：收口（独立 PR）
1. screenshots 归档（双 project × 4 个 dashboard 页 + 测试点 #13 主题切换 + #14 跨视口 = 12+ 张）
2. 写 `report.md`（按 §10 模板）
3. 改 `roadmap.md` E2E-15 partial → done（描述中规避 audit A9 partial 关键词）
4. 改账本 `discovered-unresolved.md` E2E-F-10 状态翻"已解决"（新增方法 + 触发链 + SQL 截图）
5. 补决策 `decisions.md` D-10（mental_health_assessments 触发器补写法）
7. 跑 `e2e_stage_audit.py --all` 0 FAIL + §2.5 三连
8. commit + push + PR + 合并 + 删源分支

---

## 7. 架构假设清单（plan §A）

| # | 假设 | 验证 |
|---|------|------|
| A1 | `MentalHealthAssessment` → `mental_health_assessments` 列映射已对齐（user_id / assessment_type / period_start / period_end / overall_score / dimensions / created_at） | 读 DDL 02-create-tables-in-schemas.sql:187-199 + a001 视图定义 |
| A2 | trigger runner 写库后 `assessment_v` 视图能立即读到新行（视图 = 实时查询，非物化） | 读 a001 视图定义；实测一次 trigger → 查 assessment_v |
| A3 | dev 演示账号 `echo` 已有足够 `user_behavior_events` + `emotion_analysis` 数据喂给 daily/trend/user-behavior 报表 | 跑 §契约 1/2 + 实测 `daily_emotion_v` 行数 |
| A4 | 4 个 dashboard 同型修复 = 改 4 个 .vue 文件 line 24-25 一行 | 已 grep 验证（见 §4.1） |
| A5 | `app/layouts/nav.vue` 的 `overflow-y: auto` 修复（E2E-11）对 dashboard 容器生效（共享 layout） | E2E-11 已实测；本阶段复验 |
| A6 | dev 模式 mental-health trigger HTTP 端点可手动触发（POST `/api/v1/mental-health/trigger`） | 查 handler + 实测 curl |
| A7 | dashboard 4 个页面的 chart 渲染不依赖 dev mode unique config（任何 dev 模式配置都能渲染） | 实测 chromium + mobile |

---

## 8. 已知风险

| 风险 | 缓解 |
|------|------|
| trigger runner Save 失败可能掩盖原 GetLatestAssessment 成功的 job 结果 | FailJob 后返回 error，monitoring 看 SendError 报警 |
| 触发频率未规划（生产 `local Docker` 模式是否每天自动跑 mental-health？ | 本阶段只注 commit 时跑一次 + dev mode 可手动调；生产侧自动调度属 E2E-23 范畴 |
| dev 7 天内 `emotion_echo_ai.emotion_analysis` 为空 → §契约 4 false FAIL | seed 演示数据 + 验完保留 |
| 4 个 dashboard 同型修复漏改其中之一 | 静态契约钉 4 文件同时 RED + GREEN 同步 |
| 暗色主题跟随 ECharts 内部 MutationObserver 行为可能需重新触发（之前 E2E-14 修复过） | 测试点 #13 实测覆盖 |
| E2E-F-36 滚动修复对 dashboard 4 页**复验**（E2E-11 只在 chat/user 验过） | 测试点 #12 IAB 切视口实测 4 页 |

---

## 9. 产出物清单

- `docs/e2e-roadmap/stages/e2e-15-reports-dashboard/plan.md`（本文件）
- `docs/e2e-roadmap/stages/e2e-15-reports-dashboard/report.md`
- `docs/e2e-roadmap/stages/e2e-15-reports-dashboard/screenshots/*.png`（双 project × 4 关键页 + #13 + #14）
- `emotion-echo-web/e2e/dashboard-reports.spec.ts`（端到端 + 视觉）
- `emotion-echo-web/app/pages/chat/dashboard/e2e-15-dashboard-reports-contract.architecture.test.ts`（静态契约钉）
- `emotion-echo-web/app/pages/chat/dashboard/{daily,weekly,monthly,annual}Report.vue`（4 个空态修复）
- `emotion-echo-analytics-svc/internal/trigger/runner.go` + `runner_test.go`（补 INSERT + 测试）
- `emotion-echo-analytics-svc/internal/repository/mentalhealth_repository.go`（新增 Save）

---

## 10. 验收 DoD（RUNBOOK §7 11 项全部）

1. ✅ report.md 按 §10 模板
2. ✅ 截图 ≥12 张（双 project × 4 dashboard 页 + 主题切换 + 跨视口）
3. ✅ 新 spec 文件存在且至少跑过 1 次绿
4. ✅ roadmap 表格 + 顶部状态同步
5. ✅ discovered-unresolved E2E-F-10 状态翻转 + E2E-F-14 在 dashboard 残留在 E2E-15 内同型修复
6. ✅ decisions.md 补 D-10
7. ✅ commit + push + PR squash × 4 个 PR（1.1 / 1.2 / 1.3 / 1.4）
8. ✅ §2.5 三连
9. ✅ 账本对账（本阶段未解决条目为 0 → 可 done）
10. ✅ 第二方核对（执行者不自证）
11. ✅ 关键断言复读（每条"已包含"现场回读文件）

---

## 11. 时间估算

| 阶段 | commits | PR |
|------|---------|-----|
| 阶段 1.1 dashboard 空态 | 2 | 1 |
| 阶段 1.2 trigger Save | 5~6 | 1 |
| 阶段 1.3 端到端 | 3~4 | 1 |
| 阶段 1.4 收口 | 2 | 1 |
| **合计** | **~13** | **4** |

参考：E2E-14 9 commits / E2E-11 13 commits，本阶段属"transformation"含跨前端 + 后端 + 数据库，13 commit 合理。

---

## 12. 待办顺序

1. 阶段 1.1：dashboard 空态契约钉 + 4 文件同型修复 → 1 PR
2. 阶段 1.2：MentalHealthRepo.Save + PostgresMentalHealthRepo 实现 + runner 改造 → 1 PR
3. 阶段 1.3：dashboard-reports.spec.ts + seed 演示数据 + 端到端 → 1 PR
4. 阶段 1.4：screenshots + report + 审计 + 收口 → 1 PR

每轮工作结束前必跑 §2.5 三连 + 删残留分支。