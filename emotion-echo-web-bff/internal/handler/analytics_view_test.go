// Package handler — analytics_view_test.go
//
// E2E-11 复查：补齐 3 个前端契约变换函数的**直接单测**。
//
// 此前这些函数只有 handler 级（HTTP 往返）测试，null/空态分支从未被覆盖：
//   - toFrontendDayNight(nil) / 全零 pattern
//   - toFrontendFrequency(nil)
//   - toFrontendDepth(nil)
// 这些分支正是「图表空态可读」测试点依赖的行为，且空态一旦回归只会表现为
// 前端白屏，HTTP 状态码仍是 200，靠端到端很难抓。
package handler

import (
	"testing"

	"emotion-echo-web-bff/internal/downstream"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============ toFrontendDayNight ============

func TestToFrontendDayNight_AllZeroBuckets_ReturnsEmptySlice(t *testing.T) {
	// 24 桶全 0（有事件表但窗口内无数据）：必须返回空数组而非 nil，
	// 否则前端 chartData 判断会拿到 null 导致渲染异常。
	out := toFrontendDayNight(map[int]int64{0: 0, 1: 0, 23: 0})
	require.NotNil(t, out)
	assert.NotNil(t, out.Periods, "periods 必须是空数组 []，不能是 null")
	assert.Len(t, out.Periods, 0)
}

func TestToFrontendDayNight_NilMap_ReturnsEmptySlice(t *testing.T) {
	out := toFrontendDayNight(nil)
	require.NotNil(t, out)
	assert.NotNil(t, out.Periods)
	assert.Len(t, out.Periods, 0)
}

func TestToFrontendDayNight_AggregatesIntoFourSlots(t *testing.T) {
	// 每个时段各放 1 条，验证 4 段边界不重叠（0-5/6-11/12-17/18-23）
	pattern := map[int]int64{0: 1, 5: 1, 6: 1, 11: 1, 12: 1, 17: 1, 18: 1, 23: 1}
	out := toFrontendDayNight(pattern)
	require.Len(t, out.Periods, 4, "4 个时段各应有数据")
	assert.Equal(t, "凌晨", out.Periods[0].Label)
	assert.Equal(t, "上午", out.Periods[1].Label)
	assert.Equal(t, "下午", out.Periods[2].Label)
	assert.Equal(t, "夜间", out.Periods[3].Label)
	for _, p := range out.Periods {
		assert.Equal(t, int64(2), p.Value, "%s 应聚合 2 条（边界各 1 条）", p.Label)
	}
}

func TestToFrontendDayNight_SkipsZeroSlots(t *testing.T) {
	// 只有上午有数据 ⇒ 只输出 1 段（避免饼图出现 0 值扇区）
	out := toFrontendDayNight(map[int]int64{9: 5})
	require.Len(t, out.Periods, 1)
	assert.Equal(t, "上午", out.Periods[0].Label)
	assert.Equal(t, int64(5), out.Periods[0].Value)
}

// ============ toFrontendFrequency ============

func TestToFrontendFrequency_Empty_ReturnsEmptySlices(t *testing.T) {
	out := toFrontendFrequency(nil)
	require.NotNil(t, out)
	assert.NotNil(t, out.Dates, "dates 必须是 []，不能是 null")
	assert.NotNil(t, out.MessageCount, "messageCount 必须是 []，不能是 null")
	assert.Len(t, out.Dates, 0)
}

func TestToFrontendFrequency_ParallelArraysAligned(t *testing.T) {
	out := toFrontendFrequency([]downstream.DailyCount{
		{Date: "2026-09-17", Count: 5},
		{Date: "2026-09-18", Count: 0}, // 零值日也应保留（折线图不断线）
		{Date: "2026-09-19", Count: 3},
	})
	assert.Equal(t, []string{"2026-09-17", "2026-09-18", "2026-09-19"}, out.Dates)
	assert.Equal(t, []int64{5, 0, 3}, out.MessageCount)
	assert.Equal(t, len(out.Dates), len(out.MessageCount), "两个平行数组长度必须相等")
}

// ============ toFrontendDepth ============

func TestToFrontendDepth_Nil_ReturnsZeroValue(t *testing.T) {
	out := toFrontendDepth(nil, 0, 0)
	require.NotNil(t, out)
	assert.Equal(t, int64(0), out.TotalMessages)
	assert.Equal(t, float64(0), out.AvgMessagesPerDay)
}

func TestToFrontendDepth_NoActiveDays_FallsBackToConversations(t *testing.T) {
	// frequency 取不到（activeDays=0）时，日均退化用会话数近似，不得除零
	out := toFrontendDepth(&downstream.InteractionDepth{
		TotalMessages:      100,
		TotalConversations: 4,
	}, 0, 0)
	assert.InDelta(t, 25.0, out.AvgMessagesPerDay, 0.01)
}

func TestToFrontendDepth_ZeroConversationsAndDays_NoDivideByZero(t *testing.T) {
	out := toFrontendDepth(&downstream.InteractionDepth{TotalMessages: 7}, 0, 0)
	assert.Equal(t, float64(0), out.AvgMessagesPerDay, "无分母时必须返 0 而非 NaN/Inf")
}

func TestToFrontendDepth_StreakAndDaysPassthrough(t *testing.T) {
	out := toFrontendDepth(&downstream.InteractionDepth{
		TotalMessages:      150,
		TotalConversations: 12,
		AvgMessagesPerConv: 12.5,
	}, 4, 6)
	assert.Equal(t, int64(4), out.MaxConsecutiveDays)
	assert.InDelta(t, 25.0, out.AvgMessagesPerDay, 0.01)
	assert.InDelta(t, 12.5, out.AvgSessionRounds, 0.01)
}

// ============ toFrontendTrendReport ============
//
// 2026-09-21 修复（用户实测反馈「0 段对话 33 条消息」+ 周/月/年报会话数恒 0）：
//   - 原 conversationCount 硬编码 0（注释称 "TrendReport 没有 conv 维度"）
//   - 原 messageCount 从 points 累计（= 情绪记录数，不是消息数）
// 现直传 downstream 的区间真实计数（与 DailyReport 同源 msg_summary_v）。

func TestToFrontendTrendReport_PassesThroughRealCounts(t *testing.T) {
	out := toFrontendTrendReport(&downstream.TrendReport{
		Type:              "weekly",
		StartDate:         "2026-09-15",
		EndDate:           "2026-09-21",
		MessageCount:      33,
		ConversationCount: 16,
		Points: []downstream.TrendPoint{
			{Date: "2026-09-15", Count: 5, PrimaryEmotion: "happy"},
		},
	})
	require.NotNil(t, out)
	assert.Equal(t, int64(33), out.MessageCount,
		"E2E-15 FU: messageCount 应为 msg_summary_v 真值（33），而非 points 情绪记录累计")
	assert.Equal(t, int64(16), out.ConversationCount,
		"E2E-15 FU: conversationCount 应透传真值（16），而非硬编码 0")
}

func TestToFrontendTrendReport_FallsBackToPointsSum_WhenCountsMissing(t *testing.T) {
	// 兼容：下游未提供区间计数（旧版本 / 字段缺失 → 0）时回落 points 累计（保持旧行为）
	out := toFrontendTrendReport(&downstream.TrendReport{
		Type: "weekly",
		Points: []downstream.TrendPoint{
			{Date: "2026-09-15", Count: 5},
			{Date: "2026-09-22", Count: 3},
		},
	})
	require.NotNil(t, out)
	assert.Equal(t, int64(8), out.MessageCount, "缺区间计数时回落 points 累计")
	assert.Equal(t, int64(0), out.ConversationCount, "缺区间计数时 conv 为 0（保持旧行为）")
}
