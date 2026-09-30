// analytics-svc Nacos 接入（Stage 31 PR-09，与 user-svc/chat-svc/assessment-svc 同构模板）
package main

import (
	"context"
	"fmt"
	"gopkg.in/yaml.v3"

	"github.com/emotion-echo/shared/pkg/logging"
	"os"
	"time"

	sharedconfig "github.com/emotion-echo/shared/pkg/configcenter"
	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"

	"emotion-echo-analytics-svc/internal/config"
	"emotion-echo-analytics-svc/internal/kafka"
)

type NacosRuntime struct {
	Registry     shareddiscovery.Registry
	ConfigCenter sharedconfig.ConfigCenter
	Cancel       context.CancelFunc
}

func (r *NacosRuntime) Close(ctx context.Context, svcName, host string, port int) {
	if r.Registry != nil {
		_ = r.Registry.Unregister(ctx, shareddiscovery.Instance{ServiceName: svcName, Host: host, Port: port})
	}
	if r.Cancel != nil {
		r.Cancel()
	}
	if r.ConfigCenter != nil {
		_ = r.ConfigCenter.Close()
	}
}

type bootDeps struct {
	registryFactory func(ctx context.Context, addr, namespace, group string) (shareddiscovery.Registry, error)
	configFactory   func(ctx context.Context, addr, namespace, group string) (sharedconfig.ConfigCenter, error)
	waitForNacos    func(ctx context.Context, addr string, maxWait time.Duration) error
	// consumer Kafka 消费方（E2E-23 #32）。nil 时退化为"只记录不应用"。
	consumer *kafka.Consumer
}

func defaultBootDeps() bootDeps {
	return bootDeps{
		registryFactory: func(ctx context.Context, addr, namespace, group string) (shareddiscovery.Registry, error) {
			return shareddiscovery.NewNacosRegistry(ctx, shareddiscovery.NacosConfig{ServerAddr: addr, Namespace: namespace, GroupName: group, TimeoutMs: 5000})
		},
		configFactory: func(ctx context.Context, addr, namespace, group string) (sharedconfig.ConfigCenter, error) {
			return sharedconfig.NewNacosConfig(ctx, shareddiscovery.NacosConfig{ServerAddr: addr, Namespace: namespace, GroupName: group, TimeoutMs: 5000})
		},
		waitForNacos: shareddiscovery.WaitForNacos,
	}
}

func BootNacos(ctx context.Context, cfg *config.Config, deps bootDeps) (*NacosRuntime, error) {
	if !cfg.Nacos.Enabled {
		return &NacosRuntime{}, nil
	}
	addr := cfg.Nacos.Addr
	namespace := cfg.Nacos.Namespace
	group := cfg.Nacos.GroupName

	waitCtx, waitCancel := context.WithTimeout(ctx, 60*time.Second)
	defer waitCancel()
	if err := deps.waitForNacos(waitCtx, addr, 60*time.Second); err != nil {
		return nil, fmt.Errorf("[nacos] WaitForNacos: %w", err)
	}
	reg, err := deps.registryFactory(ctx, addr, namespace, group)
	if err != nil {
		return nil, fmt.Errorf("[nacos] NewNacosRegistry: %w", err)
	}
	// Stage 75: gRPC 端口写入 metadata.grpc_port，供 web-bff resolveGRPCAddr
	// Nacos 优先解析 gRPC 拨号地址（对齐 ai-svc Stage 32 先例）。
	metadata := map[string]string{"stage": namespace, "version": gitVersion()}
	if cfg.GRPC.Enabled {
		metadata["grpc_port"] = fmt.Sprintf("%d", cfg.GRPC.Port)
	}
	instance := shareddiscovery.Instance{ServiceName: cfg.Name, Host: cfg.Host, Port: cfg.Port,
		Metadata: metadata}
	if err := reg.Register(ctx, instance); err != nil {
		return nil, fmt.Errorf("[nacos] Register: %w", err)
	}
	logging.PrintfContext(ctx, "[nacos] registered %s at %s:%d", instance.ServiceName, instance.Host, instance.Port)
	hbCtx, hbCancel := context.WithCancel(context.Background())
	reg.Heartbeat(hbCtx, instance, 5*time.Second)
	cc, err := deps.configFactory(ctx, addr, namespace, group)
	if err != nil {
		hbCancel()
		_ = reg.Unregister(ctx, instance)
		return nil, fmt.Errorf("[nacos] NewNacosConfig: %w", err)
	}
	dataId := cfg.Name + ".ops.yaml"

	// OpsConfig 是 ops dataId 的**白名单**字段集（未知 key 静默忽略）。
	// P1：反序列化前先做内容层敏感字段清洗 —— 既有 sensitivePrefixes
	// 只拦 dataId，而 ops 是"单 dataId 打包全部参数"。
	type OpsConfig struct {
		MaxRetries int `yaml:"max_retries"`
	}
	applyOps := func(content, source string) {
		if deps.consumer == nil {
			logging.PrintfContext(ctx, "[nacos] ops %s: %d bytes（未注入 consumer，仅记录）", source, len(content))
			return
		}
		cleaned, dropped := sharedconfig.SanitizeOpsContent(content)
		if len(dropped) > 0 {
			logging.PrintfContext(ctx, "[nacos] ops %s 剔除敏感 key: %v", source, dropped)
		}
		var parsed OpsConfig
		if err := yaml.Unmarshal([]byte(cleaned), &parsed); err != nil {
			logging.PrintfContext(ctx, "[nacos] ops %s parse failed (continuing): %v", source, err)
			return
		}
		deps.consumer.UpdateMaxRetries(parsed.MaxRetries)
		logging.PrintfContext(ctx, "[nacos] ops applied via %s: max_retries=%d",
			source, deps.consumer.CurrentMaxRetries())
	}

	if opsYaml, err := cc.GetConfig(ctx, dataId, group); err != nil {
		logging.PrintfContext(ctx, "[nacos] GetConfig(%s/%s) failed (continuing): %v", group, dataId, err)
	} else {
		applyOps(opsYaml, "GetConfig")
	}
	if cfg.Nacos.HotReload {
		if err := cc.ListenConfig(ctx, dataId, group, func(d, g, content string) error {
			logging.PrintfContext(ctx, "[nacos] [hot-reload] %s/%s changed, %d bytes", g, d, len(content))
			applyOps(content, "hot-reload")
			return nil
		}); err != nil {
			logging.PrintfContext(ctx, "[nacos] ListenConfig failed (continuing): %v", err)
		}
	}
	return &NacosRuntime{Registry: reg, ConfigCenter: cc, Cancel: hbCancel}, nil
}

func gitVersion() string {
	if v := os.Getenv("GIT_VERSION"); v != "" {
		return v
	}
	return "dev-build"
}
