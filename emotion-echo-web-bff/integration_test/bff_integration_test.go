//go:build integration
// +build integration

// Package integration_test — bff_integration_test.go
//
// Stage 30 / stage-30-web-bff.md T5.61-63: BFF 集成测试
//
// 覆盖：
//  61. gRPC dial：真 net.Listener + 真 grpc.Server + fake service → EmotionQueryClient.ByMessage
//  62. SSE E2E：完整 BFF Gin router 装配 → POST /api/v1/ai/stream → SSE 事件序列断言
//  63. TTS stream byte-for-byte：完整 BFF router → POST /api/v1/tts/stream → 字节一致
//
// 跑：go test -tags integration -v -timeout 2m ./integration_test/...
package integration_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"emotion-echo-web-bff/internal/config"
	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/handler"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// =====================================================
// 61. gRPC dial（真网络 + 真 server）
// =====================================================

// integrationFakeQuerySrv 实现 EmotionQueryServiceServer
type integrationFakeQuerySrv struct {
	emotionquery.UnimplementedEmotionQueryServiceServer
}

func (f *integrationFakeQuerySrv) GetEmotionByMessage(_ context.Context, req *emotionquery.GetEmotionByMessageRequest) (*emotionquery.Emotion, error) {
	return &emotionquery.Emotion{
		Id: 1, MessageId: req.MessageId, ConversationId: 10,
		PrimaryEmotion: "happy", SentimentScore: 0.7, Confidence: 0.9, Model: "integration-stub", CreatedAtMs: 123,
	}, nil
}

func (f *integrationFakeQuerySrv) GetEmotionByConversation(_ context.Context, req *emotionquery.GetEmotionByConversationRequest) (*emotionquery.EmotionList, error) {
	return &emotionquery.EmotionList{
		Items: []*emotionquery.Emotion{{Id: 1, MessageId: 1, ConversationId: req.ConversationId, PrimaryEmotion: "calm"}},
		Total: 1,
	}, nil
}

// TestBFF_GRPCDial_EmotionQueryByMessage T5.61：真 gRPC server + 真 client dial
func TestBFF_GRPCDial_EmotionQueryByMessage(t *testing.T) {
	// 真 net.Listener + 真 grpc.Server
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	gs := grpc.NewServer()
	emotionquery.RegisterEmotionQueryServiceServer(gs, &integrationFakeQuerySrv{})
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(gs.Stop)

	// 真 dial（NewClient 方式，非 bufconn）
	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	client := downstream.NewEmotionQueryClient(conn)
	e, err := client.ByMessage(context.Background(), 42)
	require.NoError(t, err)
	require.NotNil(t, e)
	assert.Equal(t, int64(42), e.MessageId)
	assert.Equal(t, "happy", e.PrimaryEmotion)
}

// =====================================================
// 62. SSE E2E（完整 BFF 装配）
// =====================================================

// buildBFFRouter 装配完整 BFF Gin router（真实 handler 链路，mock 下游 client）
func buildBFFRouter() *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()

	// fake AI client（EmotionQueryClient + AIClient）
	fakeAI := &fakeIntegrationAI{emotion: &emotionquery.Emotion{
		MessageId: 42, ConversationId: 10, PrimaryEmotion: "happy", SentimentScore: 0.7, Model: "stub",
	}}

	// 注册 ai_stream + tts_stream（与 main.go registerRoutes 同构的最小装配）
	// ⚠️ 签名早已从 `NewAIStreamHandler(aiClient)` 改为 `NewAIStreamHandler(cfg config.Config)`
	//    （下游拆成 AIStreamDeps{LLM,Files,Chat,Personality,Emotion}），本集成测试没跟着改
	//    ⇒ `go test -tags integration` 编译不过；而默认 `go test ./...` / `go vet ./...` /
	//    CI go-test **全部跳过 build tag 下的文件**，所以坏了很久没人发现（账本 E2E-F-167）。
	//    传零值 Config：APIKey 为空 ⇒ 走 mock 共情回复分支，正是本用例要验的路径（不依赖真实 LLM）。
	r.POST("/api/v1/ai/stream", handler.NewAIStreamHandler(config.Config{}))
	handler.NewTTSHandler(fakeAI, &fakeIntegrationXTTS{body: "RIFFWAVE"}, downstream.NewLocalTTSProvider(&fakeIntegrationXTTS{body: "RIFFWAVE"})).Register(r) // E2E-F-198: phonemes goes through provider (local wraps the same fake)

	// 业务 handler
	handler.NewUserHandler(&fakeIntegrationUser{}).Register(r)
	handler.NewChatHandler(&fakeIntegrationChat{}).Register(r)
	return r
}

