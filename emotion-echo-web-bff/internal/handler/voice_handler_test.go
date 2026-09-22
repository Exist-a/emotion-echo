// Package handler — voice_handler_test.go
//
// Sprint 1 PR-4c-1: voice_handler 单元测试
//
// 行为契约：
//   - POST /api/v1/voice/upload 接 multipart (conversationId + file)
//   - 调 ai-svc MultiModalAnalyze(kind=audio, file)
//   - 返回 {messageId, transcript, emotion, audioUrl}
//   - ai-svc 不可达时返 503 (而非 500)
//   - 缺 file 字段时返 400

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/storage"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeAIClient 模拟 AIClient
type fakeAIClient struct {
	resp *downstream.MultiModalAnalyzeResp
	err  error
	// 记录调用时的 kind 用于断言
	gotKind string
	// E2E-F-109：记录收到 ctx，便于断言 BFF 注入了 userID
	gotCtx context.Context
}

func (f *fakeAIClient) MultiModalAnalyze(ctx context.Context, req downstream.MultiModalAnalyzeReq) (*downstream.MultiModalAnalyzeResp, error) {
	f.gotKind = req.Kind
	f.gotCtx = ctx
	if f.err != nil {
		return nil, f.err
	}
	return f.resp, nil
}

func (f *fakeAIClient) SynthesizeSpeech(ctx context.Context, req downstream.SynthesizeSpeechReq) (*downstream.SynthesizeSpeechResp, error) {
	return nil, nil
}

func (f *fakeAIClient) AIHealth(ctx context.Context) (*downstream.AIHealthResp, error) {
	return &downstream.AIHealthResp{}, nil
}

// newVoiceRouter 最小路由（仅 voice_handler）
// sto=nil 仍合法：voice_handler 需 sto!=nil 才落 MinIO；测试可在 nil 时显式断言。
func newVoiceRouter(ai downstream.AIClient, sto storage.StorageClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&VoiceHandler{ai: ai, storage: sto}).Register(r)
	return r
}

func TestVoiceHandler_Upload_Success(t *testing.T) {
	fake := &fakeAIClient{
		resp: &downstream.MultiModalAnalyzeResp{
			Kind:       "audio",
			Emotion:    "neutral",
			Confidence: 0.92,
			Transcript: "你好世界",
		},
	}
	sto := &fakeUploadStorage{putURL: "http://localhost:9000/avatars/voice/42-rec.webm"}
	r := newVoiceRouter(fake, sto)

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	require.NoError(t, mw.WriteField("conversationId", "42"))
	fw, err := mw.CreateFormFile("file", "recording.webm")
	require.NoError(t, err)
	_, _ = io.WriteString(fw, "fake webm bytes")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/voice/upload", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	// E2E-F-109：模拟 APISIX 注入 X-User-Id（这是 BFF 主干约定，chat_handler /
	// avatar_handler 都已使用 session.WithRequestAuth(c) 透传到 ctx，
	// voice_handler 是新写的，漏了这一步 ⇒ ai-svc gRPC 拦截器拒请求）。
	req.Header.Set("X-User-Id", "42")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	var got map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "audio", fake.gotKind, "应传 kind=audio")

	// E2E-F-109：ctx 必须带 userID（downstream.UserIDFromContext 应能取出 42）。
	// 若 handler 用了 c.Request.Context() 直接传，ctx 里没有 userID，
	// ai-svc gRPC 拦截器就会拒；这条断言钉住"ctx 已注入 userID"这一契约。
	uidFromCtx, ok := downstream.UserIDFromContext(fake.gotCtx)
	assert.True(t, ok, "ctx 必须带 userID 键（session.WithRequestAuth 注入），实际 ok=false")
	assert.Equal(t, int64(42), uidFromCtx,
		"ctx 必须携带 userID=42 ⇒ ai-svc gRPC metadata 不再 missing")

	// E2E-F-103：成功数据必须放在 data 内（resp.go OK() 契约）。
	// 历史断言直接在顶层读 transcript/emotion，把"缺 data 包装"的错误结构固化成
	// 契约 ⇒ 前端 useApi（统一取 data.data）拿到 undefined，录音后整条链路静默无反馈。
	data, ok := got["data"].(map[string]any)
	require.True(t, ok, "响应必须含 data 对象（resp.go OK() 契约），实际：%v", got)
	assert.Equal(t, "你好世界", data["transcript"])
	assert.Equal(t, "neutral", data["emotion"])
	assert.NotEmpty(t, data["messageId"], "messageId 必须生成")

	// E2E-F-103 + D-11：音频必须落 MinIO 且 audioUrl 回填（=fake sto 配置的 URL）。
	// 历史 audioUrl 恒为 "" ⇒ [id].vue:13-22 的 VoiceMessage 分支永不可达。
	assert.Equal(t, "http://localhost:9000/avatars/voice/42-rec.webm", data["audioUrl"],
		"audioUrl 必须回填 MinIO URL，使前端 <audio src> 可回放")
	// storage 必须收到一次 PutObject 调用，key 以 voice/ 开头（隔离头像/通用上传）
	assert.True(t, strings.HasPrefix(sto.gotKey, "voice/"),
		"audio key 应以 voice/ 开头以隔离 bucket 前缀: %s", sto.gotKey)
	// multipart part 边界可能让 storage 收到的 size 比原始多 1-2 字节（CRLF）
	assert.GreaterOrEqual(t, sto.gotSize, int64(14),
		"storage 收到的 size 应至少含 'fake webm bytes'(14) 字节: got=%d", sto.gotSize)
}

