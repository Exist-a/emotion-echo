//go:build integration
// +build integration

// Package integration_test — conversation_update_integration_test.go
//
// Stage 72：PinConversation / UpdateConversation 真实 Postgres 集成测试。
//
// 验证（AGENTS.md §2.4 §契约 5 schema 与写入端一致性）：
//   - PostgresConversationRepo.SetPinned / UpdateTitle 对真实库持久化
//   - migration 003_add_pinned_to_conversations.sql 可对旧结构（无 pinned 列）
//     幂等应用（ALTER TABLE ADD COLUMN IF NOT EXISTS 连跑两次不报错）
//
// 跑：  go test -tags integration -v ./integration_test/ -run TestConversation
package integration_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"emotion-echo-chat-svc/internal/model"
	"emotion-echo-chat-svc/internal/repository"
)

// TestConversationRepo_Integration_SetPinnedAndUpdateTitle 起真实 Postgres
// （pgContainerDesc 建表已含 pinned 列，与 02-create-tables DDL 同步），
// 验证 SetPinned / UpdateTitle 持久化 + updated_at 刷新。
func TestConversationRepo_Integration_SetPinnedAndUpdateTitle(t *testing.T) {
	ctx := context.Background()

	pgC, db := pgContainerDesc(t, ctx)
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })

	repo := repository.NewPostgresConversationRepo(db)

	// 建会话（DB 自增 ID）
	conv := &model.Conversation{UserID: 1, Title: "original"}
	require.NoError(t, repo.CreateConversation(ctx, conv))
	require.NotZero(t, conv.ID)

	// UpdateTitle：标题持久化
	require.NoError(t, repo.UpdateTitle(ctx, conv.ID, "renamed"))
	got, err := repo.GetConversationByID(ctx, conv.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "renamed", got.Title)

	// SetPinned：置顶持久化
	require.NoError(t, repo.SetPinned(ctx, conv.ID, true))
	got, err = repo.GetConversationByID(ctx, conv.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.True(t, got.Pinned)

	// SetPinned：取消置顶持久化
	require.NoError(t, repo.SetPinned(ctx, conv.ID, false))
	got, err = repo.GetConversationByID(ctx, conv.ID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.False(t, got.Pinned)
}

// TestMigration003_Integration_IdempotentApply 对旧结构（无 pinned 列）的表
// 连跑两次 migration 003 的 ALTER TABLE，断言幂等不报错且列已存在。
func TestMigration003_Integration_IdempotentApply(t *testing.T) {
	ctx := context.Background()

	pgC, db := pgContainerDesc(t, ctx)
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })

	// 退化到旧结构：删掉 pinned 列（pgContainerDesc 按新 DDL 建了 pinned）
	require.NoError(t, db.WithContext(ctx).
		Exec(`ALTER TABLE emotion_echo_chat.conversations DROP COLUMN IF EXISTS pinned`).Error)

	const migrationSQL = `ALTER TABLE emotion_echo_chat.conversations
    ADD COLUMN IF NOT EXISTS pinned BOOLEAN NOT NULL DEFAULT FALSE;`

	// 连跑两次（幂等）
	require.NoError(t, db.WithContext(ctx).Exec(migrationSQL).Error)
	require.NoError(t, db.WithContext(ctx).Exec(migrationSQL).Error)

	// 列存在且默认 false
	var cnt int64
	require.NoError(t, db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM information_schema.columns
		 WHERE table_schema='emotion_echo_chat' AND table_name='conversations'
		   AND column_name='pinned'`).Scan(&cnt).Error)
	assert.Equal(t, int64(1), cnt, "migration 003 后 pinned 列应存在")
}
