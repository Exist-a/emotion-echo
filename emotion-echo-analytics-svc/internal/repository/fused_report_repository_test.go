// Package repository — CL-07（后端切片）Phase 2 · RED
//
// FusedReportRepo 提供"融合产物（fused_emotions）"的日聚合消费，
// 数据源是 ai-svc 的 emotion_echo_ai.daily_emotion_fused_v 视图（CL-07 migration i010）。
//
// 与 ModalityReportRepo 并存（不同维度）：
//   - ModalityReportRepo：按**单模态**（text/face/voice）拆
//   - FusedReportRepo：**收敛后**的综合情绪 + 融合方法占比（不双计单模态）
//
// RED 阶段：FusedEmotionDistribution / FusedReportRepo / InMemoryFusedReportRepo
// 定义在 fused_report_repository.go（GREEN）⇒ 本测试此时必须编译失败。

package repository

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func day(d int) time.Time { return time.Date(2026, 10, d, 0, 0, 0, 0, time.UTC) }

// TestFusedReportRepo_InMemory_EmptyRepo 无数据 → 非 nil + 空 map（前端可直接遍历）。
func TestFusedReportRepo_InMemory_EmptyRepo(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryFusedReportRepo()
	got, err := repo.GetDailyFusedEmotion(context.Background(), 7, day(1))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Empty(t, got.ByEmotion)
	assert.Empty(t, got.ByMethod)
	assert.Equal(t, int64(0), got.Total)
}

// TestFusedReportRepo_InMemory_PreloadedData 预置数据 → 逐字段正确。
func TestFusedReportRepo_InMemory_PreloadedData(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryFusedReportRepo()
	repo.data[7] = map[time.Time]*FusedEmotionDistribution{
		day(1): {
			ByEmotion:  map[string]int64{"happy": 5, "sad": 2},
			ByMethod:   map[string]int64{"llm": 4, "late_fusion_weighted": 3},
			FaceRatio:  0.5,
			VoiceRatio: 0.25,
			Total:      7,
		},
	}

	got, err := repo.GetDailyFusedEmotion(context.Background(), 7, day(1))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, map[string]int64{"happy": 5, "sad": 2}, got.ByEmotion)
	assert.Equal(t, map[string]int64{"llm": 4, "late_fusion_weighted": 3}, got.ByMethod)
	assert.Equal(t, int64(7), got.Total)
	assert.InDelta(t, 0.5, got.FaceRatio, 1e-9)
	assert.InDelta(t, 0.25, got.VoiceRatio, 1e-9)
}

// TestFusedReportRepo_InMemory_DifferentUser 不同用户互不污染。
func TestFusedReportRepo_InMemory_DifferentUser(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryFusedReportRepo()
	repo.data[7] = map[time.Time]*FusedEmotionDistribution{day(1): {ByEmotion: map[string]int64{"happy": 5}, Total: 5}}
	repo.data[8] = map[time.Time]*FusedEmotionDistribution{day(1): {ByEmotion: map[string]int64{"sad": 1}, Total: 1}}

	g7, _ := repo.GetDailyFusedEmotion(context.Background(), 7, day(1))
	g8, _ := repo.GetDailyFusedEmotion(context.Background(), 8, day(1))
	assert.Equal(t, int64(5), g7.ByEmotion["happy"])
	assert.Equal(t, int64(1), g8.ByEmotion["sad"])
}

// TestFusedReportRepo_InMemory_ErrPropagation 注入错误 → 透传（不吞错）。
func TestFusedReportRepo_InMemory_ErrPropagation(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryFusedReportRepo()
	wantErr := errors.New("boom")
	repo.err = wantErr
	_, err := repo.GetDailyFusedEmotion(context.Background(), 7, day(1))
	assert.ErrorIs(t, err, wantErr)
}

// TestFusedReportRepo_InMemory_Ping 健康检查。
func TestFusedReportRepo_InMemory_Ping(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryFusedReportRepo()
	assert.NoError(t, repo.Ping(context.Background()))
}

// TestFusedReportRepo_InterfaceConformance 编译期断言实现接口。
func TestFusedReportRepo_InterfaceConformance(t *testing.T) {
	t.Parallel()
	var _ FusedReportRepo = (*InMemoryFusedReportRepo)(nil)
	var _ FusedReportRepo = (*PostgresFusedReportRepo)(nil)
}
