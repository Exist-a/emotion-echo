// Package assessment 提供心理健康评估工作流
//
// 工作流阶段：
//   Phase 1: 数据收集（并行）
//     - collect_messages:   多会话消息聚合（支持时间范围）
//     - collect_surveys:    量表结果收集
//     - collect_activity:   行为活跃度计算
//   Phase 2: ReAct 深度分析（循环）
//     - react_analysis:     LLM 多轮反思 + 工具调用
//   Phase 3: 风险评估（顺序）
//     - calculate_risk:     六维评分 + 综合风险
//   Phase 4: 干预建议（条件分支）
//     - 根据 risk_level 选择不同强度的建议
//   Phase 5: 报告生成（顺序）
//     - generate_report:    生成警示标志和摘要
package assessment

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"emotion-echo-gin/internal/models"
	"emotion-echo-gin/internal/repository"
	"emotion-echo-gin/internal/workflow/graph"
	"emotion-echo-gin/internal/workflow/react"
	"emotion-echo-gin/internal/workflow/tools"
)

// BuildAssessmentWorkflow 构建心理状态评估工作流
func BuildAssessmentWorkflow(
	msgRepo *repository.MessageRepository,
	convRepo *repository.ConversationRepository,
	surveyResultRepo *repository.SurveyResultRepository,
	analysisRepo *repository.EmotionAnalysisRepository,
	llmCaller func(ctx context.Context, prompt string) (string, error),
	checkpointer graph.Checkpointer,
) *graph.Graph {
	// 创建工具注册表
	toolRegistry := BuildToolRegistry(msgRepo, convRepo, surveyResultRepo)

	g := graph.NewGraph("mental_health_assessment", checkpointer)

	// Phase 1: 数据收集（并行）
	dataCollection := graph.NewParallelNode("phase1_data_collection",
		buildCollectMessagesNode(msgRepo, convRepo),
		buildCollectSurveysNode(surveyResultRepo),
		buildCollectActivityNode(convRepo, msgRepo),
	)

	// Phase 2: ReAct 深度分析（替代原有的简单情绪分析）
	reactAnalysis := buildReactAnalysisNode(llmCaller, toolRegistry)

	// Phase 3: 风险评估
	riskAssessment := graph.NewSequentialNode("phase3_risk_assessment",
		buildCalculateRiskNode(),
	)

	// Phase 4: 干预建议（条件分支）
	intervention := graph.NewConditionalNode("phase4_intervention", []graph.Branch{
		{
			Condition: &RiskLevelCondition{Level: "critical"},
			Node:      buildInterventionNode("critical", llmCaller),
		},
		{
			Condition: &RiskLevelCondition{Level: "high"},
			Node:      buildInterventionNode("high", llmCaller),
		},
		{
			Condition: &RiskLevelCondition{Level: "medium"},
			Node:      buildInterventionNode("medium", llmCaller),
		},
	}, buildInterventionNode("low", llmCaller))

	// Phase 5: 报告生成
	reportGeneration := graph.NewSequentialNode("phase5_report_generation",
		buildGenerateReportNode(),
	)

	// 构建图结构
	g.AddNode(dataCollection)
	g.AddNode(reactAnalysis)
	g.AddNode(riskAssessment)
	g.AddNode(intervention)
	g.AddNode(reportGeneration)

	g.AddEdge("phase1_data_collection", "phase2_react_analysis", nil)
	g.AddEdge("phase2_react_analysis", "phase3_risk_assessment", nil)
	g.AddEdge("phase3_risk_assessment", "phase4_intervention", nil)
	g.AddEdge("phase4_intervention", "phase5_report_generation", nil)

	return g
}

