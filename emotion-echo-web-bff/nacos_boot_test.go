package main

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sharedconfig "github.com/emotion-echo/shared/pkg/configcenter"
	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"

	"emotion-echo-web-bff/internal/config"
	"emotion-echo-web-bff/internal/handler"
)

type fakeRegistry struct {
	mu                sync.Mutex
	registered        []shareddiscovery.Instance
	heartbeatsStarted int
}
func (f *fakeRegistry) Register(_ context.Context, ins shareddiscovery.Instance) error {
	f.mu.Lock()
	defer f.mu.Unlock()
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
	mu       sync.Mutex
	getCalls []getCall
	opsYaml  string // PR-4: 测试用 — 模拟 Nacos 返回的 ops.yaml 内容
}
type getCall struct{ dataId, group string }

func newFakeCC() *fakeConfigCenter { return &fakeConfigCenter{} }

func (f *fakeConfigCenter) GetConfig(_ context.Context, dataId, group string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls = append(f.getCalls, getCall{dataId, group})
	if f.opsYaml != "" {
		return f.opsYaml, nil
	}
	return "", nil
}
func (f *fakeConfigCenter) ListenConfig(context.Context, string, string, sharedconfig.ConfigChangeHandler) error {
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
		Name: "emotion-echo-web-bff", Host: "127.0.0.1", Port: 8894,
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

func TestBootNacos_RegistersWebBffAtPort8894(t *testing.T) {
	reg := &fakeRegistry{}
	cc := newFakeCC()
	rt, err := BootNacos(context.Background(), newTestConfig(), newDeps(reg, cc))
	require.NoError(t, err)
	require.Len(t, reg.registered, 1)
	got := reg.registered[0]
	// web-bff 关键差异：service-name=web-bff（不是 user-svc/chat-svc/...）
	// Stage 32 APISIX nacos-discovery 插件通过此名称自动发现 BFF upstream
	assert.Equal(t, "emotion-echo-web-bff", got.ServiceName)
	assert.Equal(t, 8894, got.Port)
	require.Len(t, cc.getCalls, 1)
	assert.Equal(t, "emotion-echo-web-bff.ops.yaml", cc.getCalls[0].dataId)
	rt.Cancel()
}

// TestBootNacos_AppliesOpsYamlToLimiter：PR-4 — 启动时 GetConfig 返回的 yaml
// 解析到 OpsConfig 并 Update 到 opsLimiter。
func TestBootNacos_AppliesOpsYamlToLimiter(t *testing.T) {
	reg := &fakeRegistry{}
	cc := newFakeCC()
	cc.opsYaml = "limit_count: 30\nburst: 50\n"

	limiter := handler.NewHotReloadLimiter(60, 100)
	deps := newDeps(reg, cc)
	deps.opsLimiter = limiter

	rt, err := BootNacos(context.Background(), newTestConfig(), deps)
	require.NoError(t, err)
	require.NotNil(t, rt)
	rt.Cancel()

	limit, burst := limiter.Snapshot()
	assert.Equal(t, 30, limit, "limiter should reflect ops.yaml limit_count")
	assert.Equal(t, 50, burst, "limiter should reflect ops.yaml burst")
}
