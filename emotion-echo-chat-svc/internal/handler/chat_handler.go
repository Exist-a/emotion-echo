package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"emotion-echo-chat-svc/internal/logic"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"
	"emotion-echo-chat-svc/internal/types"

	"github.com/gin-gonic/gin"
)

// CreateConversationHandler POST /api/v1/conversations
func CreateConversationHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.CreateConversationReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		resp, err := logic.NewCreateConversationLogic(c.Request.Context(), svcCtx).CreateConversation(&req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// SendMessageHandler POST /api/v1/conversations/:id/messages
func SendMessageHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.SendMessageReq
		// :id 取自 path
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
			return
		}
		req.Id = id
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		resp, err := logic.NewSendMessageLogic(c.Request.Context(), svcCtx).SendMessage(&req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// ListConversationsHandler GET /api/v1/conversations
//
// Stage 36-A2.1：补齐 chat-svc 缺漏的会话列表端点（G2 上半）。
// 下游 BFF 不再需要返回空 stub。
func ListConversationsHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		req := &types.ListConversationsReq{}
		if limitStr := c.Query("limit"); limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
				req.Limit = n
			}
		}
		if offsetStr := c.Query("offset"); offsetStr != "" {
			if n, err := strconv.Atoi(offsetStr); err == nil && n >= 0 {
				req.Offset = n
			}
		}
		resp, err := logic.NewListConversationsLogic(c.Request.Context(), svcCtx).ListConversations(req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// ListMessagesHandler GET /api/v1/conversations/:id/messages
func ListMessagesHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.ListMessagesReq
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
			return
		}
		req.Id = id
		if limitStr := c.Query("limit"); limitStr != "" {
			if n, err := strconv.Atoi(limitStr); err == nil && n > 0 {
				req.Limit = n
			}
		}
		resp, err := logic.NewListMessagesLogic(c.Request.Context(), svcCtx).ListMessages(&req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// DeleteConversationHandler DELETE /api/v1/conversations/:id
func DeleteConversationHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("id"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversation id"})
			return
		}
		resp, err := logic.NewDeleteConversationLogic(c.Request.Context(), svcCtx).
			DeleteConversation(&types.DeleteConversationReq{Id: id})
		if err != nil {
			status := http.StatusInternalServerError
			if errors.Is(err, repository.ErrNotFound) {
				status = http.StatusNotFound
			} else if strings.Contains(err.Error(), "forbidden") || strings.Contains(err.Error(), "unauthorized") {
				status = http.StatusForbidden
			}
			c.JSON(status, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// HealthHandler GET /health
func HealthHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := logic.NewHealthLogic(c.Request.Context(), svcCtx).Health()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}