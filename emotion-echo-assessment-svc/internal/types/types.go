package types

type HealthResp struct {
	Status  string `json:"status"`
	Time    int64  `json:"time"`
	Service string `json:"service"`
	Version string `json:"version"`
	DbOK    bool   `json:"dbOk"`
}

// ListSurveysReq GET /api/v1/surveys
type ListSurveysReq struct {
	Limit int `form:"limit,default=50"`
}

// ListSurveysResp 量表列表项
type SurveyItem struct {
	ID          uint64 `json:"id"`
	Code        string `json:"code"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Category    string `json:"category"`
	QuestionNum int    `json:"questionNum"`
	Version     int    `json:"version"`
}

type ListSurveysResp struct {
	Items []SurveyItem `json:"items"`
	Total int          `json:"total"`
}

// GetSurveyReq GET /api/v1/surveys/:id
type GetSurveyReq struct {
	Id uint64 `path:"id"`
}

// GetSurveyResp 量表详情（含题目）
type GetSurveyResp struct {
	ID        uint64         `json:"id"`
	Code      string         `json:"code"`
	Title     string         `json:"title"`
	Category  string         `json:"category"`
	Version   int            `json:"version"`
	Questions map[string]any `json:"questions"`
}

// SubmitSurveyReq POST /api/v1/surveys/:id/submit
//
// Answers 是 {questionId: score} 的 map
type SubmitSurveyReq struct {
	SurveyId    uint64         `path:"id"`
	Answers     map[string]int `json:"answers"`
	DurationSec int            `json:"durationSec,optional"`
}

type SubmitSurveyResp struct {
	ResultID   uint64  `json:"resultId"`
	SurveyID   uint64  `json:"surveyId"`
	TotalScore float64 `json:"totalScore"`
	Answered   int     `json:"answered"`
	RiskLevel  string  `json:"riskLevel"`
	// FactorScores 分项/维度分数（人格量表为五维度，PSQI 为 7 个 component）。
	// riskLevel=="dimension_profile" 时前端据此渲染雷达图。
	FactorScores map[string]float64 `json:"factorScores,omitempty"`
	// ScoreKind 说明 totalScore 的语义（E2E-F-97 ①）：
	// "risk"（症状总分，有严重度语义）/ "dimension_sum"（人格五维度之和，**无严重度语义**）/
	// "ratio"（按满分比例的通用分档）。消费方按此判断，不要默认 totalScore 是风险分。
	ScoreKind string `json:"scoreKind,omitempty"`
}

// GetSurveyResultReq GET /api/v1/surveys/results/:resultId
type GetSurveyResultReq struct {
	ResultId uint64 `path:"resultId"`
}

// SurveyResultItem 单条结果列表项
type SurveyResultItem struct {
	ResultID    uint64  `json:"resultId"`
	SurveyID    uint64  `json:"surveyId"`
	TotalScore  float64 `json:"totalScore"`
	RiskLevel   string  `json:"riskLevel"`
	SubmittedAt int64   `json:"submittedAt"`
	// FactorScores 维度分数（BFF 注入 AI 人格画像时从列表直接取，免二次请求）
	FactorScores map[string]float64 `json:"factorScores,omitempty"`
	// ScoreKind 见 SubmitSurveyResp.ScoreKind
	ScoreKind string `json:"scoreKind,omitempty"`
}

// GetSurveyResultResp 单条结果详情
type GetSurveyResultResp struct {
	ResultID     uint64             `json:"resultId"`
	SurveyID     uint64             `json:"surveyId"`
	UserID       int64              `json:"userId"`
	TotalScore   float64            `json:"totalScore"`
	RiskLevel    string             `json:"riskLevel"`
	DurationSec  int                `json:"durationSec"`
	Answers      map[string]any     `json:"answers"`
	SubmittedAt  int64              `json:"submittedAt"`
	FactorScores map[string]float64 `json:"factorScores,omitempty"`
	// ScoreKind 见 SubmitSurveyResp.ScoreKind
	ScoreKind string `json:"scoreKind,omitempty"`
}

// ListMyResultsReq GET /api/v1/surveys/results
type ListMyResultsReq struct {
	Limit int `form:"limit,default=20"`
}

// ListMyResultsResp 当前用户的所有结果
type ListMyResultsResp struct {
	Items []SurveyResultItem `json:"items"`
	Total int                `json:"total"`
}
