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
// 注：PinConversation / UpdateConversation 已在 Stage 72 落地（chat-svc gRPC RPC）。
package downstream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	"strconv"
	"time"

	bffdiscovery "emotion-echo-web-bff/internal/discovery"
	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"
	"github.com/emotion-echo/shared/pkg/skywalking/trace"

	"google.golang.org/grpc"
)

// ConversationView 对应 chat-svc types.ConversationView
type ConversationView struct {
	ID        int64  `json:"id"`
	UserID    int64  `json:"userId"`
	Title     string `json:"title"`
	MsgCount  int    `json:"msgCount"`
	Status    int    `json:"status"`
	IsPinned  bool   `json:"isPinned"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
	// CL-06：本会话当前角色（0 = 未显式选择 ⇒ 前端按默认角色展示/发送）
	AgentID int64 `json:"agentId"`
}

// UpdateConversationResp 对应 chat-svc types.UpdateConversationResp（Stage 72）
type UpdateConversationResp struct {
	Success bool   `json:"success"`
	Id      int64  `json:"id"`
	Title   string `json:"title"`
	// CL-06：生效后的角色 id（0 = 本次未改角色）
	AgentID int64 `json:"agentId"`
}

// MessageView 对应 chat-svc types.MessageView
type MessageView struct {
	ID             int64  `json:"id"`
	ConversationID int64  `json:"conversationId"`
	UserID         int64  `json:"userId"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	// Stage 79：响应视图补 contentType（proto Message content_type=8）
	ContentType string `json:"contentType"`
	// Stage 82 PR-3b：消息意图（6 类；'' = 未分类）
	Intent      string `json:"intent"`
	// Stage 89 PR-3：文件消息原始文件名（chat-svc file_name 回带）
	FileName    string `json:"fileName,omitempty"`
	TokensUsed  int    `json:"tokensUsed"`
	CreatedAt   int64  `json:"createdAt"`
	// CL-06：该 AI 消息由哪个角色产生（**写库时快照**；0 = 无角色边界的历史消息）
	AgentID int64 `json:"agentId"`
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
	// Stage 79：三个字段 tag 从 snake_case 改 camelCase——唯一客户端是前端 web
	// （stores/message.ts 发 contentType/emotionTag/clientMsgId），原 snake_case tag
	// 导致三字段一直绑定不上（e2e 实测 contentType 落库成默认 text）。
	ClientMsgID  *string `json:"clientMsgId,omitempty"`
	ContentType  string  `json:"contentType,omitempty"`
	EmotionTag   string  `json:"emotionTag,omitempty"`
	Intent       string  `json:"intent,omitempty"` // Stage 82 PR-3b：6 类消息意图
	FileName     string  `json:"fileName,omitempty"` // Stage 89 PR-3：文件原始名（contentType=file 时）
	// CL-06：本条消息的角色归属（0 = 不带 ⇒ 落 NULL；用户消息恒不带，只有 AI 回复带）
	AgentID int64 `json:"agentId,omitempty"`
}

// ConversationPage 会话列表一页（CL-05 Phase 5 · keyset 分页）。
//
// NextCursor 是**不透明**的下一页游标（由 chat-svc 编码），BFF 只回传不解析；
// 空串 = 没有更多（前端的终止条件）。
type ConversationPage struct {
	List       []ConversationView
	HasMore    bool
	NextCursor string
}

// ChatClient BFF → chat-svc HTTP 客户端
type ChatClient interface {
	// CreateConversation 新建会话
	CreateConversation(ctx context.Context, req CreateConversationReq) (*ConversationView, error)
	// SendMessage 发送消息
	SendMessage(ctx context.Context, conversationID int64, req SendMessageReq) (*MessageView, error)
	// ListMessages 列出会话消息（2026-10-13：游标分页，返回最新 limit 条 +
	// 是否还有更旧消息；cursor>0 时取 id < cursor 的更旧一页）。
	ListMessages(ctx context.Context, conversationID int64, limit int, cursor int64) ([]MessageView, bool, error)
	// ListConversations Stage 36-A2.2：列出当前用户的会话（用户隔离由 chat-svc 保证）。
	//
	// CL-05 Phase 5（2026-10-16）：`cursor` 是 chat-svc 独占编码的**不透明游标**
	// （复合键 `<unixMicros>:<id>`）；BFF 只做**原样透传/回传**，不解析其内容。
	// 空串 = 首页。返回体带 `NextCursor` 供前端推进下一页（空 = 没有更多）。
	ListConversations(ctx context.Context, limit, offset int, cursor string) (ConversationPage, error)
	// DeleteConversation 删除会话
	DeleteConversation(ctx context.Context, conversationID int64) error
	// PinConversation 置顶/取消置顶会话（Stage 72 chat-svc RPC 已落地）
	PinConversation(ctx context.Context, conversationID int64, isPinned bool) error
	// UpdateConversation 更新会话（Stage 72；CL-06 起为**两个可选动作共用一个 RPC**：
	// title 改名 / agentID 切角色。各自传空值 = 不动该项）。
	UpdateConversation(ctx context.Context, conversationID int64, title string, agentID int64) (*UpdateConversationResp, error)
}

// ChatClientOptions 构造选项
type ChatClientOptions struct {
	BaseURL   string
	TimeoutMs int
	// Resolver PR-2: 当 BaseURL 为空时，通过 Resolver.Resolve 拉 chat-svc 实例。
	Resolver bffdiscovery.Resolver
	// ServiceName 用于 Resolver，默认 "emotion-echo-chat-svc"。
	ServiceName string
	// Transport "grpc"（默认）| "http"；grpc 模式需同时传 GRPCConn
	Transport ChatTransport
	// GRPCConn gRPC 连接（仅 Transport=grpc 时用）
	GRPCConn *grpc.ClientConn
}

