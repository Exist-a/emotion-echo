package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/pkg/response"
	"emotion-echo-gin/internal/service"
)

// MessageHandler 消息处理器
type MessageHandler struct {
	msgService *service.MessageService
}

// NewMessageHandler 创建消息处理器
func NewMessageHandler(msgService *service.MessageService) *MessageHandler {
	return &MessageHandler{msgService: msgService}
}

// List 获取消息列表
func (h *MessageHandler) List(c *gin.Context) {
	userID := c.GetInt64("userId")
	convID := c.Param("id")

	// 解析分页参数
	limit := 20
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	var cursor int64 = 0
	if c := c.Query("cursor"); c != "" {
		if v, err := strconv.ParseInt(c, 10, 64); err == nil {
			cursor = v
		}
	}

	msgs, hasMore, err := h.msgService.List(c.Request.Context(), userID, convID, limit, cursor)
	if err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	// 计算下一个 cursor
	nextCursor := int64(0)
	if len(msgs) > 0 {
		nextCursor = msgs[len(msgs)-1].SendTime
	}

	response.Success(c, gin.H{
		"list":    msgs,
		"cursor":  nextCursor,
		"hasMore": hasMore,
	})
}

// Send 发送消息
func (h *MessageHandler) Send(c *gin.Context) {
	userID := c.GetInt64("userId")
	convID := c.Param("id")

	var req service.SendRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithCode(c, errors.ErrInvalidParams, err.Error())
		return
	}

	msg, err := h.msgService.Send(c.Request.Context(), userID, convID, &req)
	if err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, msg)
}
