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

	"emotion-echo-chat-svc/internal/config"
)

// fakeRegistry / fakeConfigCenter 与 user-svc 同构（PR-07 模板）；
// Stage 31 后续 PR-12 之后会抽到 shared/pkg/nacosboot 复用。
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
	go func() { <-ctx.Done() }()
}

type fakeConfigCenter struct {
	mu          sync.Mutex
	getCalls    []getCall
	listenCalls []listenCall
	getErr      error
	configs     map[string]string
}

type getCall struct{ dataId, group string }
type listenCall struct {
	dataId, group string
	handler       sharedconfig.ConfigChangeHandler
}

func newFakeCC() *fakeConfigCenter { return &fakeConfigCenter{configs: map[string]string{}} }

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

var _ shareddiscovery.Registry = (*fakeRegistry)(nil)
var _ sharedconfig.ConfigCenter = (*fakeConfigCenter)(nil)

func newTestConfig() *config.Config {
	return &config.Config{
		Name: "chat-svc",
		Host: "127.0.0.1",
		Port: 8890,
		Nacos: config.Nacos{
			Enabled: true, Addr: "fake-nacos:8848",
			Namespace: "emotion-echo-dev", GroupName: "DEFAULT_GROUP",
		},
	}
}

func newDeps(reg *fakeRegistry, cc *fakeConfigCenter) bootDeps {
	return bootDeps{
		waitForNacos: func(context.Context, string, time.Duration) error { return nil },
		registryFactory: func(context.Context, string, string, string) (shareddiscovery.Registry, error) {
			return reg, nil
		},
		configFactory: func(context.Context, string, string, string) (sharedconfig.ConfigCenter, error) {
			return cc, nil
		},
	}
}

func TestBootNacos_DisabledReturnsEmptyRuntime(t *testing.T) {
	cfg := newTestConfig()
	cfg.Nacos.Enabled = false
	rt, err := BootNacos(context.Background(), cfg, defaultBootDeps())
	require.NoError(t, err)
	assert.Nil(t, rt.Registry)
}

func TestBootNacos_RegistersChatSvcAtPort8890(t *testing.T) {
	reg := &fakeRegistry{}
	cc := newFakeCC()

	cfg := newTestConfig()
	rt, err := BootNacos(context.Background(), cfg, newDeps(reg, cc))
	require.NoError(t, err)
	require.NotNil(t, rt)

	// chat-svc 关键差异：service-name + port
	require.Len(t, reg.registered, 1)
	got := reg.registered[0]
	assert.Equal(t, "chat-svc", got.ServiceName)
	assert.Equal(t, 8890, got.Port)
	assert.Equal(t, "emotion-echo-dev", got.Metadata["stage"])
	assert.Equal(t, 1, reg.heartbeatsStarted)

	// GetConfig dataId 是 chat-svc.ops.yaml（不是 user-svc.ops.yaml）
	require.Len(t, cc.getCalls, 1)
	assert.Equal(t, "chat-svc.ops.yaml", cc.getCalls[0].dataId)

	rt.Cancel()
}

func TestBootNacos_WaitForNacosFailurePropagates(t *testing.T) {
	deps := bootDeps{
		waitForNacos: func(context.Context, string, time.Duration) error {
			return errors.New("nacos down")
		},
		registryFactory: func(context.Context, string, string, string) (shareddiscovery.Registry, error) {
			t.Fatal("must not be called"); return nil, nil
		},
		configFactory: func(context.Context, string, string, string) (sharedconfig.ConfigCenter, error) {
			t.Fatal("must not be called"); return nil, nil
		},
	}
	_, err := BootNacos(context.Background(), newTestConfig(), deps)
	require.Error(t, err)
	require.Contains(t, err.Error(), "WaitForNacos")
}

func TestBootNacos_RegisterFailurePropagates(t *testing.T) {
	reg := &fakeRegistry{registerErr: errors.New("denied")}
	cc := newFakeCC()
	_, err := BootNacos(context.Background(), newTestConfig(), newDeps(reg, cc))
	require.Error(t, err)
	require.Contains(t, err.Error(), "Register")
}

func TestBootNacos_HotReloadRegistersListener(t *testing.T) {
	reg := &fakeRegistry{}
	cc := newFakeCC()
	cfg := newTestConfig()
	cfg.Nacos.HotReload = true

	rt, err := BootNacos(context.Background(), cfg, newDeps(reg, cc))
	require.NoError(t, err)
	require.Len(t, cc.listenCalls, 1)
	assert.Equal(t, "chat-svc.ops.yaml", cc.listenCalls[0].dataId)
	require.NotNil(t, cc.listenCalls[0].handler)
	// 验证回调不 panic
	require.NoError(t, cc.listenCalls[0].handler("chat-svc.ops.yaml", "DEFAULT_GROUP", "v2"))
	rt.Cancel()
}
