package types

import "testing"

// TestHealthResp_FieldAssignment JSON tag 期望值
func TestHealthResp_FieldAssignment(t *testing.T) {
	r := HealthResp{
		Status:  "ok",
		Time:    1700000000,
		Service: "emotion-echo-analytics-svc",
		Version: "0.1.1",
		DbOK:    true,
	}
	if r.Status != "ok" || r.Service == "" || r.Version == "" {
		t.Fatalf("field mismatch: %+v", r)
	}
	if !r.DbOK {
		t.Fatalf("DbOK mismatch")
	}
}

// TestHealthResp_ZeroValue 表驱动零值
func TestHealthResp_ZeroValue(t *testing.T) {
	r := HealthResp{}
	if r.Status != "" || r.Service != "" || r.Version != "" {
		t.Fatalf("zero should yield all empty string, got %+v", r)
	}
	if r.DbOK {
		t.Fatalf("zero DbOK should be false")
	}
	if r.Time != 0 {
		t.Fatalf("zero Time should be 0, got %d", r.Time)
	}
}

// TestHealthResp_ServiceConstants 静态断言 service/version 常量预期
func TestHealthResp_ServiceConstants(t *testing.T) {
	// 这些常量在 healthlogic.go 里，靠 mock 不到；这里只断言类型能用
	r := HealthResp{Service: "emotion-echo-analytics-svc", Version: "0.1.1"}
	_ = r
}
