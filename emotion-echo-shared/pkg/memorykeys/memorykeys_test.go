// Package memorykeys 的键格式契约。
//
// 这个测试的真正价值在**跨服务**：ai-svc（写）与 web-bff（读）都用同一套键，
// 任何一侧改格式都会先在这里红。

package memorykeys

import (
	"testing"
	"time"
)

func TestSummaryKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		userID int64
		want   string
	}{
		{1, "memory:summary:user:1"},
		{42, "memory:summary:user:42"},
		{0, "memory:summary:user:0"},
	}
	for _, c := range cases {
		if got := SummaryKey(c.userID); got != c.want {
			t.Errorf("SummaryKey(%d) = %q, want %q", c.userID, got, c.want)
		}
	}
}

func TestSummaryTTLIs24h(t *testing.T) {
	t.Parallel()
	if SummaryTTL != 24*time.Hour {
		t.Fatalf("SummaryTTL = %v, want 24h（与 CL-D-07 的用户级热点口径一致）", SummaryTTL)
	}
}
