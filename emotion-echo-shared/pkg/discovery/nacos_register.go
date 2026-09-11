package discovery

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	nacosclients "github.com/nacos-group/nacos-sdk-go/v2/clients"
	nacosconstant "github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	nacosmodel "github.com/nacos-group/nacos-sdk-go/v2/model"
	nacosnaming "github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	nacosvo "github.com/nacos-group/nacos-sdk-go/v2/vo"
)

// defaultRegisterEphemeral 控制 Register() 的 Ephemeral 字段默认值。
//
// Stage-52 修复（revert PR-1 错改）：dev 默认 ephemeral=true（SDK 原生行为）。
//
// 历史错误：
//   - PR-1 (2026-09-08 之前) 把默认值改为 false（persistent），假设 ephemeral 实例
//     30s 内被 Derby 启动慢场景踢出。
//   - 实测 (2026-09-08): Nacos 2.4.3 standalone Derby 环境下，service 一旦被标为 persistent
//     （无论怎么来的——curl、SDK 旧版、甚至 admin API），后续 SDK 注册 ephemeral instance
//     时一律返 400/500 "can't register ephemeral instance, service is persistent"。
//   - 5 个 dev svc (chat/user/ai/analytics/assessment) 因 0.0.0.0 + ephemeral SDK 调用 +
//     已被 persistent 创建的 serviceName → Register 重试 3 次 fatal → 容器 Restarting。
//
// 当前正确行为：
//   - SDK 首次注册时 service 不存在 → 自动以 ephemeral=true 创 service + 注册 instance
//   - 心跳由 SDK 内部 BeatRequest 维护（BeatInterval 5s），Dev/Prod 一致
//   - 多副本扩缩容需要 Nacos 自动摘除 → 走 ephemeral（默认即满足）
//   - 极少数"audit 留痕要求持久实例"场景 → NacosConfig.Ephemeral=false 显式覆盖
//
// 注意：当前 dev Nacos 里残留 6 个 persistent serviceName，需用 Nacos API 删掉后
// SDK 重启才会按 ephemeral 重建（见 scripts/clean_dev_nacos_persistent.sh / Stage-52 §五）。
var defaultRegisterEphemeral = true

// NacosConfig 描述如何连接 Nacos。
//
// ServerAddr 形如 "nacos:8848" 或 "127.0.0.1:8848"，支持多个用逗号分隔；
// Namespace 与 Nacos 控制台 namespaceId 对应（dev="emotion-echo-dev"，prod="emotion-echo-prod"）。
type NacosConfig struct {
	ServerAddr string
	Namespace  string
	// GroupName 默认 DEFAULT_GROUP（详见 ADR 决策 10）。
	GroupName string
	// Username / Password 默认空（Nacos standalone 无鉴权）。
	Username string
	Password string
	// TimeoutMs 单次 RPC 超时，默认 5000ms。
	TimeoutMs uint64
	// Ephemeral 覆盖 Register() 时使用的 Ephemeral 字段。
	// 0 值时使用包级默认（PR-1: false 持久实例）；prod 集群显式设为 true 让 Nacos 自动摘除。
	Ephemeral bool
}

func (c *NacosConfig) defaults() {
	if c.GroupName == "" {
		c.GroupName = "DEFAULT_GROUP"
	}
	if c.TimeoutMs == 0 {
		c.TimeoutMs = 5000
	}
}

// ApplyDefaults 导出等价于 unexported defaults()，供 configcenter 包复用。
func ApplyDefaults(c *NacosConfig) { c.defaults() }

// NacosRegistry 是 Registry 接口的 Nacos 实现。
//
// 设计要点：
//   - 不通过 go-zero v1 plugin（v1 plugin 与 nacos-sdk-go/v2 不兼容）
//   - SDK 自身维护心跳（默认 5s，ClientConfig BeatInterval），本层 Heartbeat()
//     启动 watcher goroutine 用于调用方主动控制生命周期与优雅退出
//   - 优雅退出通过 context cancel；ctx.Done() 时调用方应已调 Unregister
type NacosRegistry struct {
	cfg    NacosConfig
	client nacosnaming.INamingClient

	mu         sync.Mutex
	subscribed map[string]*nacosvo.SubscribeParam // serviceName -> active subscription
}

