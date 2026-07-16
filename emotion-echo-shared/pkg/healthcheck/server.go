// Package healthcheck 提供 gRPC 标准 health/v1 协议的封装
//
// 用途：服务发现 / 负载均衡 / K8s 探活 / 启动时自检
//
// 设计：
//   - 复用 grpc-go 自带的 google.golang.org/grpc/health（无代码生成）
//   - 在 grpc_health_v1 之上提供面向业务的薄封装 API
//   - 支持多 service 名独立管理 SERVING / NOT_SERVING / UNKNOWN
//
// 标准规范：
//   https://github.com/grpc/grpc/blob/master/doc/health-checking.md
package healthcheck

import (
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// ServingStatus 是 health/v1 协议的状态枚举
//
// 与 healthpb.HealthCheckResponse_ServingStatus 一一对应，业务侧用本地类型
// 避免直接依赖 grpc-go 内部枚举
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

// toProto 把内部枚举转成 grpc_health_v1 协议枚举
func (s ServingStatus) toProto() healthpb.HealthCheckResponse_ServingStatus {
	switch s {
	case ServingStatusServing:
		return healthpb.HealthCheckResponse_SERVING
	case ServingStatusNotServing:
		return healthpb.HealthCheckResponse_NOT_SERVING
	case ServingStatusServiceUnknown:
		return healthpb.HealthCheckResponse_SERVICE_UNKNOWN
	default:
		return healthpb.HealthCheckResponse_UNKNOWN
	}
}

// fromProto 反向映射
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

// =====================================================
// Server
// =====================================================

// Server 包装 grpc-go 的 health.Server + 自维护 status map
//
// 为什么自维护 map：grpc-go 的 health.Server 内部 statusMap 未导出，
// 业务侧需要 GetServingStatus 做断言式判断，所以自己跟踪一份。
type Server struct {
	inner  *health.Server
	status map[string]ServingStatus
	ready  bool
}

// NewServer 创建一个默认 SERVING 的 health server
//
// 初始时整体服务（空 service 名）置为 SERVING，符合 grpc health 规范的
// "server liveness" 默认语义。
func NewServer() *Server {
	s := &Server{
		inner:  health.NewServer(),
		status: make(map[string]ServingStatus),
	}
	// 默认：空 service（server liveness）置为 SERVING
	s.status[""] = ServingStatusServing
	return s
}

// RegisterWith 把 health service 注册到 *grpc.Server
//
// 注册后 client 可通过 grpc_health_v1.HealthClient.Check / Watch 探活
func (s *Server) RegisterWith(gs *grpc.Server) {
	healthpb.RegisterHealthServer(gs, s.inner)
	if !s.ready {
		// 把内存里的 status 同步给 health.Server
		for svc, st := range s.status {
			s.inner.SetServingStatus(svc, st.toProto())
		}
		s.ready = true
	}
}

// SetServingStatus 设置指定 service 的健康状态
//
// service="" 表示整体 server liveness（grpc 规范约定）
func (s *Server) SetServingStatus(service string, status ServingStatus) {
	s.status[service] = status
	s.inner.SetServingStatus(service, status.toProto())
}

// GetServingStatus 查询本地状态（不走 RPC）
//
// 未注册的 service 返回 ServingStatusUnknown（符合 grpc health spec）
func (s *Server) GetServingStatus(service string) ServingStatus {
	if v, ok := s.status[service]; ok {
		return v
	}
	return ServingStatusUnknown
}

// Shutdown 把所有 service 标记为 NOT_SERVING
//
// 应在 graceful shutdown 时调用，让 client 提前停止发新请求
func (s *Server) Shutdown() {
	for svc := range s.status {
		s.inner.SetServingStatus(svc, healthpb.HealthCheckResponse_NOT_SERVING)
	}
}

// Resume 把所有 service 恢复为 SERVING
//
// 与 Shutdown 配对：暂停后再恢复
func (s *Server) Resume() {
	for svc := range s.status {
		s.inner.SetServingStatus(svc, healthpb.HealthCheckResponse_SERVING)
	}
}
