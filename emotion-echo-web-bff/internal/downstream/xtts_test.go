// Package downstream — xtts_test.go
//
// Stage 30 / stage-30-web-bff.md T2.9-11 RED: XTTSClient 契约测试
//
// 用 httptest mock XTTS，断言：
//   - Stream 返回 raw WAV 字节流（io.ReadCloser）
//   - Health 反序列化正确
//   - 下游 4xx → FastAPI {"detail"} 错误提取
package downstream

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestXTTSClient_Stream_ReturnsWAVBytes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/tts_stream", r.URL.Path)

		var req TTSStreamReq
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "你好世界", req.Text)

		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte("RIFF....WAVEfmtdata")) // 伪 WAV 头
	}))
	defer srv.Close()

	c := NewXTTSClient(XTTSClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	stream, err := c.Stream(context.Background(), TTSStreamReq{Text: "你好世界", Language: "zh-cn", Speed: 0.75})
	require.NoError(t, err)
	require.NotNil(t, stream)
	defer stream.Close()

	b, err := io.ReadAll(stream)
	require.NoError(t, err)
	assert.Equal(t, "RIFF....WAVEfmtdata", string(b), "Stream 应返回 raw WAV 字节")
}

func TestXTTSClient_Health_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/health", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(XTTSHealthResp{Status: "ok", ModelLoaded: true, ModelType: "XTTS-v2"})
	}))
	defer srv.Close()

	c := NewXTTSClient(XTTSClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	resp, err := c.Health(context.Background())
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.ModelLoaded)
	assert.Equal(t, "XTTS-v2", resp.ModelType)
}

func TestXTTSClient_Stream_Upstream400_ReturnsDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"detail": "Text is required"})
	}))
	defer srv.Close()

	c := NewXTTSClient(XTTSClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	_, err := c.Stream(context.Background(), TTSStreamReq{Text: ""})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Text is required", "应提取 FastAPI detail 错误")
}

func TestXTTSClient_Stream_EmptyBaseURL_ReturnsError(t *testing.T) {
	c := NewXTTSClient(XTTSClientOptions{BaseURL: "", TimeoutMs: 1000})
	_, err := c.Stream(context.Background(), TTSStreamReq{Text: "x"})
	require.Error(t, err)
}

func TestXTTSClient_Stream_NoAuthHeader(t *testing.T) {
	// XTTS 无鉴权：验证 client 不注入 Authorization（后端不认）
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Empty(t, r.Header.Get("Authorization"), "XTTS 不应收到 Authorization 头")
		w.Header().Set("Content-Type", "audio/wav")
		_, _ = w.Write([]byte(strings.Repeat("x", 16)))
	}))
	defer srv.Close()

	c := NewXTTSClient(XTTSClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	stream, err := c.Stream(context.Background(), TTSStreamReq{Text: "hi"})
	require.NoError(t, err)
	stream.Close()
}
