// chat-svc Nacos 接入（Stage 31 PR-08）
//
// 启动流程与 user-svc（PR-07）同构；详见 emotion-echo-user-svc/nacos_boot.go。
// 后续 Stage 31 PR-12 之后会抽到 shared/pkg/nacosboot 复用，本 PR 暂复制。
package main

import (
	"context"
	"fmt"

	"github.com/emotion-echo/shared/pkg/logging"
	"os"
	"time"

	sharedconfig "github.com/emotion-echo/shared/pkg/configcenter"
	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"
	"gopkg.in/yaml.v3"

	"emotion-echo-chat-svc/internal/config"
	"emotion-echo-chat-svc/internal/outbox"
)

// NacosRuntime 持有本 svc 的 Nacos 客户端与生命周期钩子。
type NacosRuntime struct {
	Registry     shareddiscovery.Registry
	ConfigCenter sharedconfig.ConfigCenter
	Cancel       context.CancelFunc
}

// Close 释放资源：Unregister + 关闭 listener + 关闭 ConfigCenter。
func (r *NacosRuntime) Close(ctx context.Context, svcName, host string, port int) {
	if r.Registry != nil {
		ins := shareddiscovery.Instance{ServiceName: svcName, Host: host, Port: port}
		if err := r.Registry.Unregister(ctx, ins); err != nil {
			logging.PrintfContext(ctx, "[nacos] unregister failed (continuing): %v", err)
		}
	}
	if r.Cancel != nil {
		r.Cancel()
	}
	if r.ConfigCenter != nil {
		_ = r.ConfigCenter.Close()
	}
}

// bootDeps 是 BootNacos 的依赖注入点。
type bootDeps struct {
	registryFactory func(ctx context.Context, addr, namespace, group string) (shareddiscovery.Registry, error)
	configFactory   func(ctx context.Context, addr, namespace, group string) (sharedconfig.ConfigCenter, error)
	waitForNacos    func(ctx context.Context, addr string, maxWait time.Duration) error
	// ops 运营参数容器（E2E-23 E 组）。nil 时退化为"只读不写"——
	// 即维持 F-155 修复前的行为（拿到 ops 配置只打日志）。
	// 注入它才能让 Nacos 推送真正生效。
	ops *outbox.Ops
}

func defaultBootDeps() bootDeps {
	return bootDeps{
		registryFactory: func(ctx context.Context, addr, namespace, group string) (shareddiscovery.Registry, error) {
			return shareddiscovery.NewNacosRegistry(ctx, shareddiscovery.NacosConfig{
				ServerAddr: addr, Namespace: namespace, GroupName: group, TimeoutMs: 5000,
			})
		},
		configFactory: func(ctx context.Context, addr, namespace, group string) (sharedconfig.ConfigCenter, error) {
			return sharedconfig.NewNacosConfig(ctx, shareddiscovery.NacosConfig{
				ServerAddr: addr, Namespace: namespace, GroupName: group, TimeoutMs: 5000,
			})
		},
		waitForNacos: shareddiscovery.WaitForNacos,
	}
}

// BootNacos 接入 Nacos（chat-svc 版本，与 user-svc 同构）
//
// 步骤：
//  1. WaitForNacos 指数退避 60s
//  2. Register 注册 chat-svc 实例
//  3. Heartbeat 5s 续约 goroutine
//  4. GetConfig 拉取 chat-svc.ops.yaml
//  5. ListenConfig 热重载回调（HotReload=true）
func BootNacos(ctx context.Context, cfg *config.Config, deps bootDeps) (*NacosRuntime, error) {
	if !cfg.Nacos.Enabled {
		logging.PrintfContext(ctx, "[nacos] disabled by config")
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
	instance := shareddiscovery.Instance{
		ServiceName: cfg.Name,
		Host:        cfg.Host,
		Port:        cfg.Port,
		Metadata:    metadata,
	}
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
	applyOps := func(content string, source string) {
		if deps.ops == nil {
			logging.PrintfContext(ctx, "[nacos] ops %s: %d bytes（未注入 ops 容器，仅记录）", source, len(content))
			return
		}
		// P1（账本 F-158）：反序列化**之前**先做内容层敏感字段清洗 ——
		// 既有 sensitivePrefixes 只拦 dataId，而 ops 是"单 dataId 打包全部运营
		// 参数"，往里塞 llm.api_key 不会被拦。
		cleaned, dropped := sharedconfig.SanitizeOpsContent(content)
		if len(dropped) > 0 {
			logging.PrintfContext(ctx, "[nacos] ops %s 剔除敏感 key: %v", source, dropped)
		}
		var parsed outbox.OpsConfig
		if err := yaml.Unmarshal([]byte(cleaned), &parsed); err != nil {
			logging.PrintfContext(ctx, "[nacos] ops %s parse failed (continuing): %v", source, err)
			return
		}
		deps.ops.Apply(parsed)
		got := deps.ops.Snapshot()
		logging.PrintfContext(ctx, "[nacos] ops applied via %s: max_attempts=%d sent_retention=%dd dead_retention=%dd cleanup=%ds",
			source, got.MaxAttempts, got.SentRetentionDays, got.DeadRetentionDays, got.CleanupIntervalS)
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
		} else {
			logging.PrintfContext(ctx, "[nacos] hot-reload listener registered on %s/%s", group, dataId)
		}
	}

	return &NacosRuntime{
		Registry:     reg,
		ConfigCenter: cc,
		Cancel:       hbCancel,
	}, nil
}

func gitVersion() string {
	if v := os.Getenv("GIT_VERSION"); v != "" {
		return v
	}
	return "dev-build"
}
