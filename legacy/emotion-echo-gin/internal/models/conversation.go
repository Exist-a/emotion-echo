package models

import "time"

// Conversation 会话模型
type Conversation struct {
	ID                 string     `gorm:"primaryKey;size:32" json:"id"`
	UserID             int64      `gorm:"index;not null" json:"userId,string"`
	Title              string     `gorm:"size:200;not null" json:"title"`
	IsTop              bool       `gorm:"column:is_top;default:false" json:"isTop"`
	LastMessageContent string     `gorm:"column:last_message_content;type:text" json:"lastMessage,omitempty"`
	LastMessageTime    *int64     `gorm:"column:last_message_time" json:"lastMessageTime,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

// TableName 表名
func (Conversation) TableName() string {
	return "conversations"
}
