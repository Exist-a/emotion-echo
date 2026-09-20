---
stage: e2e-14
title: 人格量表与 AI 提示词定制
type: transformation
status: partial
created: 2026-09-20
depends-on: [e2e-13]
blocks: [e2e-15]
gate: []
related-findings: [E2E-F-04]
---

# E2E-14 人格量表与 AI 提示词定制

> 详档。撰写时机：E2E-13 完成后，E2E-14 启动前。
> 执行协议见 [RUNBOOK.md](../RUNBOOK.md)——执行前必读，本文件只描述"这个阶段测什么"。
> 执行完成后，在同一目录按 [_REPORT_TEMPLATE.md](../_REPORT_TEMPLATE.md) 写 `report.md`。

## 1. 阶段目标

实现 D-02 决议：**两种量表并存**——新增人格量表产出心理画像，驱动 AI 回复针对性（改造 system prompt 注入），并在 user 页新增测评图表。

核心链路：**人格量表答题 → 维度评分 → 画像生成 → 注入 AI prompt → 个性化回复**

## 2. 范围与边界

### 做

- 设计人格量表（五维度，30 题，Likert 5 点）
- 创建人格量表种子数据（`surveys` 表新增 `category='personality'`）
- 实现人格维度评分器（`BigFiveScorer`），返回五维度分数
- 扩展 `Result` 模型支持维度分数（`Factors` 字段复用）
- BFF 层注入人格画像到 AI system prompt（动态拼接）
- 前端 `/question` 页按 category 分 tab（症状量表 vs 人格量表）
- 前端结果弹窗展示维度分数（人格量表时显示雷达图）
- user 页新增人格维度雷达图（复用已有 `RadarChart` 组件）
- Playwright spec 固守人格量表全链路 + AI 个性化回复验证

### 不做（边界）

- 症状量表的风险预警逻辑改造（保留现有 PHQ-9/GAD-7/PSQI）
- AI 模型微调或个性化训练（仅做 prompt 工程）
- 用户画像的长期追踪与趋势分析（归后续报表阶段）
- 量表的后台管理功能（管理员增删改量表）
- 多语言量表支持（归 D-04 i18n）

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| E2E-13 心理测验链路修复完成 | ✅ done（12/12 PASS） |
| assessment-svc 服务可用 | ✅ smoke 24/24 PASS |
| `emotion_echo_assessment.surveys` 表已有 PHQ-9 + GAD-7 | ✅ 已确认 |
| 前端 `RadarChart` 组件可用 | ✅ 已确认（`app/components/charts/RadarChart.vue`） |

环境启动命令（**必须带 `--env-file .env.local`**）：

```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local up -d
```

## 4. 代码探查摘要（2026-09-20 实测）

### 4.1 assessment-svc 评分器结构

**现有评分器**（`scorer.go`）：

| 评分器 | 题数 | 分数范围 | 等级 |
|--------|------|---------|------|
| PHQ9Scorer | 9 | 0-27 | none/mild/moderate/severe/extreme |
| GAD7Scorer | 7 | 0-21 | none/mild/moderate/severe |
| PSQIScorer | 7 | 0-21 | none/mild/moderate/severe |
| GenericScorer | N/A | 0-1 | low/medium/high |

**Result 模型**：

```go
type Result struct {
    TotalScore float64            // 总分
    RiskLevel  string             // 风险等级（症状量表用）
    Factors    map[string]float64 // 分项分数（可复用于人格维度）
}
```

**关键发现**：`Factors` 是 `map[string]float64`，可直接存储人格五维度分数（如 `{"openness": 3.8, "conscientiousness": 4.2, ...}`）。`RiskLevel` 语义不匹配人格量表（人格无"风险"概念），需约定人格量表的 level 命名（如 `dimension_profile`）。

**扩展点**：`GetScorer` 的 switch 分支，新增 `case "BIG5":` 即可。

### 4.2 AI 提示词相关

**system prompt 位置**（`ai_stream_handler.go`）：

- 第 226 行（gRPC 路径）：硬编码静态字符串
- 第 284 行（HTTP 路径）：硬编码静态字符串

两处完全相同：`"你是一个温柔、共情的情绪疏导陪伴者。用中文简短回应（2-3 句话），表达理解、不评判、鼓励继续说。"`