// buildCollectMessagesNode 构建收集对话节点（支持多会话聚合）
func buildCollectMessagesNode(msgRepo *repository.MessageRepository, convRepo *repository.ConversationRepository) graph.Node {
	return graph.NewFunctionNode("collect_messages", func(ctx context.Context, state graph.State) (graph.State, error) {
		userID := int64(state.GetInt("user_id"))
		convID := state.GetString("conversation_id")

		// 解析时间范围（从 state 获取）
		periodStart := state.GetString("period_start")
		periodEnd := state.GetString("period_end")

		var startTime, endTime time.Time
		var err error

		if periodStart != "" {
			startTime, err = time.Parse(time.RFC3339, periodStart)
			if err != nil {
				startTime = time.Now().AddDate(0, 0, -7)
			}
		} else {
			startTime = time.Now().AddDate(0, 0, -7)
		}

		if periodEnd != "" {
			endTime, err = time.Parse(time.RFC3339, periodEnd)
			if err != nil {
				endTime = time.Now()
			}
		} else {
			endTime = time.Now()
		}

		var messages []*models.Message
		var allMsgTexts []string

		if convID != "" {
			// 单会话模式
			messages, err = msgRepo.ListByConversationID(ctx, convID, 100, 0)
			if err != nil {
				return state, err
			}
		} else if userID > 0 {
			// 多会话模式：按时间范围查询用户的所有会话
			convs, err := convRepo.ListRecentConversations(ctx, startTime, endTime, 100)
			if err != nil {
				return state, err
			}

			for _, conv := range convs {
				if conv.UserID != userID {
					continue
				}
				msgs, err := msgRepo.ListByConversationID(ctx, conv.ID, 50, 0)
				if err != nil {
					continue
				}
				messages = append(messages, msgs...)
			}
		}

		// 构建对话文本（只取用户消息）
		for _, msg := range messages {
			if msg.Sender == "user" || msg.Sender == "assistant" {
				allMsgTexts = append(allMsgTexts, msg.Sender+": "+msg.Content)
			}
		}

		state.Set("messages_raw", messages)
		state.Set("messages_text", strings.Join(allMsgTexts, "\n"))
		state.Set("message_count", len(messages))
		state.Set("msg_count_user", len(allMsgTexts))

		return state, nil
	})
}

// buildCollectSurveysNode 构建收集量表节点
func buildCollectSurveysNode(surveyResultRepo *repository.SurveyResultRepository) graph.Node {
	return graph.NewFunctionNode("collect_surveys", func(ctx context.Context, state graph.State) (graph.State, error) {
		userID := int64(state.GetInt("user_id"))

		// 获取用户量表结果
		results, err := surveyResultRepo.ListByUserID(ctx, userID)
		if err != nil {
			return state, err
		}

		sdsScore := 50 // 默认值
		sasScore := 50

		for _, result := range results {
			switch result.SurveyID {
			case 1: // SDS
				sdsScore = result.TotalScore
			case 2: // SAS
				sasScore = result.TotalScore
			}
		}

		state.Set("sds_score", sdsScore)
		state.Set("sas_score", sasScore)
		state.Set("has_survey", len(results) > 0)

		return state, nil
	})
}

// buildCollectActivityNode 构建收集活跃度节点
func buildCollectActivityNode(convRepo *repository.ConversationRepository, msgRepo *repository.MessageRepository) graph.Node {
	return graph.NewFunctionNode("collect_activity", func(ctx context.Context, state graph.State) (graph.State, error) {
		msgCount := state.GetInt("message_count")

		// 基于消息数计算行为指标
		msgFrequency := float64(msgCount) / 7.0 // 平均每天消息数
		nightActivityRatio := 0.2                 // 夜间活跃度（简化）
		avgSessionDepth := 5.0                    // 平均会话深度（简化）

		if msgCount > 0 {
			avgSessionDepth = float64(state.GetInt("msg_count_user")) / float64(msgCount)
		}

		state.Set("msg_frequency", msgFrequency)
		state.Set("night_activity_ratio", nightActivityRatio)
		state.Set("avg_session_depth", avgSessionDepth)

		return state, nil
	})
}

