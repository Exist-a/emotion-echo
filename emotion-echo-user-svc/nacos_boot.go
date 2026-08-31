// user-svc Nacos 接入（Stage 31 PR-07）
//
// 启动流程：
//  1. WaitForNacos 指数退避等待 Nacos 可达
//  2. NewNacosRegistry 注册本实例（service-name=user-svc）
//  3. NewNacosConfig 拉取 {service}.ops.yaml 运营参数
//  4. ListenConfig 注册热更新回调
//  5. Heartbeat 启动 5s 心跳续约
//  6. 返回 *NacosRuntime + cancel 函数，调用方在 SIGINT/SIGTERM 时调用
//
// 设计：boot 流程独立成函数便于单测；main.go 只负责串联与信号处理。
// 测试可注入 fakeRegistry + fakeConfigCenter（来自 shared/pkg/discovery + configcenter 的契约测试）。
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	sharedconfig "github.com/emotion-echo/shared/pkg/configcenter"
	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"

	"emotion-echo-user-svc/internal/config"
)

// NacosRuntime 持有本 svc 的 Nacos 客户端与生命周期钩子。
//
// 调用方须在 svc 退出前调用 Close() 释放连接（Unregister + 关闭 listener）。
type NacosRuntime struct {
	Registry     shareddiscovery.Registry
	ConfigCenter sharedconfig.ConfigCenter
	Cancel       context.CancelFunc
}

// Close 释放资源：先 Unregister 再关闭 ConfigCenter，触发 SIGINT/SIGTERM 时优雅退出。
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

// bootDeps 是 BootNacos 的依赖注入点，便于单测 mock。
//
// 真实 main.go 使用 NewRealBootDeps（实际连 Nacos）；单测注入 fake。
type bootDeps struct {
	registryFactory func(ctx context.Context, addr, namespace, group string) (shareddiscovery.Registry, error)
	configFactory   func(ctx context.Context, addr, namespace, group string) (sharedconfig.ConfigCenter, error)
	waitForNacos    func(ctx context.Context, addr string, maxWait time.Duration) error
}

// defaultBootDeps 返回真实实现（NewNacosRegistry / NewNacosConfig / WaitForNacos）。
func defaultBootDeps() bootDeps {
	return bootDeps{
		registryFactory: func(ctx context.Context, addr, namespace, group string) (shareddiscovery.Registry, error) {
			return shareddiscovery.NewNacosRegistry(ctx, shareddiscovery.NacosConfig{
				ServerAddr: addr,
				Namespace:  namespace,
				GroupName:  group,
				TimeoutMs:  5000,
			})
		},
		configFactory: func(ctx context.Context, addr, namespace, group string) (sharedconfig.ConfigCenter, error) {
			return sharedconfig.NewNacosConfig(ctx, shareddiscovery.NacosConfig{
				ServerAddr: addr,
				Namespace:  namespace,
				GroupName:  group,
				TimeoutMs:  5000,
			})
		},
		waitForNacos: shareddiscovery.WaitForNacos,
	}
}

// BootNacos 接入 Nacos 注册中心 + 配置中心。
//
// 步骤：
//  1. WaitForNacos：最长 maxWait 秒等待 Nacos 可达
//  2. Registry.Register：注册本实例（service-name=cfg.Name）
//  3. Registry.Heartbeat：启动 5s 心跳续约 goroutine
//  4. ConfigCenter.GetConfig：拉取 {svc}.ops.yaml 运营参数（dev 下缺失返回空）
//  5. ConfigCenter.ListenConfig：注册热更新回调（HotReload 启用时）
//
// 失败语义：
//   - Nacos 不可达 → 返回 error（main.go 决定是否阻断启动）
//   - 配置不存在 → 不报错（首次启动是正常的；Nacos 控制台尚未 bootstrap）
//   - 心跳启动失败 → 记录日志但不阻断
func BootNacos(ctx context.Context, cfg *config.Config, deps bootDeps) (*NacosRuntime, error) {
	if !cfg.Nacos.Enabled {
		log.Printf("[nacos] disabled by config")
		return &NacosRuntime{}, nil
	}

	addr := cfg.Nacos.Addr
	namespace := cfg.Nacos.Namespace
	group := cfg.Nacos.GroupName

	// 1. WaitForNacos
	waitCtx, waitCancel := context.WithTimeout(ctx, 60*time.Second)
	defer waitCancel()
	if err := deps.waitForNacos(waitCtx, addr, 60*time.Second); err != nil {
		return nil, fmt.Errorf("[nacos] WaitForNacos: %w", err)
	}

	// 2. Registry
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

	// 3. Heartbeat
	hbCtx, hbCancel := context.WithCancel(context.Background())
	reg.Heartbeat(hbCtx, instance, 5*time.Second)

	// 4. ConfigCenter
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

	// 5. ListenConfig
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

// gitVersion 返回构建期注入的 git SHA（dev 默认 "dev-build"）。
// Stage 31 暂用环境变量；后续可在 Dockerfile -ldflags 注入。
func gitVersion() string {
	if v := os.Getenv("GIT_VERSION"); v != "" {
		return v
	}
	return "dev-build"
}