// TestBFF_SSEE2E_AIStream T5.62：POST /api/v1/ai/stream → SSE 事件序列
func TestBFF_SSEE2E_AIStream(t *testing.T) {
	router := buildBFFRouter()
	// ⚠️ 请求体契约也演进过：早期是 `{"messageId":42}`，现为 OpenAI 兼容的
	// `{"messages":[{"role":"user","content":"..."}]}` —— 少传 messages 会直接
	// 400 "validation: messages is required"（handler:371-372）。
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream",
		bytes.NewReader([]byte(`{"messages":[{"role":"user","content":"今天有点累"}]}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, "no", w.Header().Get("X-Accel-Buffering"))

	body := w.Body.String()
	// ⚠️ 断言契约已随实现演进过一次：早期 ai/stream 复用 sse.StreamAnalysis 发
	// `event: analysis` / `event: done`，现已改为 **OpenAI 兼容流式协议** ——
	// `data: {"choices":[{"delta":{"content":"..."}}]}` 分块 + 末帧 `data: [DONE]`。
	// 原断言即使能编译也永远红（analysis 事件已不由该端点发出）。
	assert.True(t, strings.Contains(body, `"delta"`), "应含 OpenAI 风格的 delta 块，实际: %s", body)
	assert.True(t, strings.Contains(body, `"content"`), "delta 里应含 content，实际: %s", body)
	assert.True(t, strings.Contains(body, "data: [DONE]"), "末帧应为 [DONE] 结束标记，实际: %s", body)
	assert.False(t, strings.Contains(body, "event: analysis"), "该端点已不发 analysis 事件（那是 sse.StreamAnalysis 的旧协议）")
}

// =====================================================
// 63. TTS stream byte-for-byte
// =====================================================

// TestBFF_TTSStream_ByteForByte T5.63：POST /api/v1/tts/stream → 字节一致
func TestBFF_TTSStream_ByteForByte(t *testing.T) {
	router := buildBFFRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/tts/stream",
		bytes.NewReader([]byte(`{"text":"你好","language":"zh-cn"}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "audio/wav", w.Header().Get("Content-Type"))
	assert.Equal(t, []byte("RIFFWAVE"), w.Body.Bytes(), "TTS stream 应逐字节转发（byte-for-byte）")
}

// =====================================================
// fake 下游（集成测试用）
// =====================================================

type fakeIntegrationAI struct {
	emotion *emotionquery.Emotion
}

func (f *fakeIntegrationAI) ByMessage(_ context.Context, messageID int64) (*emotionquery.Emotion, error) {
	f.emotion.MessageId = messageID
	return f.emotion, nil
}
func (f *fakeIntegrationAI) ByConversation(_ context.Context, _ int64, _ int) ([]*emotionquery.Emotion, int32, error) {
	return []*emotionquery.Emotion{f.emotion}, 1, nil
}
func (f *fakeIntegrationAI) MultiModalAnalyze(_ context.Context, _ downstream.MultiModalAnalyzeReq) (*downstream.MultiModalAnalyzeResp, error) {
	return &downstream.MultiModalAnalyzeResp{Kind: "text", Emotion: "happy"}, nil
}
func (f *fakeIntegrationAI) SynthesizeSpeech(_ context.Context, _ downstream.SynthesizeSpeechReq) (*downstream.SynthesizeSpeechResp, error) {
	return &downstream.SynthesizeSpeechResp{Audio: "base64", SampleRate: 24000, Mime: "audio/wav", Bytes: 4, Text: "x", Language: "zh-cn"}, nil
}
func (f *fakeIntegrationAI) AIHealth(_ context.Context) (*downstream.AIHealthResp, error) {
	return &downstream.AIHealthResp{Time: 1, AllHealthy: true}, nil
}

type fakeIntegrationXTTS struct {
	body string
}

func (f *fakeIntegrationXTTS) Stream(_ context.Context, _ downstream.TTSStreamReq) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(f.body)), nil
}

// Phonemes 是 E2E-17 D-03 之后加进 XTTSClient 的方法（真口型同步：前端按
// currentTime 驱动 BlendShape）。本 fake 一直没跟上 —— 与上面两处同型：
// **接口长方法 / 改签名，集成测试没跟着改**。build tag 让它躲过了所有默认门禁。
func (f *fakeIntegrationXTTS) Phonemes(_ context.Context, req downstream.TTSPhonemesReq) (*downstream.XTTSPhonemesResp, error) {
	return &downstream.XTTSPhonemesResp{
		Audio:      base64.StdEncoding.EncodeToString([]byte(f.body)),
		SampleRate: 24000,
		Text:       req.Text,
		Language:   req.Language,
		Phonemes:   []downstream.XTTSPhoneme{{Char: "a", Start: 0, Duration: 0.1}},
		Duration:   0.1,
	}, nil
}

