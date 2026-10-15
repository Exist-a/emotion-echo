// Package handler — analytics_view_fused_test.go
//
// CL-07（后端切片）Phase 3 · RED：日报前端契约必须透传融合产物（fused）。
//
// 契约：downstream.DailyReport.FusedEmotionDistribution → FrontendDailyReport.FusedEmotion
// （分布按 value 倒序；无融合数据时字段 nil = 前端按缺失隐藏）。

package handler

import (
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToFrontendDailyReport_FusedEmotion(t *testing.T) {
	in := &downstream.DailyReport{
		Date:          "2026-10-16",
		EmotionCounts: map[string]int64{"happy": 3},
		FusedEmotionDistribution: &downstream.FusedEmotionDistribution{
			ByEmotion:  map[string]int64{"happy": 5, "sad": 2},
			ByMethod:   map[string]int64{"llm": 4, "late_fusion_weighted": 3},
			FaceRatio:  0.5,
			VoiceRatio: 0.25,
			Total:      7,
		},
	}
	out := toFrontendDailyReport(in)
	require.NotNil(t, out.FusedEmotion, "fused 分布必须透传到前端契约（否则又是「算了没人用」）")

	assert.Equal(t, int64(7), out.FusedEmotion.Total)
	require.Len(t, out.FusedEmotion.EmotionDistribution, 2, "两个情绪各一扇区")
	assert.Equal(t, "happy", out.FusedEmotion.EmotionDistribution[0].Name, "按 value 倒序：happy(5) 在前")
	assert.Equal(t, int64(5), out.FusedEmotion.EmotionDistribution[0].Value)
	assert.Equal(t, "sad", out.FusedEmotion.EmotionDistribution[1].Name)

	byMethod := map[string]int64{}
	for _, m := range out.FusedEmotion.MethodBreakdown {
		byMethod[m.Method] = m.Count
	}
	assert.Equal(t, int64(4), byMethod["llm"])
	assert.Equal(t, int64(3), byMethod["late_fusion_weighted"])

	assert.InDelta(t, 0.5, out.FusedEmotion.FaceRatio, 1e-9)
	assert.InDelta(t, 0.25, out.FusedEmotion.VoiceRatio, 1e-9)
}

func TestToFrontendDailyReport_NoFused_OmitsField(t *testing.T) {
	out := toFrontendDailyReport(&downstream.DailyReport{
		Date:          "2026-10-16",
		EmotionCounts: map[string]int64{},
	})
	assert.Nil(t, out.FusedEmotion, "无融合数据 → 字段缺失（omitempty），前端隐藏图")
}

func TestToFrontendDailyReport_NilReport_FusedNil(t *testing.T) {
	out := toFrontendDailyReport(nil)
	require.NotNil(t, out)
	assert.Nil(t, out.FusedEmotion)
}
