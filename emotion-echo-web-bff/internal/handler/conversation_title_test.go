// Package handler — conversation_title_test.go
//
// CL-04 · T-18（2026-10-14 用户实测后追加）：**会话标题按第一轮对话自动概括**。
//
// 用户实测："会话名根据第一段对话自动概括生成这个没做，属于功能欠缺，这个要添加"。
// 三层现状（本轮亲核，见账本 CL-F-12）：
//   ① 前端 `AIStreamParams.shouldGenerateTitle` 存在，但**只有 [id].vue 显式传 false**，
//      新建会话路径 `createNewConversation` 传的是 `extraParams?.shouldGenerateTitle` ⇒ 恒 undefined；
//   ② **BFF `aiStreamReq` 结构里根本没有该字段** ⇒ 静默丢弃（Phase 0 缺陷 8 的一项）；
//   ③ chat-svc 有 `UpdateTitle`/`UpdateConversation` 能力，但**无任何调用方**（backlog R-13 实证复活）。
//
// 本实现的边界（**只动 A 轨文件面**，chat-svc / chat_handler / 前端一律不碰）：
//   · 触发：落库后该会话消息数 ≤ 2（= 第一轮对话）；或前端显式 `shouldGenerateTitle: true`；
//   · 取标题：优先用 LLM（既有 ChatCompletion RPC，小 max_tokens、低温度）；
//     无 LLM / 出错 / 结果不可用 ⇒ **规则兜底**（取首条用户消息前若干字）；
//   · 落地：调用**既有** `ChatClient.UpdateConversation`（不改其实现）；
//   · 全链降级：文件来源 / 标题更新器 / LLM 任一为 nil 都只是"不生成"，绝不阻断对话。
package handler

