// Package downstream — analytics.go
//
// Stage 30 / stage-30-web-bff.md T2.21-23: AnalyticsClient（BFF → analytics-svc）
//
// analytics-svc（Gin :8904 容器 / :8893 本地）：
//   GET /api/v1/reports/daily?user_id=&date=
//   GET /api/v1/reports/trend?user_id=&type=&start_date=&end_date=
//   GET /api/v1/user-behavior/day-night?user_id=&start_date=&end_date=
//   GET /api/v1/user-behavior/depth?user_id=&start_date=&end_date=
//   GET /api/v1/user-behavior/frequency?user_id=&start_date=&end_date=
//   GET /api/v1/mental-health/assessment?user_id=&type=
//
// 注意：query 参数用 **snake_case**（调研确认 handler 用 c.Query("user_id") 等），
// 响应 JSON 是 camelCase。
package downstream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

// DailyReport 对应 analytics-svc repository.DailyReport
type DailyReport struct {
	UserID              int64            `json:"userId"`
	Date                string           `json:"date"`
	EmotionCounts       map[string]int64 `json:"emotionCounts"`
	MessageCount        int64            `json:"messageCount"`
	ConversationCount   int64            `json:"conversationCount"`
	AssessmentCount     int64            `json:"assessmentCount"`
	AvgSentiment        float64          `json:"avgSentiment"`
	AvgConfidence       float64          `json:"avgConfidence"`
}

// TrendPoint 对应 analytics-svc repository.TrendPoint
type TrendPoint struct {
	Date            string  `json:"date"`
	PrimaryEmotion  string  `json:"primaryEmotion"`
	AvgSentiment    float64 `json:"avgSentiment"`
	AvgConfidence   float64 `json:"avgConfidence"`
	Count           int64   `json:"count"`
}

// TrendReport 对应 analytics-svc repository.TrendReport
type TrendReport struct {
	UserID    int64        `json:"userId"`
	Type      string       `json:"type"`
	StartDate string       `json:"startDate"`
	EndDate   string       `json:"endDate"`
	Points    []TrendPoint `json:"points"`
}

// InteractionDepth 对应 analytics-svc repository.InteractionDepth
type InteractionDepth struct {
	TotalMessages         int64   `json:"totalMessages"`
	TotalConversations    int64   `json:"totalConversations"`
	AvgMessagesPerConv    float64 `json:"avgMessagesPerConv"`
	LongestConversationMs int64   `json:"longestConversationMs"`
}

// DailyCount 对应 analytics-svc repository.DailyCount
type DailyCount struct {
	Date  string `json:"date"`
	Count int64  `json:"count"`
}

// MentalAssessment 对应 analytics-svc repository.MentalAssessment
type MentalAssessment struct {
	UserID       int64             `json:"userId"`
	Type         string            `json:"type"`
	WindowStart  string            `json:"windowStart"`
	WindowEnd    string            `json:"windowEnd"`
	OverallScore float64           `json:"overallScore"`
	RiskLevel    string            `json:"riskLevel"`
	Dimensions   []DimensionScore  `json:"dimensions"`
	GeneratedAt  string            `json:"generatedAt"`
}

// DimensionScore 对应 analytics-svc repository.DimensionScore
type DimensionScore struct {
	Name      string  `json:"name"`
	Score     float64 `json:"score"`
	RiskLevel string  `json:"riskLevel"`
	Count     int     `json:"count"`
}

// AnalyticsClient BFF → analytics-svc HTTP 客户端
type AnalyticsClient interface {
	// DailyReport 日报（emotion 计数 + 平均情绪）
	DailyReport(ctx context.Context, userID int64, date string) (*DailyReport, error)
	// TrendReport 趋势报表（weekly/monthly/yearly）
	TrendReport(ctx context.Context, userID int64, reportType, startDate, endDate string) (*TrendReport, error)
	// DayNightPattern 昼夜模式（24 桶）
	DayNightPattern(ctx context.Context, userID int64, startDate, endDate string) (map[int]int64, error)
	// InteractionDepth 交互深度指标
	InteractionDepth(ctx context.Context, userID int64, startDate, endDate string) (*InteractionDepth, error)
	// FrequencyTrend 频次趋势（按天）
	FrequencyTrend(ctx context.Context, userID int64, startDate, endDate string) ([]DailyCount, error)
	// MentalAssessment 心理评估（daily/weekly/comprehensive；无评估时返回 nil,nil）
	MentalAssessment(ctx context.Context, userID int64, assessmentType string) (*MentalAssessment, error)
}

