// Package model 定义 analytics-svc 拥有的领域实体。
package model

import "time"

// UserBehaviorEvent 用户行为事件
//
// EventID 用于消费幂等（Stage 30-C A1）：来自 chat-svc 发布的 Kafka 事件 ID。
// 写入数据库时由 repo 层 ON CONFLICT (event_id) DO NOTHING 去重，
// 保证 at-least-once 投递下同一事件不会落两行。
type UserBehaviorEvent struct {
	ID         int64     `gorm:"column:id;primaryKey;autoIncrement"`
	EventID    string    `gorm:"column:event_id;size:64;uniqueIndex:uq_user_behavior_events_event_id"`
	UserID     int64     `gorm:"column:user_id;index"`
	EventType  string    `gorm:"column:event_type;size:64"`
	Target     string    `gorm:"column:target;size:255"`
	SessionID  string    `gorm:"column:session_id;size:64"`
	OccurredAt time.Time `gorm:"column:occurred_at;autoCreateTime"`
}

func (UserBehaviorEvent) TableName() string { return "emotion_echo_analytics.user_behavior_events" }