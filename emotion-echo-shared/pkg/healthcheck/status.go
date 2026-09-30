// Package healthcheck 提供 gRPC 标准 health/v1 协议的 **client** 封装
//
// 用途：服务发现 / 负载均衡 / K8s 探活 / 启动时自检
//
// 设计（2026-09-30 E2E-23 F-163 更正后）：
//
//	本包**只提供 client 侧**。server 侧一律直接用上游
//	google.golang.org/grpc/health —— 5 个 Go 服务的
//	internal/grpcserver/server.go 就是这么做的（并以 MarkShuttingDown()
//	在停机时翻 NOT_SERVING）。
//
//	本包曾有一个 server 侧包装（Server / NewServer / RegisterWith /
//	SetServingStatus / GetServingStatus / Shutdown / Resume），
//	**在生产路径上一次都没有被实例化** —— 全仓只有它自己包的测试 new 过它。
//	plan B1 想要的"停机翻 NOT_SERVING"语义早已经由上游 health server 达成，
//	那个包装纯属 AP-10「孤儿产出物」，且更坏的是它与 5 个服务实际使用的
//	上游实现形成**两套并存的健康语义**，排障时会查错地方。
//	已于 2026-09-30 删除，回归钉：scripts/test_healthcheck_no_dead_server.sh
//
// 标准规范：
//
//	https://github.com/grpc/grpc/blob/master/doc/health-checking.md
package healthcheck

import (
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// ServingStatus 是 health/v1 协议的状态枚举
//
// 与 healthpb.HealthCheckResponse_ServingStatus 一一对应，业务侧用本地类型
// 避免直接依赖 grpc-go 内部枚举。
//
// 本类型是 server 侧包装被删后**仅存的、与 health 协议相关的导出符号**，
// 被 Client.Check / WaitForReady 的返回值与全部调用方依赖 —— 不要删。
type ServingStatus int32

const (
	// ServingStatusUnknown 服务状态未知（默认）
	ServingStatusUnknown ServingStatus = 0
	// ServingStatusServing 服务正常
	ServingStatusServing ServingStatus = 1
	// ServingStatusNotServing 服务不可用
	ServingStatusNotServing ServingStatus = 2
	// ServingStatusServiceUnknown 服务不存在
	ServingStatusServiceUnknown ServingStatus = 3
)

// fromProto 反向映射：协议枚举 → 本地枚举
//
// 这是 server 侧包装被删后**唯一还在使用的映射方向**（Client.Check 收协议
// 响应后调用它）。反方向 toProto() 原先只被已删除的 Server 方法使用，
// 一并删除，避免留下无调用方的孤儿。
func fromProto(p healthpb.HealthCheckResponse_ServingStatus) ServingStatus {
	switch p {
	case healthpb.HealthCheckResponse_SERVING:
		return ServingStatusServing
	case healthpb.HealthCheckResponse_NOT_SERVING:
		return ServingStatusNotServing
	case healthpb.HealthCheckResponse_SERVICE_UNKNOWN:
		return ServingStatusServiceUnknown
	default:
		return ServingStatusUnknown
	}
}
