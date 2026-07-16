package service

import (
	"context"
	"time"

	"emotion-echo-gin/internal/repository"
)

// GetFrequencyTrend 获取对话频次趋势（最近30天）
func GetFrequencyTrend(ctx context.Context, userID int64, msgRepo *repository.MessageRepository, convRepo *repository.ConversationRepository) (*FrequencyTrend, error) {
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -29) // 最近30天

	// 生成日期列表
	dates := make([]string, 30)
	messageCount := make([]int, 30)
	for i := 0; i < 30; i++ {
		d := startTime.AddDate(0, 0, i)
		dates[i] = d.Format("01-02")
	}

	// 获取用户会话
	convs, err := convRepo.ListRecentConversations(ctx, startTime, endTime, 1000)
	if err != nil {
		return nil, err
	}

	// 统计每日消息数
	for _, conv := range convs {
		if conv.UserID != userID {
			continue
		}
		msgs, err := msgRepo.ListByConversationID(ctx, conv.ID, 1000, 0)
		if err != nil {
			continue
		}
		for _, msg := range msgs {
			if msg.Sender != "user" {
				continue
			}
			t := time.UnixMilli(msg.SendTime)
			dayIndex := int(t.Sub(startTime).Hours() / 24)
			if dayIndex >= 0 && dayIndex < 30 {
				messageCount[dayIndex]++
			}
		}
	}

	return &FrequencyTrend{
		Dates:        dates,
		MessageCount: messageCount,
	}, nil
}