// buildReactAnalysisNode 构建 ReAct 深度分析节点
// 使用 react.Loop 实现多轮反思
func buildReactAnalysisNode(
	llmCaller func(ctx context.Context, prompt string) (string, error),
	toolRegistry *tools.Registry,
) graph.Node {
	return graph.NewFunctionNode("phase2_react_analysis", func(ctx context.Context, state graph.State) (graph.State, error) {
		messagesText := state.GetString("messages_text")
		if messagesText == "" {
			// 无对话数据，使用默认值
			state.Set("emotion_stability", 70)
			state.Set("positive_ratio", 60)
			state.Set("negative_intensity", 40)
			state.Set("regulation_ability", 65)
			state.Set("dominant_emotion", "neutral")
			state.Set("emotion_summary", "暂无足够对话数据")
			return state, nil
		}

		// 初始化 ReAct 对话历史
		state.Set("react_messages", messagesText)
		state.Set("react_iteration", 0)
		state.Set("react_should_continue", true)

		// 执行 ReAct 循环
		finalState, err := react.Loop(ctx, llmCaller, toolRegistry, 5, state)
		if err != nil {
			// ReAct 失败，回退到简单分析
			state.Set("emotion_stability", 70)
			state.Set("positive_ratio", 60)
			state.Set("negative_intensity", 40)
			state.Set("regulation_ability", 65)
			state.Set("dominant_emotion", "neutral")
			state.Set("emotion_summary", "深度分析暂不可用")
			return state, nil
		}

		// 从 ReAct 结果提取情绪指标
		finalAnswer := finalState.GetString("react_final_answer")
		if finalAnswer == "" {
			finalAnswer = finalState.GetString("react_thought")
		}

		// 解析 ReAct 最终结论为结构化数据
		emotionData := parseReactResult(finalAnswer)
		state.Set("emotion_stability", emotionData.EmotionStability)
		state.Set("positive_ratio", emotionData.PositiveRatio)
		state.Set("negative_intensity", emotionData.NegativeIntensity)
		state.Set("regulation_ability", emotionData.RegulationAbility)
		state.Set("dominant_emotion", emotionData.DominantEmotion)
		state.Set("emotion_summary", emotionData.Summary)

		return state, nil
	})
}

// ReactEmotionResult ReAct 情绪解析结果
type ReactEmotionResult struct {
	EmotionStability  int    `json:"emotion_stability"`
	PositiveRatio     int    `json:"positive_ratio"`
	NegativeIntensity int    `json:"negative_intensity"`
	RegulationAbility int    `json:"regulation_ability"`
	DominantEmotion   string `json:"dominant_emotion"`
	Summary           string `json:"summary"`
}

// parseReactResult 解析 ReAct 最终结论
func parseReactResult(text string) *ReactEmotionResult {
	result := &ReactEmotionResult{
		EmotionStability:  70,
		PositiveRatio:     60,
		NegativeIntensity: 40,
		RegulationAbility: 65,
		DominantEmotion:   "neutral",
		Summary:           text,
	}

	// 尝试从文本中提取 JSON
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start == -1 || end == -1 || end <= start {
		return result
	}

	jsonStr := text[start : end+1]
	var parsed struct {
		EmotionStability  int    `json:"emotion_stability"`
		PositiveRatio     int    `json:"positive_ratio"`
		NegativeIntensity int    `json:"negative_intensity"`
		RegulationAbility int    `json:"regulation_ability"`
		DominantEmotion   string `json:"dominant_emotion"`
		Summary           string `json:"summary"`
	}

	if err := json.Unmarshal([]byte(jsonStr), &parsed); err == nil {
		if parsed.EmotionStability > 0 {
			result.EmotionStability = parsed.EmotionStability
		}
		if parsed.PositiveRatio > 0 {
			result.PositiveRatio = parsed.PositiveRatio
		}
		if parsed.NegativeIntensity > 0 {
			result.NegativeIntensity = parsed.NegativeIntensity
		}
		if parsed.RegulationAbility > 0 {
			result.RegulationAbility = parsed.RegulationAbility
		}
		if parsed.DominantEmotion != "" {
			result.DominantEmotion = parsed.DominantEmotion
		}
		if parsed.Summary != "" {
			result.Summary = parsed.Summary
		}
	}

	return result
}

