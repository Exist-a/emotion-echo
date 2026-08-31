// web-bff Nacos 接入（Stage 31 PR-09）
//
// web-bff 也注册到 Nacos（service-name=web-bff）；Stage 32 APISIX 通过
// nacos-discovery 插件自动发现 BFF 作为上游，无需静态 upstream 配置。
// 与其他 svc 同构（PR-07/08 模板），仅 service-name=web-bff / port=8894。
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	sharedconfig "github.com/emotion-echo/shared/pkg/configcenter"
	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"

	"emotion-echo-web-bff/internal/config"
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
	instance := shareddiscovery.Instance{ServiceName: cfg.Name, Host: cfg.Host, Port: cfg.Port,
		Metadata: map[string]string{"stage": namespace, "version": gitVersion()}}
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
	return &NacosRuntime{Registry: reg, ConfigCenter: cc, Cancel: hbCancel}, nil
}

func gitVersion() string {
	if v := os.Getenv("GIT_VERSION"); v != "" {
		return v
	}
	return "dev-build"
}