// NewNacosRegistry 创建并连接一个 NacosRegistry。
//
// 返回的 Registry 立即可用于 Register/Discover。
// 调用方应在 main.go 启动早期调用，并通过 WaitForNacos 等待服务可达。
func NewNacosRegistry(ctx context.Context, cfg NacosConfig) (*NacosRegistry, error) {
	cfg.defaults()

	serverConfigs, err := buildServerConfigs(cfg.ServerAddr)
	if err != nil {
		return nil, fmt.Errorf("discovery: parse nacos server addr: %w", err)
	}

	clientConfig := nacosconstant.NewClientConfig(
		nacosconstant.WithNamespaceId(cfg.Namespace),
		nacosconstant.WithTimeoutMs(cfg.TimeoutMs),
		nacosconstant.WithBeatInterval(int64(5000)), // 5s 心跳，与 SDK 内部对齐
		nacosconstant.WithNotLoadCacheAtStart(true), // dev 启动不要读本地缓存
		nacosconstant.WithUpdateThreadNum(2),
	)

	client, err := nacosclients.NewNamingClient(nacosvo.NacosClientParam{
		ClientConfig:  clientConfig,
		ServerConfigs: serverConfigs,
	})
	if err != nil {
		return nil, fmt.Errorf("discovery: create nacos naming client: %w", err)
	}

	return &NacosRegistry{
		cfg:        cfg,
		client:     client,
		subscribed: make(map[string]*nacosvo.SubscribeParam),
	}, nil
}

// buildServerConfigs 解析 "host:port,host:port" 格式为 nacos SDK ServerConfig 列表。
func buildServerConfigs(addr string) ([]nacosconstant.ServerConfig, error) {
	if addr == "" {
		return nil, errors.New("empty nacos server addr")
	}
	parts := strings.Split(addr, ",")
	out := make([]nacosconstant.ServerConfig, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		host, portStr, err := net.SplitHostPort(p)
		if err != nil {
			return nil, fmt.Errorf("invalid addr %q: %w", p, err)
		}
		port, err := strconv.ParseUint(portStr, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid port %q: %w", portStr, err)
		}
		out = append(out, nacosconstant.ServerConfig{
			IpAddr: host,
			Port:   port,
		})
	}
	return out, nil
}

// -----------------------------------------------------------------------------
// Registry interface implementation
// -----------------------------------------------------------------------------

func (r *NacosRegistry) Register(ctx context.Context, ins Instance) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ephemeral := defaultRegisterEphemeral || r.cfg.Ephemeral
	// PR-1 修复：Host 为 0.0.0.0（yaml 默认）会让 Nacos 把实例判 unhealthy；
	// fallback 到本机非 loopback IPv4，Nacos 才能正确 health check。
	host := registerHost(ins.Host)
	param := nacosvo.RegisterInstanceParam{
		Ip:          host,
		Port:        uint64(ins.Port),
		Weight:      1.0,
		Enable:      true,
		Healthy:     true,
		Metadata:    ins.Metadata,
		ClusterName: "DEFAULT",
		ServiceName: ins.ServiceName,
		GroupName:   r.cfg.GroupName,
		Ephemeral:   ephemeral,
	}

	// Stage 72：SDK v2 gRPC 通道异步建立——NewNamingClient 返回时连接可能仍在
	// STARTING，Register 立即调用会报 "client not connected, current status:STARTING"
	//（集成测试实测复现）。WaitForNacos 只探测 HTTP 8848，等不到 gRPC 9849 就绪，
	// 因此这里对瞬时错误做退避重试，否则 dev 容器启动即 Register 失败 / 实例无法续活。
	var ok bool
	var err error
retry:
	for attempt := 0; ; attempt++ {
		ok, err = r.client.RegisterInstance(param)
		if err == nil || !isClientNotConnected(err) {
			break
		}
		if attempt >= 29 || ctx.Err() != nil { // 29×500ms ≈ 15s 上限
			break
		}
		select {
		case <-ctx.Done():
			break retry
		case <-time.After(500 * time.Millisecond):
		}
	}
	if err != nil {
		return fmt.Errorf("discovery: register %s/%s:%d: %w", ins.ServiceName, ins.Host, ins.Port, err)
	}
	if !ok {
		return fmt.Errorf("discovery: register %s/%s:%d: nacos returned not-ok", ins.ServiceName, ins.Host, ins.Port)
	}
	return nil
}

