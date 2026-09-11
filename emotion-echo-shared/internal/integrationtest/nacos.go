// Package integrationtest 提供跨包集成测试共享 fixtures。
//
// Stage 31 PR-06：discovery 与 configcenter 包的集成测试共用此处的
// Nacos 容器 starter，避免 import cycle（两个测试包都依赖第三方，
// 但互相不依赖）。
//
// 运行命令：
//   go test -tags=integration ./... -run Integration
//
// 跳过：默认 `go test ./...` 不会执行（build tag）。
//go:build integration
// +build integration

package integrationtest

import (
	"context"
	"net/netip"
	"testing"
	"time"

	mobycontainer "github.com/moby/moby/api/types/container"
	mobynetwork "github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	tcwait "github.com/testcontainers/testcontainers-go/wait"
)

// StartNacos 启动 nacos/nacos-server:v2.4.3 standalone 容器，返回 host / port / cleanup。
//
// 返回的 cleanup 函数必须由调用方 defer 调用（关闭容器）。
//
// Stage 72 修复：SDK v2 用「服务端口+1000」拨 gRPC（8848→9848）。此前 8848 被
// testcontainers 映射到随机宿主端口，SDK 拨「随机端口+1000」时宿主上没有对应
// 映射，gRPC 通道永远 STARTING，Register 必报
// "client not connected, current status:STARTING"（容器间互拨不受影响，
// 所以 dev compose 正常、本 fixture 从未真正通过）。
// 现固定绑定 18848/19848 保持 +1000 偏移。
func StartNacos(t *testing.T) (host, port string, cleanup func()) {
	t.Helper()
	ctx := context.Background()

	req := tc.ContainerRequest{
		Image:        "nacos/nacos-server:v2.4.3",
		ExposedPorts: []string{"8848/tcp", "9848/tcp"},
		HostConfigModifier: func(hc *mobycontainer.HostConfig) {
			p8848, err := mobynetwork.ParsePort("8848/tcp")
			require.NoError(t, err)
			p9848, err := mobynetwork.ParsePort("9848/tcp")
			require.NoError(t, err)
			hc.PortBindings = mobynetwork.PortMap{
				p8848: []mobynetwork.PortBinding{{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: "18848"}},
				p9848: []mobynetwork.PortBinding{{HostIP: netip.MustParseAddr("127.0.0.1"), HostPort: "19848"}},
			}
		},
		Env: map[string]string{
			"MODE":                       "standalone",
			"JVM_XMS":                    "256m",
			"JVM_XMX":                    "512m",
			"SPRING_DATASOURCE_PLATFORM": "derby",
		},
		WaitingFor: tcwait.ForHTTP("/nacos/actuator/health").
			WithPort("8848/tcp").
			WithStartupTimeout(120 * time.Second),
	}

	container, err := tc.GenericContainer(ctx, tc.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "start nacos container")

	host, err = container.Host(ctx)
	require.NoError(t, err)
	mapped, err := container.MappedPort(ctx, "8848/tcp")
	require.NoError(t, err)
	port = mapped.Port()

	cleanup = func() { _ = container.Terminate(context.Background()) }
	return host, port, cleanup
}
