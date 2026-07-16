package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/pkg/response"
	"emotion-echo-gin/internal/service"
)

// ConversationHandler 会话处理器
type ConversationHandler struct {
	convService *service.ConversationService
}

// NewConversationHandler 创建会话处理器
func NewConversationHandler(convService *service.ConversationService) *ConversationHandler {
	return &ConversationHandler{convService: convService}
}

// List 获取会话列表
func (h *ConversationHandler) List(c *gin.Context) {
	userID := c.GetInt64("userId")

	// 解析分页参数
	limit := 20
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}
	cursor := c.Query("cursor")

	convs, hasMore, err := h.convService.List(c.Request.Context(), userID, limit, cursor)
	if err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, gin.H{
		"list":    convs,
		"hasMore": hasMore,
	})
}

// Create 创建会话
func (h *ConversationHandler) Create(c *gin.Context) {
	userID := c.GetInt64("userId")

	var req service.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithCode(c, errors.ErrInvalidParams, err.Error())
		return
	}

	conv, err := h.convService.Create(c.Request.Context(), userID, &req)
	if err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, conv)
}

// Update 更新会话
func (h *ConversationHandler) Update(c *gin.Context) {
	userID := c.GetInt64("userId")
	convID := c.Param("id")

	var req service.UpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithCode(c, errors.ErrInvalidParams, err.Error())
		return
	}

	if err := h.convService.Update(c.Request.Context(), userID, convID, &req); err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, nil)
}

// Delete 删除会话
func (h *ConversationHandler) Delete(c *gin.Context) {
	userID := c.GetInt64("userId")
	convID := c.Param("id")

	if err := h.convService.Delete(c.Request.Context(), userID, convID); err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	c.Status(204)
}

// Pin 置顶/取消置顶
func (h *ConversationHandler) Pin(c *gin.Context) {
	userID := c.GetInt64("userId")
	convID := c.Param("id")

	var req service.PinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithCode(c, errors.ErrInvalidParams, err.Error())
		return
	}

	if err := h.convService.Pin(c.Request.Context(), userID, convID, &req); err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, nil)
}