**关键发现**：测评结果从未注入到任何 AI 请求中。BFF handler 不依赖 assessment-svc。

**注入方案决策**：BFF 侧拼接（方案 a）

- BFF 查 assessment-svc 拿最新人格结果
- 拼到 system prompt 里
- 优点：改动集中在 BFF，不改 proto + llm-service

### 4.3 前端相关

**`/question` 页**（`index.vue`）：

- category badge 已展示，但**无筛选逻辑**
- 所有量表混在一起展示，无分 tab

**user 页**（`chat/user/index.vue`）：

- 已有 3 个图表（昼夜使用、对话频次、互动深度）
- `RadarChart` 组件已就绪（`app/components/charts/RadarChart.vue`）

**结果弹窗**（`[id].vue`）：

- 只展示 `totalScore` 和 `riskLevel`
- 无维度分数展示

**关键发现**：`RadarChart` 接收 `indicators: RadarIndicator[]` + `data: number[]`，可直接用于人格五维度展示。

### 4.4 数据库

**surveys 表 category 列**：

- 已有值：`'depression'`（PHQ-9）、`'anxiety'`（GAD-7）
- 人格量表新增值：`'personality'`

**种子数据结构**（`06-seed-surveys.sql`）：

```json
{
  "q1": {
    "title": "...",
    "type": "radio",
    "options": [
      {"id": 1, "text": "...", "score": 0},
      {"id": 2, "text": "...", "score": 1},
      ...
    ]
  },
  ...
}
```

**关键发现**：`scoring_rules` 存在数据库中但**实际评分逻辑硬编码在 scorer.go**，没有从数据库读取。人格量表可继续沿用此模式。

### 4.5 Proto 定义

`proto/emotion_llm.proto` ChatCompletionRequest：

```protobuf
message ChatCompletionRequest {
  repeated ChatMessage messages = 1;
  string model = 2;
  double temperature = 3;
  int32 max_tokens = 4;
  string user_id = 5;
  bool with_intent = 6;
  repeated FileAttachment files = 7;
}
```

**关键发现**：无画像字段。采用 BFF 侧注入方案后，proto 无需改动。

## 5. 测试点清单

判定标记：`[A]` 自动可判 · `[V]` 视觉判定 · `[M]` 需人工裁定

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 1 | 人格量表种子数据已就位（surveys 表 ≥ 3 行，category='personality'） | [A] | DB 查询 `SELECT count(*) FROM emotion_echo_assessment.surveys WHERE category='personality'` | SQL 输出 | ⬜ |
| 2 | 人格量表种子数据结构正确（questions JSONB 含 30 题，每题 5 选项） | [A] | DB 查询 + 验证 questions JSONB 结构 | SQL 输出 | ⬜ |
| 3 | 人格维度评分器实现正确（BigFiveScorer 返回五维度分数） | [A] | Go 单测 `TestBigFiveScorer_Score` | 测试输出 | ⬜ |
| 4 | 人格量表提交成功（answers 格式正确，返回维度分数） | [A] | Playwright：提交人格量表 → 断言 HTTP 200 + 返回 factors 含五维度 | spec | ⬜ |
| 5 | 人格量表结果弹窗展示维度分数（非风险等级） | [A]+[V] | Playwright：提交后断言弹窗含维度标签 + 截图 | spec + 截图 | ⬜ |
| 6 | `/question` 页按 category 分 tab 显示 | [A]+[V] | Playwright：断言存在"症状量表"和"人格量表"两个 tab + 截图 | spec + 截图 | ⬜ |
| 7 | 切换 tab 后列表正确筛选 | [A] | Playwright：点击"人格量表" tab → 断言列表只含 personality 类量表 | spec | ⬜ |
| 8 | user 页人格维度雷达图正确渲染 | [A]+[V] | Playwright：访问 `/chat/user` → 断言雷达图存在 + 五维度标签 + 截图 | spec + 截图 | ⬜ |
| 9 | AI 对话注入人格画像（system prompt 含人格维度） | [A] | 截获 AI 请求 → 断言 system prompt 含人格维度关键词（如"开放性"、"尽责性"） | 网络日志 | ⬜ |
| 10 | AI 回复体现个性化（回复风格与人格画像一致） | [M] | 人工评判：高外向性用户收到更积极的回复 | 人工记录 | ⬜ |
| 11 | 人格量表结果持久化（survey_results 表含 factor_scores） | [A] | DB 查询 `SELECT factor_scores FROM emotion_echo_assessment.survey_results WHERE survey_id=<人格量表ID>` | SQL 输出 | ⬜ |
| 12 | 未完成人格量表时 AI 使用默认 prompt（无画像降级） | [A] | 新用户对话 → 断言 system prompt 为默认字符串 | 网络日志 | ⬜ |

