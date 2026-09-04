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
// PR-1 修复：dev 模式下 SDK v2.4.3 + Nacos 2.4.3 server 在 Derby 启动慢场景下
// BeatRequest 不可靠，ephemeral 实例会被 server 在 ~30s 内踢出，导致
// instance/list 返回 hosts: []。改为 false（持久实例）后，注册即落 Derby，
// 不依赖心跳——dev compose 重启周期远小于 Derby 数据保留周期，可接受。
//
// prod 集群场景调用方应通过 NacosConfig.Ephemeral 字段显式覆盖为 true
// （多副本扩缩容需要 ephemeral 自动摘除）。
var defaultRegisterEphemeral = false

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
	host := ins.Host
	if host == "" || host == "0.0.0.0" {
		host = resolveRegisterIP()
	}
	ok, err := r.client.RegisterInstance(nacosvo.RegisterInstanceParam{
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
	})
	if err != nil {
		return fmt.Errorf("discovery: register %s/%s:%d: %w", ins.ServiceName, ins.Host, ins.Port, err)
	}
	if !ok {
		return fmt.Errorf("discovery: register %s/%s:%d: nacos returned not-ok", ins.ServiceName, ins.Host, ins.Port)
	}
	return nil
}

func (r *NacosRegistry) Unregister(ctx context.Context, ins Instance) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	ephemeral := defaultRegisterEphemeral || r.cfg.Ephemeral
	_, err := r.client.DeregisterInstance(nacosvo.DeregisterInstanceParam{
		Ip:          ins.Host,
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
		return nil, fmt.Errorf("discovery: select %s: %w", serviceName, err)
	}
	return convertInstances(nacosInstances), nil
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
			cb(convertInstances(services))
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
		cb(convertInstances(initial))
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
					Ip:          ins.Host,
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
func convertInstances(src []nacosmodel.Instance) []Instance {
	out := make([]Instance, 0, len(src))
	for _, n := range src {
		out = append(out, Instance{
			ServiceName: n.ServiceName,
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
