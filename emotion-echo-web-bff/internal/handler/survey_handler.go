// Package handler — survey_handler.go
//
// Stage 30 / stage-30-web-bff.md T4.42-46: survey handler（BFF → assessment-svc）
//
// 端点：
//   GET   /api/v1/surveys              → {items, total}
//   GET   /api/v1/surveys/:id          → SurveyDetail
//   POST  /api/v1/surveys/:id/submit   → SubmitSurveyResp
//   GET   /api/v1/surveys/results      → {items, total}
//   GET   /api/v1/surveys/results/:resultId → SurveyResultDetail
//
// 注：gin 路由中 /surveys/results 必须先于 /surveys/:id 注册（静态段优先）。
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/session"

	"github.com/gin-gonic/gin"
)

// SurveyHandler 处理 /api/v1/surveys/* 端点
type SurveyHandler struct {
	assessment downstream.AssessmentClient
}

// NewSurveyHandler 构造
func NewSurveyHandler(assessment downstream.AssessmentClient) *SurveyHandler {
	return &SurveyHandler{assessment: assessment}
}

// Register 注册路由（静态段优先：results 先于 :id）
func (h *SurveyHandler) Register(r *gin.Engine) {
	r.GET("/api/v1/surveys", h.listSurveys)
	r.GET("/api/v1/surveys/results", h.listResults)
	r.GET("/api/v1/surveys/results/:resultId", h.getResult)
	r.GET("/api/v1/surveys/:id", h.getSurvey)
	r.POST("/api/v1/surveys/:id/submit", h.submitSurvey)
}

func (h *SurveyHandler) listSurveys(c *gin.Context) {
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	items, total, err := h.assessment.ListSurveys(session.WithRequestAuth(c), limit)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"items": items, "total": total})
}

func (h *SurveyHandler) getSurvey(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid survey id")
		return
	}
	s, err := h.assessment.GetSurvey(session.WithRequestAuth(c), id)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, s)
}

func (h *SurveyHandler) submitSurvey(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid survey id")
		return
	}
	var req downstream.SubmitSurveyReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil || req.Answers == nil {
		Fail(c, http.StatusBadRequest, 1, "validation: answers is required")
		return
	}
	resp, err := h.assessment.SubmitSurvey(session.WithRequestAuth(c), id, req)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, resp)
}

func (h *SurveyHandler) listResults(c *gin.Context) {
	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	items, total, err := h.assessment.ListResults(session.WithRequestAuth(c), limit)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"items": items, "total": total})
}

func (h *SurveyHandler) getResult(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("resultId"), 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid result id")
		return
	}
	r, err := h.assessment.GetResult(session.WithRequestAuth(c), id)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, r)
}
