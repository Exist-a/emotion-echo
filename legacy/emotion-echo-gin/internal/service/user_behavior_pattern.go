package service

import (
	"context"
	"time"

	"emotion-echo-gin/internal/repository"
)

// GetDayNightPattern 获取昼夜使用模式（最近30天）
func GetDayNightPattern(ctx context.Context, userID int64, msgRepo *repository.MessageRepository, convRepo *repository.ConversationRepository) (*DayNightPattern, error) {
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -30)

	// 获取用户所有会话
	convs, err := convRepo.ListRecentConversations(ctx, startTime, endTime, 1000)
	if err != nil {
		return nil, err
	}

	// 统计各时段消息数
	periods := map[string]int{
		"凌晨": 0, // 00:00-06:00
		"上午": 0, // 06:00-12:00
		"下午": 0, // 12:00-18:00
		"晚上": 0, // 18:00-24:00
	}

	total := 0
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
			hour := t.Hour()
			switch {
			case hour >= 0 && hour < 6:
				periods["凌晨"]++
			case hour >= 6 && hour < 12:
				periods["上午"]++
			case hour >= 12 && hour < 18:
				periods["下午"]++
			default:
				periods["晚上"]++
			}
			total++
		}
	}

	result := &DayNightPattern{}
	for label, count := range periods {
		ratio := 0
		if total > 0 {
			ratio = int(float64(count) / float64(total) * 100)
		}
		var hours string
		switch label {
		case "凌晨":
			hours = "00:00-06:00"
		case "上午":
			hours = "06:00-12:00"
		case "下午":
			hours = "12:00-18:00"
		case "晚上":
			hours = "18:00-24:00"
		}
		result.Periods = append(result.Periods, PeriodStat{
			Label: label,
			Hours: hours,
			Value: count,
			Ratio: ratio,
		})
	}

	return result, nil
}
