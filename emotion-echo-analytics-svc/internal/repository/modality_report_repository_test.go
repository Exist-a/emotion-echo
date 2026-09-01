// Package repository — Stage 34 · PR-15 RED
//
// ModalityReportRepo 提供按模态维度的情绪分布（face / voice / text），
// 数据源是 ai-svc 的 emotion_echo_ai.daily_emotion_by_modality_v VIEW（Stage 34 migration 005）。
//
// 设计：
//   - ReportRepo interface（依赖反转）
//   - InMemoryModalityReportRepo 测试替身
//   - PostgresModalityReportRepo 生产实现，Raw SQL 调跨 schema VIEW
//
// 数据形状：
//   - 按 userID × date × emotion × modality 聚合
//   - 返回 ModalityEmotionDistribution{Text, Face, Voice, Fused} 每个是 map[emotion]int64
//   - 与现有 DailyReport.EmotionCounts 单模态字段**并存**（向后兼容）
package repository

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ModalityEmotionDistribution 单日按模态分组的情绪计数。
//
// Text / Face / Voice 三路分别对应 emotion_echo_ai 上的三张表（emotion_analysis /
// face_emotion_results / voice_emotion_results）。Fused 留给未来 Stage 35+（融合结果
// 不与单模态并列聚合，避免双计）。
type ModalityEmotionDistribution struct {
	Text map[string]int64
	Face map[string]int64
	Voice map[string]int64
}

// ModalityReportRepo 跨模态情绪分布仓储接口
type ModalityReportRepo interface {
	// GetDailyEmotionByModality 单日按模态聚合情绪计数
	GetDailyEmotionByModality(ctx context.Context, userID int64, date time.Time) (*ModalityEmotionDistribution, error)
	Ping(ctx context.Context) error
}

// TestModalityReportRepo_InMemory_EmptyRepo 当没有任何数据时，所有 map 为 nil/空。
func TestModalityReportRepo_InMemory_EmptyRepo(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryModalityReportRepo()
	got, err := repo.GetDailyEmotionByModality(context.Background(), 7, time.Now())
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Nil(t, got.Text)
	assert.Nil(t, got.Face)
	assert.Nil(t, got.Voice)
}

// TestModalityReportRepo_InMemory_PreloadedData 预置三路数据 → 分别正确返回。
func TestModalityReportRepo_InMemory_PreloadedData(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryModalityReportRepo()
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	repo.data[7] = map[time.Time]map[string]map[string]int64{
		day: {
			"text":  {"happy": 5, "sad": 2},
			"face":  {"neutral": 3},
			"voice": {"sad": 1, "happy": 1},
		},
	}

	got, err := repo.GetDailyEmotionByModality(context.Background(), 7, day)
	require.NoError(t, err)
	require.NotNil(t, got)

	assert.Equal(t, map[string]int64{"happy": 5, "sad": 2}, got.Text)
	assert.Equal(t, map[string]int64{"neutral": 3}, got.Face)
	assert.Equal(t, map[string]int64{"sad": 1, "happy": 1}, got.Voice)
}

// TestModalityReportRepo_InMemory_DifferentUser 不同用户数据不互相污染。
func TestModalityReportRepo_InMemory_DifferentUser(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryModalityReportRepo()
	day := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)

	repo.data[7] = map[time.Time]map[string]map[string]int64{
		day: {"text": {"happy": 5}},
	}
	repo.data[8] = map[time.Time]map[string]map[string]int64{
		day: {"text": {"sad": 1}},
	}

	got7, _ := repo.GetDailyEmotionByModality(context.Background(), 7, day)
	got8, _ := repo.GetDailyEmotionByModality(context.Background(), 8, day)
	assert.Equal(t, int64(5), got7.Text["happy"])
	assert.Equal(t, int64(1), got8.Text["sad"])
}

// TestModalityReportRepo_InMemory_Ping 健康检查。
func TestModalityReportRepo_InMemory_Ping(t *testing.T) {
	t.Parallel()
	repo := NewInMemoryModalityReportRepo()
	assert.NoError(t, repo.Ping(context.Background()))
}

// TestModalityReportRepo_InterfaceConformance 编译期断言 InMemory 实现接口。
func TestModalityReportRepo_InterfaceConformance(t *testing.T) {
	t.Parallel()
	var _ ModalityReportRepo = (*InMemoryModalityReportRepo)(nil)
}