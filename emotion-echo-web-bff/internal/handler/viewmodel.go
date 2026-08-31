// Package handler — viewmodel.go
//
// BFF ViewModel：把下游 svc 数据转换为前端 UI 契约形状（types/api.ts）。
// 这是 BFF 聚合层的核心职责之一——前端直接消费这些形状，不再自己转换。
package handler

import (
	"fmt"
	"time"

	"emotion-echo-web-bff/internal/downstream"
)

// ConversationItemVM 对齐前端 types/api.ts ConversationItem
type ConversationItemVM struct {
	ID              string  `json:"id"`
	UserID          string  `json:"userId"`
	Title           string  `json:"title"`
	IsTop           bool    `json:"isTop"`
	LastMessage     *string `json:"lastMessage"`
	LastMessageTime *int64  `json:"lastMessageTime"`
	CreatedAt       string  `json:"createdAt"`
	UpdatedAt       string  `json:"updatedAt"`
}

// MessageItemVM 对齐前端 types/api.ts MessageItem（sender: user|ai）
type MessageItemVM struct {
	ID             string  `json:"id"`
	ConversationID string  `json:"conversationId"`
	Sender         string  `json:"sender"`
	Content        string  `json:"content"`
	ContentType    string  `json:"contentType"`
	EmotionTag     *string `json:"emotionTag,omitempty"`
	SendTime       int64   `json:"sendTime"`
	CreatedAt      int64   `json:"createdAt"`
}

// ProfileVM 对齐前端 types/api.ts UserInfo（profile 端点直接返回）
type ProfileVM struct {
	ID        string         `json:"id"`
	Username  string         `json:"username"`
	Nickname  string         `json:"nickname"`
	Avatar    string         `json:"avatar"`
	Age       *int           `json:"age"`
	Config    map[string]any `json:"config"`
	CreatedAt string         `json:"createdAt"`
	UpdatedAt string         `json:"updatedAt,omitempty"`
}

// toConversationItemVM 下游 ConversationView → 前端 ConversationItem
func toConversationItemVM(c *downstream.ConversationView) ConversationItemVM {
	createdAt := time.UnixMilli(c.CreatedAt)
	updatedAt := time.UnixMilli(c.UpdatedAt)
	if c.CreatedAt == 0 {
		createdAt = time.Now()
	}
	if c.UpdatedAt == 0 {
		updatedAt = time.Now()
	}
	return ConversationItemVM{
		ID:        fmt.Sprintf("%d", c.ID),
		UserID:    fmt.Sprintf("%d", c.UserID),
		Title:     c.Title,
		IsTop:     false,
		CreatedAt: createdAt.Format(time.RFC3339),
		UpdatedAt: updatedAt.Format(time.RFC3339),
	}
}

// toMessageItemVM 下游 MessageView → 前端 MessageItem
func toMessageItemVM(m *downstream.MessageView) MessageItemVM {
	sender := "user"
	if m.Role == "assistant" || m.Role == "ai" {
		sender = "ai"
	}
	return MessageItemVM{
		ID:             fmt.Sprintf("%d", m.ID),
		ConversationID: fmt.Sprintf("%d", m.ConversationID),
		Sender:         sender,
		Content:        m.Content,
		ContentType:    "text",
		SendTime:       m.CreatedAt,
		CreatedAt:      m.CreatedAt,
	}
}

// toProfileVM 下游 UserInfo（user-svc 形状）→ 前端 UserInfo（profile 形状）
func toProfileVM(u *downstream.UserInfo) ProfileVM {
	return ProfileVM{
		ID:        fmt.Sprintf("%d", u.UserID),
		Username:  u.Account,
		Nickname:  u.Nickname,
		Avatar:    "",
		Age:       nil,
		Config:    map[string]any{},
		CreatedAt: time.Now().Format(time.RFC3339),
	}
}
