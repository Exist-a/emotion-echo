// Package handler — summary.go
//
// fix/chart-contract-alignment: BFF 端 rule-based summary 模板生成器。
//
// 设计目标：把 analytics-svc 返回的 *DailyReport / *TrendReport 转成一段
// 中文"日记摘要"文本，让前端 dashboard 的 summary 字段有内容可显示。
//
// 不调 LLM / 不查 DB / 纯字符串函数——可单元测试、无副作用。
//
// 历史背景：stage-30-A 写 BFF 时 OK(c, gin.H{"report": r}) 把数据塞单数 key，
// 前端 useApi 解出 data.data 后读 reportData.summary 拿到 undefined。
// 本模块提供 Summary 字段生成；模板规则见各函数注释。
package handler

import (
	"fmt"
	"sort"

	"emotion-echo-web-bff/internal/downstream"
)

// 定性词阈值（基于 avgSentiment ∈ [-1, 1]）
const (
	sentimentPositiveThreshold = 0.3
	sentimentNeutralThreshold  = 0.0
)

// emotionLabels 中文化映射（前端 utils/getEmotionLabel 同源）
var emotionLabels = map[string]string{
	"happy":    "开心",
	"sad":      "低落",
	"angry":    "烦躁",
	"anxious":  "焦虑",
	"neutral":  "平静",
	"calm":     "宁静",
	"surprise": "惊喜",
	"disgust":  "反感",
	"fear":     "不安",
}

// translateEmotion 未知 emotion 原样输出，避免硬塞"未知"
func translateEmotion(e string) string {
	if v, ok := emotionLabels[e]; ok {
		return v
	}
	return e
}

// pickTopEmotion 返回 (name, value, isEmpty)
//
// 按 count 倒序排列；并列时按键字母序——保证测试断言稳定。
// 空 counts 返回 ("", 0, true)。
func pickTopEmotion(counts map[string]int64) (string, int64, bool) {
	if len(counts) == 0 {
		return "", 0, true
	}
	type kv struct {
		k string
		v int64
	}
	pairs := make([]kv, 0, len(counts))
	for k, v := range counts {
		pairs = append(pairs, kv{k, v})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].v != pairs[j].v {
			return pairs[i].v > pairs[j].v
		}
		return pairs[i].k < pairs[j].k
	})
	return pairs[0].k, pairs[0].v, false
}

// moodWord 按 avgSentiment 返回中文定性词
func moodWord(avg float64) string {
	switch {
	case avg >= sentimentPositiveThreshold:
		return "积极"
	case avg > sentimentNeutralThreshold:
		return "平稳"
	default:
		return "低落"
	}
}

// BuildDaily 给 dailyReport 生成中文 summary。
//
// 模板：
//   "{date}，你共有 {conversationCount} 段对话，{messageCount} 条消息。
//    主要情绪是 {topEmotionCN}（{topEmotionCount} 次），整体心境 {moodWord}。
//    今天继续和 Echo 聊聊吧。"
//
// 边界：counts 全空时退化为"今天还没有情绪记录"。
func BuildDaily(r *downstream.DailyReport) string {
	if r == nil {
		return "今天还没有对话记录。"
	}
	mood := moodWord(r.AvgSentiment)
	topName, topCnt, empty := pickTopEmotion(r.EmotionCounts)
	if empty {
		return fmt.Sprintf(
			"%s，你共有 %d 段对话，%d 条消息。整体心境 %s。今天继续和 Echo 聊聊吧。",
			r.Date, r.ConversationCount, r.MessageCount, mood,
		)
	}
	return fmt.Sprintf(
		"%s，你共有 %d 段对话，%d 条消息。主要情绪是 %s（%d 次），整体心境 %s。今天继续和 Echo 聊聊吧。",
		r.Date, r.ConversationCount, r.MessageCount,
		translateEmotion(topName), topCnt, mood,
	)
}

// BuildTrend 给 trendReport 生成中文 summary（weekly/monthly/yearly 共用）。
//
// 模板：
//   "从 {startDate} 到 {endDate}，共 {pointCount} 天有记录，
//    主情绪是 {topEmotionCN}，整体心境 {moodWord}。"
func BuildTrend(r *downstream.TrendReport) string {
	if r == nil || len(r.Points) == 0 {
		return "区间内暂无对话记录。"
	}
	// 聚合：count 总和 + primaryEmotion 投票（最大 count 胜）+ avgSentiment 简单平均
	var totalCount int64
	emotionCount := make(map[string]int64)
	var sumSentiment float64
	for _, p := range r.Points {
		totalCount += p.Count
		if p.PrimaryEmotion != "" {
			emotionCount[p.PrimaryEmotion] += p.Count
		}
		sumSentiment += p.AvgSentiment
	}
	avgSent := sumSentiment / float64(len(r.Points))
	topName, _, _ := pickTopEmotion(emotionCount)
	mood := moodWord(avgSent)
	return fmt.Sprintf(
		"从 %s 到 %s，区间内共 %d 条情绪记录，主导情绪是 %s，整体心境 %s。",
		r.StartDate, r.EndDate, totalCount,
		translateEmotion(topName), mood,
	)
}
