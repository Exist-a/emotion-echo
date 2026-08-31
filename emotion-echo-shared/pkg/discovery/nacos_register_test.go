package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// -----------------------------------------------------------------------------
// NacosConfig.defaults 单元测试
// -----------------------------------------------------------------------------

func TestNacosConfig_DefaultsApply(t *testing.T) {
	cfg := NacosConfig{ServerAddr: "127.0.0.1:8848"}
	cfg.defaults()

	assert.Equal(t, "DEFAULT_GROUP", cfg.GroupName)
	assert.Equal(t, uint64(5000), cfg.TimeoutMs)
}

func TestNacosConfig_DefaultsPreserveExplicit(t *testing.T) {
	cfg := NacosConfig{ServerAddr: "127.0.0.1:8848", GroupName: "FOO", TimeoutMs: 1000}
	cfg.defaults()

	assert.Equal(t, "FOO", cfg.GroupName, "explicit GroupName must not be overridden")
	assert.Equal(t, uint64(1000), cfg.TimeoutMs, "explicit TimeoutMs must not be overridden")
}

// -----------------------------------------------------------------------------
// buildServerConfigs 单元测试
// -----------------------------------------------------------------------------

func TestBuildServerConfigs_SingleAddr(t *testing.T) {
	out, err := buildServerConfigs("127.0.0.1:8848")
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "127.0.0.1", out[0].IpAddr)
	assert.Equal(t, uint64(8848), out[0].Port)
}

func TestBuildServerConfigs_MultiAddr(t *testing.T) {
	out, err := buildServerConfigs("127.0.0.1:8848, 10.0.0.2:8848, nacos-2:8848")
	require.NoError(t, err)
	require.Len(t, out, 3)
	assert.Equal(t, "127.0.0.1", out[0].IpAddr)
	assert.Equal(t, "10.0.0.2", out[1].IpAddr)
	assert.Equal(t, "nacos-2", out[2].IpAddr)
}

func TestBuildServerConfigs_EmptyAddrFails(t *testing.T) {
	_, err := buildServerConfigs("")
	require.Error(t, err)
}

func TestBuildServerConfigs_InvalidAddrFails(t *testing.T) {
	_, err := buildServerConfigs("not-a-host-port")
	require.Error(t, err)
}

func TestBuildServerConfigs_InvalidPortFails(t *testing.T) {
	_, err := buildServerConfigs("127.0.0.1:not-a-port")
	require.Error(t, err)
}

// -----------------------------------------------------------------------------
// WaitForNacos 行为测试
// -----------------------------------------------------------------------------

func TestWaitForNacos_EmptyAddrFailsImmediately(t *testing.T) {
	err := WaitForNacos(context.Background(), "", 1*time.Second)
	require.Error(t, err)
}

func TestWaitForNacos_InvalidAddrFailsImmediately(t *testing.T) {
	err := WaitForNacos(context.Background(), "no-port", 1*time.Second)
	require.Error(t, err)
}

func TestWaitForNacos_ReachableHostReturns(t *testing.T) {
	// 127.0.0.1:1 通常不会被监听；测试期望返回 error 或成功（取决于本地环境）。
	// 此测试仅验证：函数必然在 deadline 内返回（不无限等待）。
	short, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	var err error
	go func() {
		err = WaitForNacos(short, "127.0.0.1:1", 2*time.Second)
		close(done)
	}()

	select {
	case <-done:
		// 预期：ctx timeout 触发，WaitForNacos 返回 ctx.Err()
		assert.Error(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("WaitForNacos must respect ctx cancellation")
	}
}
