// Package downstream — assessment.go
//
// Stage 30 / stage-30-web-bff.md T2.18-20: AssessmentClient（BFF → assessment-svc）
//
// assessment-svc（Gin :8889）：
//   GET   /api/v1/surveys                → {items: [SurveyItem], total}
//   GET   /api/v1/surveys/:id            → {id, code, title, category, version, questions}
//   POST  /api/v1/surveys/:id/submit     → {resultId, surveyId, totalScore, answered, riskLevel}
//   GET   /api/v1/surveys/results        → {items: [SurveyResultItem], total}
//   GET   /api/v1/surveys/results/:resultId → {resultId, surveyId, userId, totalScore, riskLevel, durationSec, answers, submittedAt}
package downstream

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// SurveyItem 对应 assessment-svc types.SurveyItem
type SurveyItem struct {
	ID          uint64 `json:"id"`
	Code        string `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	QuestionNum int    `json:"questionNum"`
	Version     int    `json:"version"`
}

// SurveyDetail 对应 assessment-svc types.GetSurveyResp（无 description）
type SurveyDetail struct {
	ID        uint64         `json:"id"`
	Code      string         `json:"code"`
	Title     string         `json:"title"`
	Category  string         `json:"category"`
	Version   int            `json:"version"`
	Questions map[string]any `json:"questions"`
}

// SubmitSurveyReq 对应 assessment-svc types.SubmitSurveyReq
type SubmitSurveyReq struct {
	Answers     map[string]int `json:"answers"`
	DurationSec int            `json:"durationSec,omitempty"`
}

// SubmitSurveyResp 对应 assessment-svc types.SubmitSurveyResp
type SubmitSurveyResp struct {
	ResultID   uint64  `json:"resultId"`
	SurveyID   uint64  `json:"surveyId"`
	TotalScore float64 `json:"totalScore"`
	Answered   int     `json:"answered"`
	RiskLevel  string  `json:"riskLevel"`
}

// SurveyResultItem 对应 assessment-svc types.SurveyResultItem
type SurveyResultItem struct {
	ResultID    uint64  `json:"resultId"`
	SurveyID    uint64  `json:"surveyId"`
	TotalScore  float64 `json:"totalScore"`
	RiskLevel   string  `json:"riskLevel"`
	SubmittedAt int64   `json:"submittedAt"`
}

// SurveyResultDetail 对应 assessment-svc types.GetSurveyResultResp
type SurveyResultDetail struct {
	ResultID    uint64         `json:"resultId"`
	SurveyID    uint64         `json:"surveyId"`
	UserID      int64          `json:"userId"`
	TotalScore  float64        `json:"totalScore"`
	RiskLevel   string         `json:"riskLevel"`
	DurationSec int            `json:"durationSec"`
	Answers     map[string]any `json:"answers"`
	SubmittedAt int64          `json:"submittedAt"`
}

// AssessmentClient BFF → assessment-svc HTTP 客户端
type AssessmentClient interface {
	// ListSurveys 列出问卷
	ListSurveys(ctx context.Context, limit int) ([]SurveyItem, int, error)
	// GetSurvey 问卷详情（含题目）
	GetSurvey(ctx context.Context, id uint64) (*SurveyDetail, error)
	// SubmitSurvey 提交问卷答案
	SubmitSurvey(ctx context.Context, id uint64, req SubmitSurveyReq) (*SubmitSurveyResp, error)
	// ListResults 我的评估结果列表
	ListResults(ctx context.Context, limit int) ([]SurveyResultItem, int, error)
	// GetResult 单条评估结果详情
	GetResult(ctx context.Context, resultID uint64) (*SurveyResultDetail, error)
}

// AssessmentClientOptions 构造选项
type AssessmentClientOptions struct {
	BaseURL   string
	TimeoutMs int
}

// assessmentHTTPClient 是 AssessmentClient 的 HTTP 实现
type assessmentHTTPClient struct {
	baseURL string
	http    *http.Client
}

// NewAssessmentClient 构造 AssessmentClient
func NewAssessmentClient(opts AssessmentClientOptions) AssessmentClient {
	timeout := time.Duration(opts.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &assessmentHTTPClient{
		baseURL: opts.BaseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *assessmentHTTPClient) ListSurveys(ctx context.Context, limit int) ([]SurveyItem, int, error) {
	url := c.baseURL + "/api/v1/surveys"
	if limit > 0 {
		url += "?limit=" + strconv.Itoa(limit)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("downstream: list surveys: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, 0, readError(resp)
	}
	var wrapped struct {
		Items []SurveyItem `json:"items"`
		Total int          `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapped); err != nil {
		return nil, 0, fmt.Errorf("downstream: decode surveys resp: %w", err)
	}
	return wrapped.Items, wrapped.Total, nil
}

func (c *assessmentHTTPClient) GetSurvey(ctx context.Context, id uint64) (*SurveyDetail, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/surveys/"+strconv.FormatUint(id, 10), nil)
	if err != nil {
		return nil, err
	}
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("downstream: get survey: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, readError(resp)
	}
	var out SurveyDetail
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("downstream: decode survey resp: %w", err)
	}
	return &out, nil
}

func (c *assessmentHTTPClient) SubmitSurvey(ctx context.Context, id uint64, req SubmitSurveyReq) (*SubmitSurveyResp, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("downstream: marshal submit: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/api/v1/surveys/"+strconv.FormatUint(id, 10)+"/submit", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("downstream: submit survey: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, readError(resp)
	}
	var out SubmitSurveyResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("downstream: decode submit resp: %w", err)
	}
	return &out, nil
}

func (c *assessmentHTTPClient) ListResults(ctx context.Context, limit int) ([]SurveyResultItem, int, error) {
	url := c.baseURL + "/api/v1/surveys/results"
	if limit > 0 {
		url += "?limit=" + strconv.Itoa(limit)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0, err
	}
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("downstream: list results: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, 0, readError(resp)
	}
	var wrapped struct {
		Items []SurveyResultItem `json:"items"`
		Total int                `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&wrapped); err != nil {
		return nil, 0, fmt.Errorf("downstream: decode results resp: %w", err)
	}
	return wrapped.Items, wrapped.Total, nil
}

func (c *assessmentHTTPClient) GetResult(ctx context.Context, resultID uint64) (*SurveyResultDetail, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/api/v1/surveys/results/"+strconv.FormatUint(resultID, 10), nil)
	if err != nil {
		return nil, err
	}
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("downstream: get result: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, readError(resp)
	}
	var out SurveyResultDetail
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("downstream: decode result resp: %w", err)
	}
	return &out, nil
}
