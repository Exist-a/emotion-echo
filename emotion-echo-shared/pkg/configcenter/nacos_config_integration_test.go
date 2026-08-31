//go:build integration
// +build integration

// Package configcenter 集成测试：真实启动 Nacos 容器验证 SDK ConfigClient 路径。
//
// 运行命令：
//   go test -tags=integration ./pkg/configcenter -run Integration -v
//
// 跳过命令：默认 `go test ./...` 不会执行本文件（build tag 限制）。
package configcenter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/emotion-echo/shared/internal/integrationtest"
	"github.com/emotion-echo/shared/pkg/discovery"
)

// TestNacosConfig_PublishAndListen_Integration 真实发布配置并验证监听回调触发。
func TestNacosConfig_PublishAndListen_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skip integration test in -short mode")
	}
	host, port, cleanup := integrationtest.StartNacos(t)
	defer cleanup()

	addr := host + ":" + port
	cfg := discovery.NacosConfig{
		ServerAddr: addr,
		Namespace:  "emotion-echo-dev",
		GroupName:  "DEFAULT_GROUP",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cc, err := NewNacosConfig(ctx, cfg)
	require.NoError(t, err)
	defer cc.Close()

	dataId := "user-svc.ops.yaml"
	const expected = "feature_flags:\n  new_chat_ui: true\n"

	require.NoError(t, cc.PublishConfig(ctx, dataId, "DEFAULT_GROUP", expected))

	got, err := cc.GetConfig(ctx, dataId, "DEFAULT_GROUP")
	require.NoError(t, err)
	require.Equal(t, expected, got)

	// ListenConfig + 再次发布验证回调
	received := make(chan string, 1)
	require.NoError(t, cc.ListenConfig(ctx, dataId, "DEFAULT_GROUP",
		func(_, _, content string) error {
			select {
			case received <- content:
			default:
			}
			return nil
		}))

	const v2 = "feature_flags:\n  new_chat_ui: false\n"
	require.NoError(t, cc.PublishConfig(ctx, dataId, "DEFAULT_GROUP", v2))

	select {
	case got := <-received:
		require.Equal(t, v2, got)
	case <-time.After(10 * time.Second):
		t.Fatal("ListenConfig callback did not fire within 10s")
	}
}

// TestNacosConfig_PublishSensitiveDataId_Integration 验证 NacosConfig 拒绝推送敏感 dataId。
//
// 即使连真 Nacos，敏感前缀也应被包级安全约束拦下。
func TestNacosConfig_PublishSensitiveDataId_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skip integration test in -short mode")
	}
	host, port, cleanup := integrationtest.StartNacos(t)
	defer cleanup()

	addr := host + ":" + port
	cfg := discovery.NacosConfig{
		ServerAddr: addr,
		Namespace:  "emotion-echo-dev",
		GroupName:  "DEFAULT_GROUP",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cc, err := NewNacosConfig(ctx, cfg)
	require.NoError(t, err)
	defer cc.Close()

	err = cc.PublishConfig(ctx, "jwt.secret", "DEFAULT_GROUP", "leaked-token")
	require.Error(t, err, "sensitive dataId must be refused")
	require.Contains(t, err.Error(), "sensitive")
}

// TestNacosConfig_GetConfigEmptyWhenAbsent_Integration 验证未发布的 dataId GetConfig 返回空。
func TestNacosConfig_GetConfigEmptyWhenAbsent_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skip integration test in -short mode")
	}
	host, port, cleanup := integrationtest.StartNacos(t)
	defer cleanup()

	addr := host + ":" + port
	cfg := discovery.NacosConfig{
		ServerAddr: addr,
		Namespace:  "emotion-echo-dev",
		GroupName:  "DEFAULT_GROUP",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cc, err := NewNacosConfig(ctx, cfg)
	require.NoError(t, err)
	defer cc.Close()

	got, err := cc.GetConfig(ctx, "absent.ops.yaml", "DEFAULT_GROUP")
	require.NoError(t, err)
	require.Equal(t, "", got, "absent config returns empty string + nil error")
}
