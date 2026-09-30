// web-bff Nacos 接入（Stage 31 PR-09）
//
// web-bff 也注册到 Nacos（service-name=web-bff）；Stage 32 APISIX 通过
// nacos-discovery 插件自动发现 BFF 作为上游，无需静态 upstream 配置。
// 与其他 svc 同构（PR-07/08 模板），仅 service-name=web-bff / port=8894。
//
// PR-4: HotReload=true 时 web-bff.ops.yaml 解析为 OpsConfig，热更新同步到
// HotReloadLimiter；limiter 由 main.go 通过 bootDeps.opsLimiter 注入。
package main

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/emotion-echo/shared/pkg/logging"
	"os"
	"time"

	sharedconfig "github.com/emotion-echo/shared/pkg/configcenter"
	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"

	"emotion-echo-web-bff/internal/config"
	"emotion-echo-web-bff/internal/handler"
	"gopkg.in/yaml.v3"
)

type NacosRuntime struct {
	Registry     shareddiscovery.Registry
	ConfigCenter sharedconfig.ConfigCenter
	Cancel       context.CancelFunc

	// registered 记录本实例当前是否已注册到 Nacos（E2E-23 测试点 #6）。
	//
	// 为什么需要它：/health 若只看"进程活着 + 下游都通"，就会出现
	// F-137 那个悖论 —— 自查健康但**网关解析不到本实例**。
	// 探针必须能回答"我现在可被发现吗"。
	//
	// 用 atomic 而非普通 bool：BootNacos 在启动 goroutine 里写，
	// /health 在请求 goroutine 里读，无同步即为 data race
	//（E2E-21 教训：单测全绿也照样是 bug）。
	registered atomic.Bool
}

// Registered 报告本服务当前是否已注册到 Nacos。
// Nacos 未启用（runtime 为 nil）时返回 false —— 此时"是否可被发现"
// 不由服务发现决定，不应据此报降级，调用方需自行判断。
func (r *NacosRuntime) Registered() bool {
	if r == nil {
		return false
	}
	return r.registered.Load()
}

func (r *NacosRuntime) Close(ctx context.Context, svcName, host string, port int) {
	// 先置 false：注销过程中网关已可能解析不到本实例
	r.registered.Store(false)
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
	// opsLimiter 可选；为 nil 时 web-bff.ops.yaml 仅 log 不应用。
	// main.go 注入真实 limiter，单测注入 fake 或 nil。
	opsLimiter *handler.HotReloadLimiter
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

	logging.PrintfContext(ctx, "[nacos] registered %s at %s:%d", instance.ServiceName, instance.Host, instance.Port)
	hbCtx, hbCancel := context.WithCancel(context.Background())
	// Round 4.2 P1-7: BeatHeartbeat 走 Nacos /instance/beat 标准协议（HTTP POST），
	// 替代 SDK UpdateInstance 退化方式。原 SDK v2.3.5 无公开 BeatInstance API，
	// 本实现直接调 HTTP 端点（与 Java 客户端一致），失败 fallback SDK UpdateInstance。
	reg.BeatHeartbeat(hbCtx, instance, 5*time.Second)
	cc, err := deps.configFactory(ctx, addr, namespace, group)
	if err != nil {
		hbCancel()
		_ = reg.Unregister(ctx, instance)
		return nil, fmt.Errorf("[nacos] NewNacosConfig: %w", err)
	}
	dataId := cfg.Name + ".ops.yaml"
	applyOps := func(content string) {
		var ops handler.OpsConfig
		if err := yaml.Unmarshal([]byte(content), &ops); err != nil {
			logging.PrintfContext(ctx, "[nacos] ops yaml unmarshal failed (continuing): %v", err)
			return
		}
		if deps.opsLimiter != nil {
			deps.opsLimiter.Update(ops)
			logging.PrintfContext(ctx, "[nacos] ops applied: limit_count=%d burst=%d", ops.LimitCount, ops.Burst)
		}
	}
	if opsYaml, err := cc.GetConfig(ctx, dataId, group); err != nil {
		logging.PrintfContext(ctx, "[nacos] GetConfig(%s/%s) failed (continuing): %v", group, dataId, err)
	} else {
		logging.PrintfContext(ctx, "[nacos] ops config loaded: %s/%s, %d bytes", group, dataId, len(opsYaml))
		applyOps(opsYaml)
	}
	if cfg.Nacos.HotReload {
		if err := cc.ListenConfig(ctx, dataId, group, func(d, g, content string) error {
			logging.PrintfContext(ctx, "[nacos] [hot-reload] %s/%s changed, %d bytes", g, d, len(content))
			applyOps(content)
			return nil
		}); err != nil {
			logging.PrintfContext(ctx, "[nacos] ListenConfig failed (continuing): %v", err)
		}
	}
	rt := &NacosRuntime{Registry: reg, ConfigCenter: cc, Cancel: hbCancel}
	// 注册已成功 ⇒ 本实例可被网关发现（E2E-23 #6）
	rt.registered.Store(true)
	return rt, nil
}

func gitVersion() string {
	if v := os.Getenv("GIT_VERSION"); v != "" {
		return v
	}
	return "dev-build"
}