import (
	"context"
	"errors"
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/config"
	"emotion-echo-web-bff/internal/downstream"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeTitleUpdater 只实现"改名"这一窄接口（避免实现整个 downstream.ChatClient）
type fakeTitleUpdater struct {
	calls   int
	gotID   int64
	gotName string
	err     error
}

func (f *fakeTitleUpdater) UpdateConversation(_ context.Context, conversationID int64, title string, _ int64) (*downstream.UpdateConversationResp, error) {
	f.calls++
	f.gotID = conversationID
	f.gotName = title
	if f.err != nil {
		return nil, f.err
	}
	return &downstream.UpdateConversationResp{Success: true, Id: conversationID, Title: title}, nil
}

// 注意：stream 必须收 **接口** 而非 *fakeLLMStreamer —— 传 nil 指针会被包成"非 nil 接口"，
// 于是 `h.llm == nil` 判假、随后在 nil 接收者上调方法 panic（typed-nil 陷阱，本 WIP 首次跑就踩了）。
func titleTestHandler(msgs []downstream.MessageView, stream downstream.LLMChatStreamer, upd *fakeTitleUpdater) *AIStreamHandler {
	return &AIStreamHandler{
		cfg:          config.Config{},
		llm:          stream,
		files:        &captureLister{msgs: msgs},
		titleUpdater: upd,
	}
}

func twoMsgs() []downstream.MessageView {
	return []downstream.MessageView{
		{ID: 1, Role: "user", Content: "我下周要参加技术面试，一想到被追问项目细节就睡不好", ContentType: "text"},
		{ID: 2, Role: "assistant", Content: "先允许自己紧张…", ContentType: "text"},
	}
}

// T-18a：第一轮对话 ⇒ 用 LLM 结果改标题
func TestGenerateTitle_FirstExchange_UsesLLMTitle(t *testing.T) {
	upd := &fakeTitleUpdater{}
	stream := &fakeLLMStreamer{deltas: []string{"面试", "焦虑", "应对"}}
	h := titleTestHandler(twoMsgs(), stream, upd)

	h.maybeGenerateTitle(context.Background(), 42, "我下周要参加技术面试，一想到被追问项目细节就睡不好", true)

	require.Equal(t, 1, upd.calls, "第一轮对话必须触发一次改标题")
	assert.Equal(t, int64(42), upd.gotID)
	assert.Equal(t, "面试焦虑应对", upd.gotName, "标题应取 LLM 输出（去空白/标点）")
	// 标题请求必须是**非流式语义的小请求**：低温度 + 小 max_tokens（成本可控）
	assert.Equal(t, int32(titleMaxTokens), stream.gotMaxTokens, "标题请求必须用小 max_tokens（32），不可复用对话的 1024")
}

// T-18b：非第一轮 ⇒ 不改标题（避免每轮都重命名）
func TestGenerateTitle_NotFirstExchange_Skipped(t *testing.T) {
	msgs := append(twoMsgs(),
		downstream.MessageView{ID: 3, Role: "user", Content: "还是怕", ContentType: "text"},
		downstream.MessageView{ID: 4, Role: "assistant", Content: "嗯，我懂", ContentType: "text"},
	)
	upd := &fakeTitleUpdater{}
	h := titleTestHandler(msgs, &fakeLLMStreamer{deltas: []string{"不该用"}}, upd)

	h.maybeGenerateTitle(context.Background(), 42, "还是怕", false)

	assert.Equal(t, 0, upd.calls, "第二轮之后不得再改标题")
}

// T-18c：前端显式要求（shouldGenerateTitle=true）⇒ 即使不是第一轮也可改
func TestGenerateTitle_ExplicitFlag_OverridesCountRule(t *testing.T) {
	msgs := append(twoMsgs(), downstream.MessageView{ID: 3, Role: "user", Content: "再聊聊", ContentType: "text"})
	upd := &fakeTitleUpdater{}
	h := titleTestHandler(msgs, &fakeLLMStreamer{deltas: []string{"新标题"}}, upd)

	h.maybeGenerateTitle(context.Background(), 42, "再聊聊", true)

	assert.Equal(t, 1, upd.calls, "前端显式 shouldGenerateTitle=true 时必须生成")
	assert.Equal(t, "新标题", upd.gotName)
}

// T-18d：无 LLM（或 LLM 出错）⇒ 规则兜底（首条用户消息前若干字），仍然改标题
func TestGenerateTitle_LLMUnavailable_FallsBackToRule(t *testing.T) {
	upd := &fakeTitleUpdater{}
	h := titleTestHandler(twoMsgs(), nil, upd) // llm == nil

	h.maybeGenerateTitle(context.Background(), 42, "我下周要参加技术面试，一想到被追问项目细节就睡不好", true)

	require.Equal(t, 1, upd.calls, "无 LLM 也必须兜底出标题（不能只剩死路）")
	assert.NotEmpty(t, upd.gotName)
	assert.LessOrEqual(t, len([]rune(upd.gotName)), maxTitleRunes, "兜底标题必须受长度上限约束")
	assert.True(t, strings.HasPrefix(upd.gotName, "我下周要参加技术面试"), "兜底标题应取自首条用户消息")

	// LLM 报错同样兜底
	upd2 := &fakeTitleUpdater{}
	h2 := titleTestHandler(twoMsgs(), &fakeLLMStreamer{err: errors.New("upstream down")}, upd2)
	h2.maybeGenerateTitle(context.Background(), 42, "我下周要参加技术面试", true)
	assert.Equal(t, 1, upd2.calls, "LLM 报错也必须兜底")
}

// T-18e：任一依赖缺失 ⇒ 静默不生成，绝不 panic / 不阻断
func TestGenerateTitle_MissingDeps_NoPanic(t *testing.T) {
	cases := map[string]*AIStreamHandler{
		"无标题更新器": {files: &captureLister{msgs: twoMsgs()}, llm: &fakeLLMStreamer{deltas: []string{"x"}}},
		"无消息来源":  {titleUpdater: &fakeTitleUpdater{}},
		"全空":     {},
	}
	for name, h := range cases {
		t.Run(name, func(t *testing.T) {
			assert.NotPanics(t, func() {
				h.maybeGenerateTitle(context.Background(), 42, "随便说点", true)
			})
		})
	}
}

// T-18f：改标题失败 ⇒ 不冒泡（标题是增强不是依赖）
func TestGenerateTitle_UpdateFailure_NoPanic(t *testing.T) {
	upd := &fakeTitleUpdater{err: errors.New("grpc unavailable")}
	h := titleTestHandler(twoMsgs(), &fakeLLMStreamer{deltas: []string{"标题"}}, upd)
	assert.NotPanics(t, func() {
		h.maybeGenerateTitle(context.Background(), 42, "我很难过", true)
	})
	assert.Equal(t, 1, upd.calls)
}

// T-18g：标题清洗——去换行/引号/句末标点，且不超长
func TestSanitizeTitle(t *testing.T) {
	assert.Equal(t, "面试焦虑应对", sanitizeTitle("「面试焦虑应对。」"))
	assert.Equal(t, "面试焦虑", sanitizeTitle("面试焦虑\n\n"))
	assert.Equal(t, "", sanitizeTitle("   "))
	assert.LessOrEqual(t, len([]rune(sanitizeTitle(strings.Repeat("长", 80)))), maxTitleRunes)
}
