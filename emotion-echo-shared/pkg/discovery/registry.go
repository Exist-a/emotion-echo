// Package discovery 提供服务注册与发现的抽象。
//
// Stage 31 PR-02（🔴 RED）：仅定义 Registry 接口契约与 Instance 模型，
// 实现由 nacos_register.go（PR-03）提供。
//
// 设计原则：
//   - interface 严格 1:1 暴露 5 个方法，便于 mock 与替换实现
//   - 不引入 go-zero v1 plugin（v1 plugin 与 nacos-sdk-go/v2 不兼容）
//   - 调用方（svc main.go）仅依赖此 interface，不接触 Nacos SDK 类型
package discovery

import (
	"context"
	"time"
)

// Instance 是注册到注册中心的单个服务实例。
//
// ServiceName + Host + Port 三元组唯一确定一个实例；
// Metadata 携带 stage / version / grpc_port 等运维标签。
type Instance struct {
	ServiceName string
	Host        string
	Port        int
	Metadata    map[string]string
}

// Registry 是服务注册中心抽象。
//
// 实现方须保证：
//   - Register 幂等：同 (ServiceName, Host, Port) 重复注册应覆盖而非报错
//   - Unregister 幂等：未注册的实例注销不应 panic，返回 nil
//   - Heartbeat 启动 goroutine 后由 ctx cancel 优雅退出
//   - 所有方法均接受 context，便于超时控制与链路追踪
type Registry interface {
	// Register 将实例注册到注册中心；返回 nil 表示成功。
	Register(ctx context.Context, ins Instance) error

	// Unregister 将实例从注册中心注销；返回 nil 表示成功。
	Unregister(ctx context.Context, ins Instance) error

	// Discover 拉取指定 serviceName 的全部健康实例。
	// 返回的列表可能为空（表示该服务尚无在线实例）。
	Discover(ctx context.Context, serviceName string) ([]Instance, error)

	// Subscribe 订阅 serviceName 的实例变更；变更通过 callback 推送。
	// callback 可能在 Subscribe 调用本身所在 goroutine 中执行，
	// 也可能在独立 goroutine 中执行；调用方须自行保证并发安全。
	Subscribe(ctx context.Context, serviceName string, cb func([]Instance)) error

	// Heartbeat 启动后台心跳 goroutine，按 interval 周期续约；
	// ctx.Done() 时退出 goroutine 并关闭底层连接。
	Heartbeat(ctx context.Context, ins Instance, interval time.Duration)
}
