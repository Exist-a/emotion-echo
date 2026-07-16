// Package model 定义 analytics-svc 拥有的领域实体。
package model

import "time"

// UserBehaviorEvent 用户行为事件
type UserBehaviorEvent struct {
	ID         int64     `gorm:"column:id;primaryKey;autoIncrement"`
	UserID     int64     `gorm:"column:user_id;index"`
	EventType  string    `gorm:"column:event_type;size:64"`
	Target     string    `gorm:"column:target;size:255"`
	SessionID  string    `gorm:"column:session_id;size:64"`
	OccurredAt time.Time `gorm:"column:occurred_at;autoCreateTime"`
}

func (UserBehaviorEvent) TableName() string { return "emotion_echo_analytics.user_behavior_events" }