汇总：**PASS 0 / FAIL 0 / BLOCKED 0 / N/A 0**（待执行时填写）

## 6. 实现方案

### 6.1 人格量表设计（TDD：先写种子数据契约测试）

**量表选择**：NEO-FFI-30（NEO 五因素库存量表，30 题，公共领域）

**五维度定义**：

| 维度 | 英文 | 题数 | 分数范围 | 人格标签 |
|------|------|------|---------|---------|
| 开放性 | Openness | 6 | 6-30 | 高/中/低 |
| 尽责性 | Conscientiousness | 6 | 6-30 | 高/中/低 |
| 外向性 | Extraversion | 6 | 6-30 | 高/中/低 |
| 宜人性 | Agreeableness | 6 | 6-30 | 高/中/低 |
| 神经质 | Neuroticism | 6 | 6-30 | 高/中/低 |

**种子数据**：`deploy/db/06-seed-surveys.sql` 新增一条人格量表记录

```sql
INSERT INTO emotion_echo_assessment.surveys (code, title, description, category, questions, scoring_rules, version, status)
VALUES (
  'BIG5',
  '人格五因素量表',
  '评估您的五大人格特质（开放性、尽责性、外向性、宜人性、神经质）',
  'personality',
  '{"q1": {"title": "我喜欢尝试新事物", "type": "radio", "options": [{"id": 1, "text": "非常不同意", "score": 1}, {"id": 2, "text": "不同意", "score": 2}, {"id": 3, "text": "中立", "score": 3}, {"id": 4, "text": "同意", "score": 4}, {"id": 5, "text": "非常同意", "score": 5}]}, ...}',
  '{"dimensions": ["openness", "conscientiousness", "extraversion", "agreeableness", "neuroticism"]}',
  1,
  1
);
```

**Go 契约测试**：断言 seed 后 surveys 表含 `category='personality'` 行，questions JSONB 含 30 题。

### 6.2 人格维度评分器（TDD：先写失败测试）

**新增文件**：`emotion-echo-assessment-svc/internal/scoring/bigfive_scorer.go`

```go
type BigFiveScorer struct{}

func (s *BigFiveScorer) Score(answers map[string]int, rules *ScoringRules) (*Result, error) {
    // 1. 验证答案数量（30 题）
    if len(answers) != 30 {
        return nil, fmt.Errorf("requires exactly 30 answers, got %d", len(answers))
    }
    
    // 2. 按维度聚合分数
    dimensions := map[string][]string{
        "openness":         {"q1", "q6", "q11", "q16", "q21", "q26"},
        "conscientiousness": {"q2", "q7", "q12", "q17", "q22", "q27"},
        "extraversion":     {"q3", "q8", "q13", "q18", "q23", "q28"},
        "agreeableness":    {"q4", "q9", "q14", "q19", "q24", "q29"},
        "neuroticism":      {"q5", "q10", "q15", "q20", "q25", "q30"},
    }
    
    factors := make(map[string]float64)
    for dim, questions := range dimensions {
        sum := 0
        for _, q := range questions {
            sum += answers[q]
        }
        factors[dim] = float64(sum)
    }
    
    // 3. 计算总分（所有维度总和）
    totalScore := 0.0
    for _, v := range factors {
        totalScore += v
    }
    
    // 4. 生成人格标签（level 命名约定）
    level := "dimension_profile" // 人格量表无"风险"概念，用此标识
    
    return &Result{
        TotalScore: totalScore,
        RiskLevel:  level,
        Factors:    factors,
    }, nil
}
```

**修改文件**：`scorer.go` 的 `GetScorer` 函数新增 case

