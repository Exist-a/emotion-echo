package discovery

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"
)

// fakeRegistry 记录所有调用 + 注入可控结果。
type fakeRegistry struct {
	mu sync.Mutex

	discovered map[string][]shareddiscovery.Instance
	discoverErr error
}

func newFakeRegistry() *fakeRegistry {
	return &fakeRegistry{discovered: map[string][]shareddiscovery.Instance{}}
}

func (f *fakeRegistry) Register(context.Context, shareddiscovery.Instance) error {
	return nil
}
func (f *fakeRegistry) Unregister(context.Context, shareddiscovery.Instance) error {
	return nil
}
func (f *fakeRegistry) Discover(_ context.Context, svcName string) ([]shareddiscovery.Instance, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.discoverErr != nil {
		return nil, f.discoverErr
	}
	return f.discovered[svcName], nil
}
func (f *fakeRegistry) Subscribe(context.Context, string, func([]shareddiscovery.Instance)) error {
	return nil
}
func (f *fakeRegistry) Heartbeat(context.Context, shareddiscovery.Instance, time.Duration) {}

var _ shareddiscovery.Registry = (*fakeRegistry)(nil)

// TestNacosResolver_ResolveReturnsFirstHealthyInstance：Resolver.Resolve 取第一个实例。
func TestNacosResolver_ResolveReturnsFirstHealthyInstance(t *testing.T) {
	reg := newFakeRegistry()
	reg.discovered[shareddiscovery.ServiceAI] = []shareddiscovery.Instance{
		{ServiceName: shareddiscovery.ServiceAI, Host: "emotion-echo-ai-svc", Port: 8891, Metadata: map[string]string{"stage": "emotion-echo-dev"}},
	}

	r := NewNacosResolver(reg, "emotion-echo-dev")
	host, port, err := r.Resolve(context.Background(), shareddiscovery.ServiceAI)
	require.NoError(t, err)
	assert.Equal(t, "emotion-echo-ai-svc", host)
	assert.Equal(t, 8891, port)
}

// TestNacosResolver_ResolveErrorsWhenNoInstances：无实例 → 明确错误（不 panic）。
func TestNacosResolver_ResolveErrorsWhenNoInstances(t *testing.T) {
	reg := newFakeRegistry()
	r := NewNacosResolver(reg, "emotion-echo-dev")

	_, _, err := r.Resolve(context.Background(), shareddiscovery.ServiceAI)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no healthy instances")
}

// TestNacosResolver_ResolveUsesMetadataGrpcPort：当 metadata.grpc_port 存在，优先用 gRPC 端口。
// ai-svc 是 HTTP + gRPC 双端口场景（HTTP 8891 + gRPC 8892）。
func TestNacosResolver_ResolveUsesMetadataGrpcPort(t *testing.T) {
	reg := newFakeRegistry()
	reg.discovered[shareddiscovery.ServiceAI] = []shareddiscovery.Instance{
		{
			ServiceName: shareddiscovery.ServiceAI, Host: "emotion-echo-ai-svc", Port: 8891,
			Metadata: map[string]string{"grpc_port": "8892"},
		},
	}

	r := NewNacosResolver(reg, "emotion-echo-dev").WithPortHint("grpc_port")
	host, port, err := r.Resolve(context.Background(), shareddiscovery.ServiceAI)
	require.NoError(t, err)
	assert.Equal(t, "emotion-echo-ai-svc", host)
	assert.Equal(t, 8892, port)
}

// TestNacosResolver_ResolvePropagatesDiscoverError: registry 报错 → Resolver 报错，不吞。
func TestNacosResolver_ResolvePropagatesDiscoverError(t *testing.T) {
	reg := newFakeRegistry()
	reg.discoverErr = errors.New("nacos down")
	r := NewNacosResolver(reg, "emotion-echo-dev")

	_, _, err := r.Resolve(context.Background(), shareddiscovery.ServiceAI)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nacos down")
}