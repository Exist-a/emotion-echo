// internal/memory/summary_store_test.go
//
// CL-08a · A3：摘要写侧契约（假 setter，不连 Redis）。

package memory

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeSetter struct {
	key    string
	value  string
	ttl    time.Duration
	setErr error
	delErr error
	deleted bool
}

func (f *fakeSetter) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	f.key, f.value, f.ttl = key, value, ttl
	return f.setErr
}

func (f *fakeSetter) Del(ctx context.Context, key string) error {
	f.key = key
	f.deleted = true
	return f.delErr
}

func TestRedisSummaryStore_WritesKeyAndTTL(t *testing.T) {
	t.Parallel()
	fs := &fakeSetter{}
	store := newSummaryStoreWithSetter(fs, 24*time.Hour)

	if err := store.Set(context.Background(), 42, "- 2026-10 用户是后端工程师"); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if fs.key != "memory:summary:user:42" {
		t.Errorf("key = %q, want memory:summary:user:42", fs.key)
	}
	if fs.value != "- 2026-10 用户是后端工程师" {
		t.Errorf("value = %q", fs.value)
	}
	if fs.ttl != 24*time.Hour {
		t.Errorf("ttl = %v, want 24h", fs.ttl)
	}
}

func TestRedisSummaryStore_EmptySummaryDeletesKey(t *testing.T) {
	t.Parallel()
	fs := &fakeSetter{}
	store := newSummaryStoreWithSetter(fs, 24*time.Hour)

	if err := store.Set(context.Background(), 42, ""); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if !fs.deleted {
		t.Error("空摘要必须删除键（避免注入陈旧记忆）")
	}
	if fs.key != "memory:summary:user:42" {
		t.Errorf("del key = %q", fs.key)
	}
}

func TestRedisSummaryStore_PropagatesError(t *testing.T) {
	t.Parallel()
	fs := &fakeSetter{setErr: errors.New("redis down")}
	store := newSummaryStoreWithSetter(fs, time.Hour)

	if err := store.Set(context.Background(), 1, "x"); err == nil {
		t.Fatal("Redis 错误必须向上返回（调用方记日志，但不影响抽取落库）")
	}
}

func TestInMemorySummaryStore(t *testing.T) {
	t.Parallel()
	store := NewInMemorySummaryStore()
	if err := store.Set(context.Background(), 7, "摘要"); err != nil {
		t.Fatal(err)
	}
	if got, ok := store.Get(7); !ok || got != "摘要" {
		t.Fatalf("Get(7) = %q, %v", got, ok)
	}
}
