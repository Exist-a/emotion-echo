// Package logic — reports_daily_logic_fused_test.go
//
// CL-07（后端切片）Phase 2 · RED：日报 logic 必须装配 FusedReportRepo 的结果。
//
// 契约：与 EmotionDistributionByModality 同款——repo 存在则装配，nil 则跳过（不报错），
// repo 出错则冒泡（不吞错）。

package logic

import (
	"context"
	"errors"
	"testing"
	"time"

	"emotion-echo-analytics-svc/internal/config"
	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/svc"
	"emotion-echo-analytics-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeFusedReportRepo 满足 repository.FusedReportRepo。
type fakeFusedReportRepo struct {
	repository.FusedReportRepo // embedded for forward-compat

	getDailyFusedEmotion    *repository.FusedEmotionDistribution
	getDailyFusedEmotionErr error
}

func (f *fakeFusedReportRepo) GetDailyFusedEmotion(_ context.Context, _ int64, _ time.Time) (*repository.FusedEmotionDistribution, error) {
	return f.getDailyFusedEmotion, f.getDailyFusedEmotionErr
}

func (f *fakeFusedReportRepo) Ping(_ context.Context) error { return nil }

// TestReportsDailyLogic_PopulatesFusedEmotionDistribution repo 有值 → 字段被装配。
func TestReportsDailyLogic_PopulatesFusedEmotionDistribution(t *testing.T) {
	t.Parallel()
	want := &repository.FusedEmotionDistribution{
		ByEmotion:  map[string]int64{"happy": 5, "sad": 2},
		ByMethod:   map[string]int64{"llm": 4, "late_fusion_weighted": 3},
		FaceRatio:  0.5,
		VoiceRatio: 0.25,
		Total:      7,
	}
	ctx := &svc.ServiceContext{
		Config:          config.Config{},
		ReportRepo:      &fakeReportRepo{getDailyReport: &repository.DailyReport{UserID: 42, Date: "2026-10-16"}},
		FusedReportRepo: &fakeFusedReportRepo{getDailyFusedEmotion: want},
	}
	l := NewReportsDailyLogic(context.Background(), ctx)

	resp, err := l.GetDailyReport(&types.GetDailyReportReq{UserID: 42, Date: "2026-10-16"})
	require.NoError(t, err)
	require.NotNil(t, resp.Report)
	require.NotNil(t, resp.Report.FusedEmotionDistribution,
		"logic 必须把 FusedReportRepo 结果装配进 DailyReport.FusedEmotionDistribution")
	assert.Equal(t, int64(5), resp.Report.FusedEmotionDistribution.ByEmotion["happy"])
	assert.Equal(t, int64(4), resp.Report.FusedEmotionDistribution.ByMethod["llm"])
	assert.Equal(t, int64(7), resp.Report.FusedEmotionDistribution.Total)
	assert.InDelta(t, 0.5, resp.Report.FusedEmotionDistribution.FaceRatio, 1e-9)
}

// TestReportsDailyLogic_FusedNil_SkipsWithoutError repo=nil → 跳过，字段留 nil，不报错。
func TestReportsDailyLogic_FusedNil_SkipsWithoutError(t *testing.T) {
	t.Parallel()
	ctx := &svc.ServiceContext{
		Config:     config.Config{},
		ReportRepo: &fakeReportRepo{getDailyReport: &repository.DailyReport{UserID: 1, Date: "2026-10-16"}},
		// FusedReportRepo 故意 nil（模拟未接线/无融合能力）
	}
	l := NewReportsDailyLogic(context.Background(), ctx)

	resp, err := l.GetDailyReport(&types.GetDailyReportReq{UserID: 1, Date: "2026-10-16"})
	require.NoError(t, err)
	require.NotNil(t, resp.Report)
	assert.Nil(t, resp.Report.FusedEmotionDistribution)
}

// TestReportsDailyLogic_FusedRepoError_Propagates repo 报错 → 冒泡。
func TestReportsDailyLogic_FusedRepoError_Propagates(t *testing.T) {
	t.Parallel()
	boom := errors.New("fused repo boom")
	ctx := &svc.ServiceContext{
		Config:          config.Config{},
		ReportRepo:      &fakeReportRepo{getDailyReport: &repository.DailyReport{UserID: 1, Date: "2026-10-16"}},
		FusedReportRepo: &fakeFusedReportRepo{getDailyFusedEmotionErr: boom},
	}
	l := NewReportsDailyLogic(context.Background(), ctx)

	_, err := l.GetDailyReport(&types.GetDailyReportReq{UserID: 1, Date: "2026-10-16"})
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}