// AnalyticsClientOptions 构造选项
type AnalyticsClientOptions struct {
	BaseURL   string
	TimeoutMs int
}

// analyticsHTTPClient 是 AnalyticsClient 的 HTTP 实现
type analyticsHTTPClient struct {
	baseURL string
	http    *http.Client
}

// NewAnalyticsClient 构造 AnalyticsClient
func NewAnalyticsClient(opts AnalyticsClientOptions) AnalyticsClient {
	timeout := time.Duration(opts.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &analyticsHTTPClient{
		baseURL: opts.BaseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

func (c *analyticsHTTPClient) DailyReport(ctx context.Context, userID int64, date string) (*DailyReport, error) {
	q := url.Values{}
	q.Set("user_id", fmt.Sprintf("%d", userID))
	if date != "" {
		q.Set("date", date)
	}
	var out struct {
		Report *DailyReport `json:"report"`
	}
	if err := c.get(ctx, "/api/v1/reports/daily", q, &out); err != nil {
		return nil, err
	}
	return out.Report, nil
}

func (c *analyticsHTTPClient) TrendReport(ctx context.Context, userID int64, reportType, startDate, endDate string) (*TrendReport, error) {
	q := url.Values{}
	q.Set("user_id", fmt.Sprintf("%d", userID))
	if reportType != "" {
		q.Set("type", reportType)
	}
	if startDate != "" {
		q.Set("start_date", startDate)
	}
	if endDate != "" {
		q.Set("end_date", endDate)
	}
	var out struct {
		Report *TrendReport `json:"report"`
	}
	if err := c.get(ctx, "/api/v1/reports/trend", q, &out); err != nil {
		return nil, err
	}
	return out.Report, nil
}

func (c *analyticsHTTPClient) DayNightPattern(ctx context.Context, userID int64, startDate, endDate string) (map[int]int64, error) {
	q := c.baseQuery(userID, startDate, endDate)
	var out struct {
		Pattern map[int]int64 `json:"pattern"`
	}
	if err := c.get(ctx, "/api/v1/user-behavior/day-night", q, &out); err != nil {
		return nil, err
	}
	return out.Pattern, nil
}

func (c *analyticsHTTPClient) InteractionDepth(ctx context.Context, userID int64, startDate, endDate string) (*InteractionDepth, error) {
	q := c.baseQuery(userID, startDate, endDate)
	var out struct {
		Depth *InteractionDepth `json:"depth"`
	}
	if err := c.get(ctx, "/api/v1/user-behavior/depth", q, &out); err != nil {
		return nil, err
	}
	return out.Depth, nil
}

func (c *analyticsHTTPClient) FrequencyTrend(ctx context.Context, userID int64, startDate, endDate string) ([]DailyCount, error) {
	q := c.baseQuery(userID, startDate, endDate)
	var out struct {
		Counts []DailyCount `json:"counts"`
	}
	if err := c.get(ctx, "/api/v1/user-behavior/frequency", q, &out); err != nil {
		return nil, err
	}
	return out.Counts, nil
}

func (c *analyticsHTTPClient) MentalAssessment(ctx context.Context, userID int64, assessmentType string) (*MentalAssessment, error) {
	q := url.Values{}
	q.Set("user_id", fmt.Sprintf("%d", userID))
	if assessmentType != "" {
		q.Set("type", assessmentType)
	}
	var out struct {
		Assessment *MentalAssessment `json:"assessment"`
	}
	if err := c.get(ctx, "/api/v1/mental-health/assessment", q, &out); err != nil {
		return nil, err
	}
	return out.Assessment, nil
}

// baseQuery user_id + 可选 start_date/end_date（snake_case）
func (c *analyticsHTTPClient) baseQuery(userID int64, startDate, endDate string) url.Values {
	q := url.Values{}
	q.Set("user_id", fmt.Sprintf("%d", userID))
	if startDate != "" {
		q.Set("start_date", startDate)
	}
	if endDate != "" {
		q.Set("end_date", endDate)
	}
	return q
}

// get 通用 GET + 解码
func (c *analyticsHTTPClient) get(ctx context.Context, path string, q url.Values, out any) error {
	u := c.baseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	applyAuthHeader(httpReq, ctx)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return fmt.Errorf("downstream: analytics GET %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return readError(resp)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("downstream: decode analytics resp: %w", err)
	}
	return nil
}
