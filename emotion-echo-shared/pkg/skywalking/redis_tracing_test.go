package skywalking

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/redis/go-redis/v9"
)

// TestJoinCmds 空切片返回空字符串
func TestJoinCmds_Empty(t *testing.T) {
	got := joinCmds(nil)
	if got != "" {
		t.Fatalf("empty list: want '' got %q", got)
	}
	got = joinCmds([]string{})
	if got != "" {
		t.Fatalf("empty slice: want '' got %q", got)
	}
}

// TestJoinCmds_Single 单元素无逗号
func TestJoinCmds_Single(t *testing.T) {
	got := joinCmds([]string{"GET"})
	if got != "GET" {
		t.Fatalf("single: want 'GET' got %q", got)
	}
}

// TestJoinCmds_TableDriven 多元素拼接
func TestJoinCmds_TableDriven(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{[]string{"GET", "SET"}, "GET,SET"},
		{[]string{"GET", "SET", "DEL"}, "GET,SET,DEL"},
		{[]string{"MULTI", "EXEC", "DISCARD"}, "MULTI,EXEC,DISCARD"},
	}
	for _, tc := range cases {
		got := joinCmds(tc.in)
		if got != tc.want {
			t.Fatalf("in=%v want=%q got=%q", tc.in, tc.want, got)
		}
	}
}

// TestRedisTracingHook_ImplementsInterface 静态断言 hook 实现 redis.Hook
func TestRedisTracingHook_ImplementsInterface(t *testing.T) {
	var _ redis.Hook = (*redisTracingHook)(nil)
}

// TestRedisTracingHook_DialHook_ReturnsNext DialHook 应直接透传
func TestRedisTracingHook_DialHook_ReturnsNext(t *testing.T) {
	h := &redisTracingHook{addr: "redis:6379"}
	called := false
	original := func(ctx context.Context, network, addr string) (net.Conn, error) {
		called = true
		return nil, nil
	}
	wrapped := h.DialHook(original)
	if wrapped == nil {
		t.Fatalf("DialHook should return non-nil")
	}
	_, _ = wrapped(context.Background(), "tcp", "127.0.0.1:6379")
	if !called {
		t.Fatalf("DialHook should call original")
	}
}

// TestRedisTracingHook_ProcessHook_NilTracer_PassesThrough Nil tracer 时直接调用 next
func TestRedisTracingHook_ProcessHook_NilTracer_PassesThrough(t *testing.T) {
	h := &redisTracingHook{addr: "redis:6379"}
	called := false
	wantErr := errors.New("upstream-err")
	original := func(ctx context.Context, cmd redis.Cmder) error {
		called = true
		return wantErr
	}
	wrapped := h.ProcessHook(original)
	cmd := redis.NewCmd(context.Background(), "GET", "k")
	got := wrapped(context.Background(), cmd)
	if !called {
		t.Fatalf("ProcessHook should call next under nil tracer")
	}
	if got != wantErr {
		t.Fatalf("error should be passed through, got %v", got)
	}
}

// TestRedisTracingHook_ProcessHook_RedisNil_PassesThrough
func TestRedisTracingHook_ProcessHook_RedisNil_PassesThrough(t *testing.T) {
	h := &redisTracingHook{addr: "redis:6379"}
	called := false
	original := func(ctx context.Context, cmd redis.Cmder) error {
		called = true
		return redis.Nil
	}
	wrapped := h.ProcessHook(original)
	cmd := redis.NewCmd(context.Background(), "GET", "missing")
	got := wrapped(context.Background(), cmd)
	if !called {
		t.Fatalf("next should be called")
	}
	if got != redis.Nil {
		t.Fatalf("redis.Nil should be preserved, got %v", got)
	}
}

// TestRedisTracingHook_ProcessPipelineHook_NilTracer_PassesThrough
func TestRedisTracingHook_ProcessPipelineHook_NilTracer_PassesThrough(t *testing.T) {
	h := &redisTracingHook{addr: "redis:6379"}
	called := false
	wantErr := errors.New("pipe-err")
	original := func(ctx context.Context, cmds []redis.Cmder) error {
		called = true
		return wantErr
	}
	wrapped := h.ProcessPipelineHook(original)
	cmds := []redis.Cmder{
		redis.NewCmd(context.Background(), "GET", "a"),
		redis.NewCmd(context.Background(), "SET", "b", "1"),
	}
	got := wrapped(context.Background(), cmds)
	if !called {
		t.Fatalf("next should be called")
	}
	if got != wantErr {
		t.Fatalf("err should pass through, got %v", got)
	}
}

// TestInstrumentRedis_NilClient_NoPanic **Stage 26-N 修复后**：传 nil client 不得 panic
//
// 历史：Stage 26-A 暴露此 bug 并以 t.Logf 记录，N 批实现加 nil client 防御。
// 本测试同步更新为 NoPanic 断言（实现已修）。
func TestInstrumentRedis_NilClient_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("InstrumentRedis(nil) should not panic, got %v", r)
		}
	}()
	var rdb *redis.Client
	InstrumentRedis(rdb)
}
