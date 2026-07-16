// healthcheck client 部分

package healthcheck

import (
	"context"
	"fmt"
	"time"

	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc"
)

// Client 是 health/v1 协议的 client 封装
//
// 通过 *grpc.ClientConn 复用底层连接（避免重复 dial）
type Client struct {
	inner healthpb.HealthClient
}

// NewClient 创建 health client
//
// conn 应复用长连接（推荐 grpc.Dial 后全局共享）
func NewClient(conn *grpc.ClientConn) *Client {
	return &Client{
		inner: healthpb.NewHealthClient(conn),
	}
}

// Check 单次探活
//
// 返回：
//   - ServingStatus：服务端报告的状态
//   - error：网络错误 / 超时 / NOT_FOUND
//
// 注意：service 不存在时 grpc.health.Server 默认返回 ServiceUnknown + nil error；
//       server 关闭时返回 NotFound 错误
func (c *Client) Check(ctx context.Context, service string) (ServingStatus, error) {
	resp, err := c.inner.Check(ctx, &healthpb.HealthCheckRequest{Service: service})
	if err != nil {
		// server 端 grpc.health.Server 对未注册 service 返回 SERVICE_UNKNOWN + nil，
		// 但有些 server 实现会返回 NotFound 错误。统一映射为 ServiceUnknown。
		return ServingStatusServiceUnknown, err
	}
	return fromProto(resp.GetStatus()), nil
}

// WaitForReady 阻塞等待 service 进入 SERVING 状态
//
// 行为：
//   - 每 interval poll 一次（默认 200ms）
//   - 一旦 SERVING 或 context 超时则返回
//   - timeout=0 表示无限等待（受 ctx 控制）
//
// 适用于：服务启动时探活下游、CI 等待依赖就绪
func (c *Client) WaitForReady(ctx context.Context, service string, timeout time.Duration) error {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// 第一次探活：直接复用 Check
	st, err := c.Check(ctx, service)
	if err == nil && st == ServingStatusServing {
		return nil
	}

	// 轮询：每 200ms 一次，直到 SERVING 或 ctx 超时
	const pollInterval = 200 * time.Millisecond
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// 首次立即重试一次（避免 ticker 第一跳 200ms 延迟）
	st, err = c.Check(ctx, service)
	if err == nil && st == ServingStatusServing {
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for %q ready: %w (last status=%v)", service, ctx.Err(), st)
		case <-ticker.C:
			st, err = c.Check(ctx, service)
			if err == nil && st == ServingStatusServing {
				return nil
			}
		}
	}
}
