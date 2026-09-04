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

	"emotion-echo-assessment-svc/internal/config"
)

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
		Name: "emotion-echo-assessment-svc", Host: "127.0.0.1", Port: 8889,
		Nacos: config.Nacos{Enabled: true, Addr: "fake:8848", Namespace: "emotion-echo-dev", GroupName: "DEFAULT_GROUP"},
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

func TestBootNacos_RegistersAssessmentSvcAtPort8889(t *testing.T) {
	reg := &fakeRegistry{}
	cc := newFakeCC()
	rt, err := BootNacos(context.Background(), newTestConfig(), newDeps(reg, cc))
	require.NoError(t, err)
	require.Len(t, reg.registered, 1)
	got := reg.registered[0]
	assert.Equal(t, "emotion-echo-assessment-svc", got.ServiceName)
	assert.Equal(t, 8889, got.Port)
	require.Len(t, cc.getCalls, 1)
	assert.Equal(t, "emotion-echo-assessment-svc.ops.yaml", cc.getCalls[0].dataId)
	rt.Cancel()
}

func TestBootNacos_WaitForNacosFailurePropagates(t *testing.T) {
	deps := bootDeps{waitForNacos: func(context.Context, string, time.Duration) error { return errors.New("down") }}
	_, err := BootNacos(context.Background(), newTestConfig(), deps)
	require.Error(t, err)
}

func TestBootNacos_HotReloadRegistersListener(t *testing.T) {
	reg := &fakeRegistry{}
	cc := newFakeCC()
	cfg := newTestConfig()
	cfg.Nacos.HotReload = true
	rt, err := BootNacos(context.Background(), cfg, newDeps(reg, cc))
	require.NoError(t, err)
	require.Len(t, cc.listenCalls, 1)
	require.NoError(t, cc.listenCalls[0].handler("emotion-echo-assessment-svc.ops.yaml", "DEFAULT_GROUP", "v2"))
	rt.Cancel()
}