```go
case "BIG5":
    return &BigFiveScorer{}
```

**Go 单测**：`bigfive_scorer_test.go`

- 测试正常评分（30 题，五维度分数正确）
- 测试边界（答案数不足/超范围分数）
- 测试维度聚合逻辑

### 6.3 Result 模型扩展

**现有模型已够用**：`Factors map[string]float64` 可存储五维度分数，无需新增字段。

**约定**：人格量表的 `RiskLevel` 返回 `"dimension_profile"`，前端根据此值判断展示方式（雷达图 vs 风险等级）。

### 6.4 BFF 层注入人格画像（TDD：先写失败测试）

**修改文件**：`emotion-echo-web-bff/internal/handler/ai_stream_handler.go`

**方案**：在构造 `llmMessages` 时，查询用户最新人格结果，动态拼接到 system prompt。

```go
// 在 ServeHTTP 方法中，构造 llmMessages 之前
func (h *AIStreamHandler) buildSystemPrompt(userID string) string {
    basePrompt := "你是一个温柔、共情的情绪疏导陪伴者。用中文简短回应（2-3 句话），表达理解、不评判、鼓励继续说。"
    
    // 查询用户最新人格结果
    personalityResult, err := h.getLatestPersonalityResult(userID)
    if err != nil || personalityResult == nil {
        return basePrompt // 降级：无画像时使用默认 prompt
    }
    
    // 拼接人格画像
    personalityContext := h.formatPersonalityContext(personalityResult)
    return fmt.Sprintf("%s\n\n用户人格画像：%s", basePrompt, personalityContext)
}

func (h *AIStreamHandler) getLatestPersonalityResult(userID string) (*types.SurveyResult, error) {
    // 调用 assessment-svc 获取用户最新人格量表结果
    // HTTP: GET /api/v1/surveys/results?user_id=<userID>&category=personality&limit=1
    // 返回最新的一个结果
}

func (h *AIStreamHandler) formatPersonalityContext(result *types.SurveyResult) string {
    // 格式化人格维度为可读文本
    // 例："开放性高（28/30），尽责性中（18/30），外向性高（26/30），宜人性中（19/30），神经质低（10/30）"
    dimensions := []struct {
        name  string
        label string
    }{
        {"openness", "开放性"},
        {"conscientiousness", "尽责性"},
        {"extraversion", "外向性"},
        {"agreeableness", "宜人性"},
        {"neuroticism", "神经质"},
    }
    
    var parts []string
    for _, dim := range dimensions {
        score := result.Factors[dim.name]
        level := h.getDimensionLevel(score) // 高/中/低
        parts = append(parts, fmt.Sprintf("%s%s（%.0f/30）", dim.label, level, score))
    }
    
    return strings.Join(parts, "，")
}

func (h *AIStreamHandler) getDimensionLevel(score float64) string {
    // 6-30 分，18 为中位数
    if score >= 24 {
        return "高"
    } else if score >= 12 {
        return "中"
    }
    return "低"
}
```

**新增依赖**：BFF 需新增 assessment-svc HTTP 客户端方法（`GetLatestPersonalityResult`）。

**Go 契约测试**：

- 测试有人格画像时 system prompt 含维度关键词
- 测试无画像时降级为默认 prompt
- 测试画像格式化逻辑

### 6.5 前端 `/question` 页分 tab

**修改文件**：`emotion-echo-web/app/pages/question/index.vue`

```vue
<template>
  <div class="question-page">
    <!-- 新增 tab 切换 -->
    <div class="tabs">
      <button 
        :class="{ active: activeTab === 'symptom' }"
        @click="activeTab = 'symptom'"
      >
        症状量表
      </button>
      <button 
        :class="{ active: activeTab === 'personality' }"
        @click="activeTab = 'personality'"
      >
        人格量表
      </button>
    </div>
    
    <!-- 筛选后的列表 -->
    <div class="survey-list">
      <div v-for="item in filteredSurveys" :key="item.id" class="survey-card">
        <!-- 现有卡片内容 -->
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
const activeTab = ref<'symptom' | 'personality'>('symptom')

const filteredSurveys = computed(() => {
  if (activeTab.value === 'symptom') {
    return tableData.value.filter(item => item.category !== 'personality')
  }
  return tableData.value.filter(item => item.category === 'personality')
})
</script>
```

