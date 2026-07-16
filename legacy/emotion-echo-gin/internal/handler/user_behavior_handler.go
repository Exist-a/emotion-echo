package handler

import (
	"github.com/gin-gonic/gin"
	"emotion-echo-gin/internal/pkg/response"
	"emotion-echo-gin/internal/service"
)

// UserBehaviorHandler 用户行为分析处理器
type UserBehaviorHandler struct {
	behaviorService *service.UserBehaviorService
}

// NewUserBehaviorHandler 创建用户行为分析处理器
func NewUserBehaviorHandler(behaviorService *service.UserBehaviorService) *UserBehaviorHandler {
	return &UserBehaviorHandler{behaviorService: behaviorService}
}

// GetDayNightPattern 获取昼夜使用模式
func (h *UserBehaviorHandler) GetDayNightPattern(c *gin.Context) {
	userID := c.GetInt64("userId")

	pattern, err := h.behaviorService.GetDayNightPattern(c.Request.Context(), userID)
	if err != nil {
		response.ErrorWithCode(c, 500, err.Error())
		return
	}

	response.Success(c, pattern)
}

// GetInteractionDepth 获取互动深度
func (h *UserBehaviorHandler) GetInteractionDepth(c *gin.Context) {
	userID := c.GetInt64("userId")

	depth, err := h.behaviorService.GetInteractionDepth(c.Request.Context(), userID)
	if err != nil {
		response.ErrorWithCode(c, 500, err.Error())
		return
	}

	response.Success(c, depth)
}

// GetFrequencyTrend 获取对话频次趋势
func (h *UserBehaviorHandler) GetFrequencyTrend(c *gin.Context) {
	userID := c.GetInt64("userId")

	trend, err := h.behaviorService.GetFrequencyTrend(c.Request.Context(), userID)
	if err != nil {
		response.ErrorWithCode(c, 500, err.Error())
		return
	}

	response.Success(c, trend)
}
