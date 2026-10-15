package logic

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"emotion-echo-chat-svc/internal/model"
	"emotion-echo-chat-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CL-05 Phase 5 · T-4（游标往返 / 精度）：会话列表游标的编解码契约。
//
// 精度必须到**微秒**：PG timestamptz 是微秒精度。若游标降到毫秒，"同一时刻更新的多行"
// 会变多，而且截断后的 cursor 既不 `<` 也不 `=` 真实值 ⇒ 连 id tie-break 都失效 ⇒ 翻页漏行。
func TestConversationCursor_RoundTripPreservesMicroseconds(t *testing.T) {
	t.Parallel()

	at := time.Date(2026, 10, 16, 6, 0, 0, 123456000, time.UTC) // 123456 微秒
	enc := encodeConversationCursor(at, 42)

	assert.True(t, strings.HasSuffix(enc, ":42"), "游标尾部必须是 id: %q", enc)
	assert.Equal(t, strconv.FormatInt(at.UnixMicro(), 10), strings.Split(enc, ":")[0],
		"时间部分必须是 **UnixMicro**（毫秒会截断 ⇒ 同刻行漏行）")

	gotAt, gotID, err := decodeConversationCursor(enc)
	require.NoError(t, err)
	assert.Equal(t, int64(42), gotID)
	assert.Equal(t, at.UnixMicro(), gotAt.UnixMicro(), "往返必须保持微秒精度（want %v got %v）", at, gotAt)
}

func TestConversationCursor_EmptyMeansFirstPage(t *testing.T) {
	t.Parallel()

	at, id, err := decodeConversationCursor("")
	require.NoError(t, err)
	assert.True(t, at.IsZero(), "空游标 = 首页（零值时间）")
	assert.Equal(t, int64(0), id)
	assert.Equal(t, "", encodeConversationCursor(time.Time{}, 0), "零值必须编成空串")
}

func TestConversationCursor_InvalidIsErrorNotSilentFirstPage(t *testing.T) {
	t.Parallel()

	// 非法游标**必须报错**，不能静默当首页 —— 否则翻页会悄悄重来而前端无法察觉。
	for _, bad := range []string{"abc", "1:2:3", "x:1", "1:y", ":"} {
		_, _, err := decodeConversationCursor(bad)
		assert.Error(t, err, "非法游标 %q 必须报错", bad)
	}
}

func idsOfResp(r *types.ListConversationsResp) map[int64]bool {
	m := make(map[int64]bool, len(r.List))
	for _, c := range r.List {
		m[c.Id] = true
	}
	return m
}

// T-4：会话列表**第二页**与第一页不得有交集，且第一页就给出可用的 NextCursor；
// 翻完所有页必须不重不漏。构造里刻意含**同一 updated_at** 的两行
// （模拟同一事务的 NOW()）—— 单键游标在这里会漏行。
func TestListConversationsLogic_CursorPagingNoOverlapNoGap(t *testing.T) {
	t.Parallel()

	svcCtx, repo, _ := newTestCtx(t)
	uid := int64(100)
	base := time.Date(2026, 10, 16, 6, 0, 0, 0, time.UTC)
	stamps := []time.Duration{0, 1 * time.Minute, 1 * time.Minute, 2 * time.Minute, 3 * time.Minute}
	for _, d := range stamps {
		require.NoError(t, repo.CreateConversation(context.Background(), &model.Conversation{
			UserID: uid, Title: "C", UpdatedAt: base.Add(d),
		}))
	}

	ctx := ctxWithUserID(context.Background(), uid)

	page1, err := NewListConversationsLogic(ctx, svcCtx).ListConversations(&types.ListConversationsReq{Limit: 2})
	require.NoError(t, err)
	require.Len(t, page1.List, 2)
	assert.True(t, page1.HasMore)
	require.NotEmpty(t, page1.NextCursor, "第一页必须给出 NextCursor（否则前端无从推进）")

	page2, err := NewListConversationsLogic(ctx, svcCtx).ListConversations(&types.ListConversationsReq{Limit: 2, Cursor: page1.NextCursor})
	require.NoError(t, err)
	require.Len(t, page2.List, 2)
	require.NotEmpty(t, page2.NextCursor)

	i1, i2 := idsOfResp(page1), idsOfResp(page2)
	for id := range i2 {
		assert.False(t, i1[id], "第二页不得重复第一页的会话 id=%d", id)
	}

	page3, err := NewListConversationsLogic(ctx, svcCtx).ListConversations(&types.ListConversationsReq{Limit: 2, Cursor: page2.NextCursor})
	require.NoError(t, err)
	require.Len(t, page3.List, 1, "同 updated_at 的尾行**不得**被跳过")
	assert.False(t, page3.HasMore)
	assert.Empty(t, page3.NextCursor, "没有更多时 NextCursor 必须为空")

	i3 := idsOfResp(page3)
	for id := range i3 {
		assert.False(t, i1[id] || i2[id], "第三页不得与前面重复 id=%d", id)
	}
	assert.Equal(t, len(stamps), len(i1)+len(i2)+len(i3), "三页并集 = 全量（不重不漏）")
}

// T-4 负向对照：非法游标必须**报错**，不得静默退回首页。
func TestListConversationsLogic_InvalidCursorReturnsError(t *testing.T) {
	t.Parallel()

	svcCtx, _, _ := newTestCtx(t)
	_, err := NewListConversationsLogic(ctxWithUserID(context.Background(), 100), svcCtx).
		ListConversations(&types.ListConversationsReq{Limit: 2, Cursor: "not-a-cursor"})
	require.Error(t, err, "非法游标必须报错（静默回首页会让翻页悄悄重来）")
}