### 6.6 前端结果弹窗展示维度分数

**修改文件**：`emotion-echo-web/app/pages/question/[id].vue`

```vue
<template>
  <!-- 结果弹窗 -->
  <div v-if="submitResult" class="result-modal">
    <!-- 症状量表：显示总分和风险等级 -->
    <div v-if="submitResult.riskLevel !== 'dimension_profile'">
      <p>总分：{{ submitResult.totalScore }}</p>
      <p>风险等级：{{ riskLevelLabel(submitResult.riskLevel) }}</p>
    </div>
    
    <!-- 人格量表：显示维度雷达图 -->
    <div v-else>
      <RadarChart 
        :indicators="personalityIndicators"
        :data="personalityData"
      />
      <div class="dimension-details">
        <p v-for="(score, dim) in submitResult.factorScores" :key="dim">
          {{ dimensionLabels[dim] }}：{{ score }}（{{ getDimensionLevel(score) }}）
        </p>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
const personalityIndicators = [
  { name: '开放性', max: 30 },
  { name: '尽责性', max: 30 },
  { name: '外向性', max: 30 },
  { name: '宜人性', max: 30 },
  { name: '神经质', max: 30 },
]

const personalityData = computed(() => {
  if (!submitResult.value?.factorScores) return []
  return [
    submitResult.value.factorScores.openness,
    submitResult.value.factorScores.conscientiousness,
    submitResult.value.factorScores.extraversion,
    submitResult.value.factorScores.agreeableness,
    submitResult.value.factorScores.neuroticism,
  ]
})

const dimensionLabels: Record<string, string> = {
  openness: '开放性',
  conscientiousness: '尽责性',
  extraversion: '外向性',
  agreeableness: '宜人性',
  neuroticism: '神经质',
}

function getDimensionLevel(score: number): string {
  if (score >= 24) return '高'
  if (score >= 12) return '中'
  return '低'
}
</script>
```

**修改类型**：`emotion-echo-web/app/types/api.ts`

```typescript
interface SurveyResult {
  resultId: string
  totalScore: number
  riskLevel: string
  factorScores?: Record<string, number> // 新增：人格维度分数
}
```

### 6.7 user 页新增人格维度雷达图

**修改文件**：`emotion-echo-web/app/pages/chat/user/index.vue`

```vue
<template>
  <div class="user-page">
    <!-- 现有 3 个图表 -->
    ...
    
    <!-- 新增：人格维度雷达图 -->
    <div class="chart-section">
      <h3>人格维度画像</h3>
      <div v-if="personalityResult">
        <RadarChart 
          :indicators="personalityIndicators"
          :data="personalityData"
        />
      </div>
      <div v-else class="empty-state">
        <p>尚未完成人格量表测评</p>
        <NuxtLink to="/question">前往测评</NuxtLink>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
// 获取用户最新人格量表结果
const { data: personalityResult } = await useFetch('/api/v1/surveys/results', {
  query: { category: 'personality', limit: 1 }
})

const personalityIndicators = [
  { name: '开放性', max: 30 },
  { name: '尽责性', max: 30 },
  { name: '外向性', max: 30 },
  { name: '宜人性', max: 30 },
  { name: '神经质', max: 30 },
]

const personalityData = computed(() => {
  if (!personalityResult.value?.factorScores) return []
  return [
    personalityResult.value.factorScores.openness,
    personalityResult.value.factorScores.conscientiousness,
    personalityResult.value.factorScores.extraversion,
    personalityResult.value.factorScores.agreeableness,
    personalityResult.value.factorScores.neuroticism,
  ]
})
</script>
```

### 6.8 AI 个性化回复验证

**Playwright spec**：`emotion-echo-web/e2e/personality-ai.spec.ts`

