package service

import "time"

// emotionLabelMap 情绪标签英文到中文的映射
var emotionLabelMap = map[string]string{
	"happy":   "开心",
	"sad":     "悲伤",
	"angry":   "愤怒",
	"anxious": "焦虑",
	"neutral": "中性",
	"unk":     "未知",
}

// intentLabelMap 意图标签英文到中文的映射
var intentLabelMap = map[string]string{
	"emotional_support": "情感疏导",
	"study_help":        "学习问题",
	"tech_help":         "技术问题",
	"career_help":       "职业问题",
	"lifestyle":         "生活问题",
	"other":             "其他",
}

// EmotionDistribution 情绪分布
type EmotionDistribution struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

// IntentDistribution 意图分布
type IntentDistribution struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

// DailyReport 日报
type DailyReport struct {
	Date                string                 `json:"date"`
	Summary             string                 `json:"summary"`
	EmotionDistribution []EmotionDistribution `json:"emotionDistribution"`
	IntentDistribution  []IntentDistribution  `json:"intentDistribution"`
	ConversationCount   int                   `json:"conversationCount"`
	MessageCount        int                   `json:"messageCount"`
	WordCount           int                   `json:"wordCount"`
}

// TrendReport 趋势报表
type TrendReport struct {
	Type                string                `json:"type"`
	Dates               []string              `json:"dates"`
	Series              []SeriesData          `json:"series"`
	Summary             string                `json:"summary"`
	EmotionDistribution []EmotionDistribution `json:"emotionDistribution"`
	IntentDistribution  []IntentDistribution  `json:"intentDistribution"`
	ConversationCount   int                   `json:"conversationCount"`
	MessageCount        int                   `json:"messageCount"`
	WordCount           int                   `json:"wordCount"`
}

// SeriesData 系列数据
type SeriesData struct {
	Name string `json:"name"`
	Data []int  `json:"data"`
}

// TranslateEmotionLabel 将英文情绪标签翻译为中文
func TranslateEmotionLabel(emotion string) string {
	if cn, ok := emotionLabelMap[emotion]; ok {
		return cn
	}
	return emotion
}

// GetDateRange 获取日期范围
func GetDateRange(dateStr string, days int) (startDate, endDate time.Time, err error) {
	reportDate, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return
	}
	startDate = reportDate.Truncate(24 * time.Hour)
	endDate = startDate.Add(24 * time.Hour)
	return
}

// CalculateIntentDistribution 计算意图分布百分比
func CalculateIntentDistribution(intentCounts map[string]int) []IntentDistribution {
	distribution := make([]IntentDistribution, 0)
	totalCount := 0
	for _, count := range intentCounts {
		totalCount += count
	}

	for intent, count := range intentCounts {
		if count > 0 {
			percentage := int(float64(count) / float64(totalCount) * 100)
			distribution = append(distribution, IntentDistribution{
				Name:  intentLabelMap[intent],
				Value: percentage,
			})
		}
	}
	return distribution
}