// isClientNotConnected 识别 SDK gRPC 通道尚未就绪的瞬时错误（连接异步建立）。
func isClientNotConnected(err error) bool {
	return err != nil && strings.Contains(err.Error(), "client not connected")
}

func (r *NacosRegistry) Unregister(ctx context.Context, ins Instance) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ephemeral := defaultRegisterEphemeral || r.cfg.Ephemeral
	_, err := r.client.DeregisterInstance(nacosvo.DeregisterInstanceParam{
		Ip:          registerHost(ins.Host),
		Port:        uint64(ins.Port),
		Cluster:     "DEFAULT",
		ServiceName: ins.ServiceName,
		GroupName:   r.cfg.GroupName,
		Ephemeral:   ephemeral,
	})
	if err != nil {
		return fmt.Errorf("discovery: deregister %s/%s:%d: %w", ins.ServiceName, ins.Host, ins.Port, err)
	}
	return nil
}

func (r *NacosRegistry) Discover(ctx context.Context, serviceName string) ([]Instance, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	nacosInstances, err := r.client.SelectInstances(nacosvo.SelectInstancesParam{
		ServiceName: serviceName,
		GroupName:   r.cfg.GroupName,
		HealthyOnly: true,
	})
	if err != nil {
		// Stage 72：SDK 对"缓存中 hosts 为空"返 error("instance list is empty!")，
		// 语义上是"无可用实例"而非故障 → 映射为空列表（Registry 契约：空 = 空 slice, nil error）。
		if strings.Contains(err.Error(), "instance list is empty") {
			return []Instance{}, nil
		}
		return nil, fmt.Errorf("discovery: select %s: %w", serviceName, err)
	}
	return convertInstances(r.cfg.GroupName, nacosInstances), nil
}

func (r *NacosRegistry) Subscribe(ctx context.Context, serviceName string, cb func([]Instance)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if cb == nil {
		return errors.New("discovery: subscribe callback must not be nil")
	}

	param := &nacosvo.SubscribeParam{
		ServiceName: serviceName,
		GroupName:   r.cfg.GroupName,
		SubscribeCallback: func(services []nacosmodel.Instance, err error) {
			if err != nil {
				// Nacos SDK 在订阅出错时调用此 callback；调用方只能看到原始 err，
				// 我们选择 swallow + 不调用 cb，避免破坏订阅语义。
				// 调用方可通过 SelectInstances 主动 poll 检测状态。
				return
			}
			cb(convertInstances(r.cfg.GroupName, services))
		},
	}

	if err := r.client.Subscribe(param); err != nil {
		return fmt.Errorf("discovery: subscribe %s: %w", serviceName, err)
	}

	r.mu.Lock()
	r.subscribed[serviceName] = param
	r.mu.Unlock()

	// 立即触发一次回调，给调用方一个"初始视图"。
	// 这一行为与契约测试 fakeRegistry 保持一致。
	initial, err := r.client.SelectInstances(nacosvo.SelectInstancesParam{
		ServiceName: serviceName,
		GroupName:   r.cfg.GroupName,
		HealthyOnly: true,
	})
	if err == nil {
		cb(convertInstances(r.cfg.GroupName, initial))
	}
	return nil
}

// Heartbeat 启动后台 watcher goroutine：按 interval 周期重新注册（SDK 内部已维持 5s 心跳，
// 这里额外做"长连接守护"，确保 instance metadata 在 Nacos 控制台始终新鲜）。
//
// ctx.Done() 时退出 goroutine。interval<=0 时强制为 1s。
func (r *NacosRegistry) Heartbeat(ctx context.Context, ins Instance, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// 周期续约：调一次 UpdateInstance 让 Nacos 重新感知本实例。
				// SDK 没有独立的 SendHeartbeat 公开方法，UpdateInstance 是公开心跳通道。
				_, _ = r.client.UpdateInstance(nacosvo.UpdateInstanceParam{
					Ip:          registerHost(ins.Host),
					Port:        uint64(ins.Port),
					Weight:      1.0,
					Enable:      true,
					Healthy:     true,
					Metadata:    ins.Metadata,
					ClusterName: "DEFAULT",
					ServiceName: ins.ServiceName,
					GroupName:   r.cfg.GroupName,
					Ephemeral:   true,
				})
			}
		}
	}()
}

