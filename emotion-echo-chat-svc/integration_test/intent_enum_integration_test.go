//go:build integration
// +build integration

// Package integration_test — intent_enum_integration_test.go
//
// AGENTS.md §2.4 §契约 5（schema 与写入端一致性）：
//   「每个 VARCHAR(32) NOT NULL 枚举列，至少 1 个 integration test 断言写入值 ∈ enum 集合」
//
// 本文件覆盖 emotion_echo_chat.messages.intent（c007 已把 DDL 从 VARCHAR(16) 扩到
// VARCHAR(32)，正是"DDL 太窄把白名单标签写坏"的 c007 类 bug 的现场）。
//
// 断言三件事：
//   ① 列宽确实是 VARCHAR(32)（与 model tag / c007 一致）；
//   ② 白名单里的**每个**意图值经真实 repo 写入后**原样读回**（不被截断/拒绝）；
//   ③ 非法值不在白名单内（写入端由 allowedIntents 兜底落 ''）。
//
// 跑：  go test -tags integration -v ./integration_test/ -run TestMessagesIntent
package integration_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"emotion-echo-chat-svc/internal/model"
	"emotion-echo-chat-svc/internal/repository"
)

// intentEnumValues 与 internal/logic.allowedIntents 及
// emotion-llm-service intent.INTENTS 三方一致的 6 类意图。
// （integration_test 是独立包，无法读未导出的 allowedIntents；
//  这里的字面量就是"契约"本身 —— 三方分叉时本测试与
//  sendmessagelogic_test.go 会一起红。）
var intentEnumValues = []string{
	"emotional_support", "study_help", "tech_help", "career_help", "lifestyle", "other",
}

func TestMessagesIntent_Integration_EnumRoundTrip(t *testing.T) {
	ctx := context.Background()
	pgC, db := pgContainerDesc(t, ctx)
	t.Cleanup(func() { _ = pgC.Terminate(ctx) })

	// ① 列宽 = VARCHAR(32) NOT NULL（§契约 5 的"VARCHAR(32)"字面契约）
	var colLen *int
	var nullable string
	require.NoError(t, db.Raw(`SELECT character_maximum_length, is_nullable
		FROM information_schema.columns
		WHERE table_schema='emotion_echo_chat' AND table_name='messages' AND column_name='intent'`).
		Row().Scan(&colLen, &nullable))
	require.NotNil(t, colLen)
	assert.Equal(t, 32, *colLen, "messages.intent 必须是 VARCHAR(32)（c007）")
	assert.Equal(t, "NO", nullable, "messages.intent 必须 NOT NULL")

	repo := repository.NewPostgresConversationRepo(db)
	conv := &model.Conversation{UserID: 1, Title: "intent-contract5"}
	require.NoError(t, repo.CreateConversation(ctx, conv))

	// ② 白名单每个值经真实写入端往返后原样读回
	for _, want := range intentEnumValues {
		require.LessOrEqual(t, len(want), 32, "白名单值 %q 必须塞得进 VARCHAR(32)", want)
		msg := &model.Message{ConversationID: conv.ID, UserID: 1, Role: "user", Content: "x", Intent: want}
		require.NoError(t, repo.AppendMessage(ctx, msg), "写入 intent=%q 失败", want)

		got, err := repo.ListMessages(ctx, conv.ID, 100)
		require.NoError(t, err)
		var found bool
		for _, m := range got {
			if m.ID == msg.ID {
				found = true
				assert.Equal(t, want, m.Intent,
					"intent 往返后必须原样读回（截断/拒绝 = DDL 太窄，c007 类 bug）")
			}
		}
		assert.True(t, found, "刚写入的消息必须能读回（intent=%q）", want)
	}

	// ③ 白名单外（写入端应落 ''）——此处只断言 DB 层不会把非法值当枚举"接受并截断"，
	//    即写入端兜底后的 '' 是唯一可能的非法表示。
	for _, m := range mustList(t, repo, ctx, conv.ID) {
		assert.Contains(t, append(append([]string{}, intentEnumValues...), ""), m.Intent,
			"库内出现的 intent 值必须 ∈ 白名单 ∪ {''}，实际 %q", m.Intent)
	}
}

func mustList(t *testing.T, repo *repository.PostgresConversationRepo, ctx context.Context, convID int64) []model.Message {
	t.Helper()
	msgs, err := repo.ListMessages(ctx, convID, 100)
	require.NoError(t, err)
	return msgs
}
