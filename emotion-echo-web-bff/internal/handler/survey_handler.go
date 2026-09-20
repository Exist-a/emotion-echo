// Package handler — survey_handler.go
//
// Stage 30 / stage-30-web-bff.md T4.42-46: survey handler（BFF → assessment-svc）
//
// 端点：
//
//	GET   /api/v1/surveys              → {items, total}
//	GET   /api/v1/surveys/:id          → SurveyDetail
//	POST  /api/v1/surveys/:id/submit   → SubmitSurveyResp
//	GET   /api/v1/surveys/results      → {items, total}
//	GET   /api/v1/surveys/results/:resultId → SurveyResultDetail
//
// 注：gin 路由中 /surveys/results 必须先于 /surveys/:id 注册（静态段优先）。
package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"time"

	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/session"

	"github.com/gin-gonic/gin"
)

// SurveyHandler 处理 /api/v1/surveys/* 端点
type SurveyHandler struct {
	assessment     downstream.AssessmentClient
	assessmentBase string // assessment-svc HTTP base URL（绕过 gRPC 转换用）
	assessmentHTTP *http.Client
}

// NewSurveyHandler 构造
func NewSurveyHandler(assessment downstream.AssessmentClient) *SurveyHandler {
	return &SurveyHandler{
		assessment:     assessment,
		assessmentHTTP: &http.Client{Timeout: 5 * time.Second},
	}
}

// WithAssessmentBase 注入 assessment-svc HTTP base URL（用于绕过 gRPC questions 转换）
func (h *SurveyHandler) WithAssessmentBase(base string) *SurveyHandler {
	h.assessmentBase = base
	return h
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
	// 优先走 HTTP：proto `SurveyItem` 无 description 字段，gRPC 路径会静默丢弃
	// （E2E-14 实测 3 个量表描述全空；E2E-13 已就此决议 survey 端点走 HTTP，
	//  见 adr-2026-09-survey-http-bypass.md —— 本端点是当时漏掉的第 5 个）
	if h.assessmentBase != "" {
		h.listSurveysHTTP(c, limit)
		return
	}
	items, total, err := h.assessment.ListSurveys(session.WithRequestAuth(c), limit)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"items": items, "total": total})
}

// listSurveysHTTP 直接调 assessment-svc HTTP 端点，保留 description 等完整字段
func (h *SurveyHandler) listSurveysHTTP(c *gin.Context, limit int) {
	url := fmt.Sprintf("%s/api/v1/surveys?limit=%d", h.assessmentBase, limit)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, url, nil)
	if err != nil {
		Fail(c, http.StatusInternalServerError, 1, err.Error())
		return
	}
	if uid, ok := downstream.UserIDFromContext(session.WithRequestAuth(c)); ok {
		req.Header.Set("X-User-Id", strconv.FormatInt(uid, 10))
	}
	resp, err := h.assessmentHTTP.Do(req)
	if err != nil {
		Fail(c, http.StatusBadGateway, 1, fmt.Errorf("assessment-svc: %w", err).Error())
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		Fail(c, resp.StatusCode, 1, string(respBody))
		return
	}
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		Fail(c, http.StatusBadGateway, 1, err.Error())
		return
	}
	OK(c, result)
}

func (h *SurveyHandler) getSurvey(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid survey id")
		return
	}
	// 优先走 HTTP 直取（保留 JSONB 原始格式），gRPC 转换会丢失 options 结构
	if h.assessmentBase != "" {
		h.getSurveyHTTP(c, id)
		return
	}
	s, err := h.assessment.GetSurvey(session.WithRequestAuth(c), id)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, s)
}

// getSurveyHTTP 直接调 assessment-svc HTTP 端点，保留 questions JSONB 原始格式
func (h *SurveyHandler) getSurveyHTTP(c *gin.Context, id uint64) {
	url := fmt.Sprintf("%s/api/v1/surveys/%d", h.assessmentBase, id)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, url, nil)
	if err != nil {
		Fail(c, http.StatusInternalServerError, 1, err.Error())
		return
	}
	// 从 context 提取 user ID（APISIX 注入或 BFF auth middleware 设置）
	if uid, ok := downstream.UserIDFromContext(session.WithRequestAuth(c)); ok {
		req.Header.Set("X-User-Id", strconv.FormatInt(uid, 10))
	}
	resp, err := h.assessmentHTTP.Do(req)
	if err != nil {
		Fail(c, http.StatusBadGateway, 1, fmt.Errorf("assessment-svc: %w", err).Error())
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		Fail(c, resp.StatusCode, 1, string(body))
		return
	}
	// 解包 assessment-svc 的 JSON 响应，提取 questions 并转为数组
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		Fail(c, http.StatusBadGateway, 1, err.Error())
		return
	}
	// questions 从 map 转为有序数组（按 key q1,q2,...,qN 排序），注入 id 字段
	if questionsRaw, ok := raw["questions"].(map[string]any); ok {
		keys := make([]string, 0, len(questionsRaw))
		for k := range questionsRaw {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		questions := make([]map[string]any, 0, len(questionsRaw))
		for _, k := range keys {
			if m, ok := questionsRaw[k].(map[string]any); ok {
				m["id"] = k
				questions = append(questions, m)
			}
		}
		raw["questions"] = questions
	}
	OK(c, raw)
}

