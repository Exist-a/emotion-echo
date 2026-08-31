// Package downstream — chat.go
//
// Stage 30 / stage-30-web-bff.md T2.15-17: ChatClient（BFF → chat-svc）
//
// chat-svc（Gin :8890）：
//   POST   /api/v1/conversations            → {conversation: ConversationView}
//   POST   /api/v1/conversations/:id/messages → {message: MessageView}
//   GET    /api/v1/conversations/:id/messages?limit= → {messages: [MessageView]}
//   DELETE /api/v1/conversations/:id        → {success, id}
//
// 注：文档 T2 列的 PinConversation 下游尚未实现（chat-svc 无 pin 端点）；
// 接口保留占位，实现返回下游 404 错误（未来 chat-svc 增加后可直接用）。
package downstream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// ConversationView 对应 chat-svc types.ConversationView
type ConversationView struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"userId"`
	Title     string `json:"title"`
	MsgCount  int    `json:"msgCount"`
	Status    int    `json:"status"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
}

// MessageView 对应 chat-svc types.MessageView
type MessageView struct {
	ID             int64  `json:"id"`
	ConversationID int64  `json:"conversationId"`
	UserID         int64  `json:"userId"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	TokensUsed     int    `json:"tokensUsed"`
	CreatedAt      int64  `json:"createdAt"`
}

// CreateConversationReq 对应 chat-svc types.CreateConversationReq
type CreateConversationReq struct {
	Title string `json:"title,omitempty"`
}

// SendMessageReq 对应 chat-svc types.SendMessageReq
type SendMessageReq struct {
	Role    string `json:"role,omitempty"`
	Content string `json:"content"`
}

// ChatClient BFF → chat-svc HTTP 客户端
type ChatClient interface {
	// CreateConversation 新建会话
	CreateConversation(ctx context.Context, req CreateConversationReq) (*ConversationView, error)
	// SendMessage 发送消息
	SendMessage(ctx context.Context, conversationID int64, req SendMessageReq) (*MessageView, error)
	// ListMessages 列出会话消息
	ListMessages(ctx context.Context, conversationID int64, limit int) ([]MessageView, error)
	// DeleteConversation 删除会话
	DeleteConversation(ctx context.Context, conversationID int64) error
	// PinConversation 置顶会话（下游未实现；接口保留，未来 chat-svc 支持后可用）
	PinConversation(ctx context.Context, conversationID int64) error
}

// ChatClientOptions 构造选项
type ChatClientOptions struct {
	BaseURL   string
	TimeoutMs int
}

// chatHTTPClient 是 ChatClient 的 HTTP 实现
type chatHTTPClient struct {
	baseURL string
	http    *http.Client
}

// NewChatClient 构造 ChatClient
func NewChatClient(opts ChatClientOptions) ChatClient {
	timeout := time.Duration(opts.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &chatHTTPClient{
		baseURL: opts.BaseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *chatHTTPClient) CreateConversation(ctx context.Context, req CreateConversationReq) (*ConversationView, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("downstream: marshal create conv: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/v1/conversations", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("downstream: create conversation: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, readError(resp)
	}
	var wrapped struct {
		Conversation *ConversationView `json:"conversation"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapped); err != nil {
		return nil, fmt.Errorf("downstream: decode conv resp: %w", err)
	}
	if wrapped.Conversation == nil {
		return nil, fmt.Errorf("downstream: conversation response missing 'conversation' field")
	}
	return wrapped.Conversation, nil
}

func (c *chatHTTPClient) SendMessage(ctx context.Context, conversationID int64, req SendMessageReq) (*MessageView, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("downstream: marshal send msg: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/conversations/"+strconv.FormatInt(conversationID, 10)+"/messages", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("downstream: send message: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, readError(resp)
	}
	var wrapped struct {
		Message *MessageView `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapped); err != nil {
		return nil, fmt.Errorf("downstream: decode msg resp: %w", err)
	}
	if wrapped.Message == nil {
		return nil, fmt.Errorf("downstream: message response missing 'message' field")
	}
	return wrapped.Message, nil
}

func (c *chatHTTPClient) ListMessages(ctx context.Context, conversationID int64, limit int) ([]MessageView, error) {
	url := c.baseURL + "/api/v1/conversations/" + strconv.FormatInt(conversationID, 10) + "/messages"
	if limit > 0 {
		url += "?limit=" + strconv.Itoa(limit)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("downstream: list messages: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, readError(resp)
	}
	var wrapped struct {
		Messages []MessageView `json:"messages"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapped); err != nil {
		return nil, fmt.Errorf("downstream: decode messages resp: %w", err)
	}
	return wrapped.Messages, nil
}

func (c *chatHTTPClient) DeleteConversation(ctx context.Context, conversationID int64) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		c.baseURL+"/api/v1/conversations/"+strconv.FormatInt(conversationID, 10), nil)
	if err != nil {
		return err
	}
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("downstream: delete conversation: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return readError(resp)
	}
	return nil
}

func (c *chatHTTPClient) PinConversation(ctx context.Context, conversationID int64) error {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut,
		c.baseURL+"/api/v1/conversations/"+strconv.FormatInt(conversationID, 10)+"/pin", nil)
	if err != nil {
		return err
	}
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("downstream: pin conversation: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return readError(resp) // 下游未实现 → 404（未来支持后自动工作）
	}
	return nil
}
