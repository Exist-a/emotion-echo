package service

// StreamRequest 流式请求
type StreamRequest struct {
	ConversationID      string `json:"conversationId,omitempty"`
	Message            string `json:"message" binding:"required"`
	Emotion            string `json:"emotion,omitempty"`
	VoiceEmotion       string `json:"voiceEmotion,omitempty"`
	Model              string `json:"model,omitempty"`
	ShouldGenerateTitle bool   `json:"shouldGenerateTitle,omitempty"`
}

// StreamEvent 流式事件
type StreamEvent struct {
	Type           string `json:"type"`
	ConversationID string `json:"conversationId,omitempty"`
	UserMessageID  string `json:"userMessageId,omitempty"`
	Content       string `json:"content,omitempty"`
	MessageID     string `json:"messageId,omitempty"`
	Title         string `json:"title,omitempty"`
	Code          int    `json:"code,omitempty"`
	Error         string `json:"error,omitempty"`
	Emotion       string `json:"emotion,omitempty"`
}

// StreamResponse 流式响应
type StreamResponse struct {
	Event *StreamEvent
	Error error
}
