package healthcheck

import (
	"testing"
)

// TestServer_NewServer_DefaultServing 新建 server 默认是 SERVING
func TestServer_NewServer_DefaultServing(t *testing.T) {
	s := NewServer()
	if got := s.GetServingStatus(""); got != ServingStatusServing {
		t.Fatalf("default status want Serving, got %v", got)
	}
}

// TestServer_SetServingStatus 写入后 GetServingStatus 应回显
func TestServer_SetServingStatus(t *testing.T) {
	s := NewServer()
	s.SetServingStatus("emotion-llm", ServingStatusNotServing)
	if got := s.GetServingStatus("emotion-llm"); got != ServingStatusNotServing {
		t.Fatalf("want NotServing, got %v", got)
	}
}

// TestServer_GetServingStatus_Unknown 未注册 service 返回 Unknown
func TestServer_GetServingStatus_Unknown(t *testing.T) {
	s := NewServer()
	if got := s.GetServingStatus("never-registered"); got != ServingStatusUnknown {
		t.Fatalf("want Unknown, got %v", got)
	}
}

// TestServer_SetServingStatus_Overwrite 写入两次以新值为准
func TestServer_SetServingStatus_Overwrite(t *testing.T) {
	s := NewServer()
	s.SetServingStatus("svc", ServingStatusNotServing)
	s.SetServingStatus("svc", ServingStatusServing)
	if got := s.GetServingStatus("svc"); got != ServingStatusServing {
		t.Fatalf("want Serving after overwrite, got %v", got)
	}
}

// TestServer_Shutdown_MarksAllNotServing Shutdown 应把已知 svc 全部置 NOT_SERVING
//
// **当前实现 bug**：Shutdown 只更新 grpc.inner 的状态，本地 status map 未刷新。
// 本测试如实记录当前行为并标 fail（known bug）。
// 修复方向：Shutdown 应同时 `s.status[svc] = ServingStatusNotServing` for all svc。
func TestServer_Shutdown_MarksAllNotServing(t *testing.T) {
	s := NewServer()
	s.SetServingStatus("svc-a", ServingStatusServing)
	s.SetServingStatus("svc-b", ServingStatusServing)

	s.Shutdown()

	// Stage 26-N 修复后：Shutdown 必须同步本地 status map（GetServingStatus 返 NOT_SERVING）
	gotA := s.GetServingStatus("svc-a")
	gotB := s.GetServingStatus("svc-b")
	if gotA != ServingStatusNotServing {
		t.Fatalf("svc-a: want NotServing, got %v", gotA)
	}
	if gotB != ServingStatusNotServing {
		t.Fatalf("svc-b: want NotServing, got %v", gotB)
	}
}

// TestServer_Shutdown_Idempotent 第二次 Shutdown 不 panic
func TestServer_Shutdown_Idempotent(t *testing.T) {
	s := NewServer()
	s.SetServingStatus("svc", ServingStatusServing)

	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Second Shutdown should not panic, got %v", r)
		}
	}()
	s.Shutdown()
	s.Shutdown() // idempotent — Stage 26-N bug #4
	s.Shutdown()
}

// TestServer_Resume_AfterShutdown 把已知 svc 恢复 SERVING
func TestServer_Resume_AfterShutdown(t *testing.T) {
	s := NewServer()
	s.SetServingStatus("svc", ServingStatusServing)
	s.Shutdown()
	s.Resume()
	if got := s.GetServingStatus("svc"); got != ServingStatusServing {
		t.Fatalf("Resume should restore Serving, got %v", got)
	}
}

// TestServingStatus_ToProto 表驱动：4 个常量 -> proto 枚举
func TestServingStatus_ToProto(t *testing.T) {
	cases := []struct {
		in   ServingStatus
		want string
	}{
		{ServingStatusUnknown, "UNKNOWN"},
		{ServingStatusServing, "SERVING"},
		{ServingStatusNotServing, "NOT_SERVING"},
		{ServingStatusServiceUnknown, "SERVICE_UNKNOWN"},
	}
	for _, tc := range cases {
		// 通过反射或枚举字段名 — 这里用 switch 验证
		if tc.in.toProto().String() != tc.want {
			t.Fatalf("toProto(%v) want=%s got=%s", tc.in, tc.want, tc.in.toProto().String())
		}
	}
}

// TestFromProto_AllCases 表驱动：proto 枚举 -> 内部枚举（不通过 toProto 闭环验证）
func TestFromProto_AllCases(t *testing.T) {
	// 由于 fromProto 接受 protobuf enum，需通过反射或显式调用
	// 简单验证：反转 fromProto := fromProto(s.toProto()) 应回到原值
	for _, s := range []ServingStatus{
		ServingStatusUnknown,
		ServingStatusServing,
		ServingStatusNotServing,
		ServingStatusServiceUnknown,
	} {
		got := fromProto(s.toProto())
		if got != s {
			t.Fatalf("to/from round-trip mismatch: in=%v got=%v", s, got)
		}
	}
}
