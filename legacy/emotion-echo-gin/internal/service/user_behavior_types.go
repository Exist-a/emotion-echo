package service

// DayNightPattern 昼夜使用模式
type DayNightPattern struct {
	Periods []PeriodStat `json:"periods"`
}

// PeriodStat 时间段统计
type PeriodStat struct {
	Label string `json:"label"`
	Hours string `json:"hours"`
	Value int    `json:"value"` // 消息数
	Ratio int    `json:"ratio"` // 占比(%)
}

// InteractionDepth 互动深度
type InteractionDepth struct {
	AvgSessionRounds   float64 `json:"avgSessionRounds"`   // 平均会话轮数
	MaxConsecutiveDays int     `json:"maxConsecutiveDays"` // 最长连续对话天数
	TotalConversations int     `json:"totalConversations"` // 总会话数
	TotalMessages      int     `json:"totalMessages"`      // 总消息数
	AvgMessagesPerDay  float64 `json:"avgMessagesPerDay"`  // 日均消息数
}

// FrequencyTrend 对话频次趋势
type FrequencyTrend struct {
	Dates        []string `json:"dates"`        // 日期列表
	MessageCount []int    `json:"messageCount"` // 每日消息数
}