// buildCalculateRiskNode 构建计算风险节点
func buildCalculateRiskNode() graph.Node {
	return graph.NewFunctionNode("calculate_risk", func(ctx context.Context, state graph.State) (graph.State, error) {
		// 获取维度分数
		scores := CalculateDimensionScores(
			state.GetInt("emotion_stability"),
			state.GetInt("positive_ratio"),
			state.GetInt("negative_intensity"),
			state.GetInt("regulation_ability"),
			state.GetInt("sds_score"),
			state.GetInt("sas_score"),
			state.GetFloat("msg_frequency"),
			state.GetFloat("night_activity_ratio"),
			state.GetFloat("avg_session_depth"),
		)

		// 计算综合风险
		overallScore, riskLevel, riskFactors := CalculateOverallScore(
			scores,
			state.GetString("text_risk_severity"),
		)

		// 写入 state
		state.Set("emotion_score", scores.Emotion)
		state.Set("depression_score", scores.Depression)
		state.Set("anxiety_score", scores.Anxiety)
		state.Set("stress_score", scores.Stress)
		state.Set("social_score", scores.Social)
		state.Set("overall_score", overallScore)
		state.Set("risk_level", riskLevel)
		state.Set("risk_factors", riskFactors)

		return state, nil
	})
}

// buildInterventionNode 构建干预建议节点
func buildInterventionNode(level string, llmCaller func(ctx context.Context, prompt string) (string, error)) graph.Node {
	return graph.NewFunctionNode(fmt.Sprintf("intervention_%s", level), func(ctx context.Context, state graph.State) (graph.State, error) {
		if llmCaller == nil {
			// 使用默认建议
			suggestions := getDefaultSuggestions(level)
			state.Set("suggestions", suggestions)
			state.Set("summary", getDefaultSummary(level))
			return state, nil
		}

		// 构建 prompt
		prompt := InterventionPrompt
		prompt = strings.ReplaceAll(prompt, "{{.OverallScore}}", fmt.Sprintf("%d", state.GetInt("overall_score")))
		prompt = strings.ReplaceAll(prompt, "{{.RiskLevel}}", state.GetString("risk_level"))
		prompt = strings.ReplaceAll(prompt, "{{.RiskFactors}}", strings.Join(state.GetStringSlice("risk_factors"), ", "))
		prompt = strings.ReplaceAll(prompt, "{{.EmotionScore}}", fmt.Sprintf("%d", state.GetInt("emotion_score")))
		prompt = strings.ReplaceAll(prompt, "{{.DepressionScore}}", fmt.Sprintf("%d", state.GetInt("depression_score")))
		prompt = strings.ReplaceAll(prompt, "{{.AnxietyScore}}", fmt.Sprintf("%d", state.GetInt("anxiety_score")))
		prompt = strings.ReplaceAll(prompt, "{{.StressScore}}", fmt.Sprintf("%d", state.GetInt("stress_score")))
		prompt = strings.ReplaceAll(prompt, "{{.SocialScore}}", fmt.Sprintf("%d", state.GetInt("social_score")))

		// 调用 LLM
		response, err := llmCaller(ctx, prompt)
		if err != nil {
			// 使用默认建议
			suggestions := getDefaultSuggestions(level)
			state.Set("suggestions", suggestions)
			state.Set("summary", getDefaultSummary(level))
			return state, nil
		}

		// 解析响应
		var result struct {
			Summary     string              `json:"summary"`
			Suggestions []models.Suggestion `json:"suggestions"`
		}

		if err := json.Unmarshal([]byte(extractJSON(response)), &result); err != nil {
			suggestions := getDefaultSuggestions(level)
			state.Set("suggestions", suggestions)
			state.Set("summary", getDefaultSummary(level))
			return state, nil
		}

		state.Set("summary", result.Summary)
		state.Set("suggestions", result.Suggestions)

		return state, nil
	})
}

