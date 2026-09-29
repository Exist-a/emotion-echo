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

// HealthReadyHandler 就绪检查（无鉴权，D-29 新增）
//
// readiness 语义：依赖不可用时返 **503**，供 compose healthcheck 判定。
// 与 /health 共用同一套依赖检查（同一个 HealthLogic），差别仅在 HTTP 码 ——
// 两者的响应体 status 字段必然一致。
func HealthReadyHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := logic.NewHealthLogic(c.Request.Context(), svcCtx).Health()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		code := http.StatusOK
		if resp.Status != logic.StatusOk {
			code = http.StatusServiceUnavailable
		}
		c.JSON(code, resp)
	}
}