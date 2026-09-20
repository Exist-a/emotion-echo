---
stage: e2e-13
title: 心理测验链路修复
type: transformation
status: done
created: 2026-09-20
depends-on: [e2e-06]
blocks: [e2e-14]
gate: []
related-findings: [E2E-F-02, E2E-F-03]
---

# E2E-13 心理测验链路修复

> 详档。撰写时机：E2E-12 完成后，E2E-13 启动前。
> 执行协议见 [RUNBOOK.md](../RUNBOOK.md)——执行前必读，本文件只描述"这个阶段测什么"。
> 执行完成后，在同一目录按 [_REPORT_TEMPLATE.md](../_REPORT_TEMPLATE.md) 写 `report.md`。

## 1. 阶段目标

让心理测验链路（列表→答题→提交→结果查看）从"三层契约错位、100% 失败"变成"端到端跑通、回归钉固守"。

## 2. 范围与边界

### 做

- 修复前端↔BFF↔assessment-svc 的 6 处数据契约错位
- 创建量表种子数据（PHQ-9 + SAS，从 legacy 迁移并适配新 schema）
- 前端结果展示读正确字段 + 风险等级中文映射
- Playwright spec 固守列表→答题→提交→结果全链路
- Go 侧契约测试

### 不做（边界）

- 人格量表设计（归 E2E-14）
- 量表内容的医学准确性审核（种子数据用标准 PHQ-9/SAS 原文）
- assessment-svc 的 scoring 引擎改造（现有 PHQ-9/GAD-7/PSQI/Generic scorer 已可用）
- 用户历史答题记录的高级查询（归后续报表阶段）

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| E2E-06 数据库改造完成 | ✅ done |
| assessment-svc 服务可用 | ✅ smoke 24/24 PASS |
| `emotion_echo_assessment.surveys` 表存在但为空 | ✅ 已确认 |

环境启动命令（**必须带 `--env-file .env.local`**）：

```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local up -d
```

## 4. 代码探查摘要（2026-09-20 实测）

### 4.1 已发现的 6 处契约错位

| # | 位置 | 前端发/读 | 后端/BFF 期望/返回 | 后果 |
|---|------|----------|-------------------|------|
| M1 | 提交答案格式 | `[{questionId, optionId}]` 数组 | `{"q1": 3, "q2": 2}` map | 提交恒 400 |
| M2 | 结果字段名 | `level` / `suggestion` | `riskLevel`（无 suggestion） | 结果弹窗等级恒空 |
| M3 | 列表 wrapper key | `data.list` | `data.items` | 列表页取数失败报错 |
| M4 | 列表项字段 | 需 `estimatedTime`/`status`/`completedAt` | 无这些字段，有 `code`/`questionNum`/`category` | 列表渲染缺字段 |
| M5 | 详情字段 | 需结构化 `questions: Question[]` | 返回 `questions: map[string]any`（JSONB 原始 blob） | 题目渲染取决于种子数据格式 |
| M6 | 结果查询 URL | `/surveys/result/${id}`（单数） | `/api/v1/surveys/results/${id}`（复数） | 结果查询 404 |

### 4.2 种子数据现状

- `emotion_echo_assessment.surveys` 表存在但**零行数据**
- legacy 有 `002_seed_surveys.up.sql`（SDS + SAS），但 schema 不兼容（无 `code`/`category`/`scoring_rules`/`version`/`status` 列）
- 需要写新的种子 SQL 适配当前 schema

### 4.3 前端文件清单

| 文件 | 关键内容 |
|------|---------|
| `emotion-echo-web/app/pages/question/index.vue` | 列表页：fetchSurveys() 读 `data.list`、checkRes() 硬编码路径 |
| `emotion-echo-web/app/pages/question/[id].vue` | 答题页：handleSubmit() 构建数组格式答案、结果读 `level`/`suggestion` |
| `emotion-echo-web/app/types/api.ts:247-309` | SurveyItem / SurveyDetail / SubmitSurveyParams / SurveyResult 类型定义 |

### 4.4 后端文件清单

| 文件 | 关键内容 |
|------|---------|
| `emotion-echo-assessment-svc/internal/handler/survey_handler.go` | Gin HTTP handler，BFF 直接透传 |
| `emotion-echo-assessment-svc/internal/types/types.go` | SubmitSurveyReq `Answers map[string]int`、SubmitSurveyResp `RiskLevel` |
| `emotion-echo-assessment-svc/internal/scoring/scorer.go` | PHQ-9 期望 `{"q1":0-3,...,"q9":0-3}` |
| `emotion-echo-web-bff/internal/handler/survey_handler.go` | BFF handler，list 返回 `gin.H{"items":...,"total":...}` |
| `emotion-echo-web-bff/internal/downstream/assessment.go` | HTTP 客户端，SubmitSurveyReq `Answers map[string]int` |

