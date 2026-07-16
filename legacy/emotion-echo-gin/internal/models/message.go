package models

// Message 消息模型
type Message struct {
	ID             string  `gorm:"primaryKey;size:32" json:"id"`
	ConversationID string  `gorm:"index;size:32;not null" json:"conversationId"`
	Sender         string  `gorm:"size:10;not null" json:"sender"`
	Content        string  `gorm:"type:text" json:"content"`
	ContentType    string  `gorm:"size:10;default:'text'" json:"contentType"`
	EmotionTag     *string `gorm:"size:10" json:"emotionTag,omitempty"`
	IntentType     string  `gorm:"size:50;default:'other'" json:"intentType"`
	AudioURL       string  `gorm:"size:500" json:"audioUrl,omitempty"`
	AudioDuration  int     `gorm:"default:0" json:"audioDuration,omitempty"`
	SendTime       int64   `gorm:"index;not null" json:"sendTime"`
	CreatedAt      int64   `gorm:"autoCreateTime" json:"createdAt"`
}

// TableName 表名
func (Message) TableName() string {
	return "messages"
}
