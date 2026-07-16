// Package model 定义 chat-svc 拥有的领域实体。
//
// 这些 struct 对应 emotion_echo_chat schema 中的表。
// 跨服务查询必须通过 RPC，禁止跨 schema JOIN。
//
// ID 类型选择：全部用 int64 而非 uint64，因为：
//   - GORM 用 int64 自增更自然（虽然 DB 是 BIGSERIAL）
//   - 与 user_id（int64）保持一致，避免类型转换
//   - json/对外 API 用 int64 更常见
package model

import "time"

// Conversation 会话（emotion_echo_chat.conversations 表对应）
type Conversation struct {
	ID            int64      `gorm:"column:id;primaryKey;autoIncrement"`
	UserID        int64      `gorm:"column:user_id;index"`
	Title         string     `gorm:"column:title;size:255"`
	MessageCount  int        `gorm:"column:message_count;default:0"`
	LastMessageAt *time.Time `gorm:"column:last_message_at"`
	Status        int16      `gorm:"column:status;default:1"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 显式指定 schema + 表名
func (Conversation) TableName() string { return "emotion_echo_chat.conversations" }

// Message 消息（emotion_echo_chat.messages 表对应）
type Message struct {
	ID             int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ConversationID int64     `gorm:"column:conversation_id;index"`
	UserID         int64     `gorm:"column:user_id"`
	Role           string    `gorm:"column:role;size:16"`
	Content        string    `gorm:"column:content"`
	ContentType    string    `gorm:"column:content_type;size:16;default:text"`
	TokensUsed     int       `gorm:"column:tokens_used;default:0"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
}

// TableName 显式指定 schema + 表名
func (Message) TableName() string { return "emotion_echo_chat.messages" }