// Package handler — multimodal_tts_upload_handler_test.go
//
// Stage 30 / stage-30-web-bff.md T4.53/55/58 RED: multimodal + tts + upload handler 契约测试
package handler

import (
	"bytes"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// fakeAIForTTS 实现 downstream.AIClient
type fakeAIForTTS struct {
	multimodal *downstream.MultiModalAnalyzeResp
	tts        *downstream.SynthesizeSpeechResp
	health     *downstream.AIHealthResp
	err        error
}

func (f *fakeAIForTTS) MultiModalAnalyze(_ context.Context, req downstream.MultiModalAnalyzeReq) (*downstream.MultiModalAnalyzeResp, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.multimodal, nil
}
func (f *fakeAIForTTS) SynthesizeSpeech(_ context.Context, _ downstream.SynthesizeSpeechReq) (*downstream.SynthesizeSpeechResp, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.tts, nil
}
func (f *fakeAIForTTS) AIHealth(_ context.Context) (*downstream.AIHealthResp, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.health, nil
}

// fakeXTTSForTTS 实现 downstream.XTTSClient
type fakeXTTSForTTS struct {
	streamBody string
	err        error
}

func (f *fakeXTTSForTTS) Stream(_ context.Context, _ downstream.TTSStreamReq) (io.ReadCloser, error) {
	if f.err != nil {
		return nil, f.err
	}
	return io.NopCloser(strings.NewReader(f.streamBody)), nil
}
func (f *fakeXTTSForTTS) Health(_ context.Context) (*downstream.XTTSHealthResp, error) {
	return &downstream.XTTSHealthResp{Status: "ok", ModelLoaded: true}, nil
}

func TestMultimodalHandler_Text_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&MultimodalHandler{ai: &fakeAIForTTS{multimodal: &downstream.MultiModalAnalyzeResp{
		Kind: "text", Emotion: "happy", Confidence: 0.8, Model: "keyword-stub-v1",
	}}}).Register(r)

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("kind", "text")
	_ = w.WriteField("text", "今天很开心")
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/multimodal/analyze", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Body.String(), `"happy"`)
}

func TestMultimodalHandler_Image_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&MultimodalHandler{ai: &fakeAIForTTS{multimodal: &downstream.MultiModalAnalyzeResp{
		Kind: "image", Emotion: "calm", Confidence: 0.9,
	}}}).Register(r)

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("kind", "image")
	fw, _ := w.CreateFormFile("file", "photo.jpg")
	_, _ = fw.Write([]byte("fake-image"))
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/multimodal/analyze", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Body.String(), `"calm"`)
}

func TestMultimodalHandler_ImageNoFile_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&MultimodalHandler{ai: &fakeAIForTTS{}}).Register(r)

	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("kind", "image")
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/multimodal/analyze", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
	assert.Contains(t, resp.Body.String(), "file is required")
}

func TestTTSHandler_Synthesize_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&TTSHandler{ai: &fakeAIForTTS{tts: &downstream.SynthesizeSpeechResp{
		Audio: "base64wav", SampleRate: 24000, Mime: "audio/wav", Bytes: 1024, Text: "你好", Language: "zh-cn",
	}}}).Register(r)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tts/synthesize",
		bytes.NewReader([]byte(`{"text":"你好","language":"zh-cn","speed":0.75}`)))
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Contains(t, resp.Body.String(), `"base64wav"`)
	assert.Contains(t, resp.Body.String(), `"sampleRate":24000`)
}

func TestTTSHandler_Stream_ForwardsWAV(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&TTSHandler{xtts: &fakeXTTSForTTS{streamBody: "RIFF....WAVEfmtdata"}}).Register(r)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tts/stream",
		bytes.NewReader([]byte(`{"text":"你好","language":"zh-cn"}`)))
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusOK, resp.Code)
	assert.Equal(t, "audio/wav", resp.Header().Get("Content-Type"))
	assert.Equal(t, "no", resp.Header().Get("X-Accel-Buffering"))
	assert.Equal(t, "RIFF....WAVEfmtdata", resp.Body.String(), "raw WAV 应原样转发")
}

func TestTTSHandler_Stream_EmptyText_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&TTSHandler{xtts: &fakeXTTSForTTS{}}).Register(r)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/tts/stream",
		bytes.NewReader([]byte(`{"text":""}`)))
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadRequest, resp.Code)
}

func TestUploadHandler_Returns502(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&UploadHandler{}).Register(r)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/uploads/image", nil)
	resp := httptest.NewRecorder()
	r.ServeHTTP(resp, req)

	assert.Equal(t, http.StatusBadGateway, resp.Code)
	assert.Contains(t, resp.Body.String(), "not implemented")
}
