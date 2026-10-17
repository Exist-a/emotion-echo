
package types

type ConversationView struct {
	Id        int64  `json:"id"`
	UserId    int64  `json:"userId"`
	Title     string `json:"title"`
	MsgCount  int    `json:"msgCount"`
	Status    int    `json:"status"`
	IsPinned  bool   `json:"isPinned"`
	CreatedAt int64  `json:"createdAt"`
	UpdatedAt int64  `json:"updatedAt"`
	// CL-06：本会话当前角色（0 = 未显式选择 ⇒ 用用户默认角色）
	AgentId int64 `json:"agentId"`
}

type CreateConversationReq struct {
	Title string `json:"title,optional"`
}

type CreateConversationResp struct {
	Conversation ConversationView `json:"conversation"`
}

type HealthResp struct {
	Status  string `json:"status"`
	Time    int64  `json:"time"`
	Service string `json:"service"`
	Version string `json:"version"`
	DbOK    bool   `json:"dbOk"`
	KafkaOK bool   `json:"kafkaOk"`
}

type ListMessagesReq struct {
	Id    int64 `path:"id"`
	Limit int   `json:"limit,default=50"`
	// Cursor 2026-10-13：游标分页 —— 只返回 id < Cursor 的消息（0 = 取最新）
	Cursor int64 `json:"cursor,default=0"`
}

type ListMessagesResp struct {
	Messages []MessageView `json:"messages"`
	// HasMore 2026-10-13：是否还有更旧的消息
	HasMore bool `json:"hasMore"`
}

type MessageView struct {
	Id             int64  `json:"id"`
	ConversationId int64  `json:"conversationId"`
	UserId         int64  `json:"userId"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	// Stage 79：响应视图补 contentType（proto Message 同步加 content_type=8）
	ContentType string `json:"contentType"`
	// Stage 82 PR-3b：消息意图（6 类；'' = 未分类）
	Intent      string `json:"intent"`
	// Stage 89 PR-2：文件消息原始文件名（'' = 非文件或未提供）
	FileName    string `json:"fileName,omitempty"`
	TokensUsed  int    `json:"tokensUsed"`
	CreatedAt   int64  `json:"createdAt"`
	// CL-06：该 AI 消息由哪个角色产生（快照；0 = 未记录）
	AgentId int64 `json:"agentId,omitempty"`
}

type SendMessageReq struct {
	Id           int64   `path:"id"`
	Role         string  `json:"role,default=user"`
	Content      string  `json:"content"`
	ClientMsgID  *string `json:"client_msg_id,optional"`
	ContentType  string  `json:"content_type,optional"`
	EmotionTag   string  `json:"emotion_tag,optional"`
	Intent       string  `json:"intent,optional"` // Stage 82 PR-3b：6 类消息意图
	FileName     string  `json:"file_name,optional"` // Stage 89 PR-2：文件原始名（contentType=file 时）
	// CL-06：本条消息由哪个角色产生（**快照**；仅 assistant 消息由 BFF 传入，0 = 不记录）
	AgentId int64 `json:"agent_id,optional"`
}

type SendMessageResp struct {
	Message MessageView `json:"message"`
}

// DeleteConversationReq DELETE /api/v1/conversations/:id
type DeleteConversationReq struct {
	Id int64 `path:"id"`
}

// DeleteConversationResp 删除成功响应
type DeleteConversationResp struct {
	Success bool  `json:"success"`
	Id      int64 `json:"id"`
}

// ListConversationsReq GET /api/v1/conversations（用户隔离，按 updated_at desc）
type ListConversationsReq struct {
	Limit  int `json:"limit,default=20"`
	Offset int `json:"offset,default=0"`
	// Cursor CL-05 Phase 5：不透明游标（`<unixMicros>:<id>`，复合键）。
	// 空串 = 首页；非空时 Offset 被忽略。
	Cursor string `json:"cursor"`
}

// ListConversationsResp 列表响应 + hasMore（取 limit+1 探测）
type ListConversationsResp struct {
	List    []ConversationView `json:"list"`
	HasMore bool               `json:"hasMore"`
	// NextCursor 下一页游标（本页末行的复合键）；没有更多时为空串。
	NextCursor string `json:"nextCursor"`
}

// PinConversationReq POST /api/v1/conversations/:id/pin（Stage 72）
type PinConversationReq struct {
	Id       int64 `path:"id"`
	IsPinned bool  `json:"isPinned"`
}

// PinConversationResp 置顶响应（回显最终状态）
type PinConversationResp struct {
	Success  bool  `json:"success"`
	Id       int64 `json:"id"`
	IsPinned bool  `json:"isPinned"`
}

// UpdateConversationReq PATCH /api/v1/conversations/:id（Stage 72，当前仅支持 title）
type UpdateConversationReq struct {
	Id    int64  `path:"id"`
	Title string `json:"title"`
	// CL-06：把本会话当前角色改为该 id（0 = 本次不改角色）。
	AgentId int64 `json:"agentId,optional"`
}

// UpdateConversationResp 更新响应
type UpdateConversationResp struct {
	Success bool   `json:"success"`
	Id      int64  `json:"id"`
	Title   string `json:"title"`
	// CL-06：生效后的角色 id（0 = 本次未改）
	AgentId int64 `json:"agentId"`
}
