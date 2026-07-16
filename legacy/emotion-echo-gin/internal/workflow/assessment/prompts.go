package assessment

// EmotionAnalysisPrompt 情绪分析Prompt
const EmotionAnalysisPrompt = `请分析以下对话记录的情绪状态。

对话记录：
{{.Messages}}

请从以下维度分析并返回JSON格式结果：
{
  "emotion_stability": 75,    // 情绪稳定性 (0-100)
  "positive_ratio": 60,       // 积极情绪占比 (0-100)
  "negative_intensity": 40,   // 消极情绪强度 (0-100，越高越消极)
  "regulation_ability": 70,   // 情绪调节能力 (0-100)
  "dominant_emotion": "anxious", // 主导情绪: sad/angry/anxious/calm/joy/neutral
  "summary": "..."            // 一句话总结
}

评分标准：
- 情绪稳定性：情绪是否大起大落，稳定得高分
- 积极情绪占比：joy/neutral出现频率
- 消极情绪强度：sad/angry/anxious的深度和频率
- 情绪调节能力：从消极到积极的恢复速度
- 所有分数0-100，越高表示越健康

只返回JSON，不要有其他内容。`

// TextRiskDetectionPrompt 文本风险检测Prompt
const TextRiskDetectionPrompt = `请检查以下对话记录中是否有心理健康风险信号。

风险关键词（自杀、自伤、绝望、活着没意思、想死、不想活了、结束痛苦、跳楼、割腕）：
- 检测频率和上下文
- 判断是否为真实风险（排除比喻、讨论他人等情况）

对话记录：
{{.Messages}}

返回JSON格式：
{
  "has_risk_keywords": true,
  "risk_keyword_count": 3,
  "risk_contexts": ["..."],
  "risk_severity": "moderate", // none / low / moderate / high
  "assessment": "..."          // 风险判断说明
}

只返回JSON，不要有其他内容。`

// InterventionPrompt 干预建议生成Prompt（通用模板）
const InterventionPrompt = `根据以下心理健康评估结果，生成干预建议。

评估结果：
- 综合评分：{{.OverallScore}}/100
- 风险等级：{{.RiskLevel}}
- 风险因子：{{.RiskFactors}}
- 五维评分：
  - 情绪稳定：{{.EmotionScore}}
  - 抑郁风险：{{.DepressionScore}}
  - 焦虑风险：{{.AnxietyScore}}
  - 压力指数：{{.StressScore}}
  - 社交活力：{{.SocialScore}}

请生成 3-5 条具体可执行的建议，返回JSON格式：
{
  "summary": "一句话总结当前心理状态和建议方向",
  "suggestions": [
    {
      "level": "short_term",      // immediate / short_term / long_term
      "category": "self_help",    // professional / self_help / lifestyle
      "title": "建议标题",
      "content": "具体建议内容，包含可操作步骤",
      "actions": ["可执行动作1", "可执行动作2"]
    }
  ]
}

建议要求：
- 根据风险等级调整建议强度
- critical/high: 包含专业求助建议
- medium: 包含自我调节方法
- low: 积极心态和预防建议
- 每条建议都要具体可执行

只返回JSON，不要有其他内容。`

// SummaryGenerationPrompt 报告摘要生成Prompt
const SummaryGenerationPrompt = `根据以下心理健康评估数据，生成一份通俗易懂的摘要。

数据：
- 风险等级：{{.RiskLevel}}
- 主导问题：{{.DominantIssues}}
- 积极方面：{{.PositiveAspects}}

要求：
1. 语言温和、积极、避免医学术语
2. 一句话总结（50字以内）
3. 面向普通用户，不要吓到人
4. 即使是critical等级，也要温和但明确地表达

返回纯文本摘要，不要加任何格式标记。`
