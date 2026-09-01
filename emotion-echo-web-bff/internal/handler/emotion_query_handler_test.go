// Package handler — emotion_query_handler_test.go
//
// Stage 34 · PR-17 RED
//
// 新增端点：GET /api/v1/emotion/message/:id/fused → BFF → ai-svc gRPC GetFusedEmotion
//
// 当前 RED：测试引用未定义的符号（GetFusedEmotion、handler.Register / GetFusedEmotionHandler /
// EmotionQueryClient 扩展方法 ByFusedMessage）→ 必须失败。
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeEmotionQueryClientV2 实现 emotion_query.EmotionQueryClient 扩展版
// （增加了 ByFusedMessage 方法）。
//
// 当前 EmotionQueryClient 接口只有 ByMessage / ByConversation；RED 阶段这个 fake
// 必须显式满足扩展接口（PR-18 会扩展 EmotionQueryClient interface）。
type fakeEmotionQueryClientV2 struct {
	byFusedMessage      *emotionquery.FusedEmotion
	byFusedMessageErr   error
	byFusedMessageCalls int32
}

func (f *fakeEmotionQueryClientV2) ByMessage(_ context.Context, _ int64) (*emotionquery.Emotion, error) {
	return nil, nil // unused in this test
}
func (f *fakeEmotionQueryClientV2) ByConversation(_ context.Context, _ int64, _ int) ([]*emotionquery.Emotion, int32, error) {
	return nil, 0, nil // unused
}
func (f *fakeEmotionQueryClientV2) ByFusedMessage(_ context.Context, _ int64) (*emotionquery.FusedEmotion, error) {
	f.byFusedMessageCalls++
	return f.byFusedMessage, f.byFusedMessageErr
}

// TestEmotionQueryHandler_FusedByMessage_Success 验证 fused 端点路由 + handler 委派给 client。
func TestEmotionQueryHandler_FusedByMessage_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := &fakeEmotionQueryClientV2{
		byFusedMessage: &emotionquery.FusedEmotion{
			MessageId:      100,
			PrimaryEmotion: "sad",
			FusionMethod:   "llm",
		},
	}

	h := NewEmotionQueryHandler(client)
	r := gin.New()
	h.Register(r)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/emotion/message/100/fused", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	t.Logf("response body: %s", w.Body.String())

	var resp struct {
		Code int                       `json:"code"`
		Data map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, 0, resp.Code)
	require.Contains(t, resp.Data, "fused", "data must contain 'fused' key")

	var fused emotionquery.FusedEmotion
	require.NoError(t, json.Unmarshal(resp.Data["fused"], &fused))
	assert.Equal(t, "sad", fused.PrimaryEmotion)
	assert.Equal(t, "llm", fused.FusionMethod)
	assert.Equal(t, int64(100), fused.MessageId)
	assert.Equal(t, int32(1), client.byFusedMessageCalls, "client.ByFusedMessage must be called once")
}

// TestEmotionQueryHandler_FusedByMessage_NotFound_Returns404 client 返 nil → handler 返 404
func TestEmotionQueryHandler_FusedByMessage_NotFound_Returns404(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := &fakeEmotionQueryClientV2{byFusedMessage: nil, byFusedMessageErr: nil}

	h := NewEmotionQueryHandler(client)
	r := gin.New()
	h.Register(r)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/emotion/message/999/fused", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusNotFound, w.Code)
}

// TestEmotionQueryHandler_FusedByMessage_InvalidID_Returns400 messageID <= 0 → 400
func TestEmotionQueryHandler_FusedByMessage_InvalidID_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := &fakeEmotionQueryClientV2{}

	h := NewEmotionQueryHandler(client)
	r := gin.New()
	h.Register(r)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/emotion/message/abc/fused", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
}

// TestEmotionQueryHandler_FusedByMessage_ClientError_Propagates502 client 返 error（非 gRPC NotFound/Unimplemented）→ statusFor 默认 502
func TestEmotionQueryHandler_FusedByMessage_ClientError_Propagates502(t *testing.T) {
	gin.SetMode(gin.TestMode)
	client := &fakeEmotionQueryClientV2{byFusedMessageErr: assertAnError()}

	h := NewEmotionQueryHandler(client)
	r := gin.New()
	h.Register(r)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/emotion/message/100/fused", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadGateway, w.Code)
}

func assertAnError() error { return errStub }

var errStub = simpleErr("downstream unavailable")

type simpleErr string

func (e simpleErr) Error() string { return string(e) }