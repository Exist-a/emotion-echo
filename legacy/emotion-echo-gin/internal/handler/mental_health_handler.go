package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/pkg/response"
	"emotion-echo-gin/internal/service"
)

// MentalHealthHandler 心理健康处理器
type MentalHealthHandler struct {
	mentalHealthService *service.MentalHealthService
}

// NewMentalHealthHandler 创建心理健康处理器
func NewMentalHealthHandler(mentalHealthService *service.MentalHealthService) *MentalHealthHandler {
	return &MentalHealthHandler{mentalHealthService: mentalHealthService}
}

// GetAssessment 获取最新评估
func (h *MentalHealthHandler) GetAssessment(c *gin.Context) {
	userID := c.GetInt64("userId")
	assessmentType := c.DefaultQuery("type", "daily")

	assessment, err := h.mentalHealthService.GetLatestAssessment(c.Request.Context(), userID, assessmentType)
	if err != nil {
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	if assessment == nil {
		response.Success(c, nil)
		return
	}

	response.Success(c, gin.H{
		"id":              assessment.ID,
		"assessmentType":  assessment.AssessmentType,
		"periodStart":     assessment.PeriodStart,
		"periodEnd":       assessment.PeriodEnd,
		"overallScore":    assessment.OverallScore,
		"riskLevel":       assessment.RiskLevel,
		"riskFactors":     assessment.RiskFactors,
		"dimensions":      assessment.ToDimensionScores(),
		"summary":         assessment.Summary,
		"suggestions":     assessment.Suggestions,
		"warningFlags":    assessment.WarningFlags,
		"createdAt":       assessment.CreatedAt,
	})
}

// GetHistory 获取历史评估
func (h *MentalHealthHandler) GetHistory(c *gin.Context) {
	userID := c.GetInt64("userId")

	assessmentType := c.Query("type")
	
	limit := 20
	if l := c.Query("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 {
			limit = v
		}
	}

	var cursor int64
	if c := c.Query("cursor"); c != "" {
		if v, err := strconv.ParseInt(c, 10, 64); err == nil {
			cursor = v
		}
	}

	assessments, hasMore, err := h.mentalHealthService.GetAssessmentHistory(c.Request.Context(), userID, assessmentType, limit, cursor)
	if err != nil {
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, gin.H{
		"list":    assessments,
		"cursor":  cursor,
		"hasMore": hasMore,
	})
}

// TriggerAssessment 手动触发评估
func (h *MentalHealthHandler) TriggerAssessment(c *gin.Context) {
	userID := c.GetInt64("userId")

	var req service.TriggerAssessmentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithCode(c, errors.ErrInvalidParams, err.Error())
		return
	}

	resp, err := h.mentalHealthService.TriggerAssessment(c.Request.Context(), userID, &req)
	if err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, resp)
}

// GetTrend 获取趋势数据
func (h *MentalHealthHandler) GetTrend(c *gin.Context) {
	userID := c.GetInt64("userId")
	trendType := c.DefaultQuery("type", "weekly")

	data, err := h.mentalHealthService.GetTrendData(c.Request.Context(), userID, trendType)
	if err != nil {
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, data)
}
