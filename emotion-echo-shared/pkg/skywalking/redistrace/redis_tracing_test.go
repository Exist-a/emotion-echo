package redistrace

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/redis/go-redis/v9"
)

// TestJoinCmds_Empty 空切片返回空字符串
func TestJoinCmds_Empty(t *testing.T) {
	if got := joinCmds(nil); got != "" {
		t.Fatalf("empty list: want '' got %q", got)
	}
	if got := joinCmds([]string{}); got != "" {
		t.Fatalf("empty slice: want '' got %q", got)
	}
}

// TestJoinCmds_Single 单元素无逗号
func TestJoinCmds_Single(t *testing.T) {
	if got := joinCmds([]string{"GET"}); got != "GET" {
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
		if got := joinCmds(tc.in); got != tc.want {
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
	if got := wrapped(context.Background(), cmd); got != wantErr {
		t.Fatalf("error should be passed through, got %v", got)
	}
	if !called {
		t.Fatalf("ProcessHook should call next under nil tracer")
	}
}

// TestRedisTracingHook_ProcessHook_RedisNil_PassesThrough redis.Nil 语义保留
func TestRedisTracingHook_ProcessHook_RedisNil_PassesThrough(t *testing.T) {
	h := &redisTracingHook{addr: "redis:6379"}
	called := false
	original := func(ctx context.Context, cmd redis.Cmder) error {
		called = true
		return redis.Nil
	}
	wrapped := h.ProcessHook(original)
	cmd := redis.NewCmd(context.Background(), "GET", "missing")
	if got := wrapped(context.Background(), cmd); got != redis.Nil {
		t.Fatalf("redis.Nil should be preserved, got %v", got)
	}
	if !called {
		t.Fatalf("next should be called")
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
	if got := wrapped(context.Background(), cmds); got != wantErr {
		t.Fatalf("err should pass through, got %v", got)
	}
	if !called {
		t.Fatalf("next should be called")
	}
}

// TestInitRedis_NilSafe nil client 不 panic 且返 false
func TestInitRedis_NilSafe(t *testing.T) {
	if InitRedis(nil) {
		t.Fatalf("InitRedis(nil) should return false")
	}
}

// TestInitRedis_RealClientRegistersHook 真 *redis.Client → 返 true（hook 已注册）
func TestInitRedis_RealClientRegistersHook(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"}) // 不真连
	defer rdb.Close()
	if !InitRedis(rdb) {
		t.Fatalf("*redis.Client 必须返 true（hook 已注册信号）")
	}
}

// TestInstrumentRedis_NilClient_NoPanic 传 nil client 不得 panic
func TestInstrumentRedis_NilClient_NoPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("InstrumentRedis(nil) should not panic, got %v", r)
		}
	}()
	var rdb *redis.Client
	InstrumentRedis(rdb)
}
