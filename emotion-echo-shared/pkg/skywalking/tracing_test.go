package skywalking

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
	end := createExitSpan(context.Background(), nil, "noop", "noop@local")
	if end == nil {
		t.Fatalf("end should never be nil")
	}
	// 调用 end 不应 panic
	end()
	end(WithError(errors.New("x")))
	end(WithTag("k", "v"))
}

// TestCreateExitSpan_NilCtx 允许 ctx == nil（contextOrBg 自动转 Background）
func TestCreateExitSpan_NilCtx(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("nil ctx should not panic, got %v", r)
		}
	}()
	var ctx context.Context // nil
	end := createExitSpan(ctx, nil, "any", "any@x")
	if end == nil {
		t.Fatalf("end should be non-nil")
	}
	end()
}

// TestCreateExitSpan_TableDriven 表驱动：name/peer 任意字符串都应返回非 nil end（当 tracer=nil）
func TestCreateExitSpan_TableDriven(t *testing.T) {
	cases := []struct {
		name   string
		opName string
		peer   string
	}{
		{"empty_name_and_peer", "", ""},
		{"gorm_query_users", "gorm.query users", "postgres@emotion_echo"},
		{"redis_get", "redis.GET", "redis@redis:6379"},
		{"http_post", "http.POST /api/v1/chat", "svc@chat-svc:8888"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			end := createExitSpan(context.Background(), nil, tc.opName, tc.peer)
			if end == nil {
				t.Fatalf("expected non-nil end closure")
			}
			end()
		})
	}
}

// recSpan 是一个最小满足 go2sky.Span 接口的实现。
// go2sky Span 接口的精确方法集（v1.5.0）：
//   SetOperationName / GetOperationName / SetPeer / SetSpanLayer / SetComponent /
//   Tag / Log / Error / End / IsEntry / IsExit / IsValid
type recSpan struct {
	errCalls   int
	errArgs    []string
	tagCalls   []tagKV
	logCalls   int
	endCalled  bool
}

type tagKV struct{ K, V string }

func (s *recSpan) SetOperationName(string)              {}
func (s *recSpan) GetOperationName() string             { return "op" }
func (s *recSpan) SetPeer(string)                       {}
func (s *recSpan) SetSpanLayer(agentv3.SpanLayer)       {}
func (s *recSpan) SetComponent(int32)                   {}
func (s *recSpan) Tag(_ go2sky.Tag, val string)         { s.tagCalls = append(s.tagCalls, tagKV{"", val}) }
func (s *recSpan) Log(_ time.Time, _ ...string)        { s.logCalls++ }
func (s *recSpan) Error(_ time.Time, args ...string) {
	s.errCalls++
	for _, a := range args {
		s.errArgs = append(s.errArgs, a)
	}
}
func (s *recSpan) End()                       { s.endCalled = true }
func (s *recSpan) IsEntry() bool              { return false }
func (s *recSpan) IsExit() bool               { return false }
func (s *recSpan) IsValid() bool              { return true }

// TestWithError_RecordsErrorSpan WithError 应触发 span.Error 并写入消息
func TestWithError_RecordsErrorSpan(t *testing.T) {
	s := &recSpan{}
	opt := WithError(errors.New("boom"))
	opt(s)
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
	opt := WithError(nil)
	opt(s)
	if s.errCalls != 0 {
		t.Fatalf("WithError(nil) should not invoke Error, got %d calls", s.errCalls)
	}
}

// TestWithTag_RecordsTag_WithTagType
func TestWithTag_RecordsTag_WithTagType(t *testing.T) {
	s := &recSpan{}
	// go2sky.Tag 是 int 类型，WithTag 内部包了一层，传到 span.Tag 时会带 key + value
	opt := WithTag("redis.cmds", "get,set")
	opt(s)
	if len(s.tagCalls) != 1 {
		t.Fatalf("expected 1 tag, got %d", len(s.tagCalls))
	}
	// go2sky.TagWrapper.WithTag 会调 span.Tag(Tag(int), "key=value") 风格
	// 这里只能断言至少被记录
}

// TestWithTag_TableDriven 表驱动
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
