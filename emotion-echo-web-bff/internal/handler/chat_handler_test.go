// Package handler — chat_handler_test.go
//
// Stage 30 / stage-30-web-bff.md T4.38 RED: chat handler 契约测试
package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// fakeChatClient 实现 downstream.ChatClient
type fakeChatClient struct {
	conv      *downstream.ConversationView
	msg       *downstream.MessageView
	messages  []downstream.MessageView
	delErr    error
	listErr   error
	convErr   error
	sendErr   error
	gotConvID int64
	gotLimit  int
}

func (f *fakeChatClient) CreateConversation(_ context.Context, _ downstream.CreateConversationReq) (*downstream.ConversationView, error) {
	if f.convErr != nil {
		return nil, f.convErr
	}
	return f.conv, nil
}
func (f *fakeChatClient) SendMessage(_ context.Context, conversationID int64, _ downstream.SendMessageReq) (*downstream.MessageView, error) {
	f.gotConvID = conversationID
	if f.sendErr != nil {
		return nil, f.sendErr
	}
	return f.msg, nil
}
func (f *fakeChatClient) ListMessages(_ context.Context, conversationID int64, limit int) ([]downstream.MessageView, error) {
	f.gotConvID, f.gotLimit = conversationID, limit
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.messages, nil
}
func (f *fakeChatClient) DeleteConversation(_ context.Context, conversationID int64) error {
	f.gotConvID = conversationID
	return f.delErr
}
func (f *fakeChatClient) PinConversation(_ context.Context, _ int64) error { return nil }

func newChatRouter(client downstream.ChatClient) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	(&ChatHandler{chat: client}).Register(r)
	return r
}

func TestChatHandler_CreateConversation_Success(t *testing.T) {
	r := newChatRouter(&fakeChatClient{conv: &downstream.ConversationView{ID: 5, Title: "今晚咨询"}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations",
		bytes.NewReader([]byte(`{"title":"今晚咨询"}`)))
	req.Header.Set("Authorization", "Bearer jwt-c")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"code":0`)
	assert.Contains(t, w.Body.String(), `"id":"5"`)
}

func TestChatHandler_SendMessage_Success(t *testing.T) {
	fc := &fakeChatClient{msg: &downstream.MessageView{ID: 10, ConversationID: 5, Content: "hi"}}
	r := newChatRouter(fc)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/5/messages",
		bytes.NewReader([]byte(`{"content":"hi"}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int64(5), fc.gotConvID, "conversation id 应从 path 解析")
	assert.Contains(t, w.Body.String(), `"code":0`)
}

func TestChatHandler_ListMessages_Success(t *testing.T) {
	fc := &fakeChatClient{messages: []downstream.MessageView{
		{ID: 1, Content: "a"}, {ID: 2, Content: "b"},
	}}
	r := newChatRouter(fc)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/conversations/5/messages?limit=10", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int64(5), fc.gotConvID)
	assert.Equal(t, 10, fc.gotLimit)
	assert.Contains(t, w.Body.String(), `"list"`)
}

func TestChatHandler_DeleteConversation_Success(t *testing.T) {
	fc := &fakeChatClient{}
	r := newChatRouter(fc)
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/conversations/5", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, int64(5), fc.gotConvID)
	assert.Contains(t, w.Body.String(), `"success":true`)
}

func TestChatHandler_InvalidID_Returns400(t *testing.T) {
	r := newChatRouter(&fakeChatClient{})
	for _, method := range []string{http.MethodDelete} {
		req := httptest.NewRequest(method, "/api/v1/conversations/abc", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		assert.Equal(t, http.StatusBadRequest, w.Code, "非法 id 应 400")
	}
}

func TestChatHandler_Upstream404_Returns404(t *testing.T) {
	r := newChatRouter(&fakeChatClient{delErr: &downstream.APIError{StatusCode: http.StatusNotFound, Msg: "conversation not found"}})
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/conversations/999", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "conversation not found")
}
