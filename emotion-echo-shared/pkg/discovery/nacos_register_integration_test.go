//go:build integration
// +build integration

// Package discovery 集成测试：真实启动 Nacos 容器验证 SDK RPC 路径。
//
// 运行命令：
//   go test -tags=integration ./pkg/discovery -run Integration -v
//
// 跳过命令：默认 `go test ./...` 不会执行本文件（build tag 限制）。
//
// 覆盖目标：把单测无法覆盖的 Nacos SDK RPC 路径（Register / Discover /
// Heartbeat）从 ~30% 单测覆盖率提升到 ≥ 70%（AGENTS.md §2.3 三方适配层底线）。
package discovery

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/emotion-echo/shared/internal/integrationtest"
)

// TestNacosRegistry_RegisterAndDiscover_Integration 真实启动 Nacos 容器，
// 注册一个实例，验证 Discover 能拉回。
func TestNacosRegistry_RegisterAndDiscover_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skip integration test in -short mode")
	}
	host, port, cleanup := integrationtest.StartNacos(t)
	defer cleanup()

	addr := host + ":" + port
	cfg := NacosConfig{
		ServerAddr: addr,
		Namespace:  "emotion-echo-dev",
		GroupName:  "DEFAULT_GROUP",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reg, err := NewNacosRegistry(ctx, cfg)
	require.NoError(t, err)

	ins := Instance{
		ServiceName: "user-svc",
		Host:        "127.0.0.1",
		Port:        8888,
		Metadata:    map[string]string{"stage": "dev", "version": "integration-test"},
	}
	require.NoError(t, reg.Register(ctx, ins))
	defer func() { _ = reg.Unregister(ctx, ins) }()

	// Nacos 内部对 RegisterInstance 有秒级索引延迟，等待并轮询 Discover。
	var found []Instance
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		found, err = reg.Discover(ctx, "user-svc")
		require.NoError(t, err)
		if len(found) >= 1 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	require.GreaterOrEqual(t, len(found), 1, "registered instance must be discoverable within 15s")
	require.Equal(t, "user-svc", found[0].ServiceName)
	require.Equal(t, 8888, found[0].Port)
	require.Equal(t, "dev", found[0].Metadata["stage"])
}

// TestNacosRegistry_HeartbeatKeepsInstanceAlive_Integration 验证 Heartbeat goroutine
// 能让实例在 SDK 5s 内部心跳间隔下保持 healthy。
func TestNacosRegistry_HeartbeatKeepsInstanceAlive_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skip integration test in -short mode")
	}
	host, port, cleanup := integrationtest.StartNacos(t)
	defer cleanup()

	addr := host + ":" + port
	cfg := NacosConfig{
		ServerAddr: addr,
		Namespace:  "emotion-echo-dev",
		GroupName:  "DEFAULT_GROUP",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reg, err := NewNacosRegistry(ctx, cfg)
	require.NoError(t, err)

	ins := Instance{
		ServiceName: "heartbeat-svc",
		Host:        "127.0.0.1",
		Port:        9999,
	}
	require.NoError(t, reg.Register(ctx, ins))
	defer func() { _ = reg.Unregister(ctx, ins) }()

	hbCtx, hbCancel := context.WithCancel(ctx)
	defer hbCancel()
	reg.Heartbeat(hbCtx, ins, 2*time.Second)

	// 等 8s 后实例应仍然可发现（SDK 心跳续约生效）
	time.Sleep(8 * time.Second)
	found, err := reg.Discover(ctx, "heartbeat-svc")
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(found), 1, "instance must remain discoverable during heartbeat")
}

// TestNacosRegistry_UnregisterRemovesInstance_Integration 验证 Unregister 后实例不再可发现。
func TestNacosRegistry_UnregisterRemovesInstance_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skip integration test in -short mode")
	}
	host, port, cleanup := integrationtest.StartNacos(t)
	defer cleanup()

	addr := host + ":" + port
	cfg := NacosConfig{
		ServerAddr: addr,
		Namespace:  "emotion-echo-dev",
		GroupName:  "DEFAULT_GROUP",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	reg, err := NewNacosRegistry(ctx, cfg)
	require.NoError(t, err)

	ins := Instance{
		ServiceName: "ephemeral-svc",
		Host:        "127.0.0.1",
		Port:        7777,
	}
	require.NoError(t, reg.Register(ctx, ins))

	// 等待 Discover 可见
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		found, _ := reg.Discover(ctx, "ephemeral-svc")
		if len(found) >= 1 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}

	require.NoError(t, reg.Unregister(ctx, ins))

	// Unregister 后 5s 内 Discover 应返回空
	deadline = time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		found, _ := reg.Discover(ctx, "ephemeral-svc")
		if len(found) == 0 {
			return // pass
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatal("instance must be removed from discovery within 10s after Unregister")
}
