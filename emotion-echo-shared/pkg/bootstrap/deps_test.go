package bootstrap

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestShouldFailFast(t *testing.T) {
	cases := []struct {
		env   string
		want  bool
		setup func()
	}{
		{"", false, nil},
		{"false", false, nil},
		{"true", true, nil},
		{"1", true, nil},
		{"yes", true, nil},
	}
	for _, c := range cases {
		if c.setup != nil {
			c.setup()
		}
		t.Setenv("STARTUP_STRICT", c.env)
		got := ShouldFailFast()
		if got != c.want {
			t.Errorf("STARTUP_STRICT=%q: got %v, want %v", c.env, got, c.want)
		}
	}
}

func TestIsRequired(t *testing.T) {
	t.Setenv("STARTUP_STRICT_DEPS", "postgres,kafka")
	cases := []struct {
		dep  string
		want bool
	}{
		{"postgres", true},
		{"kafka", true},
		{"skywalking", false},
		{"llm", false},
	}
	for _, c := range cases {
		got := IsRequired(c.dep)
		if got != c.want {
			t.Errorf("IsRequired(%q): got %v, want %v", c.dep, got, c.want)
		}
	}
}

func TestCheckTCP_LiveAddr(t *testing.T) {
	// 起一个临时 listener
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skipf("cannot listen: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().String()

	if err := CheckTCP(context.Background(), addr, 2*time.Second); err != nil {
		t.Errorf("CheckTCP(%s): %v", addr, err)
	}
}

func TestCheckTCP_DeadAddr(t *testing.T) {
	// 找一个肯定关闭的端口
	if err := CheckTCP(context.Background(), "127.0.0.1:1", 500*time.Millisecond); err == nil {
		t.Error("expected error connecting to dead addr, got nil")
	}
}