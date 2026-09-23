// Package downstream — emotion_history.go
//
// E2E-F-122（D-14 emotionSource 高级模式）：会话情绪历史来源。
//
// 用途：/api/v1/ai/stream 组装 system prompt 时，若前端 payload（face/voice，
// 3 秒实时窗口）为空 —— 摄像头关闭 / 权限被拒 / 窗口过期 —— 回落本来源查
// emotion_analysis 历史，产出"最近情绪模式"注入 prompt，让 AI 仍能感知用户情绪。
//
// 取数：EmotionQueryService.ByConversation（BFF → ai-svc gRPC）。
//
// 判定规则（与 emotion_history_test.go 对应）：
//   - 过滤 neutral（dev 模式 sync-fallback 占位行 confidence=0 无信息量）
//   - 按 created_at_ms 降序（不依赖上游排序，与 personalitySource 同款纪律）
//   - 非 neutral 行 >= 3 且众数占比 > 50% → 众数（"历史情绪模式"）
//   - 其余（1~2 行或无主导众数）→ 最新一行
//   - 全 neutral / 空 / 上游错误 → ""（回落是增强不是依赖，错误不冒泡）
package downstream

import (
	"context"
	"sort"

	emotionquery "github.com/emotion-echo/shared/pkg/emotionquery"
)

// emotionHistoryLimit 拉取上限：情绪历史只需近端窗口做模式统计，50 行足够
// 覆盖"最近一次会话 + 前几次短对话"，同时控制 gRPC 响应体积。
const emotionHistoryLimit = 50

// emotionHistoryModeMinRows 走"众数=模式"路径的最小行数：
// 少于 3 行时统计无意义（1 行 = 最新即模式，2 行可能对半），直接取最新。
const emotionHistoryModeMinRows = 3

// EmotionHistorySource 从 ai-svc 取会话最近情绪历史（E2E-F-122）
type EmotionHistorySource struct {
	query EmotionQueryClient
}

// NewEmotionHistorySource 构造（query 可为 nil → 恒返回空串的 no-op 源）
func NewEmotionHistorySource(query EmotionQueryClient) *EmotionHistorySource {
	return &EmotionHistorySource{query: query}
}

// RecentEmotionPattern 返回会话最近情绪模式（英文标签，调用方负责中文化）。
//
// 返回值：
//   - (pattern, nil) 统计出的主导情绪
//   - ("", nil)      无可用历史 / 上游错误 —— 正常情形，调用方不注入情绪段
func (s *EmotionHistorySource) RecentEmotionPattern(ctx context.Context, conversationID int64) (string, error) {
	if s == nil || s.query == nil || conversationID <= 0 {
		return "", nil
	}

	rows, _, err := s.query.ByConversation(ctx, conversationID, emotionHistoryLimit)
	if err != nil {
		// 回落是增强不是依赖：错误静默降级为空（与 personalitySource 一致）
		return "", nil
	}

	// 过滤 neutral 占位行
	nonNeutral := make([]*emotionquery.Emotion, 0, len(rows))
	for _, r := range rows {
		if r == nil || r.PrimaryEmotion == "" || r.PrimaryEmotion == "neutral" {
			continue
		}
		nonNeutral = append(nonNeutral, r)
	}
	if len(nonNeutral) == 0 {
		return "", nil
	}

	// 按 created_at_ms 降序（不依赖上游排序）
	sort.SliceStable(nonNeutral, func(i, j int) bool {
		return nonNeutral[i].CreatedAtMs > nonNeutral[j].CreatedAtMs
	})

	// >= 3 行且众数占比 > 50% → 众数（历史情绪模式）；否则取最新
	if len(nonNeutral) >= emotionHistoryModeMinRows {
		counts := map[string]int{}
		for _, r := range nonNeutral {
			counts[r.PrimaryEmotion]++
		}
		best, bestN := "", 0
		for emotion, n := range counts {
			if n > bestN {
				best, bestN = emotion, n
			}
		}
		if bestN*2 > len(nonNeutral) {
			return best, nil
		}
	}

	return nonNeutral[0].PrimaryEmotion, nil
}
