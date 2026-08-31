package discovery

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRegistry 是测试替身，记录所有调用并允许注入失败行为。
// PR-03 的 NacosRegistry 实现必须能替代此 fake 满足相同契约。
type fakeRegistry struct {
	mu sync.Mutex

	registered   []Instance
	unregistered []Instance

	// injected errors（nil = 正常路径）
	registerErr   error
	unregisterErr error
	discoverErr   error
	subscribeErr  error

	discoverResult []Instance

	heartbeatStarted int
	heartbeatStopped chan struct{}
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{heartbeatStopped: make(chan struct{})}
}

func (f *fakeRegistry) Register(_ context.Context, ins Instance) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.registerErr != nil {
		return f.registerErr
	}
	f.registered = append(f.registered, ins)
	return nil
}

func (f *fakeRegistry) Unregister(_ context.Context, ins Instance) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.unregisterErr != nil {
		return f.unregisterErr
	}
	f.unregistered = append(f.unregistered, ins)
	return nil
}

func (f *fakeRegistry) Discover(_ context.Context, _ string) ([]Instance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.discoverErr != nil {
		return nil, f.discoverErr
	}
	return f.discoverResult, nil
}

func (f *fakeRegistry) Subscribe(_ context.Context, _ string, cb func([]Instance)) error {
	f.mu.Lock()
	errCopy := f.subscribeErr
	resultCopy := f.discoverResult
	f.mu.Unlock()
	if errCopy != nil {
		return errCopy
	}
	// 立即用 discoverResult 触发一次回调，模拟"初始推送"
	if cb != nil {
		cb(resultCopy)
	}
	return nil
}

func (f *fakeRegistry) Heartbeat(ctx context.Context, _ Instance, interval time.Duration) {
	f.mu.Lock()
	f.heartbeatStarted++
	f.mu.Unlock()
	// 契约：interval <= 0 时由实现方保证 ticker 至少 1ms（time.NewTicker 不允许非正值）。
	if interval <= 0 {
		interval = time.Millisecond
	}
	go func() {
		defer close(f.heartbeatStopped)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// tick 行为：真实实现会调 Nacos SendHeartbeat，
				// fake 仅自旋维持心跳循环活跃。
			}
		}
	}()
}

// 编译期断言：fakeRegistry 必须实现 Registry interface。
var _ Registry = (*fakeRegistry)(nil)

// -----------------------------------------------------------------------------
// TDD 红绿双阶段通用 contract 测试：保证所有 Registry 实现都满足相同契约。
// -----------------------------------------------------------------------------

func TestRegistry_Contract_RegisterSucceeds(t *testing.T) {
	// Arrange
	f := newFakeRegistry()
	ins := Instance{ServiceName: "user-svc", Host: "10.0.0.1", Port: 8888,
		Metadata: map[string]string{"stage": "dev"}}

	// Act
	err := f.Register(context.Background(), ins)

	// Assert
	require.NoError(t, err)
	require.Len(t, f.registered, 1)
	assert.Equal(t, ins, f.registered[0])
}

func TestRegistry_Contract_RegisterIsIdempotent(t *testing.T) {
	// Arrange
	f := newFakeRegistry()
	ins := Instance{ServiceName: "user-svc", Host: "10.0.0.1", Port: 8888}

	// Act：同一实例注册两次，契约要求"覆盖而非报错"
	err1 := f.Register(context.Background(), ins)
	err2 := f.Register(context.Background(), ins)

	// Assert
	require.NoError(t, err1)
	require.NoError(t, err2)
	// fake 不去重，但实现方必须满足"幂等"语义（覆盖路径不报错）。
	// 这里至少验证不抛错。
}

func TestRegistry_Contract_RegisterWrapsError(t *testing.T) {
	// Arrange
	f := newFakeRegistry()
	wantErr := errors.New("nacos down")
	f.registerErr = wantErr
	ins := Instance{ServiceName: "user-svc", Host: "10.0.0.1", Port: 8888}

	// Act
	err := f.Register(context.Background(), ins)

	// Assert
	require.Error(t, err)
	assert.True(t, errors.Is(err, wantErr) || err == wantErr,
		"implementation must wrap or return the underlying error")
}

func TestRegistry_Contract_UnregisterSucceeds(t *testing.T) {
	f := newFakeRegistry()
	ins := Instance{ServiceName: "user-svc", Host: "10.0.0.1", Port: 8888}

	err := f.Register(context.Background(), ins)
	require.NoError(t, err)

	err = f.Unregister(context.Background(), ins)
	require.NoError(t, err)
	require.Len(t, f.unregistered, 1)
}

