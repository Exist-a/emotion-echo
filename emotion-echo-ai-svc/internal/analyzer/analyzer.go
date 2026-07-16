// Package analyzer 情绪分析器
//
// 核心接口：Analyzer
//
// 实现：
//   - KeywordAnalyzer：基于关键词的占位实现（用于开发/演示）
//   - 未来接 LLM/ML 模型时，只需替换实现
//
// 设计原则：业务逻辑（如何分析）独立于数据访问（写 emotion_analysis 表）
package analyzer

import (
	"context"
	"strings"
)

// EmotionResult 是情绪分析的输出
//
// 与 emotion_echo_ai.emotion_analysis 表对应
type EmotionResult struct {
	PrimaryEmotion string
	SentimentScore float64 // -1.0 (负面) ~ 1.0 (正面)
	Confidence     float64 // 0.0 ~ 1.0
	Model          string  // 用于追溯使用了哪个模型
}

// Analyzer 情绪分析接口
//
// 任何能给定文本输出 EmotionResult 的实现都满足
type Analyzer interface {
	Analyze(ctx context.Context, text string) (*EmotionResult, error)
}

// =====================================================
// KeywordAnalyzer（占位实现）
// =====================================================

// KeywordAnalyzer 用关键词匹配实现情绪分析
//
// 简单但够用：
//   - 正面词 / 负面词计数 → 计算 sentiment_score
//   - 命中关键词最多的类别作为 primary_emotion
//   - confidence = 命中词数 / 总词数（启发式）
//
// 真实生产应该接 LLM/ML，此实现只用于：
//   - 演示与开发
//   - TDD 测试替身
//   - 离线验证管道
type KeywordAnalyzer struct{}

func NewKeywordAnalyzer() *KeywordAnalyzer { return &KeywordAnalyzer{} }

// emotionKeywords 是情绪分类关键词字典
//
// key: 情绪标签
// value: 该情绪的关键词列表（中文）
var emotionKeywords = map[string][]string{
	"happy":   {"开心", "高兴", "快乐", "哈哈", "😊", "😄", "好极了", "棒", "喜欢", "感谢", "谢谢"},
	"sad":     {"难过", "伤心", "哭", "失落", "沮丧", "抑郁", "😭", "失落", "糟糕", "唉"},
	"angry":   {"生气", "愤怒", "气死", "烦死", "受不了", "滚", "操蛋", "可恶", "火大"},
	"anxious": {"焦虑", "紧张", "担心", "害怕", "恐惧", "慌", "不安", "压力", "失眠"},
	"calm":    {"平静", "放松", "安心", "舒适", "冥想", "佛系", "无所谓"},
}

// sentimentWords 是情感词（正面/负面）
var sentimentWords = map[string]float64{
	"好": 0.5, "棒": 0.5, "喜欢": 0.6, "感谢": 0.7, "谢谢": 0.5,
	"开心": 0.7, "高兴": 0.7, "快乐": 0.8, "好极了": 0.9,
	"难": -0.4, "糟": -0.5, "烂": -0.6, "糟糕": -0.7, "气": -0.5,
	"死": -0.6, "害怕": -0.5, "担心": -0.3, "压力": -0.4,
}

func (a *KeywordAnalyzer) Analyze(ctx context.Context, text string) (*EmotionResult, error) {
	if text == "" {
		return &EmotionResult{
			PrimaryEmotion: "neutral",
			SentimentScore: 0,
			Confidence:     0,
			Model:          "keyword-stub",
		}, nil
	}

	// 1. 计算 sentiment_score：遍历情感词，求平均
	scoreSum := 0.0
	hits := 0
	for word, weight := range sentimentWords {
		if strings.Contains(text, word) {
			scoreSum += weight
			hits++
		}
	}
	sentiment := 0.0
	if hits > 0 {
		sentiment = scoreSum / float64(hits)
	}

	// 2. 计算 primary_emotion：找命中关键词最多的类别
	maxHits := 0
	primary := "neutral"
	for emotion, kws := range emotionKeywords {
		cnt := 0
		for _, kw := range kws {
			if strings.Contains(text, kw) {
				cnt++
			}
		}
		if cnt > maxHits {
			maxHits = cnt
			primary = emotion
		}
	}

	// 3. confidence：命中关键词数 / 总词数（粗略）
	totalWords := len([]rune(text)) / 2 // 中文按 2 字/词估算
	confidence := 0.0
	if totalWords > 0 {
		confidence = float64(maxHits) / float64(totalWords)
		if confidence > 1 {
			confidence = 1
		}
	}

	return &EmotionResult{
		PrimaryEmotion: primary,
		SentimentScore: sentiment,
		Confidence:     confidence,
		Model:          "keyword-stub-v1",
	}, nil
}