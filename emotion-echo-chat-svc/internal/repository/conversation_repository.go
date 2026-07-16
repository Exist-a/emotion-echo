// Package repository 定义 chat-svc 的数据访问层
//
// 核心约束：
//   - chat-svc 拥有 emotion_echo_chat schema 中的所有表
//   - 跨域查询（users/conversations）必须通过 RPC，禁止跨 schema JOIN
//   - 所有 DB 操作必须接受 ctx，便于 trace/cancel 传递
package repository

import (
	"context"
	"errors"
	"sync"

	"emotion-echo-chat-svc/internal/model"

	"gorm.io/gorm"
)

// ErrNotFound 在资源不存在时返回，业务层可 errors.Is 判定
var ErrNotFound = errors.New("chat: resource not found")

// ConversationRepo 会话仓储接口
//
// 实现：
//   - InMemoryConversationRepo：测试替身，零外部依赖
//   - PostgresConversationRepo：生产实现，连 emotion_echo_chat schema
type ConversationRepo interface {
	// CreateConversation 新建会话
	CreateConversation(ctx context.Context, c *model.Conversation) error
	// GetConversationByID 按 ID 查会话；不存在返回 nil, nil（不视为错误）
	GetConversationByID(ctx context.Context, id int64) (*model.Conversation, error)
	// IncrementMessageCount 原子地增加 msg_count + 更新 last_message_at
	IncrementMessageCount(ctx context.Context, conversationID int64) error
	// AppendMessage 追加一条消息
	AppendMessage(ctx context.Context, m *model.Message) error
	// ListMessages 列出会话的消息
	ListMessages(ctx context.Context, conversationID int64, limit int) ([]model.Message, error)
	// Ping 健康检查
	Ping(ctx context.Context) error
}

// =====================================================
// InMemoryConversationRepo（测试替身）
// =====================================================

// InMemoryConversationRepo 是 ConversationRepo 的内存实现
//
// 并发安全：使用 sync.RWMutex
// 数据存于两个 map：conversations / messages
type InMemoryConversationRepo struct {
	mu           sync.RWMutex
	conversations map[int64]*model.Conversation
	messages     map[int64]*model.Message
	nextConvID   int64
	nextMsgID    int64
}

// NewInMemoryConversationRepo 构造空仓库
func NewInMemoryConversationRepo() *InMemoryConversationRepo {
	return &InMemoryConversationRepo{
		conversations: make(map[int64]*model.Conversation),
		messages:      make(map[int64]*model.Message),
		nextConvID:    1,
		nextMsgID:     1,
	}
}

func (r *InMemoryConversationRepo) CreateConversation(ctx context.Context, c *model.Conversation) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c.ID == 0 {
		c.ID = r.nextConvID
		r.nextConvID++
	}
	r.conversations[c.ID] = c
	return nil
}

func (r *InMemoryConversationRepo) GetConversationByID(ctx context.Context, id int64) (*model.Conversation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.conversations[id]
	if !ok {
		return nil, nil // 约定：不存在返回 nil, nil
	}
	return c, nil
}

func (r *InMemoryConversationRepo) IncrementMessageCount(ctx context.Context, conversationID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.conversations[conversationID]; ok {
		c.MessageCount++
	}
	return nil
}

func (r *InMemoryConversationRepo) AppendMessage(ctx context.Context, m *model.Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if m.ID == 0 {
		m.ID = r.nextMsgID
		r.nextMsgID++
	}
	r.messages[m.ID] = m
	return nil
}

func (r *InMemoryConversationRepo) ListMessages(ctx context.Context, conversationID int64, limit int) ([]model.Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.Message, 0)
	for _, m := range r.messages {
		if m.ConversationID == conversationID {
			out = append(out, *m)
		}
	}
	return out, nil
}

func (r *InMemoryConversationRepo) Ping(ctx context.Context) error { return nil }

// =====================================================
// PostgresConversationRepo（生产实现）
// =====================================================

// PostgresConversationRepo 是 ConversationRepo 的 GORM/Postgres 实现
//
// 表归属：emotion_echo_chat.conversations / emotion_echo_chat.messages
type PostgresConversationRepo struct {
	db *gorm.DB
}

// NewPostgresConversationRepo 构造生产仓库
func NewPostgresConversationRepo(db *gorm.DB) *PostgresConversationRepo {
	return &PostgresConversationRepo{db: db}
}

func (r *PostgresConversationRepo) CreateConversation(ctx context.Context, c *model.Conversation) error {
	// 让 DB 自增 ID：先清零，让 GORM 接管
	c.ID = 0
	return r.db.WithContext(ctx).Create(c).Error
}

func (r *PostgresConversationRepo) GetConversationByID(ctx context.Context, id int64) (*model.Conversation, error) {
	var c model.Conversation
	err := r.db.WithContext(ctx).First(&c, id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &c, nil
}

func (r *PostgresConversationRepo) IncrementMessageCount(ctx context.Context, conversationID int64) error {
	// 用 SQL 而非 ORM，避免 GORM 默认 update 不带 WHERE 0=0 之类的坑
	return r.db.WithContext(ctx).
		Exec(`UPDATE emotion_echo_chat.conversations
		      SET message_count = message_count + 1,
		          last_message_at = NOW(),
		          updated_at = NOW()
		      WHERE id = ?`, conversationID).Error
}

func (r *PostgresConversationRepo) AppendMessage(ctx context.Context, m *model.Message) error {
	m.ID = 0
	return r.db.WithContext(ctx).Create(m).Error
}

func (r *PostgresConversationRepo) ListMessages(ctx context.Context, conversationID int64, limit int) ([]model.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	var out []model.Message
	err := r.db.WithContext(ctx).
		Where("conversation_id = ?", conversationID).
		Order("id ASC").
		Limit(limit).
		Find(&out).Error
	return out, err
}

func (r *PostgresConversationRepo) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}