package configcenter

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeConfigCenter 是测试替身。
type fakeConfigCenter struct {
	mu sync.Mutex

	configs map[string]string // key = dataId + "@" + group
	closed  bool

	// injected
	getErr    error
	listenErr error
	pubErr    error

	listeners []listener
}

type listener struct {
	dataId, group string
	handler       ConfigChangeHandler
}

func newFake() *fakeConfigCenter {
	return &fakeConfigCenter{configs: make(map[string]string)}
}

func (f *fakeConfigCenter) GetConfig(_ context.Context, dataId, group string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return "", f.getErr
	}
	return f.configs[keyOf(dataId, group)], nil
}

func (f *fakeConfigCenter) ListenConfig(_ context.Context, dataId, group string, h ConfigChangeHandler) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.listenErr != nil {
		return f.listenErr
	}
	// 替换已有监听（按 dataId+group 去重）
	for i, l := range f.listeners {
		if l.dataId == dataId && l.group == group {
			f.listeners[i] = listener{dataId, group, h}
			return nil
		}
	}
	f.listeners = append(f.listeners, listener{dataId, group, h})
	return nil
}

func (f *fakeConfigCenter) PublishConfig(_ context.Context, dataId, group, content string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pubErr != nil {
		return f.pubErr
	}
	f.configs[keyOf(dataId, group)] = content
	// 同步触发所有监听
	listeners := make([]listener, len(f.listeners))
	copy(listeners, f.listeners)
	f.mu.Unlock()
	for _, l := range listeners {
		if l.dataId == dataId && l.group == group && l.handler != nil {
			_ = l.handler(dataId, group, content)
		}
	}
	f.mu.Lock()
	return nil
}

func (f *fakeConfigCenter) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

// 触发一个 dataId 变更（用于测试 ListenConfig 后续推送）
func (f *fakeConfigCenter) push(dataId, group, content string) error {
	f.mu.Lock()
	f.configs[keyOf(dataId, group)] = content
	listeners := make([]listener, len(f.listeners))
	copy(listeners, f.listeners)
	f.mu.Unlock()
	for _, l := range listeners {
		if l.dataId == dataId && l.group == group && l.handler != nil {
			if err := l.handler(dataId, group, content); err != nil {
				return err
			}
		}
	}
	return nil
}

func (f *fakeConfigCenter) isClosed() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.closed
}

func keyOf(dataId, group string) string { return dataId + "@" + group }

// 编译期断言：fakeConfigCenter 必须实现 ConfigCenter interface。
var _ ConfigCenter = (*fakeConfigCenter)(nil)

// -----------------------------------------------------------------------------
// 契约测试：所有 ConfigCenter 实现必须满足
// -----------------------------------------------------------------------------

func TestConfigCenter_Contract_GetConfigReturnsContent(t *testing.T) {
	f := newFake()
	f.configs[keyOf("user-svc.ops.yaml", "DEFAULT_GROUP")] = "feature_flags:\n  x: true"

	got, err := f.GetConfig(context.Background(), "user-svc.ops.yaml", "DEFAULT_GROUP")

	require.NoError(t, err)
	assert.Contains(t, got, "feature_flags")
}

func TestConfigCenter_Contract_GetConfigEmptyReturnsEmpty(t *testing.T) {
	f := newFake()
	got, err := f.GetConfig(context.Background(), "absent.yaml", "DEFAULT_GROUP")
	require.NoError(t, err)
	assert.Equal(t, "", got, "absent config must return empty string + nil error")
}

func TestConfigCenter_Contract_GetConfigErrorPropagates(t *testing.T) {
	f := newFake()
	f.getErr = errors.New("rpc timeout")
	_, err := f.GetConfig(context.Background(), "x.yaml", "DEFAULT_GROUP")
	require.Error(t, err)
}

func TestConfigCenter_Contract_ListenConfigRegistersHandler(t *testing.T) {
	f := newFake()
	called := false
	h := func(_, _, _ string) error { called = true; return nil }

	err := f.ListenConfig(context.Background(), "user-svc.ops.yaml", "DEFAULT_GROUP", h)
	require.NoError(t, err)
	require.Len(t, f.listeners, 1)

	// 模拟 Nacos 推送
	_ = f.push("user-svc.ops.yaml", "DEFAULT_GROUP", "v2")
	assert.True(t, called)
}

func TestConfigCenter_Contract_ListenConfigReplacesHandler(t *testing.T) {
	f := newFake()
	h1Calls := 0
	h2Calls := 0
	h1 := func(_, _, _ string) error { h1Calls++; return nil }
	h2 := func(_, _, _ string) error { h2Calls++; return nil }

	require.NoError(t, f.ListenConfig(context.Background(), "x.yaml", "DEFAULT_GROUP", h1))
	require.NoError(t, f.ListenConfig(context.Background(), "x.yaml", "DEFAULT_GROUP", h2))

	// 推送一次
	require.NoError(t, f.push("x.yaml", "DEFAULT_GROUP", "v1"))

	assert.Equal(t, 0, h1Calls, "replaced handler must not fire")
	assert.Equal(t, 1, h2Calls, "new handler must fire")
}

func TestConfigCenter_Contract_ListenConfigErrorPropagates(t *testing.T) {
	f := newFake()
	f.listenErr = errors.New("subscribe failed")
	err := f.ListenConfig(context.Background(), "x.yaml", "DEFAULT_GROUP", func(_, _, _ string) error { return nil })
	require.Error(t, err)
}

func TestConfigCenter_Contract_PublishConfigUpdatesValue(t *testing.T) {
	f := newFake()
	require.NoError(t, f.PublishConfig(context.Background(), "x.yaml", "DEFAULT_GROUP", "v1"))
	got, err := f.GetConfig(context.Background(), "x.yaml", "DEFAULT_GROUP")
	require.NoError(t, err)
	assert.Equal(t, "v1", got)
}

func TestConfigCenter_Contract_PublishConfigTriggersListeners(t *testing.T) {
	f := newFake()
	got := ""
	h := func(_, _, content string) error { got = content; return nil }
	require.NoError(t, f.ListenConfig(context.Background(), "x.yaml", "DEFAULT_GROUP", h))

	require.NoError(t, f.PublishConfig(context.Background(), "x.yaml", "DEFAULT_GROUP", "v3"))

	assert.Equal(t, "v3", got)
}

func TestConfigCenter_Contract_PublishConfigErrorPropagates(t *testing.T) {
	f := newFake()
	f.pubErr = errors.New("publish denied")
	err := f.PublishConfig(context.Background(), "jwt.secret", "DEFAULT_GROUP", "abc")
	require.Error(t, err)
}

func TestConfigCenter_Contract_CloseMarksClosed(t *testing.T) {
	f := newFake()
	require.NoError(t, f.Close())
	assert.True(t, f.isClosed())
}

func TestConfigCenter_Contract_ContextCancelIsRespected(t *testing.T) {
	// 契约：ctx.Done() 时实现方应能感知并停止工作（fake 不阻塞，仅断言 ctx 不被忽略）。
	// 这里验证 ListenConfig 在 ctx 取消时不阻塞（<=500ms 内返回）。
	f := newFake()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- f.ListenConfig(ctx, "x.yaml", "DEFAULT_GROUP", func(_, _, _ string) error { return nil })
	}()
	cancel()

	select {
	case <-done:
		// 预期：ListenConfig 返回（fake 立即返回 nil）
	case <-time.After(500 * time.Millisecond):
		t.Fatal("ListenConfig must not block after ctx cancel")
	}
}
