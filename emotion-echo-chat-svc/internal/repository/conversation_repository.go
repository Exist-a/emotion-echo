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
	"sort"
	"sync"
	"time"

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
	// ListMessagesPage 游标分页列消息（2026-10-13）：返回"最新 limit 条"，
	// 传入 cursor>0 时返回 id < cursor 的更旧消息。返回升序（时间正序）切片 +
	// 是否还有更旧消息（用于前端上翻加载历史）。
	ListMessagesPage(ctx context.Context, conversationID int64, limit int, cursor int64) ([]model.Message, bool, error)
	// GetMessageByClientMsgID Stage 33 PR-18：按 client_msg_id 查消息（幂等查重）。
	// userID 参与限定避免跨用户冲突；conversationID 进一步限定到指定会话。
	// 命中返回 *Message，未命中返回 nil, nil（约定与 GetConversationByID 一致）。
	GetMessageByClientMsgID(ctx context.Context, userID, conversationID int64, clientMsgID string) (*model.Message, error)
	// DeleteConversation 删除会话（级联删除其消息）
	DeleteConversation(ctx context.Context, id int64) error
	// Ping 健康检查
	Ping(ctx context.Context) error

	// Stage 36-A2.1：按用户列出最近会话，按 updated_at desc 排序。
	// limit<=0 视为 20，offset<0 视为 0。
	ListConversations(ctx context.Context, userID int64, limit, offset int) ([]model.Conversation, error)

	// ListConversationsPage CL-05 Phase 5（2026-10-16）：会话列表 **keyset 游标分页**。
	//
	// 稳定序 = `updated_at DESC, id DESC`。游标是**复合键** `(cursorUpdatedAt, cursorID)`：
	// 返回**严格早于**该键的下一页。为什么不能只用 updated_at —— 同一时刻（同一事务
	// NOW() / 种子数据）更新的多个会话会在翻页时被整批跳过（`updated_at < cursor` 恒假）。
	//
	// cursorUpdatedAt.IsZero() 且 cursorID==0 ⇒ 首页（从最新开始）。
	// limit<=0 视为 20。返回本页 + 是否还有更多（内部 limit+1 探测，末位丢弃）。
	ListConversationsPage(ctx context.Context, userID int64, limit int, cursorUpdatedAt time.Time, cursorID int64) ([]model.Conversation, bool, error)

	// Stage 72：置顶/取消置顶会话（PinConversation RPC）；同时刷新 updated_at
	SetPinned(ctx context.Context, id int64, pinned bool) error
	// Stage 72：更新会话标题（UpdateConversation RPC）；同时刷新 updated_at
	UpdateTitle(ctx context.Context, id int64, title string) error
	// UpdateAgentID CL-06：把会话的当前角色改为 agentID（agentID<=0 视为不合法，调用方已校验）。
	// 仅对未删除的会话生效（与 UpdateTitle 同款语义）。
	UpdateAgentID(ctx context.Context, id int64, agentID int64) error

	// Stage 30-C A3: 事务版本（接受可选 tx，nil = 非事务）
	CreateConversationTx(tx *gorm.DB, ctx context.Context, c *model.Conversation) error
	AppendMessageTx(tx *gorm.DB, ctx context.Context, m *model.Message) error
	DeleteConversationTx(tx *gorm.DB, ctx context.Context, id int64) error
	IncrementMessageCountTx(tx *gorm.DB, ctx context.Context, conversationID int64) error
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
	if !ok || c.DeletedAt != nil {
		return nil, nil // 约定：不存在或已软删除返回 nil, nil
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

// ListMessagesPage 内存版游标分页（与 Postgres 版语义一致）。
func (r *InMemoryConversationRepo) ListMessagesPage(ctx context.Context, conversationID int64, limit int, cursor int64) ([]model.Message, bool, error) {
	if limit <= 0 {
		limit = 50
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []model.Message
	for _, m := range r.messages {
		if m.ConversationID == conversationID && m.DeletedAt == nil && (cursor == 0 || m.ID < cursor) {
			out = append(out, *m)
		}
	}
	// id DESC（最新优先）后取 limit+1 探测 hasMore
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, hasMore, nil
}

// GetMessageByClientMsgID Stage 33 PR-18：内存版按 (user_id, conversation_id, client_msg_id) 查重
func (r *InMemoryConversationRepo) GetMessageByClientMsgID(ctx context.Context, userID, conversationID int64, clientMsgID string) (*model.Message, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, m := range r.messages {
		if m.UserID == userID && m.ConversationID == conversationID && m.ClientMsgID != nil && *m.ClientMsgID == clientMsgID {
			return m, nil
		}
	}
	return nil, nil
}

// SetPinned Stage 72：内存版置顶/取消置顶（不存在或已删除的 id 为 no-op）
func (r *InMemoryConversationRepo) SetPinned(ctx context.Context, id int64, pinned bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.conversations[id]; ok && c.DeletedAt == nil {
		c.Pinned = pinned
		c.UpdatedAt = time.Now()
	}
	return nil
}

// UpdateTitle Stage 72：内存版改标题（不存在或已删除的 id 为 no-op）
func (r *InMemoryConversationRepo) UpdateTitle(ctx context.Context, id int64, title string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.conversations[id]; ok && c.DeletedAt == nil {
		c.Title = title
		c.UpdatedAt = time.Now()
	}
	return nil
}

// UpdateAgentID CL-06：内存版切角色（不存在/已删除的 id 为 no-op，与 UpdateTitle 同款）
func (r *InMemoryConversationRepo) UpdateAgentID(ctx context.Context, id int64, agentID int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.conversations[id]; ok && c.DeletedAt == nil {
		v := agentID
		c.AgentID = &v
		c.UpdatedAt = time.Now()
	}
	return nil
}

func (r *InMemoryConversationRepo) Ping(ctx context.Context) error { return nil }

// CreateConversationTx InMemory 实现：tx 参数忽略（InMemory 无事务概念）
func (r *InMemoryConversationRepo) CreateConversationTx(_ *gorm.DB, ctx context.Context, c *model.Conversation) error {
	return r.CreateConversation(ctx, c)
}

func (r *InMemoryConversationRepo) AppendMessageTx(_ *gorm.DB, ctx context.Context, m *model.Message) error {
	return r.AppendMessage(ctx, m)
}

func (r *InMemoryConversationRepo) DeleteConversationTx(_ *gorm.DB, ctx context.Context, id int64) error {
	return r.DeleteConversation(ctx, id)
}

func (r *InMemoryConversationRepo) IncrementMessageCountTx(_ *gorm.DB, ctx context.Context, conversationID int64) error {
	return r.IncrementMessageCount(ctx, conversationID)
}

// DeleteConversation 软删除会话 + 级联软删除其消息（不存在的 id 为 no-op）。
// 与 PostgresConversationRepo 保持一致：设 deleted_at 而非从 map 中移除。
func (r *InMemoryConversationRepo) DeleteConversation(ctx context.Context, id int64) error {
	now := time.Now()
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.conversations[id]; ok {
		c.DeletedAt = &now
	}
	for _, m := range r.messages {
		if m.ConversationID == id && m.DeletedAt == nil {
			m.DeletedAt = &now
		}
	}
	return nil
}

// ListConversations Stage 36-A2.1：InMemory 版按 updated_at desc 列出该 user 的会话。
// limit<=0 → 20；offset<0 → 0。
func (r *InMemoryConversationRepo) ListConversations(ctx context.Context, userID int64, limit, offset int) ([]model.Conversation, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.Conversation, 0, limit+1)
	for _, c := range r.conversations {
		if c.UserID != userID || c.DeletedAt != nil {
			continue
		}
		out = append(out, *c)
	}
	// updated_at desc（updated_at 相同则 id desc 兜底，保证稳定排序）
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	// offset 截断
	if offset >= len(out) {
		return []model.Conversation{}, nil
	}
	out = out[offset:]
	// limit 截断
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// ListConversationsPage CL-05 Phase 5：内存版 keyset 分页（与 Postgres 版语义一致）。
// 游标是复合键 (updated_at, id)：只有**严格早于**它的行才属于下一页。
func (r *InMemoryConversationRepo) ListConversationsPage(ctx context.Context, userID int64, limit int, cursorUpdatedAt time.Time, cursorID int64) ([]model.Conversation, bool, error) {
	if limit <= 0 {
		limit = 20
	}
	hasCursor := !cursorUpdatedAt.IsZero() || cursorID != 0

	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]model.Conversation, 0, limit+1)
	for _, c := range r.conversations {
		if c.UserID != userID || c.DeletedAt != nil {
			continue
		}
		if hasCursor && !conversationBeforeCursor(*c, cursorUpdatedAt, cursorID) {
			continue
		}
		out = append(out, *c)
	}
	// updated_at desc，同刻 id desc 兜底（与 ListConversations 同一比较器）
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})

	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	return out, hasMore, nil
}

