package handler

import (
	"net/http"
	"strconv"

	"emotion-echo-ai-svc/internal/logic"
	"emotion-echo-ai-svc/internal/svc"
	"emotion-echo-ai-svc/internal/types"

	"github.com/gin-gonic/gin"
)

// GetEmotionByMessageHandler GET /api/v1/emotion/message/:messageId
func GetEmotionByMessageHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.GetEmotionByMessageReq
		id, err := strconv.ParseInt(c.Param("messageId"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid messageId"})
			return
		}
		req.MessageId = id
		resp, err := logic.NewGetEmotionByMessageLogic(c.Request.Context(), svcCtx).GetEmotionByMessage(&req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// ListEmotionByConversationHandler GET /api/v1/emotion/conversation/:conversationId
func ListEmotionByConversationHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.ListEmotionByConversationReq
		id, err := strconv.ParseInt(c.Param("conversationId"), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid conversationId"})
			return
		}
		req.ConversationId = id
		resp, err := logic.NewListEmotionByConversationLogic(c.Request.Context(), svcCtx).ListEmotionByConversation(&req)
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