func TestVoiceHandler_Upload_AIServiceError_Returns503(t *testing.T) {
	// 用真 connection error 触发 isConnectionErr 分支
	fake := &fakeAIClient{err: errors.New("dial tcp 127.0.0.1:8891: connect: connection refused")}
	r := newVoiceRouter(fake, &fakeUploadStorage{})

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	mw.WriteField("conversationId", "1")
	fw, _ := mw.CreateFormFile("file", "x.webm")
	_, _ = io.WriteString(fw, "data")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/voice/upload", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// ai-svc 不可达 → 503 (而非 500)，让前端能区分
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
}

func TestVoiceHandler_Upload_MissingFile_Returns400(t *testing.T) {
	fake := &fakeAIClient{}
	r := newVoiceRouter(fake, &fakeUploadStorage{})

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	mw.WriteField("conversationId", "1")
	// 故意缺 file 字段
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/voice/upload", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	// ai 不应被调用
	assert.Empty(t, fake.gotKind)
}

func TestVoiceHandler_Register_PathContract(t *testing.T) {
	// 防御性：保证 register 注册的是 POST /api/v1/voice/upload
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&VoiceHandler{ai: &fakeAIClient{}}).Register(r)
	assert.Equal(t, 1, len(r.Routes()), "VoiceHandler 应注册 1 条路由")
	for _, ri := range r.Routes() {
		assert.True(t, strings.HasPrefix(ri.Path, "/api/v1/voice/"), "path 应在 /api/v1/voice/ 下")
		assert.Equal(t, http.MethodPost, ri.Method)
	}
}
// TestVoiceHandler_Upload_StorageNotConfigured_Returns503 守住 D-11 的接口契约：
// storage 未配置（dev 环境变量缺失 / 启动失败）时不应让请求穿透到 MinIO，否则
// 会留下「请求成功但音频实际没存」的不可见失败（D-12 的同类教训）。
func TestVoiceHandler_Upload_StorageNotConfigured_Returns503(t *testing.T) {
	fake := &fakeAIClient{
		resp: &downstream.MultiModalAnalyzeResp{Kind: "audio", Emotion: "neutral", Confidence: 0.5},
	}
	r := newVoiceRouter(fake, nil) // storage=nil

	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	mw.WriteField("conversationId", "1")
	fw, _ := mw.CreateFormFile("file", "x.webm")
	_, _ = io.WriteString(fw, "audio-bytes")
	mw.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/voice/upload", body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusServiceUnavailable, w.Code,
		"storage 未配置必须返 503 而不是 200，否则会留下「假成功」")
}
