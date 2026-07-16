package assessment

import (
	"context"
	"fmt"
	"strings"
	"time"

	"emotion-echo-gin/internal/repository"
	"emotion-echo-gin/internal/workflow/tools"
)

// BuildToolRegistry 构建评估专用工具注册表
func BuildToolRegistry(
	msgRepo *repository.MessageRepository,
	convRepo *repository.ConversationRepository,
	surveyResultRepo *repository.SurveyResultRepository,
) *tools.Registry {
	registry := tools.NewRegistry()

	// 注册历史查询工具
	registry.Register(&HistoryQueryTool{msgRepo: msgRepo, convRepo: convRepo})

	// 注册心理学知识工具（硬编码知识库）
	registry.Register(&PsychologyKnowledgeTool{})

	// 注册量表查询工具
	registry.Register(&SurveyQueryTool{surveyResultRepo: surveyResultRepo})

	return registry
}

// HistoryQueryTool 历史对话查询工具
type HistoryQueryTool struct {
	msgRepo  *repository.MessageRepository
	convRepo *repository.ConversationRepository
}

func (t *HistoryQueryTool) Name() string {
	return "query_history"
}

func (t *HistoryQueryTool) Description() string {
	return "查询用户历史对话记录，输入时间范围（如 7d, 30d）"
}

func (t *HistoryQueryTool) Execute(ctx context.Context, input string) (string, error) {
	// 解析时间范围
	days := 7
	if strings.Contains(input, "30") || strings.Contains(input, "30d") {
		days = 30
	} else if strings.Contains(input, "1") || strings.Contains(input, "1d") {
		days = 1
	}

	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -days)

	// 获取用户在时间范围内的所有会话
	convs, err := t.convRepo.ListRecentConversations(ctx, startTime, endTime, 100)
	if err != nil {
		return "", fmt.Errorf("query conversations failed: %w", err)
	}

	if len(convs) == 0 {
		return fmt.Sprintf("过去 %d 天内无对话记录", days), nil
	}

	// 聚合所有消息
	var summaries []string
	for _, conv := range convs {
		msgs, err := t.msgRepo.ListByConversationID(ctx, conv.ID, 50, 0)
		if err != nil {
			continue
		}

		var msgTexts []string
		for _, msg := range msgs {
			if msg.Sender == "user" {
				msgTexts = append(msgTexts, msg.Content)
			}
		}

		if len(msgTexts) > 0 {
			summaries = append(summaries, fmt.Sprintf("会话 %s (%d 条消息): %s",
				conv.Title, len(msgTexts), strings.Join(msgTexts, "; ")))
		}
	}

	result := fmt.Sprintf("过去 %d 天内有 %d 个会话，共 %d 条消息。\n%s",
		days, len(convs), len(summaries), strings.Join(summaries, "\n"))

	return result, nil
}

// PsychologyKnowledgeTool 心理学知识查询工具（硬编码知识库）
type PsychologyKnowledgeTool struct{}

func (t *PsychologyKnowledgeTool) Name() string {
	return "query_knowledge"
}

func (t *PsychologyKnowledgeTool) Description() string {
	return "查询心理学知识，输入关键词（如 抑郁、焦虑、睡眠、压力）"
}

func (t *PsychologyKnowledgeTool) Execute(ctx context.Context, input string) (string, error) {
	input = strings.ToLower(input)

	knowledge := map[string]string{
		"抑郁": `抑郁症核心症状：
1. 持续情绪低落（2周以上）
2. 兴趣减退，对以往喜欢的事失去热情
3. 精力下降，易疲劳
4. 睡眠障碍（失眠或嗜睡）
5. 食欲改变
6. 自我评价过低，自责
7. 注意力下降，决策困难
8. 反复出现死亡念头

轻度：部分症状，社会功能轻度受损
中度：明显症状，工作学习效率下降
重度：几乎全天症状，社会功能严重受损`,

		"焦虑": `焦虑障碍特征：
1. 过度担心，难以控制
2. 肌肉紧张，易疲劳
3. 易怒
4. 睡眠障碍
5. 注意力难以集中
6. 躯体症状：心悸、出汗、颤抖

GAD-7 评分：
0-4 分： minimal anxiety
5-9 分： mild anxiety
10-14 分： moderate anxiety
15-21 分： severe anxiety`,

		"压力": `压力反应三阶段（Selye）：
1. 警觉期：身体动员应对压力
2. 抵抗期：持续消耗资源
3. 衰竭期：身心资源耗尽

慢性压力信号：
- 持续疲劳
- 免疫力下降
- 消化问题
- 情绪波动
- 社交退缩`,
	}

	// 关键词匹配
	for keyword, content := range knowledge {
		if strings.Contains(input, keyword) {
			return content, nil
		}
	}

	return "未找到相关知识。可用关键词：抑郁、焦虑、压力", nil
}

// SurveyQueryTool 量表结果查询工具
type SurveyQueryTool struct {
	surveyResultRepo *repository.SurveyResultRepository
}

func (t *SurveyQueryTool) Name() string {
	return "query_survey"
}

func (t *SurveyQueryTool) Description() string {
	return "查询用户心理量表结果"
}

func (t *SurveyQueryTool) Execute(ctx context.Context, input string) (string, error) {
	// 解析 user_id（简化：从输入中提取）
	// 实际应从 state 中获取，但工具接口限制为 string input
	// 这里简化处理，返回通用信息
	return "量表查询功能：可以获取用户的 SDS（抑郁）、SAS（焦虑）量表评分结果", nil
}