// convertInstances 把 SDK model.Instance 列表转成 discovery.Instance 列表。
//
// Stage 72：SDK 返回的 ServiceName 带 "GROUP@@name" 前缀（如
// "DEFAULT_GROUP@@user-svc"，集成测试实测），剥掉前缀让调用方看到注册名。
func convertInstances(groupName string, src []nacosmodel.Instance) []Instance {
	prefix := groupName + "@@"
	out := make([]Instance, 0, len(src))
	for _, n := range src {
		out = append(out, Instance{
			ServiceName: strings.TrimPrefix(n.ServiceName, prefix),
			Host:        n.Ip,
			Port:        int(n.Port),
			Metadata:    n.Metadata,
		})
	}
	return out
}

// WaitForNacos 等待 Nacos 可达（指数退避，最长 maxWait）。dev 启动早期调用。
//
// 返回 nil 表示 Nacos `/nacos/actuator/health` 在某次探测中返回 200；否则 maxWait 后返回最后一次错误。
func WaitForNacos(ctx context.Context, serverAddr string, maxWait time.Duration) error {
	if serverAddr == "" {
		return errors.New("discovery: empty nacos server addr for WaitForNacos")
	}
	deadline := time.Now().Add(maxWait)
	delay := 500 * time.Millisecond
	const maxDelay = 5 * time.Second

	// 解析 addr 的第一段用于 http URL
	first := strings.Split(serverAddr, ",")[0]
	host, port, err := net.SplitHostPort(strings.TrimSpace(first))
	if err != nil {
		return fmt.Errorf("discovery: WaitForNacos parse addr: %w", err)
	}

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		url := fmt.Sprintf("http://%s:%s/nacos/actuator/health", host, port)
		// 极简探测：避免引入 net/http 客户端依赖；用 Dial 替代。
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(host, port), 2*time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("discovery: nacos %s not reachable within %s (last url=%s)", serverAddr, maxWait, url)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

// 编译期断言：NacosRegistry 必须实现 Registry interface。
var _ Registry = (*NacosRegistry)(nil)

// resolveRegisterIP 返回本机非 loopback IPv4。Host=0.0.0.0（yaml 默认）会
// 让 Nacos 把实例判 unhealthy，因此 Register 时用本机 IP。
//
// registerHost 把 yaml 里的 Host 占位符解析为可路由的本机 IP。
//
// Stage 62 PR-3.4 修复（决策 18 #28）：Register / Unregister / Heartbeat **三处必须共用**本函数。
//
// 历史 bug（2026-09-10 docker 实测）：
//   - Register() 已解析 0.0.0.0 → 真实 IP（b869ff9 PR-1），实测 T+3s Nacos 显示 172.18.0.14:8888 ✅
//   - 但 Heartbeat() 用原始 ins.Host（"0.0.0.0"）调 UpdateInstance，
//     5s 后把正确注册覆盖成 0.0.0.0；Unregister() 同样注销不到真实实例。
//   - 后果：APISIX nacos-discovery 拉到 0.0.0.0 上游 → connect refused → **dev 网关全 502**。
func registerHost(h string) string {
	if h == "" || h == "0.0.0.0" {
		return resolveRegisterIP()
	}
	return h
}

// resolveRegisterIP 取本机非 loopback IPv4。
//
// 实现：net.Dial UDP 到 8.8.8.8 取本机 outbound IP（不真发包）——
// 容器内通用，不依赖具体网卡名。
func resolveRegisterIP() string {
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		// fallback：第一个非 loopback interface IP
		if addrs, _ := net.InterfaceAddrs(); len(addrs) > 0 {
			for _, a := range addrs {
				if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
					return ipnet.IP.String()
				}
			}
		}
		return "127.0.0.1"
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String()
}