### 4.5 Proto 定义

`proto/agent.proto` AssessmentService：
- `ListSurveys` → `{items: [SurveyItem], total}`
- `GetSurvey` → `{id, code, title, category, version, questions: [SurveyQuestion]}`
- `SubmitSurvey` → answers 为 `repeated Answer`（oneof: scale_value/text_value/option_value）
- `GetSurveyResult` → `{resultId, surveyId, totalScore, riskLevel, ...}`

## 5. 测试点清单

判定标记：`[A]` 自动可判 · `[V]` 视觉判定 · `[M]` 需人工裁定

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 1 | 量表种子数据已就位（surveys 表 ≥ 2 行，PHQ-9 + SAS） | [A] | DB 查询 `SELECT count(*) FROM emotion_echo_assessment.surveys` | SQL 输出 | ⬜ |
| 2 | 种子数据结构正确（questions JSONB 含 q1~qN 键，与 scorer 期望一致） | [A] | DB 查询 + 对比 scorer.go 的 PHQ-9 期望格式 | SQL 输出 + 代码对比 | ⬜ |
| 3 | 列表页正常加载并显示量表卡片 | [A]+[V] | Playwright：访问 `/question`，断言列表项 ≥ 2 + 截图 | spec + 截图 | ⬜ |
| 4 | 列表项显示标题、描述、题数等基本信息 | [V] | Playwright：断言卡片含 title + questionNum 文本；截图 | spec + 截图 | ⬜ |
| 5 | 点击量表进入答题页，题目正确渲染 | [A]+[V] | Playwright：点击第一项 → 断言题目列表非空 + 每题有选项；截图 | spec + 截图 | ⬜ |
| 6 | 答题后提交成功（不再 400） | [A] | Playwright：选完所有题 → 提交 → 断言 HTTP 200 + 返回 resultId | spec | ⬜ |
| 7 | 提交的 answers 格式为 map[string]int（非数组） | [A] | 截获请求体，断言 `typeof answers === 'object' && !Array.isArray(answers)` | spec + 网络日志 | ⬜ |
| 8 | 结果弹窗显示分数和风险等级（level 非空） | [A]+[V] | Playwright：提交后断言弹窗含 totalScore 数字 + riskLevel 文本；截图 | spec + 截图 | ⬜ |
| 9 | 风险等级为中文可读文本（非 raw 英文如 "mild"） | [A] | Playwright：断言 riskLevel 文本匹配 `/正常|轻度|中度|重度|极重度/` | spec | ⬜ |
| 10 | 列表页刷新后已答量表显示"已完成"状态 | [A] | Playwright：刷新列表页 → 断言对应项 status 文本含"已完成"或类似标记 | spec | ⬜ |
| 11 | 结果查询接口返回正确数据（URL 路径正确） | [A] | Playwright：监听 `/surveys/results/` 请求 → 断言 200 + 含 totalScore | spec | ⬜ |
| 12 | 未知/不存在的量表 ID 返回友好错误（非白屏） | [A] | Playwright：访问 `/question/99999` → 断言页面有错误提示或重定向 | spec | ⬜ |

汇总：**PASS 0 / FAIL 0 / BLOCKED 0 / N/A 0**（待执行时填写）

## 6. 修复方案

### 6.1 种子数据（TDD：先写契约测试）

**新增文件**：`deploy/db/06-seed-surveys.sql`

内容：插入 PHQ-9（9 题，category=depression）和 SAS（20 题，category=anxiety）到 `emotion_echo_assessment.surveys`。

关键约束：
- `questions` JSONB 格式必须匹配 scorer 期望：`{"q1": {"text":"...", "options":[{"text":"...","score":0},...]}, ...}`
- `scoring_rules` JSONB 包含阈值：`{"thresholds":[{"min":0,max:4,level:"normal"},...]}`
- `code` 为唯一标识（`PHQ-9` / `SAS`）
- `version=1`, `status=1`（启用）

**Go 契约测试**：断言 seed 后 surveys 行数 ≥ 2 且 questions JSONB 结构合法。

### 6.2 提交答案格式（M1）

**改前端**：`[id].vue` handleSubmit() 从构建数组改为构建 map：

```ts
// 改前（M1 根因）
const answers = Object.entries(answerMap.value)
  .filter(([, optionId]) => optionId > 0)
  .map(([questionId, optionId]) => ({ questionId: Number(questionId), optionId }))

// 改后
const answers: Record<string, number> = {}
Object.entries(answerMap.value).forEach(([qId, optId]) => {
  if (optId > 0) answers[qId] = optId
})
```

注意：key 必须是 `"q1"`, `"q2"` 等（匹配种子数据的 questions JSONB 键名），不是数据库 ID。前端 `answerMap` 的 key 来自题目渲染时的 key，需确保与种子数据一致。

