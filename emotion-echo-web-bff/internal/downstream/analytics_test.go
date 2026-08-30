// Package downstream — analytics_test.go
//
// Stage 30 / stage-30-web-bff.md T2.21-23 RED: AnalyticsClient 契约测试
package downstream

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAnalyticsClient_DailyReport_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/reports/daily", r.URL.Path)
		// snake_case query 参数
		assert.Equal(t, "42", r.URL.Query().Get("user_id"))
		assert.Equal(t, "2026-08-31", r.URL.Query().Get("date"))

		_ = json.NewEncoder(w).Encode(map[string]any{"report": DailyReport{
			UserID: 42, Date: "2026-08-31",
			EmotionCounts:    map[string]int64{"happy": 3, "sad": 1},
			MessageCount:     4,
			ConversationCount: 1,
			AvgSentiment:     0.5, AvgConfidence: 0.8,
		}})
	}))
	defer srv.Close()

	c := NewAnalyticsClient(AnalyticsClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	r, err := c.DailyReport(context.Background(), 42, "2026-08-31")
	require.NoError(t, err)
	require.NotNil(t, r)
	assert.Equal(t, int64(4), r.MessageCount)
	assert.Equal(t, int64(3), r.EmotionCounts["happy"])
}

func TestAnalyticsClient_TrendReport_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/reports/trend", r.URL.Path)
		assert.Equal(t, "weekly", r.URL.Query().Get("type"))
		assert.Equal(t, "42", r.URL.Query().Get("user_id"))

		_ = json.NewEncoder(w).Encode(map[string]any{"report": TrendReport{
			UserID: 42, Type: "weekly", StartDate: "2026-08-01", EndDate: "2026-08-31",
			Points: []TrendPoint{{Date: "2026-08-31", PrimaryEmotion: "happy", AvgSentiment: 0.6, Count: 5}},
		}})
	}))
	defer srv.Close()

	c := NewAnalyticsClient(AnalyticsClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	r, err := c.TrendReport(context.Background(), 42, "weekly", "2026-08-01", "2026-08-31")
	require.NoError(t, err)
	require.NotNil(t, r)
	require.Len(t, r.Points, 1)
	assert.Equal(t, "happy", r.Points[0].PrimaryEmotion)
}

func TestAnalyticsClient_DayNightPattern_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/user-behavior/day-night", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"pattern": map[int]int64{9: 2, 20: 5}})
	}))
	defer srv.Close()

	c := NewAnalyticsClient(AnalyticsClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	p, err := c.DayNightPattern(context.Background(), 42, "2026-08-01", "2026-08-31")
	require.NoError(t, err)
	assert.Equal(t, int64(2), p[9])
	assert.Equal(t, int64(5), p[20])
}

func TestAnalyticsClient_InteractionDepth_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/user-behavior/depth", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"depth": InteractionDepth{
			TotalMessages: 10, TotalConversations: 2, AvgMessagesPerConv: 5.0, LongestConversationMs: 3600000,
		}})
	}))
	defer srv.Close()

	c := NewAnalyticsClient(AnalyticsClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	d, err := c.InteractionDepth(context.Background(), 42, "", "")
	require.NoError(t, err)
	require.NotNil(t, d)
	assert.Equal(t, int64(10), d.TotalMessages)
	assert.Equal(t, float64(5.0), d.AvgMessagesPerConv)
}

func TestAnalyticsClient_FrequencyTrend_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/user-behavior/frequency", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"counts": []DailyCount{
			{Date: "2026-08-30", Count: 3}, {Date: "2026-08-31", Count: 5},
		}})
	}))
	defer srv.Close()

	c := NewAnalyticsClient(AnalyticsClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	counts, err := c.FrequencyTrend(context.Background(), 42, "", "")
	require.NoError(t, err)
	assert.Len(t, counts, 2)
	assert.Equal(t, int64(5), counts[1].Count)
}

func TestAnalyticsClient_MentalAssessment_NilWhenEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/v1/mental-health/assessment", r.URL.Path)
		assert.Equal(t, "daily", r.URL.Query().Get("type"))
		_ = json.NewEncoder(w).Encode(map[string]any{"assessment": nil})
	}))
	defer srv.Close()

	c := NewAnalyticsClient(AnalyticsClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	a, err := c.MentalAssessment(context.Background(), 42, "daily")
	require.NoError(t, err)
	assert.Nil(t, a, "无评估时下游返 assessment=null → client 应返 nil,nil")
}

func TestAnalyticsClient_ValidationError_ReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "validation: user_id is required"})
	}))
	defer srv.Close()

	c := NewAnalyticsClient(AnalyticsClientOptions{BaseURL: srv.URL, TimeoutMs: 1000})
	_, err := c.DailyReport(context.Background(), 0, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user_id is required")
}
