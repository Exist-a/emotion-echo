
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
}

type ListMessagesResp struct {
	Messages []MessageView `json:"messages"`
}

type MessageView struct {
	Id             int64  `json:"id"`
	ConversationId int64  `json:"conversationId"`
	UserId         int64  `json:"userId"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	// Stage 79：响应视图补 contentType（proto Message 同步加 content_type=8）
	ContentType string `json:"contentType"`
	TokensUsed  int    `json:"tokensUsed"`
	CreatedAt   int64  `json:"createdAt"`
}

type SendMessageReq struct {
	Id           int64   `path:"id"`
	Role         string  `json:"role,default=user"`
	Content      string  `json:"content"`
	ClientMsgID  *string `json:"client_msg_id,optional"`
	ContentType  string  `json:"content_type,optional"`
	EmotionTag   string  `json:"emotion_tag,optional"`
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
}

// ListConversationsResp 列表响应 + hasMore（取 limit+1 探测）
type ListConversationsResp struct {
	List    []ConversationView `json:"list"`
	HasMore bool               `json:"hasMore"`
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
}

// UpdateConversationResp 更新响应
type UpdateConversationResp struct {
	Success bool   `json:"success"`
	Id      int64  `json:"id"`
	Title   string `json:"title"`
}
