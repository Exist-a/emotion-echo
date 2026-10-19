// internal/memory/summary_test.go
//
// CL-08a · A3：摘要构建契约（纯函数）。

package memory

import (
	"strings"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/model"
)

func sumFact(id int64, text string, validFrom time.Time, status string) model.MemoryFact {
	return model.MemoryFact{
		ID:        id,
		UserID:    1,
		Subject:   "用户",
		Predicate: "p",
		Object:    "o",
		FactText:  text,
		ValidFrom: validFrom,
		Status:    status,
	}
}

var (
	tSumOld = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	tSumNew = time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
)

func TestBuildSummary_Empty(t *testing.T) {
	t.Parallel()
	if got := BuildSummary(nil, 600); got != "" {
		t.Fatalf("无事实 ⇒ 空摘要，got %q", got)
	}
}

func TestBuildSummary_OrdersRecentFirst(t *testing.T) {
	t.Parallel()
	got := BuildSummary([]model.MemoryFact{
		sumFact(1, "用户住在北京", tSumOld, model.MemoryFactStatusActive),
		sumFact(2, "用户是后端工程师", tSumNew, model.MemoryFactStatusActive),
	}, 600)

	lines := strings.Split(got, "\n")
	if len(lines) != 2 {
		t.Fatalf("期望 2 行，got %d: %q", len(lines), got)
	}
	if !strings.Contains(lines[0], "后端工程师") {
		t.Errorf("最近的事实必须排在最前，got 首行 %q", lines[0])
	}
	if !strings.HasPrefix(lines[0], "- 2026-10 ") {
		t.Errorf("行格式应为 `- YYYY-MM <事实>`，got %q", lines[0])
	}
}

func TestBuildSummary_SkipsInactive(t *testing.T) {
	t.Parallel()
	got := BuildSummary([]model.MemoryFact{
		sumFact(1, "旧事实", tSumOld, model.MemoryFactStatusSuperseded),
		sumFact(2, "删掉的事实", tSumOld, model.MemoryFactStatusDeleted),
		sumFact(3, "有效事实", tSumNew, model.MemoryFactStatusActive),
	}, 600)

	if strings.Contains(got, "旧事实") || strings.Contains(got, "删掉的事实") {
		t.Errorf("superseded/deleted 不得进摘要：%q", got)
	}
	if !strings.Contains(got, "有效事实") {
		t.Errorf("active 事实必须出现：%q", got)
	}
}

func TestBuildSummary_BudgetDropsOldest(t *testing.T) {
	t.Parallel()
	facts := []model.MemoryFact{
		sumFact(1, "很旧的记忆内容", tSumOld, model.MemoryFactStatusActive),
		sumFact(2, "较新的记忆内容", tSumNew, model.MemoryFactStatusActive),
	}
	// 只够放一行（每行中文约 12 token）
	got := BuildSummary(facts, 14)

	lines := strings.Split(got, "\n")
	if len(lines) != 1 {
		t.Fatalf("预算只够一行，got %d 行: %q", len(lines), got)
	}
	if !strings.Contains(lines[0], "较新的记忆内容") {
		t.Errorf("超预算应从最旧的行开始丢，got %q", got)
	}
}

func TestBuildSummary_SanitizesNewlines(t *testing.T) {
	t.Parallel()
	got := BuildSummary([]model.MemoryFact{
		sumFact(1, "用户喜欢\n\n跑步\t和游泳", tSumNew, model.MemoryFactStatusActive),
	}, 600)

	if strings.Count(got, "\n") != 0 {
		t.Errorf("事实文本里的换行必须被压平（摘要按行解析），got %q", got)
	}
	if !strings.Contains(got, "用户喜欢 跑步 和游泳") {
		t.Errorf("连续空白应归一化，got %q", got)
	}
}

func TestBuildSummary_FallsBackToTriple(t *testing.T) {
	t.Parallel()
	f := model.MemoryFact{
		ID: 1, UserID: 1, Subject: "用户", Predicate: "职业", Object: "工程师",
		ValidFrom: tSumNew, Status: model.MemoryFactStatusActive,
	}
	got := BuildSummary([]model.MemoryFact{f}, 600)
	if !strings.Contains(got, "用户职业工程师") {
		t.Errorf("FactText 为空时应回落到三元组，got %q", got)
	}
}

func TestEstimateTokens(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 1},
		{"abcd", 1},
		{"abcde", 2},
		{"中文", 2},
	}
	for _, c := range cases {
		if got := estimateTokens(c.in); got != c.want {
			t.Errorf("estimateTokens(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
