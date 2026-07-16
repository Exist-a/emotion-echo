package service

// TriggerAssessmentRequest 触发评估请求
type TriggerAssessmentRequest struct {
	Type       string `json:"type" binding:"omitempty,oneof=daily weekly comprehensive"`
	PeriodDays int    `json:"periodDays" binding:"omitempty,min=1,max=30"`
}

// TriggerAssessmentResponse 触发评估响应
type TriggerAssessmentResponse struct {
	Status        string `json:"status"`
	EstimatedTime string `json:"estimatedTime"`
}

// TrendData 趋势数据
type TrendData struct {
	Type   string         `json:"type"`
	Dates  []string       `json:"dates"`
	Scores map[string][]int `json:"scores"`
}
