// Package grpcserver — agent_server_convert_test.go
//
// E2E-31（2026-10-08/09，内部 RPC 收敛）：**服务端转换函数**的直接单测。
//
// 为什么单独补这一组（2026-10-09 第二方核对指出的证据归属问题）：
//   BFF 侧的 `assessment_grpc_test.go` 用 bufconn 起的是**手写 stub server**
//   （返回硬编码的 `Key:"q1"/"q2"`），只验证「BFF 转换层 + wire」，**不经过本包的
//   `toProtoSurvey` / `toProtoOptionItems` / `sortedQuestionKeys`**。
//   于是本阶段改动最大的那块（服务端把 JSONB 转成 proto）此前**没有直接单测**，
//   只靠运行时实测 + 人工读码支撑 ⇒ 补此文件把"有序 / 键保真 / 结构化选项"钉死。

package grpcserver

import (
	"testing"

	"emotion-echo-assessment-svc/internal/types"
)

func TestSortedQuestionKeys_NumericAscending(t *testing.T) {
	// map 遍历本就无序；这里用 q10/q2/q1 验证是按**尾部数字**而非字典序
	raw := map[string]any{"q10": 1, "q2": 1, "q1": 1}
	got := sortedQuestionKeys(raw)
	want := []string{"q1", "q2", "q10"}
	if len(got) != len(want) {
		t.Fatalf("len=%d want=%d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sortedQuestionKeys=%v want=%v（字典序会把 q10 排在 q2 前）", got, want)
		}
	}
}

func TestTrailingNumber(t *testing.T) {
	cases := map[string]int{"q1": 1, "q12": 12, "q": 0, "": 0, "abc007": 7}
	for in, want := range cases {
		if got := trailingNumber(in); got != want {
			t.Errorf("trailingNumber(%q)=%d want=%d", in, got, want)
		}
	}
}

// TestToProtoSurvey_OrderedKeysTitleAndStructuredOptions —— E2E-31 测点 #1/#4 的服务端侧
func TestToProtoSurvey_OrderedKeysTitleAndStructuredOptions(t *testing.T) {
	// 形状与 DB JSONB 一致（`psql` 实读样本）：题干键名为 title、options 是对象数组
	resp := &types.GetSurveyResp{
		ID: 1, Code: "PHQ-9", Title: "PHQ-9 抑郁症筛查量表",
		Description: "过去两周内，以下问题困扰你的频率是多少？",
		Questions: map[string]any{
			"q3": map[string]any{"type": "radio", "title": "第三题", "options": []any{}},
			"q1": map[string]any{"type": "radio", "title": "做事时提不起劲或没有兴趣", "options": []any{
				map[string]any{"id": float64(1), "text": "完全没有", "score": float64(0)},
				map[string]any{"id": float64(4), "text": "几乎每天", "score": float64(3)},
			}},
			"q2": map[string]any{"type": "radio", "title": "感到心情低落", "options": []any{}},
		},
	}

	got := toProtoSurvey(resp)
	if got == nil {
		t.Fatal("toProtoSurvey 返回 nil")
	}
	if got.GetDescription() != "过去两周内，以下问题困扰你的频率是多少？" {
		t.Fatalf("description 未透传: %q", got.GetDescription())
	}
	qs := got.GetQuestions()
	if len(qs) != 3 {
		t.Fatalf("questions 数=%d want=3", len(qs))
	}
	// 有序 + 键名保真（不是按下标重编的 q0/q1）
	if qs[0].GetKey() != "q1" || qs[1].GetKey() != "q2" || qs[2].GetKey() != "q3" {
		t.Fatalf("顺序/键名错: %q %q %q", qs[0].GetKey(), qs[1].GetKey(), qs[2].GetKey())
	}
	// 题干取自 JSONB 的 title（旧实现把键名塞进 prompt）
	if qs[0].GetTitle() != "做事时提不起劲或没有兴趣" {
		t.Fatalf("title 错: %q", qs[0].GetTitle())
	}
	if qs[0].GetOrder() != 1 || qs[2].GetOrder() != 3 {
		t.Fatalf("order 错: %d %d", qs[0].GetOrder(), qs[2].GetOrder())
	}
	// 结构化选项：id/text/score 三项（旧实现只认 []string，这里会整块丢）
	opts := qs[0].GetOptionItems()
	if len(opts) != 2 {
		t.Fatalf("option_items 数=%d want=2（旧实现此处为 0）", len(opts))
	}
	if opts[0].GetId() != 1 || opts[0].GetText() != "完全没有" || opts[0].GetScore() != 0 {
		t.Fatalf("选项[0] 错: %+v", opts[0])
	}
	if opts[1].GetScore() != 3 || opts[1].GetText() != "几乎每天" {
		t.Fatalf("选项[1] 错: %+v", opts[1])
	}
	if qs[0].GetQuestionType() != "radio" {
		t.Fatalf("question_type 错: %q", qs[0].GetQuestionType())
	}
}

// TestToProtoAnswers_KeyPreserved —— E2E-31 测点 #3 的服务端侧（结果详情）
func TestToProtoAnswers_KeyPreserved(t *testing.T) {
	got := toProtoAnswers(map[string]any{"q1": float64(3), "q2": 0, "q10": int64(1)})
	if len(got) != 3 {
		t.Fatalf("len=%d want=3 (%v)", len(got), got)
	}
	if got["q1"] != 3 {
		t.Fatalf("q1=%d want=3", got["q1"])
	}
	if _, ok := got["1"]; ok {
		t.Fatal("出现数字键 \"1\"：键被数值化（E2E-13 根因复发）")
	}
	if _, ok := got["q10"]; !ok {
		t.Fatal("q10 丢失")
	}
}

// TestToProtoSurveyResultItem_CarriesScoreSemantics —— E2E-31 测点 #5 的服务端侧
func TestToProtoSurveyResultItem_CarriesScoreSemantics(t *testing.T) {
	got := toProtoSurveyResultItem(types.SurveyResultItem{
		ResultID: 9, SurveyID: 1, TotalScore: 12.5, RiskLevel: "dimension_profile",
		SubmittedAt: 1791525238171, FactorScores: map[string]float64{"O": 12.5}, ScoreKind: "dimension_sum",
	})
	if got.GetTotalScore() != 12.5 {
		t.Fatalf("total_score 小数丢失: %v", got.GetTotalScore())
	}
	if got.GetFactorScores()["O"] != 12.5 {
		t.Fatalf("factor_scores 丢失: %v", got.GetFactorScores())
	}
	if got.GetScoreKind() != "dimension_sum" {
		t.Fatalf("score_kind 丢失: %q", got.GetScoreKind())
	}
	if got.GetSubmittedAt() != 1791525238171 {
		t.Fatalf("submitted_at 丢失: %d", got.GetSubmittedAt())
	}
}
