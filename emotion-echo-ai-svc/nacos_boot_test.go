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

	"emotion-echo-ai-svc/internal/config"
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
}
type getCall struct{ dataId, group string }

func newFakeCC() *fakeConfigCenter { return &fakeConfigCenter{} }

func (f *fakeConfigCenter) GetConfig(_ context.Context, dataId, group string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls = append(f.getCalls, getCall{dataId, group})
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
		Name: "emotion-echo-ai-svc", Host: "127.0.0.1", Port: 8891,
		GRPC: config.GRPCServer{Enabled: true, Port: 8892},
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

func TestBootNacos_RegistersAiSvcAtPort8891WithGrpcMetadata(t *testing.T) {
	reg := &fakeRegistry{}
	cc := newFakeCC()
	rt, err := BootNacos(context.Background(), newTestConfig(), newDeps(reg, cc))
	require.NoError(t, err)
	require.Len(t, reg.registered, 1)
	got := reg.registered[0]
	assert.Equal(t, "emotion-echo-ai-svc", got.ServiceName)
	assert.Equal(t, 8891, got.Port, "HTTP port for ai-svc must be 8891")
	// ai-svc 关键差异：metadata.grpc_port=8892 供 Stage 32 APISIX 决策
	assert.Equal(t, "8892", got.Metadata["grpc_port"], "gRPC port must be in metadata for Stage 32")
	require.Len(t, cc.getCalls, 1)
	assert.Equal(t, "emotion-echo-ai-svc.ops.yaml", cc.getCalls[0].dataId)
	rt.Cancel()
}

func TestBootNacos_DisabledGrpcExcludesGrpcPort(t *testing.T) {
	reg := &fakeRegistry{}
	cc := newFakeCC()
	cfg := newTestConfig()
	cfg.GRPC.Enabled = false
	rt, err := BootNacos(context.Background(), cfg, newDeps(reg, cc))
	require.NoError(t, err)
	require.Len(t, reg.registered, 1)
	_, hasGrpcPort := reg.registered[0].Metadata["grpc_port"]
	assert.False(t, hasGrpcPort, "no grpc_port metadata when GRPC.Enabled=false")
	rt.Cancel()
}
