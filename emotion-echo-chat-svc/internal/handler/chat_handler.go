package handler

import (
	"net/http"
	"strconv"

	"emotion-echo-chat-svc/internal/logic"
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