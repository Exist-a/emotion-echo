// emotion-echo-shared/pkg/emotionassessment/agent_contract_test.go
//
// E2E-31 L1（内部 RPC 收敛）契约测试 —— proto 契约扩展的 RED→GREEN 守卫。
//
// 背景（账本 E2E-F-208 / 用户 2026-10-08 裁定「得使用 grpc」）：
//   BFF→assessment-svc 的 gRPC 通道曾按「决策 4」接线，2026-09-20 因 proto
//   表达能力不足（survey-http-bypass ADR）被 HTTP 旁路绕开，此后零调用。
//   本测试钉住"回归 gRPC 所必需的最小充分 proto 契约"，防止再次退化。
//
// 对应任务书测试点：E2E-31 #1（SurveyOption）/ #2（description）/ #3（answers 键保真）
//                  / #4（题干键名与顺序）/ #5（结果字段）
//
// 为什么写成契约测试（AGENTS §三 可测试性）：
//   这些字段的语义是"跨进程契约"，用字面量断言字段存在 + 序列化往返语义，
//   比依赖真实 gRPC server 更稳定（后者需起 :8886 且掩盖编译期缺口）。

package emotionassessment

import (
	"testing"

	"google.golang.org/protobuf/proto"
)

// TestAgentProto_SurveyOption_Exists —— 测点 #1：选项必须是结构化对象，不是 []string。
//
// 现状（RED）：proto `SurveyQuestion.options` 是 `repeated string`，
// 表达不了 JSONB 里的 `[{id,text,score}]`，前端 `opt.score` 取不到（E2E-13 症状）。
func TestAgentProto_SurveyOption_CarriesIDTextScore(t *testing.T) {
	opt := &SurveyOption{Id: 4, Text: "几乎每天", Score: 3}
	if opt.GetId() != 4 || opt.GetText() != "几乎每天" || opt.GetScore() != 3 {
		t.Fatalf("SurveyOption 三元组不保真: %+v", opt)
	}

	// 挂到题目上（M1 裁定 = 新增 option_items 字段，旧 options 保留 deprecated）
	q := &SurveyQuestion{
		Key:         "q1",
		Title:       "做事时提不起劲或没有兴趣",
		OptionItems: []*SurveyOption{opt},
	}
	if got := q.GetOptionItems(); len(got) != 1 || got[0].GetScore() != 3 {
		t.Fatalf("SurveyQuestion.option_items 未保真: %+v", got)
	}
}

// TestAgentProto_Description_OnItemAndSurvey —— 测点 #2：两处都要有 description。
//
// 现状（RED）：`SurveyItem` / `Survey` 均无该字段 ⇒ E2E-14 实测 3 个量表描述全空。
func TestAgentProto_Description_OnItemAndSurvey(t *testing.T) {
	item := &SurveyItem{Code: "PHQ-9", Description: "过去两周内，以下问题困扰你的频率是多少？"}
	if item.GetDescription() == "" {
		t.Fatal("SurveyItem.description 缺失或为空")
	}
	sv := &Survey{Code: "PHQ-9", Description: "过去两周内，以下问题困扰你的频率是多少？"}
	if sv.GetDescription() == "" {
		t.Fatal("Survey.description 缺失或为空")
	}
}

// TestAgentProto_Answers_KeyedByQuestionKey —— 测点 #3：作答键必须保真（"q1" 不是 1）。
//
// 现状（RED）：`SubmitSurveyRequest.answers` 是 `repeated Answer{question_id int64}`，
// BFF 把 "q1" parseInt64 成 1、服务端再 fmt.Sprintf 成 "1"，而 scorer 期望 "q1"。
func TestAgentProto_Answers_KeyedByQuestionKey(t *testing.T) {
	req := &SubmitSurveyRequest{
		SurveyId:    1,
		Answers:     map[string]int32{"q1": 0, "q2": 3},
		DurationSec: 42,
	}
	if req.GetAnswers()["q1"] != 0 || req.GetAnswers()["q2"] != 3 {
		t.Fatalf("answers 键/值不保真: %+v", req.GetAnswers())
	}

	// 序列化往返后键名仍为 "q1"（跨进程传输的真实断言）
	blob, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal 失败: %v", err)
	}
	var back SubmitSurveyRequest
	if err := proto.Unmarshal(blob, &back); err != nil {
		t.Fatalf("unmarshal 失败: %v", err)
	}
	if _, ok := back.GetAnswers()["q1"]; !ok {
		t.Fatalf("往返后 \"q1\" 键丢失，实际键=%v", back.GetAnswers())
	}
	if _, ok := back.GetAnswers()["1"]; ok {
		t.Fatal("往返后出现数字键 \"1\"，说明键被数值化（E2E-13 症状复发）")
	}
}

// TestAgentProto_SurveyResult_CarriesScoreSemantics —— 测点 #5：结果分数语义完整。
//
// 现状（RED）：`total_score` 是 int32（DB 是 float64，会截断）；且无
// `factor_scores` / `score_kind` ⇒ 人格雷达图取不到维度分、语义标注丢失（E2E-F-97）。
func TestAgentProto_SurveyResult_CarriesScoreSemantics(t *testing.T) {
	r := &SurveyResult{
		ResultId:     7,
		SurveyId:     1,
		TotalScore:   12.5, // 必须能表达小数（int32 会截断成 12）
		RiskLevel:    "moderate",
		ScoreKind:    "dimension_sum",
		FactorScores: map[string]float64{"O": 12.5, "C": 3.0},
		Answers:      map[string]int32{"q1": 2},
	}
	if r.GetTotalScore() != 12.5 {
		t.Fatalf("total_score 小数丢失：got=%v want=12.5（int32 截断？）", r.GetTotalScore())
	}
	if r.GetScoreKind() != "dimension_sum" {
		t.Fatalf("score_kind 丢失: %q", r.GetScoreKind())
	}
	if r.GetFactorScores()["O"] != 12.5 {
		t.Fatalf("factor_scores 丢失: %+v", r.GetFactorScores())
	}
	if _, ok := r.GetAnswers()["q1"]; !ok {
		t.Fatalf("结果侧 answers 键未保真: %+v", r.GetAnswers())
	}
}