func (h *SurveyHandler) submitSurvey(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid survey id")
		return
	}
	// 优先走 HTTP 直传（gRPC 会把 "q1" 转成数字再转回 "1"，scorer 期望 "q1"）
	if h.assessmentBase != "" {
		h.submitSurveyHTTP(c, id)
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

// submitSurveyHTTP 直接调 assessment-svc HTTP 端点，保留 answers key 原始格式
func (h *SurveyHandler) submitSurveyHTTP(c *gin.Context, id uint64) {
	body, _ := io.ReadAll(c.Request.Body)
	url := fmt.Sprintf("%s/api/v1/surveys/%d/submit", h.assessmentBase, id)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		Fail(c, http.StatusInternalServerError, 1, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if uid, ok := downstream.UserIDFromContext(session.WithRequestAuth(c)); ok {
		req.Header.Set("X-User-Id", strconv.FormatInt(uid, 10))
	}
	resp, err := h.assessmentHTTP.Do(req)
	if err != nil {
		Fail(c, http.StatusBadGateway, 1, fmt.Errorf("assessment-svc: %w", err).Error())
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		Fail(c, resp.StatusCode, 1, string(respBody))
		return
	}
	// 透传 assessment-svc 响应
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		Fail(c, http.StatusBadGateway, 1, err.Error())
		return
	}
	OK(c, result)
}

func (h *SurveyHandler) listResults(c *gin.Context) {
	limit := 20
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	// 优先走 HTTP（gRPC ListResults 未实现）
	if h.assessmentBase != "" {
		h.listResultsHTTP(c, limit)
		return
	}
	items, total, err := h.assessment.ListResults(session.WithRequestAuth(c), limit)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"items": items, "total": total})
}

// listResultsHTTP 直接调 assessment-svc HTTP 端点
func (h *SurveyHandler) listResultsHTTP(c *gin.Context, limit int) {
	url := fmt.Sprintf("%s/api/v1/surveys/results?limit=%d", h.assessmentBase, limit)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, url, nil)
	if err != nil {
		Fail(c, http.StatusInternalServerError, 1, err.Error())
		return
	}
	if uid, ok := downstream.UserIDFromContext(session.WithRequestAuth(c)); ok {
		req.Header.Set("X-User-Id", strconv.FormatInt(uid, 10))
	}
	resp, err := h.assessmentHTTP.Do(req)
	if err != nil {
		Fail(c, http.StatusBadGateway, 1, fmt.Errorf("assessment-svc: %w", err).Error())
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		Fail(c, resp.StatusCode, 1, string(respBody))
		return
	}
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		Fail(c, http.StatusBadGateway, 1, err.Error())
		return
	}
	OK(c, result)
}

func (h *SurveyHandler) getResult(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("resultId"), 10, 64)
	if err != nil || id == 0 {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid result id")
		return
	}
	// 优先走 HTTP（gRPC GetResult 未实现）
	if h.assessmentBase != "" {
		h.getResultHTTP(c, id)
		return
	}
	r, err := h.assessment.GetResult(session.WithRequestAuth(c), id)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, r)
}

// getResultHTTP 直接调 assessment-svc HTTP 端点
func (h *SurveyHandler) getResultHTTP(c *gin.Context, id uint64) {
	url := fmt.Sprintf("%s/api/v1/surveys/results/%d", h.assessmentBase, id)
	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, url, nil)
	if err != nil {
		Fail(c, http.StatusInternalServerError, 1, err.Error())
		return
	}
	if uid, ok := downstream.UserIDFromContext(session.WithRequestAuth(c)); ok {
		req.Header.Set("X-User-Id", strconv.FormatInt(uid, 10))
	}
	resp, err := h.assessmentHTTP.Do(req)
	if err != nil {
		Fail(c, http.StatusBadGateway, 1, fmt.Errorf("assessment-svc: %w", err).Error())
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		Fail(c, resp.StatusCode, 1, string(respBody))
		return
	}
	var result map[string]any
	if err := json.Unmarshal(respBody, &result); err != nil {
		Fail(c, http.StatusBadGateway, 1, err.Error())
		return
	}
	OK(c, result)
}
