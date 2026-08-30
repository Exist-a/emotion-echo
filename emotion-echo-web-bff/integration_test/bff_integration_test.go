//go:build integration
// +build integration

// Package integration_test — bff_integration_test.go
//
// Stage 30 / stage-30-web-bff.md T5.61-63: BFF 集成测试
//
// 覆盖：
//   61. gRPC dial：真 net.Listener + 真 grpc.Server + fake service → EmotionQueryClient.ByMessage
//   62. SSE E2E：完整 BFF Gin router 装配 → POST /api/v1/ai/stream → SSE 事件序列断言
//   63. TTS stream byte-for-byte：完整 BFF router → POST /api/v1/tts/stream → 字节一致
//
// 跑：go test -tags integration -v -timeout 2m ./integration_test/...
package integration_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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
	r.POST("/api/v1/ai/stream", handler.NewAIStreamHandler(fakeAI))
	handler.NewTTSHandler(fakeAI, &fakeIntegrationXTTS{body: "RIFFWAVE"}).Register(r)

	// 业务 handler
	handler.NewUserHandler(&fakeIntegrationUser{}).Register(r)
	handler.NewChatHandler(&fakeIntegrationChat{}).Register(r)
	return r
}

// TestBFF_SSEE2E_AIStream T5.62：POST /api/v1/ai/stream → SSE 事件序列
func TestBFF_SSEE2E_AIStream(t *testing.T) {
	router := buildBFFRouter()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/ai/stream",
		bytes.NewReader([]byte(`{"messageId":42}`)))
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/event-stream", w.Header().Get("Content-Type"))
	assert.Equal(t, "no", w.Header().Get("X-Accel-Buffering"))

	body := w.Body.String()
	assert.True(t, strings.Contains(body, "event: analysis\n"), "应含 analysis 事件")
	assert.True(t, strings.Contains(body, "event: done\n"), "应含 done 事件")
	assert.True(t, strings.Contains(body, `"messageId":42`), "SSE 数据应含 messageId")
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
func (f *fakeIntegrationXTTS) Health(_ context.Context) (*downstream.XTTSHealthResp, error) {
	return &downstream.XTTSHealthResp{Status: "ok", ModelLoaded: true}, nil
}

type fakeIntegrationUser struct{}

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
func (f *fakeIntegrationChat) PinConversation(_ context.Context, _ int64) error   { return nil }

var _ = time.Second // 保留 time import（部分平台编译）
