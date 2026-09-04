// Package discovery 提供 web-bff 内的服务发现抽象。
//
// PR-2：让 web-bff 真正消费 Nacos。三个 downstream client（ai / chat / analytics）
// 在 BaseURL 未注入时通过 Resolver.Resolve(svcName) 从 Nacos 拉实例。
//
// 设计取舍：
//   - Resolver 是 web-bff 内部 interface，便于单测注入 fake（NacosResolver 是真实实现）
//   - env 注入仍优先生效（向后兼容现有部署）
//   - WithPortHint("grpc_port") 切换端口来源，应对 ai-svc HTTP+gRPC 双端口场景
package discovery

import (
	"context"
	"fmt"
	"strconv"

	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"
)

// Resolver 把 Nacos serviceName 解析成 host:port。
//
// 调用方：downstream.New*(opts) 当 opts.BaseURL == "" 时调 Resolve(ctx, svcName)。
// 失败语义：返回 error，调用方决定 fast-fail 或 fallback（web-bff 选 fast-fail）。
type Resolver interface {
	Resolve(ctx context.Context, serviceName string) (host string, port int, err error)
}

// NacosResolver 通过 Nacos Registry 解析实例。
type NacosResolver struct {
	registry  shareddiscovery.Registry
	namespace string
	// portHint 当 metadata 里有该键时（如 "grpc_port"），优先使用它；
	// 缺省时用 instance.Port。
	portHint string
}

// NewNacosResolver 构造真实 Resolver。
func NewNacosResolver(reg shareddiscovery.Registry, namespace string) *NacosResolver {
	return &NacosResolver{registry: reg, namespace: namespace}
}

// WithPortHint 切换端口来源（链式）。
//
// 用法：r := NewNacosResolver(reg, ns).WithPortHint("grpc_port")
func (r *NacosResolver) WithPortHint(hint string) *NacosResolver {
	r.portHint = hint
	return r
}

// Resolve 取第一个 healthy 实例的 host:port。
//
// 错误：
//   - registry.Discover 报错 → 原样返回
//   - 实例列表为空 → "no healthy instances for <svc>"
func (r *NacosResolver) Resolve(ctx context.Context, serviceName string) (string, int, error) {
	instances, err := r.registry.Discover(ctx, serviceName)
	if err != nil {
		return "", 0, fmt.Errorf("discovery: resolve %s: %w", serviceName, err)
	}
	if len(instances) == 0 {
		return "", 0, fmt.Errorf("discovery: no healthy instances for %s in namespace %s", serviceName, r.namespace)
	}
	ins := instances[0]
	port := ins.Port
	if r.portHint != "" {
		if v, ok := ins.Metadata[r.portHint]; ok {
			if p, perr := strconv.Atoi(v); perr == nil {
				port = p
			}
		}
	}
	return ins.Host, port, nil
}