// conversationBeforeCursor 判定会话 c 是否**严格早于**复合游标 (cursorAt, cursorID)。
// 序是 updated_at DESC, id DESC ⇒ "更早" = updated_at 更小，或 updated_at 相同而 id 更小。
// 这一条 tie-break 就是"同一时刻更新的多个会话不会被跳过"的全部依据。
func conversationBeforeCursor(c model.Conversation, cursorAt time.Time, cursorID int64) bool {
	if c.UpdatedAt.Before(cursorAt) {
		return true
	}
	return c.UpdatedAt.Equal(cursorAt) && c.ID < cursorID
}

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
	err := r.db.WithContext(ctx).
		Where("deleted_at IS NULL").
		First(&c, id).Error
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

// ListMessagesPage 2026-10-13：游标分页，默认取最新 limit 条（id DESC），
// cursor>0 时取 id < cursor 的更旧消息；取 limit+1 探测 hasMore；最后反转为升序。
func (r *PostgresConversationRepo) ListMessagesPage(ctx context.Context, conversationID int64, limit int, cursor int64) ([]model.Message, bool, error) {
	if limit <= 0 {
		limit = 50
	}
	q := r.db.WithContext(ctx).
		Where("conversation_id = ? AND deleted_at IS NULL", conversationID)
	if cursor > 0 {
		q = q.Where("id < ?", cursor)
	}
	var out []model.Message
	if err := q.Order("id DESC").Limit(limit + 1).Find(&out).Error; err != nil {
		return nil, false, err
	}
	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	// 反转成时间升序，供前端直接渲染（最新一条在末尾）
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, hasMore, nil
}

