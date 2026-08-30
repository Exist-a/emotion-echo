// Package types — mental_health.go
//
// Stage 30-A mental-health endpoint request/response types.
package types

import (
	"emotion-echo-analytics-svc/internal/repository"
)

// Re-exports
type (
	MentalAssessment       = repository.MentalAssessment
	DimensionScore         = repository.DimensionScore
	AssessmentHistoryItem  = repository.AssessmentHistoryItem
)

// GetMentalAssessmentReq GET /api/v1/mental-health/assessment?type=daily|weekly|comprehensive
type GetMentalAssessmentReq struct {
	UserID int64  `json:"userId"`
	Type   string `json:"type"`
}

// GetMentalAssessmentResp
type GetMentalAssessmentResp struct {
	Assessment *MentalAssessment `json:"assessment"`
}

// GetMentalHealthHistoryReq GET /api/v1/mental-health/history
//
// cursor: 上次响应 nextCursor（首次空）
// limit: 1-100
type GetMentalHealthHistoryReq struct {
	UserID int64  `json:"userId"`
	Type   string `json:"type"`   // 可选："" / "PHQ-9" / "GAD-7" 等
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit"`
}

// GetMentalHealthHistoryResp
type GetMentalHealthHistoryResp struct {
	Items      []AssessmentHistoryItem `json:"items"`
	NextCursor string                  `json:"nextCursor"`
}

// TriggerMentalHealthReq POST /api/v1/mental-health/trigger
type TriggerMentalHealthReq struct {
	UserID         int64  `json:"userId"`
	AssessmentType string `json:"assessmentType"`
}

// TriggerMentalHealthResp 异步触发，返回 task_id
type TriggerMentalHealthResp struct {
	TaskID  string `json:"taskId"`
	Status  string `json:"status"`  // "accepted" / "queued" / "running"
	TraceID string `json:"traceId,omitempty"`
}

// GetMentalHealthTrendReq GET /api/v1/mental-health/trend?type=weekly|monthly
type GetMentalHealthTrendReq struct {
	UserID    int64  `json:"userId"`
	Type      string `json:"type"` // weekly | monthly
	StartDate string `json:"startDate"`
	EndDate   string `json:"endDate"`
}

// GetMentalHealthTrendResp
type GetMentalHealthTrendResp struct {
	Report *TrendReport `json:"report"`
}