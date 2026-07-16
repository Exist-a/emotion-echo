package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"
	"emotion-echo-gin/internal/pkg/errors"
	"emotion-echo-gin/internal/pkg/response"
	"emotion-echo-gin/internal/service"
)

// SurveyHandler 测验处理器
type SurveyHandler struct {
	surveyService *service.SurveyService
}

// NewSurveyHandler 创建测验处理器
func NewSurveyHandler(surveyService *service.SurveyService) *SurveyHandler {
	return &SurveyHandler{surveyService: surveyService}
}

// List 获取量表列表
func (h *SurveyHandler) List(c *gin.Context) {
	userID := c.GetInt64("userId")

	items, err := h.surveyService.List(c.Request.Context(), userID)
	if err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, gin.H{"list": items})
}

// Get 获取量表详情
func (h *SurveyHandler) Get(c *gin.Context) {
	surveyID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.ErrorWithCode(c, errors.ErrInvalidParams, "invalid survey id")
		return
	}

	survey, err := h.surveyService.Get(c.Request.Context(), surveyID)
	if err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, survey)
}

// GetResult 获取测验结果详情
func (h *SurveyHandler) GetResult(c *gin.Context) {
	resultID := c.Param("resultId")
	if resultID == "" {
		response.ErrorWithCode(c, errors.ErrInvalidParams, "resultId 不能为空")
		return
	}

	result, err := h.surveyService.GetResult(c.Request.Context(), resultID)
	if err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, result)
}

// Submit 提交答案
func (h *SurveyHandler) Submit(c *gin.Context) {
	userID := c.GetInt64("userId")
	surveyID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.ErrorWithCode(c, errors.ErrInvalidParams, "invalid survey id")
		return
	}

	var req service.SubmitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.ErrorWithCode(c, errors.ErrInvalidParams, err.Error())
		return
	}

	result, err := h.surveyService.Submit(c.Request.Context(), userID, surveyID, &req)
	if err != nil {
		if be, ok := errors.IsBusinessError(err); ok {
			response.ErrorFromBusinessError(c, be)
			return
		}
		response.ErrorWithCode(c, errors.ErrInternalServer)
		return
	}

	response.Success(c, result)
}
