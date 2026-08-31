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
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	tc "github.com/testcontainers/testcontainers-go"
	tcwait "github.com/testcontainers/testcontainers-go/wait"
)

// StartNacos 启动 nacos/nacos-server:v2.4.3 standalone 容器，返回 host / port / cleanup。
//
// 返回的 cleanup 函数必须由调用方 defer 调用（关闭容器）。
func StartNacos(t *testing.T) (host, port string, cleanup func()) {
	t.Helper()
	ctx := context.Background()

	req := tc.ContainerRequest{
		Image:        "nacos/nacos-server:v2.4.3",
		ExposedPorts: []string{"8848/tcp", "9848/tcp"},
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
