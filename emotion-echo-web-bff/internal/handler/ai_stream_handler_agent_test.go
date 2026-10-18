// Package handler — ai_stream_handler_agent_test.go
//
// CL-06 · Phase 4：**请求 → 角色 → 注入 → 写库快照** 的端到端契约（T-4/T-7/T-20）。
//
// 决议（CL-D-30）：生效角色由**请求携带的 agentId**决定，且 BFF 必须拿它去**自己的角色列表**里查
// —— 因此客户端传的 id 既不能越权到别人的角色，也不需要 BFF 反向依赖 chat-svc 再读一次会话。
//
//	找不到 / 未传 / 已删 ⇒ **回落用户默认角色**（T-4）；一份角色都没有 ⇒ 无角色段（T-20）。
//
// 写入快照（T-7）= **本轮真正生效的那个角色 id**（提示词用谁，写库就记谁 —— 二者同源，
// 不会出现"人设是 A、标注是 B"）。
package handler

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/config"
	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAgentSource 满足 agentProfileSource（窄接口：只要 List）
type fakeAgentSource struct {
	profiles []downstream.AgentProfile
	err      error
	calls    int
	gotCtxOK bool
}

func (f *fakeAgentSource) ListAgentProfiles(ctx context.Context) ([]downstream.AgentProfile, error) {
	f.calls++
	// 角色列表要走鉴权（x-user-id metadata）；这里只需确认 ctx 非空即可
	f.gotCtxOK = ctx != nil
	if f.err != nil {
		return nil, f.err
	}
	return f.profiles, nil
}

func twoAgentProfiles() []downstream.AgentProfile {
	return []downstream.AgentProfile{
		{ID: 7, Name: "小E", Persona: "温柔、愿意倾听的陪伴者。", IsDefault: true, Voice: "FunAudioLLM/CosyVoice2-0.5B:anna"},
		{ID: 9, Name: "阿哲", Persona: "一个直率、说话不绕弯的朋友。", Tone: "直接、简短",
			Boundaries: []string{"不评价外貌"}, Voice: "FunAudioLLM/CosyVoice2-0.5B:alex"},
	}
}

func newAgentStreamRouter(streamer *fakeLLMStreamer, chat downstream.ChatClient, src agentProfileSource) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/ai/stream", NewAIStreamHandlerWithDeps(config.Config{},
		AIStreamDeps{LLM: streamer, Chat: chat, AgentProfiles: src}))
	return r
}

// mock 兜底路径（无 llm-gRPC 上游）：**不设 LLM 字段**而非传 nil 指针
// —— typed-nil 会被包成非 nil 接口，`h.llm == nil` 判假后在 nil 接收者上调方法 panic
// （本仓 conversation_title_test.go 记过同款坑，这里踩第二次）。
func newMockStreamRouter(chat downstream.ChatClient, src agentProfileSource) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/ai/stream", NewAIStreamHandlerWithDeps(config.Config{},
		AIStreamDeps{Chat: chat, AgentProfiles: src}))
	return r
}

