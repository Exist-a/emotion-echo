// Package handler — prompt_history_test.go
//
// CL-04 · Phase 2（A 轨，2026-10-14）：**同会话历史注入**（滑动窗口 + token 预算 + 去重 + 上界）。
//
// 用户 2026-10-14 裁定（roadmap §4 CL-04 已定参数）：
//   · 滑动窗口 **默认最近 10 条消息，上限 20 条**，可配 —— 配置值夹在 1..20，**token 预算只做"缩"不做"扩"**；
//   · token 预算硬顶 **2000**（中文按 1 字 ≈ 1 token 估算，不追求精确，只需防爆）。
//
// 契约（对应 plan §2 的 T-3/T-4/T-5/T-6/T-15）：
//   T-3  多轮会话 ⇒ messages = [system, 历史(时间正序)…, 当前 user]；且**不含当前轮重复**
//   T-4  单轮 / 无会话 / 无来源 / 来源报错 ⇒ 仍为两条（不产生空 role 消息）
//   T-5  超预算 ⇒ 从**最旧端**裁剪，且**至少保留最近 1 轮**（不断当前上下文）
//   T-6  超长 userContent ⇒ 按 rune 边界截断（不切裂 UTF-8、不 panic）
//   T-15 负向对照：关掉历史来源 ⇒ 复现"无记忆"（证明 T-3 的钉子真的能红）
package handler

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLister 记录取数参数的 fake 会话消息来源
type captureLister struct {
	msgs      []downstream.MessageView
	err       error
	calls     int
	gotLimit  int
	gotCursor int64
}

func (c *captureLister) ListMessages(_ context.Context, _ int64, limit int, cursor int64) ([]downstream.MessageView, bool, error) {
	c.calls++
	c.gotLimit = limit
	c.gotCursor = cursor
	if c.err != nil {
		return nil, false, c.err
	}
	return c.msgs, false, nil
}

// histMsg 构造一条历史消息（时间正序由调用方保证）
func histMsg(id int64, role, content, contentType string) downstream.MessageView {
	return downstream.MessageView{ID: id, Role: role, Content: content, ContentType: contentType}
}

// ---------------------------------------------------------------------------
// T-3 历史注入：位置、顺序、内容
// ---------------------------------------------------------------------------

func TestHistory_InjectedInChronologicalOrderBetweenSystemAndCurrentUser(t *testing.T) {
	lister := &captureLister{msgs: []downstream.MessageView{
		histMsg(1, "user", "我上周面试没过", "text"),
		histMsg(2, "assistant", "听起来那阵子挺不好受的", "text"),
		histMsg(3, "user", "今天又想到这件事了", "text"),
	}}
	h := &AIStreamHandler{files: lister}

	msgs := h.assembleMessages(assembleInput{
		ctx:            context.Background(),
		conversationID: 42,
		userContent:    "我该怎么办",
	})

	require.Len(t, msgs, 5, "system + 3 条历史 + 当前 user")
	assert.Equal(t, "system", msgs[0].Role)
	assert.Equal(t, []string{"user", "assistant", "user", "user"},
		[]string{msgs[1].Role, msgs[2].Role, msgs[3].Role, msgs[4].Role})
	// 历史必须**时间正序**（最早在前），当前轮恒在末
	assert.Equal(t, "我上周面试没过", msgs[1].Content)
	assert.Equal(t, "听起来那阵子挺不好受的", msgs[2].Content)
	assert.Equal(t, "今天又想到这件事了", msgs[3].Content)
	assert.Equal(t, "我该怎么办", msgs[4].Content, "当前轮必须恒为最后一条")
	// 取数契约：cursor=0（从最新开始），limit = 上限+1（多取一条用于识别当前轮重复）
	assert.Equal(t, 1, lister.calls)
	assert.Equal(t, int64(0), lister.gotCursor)
	assert.Equal(t, maxHistoryMessages+1, lister.gotLimit)
}

