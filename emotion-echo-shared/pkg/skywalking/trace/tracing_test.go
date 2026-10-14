package trace

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/SkyAPM/go2sky"
	agentv3 "skywalking.apache.org/repo/goapi/collect/language/agent/v3"
)

// TestCreateExitSpan_NilTracer_NoPanic 当 tracer 为 nil 时不 panic 且返回 noop end
func TestCreateExitSpan_NilTracer_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("should not panic on nil tracer, got %v", r)
		}
	}()
	end := CreateExitSpan(context.Background(), nil, "noop", "noop@local")
	if end == nil {
		t.Fatalf("end should never be nil")
	}
	end()
	end(WithError(errors.New("x")))
	end(WithTag("k", "v"))
}

// TestCreateExitSpan_NilCtx 允许 ctx == nil（ContextOrBg 自动转 Background）
func TestCreateExitSpan_NilCtx(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil ctx should not panic, got %v", r)
		}
	}()
	var ctx context.Context // nil
	end := CreateExitSpan(ctx, nil, "any", "any@x")
	if end == nil {
		t.Fatalf("end should be non-nil")
	}
	end()
}

// TestCreateExitSpan_EmptyNameOrPeer 空 name/peer 直接 noop（go2sky 会返回 errParameter）
func TestCreateExitSpan_EmptyNameOrPeer(t *testing.T) {
	if end := CreateExitSpan(context.Background(), nil, "", "peer"); end == nil {
		t.Fatalf("empty name should still return non-nil end")
	}
	if end := CreateExitSpan(context.Background(), nil, "name", ""); end == nil {
		t.Fatalf("empty peer should still return non-nil end")
	}
}

// TestCreateExitSpan_TableDriven 表驱动：name/peer 任意字符串都应返回非 nil end（当 tracer=nil）
func TestCreateExitSpan_TableDriven(t *testing.T) {
	cases := []struct {
		name   string
		opName string
		peer   string
	}{
		{"gorm_query_users", "gorm.query users", "postgres@emotion_echo"},
		{"redis_get", "redis.GET", "redis@redis:6379"},
		{"http_post", "http.POST /api/v1/chat", "svc@chat-svc:8888"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			end := CreateExitSpan(context.Background(), nil, tc.opName, tc.peer)
			if end == nil {
				t.Fatalf("expected non-nil end closure")
			}
			end()
		})
	}
}

// TestContextOrBg_NilToBackground nil ctx 替换为 Background
func TestContextOrBg_NilToBackground(t *testing.T) {
	got := ContextOrBg(nil)
	if got == nil {
		t.Fatalf("nil ctx should be replaced with non-nil")
	}
	if _, ok := got.Deadline(); ok {
		t.Fatalf("background ctx should have no deadline")
	}
}

// TestContextOrBg_NonNilPreserved 非 nil 上下文原样返回
func TestContextOrBg_NonNilPreserved(t *testing.T) {
	type k struct{}
	parent := context.WithValue(context.Background(), k{}, "v")
	got := ContextOrBg(parent)
	if got.Value(k{}) != "v" {
		t.Fatalf("non-nil ctx should be preserved")
	}
}

// recSpan 是一个最小满足 go2sky.Span 接口的实现。
type recSpan struct {
	errCalls  int
	errArgs   []string
	tagCalls  []tagKV
	logCalls  int
	endCalled bool
}

type tagKV struct{ K, V string }

func (s *recSpan) SetOperationName(string)        {}
func (s *recSpan) GetOperationName() string       { return "op" }
func (s *recSpan) SetPeer(string)                 {}
func (s *recSpan) SetSpanLayer(agentv3.SpanLayer) {}
func (s *recSpan) SetComponent(int32)             {}
func (s *recSpan) Tag(_ go2sky.Tag, val string)   { s.tagCalls = append(s.tagCalls, tagKV{"", val}) }
func (s *recSpan) Log(_ time.Time, _ ...string)   { s.logCalls++ }
func (s *recSpan) Error(_ time.Time, args ...string) {
	s.errCalls++
	s.errArgs = append(s.errArgs, args...)
}
func (s *recSpan) End()           { s.endCalled = true }
func (s *recSpan) IsEntry() bool  { return false }
func (s *recSpan) IsExit() bool   { return false }
func (s *recSpan) IsValid() bool  { return true }

// TestWithError_RecordsErrorSpan WithError 应触发 span.Error 并写入消息
func TestWithError_RecordsErrorSpan(t *testing.T) {
	s := &recSpan{}
	WithError(errors.New("boom"))(s)
	if s.errCalls != 1 {
		t.Fatalf("expected 1 Error call, got %d", s.errCalls)
	}
	if len(s.errArgs) < 1 || s.errArgs[0] != "boom" {
		t.Fatalf("expected error message=boom, got %+v", s.errArgs)
	}
}

// TestWithError_NilError_NoRecord nil 错误不应触发 Error
func TestWithError_NilError_NoRecord(t *testing.T) {
	s := &recSpan{}
	WithError(nil)(s)
	if s.errCalls != 0 {
		t.Fatalf("WithError(nil) should not invoke Error, got %d calls", s.errCalls)
	}
}

// TestWithTag_RecordsTag 表驱动：WithTag 应记录 tag
func TestWithTag_TableDriven(t *testing.T) {
	cases := []struct{ k, v string }{
		{"k1", "v1"},
		{"http.status", "200"},
		{"db.table", "users"},
	}
	for _, tc := range cases {
		s := &recSpan{}
		WithTag(tc.k, tc.v)(s)
		if len(s.tagCalls) != 1 {
			t.Fatalf("expected 1 tag for k=%s v=%s, got %d", tc.k, tc.v, len(s.tagCalls))
		}
	}
}
