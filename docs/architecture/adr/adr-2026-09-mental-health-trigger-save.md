---
adr: 2026-09-mental-health-trigger-save
title: mental_health_assessments 表写入链 = trigger runner 末尾调 Save
status: accepted
date: 2026-09-21
owners: [analytics-svc]
references: [E2E-F-10, E2E-15/stages/e2e-15-reports-dashboard/plan.md, D-10]
supersedes: null
---

# ADR-2026-09 · mental_health_assessments 表写入链 = trigger runner 调 Save

## 上下文

账本 E2E-F-10（2026-09-17 预探查）：`emotion_echo_assessment.mental_health_assessments` 表**生产写入方 = 0**，导致 analytics-svc `/api/v1/mental-health/*` 端点恒返空（`{code:0, data:{assessment: null}}` 或 dimensions=[]）。该表由 `emotion_echo_assessment.assessment_v` 视图暴露给 analytics-svc，是 mental-health 报表的**唯一**数据源。

原 `MentalHealthRunner.Run` 设计：
```
InsertJob(running) → GetLatestAssessment (只读 mental_health_assessments) → CompleteJob
```
**只读不写** —— 假设其他评估服务（ai-svc / survey-results trigger 等）会先写 assessment，再由 runner 读取并打包成 result。但实际上**没有**任何业务路径写 mental_health_assessments：
- `survey_handler.go` 仅 `SaveResult` 写 survey_results（评估问卷结果）
- `trigger/runner.go` 仅读
- `integration_test/*` 才有 INSERT（不算生产写入方）

## 决策

**沿现有 `MentalHealthRunner` 触发链补 INSERT**：

1. **`MentalHealthRepo` 接口加 `Save(ctx, *MentalAssessment) error`**：跨 schema 写入 `emotion_echo_assessment.mental_health_assessments`
2. **`PostgresMentalHealthRepo.Save` 实现**：
   ```sql
   INSERT INTO emotion_echo_assessment.mental_health_assessments
     (user_id, assessment_type, period_start, period_end, overall_score, dimensions)
   VALUES ($1, $2, NULLIF($3, '')::date, NULLIF($4, '')::date, $5, COALESCE($6::jsonb, '{}'::jsonb))
   RETURNING id
   ```
   summary / recommendations 用 DEFAULT（NULL / `[]`），created_at 用 `NOW()`
3. **`MentalHealthRunner.Run` 末尾调 Save**：
   - `assessment != nil` → Save(assessment)（镜像 GetLatestAssessment 读到的数据）
   - `assessment == nil` → Save(placeholder) —— **UserID / Type 透传 Request + 今日窗口 + score=0 + risk="low"**，让 mental_health_assessments 表**首次 trigger 后不再为空**
   - Save 失败 → FailJob + 返回 error（不能让"读成功但写失败"静默通过）
4. **trigger HTTP 端点保留**（POST `/api/v1/mental-health/trigger`）—— 鉴权失败属 dev mode 配 env_secret 不一致，留账下一轮统一治理

## 后果

### 正面
- 账本 E2E-F-10 已翻"已解决"（2026-09-21）
- mental-health 端点端到端打通：trigger → runner → Save → 视图可读 → BFF 返回真数据
- E2E-15 报表 Dashboard 14/14 测试点 + 24/24 Playwright 双 project + 16/16 截图归档
- 客观条件写入 `MentalAssessment` 字段（如 demo 账号 echo 首次触发后 totalScore=999, risk="low"）让表非空，避免 dev 模式首次请求空响应
- 7 条 trigger 单测（4 原有 + 3 新）

### 风险 / 后续

| 风险 | 处置 |
|------|------|
| 触发频率未定义（生产 cron 调度） | 本轮只注 dev mode + CI 一次性手动验证；生产侧自动调度归 E2E-23（健康检查 + 服务发现） |
| GetLatestAssessment SQL 未按 `assessment_type` 过滤（weekly 端点也返 daily 最新一条） | 留账次级 bug，下一轮修（避免本轮范围扩张） |
| dev mode trigger HTTP JWT 401（analytics-svc secret 与 user-svc 不一致） | 留账：dev mode 鉴权一致性下一轮治理 |
| `runner.Run` 写库耗时增加（DB INSERT 多一次往返） | 单测 < 5s；实际 dev 实测 < 50ms；prod 应做 async batch（本轮不涉及） |

### 配套决策
- **D-10**（decisions.md）：mental-health 评估写入链 = trigger runner 末尾补 INSERT
- **D-11**（decisions.md，待补）：dev mode trigger HTTP 鉴权一致性（留账）

### 配套行动
- E2E-15 阶段 1.2 PR #47 + 阶段 1.3 端到端 PR #48（本轮）
- `docs/e2e-roadmap/stages/e2e-15-reports-dashboard/{plan,report}.md` 详档

## 关联

- **E2E-F-10**（账本条目，已翻"已解决"）：mental_health_assessments 表生产写入方 = 0
- **E2E-F-14**（同阶段修）：4 dashboard 空态 div 无 v-if（dashboard 前端层）
- **E2E-F-36**（同阶段复验）：`.page-content overflow-y: auto` 在 dashboard 4 页生效
- `docs/e2e-roadmap/stages/e2e-15-reports-dashboard/plan.md §6 阶段 1.2` 实施范式

## 变更记录

- 2026-09-21：决策落地（E2E-15 阶段 1.2 PR #47 + 1.3 端到端 PR #48）