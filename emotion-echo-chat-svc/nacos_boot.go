// chat-svc Nacos 接入（Stage 31 PR-08）
//
// 启动流程与 user-svc（PR-07）同构；详见 emotion-echo-user-svc/nacos_boot.go。
// 后续 Stage 31 PR-12 之后会抽到 shared/pkg/nacosboot 复用，本 PR 暂复制。
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	sharedconfig "github.com/emotion-echo/shared/pkg/configcenter"
	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"

	"emotion-echo-chat-svc/internal/config"
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
			log.Printf("[nacos] unregister failed (continuing): %v", err)
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
		log.Printf("[nacos] disabled by config")
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

	instance := shareddiscovery.Instance{
		ServiceName: cfg.Name,
		Host:        cfg.Host,
		Port:        cfg.Port,
		Metadata: map[string]string{
			"stage":   namespace,
			"version": gitVersion(),
		},
	}
	if err := reg.Register(ctx, instance); err != nil {
		return nil, fmt.Errorf("[nacos] Register: %w", err)
	}
	log.Printf("[nacos] registered %s at %s:%d", instance.ServiceName, instance.Host, instance.Port)

	hbCtx, hbCancel := context.WithCancel(context.Background())
	reg.Heartbeat(hbCtx, instance, 5*time.Second)

	cc, err := deps.configFactory(ctx, addr, namespace, group)
	if err != nil {
		hbCancel()
		_ = reg.Unregister(ctx, instance)
		return nil, fmt.Errorf("[nacos] NewNacosConfig: %w", err)
	}

	dataId := cfg.Name + ".ops.yaml"
	if opsYaml, err := cc.GetConfig(ctx, dataId, group); err != nil {
		log.Printf("[nacos] GetConfig(%s/%s) failed (continuing): %v", group, dataId, err)
	} else {
		log.Printf("[nacos] ops config loaded: %s/%s, %d bytes", group, dataId, len(opsYaml))
	}

	if cfg.Nacos.HotReload {
		if err := cc.ListenConfig(ctx, dataId, group, func(d, g, content string) error {
			log.Printf("[nacos] [hot-reload] %s/%s changed, %d bytes", g, d, len(content))
			return nil
		}); err != nil {
			log.Printf("[nacos] ListenConfig failed (continuing): %v", err)
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
