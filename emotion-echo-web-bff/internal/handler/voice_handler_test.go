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
	"strconv"
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
	sto := &fakeUploadStorage{} // E2E-F-113：audioUrl 改相对路径，不再依赖 putURL
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

	// E2E-F-103 + D-11 + E2E-F-113：audioUrl 必须是**单段相对路径 /api/v1/voice/audio/<fileKey>**。
	// 历史 audioUrl = "http://localhost:9000/..."，在非宿主浏览器视角下 host 不可达，
	// 浏览器 <audio> 永远拉不到 metadata（readyState=0、duration=null）。
	// 新实现：audioUrl = "/api/v1/voice/audio/<fileKey>"（fileKey 是 storage PutObject
	// key 砍掉 "voice/" 前缀的单段形式），前端 getFullAudioUrl 拼 apiBase 走 APISIX →
	// BFF audio handler 内部补 "voice/" 前缀后反代 MinIO GetObject。
	audioURL, _ := data["audioUrl"].(string)
	require.True(t, strings.HasPrefix(audioURL, "/api/v1/voice/audio/"),
		"audioUrl 必须以 /api/v1/voice/audio/ 开头（E2E-F-113），实际：%q", audioURL)
	// 单段 + 无 '..' 防御（handler 在 audio 路径会再次检查；这里是 upload 时直接断言）
	audioTail := audioURL[len("/api/v1/voice/audio/"):]
	assert.NotContains(t, audioTail, "/",
		"audioUrl 尾段必须是单段文件键（gin 路由：:filekey 一段），实际：%s", audioTail)
	assert.NotContains(t, audioTail, "..",
		"audioUrl 尾段不能含 '..'（防穿越），实际：%s", audioTail)
	// 与 storage key 关系：tail = key 去掉 "voice/" 前缀
	const voicePrefix = "voice/"
	expectedTail := strings.TrimPrefix(sto.gotKey, voicePrefix)
	assert.Equal(t, expectedTail, audioTail,
		"audioUrl 尾段必须 = storage PutObject key 去掉 voice/ 前缀（让 audio handler 还原）")
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
	// 防御性：保证 register 注册的 2 条路由都对：
	//   - POST /api/v1/voice/upload
	//   - GET  /api/v1/voice/audio/:filekey（E2E-F-113 新增）
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&VoiceHandler{ai: &fakeAIClient{}}).Register(r)
	assert.Equal(t, 2, len(r.Routes()), "VoiceHandler 应注册 2 条路由")

	wantPaths := map[string]string{
		"/api/v1/voice/upload":                 http.MethodPost,
		"/api/v1/voice/audio/:filekey":         http.MethodGet,
	}
	for _, ri := range r.Routes() {
		assert.True(t, strings.HasPrefix(ri.Path, "/api/v1/voice/"),
			"path 应在 /api/v1/voice/ 下：%s", ri.Path)
		want, ok := wantPaths[ri.Path]
		assert.True(t, ok, "未知路由：%s", ri.Path)
		assert.Equal(t, want, ri.Method, "%s 方法不对", ri.Path)
	}
}
// E2E-F-113：GET /api/v1/voice/audio/:filekey 反代 MinIO StreamObject。
//
// 根因（2026-09-22 实测）：audioUrl 由 storage.GetObjectURL 拼成
// http://localhost:9000/<bucket>/<key>（dev PublicBaseURL），
// 在以下两类视角下 host 不可达：
//   1) 浏览器不在宿主机的环境（远程协作、生产部署）；
//   2) 任何通过 web 容器反向代理（如 Nginx / APISIX 网关）而宿主 9000 未转发的部署。
//
// 修法：audioUrl = "/api/v1/voice/audio/<filekey>"（单段，gin 路由限制），
// handler 内部补 "voice/" 前缀还原为 bucket 内完整 key（"voice/<filekey>"），
// 反代 MinIO GetObject 流式输出。所有环境走同一路径，host 与 PublicBaseURL 解耦。
//
// 这里钉的是：audio handler 真实从 storage 拿对象并 io.Copy 给 Gin writer，
// Content-Length 与 Content-Type 与 storage 报的一致，body 与 storage 流的字节对齐。
func TestVoiceHandler_Audio_Success(t *testing.T) {
	const fileKey = "abc-rec.webm"         // path 段（无 "/"）
	const objectKey = "voice/" + fileKey  // handler 内部拼前缀还原
	const wantBody = "fake webm bytes from minio"
	const wantCT = "audio/webm"
	const wantSize = int64(len(wantBody))

	sto := &fakeUploadStorage{
		getObjBytes: []byte(wantBody),
		getObjCT:    wantCT,
	}
	r := newVoiceRouter(&fakeAIClient{}, sto)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/voice/audio/"+fileKey, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, "应为 200")
	assert.Equal(t, wantCT, w.Header().Get("Content-Type"),
		"Content-Type 必须与 storage 报的一致（否则浏览器 <audio> 类型嗅探会失败）")
	assert.Equal(t, strconv.FormatInt(wantSize, 10), w.Header().Get("Content-Length"),
		"Content-Length 必须 = storage 报的真实字节数（否则浏览器无法 seek）")
	assert.Equal(t, []byte(wantBody), w.Body.Bytes(),
		"响应 body 必须严格等于 storage GetObject 流——不能丢、不能改")
	// 端到端契约：handler 内部补 voice/ 前缀后传给 storage
	assert.Equal(t, objectKey, sto.gotGetObjKey,
		"handler 应补 voice/ 前缀后传 storage GetObject（解耦 URL host 与 bucket 路径）")
}

