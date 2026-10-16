package repository

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CL-05 Phase 6 · T-8：仓储层**不得**再出现"取最早 N 条"（`ORDER BY id ASC`）的旧语义。
//
// 背景（2026-10-14 亲核 + 用户裁定"删掉"）：旧 `ListMessages(ctx, conversationID, limit)`
// 的实现是 `Order("id ASC")`（**取最早 N 条**），而 chat-svc 的生产路径早已切到
// `ListMessagesPage`（`id DESC` + cursor，返回**最新** N 条）。旧方法**只剩 integration
// 测试在调**（gRPC/HTTP 两条 handler 都经 logic 走 Page 版）⇒ 属死路径，已按裁定删除。
//
// 本钉的作用是**防止它（或同型的"最早优先"查询）日后被重新引入** ——
// 聊天场景"先给最新"是产品口径，反向语义回归会直接表现成"打开会话看到最老的消息"。
func TestRepository_NoLegacyEarliestFirstListMessages(t *testing.T) {
	src, err := os.ReadFile("conversation_repository.go")
	require.NoError(t, err)
	s := string(src)

	assert.NotContains(t, s, `Order("id ASC")`,
		"仓储层不得再有 ORDER BY id ASC 的'取最早 N 条'查询（聊天应先给最新）")
	assert.NotContains(t, s, "ListMessages(ctx context.Context, conversationID int64, limit int)",
		"旧 ListMessages(ASC) 已按用户裁定删除，不得复活")

	// 反向对照：**必须有**游标分页方法存在 —— 否则"把两个方法都删了"也能让上面两条变绿（假绿）
	assert.Contains(t, s,
		"ListMessagesPage(ctx context.Context, conversationID int64, limit int, cursor int64)",
		"游标分页方法必须仍在（防'删光即绿'式假通过）")
	assert.True(t,
		strings.Contains(s, `Order("id DESC")`) || strings.Contains(s, "id < ?"),
		"游标分页应基于 id DESC / id < cursor")
}
