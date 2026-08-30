// Package downstream — chat_test.go
//
// Stage 30 / stage-30-web-bff.md T2.15-17 RED: ChatClient 契约测试
package downstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatClient_CreateConversation_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/conversations", r.URL.Path)
		assert.Equal(t, "Bearer jwt-c", r.Header.Get("Authorization"))

		var req CreateConversationReq
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "今晚咨询", req.Title)

		_ = json.NewEncoder(w).Encode(map[string]any{"conversation": ConversationView{
			ID: 5, UserID: 1, Title: "今晚咨询", MsgCount: 0, Status: 1,
		}})
	}))
	defer srv.Close()

	c := NewChatClient(ChatClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	conv, err := c.CreateConversation(WithJWT(context.Background(), "jwt-c"), CreateConversationReq{Title: "今晚咨询"})
	require.NoError(t, err)
	require.NotNil(t, conv)
	assert.Equal(t, int64(5), conv.ID)
	assert.Equal(t, "今晚咨询", conv.Title)
}

func TestChatClient_SendMessage_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/v1/conversations/5/messages", r.URL.Path)

		var req SendMessageReq
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, "我今天心情不好", req.Content)

		_ = json.NewEncoder(w).Encode(map[string]any{"message": MessageView{
			ID: 10, ConversationID: 5, UserID: 1, Role: "user", Content: "我今天心情不好",
		}})
	}))
	defer srv.Close()

	c := NewChatClient(ChatClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	msg, err := c.SendMessage(context.Background(), 5, SendMessageReq{Content: "我今天心情不好"})
	require.NoError(t, err)
	require.NotNil(t, msg)
	assert.Equal(t, int64(10), msg.ID)
	assert.Equal(t, "我今天心情不好", msg.Content)
}

func TestChatClient_ListMessages_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/conversations/5/messages", r.URL.Path)
		assert.Equal(t, "50", r.URL.Query().Get("limit"))

		_ = json.NewEncoder(w).Encode(map[string]any{"messages": []MessageView{
			{ID: 1, ConversationID: 5, Role: "user", Content: "hi"},
			{ID: 2, ConversationID: 5, Role: "assistant", Content: "hello"},
		}})
	}))
	defer srv.Close()

	c := NewChatClient(ChatClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	msgs, err := c.ListMessages(context.Background(), 5, 50)
	require.NoError(t, err)
	assert.Len(t, msgs, 2)
	assert.Equal(t, "hi", msgs[0].Content)
}

func TestChatClient_DeleteConversation_Success(t *testing.T) {
	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "id": 5})
	}))
	defer srv.Close()

	c := NewChatClient(ChatClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	require.NoError(t, c.DeleteConversation(context.Background(), 5))
	assert.Equal(t, http.MethodDelete, gotMethod)
	assert.Equal(t, "/api/v1/conversations/5", gotPath)
}

func TestChatClient_PinConversation_NotImplemented(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "404 page not found"})
	}))
	defer srv.Close()

	c := NewChatClient(ChatClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	err := c.PinConversation(context.Background(), 5)
	require.Error(t, err, "下游未实现 pin → 404 → error")
}

func TestChatClient_Upstream500_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "db down"})
	}))
	defer srv.Close()

	c := NewChatClient(ChatClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	_, err := c.ListMessages(context.Background(), 1, 10)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "db down")
}