func TestVoiceHandler_Audio_ObjectNotFound_Returns404(t *testing.T) {
	sto := &fakeUploadStorage{getObjErr: errors.New("NoSuchKey: The specified key does not exist.")}
	r := newVoiceRouter(&fakeAIClient{}, sto)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/voice/audio/missing.webm", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code,
		"storage 返 NoSuchKey 必须 404——绝不能 200 返空 body（那会让 <audio> readyState=0）")
}

// 防御性：拒绝任何带 .. 的 :filekey。gin 路由层负责拦 "" 与 "/"（直接在路由
// 树层面不 match）；本 handler 兜底防御 gin 匹配的合法 URL 但 filekey 内容异常。
func TestVoiceHandler_Audio_PathTraversal_Returns400(t *testing.T) {
	sto := &fakeUploadStorage{}
	r := newVoiceRouter(&fakeAIClient{}, sto)

	// 这两个 filekey 是合法 gin 路径段（不含 /），能进 handler
	handlerLevelCases := []struct {
		name string
		key  string
	}{
		{"含 .. 段", "..vhidden"},
		{"中段含 ..", "abc..xyz"},
	}
	for _, tc := range handlerLevelCases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/voice/audio/"+tc.key, nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			assert.Equal(t, http.StatusBadRequest, w.Code,
				"含 '..' 的 filekey 必须 400（防 path traversal），实际：%d", w.Code)
		})
	}

	// gin 路由层（httprouter）直接 404；记作"路由层拒绝"，handler 不参与
	t.Run("含 /（gin 路由层 404）", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/voice/audio/voice/foo/bar", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code,
			"含 / 应被 gin 路由层 404")
	})
	t.Run("空 filekey（gin 路由层 404）", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/voice/audio/", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code,
			"空 filekey 应被 gin 路由层 404")
	})
	// 任何 handler-level 进来的 case 都应被 handler 拦下，**不应**调 storage
	assert.Empty(t, sto.gotGetObjKey,
		"被 handler 拦截的非法 key 不应调 storage GetObject")
}

// TestVoiceHandler_Upload_StorageNotConfigured_Returns503 守住 D-11 的接口契约：
// storage 未配置（dev 环境变量缺失 / 启动失败）时不应让请求穿透到 MinIO，否则
// 会留下「请求成功但音频实际没存」的不可见失败（D-12 的同类教训）。（dev 环境变量缺失 / 启动失败）时不应让请求穿透到 MinIO，否则
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