// 前端流程：当前轮 user 消息**先落库**再打 /ai/stream ⇒ 库里最新一条就是当前轮，必须去重
func TestHistory_DoesNotDuplicateCurrentTurn(t *testing.T) {
	lister := &captureLister{msgs: []downstream.MessageView{
		histMsg(1, "user", "上一轮的问题", "text"),
		histMsg(2, "assistant", "上一轮的回答", "text"),
		histMsg(3, "user", "这一轮的问题", "text"), // ← 当前轮已落库
	}}
	h := &AIStreamHandler{files: lister}

	msgs := h.assembleMessages(assembleInput{
		ctx:            context.Background(),
		conversationID: 42,
		userContent:    "这一轮的问题",
	})

	require.Len(t, msgs, 4, "当前轮不得在历史里再出现一次（system + 2 历史 + 当前）")
	assert.Equal(t, "上一轮的问题", msgs[1].Content)
	assert.Equal(t, "上一轮的回答", msgs[2].Content)
	assert.Equal(t, "这一轮的问题", msgs[3].Content)
	assert.Equal(t, 1, strings.Count(strings.Join(contentsOf(msgs), "|"), "这一轮的问题"),
		"当前轮内容在整份 messages 里只能出现一次")
}

// 文件类消息走文件上下文（Files），**不塞历史原文**；空内容 / 不识别的 role 一律跳过
func TestHistory_ExcludesFileEmptyAndUnknownRoles(t *testing.T) {
	lister := &captureLister{msgs: []downstream.MessageView{
		histMsg(1, "user", "http://localhost:9000/avatars/uploads/a.txt", "file"),
		histMsg(2, "user", "   ", "text"),
		histMsg(3, "tool", "工具输出不应进对话历史", "text"),
		histMsg(4, "user", "正常的一条", "text"),
	}}
	h := &AIStreamHandler{files: lister}

	msgs := h.assembleMessages(assembleInput{
		ctx: context.Background(), conversationID: 42, userContent: "当前问题",
	})

	require.Len(t, msgs, 3, "只有 1 条可用历史（file/空白/tool 全被排除）")
	assert.Equal(t, "正常的一条", msgs[1].Content)
}

// ---------------------------------------------------------------------------
// T-4 / T-15 单轮不多喂 + 负向对照（关掉历史 ⇒ 复现无记忆）
// ---------------------------------------------------------------------------

func TestHistory_NoHistorySignals_YieldsExactlyTwoMessages(t *testing.T) {
	cases := []struct {
		name string
		h    *AIStreamHandler
		in   assembleInput
	}{
		{"无会话上下文（conversationID=0）", &AIStreamHandler{files: &captureLister{msgs: []downstream.MessageView{histMsg(1, "user", "不该被取", "text")}}},
			assembleInput{ctx: context.Background(), userContent: "hi"}},
		{"无历史来源（files=nil）", &AIStreamHandler{},
			assembleInput{ctx: context.Background(), conversationID: 42, userContent: "hi"}},
		{"会话无历史（空列表）", &AIStreamHandler{files: &captureLister{}},
			assembleInput{ctx: context.Background(), conversationID: 42, userContent: "hi"}},
		{"来源报错（增强不是依赖）", &AIStreamHandler{files: &captureLister{err: assert.AnError}},
			assembleInput{ctx: context.Background(), conversationID: 42, userContent: "hi"}},
		{"历史里只有当前轮（去重后为空）", &AIStreamHandler{files: &captureLister{msgs: []downstream.MessageView{histMsg(1, "user", "hi", "text")}}},
			assembleInput{ctx: context.Background(), conversationID: 42, userContent: "hi"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			msgs := c.h.assembleMessages(c.in)
			require.Len(t, msgs, 2, "T-4/T-15：无历史信号时必须仍是 [system, user] 两条（复现无记忆）")
			assert.Equal(t, "system", msgs[0].Role)
			assert.Equal(t, "user", msgs[1].Role)
			assert.Equal(t, "hi", msgs[1].Content)
		})
	}
}

// ---------------------------------------------------------------------------
// 窗口：默认最近 10 条
// ---------------------------------------------------------------------------

