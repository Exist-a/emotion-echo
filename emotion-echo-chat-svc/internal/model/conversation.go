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
	Pinned        bool       `gorm:"column:pinned;default:false"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt     time.Time  `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt     *time.Time `gorm:"column:deleted_at;index"`
	// CL-06（用户自定义 Agent·多角色）：本会话当前角色（迁移 c012）。
	// 跨 schema 引用 user_agent_profile.id，**故意无外键**（角色归 user-svc）；
	// NULL = 未显式选择 ⇒ 消费方回落用户默认角色。
	AgentID *int64 `gorm:"column:agent_id"`
}

// TableName 显式指定 schema + 表名
func (Conversation) TableName() string { return "emotion_echo_chat.conversations" }

// Message 消息（emotion_echo_chat.messages 表对应）
//
// ClientMsgID（Stage 33 PR-18）：客户端生成的幂等键，用于网络重试场景下的
// "同一请求多次发送只落库一次"。partial UNIQUE INDEX 仅在该字段非 NULL
// 时生效（`uq_messages_client_msg_id WHERE client_msg_id IS NOT NULL`）。
type Message struct {
	ID             int64      `gorm:"column:id;primaryKey;autoIncrement"`
	ConversationID int64      `gorm:"column:conversation_id;index"`
	UserID         int64      `gorm:"column:user_id"`
	Role           string     `gorm:"column:role;size:16"`
	Content        string     `gorm:"column:content"`
	ContentType    string     `gorm:"column:content_type;size:16;default:text"`
	// Stage 82 PR-3b：6 类消息意图（§契约 5：allowedIntents 白名单校验，非法落 ''）
	// Stage 89 PR-6（c007_widen_intent_to_32）：DDL 由 VARCHAR(16) 扩到 VARCHAR(32)，
	// tag 必须同步（§契约 5「DDL ↔ model」一致；由 intent_contract5_test.go 钉住）。
	Intent         string     `gorm:"column:intent;size:32;default:''"`
	// Stage 89 PR-2：文件消息原始文件名（migration 006；§契约 5：写入端超 255 截断）
	FileName       string     `gorm:"column:file_name;size:255;default:''"`
	TokensUsed     int        `gorm:"column:tokens_used;default:0"`
	ClientMsgID    *string    `gorm:"column:client_msg_id;uniqueIndex:uq_messages_client_msg_id,where:client_msg_id IS NOT NULL"`
	CreatedAt      time.Time  `gorm:"column:created_at;autoCreateTime"`
	DeletedAt      *time.Time `gorm:"column:deleted_at;index"`
	// CL-06：该 AI 消息由哪个角色产生（迁移 c013；**快照**语义，角色改名/删除不影响历史）。
	// 仅 assistant 消息有值；NULL = 未记录（用户消息 / 角色功能上线前的历史行）。
	AgentID *int64 `gorm:"column:agent_id"`
}

// TableName 显式指定 schema + 表名
func (Message) TableName() string { return "emotion_echo_chat.messages" }