func (f *fakeIntegrationXTTS) Health(_ context.Context) (*downstream.XTTSHealthResp, error) {
	return &downstream.XTTSHealthResp{Status: "ok", ModelLoaded: true}, nil
}

type fakeIntegrationUser struct{}

// 以下 6 个方法是 E2E-12（找回密码/密保）之后 UserClient 新增的，本 fake 一直没跟上。
// 与上面 ai_stream / XTTSPhonemes 同型：**生产接口长方法，集成测试的替身没跟着长**，
// 而 build tag 让它躲过所有默认门禁，坏了整整两个阶段。
// 语义纪律：未被本文件用例覆盖的路径一律 panic（与同文件其它 fake 一致），
// 而不是返回零值 —— 静默返回零值会把"这个用例根本没走到那条路"变成绿灯。
func (f *fakeIntegrationUser) ResetPassword(_ context.Context, _ downstream.ResetPasswordReq) (*downstream.UserInfo, error) {
	panic("fakeIntegrationUser.ResetPassword should not be called in this suite")
}
func (f *fakeIntegrationUser) Login(_ context.Context, _, _ string) (*downstream.UserInfo, error) {
	panic("fakeIntegrationUser.Login should not be called in this suite")
}
func (f *fakeIntegrationUser) Register(_ context.Context, _, _, _ string, _ []downstream.SecurityQuestion) (*downstream.UserInfo, error) {
	panic("fakeIntegrationUser.Register should not be called in this suite")
}
func (f *fakeIntegrationUser) VerifySecurityAnswer(_ context.Context, _ int64, _ int, _ string) error {
	panic("fakeIntegrationUser.VerifySecurityAnswer should not be called in this suite")
}
func (f *fakeIntegrationUser) VerifySecurityAnswerByUsername(_ context.Context, _ string, _ int, _ string) error {
	panic("fakeIntegrationUser.VerifySecurityAnswerByUsername should not be called in this suite")
}
func (f *fakeIntegrationUser) GetSecurityQuestionsByUsername(_ context.Context, _ string) ([]downstream.SecurityQuestionInfo, error) {
	panic("fakeIntegrationUser.GetSecurityQuestionsByUsername should not be called in this suite")
}

func (f *fakeIntegrationUser) GetMe(_ context.Context) (*downstream.UserInfo, error) {
	return &downstream.UserInfo{UserID: 1, Account: "it-user", Nickname: "IT"}, nil
}
func (f *fakeIntegrationUser) UpdateMe(_ context.Context, _ downstream.UpdateProfileReq) (*downstream.UserInfo, error) {
	return &downstream.UserInfo{UserID: 1}, nil
}
func (f *fakeIntegrationUser) GetByID(_ context.Context, _ int64) (*downstream.UserInfo, error) {
	return &downstream.UserInfo{UserID: 1}, nil
}

type fakeIntegrationChat struct{}

// ListConversations 是 E2E-11（会话列表侧栏标题）之后加进 ChatClient 的方法。
// 同型：生产接口长方法、集成测试替身没跟着长，被 build tag 藏了两个阶段。
// 本文件用例不覆盖会话列表，故 panic —— 静默返回空切片会把"没走到"变成绿灯。
func (f *fakeIntegrationChat) ListConversations(_ context.Context, _, _ int) ([]downstream.ConversationView, bool, error) {
	panic("fakeIntegrationChat.ListConversations should not be called in this suite")
}

func (f *fakeIntegrationChat) CreateConversation(_ context.Context, _ downstream.CreateConversationReq) (*downstream.ConversationView, error) {
	return &downstream.ConversationView{ID: 1, UserID: 1, Title: "it"}, nil
}
func (f *fakeIntegrationChat) SendMessage(_ context.Context, _ int64, _ downstream.SendMessageReq) (*downstream.MessageView, error) {
	return &downstream.MessageView{ID: 1, Content: "hi"}, nil
}
func (f *fakeIntegrationChat) ListMessages(_ context.Context, _ int64, _ int) ([]downstream.MessageView, error) {
	return []downstream.MessageView{{ID: 1, Content: "hi"}}, nil
}
func (f *fakeIntegrationChat) DeleteConversation(_ context.Context, _ int64) error { return nil }
func (f *fakeIntegrationChat) PinConversation(_ context.Context, _ int64, _ bool) error {
	return nil
}
func (f *fakeIntegrationChat) UpdateConversation(_ context.Context, conversationID int64, title string) (*downstream.UpdateConversationResp, error) {
	return &downstream.UpdateConversationResp{Success: true, Id: conversationID, Title: title}, nil
}

var _ = time.Second // 保留 time import（部分平台编译）