// buildGenerateReportNode 构建生成报告节点
func buildGenerateReportNode() graph.Node {
	return graph.NewFunctionNode("generate_report", func(ctx context.Context, state graph.State) (graph.State, error) {
		// 生成警示标志
		warningFlags := []string{}
		if state.GetString("text_risk_severity") == "high" {
			warningFlags = append(warningFlags, "检测到高风险心理健康信号")
		}
		if state.GetInt("depression_score") < 30 {
			warningFlags = append(warningFlags, "抑郁风险较高")
		}
		if state.GetInt("anxiety_score") < 30 {
			warningFlags = append(warningFlags, "焦虑风险较高")
		}

		state.Set("warning_flags", warningFlags)

		return state, nil
	})
}

// extractJSON 从文本中提取 JSON
func extractJSON(text string) string {
	start := strings.Index(text, "{")
	end := strings.LastIndex(text, "}")
	if start != -1 && end != -1 && end > start {
		return text[start : end+1]
	}
	return text
}

// getDefaultSuggestions 获取默认建议
func getDefaultSuggestions(level string) []models.Suggestion {
	switch level {
	case "critical":
		return []models.Suggestion{
			{
				Level:    "immediate",
				Category: "professional",
				Title:    "立即寻求专业帮助",
				Content:  "你的状态让我们很担心，建议立即联系心理咨询师或拨打心理援助热线。",
				Actions:  []string{"拨打热线", "联系咨询师"},
			},
			{
				Level:    "immediate",
				Category: "self_help",
				Title:    "联系信任的人",
				Content:  "给信任的朋友或家人打个电话，告诉他们你的感受。",
				Actions:  []string{"联系亲友"},
			},
		}
	case "high":
		return []models.Suggestion{
			{
				Level:    "short_term",
				Category: "professional",
				Title:    "建议心理咨询",
				Content:  "建议预约心理咨询师进行专业评估和辅导。",
				Actions:  []string{"预约咨询"},
			},
			{
				Level:    "short_term",
				Category: "self_help",
				Title:    "深呼吸放松",
				Content:  "每天进行3次深呼吸练习，每次5分钟，帮助缓解焦虑。",
				Actions:  []string{"开始练习"},
			},
		}
	case "medium":
		return []models.Suggestion{
			{
				Level:    "short_term",
				Category: "self_help",
				Title:    "规律作息",
				Content:  "保持固定的睡眠时间，睡前1小时不使用手机。",
				Actions:  []string{"设置提醒"},
			},
			{
				Level:    "long_term",
				Category: "lifestyle",
				Title:    "适度运动",
				Content:  "每周进行3次有氧运动，每次30分钟。",
				Actions:  []string{"制定计划"},
			},
		}
	default: // low
		return []models.Suggestion{
			{
				Level:    "long_term",
				Category: "lifestyle",
				Title:    "保持积极心态",
				Content:  "你的状态不错，继续保持良好的生活习惯。",
				Actions:  []string{"记录心情"},
			},
		}
	}
}

// getDefaultSummary 获取默认摘要
func getDefaultSummary(level string) string {
	switch level {
	case "critical":
		return "你的状态让我们很担心，建议立即寻求专业帮助。"
	case "high":
		return "你最近的压力较大，建议多关注自己的心理健康。"
	case "medium":
		return "你最近有些情绪波动，适当放松会有所帮助。"
	default:
		return "你的状态不错，继续保持！"
	}
}
