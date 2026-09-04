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

	bffdiscovery "emotion-echo-web-bff/internal/discovery"
	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"
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
//
// Stage 33 PR-18：ClientMsgID 透传给 chat-svc 用于幂等查重（同一 uuid 多次
// 提交仅落库一次）。ContentType / EmotionTag 同步透传以保留前端语义。
type SendMessageReq struct {
	Role         string  `json:"role,omitempty"`
	Content      string  `json:"content"`
	ClientMsgID  *string `json:"client_msg_id,omitempty"`
	ContentType  string  `json:"content_type,omitempty"`
	EmotionTag   string  `json:"emotion_tag,omitempty"`
}

// ChatClient BFF → chat-svc HTTP 客户端
type ChatClient interface {
	// CreateConversation 新建会话
	CreateConversation(ctx context.Context, req CreateConversationReq) (*ConversationView, error)
	// SendMessage 发送消息
	SendMessage(ctx context.Context, conversationID int64, req SendMessageReq) (*MessageView, error)
	// ListMessages 列出会话消息
	ListMessages(ctx context.Context, conversationID int64, limit int) ([]MessageView, error)
	// ListConversations Stage 36-A2.2：列出当前用户的会话（用户隔离由 chat-svc 保证）。
	// 返回值：list + hasMore（取 limit+1 探测）。
	ListConversations(ctx context.Context, limit, offset int) ([]ConversationView, bool, error)
	// DeleteConversation 删除会话
	DeleteConversation(ctx context.Context, conversationID int64) error
	// PinConversation 置顶会话（下游未实现；接口保留，未来 chat-svc 支持后可用）
	PinConversation(ctx context.Context, conversationID int64) error
}

// ChatClientOptions 构造选项
type ChatClientOptions struct {
	BaseURL   string
	TimeoutMs int
	// Resolver PR-2: 当 BaseURL 为空时，通过 Resolver.Resolve 拉 chat-svc 实例。
	Resolver bffdiscovery.Resolver
	// ServiceName 用于 Resolver，默认 "emotion-echo-chat-svc"。
	ServiceName string
}

// chatHTTPClient 是 ChatClient 的 HTTP 实现
type chatHTTPClient struct {
	baseURL string
	http    *http.Client
}

// NewChatClient 构造 ChatClient。
//
// BaseURL 解析优先级：opts.BaseURL（env 注入）> opts.Resolver.Resolve(ServiceName)。
// 两者都缺 → 返回 nil。
func NewChatClient(opts ChatClientOptions) ChatClient {
	timeout := time.Duration(opts.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	svcName := opts.ServiceName
	if svcName == "" {
		svcName = shareddiscovery.ServiceChat
	}
	client := &chatHTTPClient{
		baseURL: opts.BaseURL,
		http:    &http.Client{Timeout: timeout},
	}
	if opts.BaseURL == "" && opts.Resolver != nil {
		host, port, err := opts.Resolver.Resolve(context.Background(), svcName)
		if err == nil {
			client.baseURL = fmt.Sprintf("http://%s:%d", host, port)
		}
	}
	if client.baseURL == "" {
		return nil
	}
	return client
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

// ListConversations Stage 36-A2.2：列出当前用户的会话，按 updated_at desc。
//
// chat-svc 端点返回 ListConversationsResp{list, hasMore}，下游已做 limit+1 探测，
// BFF 这层只透传并把 list 转成下游 ConversationView 切片 + hasMore 标志。
func (c *chatHTTPClient) ListConversations(ctx context.Context, limit, offset int) ([]ConversationView, bool, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	url := fmt.Sprintf("%s/api/v1/conversations?limit=%d&offset=%d", c.baseURL, limit, offset)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, false, fmt.Errorf("downstream: list conversations: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, false, readError(resp)
	}
	var wrapped struct {
		List    []ConversationView `json:"list"`
		HasMore bool               `json:"hasMore"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapped); err != nil {
		return nil, false, fmt.Errorf("downstream: decode conversations resp: %w", err)
	}
	if wrapped.List == nil {
		wrapped.List = []ConversationView{}
	}
	return wrapped.List, wrapped.HasMore, nil
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
