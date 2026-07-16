package service

import (
	"context"
	"time"

	"emotion-echo-gin/internal/repository"
)

// GetInteractionDepth 获取互动深度（全部历史）
func GetInteractionDepth(ctx context.Context, userID int64, msgRepo *repository.MessageRepository, convRepo *repository.ConversationRepository) (*InteractionDepth, error) {
	// 获取用户所有会话
	convs, err := convRepo.ListByUserID(ctx, userID, 1000, "")
	if err != nil {
		return nil, err
	}

	totalConversations := len(convs)
	totalMessages := 0
	totalRounds := 0

	// 按日期统计消息数（用于计算连续天数）
	dailyCount := make(map[string]int)
	firstDate := time.Now()
	lastDate := time.Time{}

	for _, conv := range convs {
		msgs, err := msgRepo.ListByConversationID(ctx, conv.ID, 1000, 0)
		if err != nil {
			continue
		}

		// 计算会话轮数（用户消息数）
		userMsgs := 0
		for _, msg := range msgs {
			if msg.Sender == "user" {
				userMsgs++
				totalMessages++
				t := time.UnixMilli(msg.SendTime)
				dateStr := t.Format("2006-01-02")
				dailyCount[dateStr]++

				if t.Before(firstDate) {
					firstDate = t
				}
				if t.After(lastDate) {
					lastDate = t
				}
			}
		}
		totalRounds += userMsgs
	}

	avgRounds := 0.0
	if totalConversations > 0 {
		avgRounds = float64(totalRounds) / float64(totalConversations)
	}

	// 计算最长连续对话天数
	maxConsecutive := 0
	currentConsecutive := 0
	if len(dailyCount) > 0 {
		for d := firstDate; !d.After(lastDate); d = d.AddDate(0, 0, 1) {
			dateStr := d.Format("2006-01-02")
			if dailyCount[dateStr] > 0 {
				currentConsecutive++
				if currentConsecutive > maxConsecutive {
					maxConsecutive = currentConsecutive
				}
			} else {
				currentConsecutive = 0
			}
		}
	}

	// 计算日均消息数
	days := int(lastDate.Sub(firstDate).Hours()/24) + 1
	avgPerDay := 0.0
	if days > 0 {
		avgPerDay = float64(totalMessages) / float64(days)
	}

	return &InteractionDepth{
		AvgSessionRounds:   avgRounds,
		MaxConsecutiveDays: maxConsecutive,
		TotalConversations: totalConversations,
		TotalMessages:      totalMessages,
		AvgMessagesPerDay:  avgPerDay,
	}, nil
}
