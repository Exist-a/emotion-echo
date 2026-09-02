// Package handler — summary_test.go
//
// fix/chart-contract-alignment: 锁住 summary.go 纯函数的契约。
// 这些是 dashboard 显示中文摘要文本的唯一来源；任何模板改动
// 必须先更新对应测试。
package handler

import (
	"strings"
	"testing"

	"emotion-echo-web-bff/internal/downstream"
)

func TestBuildDaily_NilReport_ReturnsPlaceholder(t *testing.T) {
	got := BuildDaily(nil)
	if !strings.Contains(got, "今天还没有对话记录") {
		t.Fatalf("nil 入参应返回占位句，实际：%q", got)
	}
}

func TestBuildDaily_HappyDominant_PositiveSentiment(t *testing.T) {
	r := &downstream.DailyReport{
		Date:              "2026-08-31",
		ConversationCount: 2,
		MessageCount:      4,
		EmotionCounts:     map[string]int64{"happy": 3, "sad": 1},
		AvgSentiment:      0.4,
	}
	got := BuildDaily(r)
	if !strings.Contains(got, "2026-08-31") {
		t.Errorf("必须包含日期，实际：%q", got)
	}
	if !strings.Contains(got, "2 段对话") {
		t.Errorf("必须包含会话数 2，实际：%q", got)
	}
	if !strings.Contains(got, "4 条消息") {
		t.Errorf("必须包含消息数 4，实际：%q", got)
	}
	if !strings.Contains(got, "开心") {
		t.Errorf("happy 应翻译为'开心'，实际：%q", got)
	}
	if !strings.Contains(got, "3 次") {
		t.Errorf("top emotion count 应出现，实际：%q", got)
	}
	if !strings.Contains(got, "积极") {
		t.Errorf("avgSentiment=0.4 应定性'积极'，实际：%q", got)
	}
}

func TestBuildDaily_EmptyCounts_DegradesClean(t *testing.T) {
	r := &downstream.DailyReport{
		Date:              "2026-08-31",
		ConversationCount: 1,
		MessageCount:      0,
		AvgSentiment:      0.0,
	}
	got := BuildDaily(r)
	if strings.Contains(got, "主要情绪") {
		t.Errorf("空 counts 不应输出'主要情绪'段，实际：%q", got)
	}
	if !strings.Contains(got, "平稳") {
		t.Errorf("avgSentiment=0.0 应定性'平稳'，实际：%q", got)
	}
}

func TestBuildDaily_NegativeSentiment_UsesMoodWord(t *testing.T) {
	r := &downstream.DailyReport{
		Date:              "2026-08-31",
		ConversationCount: 1,
		MessageCount:      1,
		EmotionCounts:     map[string]int64{"sad": 1},
		AvgSentiment:      -0.2,
	}
	got := BuildDaily(r)
	if !strings.Contains(got, "低落") {
		t.Errorf("avgSentiment=-0.2 应定性'低落'，实际：%q", got)
	}
}

func TestMoodWord_ThresholdBoundaries(t *testing.T) {
	cases := []struct {
		avg  float64
		want string
	}{
		{-0.5, "低落"},
		{-0.01, "低落"}, // 任何 < 0 都低落
		{0.0, "平稳"},   // 边界：正好 0 不算积极
		{0.29, "平稳"}, // 边界：差一点 0.3
		{0.3, "积极"},   // 边界：正好 0.3 算积极
		{0.5, "积极"},
		{1.0, "积极"},
	}
	for _, c := range cases {
		got := moodWord(c.avg)
		if got != c.want {
			t.Errorf("moodWord(%v) = %q, want %q", c.avg, got, c.want)
		}
	}
}

func TestPickTopEmotion_StableSort(t *testing.T) {
	// 并列时按 name 字母序，确保测试断言稳定
	m := map[string]int64{"happy": 2, "sad": 2, "anxious": 1}
	name, cnt, empty := pickTopEmotion(m)
	if empty {
		t.Fatal("非空 map 不应 empty")
	}
	if name != "happy" || cnt != 2 {
		// happy 比 sad 字母靠前，所以并列时 happy 胜
		t.Errorf("并列按字母序应选 happy，实际 name=%q cnt=%d", name, cnt)
	}
}

func TestPickTopEmotion_EmptyMap(t *testing.T) {
	_, _, empty := pickTopEmotion(map[string]int64{})
	if !empty {
		t.Error("空 map 应返回 empty=true")
	}
	_, _, empty = pickTopEmotion(nil)
	if !empty {
		t.Error("nil map 应返回 empty=true")
	}
}

func TestTranslateEmotion_KnownAndUnknown(t *testing.T) {
	if translateEmotion("happy") != "开心" {
		t.Error("happy 应翻译为'开心'")
	}
	if translateEmotion("custom_unknown_label") != "custom_unknown_label" {
		t.Error("未知 emotion 应原样输出，不硬塞'未知'")
	}
}

func TestBuildTrend_EmptyPoints_ReturnsPlaceholder(t *testing.T) {
	r := &downstream.TrendReport{StartDate: "2026-08-25", EndDate: "2026-08-31"}
	got := BuildTrend(r)
	if !strings.Contains(got, "区间内暂无对话记录") {
		t.Errorf("空 Points 应返回占位句，实际：%q", got)
	}
}

func TestBuildTrend_MultiEmotion_AggregatesCorrectly(t *testing.T) {
	r := &downstream.TrendReport{
		StartDate: "2026-08-25",
		EndDate:   "2026-08-31",
		Points: []downstream.TrendPoint{
			{Date: "2026-08-25", PrimaryEmotion: "happy", AvgSentiment: 0.5, Count: 3},
			{Date: "2026-08-26", PrimaryEmotion: "happy", AvgSentiment: 0.3, Count: 2},
			{Date: "2026-08-27", PrimaryEmotion: "sad", AvgSentiment: -0.2, Count: 1},
		},
	}
	got := BuildTrend(r)
	// 必须包含：日期区间、情绪总数、主导情绪中文、定性词
	if !strings.Contains(got, "2026-08-25") || !strings.Contains(got, "2026-08-31") {
		t.Errorf("必须包含起止日期，实际：%q", got)
	}
	if !strings.Contains(got, "6 条情绪记录") {
		t.Errorf("总 count 应为 3+2+1=6，实际：%q", got)
	}
	if !strings.Contains(got, "开心") {
		t.Errorf("happy 总和最大应是'开心'，实际：%q", got)
	}
	// avgSentiment = (0.5+0.3-0.2)/3 ≈ 0.2 → 平稳
	if !strings.Contains(got, "平稳") {
		t.Errorf("avg=0.2 应定性'平稳'，实际：%q", got)
	}
}

func TestMonthEndDay(t *testing.T) {
	cases := []struct {
		month string
		want  string
	}{
		{"2026-01", "2026-01-31"},
		{"2026-02", "2026-02-28"}, // 非闰年
		{"2024-02", "2024-02-29"}, // 闰年
		{"2026-04", "2026-04-30"},
		{"2026-09", "2026-09-30"},
		{"2026-12", "2026-12-31"},
		{"bad", "2006-01-31"}, // fallback
	}
	for _, c := range cases {
		got := monthEndDay(c.month)
		if got != c.want {
			t.Errorf("monthEndDay(%q) = %q, want %q", c.month, got, c.want)
		}
	}
}
