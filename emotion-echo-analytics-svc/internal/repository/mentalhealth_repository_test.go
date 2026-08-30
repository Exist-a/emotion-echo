// Package repository — mentalhealth_repository_test.go
//
// Stage 30-A SQL 落地 Part 4 RED: riskFromScore 纯函数单测
// （MentalHealthRepo 真 SQL 落地时由 Go 侧从 overall_score 推导 RiskLevel）。
package repository

import "testing"

func TestRiskFromScore_Thresholds(t *testing.T) {
	tests := []struct {
		name  string
		score float64
		want  string
	}{
		{"zero", 0, "low"},
		{"below low boundary", 39.9, "low"},
		{"low boundary", 40, "moderate"},
		{"mid moderate", 59.9, "moderate"},
		{"moderate boundary", 60, "high"},
		{"mid high", 79.9, "high"},
		{"high boundary", 80, "severe"},
		{"max", 100, "severe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := riskFromScore(tt.score); got != tt.want {
				t.Fatalf("riskFromScore(%v) = %q, want %q", tt.score, got, tt.want)
			}
		})
	}
}
