// Package types — user_behavior.go
//
// Stage 30-A user-behavior endpoint request/response types.
package types

import (
	"emotion-echo-analytics-svc/internal/repository"
)

// Re-exports（同 reports.go 模式）
type (
	InteractionDepth = repository.InteractionDepth
	DailyCount       = repository.DailyCount
)

// GetDayNightPatternReq GET /api/v1/user-behavior/day-night
type GetDayNightPatternReq struct {
	UserID    int64  `json:"userId"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

// GetDayNightPatternResp Pattern: {hour: count} 完整 24 个 key (0..23)
type GetDayNightPatternResp struct {
	Pattern map[int]int64 `json:"pattern"`
}

// GetInteractionDepthReq GET /api/v1/user-behavior/depth
type GetInteractionDepthReq struct {
	UserID    int64  `json:"userId"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

// GetInteractionDepthResp
type GetInteractionDepthResp struct {
	Depth *InteractionDepth `json:"depth"`
}

// GetFrequencyTrendReq GET /api/v1/user-behavior/frequency
type GetFrequencyTrendReq struct {
	UserID    int64  `json:"userId"`
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

// GetFrequencyTrendResp
type GetFrequencyTrendResp struct {
	Counts []DailyCount `json:"counts"`
}