func TestHistory_WindowKeepsNewestTenByDefault(t *testing.T) {
	var msgs []downstream.MessageView
	for i := 1; i <= 25; i++ {
		msgs = append(msgs, histMsg(int64(i), "user", "第"+strconv.Itoa(i)+"条", "text"))
	}
	lister := &captureLister{msgs: msgs}
	h := &AIStreamHandler{files: lister}

	got := h.assembleMessages(assembleInput{ctx: context.Background(), conversationID: 42, userContent: "当前"})

	require.Len(t, got, 1+defaultHistoryMessages+1, "默认窗口 = 最近 %d 条", defaultHistoryMessages)
	assert.Equal(t, "第16条", got[1].Content, "窗口应取**最新**的 10 条（16..25），最旧的 15 条被丢弃")
	assert.Equal(t, "第25条", got[len(got)-2].Content)
}

// ---------------------------------------------------------------------------
// T-5 预算裁剪：从最旧端裁剪，至少保留最近 1 轮
// ---------------------------------------------------------------------------

func TestHistory_TokenBudgetTrimsOldestFirst(t *testing.T) {
	big := strings.Repeat("啊", 900) // 每条 ≈900 token
	lister := &captureLister{msgs: []downstream.MessageView{
		histMsg(1, "user", "旧" + big, "text"),
		histMsg(2, "assistant", "旧" + big, "text"),
		histMsg(3, "user", "新" + big, "text"),
		histMsg(4, "assistant", "新" + big, "text"),
	}}
	h := &AIStreamHandler{files: lister}

	msgs := h.assembleMessages(assembleInput{ctx: context.Background(), conversationID: 42, userContent: "当前"})

	require.Len(t, msgs, 4, "4 条历史共 ≈3600 token > 2000 ⇒ 从最旧端丢 2 条，保留最近 2 条")
	assert.True(t, strings.HasPrefix(msgs[1].Content, "新"), "保留的必须是**最近**两条")
	assert.LessOrEqual(t, estimateTokens(msgs[1].Content)+estimateTokens(msgs[2].Content), historyTokenBudget,
		"裁剪后历史总估算不得超预算")
}

func TestHistory_BudgetNeverDropsBelowOneTurn(t *testing.T) {
	huge := strings.Repeat("啊", 5000) // 单条就超预算
	lister := &captureLister{msgs: []downstream.MessageView{
		histMsg(1, "user", huge, "text"),
		histMsg(2, "assistant", huge, "text"),
	}}
	h := &AIStreamHandler{files: lister}

	msgs := h.assembleMessages(assembleInput{ctx: context.Background(), conversationID: 42, userContent: "当前"})

	require.Len(t, msgs, 4, "即使超预算也必须保留最近 1 轮（user+assistant），不能把上下文断掉")
}

// ---------------------------------------------------------------------------
// T-6 userContent 上界：rune 边界截断，不 panic
// ---------------------------------------------------------------------------

func TestHistory_UserContentClampedAtRuneBoundary(t *testing.T) {
	h := &AIStreamHandler{}
	long := strings.Repeat("汉", maxUserContentTokens+500)

	msgs := h.assembleMessages(assembleInput{ctx: context.Background(), userContent: long})

	require.Len(t, msgs, 2)
	last := msgs[len(msgs)-1]
	assert.Equal(t, maxUserContentTokens, len([]rune(last.Content)), "超长输入必须截断到上界")
	assert.True(t, utf8.ValidString(last.Content), "截断点不得切裂 UTF-8")
	assert.True(t, strings.HasPrefix(long, last.Content), "截断必须是对原串的前缀（不重排、不注入）")
}

// 未超上界时**逐字不变**（不能顺手改写用户输入）
func TestHistory_UserContentUntouchedWhenWithinLimit(t *testing.T) {
	h := &AIStreamHandler{}
	content := "  今天有点累  "
	msgs := h.assembleMessages(assembleInput{ctx: context.Background(), userContent: content})
	require.Len(t, msgs, 2)
	assert.Equal(t, content, msgs[1].Content, "未超上界的输入必须原样透传（含空白）")
}

// ---------------------------------------------------------------------------
// 小工具
// ---------------------------------------------------------------------------

func contentsOf(msgs []downstream.Message) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, m.Content)
	}
	return out
}