func postStream(t *testing.T, r *gin.Engine, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream", bytes.NewReader([]byte(body)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ─────────── 生效角色 = 请求携带的那个（且必须属于自己） ───────────

func TestAIStream_RequestedAgent_InjectsPersona_AndStampsMessage(t *testing.T) {
	streamer := &fakeLLMStreamer{deltas: []string{"我在听"}}
	chat := &fakeChatClient{}
	r := newAgentStreamRouter(streamer, chat, &fakeAgentSource{profiles: twoAgentProfiles()})

	w := postStream(t, r, `{"message":"我今天有点累","conversationId":"5","agentId":9}`)
	require.Equal(t, http.StatusOK, w.Code)

	require.NotEmpty(t, streamer.gotMsgs)
	system := streamer.gotMsgs[0].Content
	assert.Contains(t, system, "角色名：阿哲", "请求指定的角色必须进 system prompt")
	assert.Contains(t, system, "不要：不评价外貌")
	assert.Contains(t, system, "【护栏】", "有角色段 ⇒ 必须有护栏段")

	assert.Equal(t, int64(9), chat.gotSendReq.AgentID, "写库快照 = 本轮生效角色（T-7）")
	assert.Equal(t, "assistant", chat.gotSendReq.Role)
}

func TestAIStream_NoAgentID_UsesDefaultProfile(t *testing.T) {
	streamer := &fakeLLMStreamer{deltas: []string{"嗯"}}
	chat := &fakeChatClient{}
	r := newAgentStreamRouter(streamer, chat, &fakeAgentSource{profiles: twoAgentProfiles()})

	postStream(t, r, `{"message":"在吗","conversationId":"5"}`)

	assert.Contains(t, streamer.gotMsgs[0].Content, "角色名：小E", "未指定 ⇒ 用户默认角色")
	assert.NotContains(t, streamer.gotMsgs[0].Content, "角色名：阿哲")
	assert.Equal(t, int64(7), chat.gotSendReq.AgentID)
}

// T-4：会话 agent_id 指向**已删/非本人**角色 ⇒ 回落默认角色（不报错、不给空 prompt）
func TestAIStream_UnknownAgentID_FallsBackToDefault(t *testing.T) {
	streamer := &fakeLLMStreamer{deltas: []string{"嗯"}}
	chat := &fakeChatClient{}
	r := newAgentStreamRouter(streamer, chat, &fakeAgentSource{profiles: twoAgentProfiles()})

	w := postStream(t, r, `{"message":"在吗","conversationId":"5","agentId":999}`)

	require.Equal(t, http.StatusOK, w.Code, "未知 agentId 不得让对话失败")
	assert.Contains(t, streamer.gotMsgs[0].Content, "角色名：小E")
	assert.Equal(t, int64(7), chat.gotSendReq.AgentID, "快照必须是**实际生效**的角色（回落后的默认）")
}

// 请求指定的 id 恰是默认：不得因为"非 0"就走成"用户角色"而漏掉默认语义
func TestAIStream_RequestedDefaultAgentID_UsesThatProfile(t *testing.T) {
	streamer := &fakeLLMStreamer{deltas: []string{"嗯"}}
	chat := &fakeChatClient{}
	r := newAgentStreamRouter(streamer, chat, &fakeAgentSource{profiles: twoAgentProfiles()})

	postStream(t, r, `{"message":"在吗","conversationId":"5","agentId":7}`)

	assert.Contains(t, streamer.gotMsgs[0].Content, "角色名：小E")
	assert.Equal(t, int64(7), chat.gotSendReq.AgentID)
}

// 没有 isDefault 标记时取列表首项（user-svc 正常情况下总有默认，这是兜底）
func TestAIStream_NoDefaultFlag_UsesFirstProfile(t *testing.T) {
	streamer := &fakeLLMStreamer{deltas: []string{"嗯"}}
	chat := &fakeChatClient{}
	r := newAgentStreamRouter(streamer, chat, &fakeAgentSource{profiles: []downstream.AgentProfile{
		{ID: 11, Name: "甲"}, {ID: 12, Name: "乙"},
	}})

	postStream(t, r, `{"message":"在吗","conversationId":"5"}`)

	assert.Contains(t, streamer.gotMsgs[0].Content, "角色名：甲")
	assert.Equal(t, int64(11), chat.gotSendReq.AgentID)
}

// ─────────── T-20 负向对照：无角色（或删光角色）⇒ 与现状一致 ───────────

func TestAIStream_NoProfiles_NoAgentSegment_NoStamp(t *testing.T) {
	streamer := &fakeLLMStreamer{deltas: []string{"嗯"}}
	chat := &fakeChatClient{}
	r := newAgentStreamRouter(streamer, chat, &fakeAgentSource{profiles: nil})

	postStream(t, r, `{"message":"在吗","conversationId":"5","agentId":9}`)

	system := streamer.gotMsgs[0].Content
	assert.NotContains(t, system, "【角色设定】", "用户已删光角色 ⇒ 不注入人设段")
	assert.NotContains(t, system, "【护栏】", "无角色段则无护栏段（零回归）")
	assert.Equal(t, goldenBase, system, "无角色时 system 逐字等于现状基线")
	assert.Equal(t, int64(0), chat.gotSendReq.AgentID, "无角色 ⇒ 不盖角色戳（落 NULL）")
}

func TestAIStream_AgentSourceError_DegradesSilently(t *testing.T) {
	streamer := &fakeLLMStreamer{deltas: []string{"嗯"}}
	chat := &fakeChatClient{}
	r := newAgentStreamRouter(streamer, chat, &fakeAgentSource{err: errors.New("user-svc down")})

	w := postStream(t, r, `{"message":"在吗","conversationId":"5","agentId":9}`)

	require.Equal(t, http.StatusOK, w.Code, "角色取数失败不得让对话失败（增强不是依赖）")
	assert.Contains(t, w.Body.String(), "data: [DONE]")
	assert.NotContains(t, streamer.gotMsgs[0].Content, "【角色设定】")
	assert.Equal(t, int64(0), chat.gotSendReq.AgentID)
}

func TestAIStream_NoAgentSource_ZeroRegressionOnPrompt(t *testing.T) {
	streamer := &fakeLLMStreamer{deltas: []string{"嗯"}}
	chat := &fakeChatClient{}
	r := newAgentStreamRouter(streamer, chat, nil) // 未装配角色来源

	postStream(t, r, `{"message":"在吗","conversationId":"5"}`)

	assert.Equal(t, goldenBase, streamer.gotMsgs[0].Content, "未装配 ⇒ 与现状逐字相同")
	assert.Equal(t, int64(0), chat.gotSendReq.AgentID)
}

// ─────────── 两处组装点都要带 agent（防"只接了一条路径"的静默漂移） ───────────

func TestAIStreamHandler_BothAssembleSites_PassAgent(t *testing.T) {
	src, err := os.ReadFile("ai_stream_handler.go")
	require.NoError(t, err)
	text := string(src)

	// llm-gRPC 路径 + HTTP 直连路径各一处 assembleInput
	require.Equal(t, 2, strings.Count(text, "assembleInput{"),
		"两处上游路径都必须经组装层（CL-04 缺陷 11 的不变量）")
	assert.Equal(t, 2, strings.Count(text, "agent:"),
		"两处 assembleInput 都要传 agent —— 只接一条路径时另一条静默无人设（本阶段风险表已列）")
}

// ─────────── mock 兜底路径也要带角色戳（无 LLM key 的 dev 环境同样可追角色） ───────────

func TestAIStream_MockFallback_StampsAgentMessage(t *testing.T) {
	chat := &fakeChatClient{}
	r := newMockStreamRouter(chat, &fakeAgentSource{profiles: twoAgentProfiles()})

	w := postStream(t, r, `{"message":"我今天有点累","conversationId":"5","agentId":9}`)

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int64(9), chat.gotSendReq.AgentID,
		"mock 路径的 AI 回复同样来自该角色 ⇒ 快照一致（否则 dev 环境标注缺失）")
}
