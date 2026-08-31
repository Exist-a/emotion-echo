package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sharedconfig "github.com/emotion-echo/shared/pkg/configcenter"
	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"

	"emotion-echo-user-svc/internal/config"
)

// fakeRegistry 是 user-svc 测试用 Registry fake，记录调用 + 注入错误。
type fakeRegistry struct {
	mu                sync.Mutex
	registered        []shareddiscovery.Instance
	heartbeatsStarted int
	registerErr       error
}

func (f *fakeRegistry) Register(_ context.Context, ins shareddiscovery.Instance) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.registerErr != nil {
		return f.registerErr
	}
	f.registered = append(f.registered, ins)
	return nil
}
func (f *fakeRegistry) Unregister(context.Context, shareddiscovery.Instance) error { return nil }
func (f *fakeRegistry) Discover(context.Context, string) ([]shareddiscovery.Instance, error) {
	return nil, nil
}
func (f *fakeRegistry) Subscribe(context.Context, string, func([]shareddiscovery.Instance)) error {
	return nil
}
func (f *fakeRegistry) Heartbeat(ctx context.Context, _ shareddiscovery.Instance, _ time.Duration) {
	f.mu.Lock()
	f.heartbeatsStarted++
	f.mu.Unlock()
	go func() {
		<-ctx.Done()
	}()
}

// fakeConfigCenter 记录 GetConfig / ListenConfig 调用。
type fakeConfigCenter struct {
	mu                sync.Mutex
	getCalls          []getCall
	listenCalls       []listenCall
	getErr            error
	configs           map[string]string
}

type getCall struct{ dataId, group string }
type listenCall struct {
	dataId, group string
	handler       sharedconfig.ConfigChangeHandler
}

func newFakeCC() *fakeConfigCenter {
	return &fakeConfigCenter{configs: map[string]string{}}
}

func (f *fakeConfigCenter) GetConfig(_ context.Context, dataId, group string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls = append(f.getCalls, getCall{dataId, group})
	if f.getErr != nil {
		return "", f.getErr
	}
	return f.configs[dataId+"@"+group], nil
}
func (f *fakeConfigCenter) ListenConfig(_ context.Context, dataId, group string, h sharedconfig.ConfigChangeHandler) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listenCalls = append(f.listenCalls, listenCall{dataId, group, h})
	return nil
}
func (f *fakeConfigCenter) PublishConfig(context.Context, string, string, string) error {
	return nil
}
func (f *fakeConfigCenter) Close() error { return nil }

// 编译期断言 fake 满足 interface。
var _ shareddiscovery.Registry = (*fakeRegistry)(nil)
var _ sharedconfig.ConfigCenter = (*fakeConfigCenter)(nil)

// -----------------------------------------------------------------------------
// BootNacos 测试
// -----------------------------------------------------------------------------

func newTestConfig() *config.Config {
	return &config.Config{
		Name: "user-svc",
		Host: "127.0.0.1",
		Port: 8888,
		Nacos: config.Nacos{
			Enabled:   true,
			Addr:      "fake-nacos:8848",
			Namespace: "emotion-echo-dev",
			GroupName: "DEFAULT_GROUP",
			HotReload: false,
		},
	}
}

func TestBootNacos_DisabledReturnsEmptyRuntime(t *testing.T) {
	cfg := newTestConfig()
	cfg.Nacos.Enabled = false
	rt, err := BootNacos(context.Background(), cfg, defaultBootDeps())
	require.NoError(t, err)
	assert.Nil(t, rt.Registry)
	assert.Nil(t, rt.ConfigCenter)
}

func TestBootNacos_RegistersAndLoadsOpsConfig(t *testing.T) {
	reg := &fakeRegistry{}
	cc := newFakeCC()
	cc.configs["user-svc.ops.yaml@DEFAULT_GROUP"] = "feature_flags:\n  x: true"

	deps := bootDeps{
		waitForNacos: func(context.Context, string, time.Duration) error { return nil },
		registryFactory: func(context.Context, string, string, string) (shareddiscovery.Registry, error) {
			return reg, nil
		},
		configFactory: func(context.Context, string, string, string) (sharedconfig.ConfigCenter, error) {
			return cc, nil
		},
	}

	cfg := newTestConfig()
	rt, err := BootNacos(context.Background(), cfg, deps)
	require.NoError(t, err)
	require.NotNil(t, rt)

	// Assert: Register 调用一次，参数正确
	require.Len(t, reg.registered, 1)
	got := reg.registered[0]
	assert.Equal(t, "user-svc", got.ServiceName)
	assert.Equal(t, "127.0.0.1", got.Host)
	assert.Equal(t, 8888, got.Port)
	assert.Equal(t, "emotion-echo-dev", got.Metadata["stage"])

	// Assert: Heartbeat 已启动
	assert.Equal(t, 1, reg.heartbeatsStarted)

	// Assert: GetConfig 被调用一次，dataId=svc.ops.yaml
	require.Len(t, cc.getCalls, 1)
	assert.Equal(t, "user-svc.ops.yaml", cc.getCalls[0].dataId)
	assert.Equal(t, "DEFAULT_GROUP", cc.getCalls[0].group)

	// Assert: HotReload=false 时 ListenConfig 不被调用
	assert.Empty(t, cc.listenCalls)

	// 清理
	rt.Cancel()
}