```typescript
test('AI 回复注入人格画像', async ({ page }) => {
  // 1. 登录并完成人格量表
  await completePersonalitySurvey(page)
  
  // 2. 进入聊天页
  await page.goto('/chat')
  
  // 3. 监听 AI 请求
  const aiRequest = page.waitForRequest('**/api/v1/ai/stream')
  
  // 4. 发送消息
  await page.fill('.chat-input', '我今天心情不好')
  await page.click('.send-button')
  
  // 5. 验证 system prompt 含人格维度
  const request = await aiRequest
  const body = request.postDataJSON()
  const systemMessage = body.messages.find(m => m.role === 'system')
  expect(systemMessage.content).toContain('开放性')
  expect(systemMessage.content).toContain('尽责性')
})
```

## 7. 验收标准（DoD）

- [ ] 全部 12 个测试点通过（或发现问题已分类：范围内修复 / 范围外记账本）
- [ ] 修复项走完 TDD（Red → Green → Refactor）
- [ ] 已验证行为固化为 Playwright spec 回归钉（`e2e/personality.spec.ts` + `e2e/personality-ai.spec.ts`）
- [ ] Go 侧新增 ≥ 10 条测试（BigFiveScorer 单测 + BFF prompt 注入契约测试）
- [ ] 前端 vitest 新增 ≥ 5 条（RadarChart 渲染 + tab 筛选 + 结果弹窗）
- [ ] roadmap 状态更新 + 账本更新（E2E-F-04 翻状态）
- [ ] §2.5 收口自检三连通过

## 8. 已知风险

| 风险 | 应对 |
|------|------|
| 人格量表题目设计的医学准确性 | 使用公共领域的 NEO-FFI-30 标准题目，不做修改 |
| BigFiveScorer 的维度聚合逻辑与题目映射 | 先写契约测试钉住维度-题目映射关系，再实现评分逻辑 |
| BFF 读取测评结果的性能（每次 AI 对话都查 assessment-svc） | 人格画像变化频率极低，可加内存缓（TTL 5 分钟）；或在 chat-svc conversation context 存画像快照 |
| 前端 answers 提交格式（id vs score） | 人格量表种子数据的 option id 和 score 保持一致（id:1→score:1, id:2→score:2, ...），避免映射问题 |
| 人格量表结果弹窗的 UI 空间（雷达图 + 5 个维度详情） | 弹窗尺寸足够（现已有 400px 宽），雷达图 200x200 + 文字列表可放下 |
| 未完成人格量表时的降级策略 | BFF 查询失败或无结果时，使用默认 system prompt；前端显示空态引导 |

## 9. 产出物

- Playwright spec：`emotion-echo-web/e2e/personality.spec.ts`（人格量表链路）
- Playwright spec：`emotion-echo-web/e2e/personality-ai.spec.ts`（AI 个性化回复）
- Go 评分器：`emotion-echo-assessment-svc/internal/scoring/bigfive_scorer.go` + `bigfive_scorer_test.go`
- Go BFF 扩展：`emotion-echo-web-bff/internal/handler/ai_stream_handler.go`（prompt 注入）
- Go BFF 客户端：`emotion-echo-web-bff/internal/downstream/assessment.go`（新增方法）
- 种子数据：`deploy/db/06-seed-surveys.sql`（新增人格量表）
- 前端组件：`emotion-echo-web/app/pages/question/index.vue`（tab 筛选）
- 前端组件：`emotion-echo-web/app/pages/question/[id].vue`（结果弹窗）
- 前端组件：`emotion-echo-web/app/pages/chat/user/index.vue`（雷达图）
- 执行记录：`stages/e2e-14-personality-ai-prompt/report.md`
- 截图：`stages/e2e-14-personality-ai-prompt/screenshots/`

## 10. 依赖与阻塞

- **上游**：E2E-13 心理测验链路修复（✅ done）
- **下游**：E2E-15 报表 Dashboard（可并行，但人格量表数据可用于报表）
- **阻塞项**：无（D-02 已决议）

## 11. 时间估算

| 子任务 | 估算 |
|--------|------|
| 人格量表种子数据 | 2h |
| BigFiveScorer 实现 + 测试 | 4h |
| BFF prompt 注入 + 测试 | 4h |
| 前端 tab 筛选 + 结果弹窗 | 3h |
| user 页雷达图 | 2h |
| Playwright spec 编写 + 调试 | 4h |
| 端到端验收 + 截图 | 2h |
| 文档 + 收口 | 1h |
| **总计** | **22h**（约 3 个工作日）