// chatHTTPClient 是 ChatClient 的 HTTP 实现
type chatHTTPClient struct {
	baseURL string
	http    *http.Client
}

// ChatTransport 决定 NewChatClient 返回 HTTP 还是 gRPC 实现
//
// 读取 opts.Transport，缺省 "grpc"（PR-GRPC-4 默认；HTTP 路径仍保留作 fallback）。
type ChatTransport string

const (
	ChatTransportGRPC ChatTransport = "grpc"
	ChatTransportHTTP ChatTransport = "http"
)

// NewChatClient 构造 ChatClient。
//
// 行为分支（Stage 58 PR-GRPC-4）：
//   - Transport=grpc + GRPCConn != nil → 返 chatGRPCClient
//   - Transport=http（默认 fallback）→ 返 chatHTTPClient（保留旧行为）
//   - 都缺 → 返 nil
//
// BaseURL 解析优先级（HTTP 模式）：opts.BaseURL > opts.Resolver.Resolve(ServiceName)
func NewChatClient(opts ChatClientOptions) ChatClient {
	// gRPC 优先（PR-GRPC-4）
	if opts.Transport == "" || opts.Transport == ChatTransportGRPC {
		if opts.GRPCConn != nil {
			return NewChatGRPCClient(opts.GRPCConn)
		}
		// grpc 但无 conn → 退化为 http（保留向后兼容）
	}

	// HTTP fallback（保留旧 chatHTTPClient 行为）
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
		// CL-02 收口（2026-10-15）：HTTP 出站埋点补齐 —— 本文件属 C 轨独占列，
		// CL-01 收口时明确豁免并移交本阶段（见 cl-01 report §收口自检 第 5 项）。
		// 与 ai.go/analytics.go 等一致：exit span + sw8 注入（无父 span 时不硬造根 span）。
		http: trace.NewTracedClient(timeout),
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

func (c *chatHTTPClient) ListMessages(ctx context.Context, conversationID int64, limit int, cursor int64) ([]MessageView, bool, error) {
	url := c.baseURL + "/api/v1/conversations/" + strconv.FormatInt(conversationID, 10) + "/messages"
	q := buildMessageQuery(limit, cursor)
	if q != "" {
		url += "?" + q
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, false, err
	}
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, false, fmt.Errorf("downstream: list messages: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, false, readError(resp)
	}
	var wrapped struct {
		Messages []MessageView `json:"messages"`
		HasMore  bool          `json:"hasMore"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapped); err != nil {
		return nil, false, fmt.Errorf("downstream: decode messages resp: %w", err)
	}
	return wrapped.Messages, wrapped.HasMore, nil
}

// buildMessageQuery 组装 limit/cursor 查询串（cursor>0 才带）。
func buildMessageQuery(limit int, cursor int64) string {
	q := ""
	if limit > 0 {
		q = "limit=" + strconv.Itoa(limit)
	}
	if cursor > 0 {
		if q != "" {
			q += "&"
		}
		q += "cursor=" + strconv.FormatInt(cursor, 10)
	}
	return q
}

// ListConversations Stage 36-A2.2：列出当前用户的会话，按 updated_at desc。
//
// CL-05 Phase 5：`cursor`（不透明，空=首页）原样透传给 chat-svc，
// 并把 chat-svc 返回的 `nextCursor` 原样回传（BFF **不**解析游标内容）。
func (c *chatHTTPClient) ListConversations(ctx context.Context, limit, offset int, cursor string) (ConversationPage, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	url := fmt.Sprintf("%s/api/v1/conversations?limit=%d&offset=%d", c.baseURL, limit, offset)
	if cursor != "" {
		url += "&cursor=" + neturl.QueryEscape(cursor)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ConversationPage{}, err
	}
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return ConversationPage{}, fmt.Errorf("downstream: list conversations: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return ConversationPage{}, readError(resp)
	}
	var wrapped struct {
		List       []ConversationView `json:"list"`
		HasMore    bool               `json:"hasMore"`
		NextCursor string             `json:"nextCursor"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapped); err != nil {
		return ConversationPage{}, fmt.Errorf("downstream: decode conversations resp: %w", err)
	}
	if wrapped.List == nil {
		wrapped.List = []ConversationView{}
	}
	return ConversationPage{List: wrapped.List, HasMore: wrapped.HasMore, NextCursor: wrapped.NextCursor}, nil
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

func (c *chatHTTPClient) PinConversation(ctx context.Context, conversationID int64, isPinned bool) error {
	body, err := json.Marshal(map[string]bool{"isPinned": isPinned})
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut,
		c.baseURL+"/api/v1/conversations/"+strconv.FormatInt(conversationID, 10)+"/pin", bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("downstream: pin conversation: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return readError(resp)
	}
	return nil
}

// UpdateConversation HTTP fallback：PATCH chat-svc /api/v1/conversations/:id。
// chat-svc HTTP 端点未实现（Stage 72 走 gRPC only）→ 404 readError；未来补 HTTP 端点后自动工作。
func (c *chatHTTPClient) UpdateConversation(ctx context.Context, conversationID int64, title string, agentID int64) (*UpdateConversationResp, error) {
	body, err := json.Marshal(map[string]any{"title": title, "agentId": agentID})
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPatch,
		c.baseURL+"/api/v1/conversations/"+strconv.FormatInt(conversationID, 10), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("downstream: update conversation: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, readError(resp)
	}
	var out UpdateConversationResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("downstream: update conversation: decode resp: %w", err)
	}
	return &out, nil
}