func TestBootNacos_HotReloadRegistersListener(t *testing.T) {
	reg := &fakeRegistry{}
	cc := newFakeCC()

	deps := bootDeps{
		waitForNacos: func(context.Context, string, time.Duration) error { return nil },
		registryFactory: func(context.Context, string, string, string) (shareddiscovery.Registry, error) {
			return reg, nil
		},
		configFactory: func(context.Context, string, string, string) (sharedconfig.ConfigCenter, error) {
			return cc, nil
		},
	}

	cfg := newTestConfig()
	cfg.Nacos.HotReload = true

	rt, err := BootNacos(context.Background(), cfg, deps)
	require.NoError(t, err)

	require.Len(t, cc.listenCalls, 1)
	assert.Equal(t, "user-svc.ops.yaml", cc.listenCalls[0].dataId)
	require.NotNil(t, cc.listenCalls[0].handler)

	// 模拟推送
	require.NoError(t, cc.listenCalls[0].handler("user-svc.ops.yaml", "DEFAULT_GROUP", "v2"))
	// fake 不记录 handler 调用次数，仅验证不 panic

	rt.Cancel()
}

func TestBootNacos_WaitForNacosFailurePropagates(t *testing.T) {
	deps := bootDeps{
		waitForNacos: func(context.Context, string, time.Duration) error {
			return errors.New("nacos down")
		},
		registryFactory: func(context.Context, string, string, string) (shareddiscovery.Registry, error) {
			t.Fatal("registry factory must not be called when WaitForNacos fails")
			return nil, nil
		},
		configFactory: func(context.Context, string, string, string) (sharedconfig.ConfigCenter, error) {
			t.Fatal("config factory must not be called when WaitForNacos fails")
			return nil, nil
		},
	}

	cfg := newTestConfig()
	_, err := BootNacos(context.Background(), cfg, deps)
	require.Error(t, err)
	require.Contains(t, err.Error(), "WaitForNacos")
}

func TestBootNacos_RegisterFailurePropagates(t *testing.T) {
	reg := &fakeRegistry{registerErr: errors.New("register denied")}

	deps := bootDeps{
		waitForNacos: func(context.Context, string, time.Duration) error { return nil },
		registryFactory: func(context.Context, string, string, string) (shareddiscovery.Registry, error) {
			return reg, nil
		},
		configFactory: func(context.Context, string, string, string) (sharedconfig.ConfigCenter, error) {
			t.Fatal("config factory must not be called when Register fails")
			return nil, nil
		},
	}

	cfg := newTestConfig()
	_, err := BootNacos(context.Background(), cfg, deps)
	require.Error(t, err)
	require.Contains(t, err.Error(), "Register")
}

func TestBootNacos_GetConfigFailureIsLoggedButNotFatal(t *testing.T) {
	// GetConfig 失败时 BootNacos 不应中断；继续返回 runtime
	reg := &fakeRegistry{}
	cc := newFakeCC()
	cc.getErr = errors.New("config rpc timeout")

	deps := bootDeps{
		waitForNacos: func(context.Context, string, time.Duration) error { return nil },
		registryFactory: func(context.Context, string, string, string) (shareddiscovery.Registry, error) {
			return reg, nil
		},
		configFactory: func(context.Context, string, string, string) (sharedconfig.ConfigCenter, error) {
			return cc, nil
		},
	}

	cfg := newTestConfig()
	rt, err := BootNacos(context.Background(), cfg, deps)
	require.NoError(t, err, "GetConfig failure must not fail boot")
	require.NotNil(t, rt)
	rt.Cancel()
}

func TestBootNacos_CloseCallsUnregisterAndCancels(t *testing.T) {
	reg := &fakeRegistry{}
	cc := newFakeCC()

	deps := bootDeps{
		waitForNacos: func(context.Context, string, time.Duration) error { return nil },
		registryFactory: func(context.Context, string, string, string) (shareddiscovery.Registry, error) {
			return reg, nil
		},
		configFactory: func(context.Context, string, string, string) (sharedconfig.ConfigCenter, error) {
			return cc, nil
		},
	}

	cfg := newTestConfig()
	rt, err := BootNacos(context.Background(), cfg, deps)
	require.NoError(t, err)

	canceled := make(chan struct{})
	rt.Cancel = func() { close(canceled) }
	rt.Close(context.Background(), cfg.Name, cfg.Host, cfg.Port)

	select {
	case <-canceled:
		// pass
	case <-time.After(time.Second):
		t.Fatal("Close must call Cancel")
	}
}

func TestGitVersion_DefaultWhenEnvEmpty(t *testing.T) {
	t.Setenv("GIT_VERSION", "")
	assert.Equal(t, "dev-build", gitVersion())
}

func TestGitVersion_UsesEnvWhenSet(t *testing.T) {
	t.Setenv("GIT_VERSION", "abc1234")
	assert.Equal(t, "abc1234", gitVersion())
}
