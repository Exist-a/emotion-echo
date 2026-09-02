// Package handler — analytics_view.go
//
// fix/chart-contract-alignment: BFF 端 presentation 层。
// 把 downstream.AnalyticsClient 返回的内部结构转换成前端期望的扁平 JSON 形状。
//
// 历史背景：
//   - 前端 useApi.get<T>() 把 data.data 直接当 T 返回；
//   - stage-30-A 时 BFF 用 OK(c, gin.H{"report": r}) 把数据塞单数 key，
//     前端 dashboard 读 reportData.summary / reportData.emotionDistribution 全部 undefined；
//   - analytics-svc 输出的 *DailyReport 是 map[string]int64 形态而非 [{name, value}]，
//     即便前端绕过 .report 这层也读不到正确结构。
//
// 本模块提供 FrontendDailyReport / FrontendEmotionTrend + 转换函数，
// handler 改用 OK(c, toFrontendDailyReport(r)) 后响应直接对齐前端类型。
package handler

import (
	"sort"

	"emotion-echo-web-bff/internal/downstream"
)

// EmotionDistributionItem 前端期望的 {name, value} 数组元素
type EmotionDistributionItem struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}

// FrontendDailyReport 前端 DailyReport 类型契约
type FrontendDailyReport struct {
	Date                string                   `json:"date"`
	Summary             string                   `json:"summary"`
	EmotionDistribution []EmotionDistributionItem `json:"emotionDistribution"`
	ConversationCount   int64                    `json:"conversationCount"`
	MessageCount        int64                    `json:"messageCount"`
}

// FrontendEmotionTrendSeries 前端线图 series 元素
type FrontendEmotionTrendSeries struct {
	Name string  `json:"name"`
	Data []int64 `json:"data"`
}

// FrontendEmotionTrend 前端 EmotionTrend 类型契约
type FrontendEmotionTrend struct {
	Type                string                    `json:"type"`
	Dates               []string                  `json:"dates"`
	Series              []FrontendEmotionTrendSeries `json:"series"`
	Summary             string                    `json:"summary"`
	EmotionDistribution []EmotionDistributionItem `json:"emotionDistribution"`
	ConversationCount   int64                     `json:"conversationCount"`
	MessageCount        int64                     `json:"messageCount"`
}

// toFrontendDailyReport 把下游 *DailyReport 转成前端契约
//
// nil 入参返回零值结构（前端拿到仍是合法 DailyReport，summary 退化）。
func toFrontendDailyReport(r *downstream.DailyReport) *FrontendDailyReport {
	if r == nil {
		return &FrontendDailyReport{Summary: "今日还没有数据。"}
	}
	return &FrontendDailyReport{
		Date:                r.Date,
		Summary:             BuildDaily(r),
		EmotionDistribution: emotionCountsToSlice(r.EmotionCounts),
		ConversationCount:   r.ConversationCount,
		MessageCount:        r.MessageCount,
	}
}

// toFrontendTrendReport 把下游 *TrendReport 转成前端契约
//
// points[] → dates[] + 按 primaryEmotion 分桶的 series[] + 累计 count。
func toFrontendTrendReport(r *downstream.TrendReport) *FrontendEmotionTrend {
	if r == nil || len(r.Points) == 0 {
		return &FrontendEmotionTrend{
			Type:    rTypeOrEmpty(r),
			Dates:   []string{},
			Series:  []FrontendEmotionTrendSeries{},
			Summary: BuildTrend(r),
		}
	}

	// 1. 按 primaryEmotion 分桶
	type bucket struct {
		name  string
		date  []string
		count []int64
	}
	buckets := make(map[string]*bucket)
	for _, p := range r.Points {
		key := p.PrimaryEmotion
		if key == "" {
			key = "neutral"
		}
		b, ok := buckets[key]
		if !ok {
			b = &bucket{name: key}
			buckets[key] = b
		}
		b.date = append(b.date, p.Date)
		b.count = append(b.count, p.Count)
	}

	// 2. dates[] = 全部 points 的 date 去重保序
	seen := make(map[string]bool)
	var dates []string
	for _, p := range r.Points {
		if !seen[p.Date] {
			seen[p.Date] = true
			dates = append(dates, p.Date)
		}
	}

	// 3. series[] 转换：每个 bucket 展开成跟 dates 等长的 count 切片（缺的填 0）
	dateIdx := make(map[string]int, len(dates))
	for i, d := range dates {
		dateIdx[d] = i
	}
	emotionTotal := make(map[string]int64)
	var series []FrontendEmotionTrendSeries
	for _, b := range buckets {
		full := make([]int64, len(dates))
		for _, c := range b.count {
			_ = c
		}
		// 用 bucket.date + bucket.count 配对填回 full
		for i, d := range b.date {
			if idx, ok := dateIdx[d]; ok {
				// 取对应位置的 count
				var c int64
				if i < len(b.count) {
					c = b.count[i]
				}
				full[idx] = c
				emotionTotal[b.name] += c
			}
		}
		series = append(series, FrontendEmotionTrendSeries{Name: b.name, Data: full})
	}

	// 4. conversationCount / messageCount：TrendReport 没有，从 points 累计
	var totalCount int64
	for _, p := range r.Points {
		totalCount += p.Count
	}

	return &FrontendEmotionTrend{
		Type:                rTypeOrEmpty(r),
		Dates:               dates,
		Series:              series,
		Summary:             BuildTrend(r),
		EmotionDistribution: emotionTotalToSlice(emotionTotal),
		MessageCount:        totalCount,
		ConversationCount:   0, // TrendReport 没有 conv 维度；前端目前模板用 0 等同"未提供"
	}
}

func rTypeOrEmpty(r *downstream.TrendReport) string {
	if r == nil {
		return ""
	}
	return r.Type
}

// emotionCountsToSlice 把 map → [{name, value}]，按 value 倒序 + name 字母序
func emotionCountsToSlice(m map[string]int64) []EmotionDistributionItem {
	if len(m) == 0 {
		return []EmotionDistributionItem{}
	}
	pairs := make([]EmotionDistributionItem, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, EmotionDistributionItem{Name: k, Value: v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].Value != pairs[j].Value {
			return pairs[i].Value > pairs[j].Value
		}
		return pairs[i].Name < pairs[j].Name
	})
	return pairs
}

// emotionTotalToSlice 给 trendReport 用（bucket 已聚合）
func emotionTotalToSlice(m map[string]int64) []EmotionDistributionItem {
	return emotionCountsToSlice(m)
}
