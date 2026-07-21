package model

import (
	"encoding/json"
	"testing"
	"time"
)

// TestSurvey_TableName 表名校验
func TestSurvey_TableName(t *testing.T) {
	s := Survey{}
	if got := s.TableName(); got != "emotion_echo_assessment.surveys" {
		t.Fatalf("want 'emotion_echo_assessment.surveys' got %q", got)
	}
}

// TestSurveyResult_TableName 表名
func TestSurveyResult_TableName(t *testing.T) {
	r := SurveyResult{}
	if got := r.TableName(); got != "emotion_echo_assessment.survey_results" {
		t.Fatalf("want 'emotion_echo_assessment.survey_results' got %q", got)
	}
}

// TestJSONMap_Value_Nil 入库：nil JSONMap -> NULL
func TestJSONMap_Value_Nil(t *testing.T) {
	var m JSONMap
	v, err := m.Value()
	if err != nil {
		t.Fatalf("Value(nil): %v", err)
	}
	if v != nil {
		t.Fatalf("nil JSONMap should yield nil driver value, got %v", v)
	}
}

// TestJSONMap_Value_NonEmpty 编码为 JSON 字节
func TestJSONMap_Value_NonEmpty(t *testing.T) {
	m := JSONMap{"q1": float64(2), "q2": "v"}
	v, err := m.Value()
	if err != nil {
		t.Fatalf("Value: %v", err)
	}
	if v == nil {
		t.Fatalf("non-empty JSONMap should yield non-nil driver value")
	}
	b, ok := v.([]byte)
	if !ok {
		t.Fatalf("expected []byte, got %T", v)
	}
	var back JSONMap
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("unmarshal back: %v", err)
	}
	if back["q1"] == nil || back["q2"] == nil {
		t.Fatalf("round-trip lost data: %+v", back)
	}
}

// TestJSONMap_Scan_Nil 入参 nil -> 结果 nil
func TestJSONMap_Scan_Nil(t *testing.T) {
	var m JSONMap
	if err := m.Scan(nil); err != nil {
		t.Fatalf("Scan(nil): %v", err)
	}
	if m != nil {
		t.Fatalf("nil source should yield nil JSONMap, got %+v", m)
	}
}

// TestJSONMap_Scan_Bytes 与 Scan_String 类型驱动
func TestJSONMap_Scan_TableDriven(t *testing.T) {
	cases := []struct {
		src      any
		wantSize int
	}{
		{[]byte(`{"a":1}`), 1},
		{`{"b":2}`, 1},
		{[]byte(``), 0}, // empty -> nil
	}
	for _, tc := range cases {
		var m JSONMap
		if err := m.Scan(tc.src); err != nil {
			t.Fatalf("Scan(%v): %v", tc.src, err)
		}
		if tc.wantSize == 0 && m != nil {
			t.Fatalf("expected nil for empty, got %+v", m)
		}
		if tc.wantSize > 0 && len(m) != tc.wantSize {
			t.Fatalf("size mismatch for %v: want %d got %d", tc.src, tc.wantSize, len(m))
		}
	}
}

// TestSurvey_Fields 表驱动字段读写
func TestSurvey_Fields(t *testing.T) {
	now := time.Now()
	s := Survey{
		ID:           1,
		Code:         "PHQ-9",
		Title:        "抑郁量表",
		Description:  "9 题",
		Category:     "depression",
		Questions:    JSONMap{"q1": "情绪低落"},
		ScoringRules: JSONMap{"0-4": "none"},
		Version:      1,
		Status:       1,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if s.Code != "PHQ-9" || s.Version != 1 {
		t.Fatalf("field mismatch: %+v", s)
	}
}

// TestSurveyResult_Fields 表驱动
func TestSurveyResult_Fields(t *testing.T) {
	now := time.Now()
	r := SurveyResult{
		ID:           100,
		UserID:       7,
		SurveyID:     1,
		Answers:      JSONMap{"q1": 2},
		TotalScore:   18.5,
		FactorScores: JSONMap{"cognitive": 6.5},
		RiskLevel:    "severe",
		DurationSec:  120,
		SubmittedAt:  now,
	}
	if r.UserID != 7 || r.RiskLevel != "severe" {
		t.Fatalf("field mismatch: %+v", r)
	}
}