// GetMessageByClientMsgID Stage 33 PR-18：Postgres 版按 (user_id, conversation_id, client_msg_id) 查重
func (r *PostgresConversationRepo) GetMessageByClientMsgID(ctx context.Context, userID, conversationID int64, clientMsgID string) (*model.Message, error) {
	var m model.Message
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND conversation_id = ? AND client_msg_id = ?", userID, conversationID, clientMsgID).
		First(&m).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &m, nil
}

// DeleteConversation 软删除会话 + 其消息（E2E-06：物理 DELETE → UPDATE deleted_at）
func (r *PostgresConversationRepo) DeleteConversation(ctx context.Context, id int64) error {
	now := time.Now()
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 软删除消息
		if err := tx.Model(&model.Message{}).
			Where("conversation_id = ? AND deleted_at IS NULL", id).
			Update("deleted_at", now).Error; err != nil {
			return err
		}
		// 软删除会话
		return tx.Model(&model.Conversation{}).
			Where("id = ? AND deleted_at IS NULL", id).
			Update("deleted_at", now).Error
	})
}

func (r *PostgresConversationRepo) Ping(ctx context.Context) error {
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// ListConversations Stage 36-A2.1：Postgres 版按 user_id 过滤 + updated_at desc + id desc 兜底。
// limit<=0 → 20；offset<0 → 0。schema 限定 emotion_echo_chat.conversations。
// E2E-06: 过滤已软删除的会话
func (r *PostgresConversationRepo) ListConversations(ctx context.Context, userID int64, limit, offset int) ([]model.Conversation, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	var out []model.Conversation
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND deleted_at IS NULL", userID).
		Order("updated_at DESC, id DESC").
		Limit(limit).
		Offset(offset).
		Find(&out).Error
	return out, err
}

// ListConversationsPage CL-05 Phase 5：Postgres 版 keyset 分页（**复合游标**）。
//
// `(updated_at, id) < (cursor_at, cursor_id)` 的手写形式 —— 与 InMemory 版
// conversationBeforeCursor 逐字对应。**不可**退化成只比 updated_at：
// 同一时刻（同一事务 NOW() / 种子数据）更新的多行会被整批跳过。
func (r *PostgresConversationRepo) ListConversationsPage(ctx context.Context, userID int64, limit int, cursorUpdatedAt time.Time, cursorID int64) ([]model.Conversation, bool, error) {
	if limit <= 0 {
		limit = 20
	}
	q := r.db.WithContext(ctx).Where("user_id = ? AND deleted_at IS NULL", userID)
	if !cursorUpdatedAt.IsZero() || cursorID != 0 {
		q = q.Where("(updated_at < ? OR (updated_at = ? AND id < ?))", cursorUpdatedAt, cursorUpdatedAt, cursorID)
	}
	var out []model.Conversation
	// 取 limit+1 探测 hasMore（与 logic 层既有口径一致）
	err := q.Order("updated_at DESC, id DESC").Limit(limit + 1).Find(&out).Error
	if err != nil {
		return nil, false, err
	}
	hasMore := len(out) > limit
	if hasMore {
		out = out[:limit]
	}
	return out, hasMore, nil
}

// SetPinned Stage 72：Postgres 版置顶/取消置顶（同时刷新 updated_at）。
// 仅对未删除的会话生效（E2E-08：防止对软删除记录操作）。
func (r *PostgresConversationRepo) SetPinned(ctx context.Context, id int64, pinned bool) error {
	return r.db.WithContext(ctx).
		Exec(`UPDATE emotion_echo_chat.conversations
		      SET pinned = ?, updated_at = NOW()
		      WHERE id = ? AND deleted_at IS NULL`, pinned, id).Error
}

// UpdateTitle Stage 72：Postgres 版改标题（同时刷新 updated_at）。
// 仅对未删除的会话生效（E2E-08：防止对软删除记录操作）。
func (r *PostgresConversationRepo) UpdateTitle(ctx context.Context, id int64, title string) error {
	return r.db.WithContext(ctx).
		Exec(`UPDATE emotion_echo_chat.conversations
		      SET title = ?, updated_at = NOW()
		      WHERE id = ? AND deleted_at IS NULL`, title, id).Error
}

// UpdateAgentID CL-06：Postgres 版切角色（同时刷新 updated_at；仅未删除会话生效）。
// ⚠️ 只存引用：agent_id 指向 user-svc 的角色行，本服务**不校验属主**（跨服务不耦合）——
// 属主校验由 BFF 在调用前完成（它会读角色档案）；非法引用由 BFF 读取时回落默认角色。
func (r *PostgresConversationRepo) UpdateAgentID(ctx context.Context, id int64, agentID int64) error {
	return r.db.WithContext(ctx).
		Exec(`UPDATE emotion_echo_chat.conversations
		      SET agent_id = ?, updated_at = NOW()
		      WHERE id = ? AND deleted_at IS NULL`, agentID, id).Error
}

// Stage 30-C A3: 事务版本（tx == nil 退化为 r.db；非事务路径）

func (r *PostgresConversationRepo) CreateConversationTx(tx *gorm.DB, ctx context.Context, c *model.Conversation) error {
	c.ID = 0
	if tx == nil {
		tx = r.db
	}
	return tx.WithContext(ctx).Create(c).Error
}

func (r *PostgresConversationRepo) AppendMessageTx(tx *gorm.DB, ctx context.Context, m *model.Message) error {
	m.ID = 0
	if tx == nil {
		tx = r.db
	}
	return tx.WithContext(ctx).Create(m).Error
}

func (r *PostgresConversationRepo) IncrementMessageCountTx(tx *gorm.DB, ctx context.Context, conversationID int64) error {
	if tx == nil {
		tx = r.db
	}
	return tx.WithContext(ctx).
		Exec(`UPDATE emotion_echo_chat.conversations
		      SET message_count = message_count + 1,
		          last_message_at = NOW(),
		          updated_at = NOW()
		      WHERE id = ?`, conversationID).Error
}

func (r *PostgresConversationRepo) DeleteConversationTx(tx *gorm.DB, ctx context.Context, id int64) error {
	if tx == nil {
		return r.DeleteConversation(ctx, id)
	}
	now := time.Now()
	return tx.WithContext(ctx).Transaction(func(sub *gorm.DB) error {
		// 软删除消息
		if err := sub.Model(&model.Message{}).
			Where("conversation_id = ? AND deleted_at IS NULL", id).
			Update("deleted_at", now).Error; err != nil {
			return err
		}
		// 软删除会话
		return sub.Model(&model.Conversation{}).
			Where("id = ? AND deleted_at IS NULL", id).
			Update("deleted_at", now).Error
	})
}