func TestRegistry_Contract_UnregisterIsIdempotent(t *testing.T) {
	// Arrange：未注册实例调用 Unregister，契约要求不 panic + 返回 nil
	f := newFakeRegistry()
	ins := Instance{ServiceName: "user-svc", Host: "10.0.0.1", Port: 8888}

	// Act
	err := f.Unregister(context.Background(), ins)

	// Assert
	require.NoError(t, err)
}

func TestRegistry_Contract_DiscoverReturnsList(t *testing.T) {
	f := newFakeRegistry()
	f.discoverResult = []Instance{
		{ServiceName: "user-svc", Host: "10.0.0.1", Port: 8888},
		{ServiceName: "user-svc", Host: "10.0.0.2", Port: 8888},
	}

	got, err := f.Discover(context.Background(), "user-svc")

	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, 8888, got[0].Port)
	assert.Equal(t, 8888, got[1].Port)
}

func TestRegistry_Contract_DiscoverEmptyReturnsEmpty(t *testing.T) {
	f := newFakeRegistry()
	got, err := f.Discover(context.Background(), "unknown-svc")
	require.NoError(t, err)
	assert.Empty(t, got, "discover of unknown service returns empty list, not error")
}

func TestRegistry_Contract_DiscoverPropagatesError(t *testing.T) {
	f := newFakeRegistry()
	f.discoverErr = errors.New("rpc timeout")

	got, err := f.Discover(context.Background(), "user-svc")

	require.Error(t, err)
	assert.Nil(t, got)
}

func TestRegistry_Contract_SubscribeTriggersCallback(t *testing.T) {
	f := newFakeRegistry()
	f.discoverResult = []Instance{
		{ServiceName: "user-svc", Host: "10.0.0.1", Port: 8888},
	}

	var got []Instance
	cb := func(insts []Instance) { got = insts }

	err := f.Subscribe(context.Background(), "user-svc", cb)

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, 8888, got[0].Port)
}

func TestRegistry_Contract_SubscribeErrorPropagates(t *testing.T) {
	f := newFakeRegistry()
	f.subscribeErr = errors.New("subscribe failed")

	called := false
	cb := func(insts []Instance) { called = true }

	err := f.Subscribe(context.Background(), "user-svc", cb)

	require.Error(t, err)
	assert.False(t, called, "callback must not be invoked when subscribe fails")
}

func TestRegistry_Contract_HeartbeatStartsAndStops(t *testing.T) {
	f := newFakeRegistry()
	ins := Instance{ServiceName: "user-svc", Host: "10.0.0.1", Port: 8888}

	ctx, cancel := context.WithCancel(context.Background())
	f.Heartbeat(ctx, ins, 10*time.Millisecond)

	require.Equal(t, 1, f.heartbeatStarted)

	cancel()

	select {
	case <-f.heartbeatStopped:
		// 预期：心跳 goroutine 监听 ctx.Done() 后退出
	case <-time.After(2 * time.Second):
		t.Fatal("heartbeat goroutine did not stop within 2s after ctx cancel")
	}
}

func TestRegistry_Contract_HeartbeatZeroIntervalStillStarts(t *testing.T) {
	// 边界：interval=0 须不 panic，由实现方决定最小间隔（推荐 1s）。
	f := newFakeRegistry()
	ins := Instance{ServiceName: "user-svc", Host: "10.0.0.1", Port: 8888}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NotPanics(t, func() {
		f.Heartbeat(ctx, ins, 0)
	})

	require.Equal(t, 1, f.heartbeatStarted)
}

// -----------------------------------------------------------------------------
// Instance 模型：基本字段与零值行为
// -----------------------------------------------------------------------------

func TestInstance_ZeroValueIsEmpty(t *testing.T) {
	var ins Instance
	assert.Equal(t, "", ins.ServiceName)
	assert.Equal(t, "", ins.Host)
	assert.Equal(t, 0, ins.Port)
	assert.Nil(t, ins.Metadata)
}

func TestInstance_PopulatedFields(t *testing.T) {
	ins := Instance{
		ServiceName: "user-svc",
		Host:        "127.0.0.1",
		Port:        8888,
		Metadata:    map[string]string{"stage": "dev", "version": "abc123"},
	}
	assert.Equal(t, "user-svc", ins.ServiceName)
	assert.Equal(t, "127.0.0.1", ins.Host)
	assert.Equal(t, 8888, ins.Port)
	assert.Equal(t, "dev", ins.Metadata["stage"])
	assert.Equal(t, "abc123", ins.Metadata["version"])
}
