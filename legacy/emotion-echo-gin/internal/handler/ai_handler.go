package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/pkg/response"
	"emotion-echo-gin/internal/service"
)

// AIHandler AI 处理器
type AIHandler struct {
	aiService *service.AIService
}

// NewAIHandler 创建 AI 处理器
func NewAIHandler(aiService *service.AIService) *AIHandler {
	return &AIHandler{aiService: aiService}
}

// Stream 流式对话
func (h *AIHandler) Stream(c *gin.Context) {
	userID := c.GetInt64("userId")

	var req service.StreamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithCode(c, errors.ErrInvalidParams, err.Error())
		return
	}

	// 设置 SSE 响应头
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("Connection", "keep-alive")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.WriteHeader(http.StatusOK)

	// 刷新响应头
	c.Writer.Flush()

	// 使用支持取消的 context
	ctx := c.Request.Context()

	// 执行流式对话
	respChan, err := h.aiService.StreamChat(ctx, userID, &req)
	if err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			sendSSEError(c, be.Code, be.Message)
		} else {
			sendSSEError(c, errors.ErrInternalServer, "internal server error")
		}
		return
	}

	// 转发流式事件（监听客户端断开）
	for {
		select {
		case <-ctx.Done():
			// 客户端断开连接
			return
		case resp, ok := <-respChan:
			if !ok {
				return
			}
			if resp.Error != nil {
				if be, ok := errors.IsBusinessError(resp.Error); ok {
					sendSSEError(c, be.Code, be.Message)
				} else {
					sendSSEError(c, errors.ErrInternalServer, "internal server error")
				}
				return
			}

			if resp.Event != nil {
				if !sendSSEvent(c, resp.Event) {
					return
				}
			}
		}
	}
}

// sendSSEvent 发送 SSE 事件
func sendSSEvent(c *gin.Context, event *service.StreamEvent) bool {
	data, err := json.Marshal(event)
	if err != nil {
		return false
	}
	if _, err := c.Writer.WriteString("data: " + string(data) + "\n\n"); err != nil {
		return false
	}
	c.Writer.Flush()
	return true
}

// sendSSEError 发送 SSE 错误事件
func sendSSEError(c *gin.Context, code int, message string) {
	event := service.StreamEvent{
		Type:  "error",
		Code:  code,
		Error: message,
	}
	sendSSEvent(c, &event)
}