**Go 契约测试**：用 `map[string]int` 格式提交 → 断言 200 + 返回 resultId。

### 6.3 结果字段名（M2）

**改前端**：`types/api.ts` SurveyResult 类型：

```ts
// 改前
{ resultId: string, totalScore: number, level: string, suggestion: string }

// 改后
{ resultId: string, totalScore: number, riskLevel: string }
```

**改前端**：`[id].vue` 和 `index.vue` 模板中 `{{ submitResult.level }}` → `{{ submitResult.riskLevel }}`，移除 `suggestion` 引用。

**增加前端映射层**：将 `riskLevel` 英文值映射为中文：

```ts
const riskLevelMap: Record<string, string> = {
  normal: '正常', mild: '轻度', moderate: '中度',
  severe: '重度', extremely_severe: '极重度'
}
```

### 6.4 列表 wrapper key（M3）

**改前端**：`index.vue` line 108-109：

```ts
// 改前
const data = await get<{ list: SurveyItem[] }>(API_ROUTES.surveys.path)
tableData.value = data.list.map(...)

// 改后
const data = await get<{ items: SurveyItem[] }>(API_ROUTES.surveys.path)
tableData.value = data.items.map(...)
```

### 6.5 列表项字段（M4）

**改前端**：`types/api.ts` SurveyItem 类型，移除后端不提供的字段，添加后端实际返回的字段：

```ts
interface SurveyItem {
  id: number
  code: string        // 新增（后端返回）
  title: string
  description: string
  category: string    // 新增（后端返回）
  questionNum: number // 新增（后端返回）
  version: number     // 新增（后端返回）
  // status / estimatedTime / completedAt / resultId → 由前端根据用户答题记录计算或移除
}
```

列表页模板需相应调整，用 `questionNum` 显示题数，移除 `estimatedTime` 和 `status` 显示（或改为从结果查询接口获取已完成状态）。

### 6.6 结果查询 URL（M6）

**改前端**：`index.vue` line 135：

```ts
// 改前
const result = await get<SurveyResult>(`/surveys/result/${data.resultId}`)

// 改后（使用 API_ROUTES 或直接修正路径）
const result = await get<SurveyResult>(`/surveys/results/${data.resultId}`)
```

### 6.7 详情字段对齐（M5）

后端 `GetSurveyResp` 返回 `Questions map[string]any`，即 JSONB 原始 blob。种子数据写入时确保 JSONB 结构与前端 `Question` 类型兼容：

```ts
// 前端期望的 Question 结构
{ id: number, title: string, type: 'radio', options: [{ id: number, text: string, score: number }] }
```

种子数据的 questions JSONB 需要包含这个结构，或者后端做一次映射。**推荐**：种子数据直接按前端期望的结构写入 JSONB，避免后端改动。

## 7. 验收标准（DoD）

- [ ] 全部 12 个测试点通过（或发现问题已分类：范围内修复 / 范围外记账本）
- [ ] 修复项走完 TDD（Red → Green → Refactor）
- [ ] 已验证行为固化为 Playwright spec 回归钉（`e2e/quiz.spec.ts`）
- [ ] Go 侧新增 ≥ 5 条契约测试（种子数据格式、提交答案格式、结果字段）
- [ ] roadmap 状态更新 + 账本更新（E2E-F-02/03 翻状态）
- [ ] §2.5 收口自检三连通过

## 8. 已知风险

| 风险 | 应对 |
|------|------|
| 种子数据 questions JSONB 格式与 scorer 期望不匹配 | 先读 scorer.go 的 PHQ-9 期望格式，按其写种子数据；用契约测试钉住 |
| 前端 answerMap 的 key 与种子数据 questions 的 key 不一致 | 在渲染题目时用种子数据的 key（q1/q2/...）作为 value，而非数据库 ID |
| Proto `SubmitSurvey` 的 `repeated Answer` 格式与 HTTP 层 `map[string]int` 不一致 | BFF 做转换（HTTP→gRPC），需确认 BFF survey_handler.go 的转换逻辑 |
| 改动涉及前端 + BFF + assessment-svc + 种子数据 4 层，任一层漏改都会链路失败 | 每个测试点逐层验证；先写端到端 Playwright spec（红），再逐层修通（绿） |

## 9. 产出物

- Playwright spec：`emotion-echo-web/e2e/quiz.spec.ts`
- Go 契约测试：`emotion-echo-assessment-svc/internal/handler/survey_handler_test.go`（扩展）
- 种子数据：`deploy/db/06-seed-surveys.sql`
- 执行记录：`stages/e2e-13-quiz/report.md`
- 截图：`stages/e2e-13-quiz/screenshots/`
