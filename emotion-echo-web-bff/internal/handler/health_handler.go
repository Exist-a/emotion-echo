// Package handler — health_handler.go
//
// Stage 30 / stage-30-web-bff.md T4.59-60: health handler（聚合下游探测）
//
// 端点：GET /health
// 响应：
//
//	{
//	  "status": "ok",
//	  "version": "git-sha 或 dev-build",
//	  "build_time": "2026-09-24T12:34:56Z",
//	  "downstream": {
//	    "user": {"status":"ok"},
//	    ...
//	  }
//	}
//
// 实现：并发 GET 各下游 /health（带超时），单个下游失败不影响整体（标记 unhealthy）。
// 全部下游 ok → status: ok；任一失败 → status: degraded。
//
// E2E-F-130 / E2E-F-99（2026-09-21/24）：在响应里加 version + build_time，便于
// 一眼判定容器跑的是不是修复后代码（防 dev 跑旧 bundle 类 bug 复发）。
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// DownstreamTarget 是 health 探测的下游目标
type DownstreamTarget struct {
	Name    string
	BaseURL string
	Timeout time.Duration
}

// BuildInfo 描述构建信息（注入便于测试 + main 实注入 git SHA/build time）
type BuildInfo struct {
	Version   string
	BuildTime string
}

// Pinger 是健康探针的最小接口（Redis 等依赖实现之）。
//
// 刻意只要 Ping：健康检查需要的是"能不能连上"，不是业务能力。
type Pinger interface {
	Ping(ctx context.Context) error
}

// HealthDeps 是 /health 除下游之外还要覆盖的依赖（E2E-23 测试点 #5/#6）。
//
// 为什么需要它：改动前 /health 只探 6 个下游，于是"F-137 悖论"成立 ——
// **进程活着 + 下游都通 = 自查健康，但它自己在 Nacos 里没有实例**，
// APISIX 解析不到节点 ⇒ 全站 502。探针看不到自己"是否可被发现"。
//
// #5 的载体问题：dev 下 BFF 确有 Redis（E2E-20 的 LoginLockStore），
// 但 client 藏在 buildAuthLockStore() 内部、没进 ServiceContext，
// 结构上无法探 —— 不是忘了写，是没有注入位。
//
// 零值（两项皆 nil）= 不启用这些维度，向后兼容：老部署无 Redis 时
// 不得凭空报降级。
type HealthDeps struct {
	// Redis 探针；nil 表示该部署不使用 Redis，不参与判定
	Redis Pinger
	// NacosRegistered 返回本服务当前是否已注册到 Nacos；nil 表示不检查
	NacosRegistered func() bool
}

// HasDeps 是否配置了任何额外维度（决定响应里要不要带 deps 字段）。
func (d HealthDeps) HasDeps() bool { return d.Redis != nil || d.NacosRegistered != nil }

// HealthHandler 聚合下游健康探测。
//
// 语义（D-29）：本 handler 走 **liveness** 路径，下游全挂也返 HTTP 200；
// 需要按下游健康判定时用 NewHealthReadyHandler。两者共用同一套探测与
// status 计算，响应体必然一致，只有 HTTP 码不同。
type HealthHandler struct {
	targets []DownstreamTarget
	client  *http.Client
	build   BuildInfo
	// readiness 为 true 时，下游降级返 503 而非 200。
	readiness bool
	// deps 额外依赖探针（Redis / Nacos 注册状态）
	deps HealthDeps
}

// NewHealthHandler 构造 liveness handler（恒 200）。
// targets == nil 时不探测下游（仅返回 build 信息）；用于测试 / 启动早期自检。
func NewHealthHandler(targets []DownstreamTarget, timeout time.Duration) gin.HandlerFunc {
	return NewHealthHandlerWithBuild(targets, timeout, BuildInfo{})
}

// NewHealthHandlerWithBuild 含构建信息；main.go 用此入口传 version/build_time。
func NewHealthHandlerWithBuild(targets []DownstreamTarget, timeout time.Duration, build BuildInfo) gin.HandlerFunc {
	return newHealthHandler(targets, timeout, build, false)
}

// NewHealthReadyHandler 构造 readiness handler：任一下游降级即返 503。
// compose healthcheck 指向本端点，使"下游挂了"能真正影响容器健康判定
// （此前恒返 200，探针形同虚设 —— 见 plan §0 F-b）。
func NewHealthReadyHandler(targets []DownstreamTarget, timeout time.Duration) gin.HandlerFunc {
	return newHealthHandler(targets, timeout, BuildInfo{}, true)
}

func newHealthHandler(targets []DownstreamTarget, timeout time.Duration, build BuildInfo, readiness bool) gin.HandlerFunc {
	return newHealthHandlerWithDeps(targets, timeout, build, readiness, HealthDeps{})
}

func newHealthHandlerWithDeps(targets []DownstreamTarget, timeout time.Duration, build BuildInfo, readiness bool, deps HealthDeps) gin.HandlerFunc {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	h := &HealthHandler{
		targets:   targets,
		client:    &http.Client{Timeout: timeout},
		build:     build,
		readiness: readiness,
		deps:      deps,
	}
	return h.ServeHTTP
}

// NewHealthHandlerWithBuildAndDeps 带 build 信息与额外依赖探针的 liveness handler。
func NewHealthHandlerWithBuildAndDeps(targets []DownstreamTarget, timeout time.Duration, build BuildInfo, deps HealthDeps) gin.HandlerFunc {
	return newHealthHandlerWithDeps(targets, timeout, build, false, deps)
}

// NewHealthHandlerWithDeps 带额外依赖探针的 liveness handler。
func NewHealthHandlerWithDeps(targets []DownstreamTarget, timeout time.Duration, build BuildInfo, deps HealthDeps) gin.HandlerFunc {
	return newHealthHandlerWithDeps(targets, timeout, build, false, deps)
}

// NewHealthReadyHandlerWithDeps 带额外依赖探针的 readiness handler。
func NewHealthReadyHandlerWithDeps(targets []DownstreamTarget, timeout time.Duration, deps HealthDeps) gin.HandlerFunc {
	return newHealthHandlerWithDeps(targets, timeout, BuildInfo{}, true, deps)
}

type healthResult struct {
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
}

type healthResponse struct {
	Status     string                  `json:"status"`
	Version    string                  `json:"version"`
	BuildTime  string                  `json:"build_time"`
	Downstream map[string]healthResult `json:"downstream"`
	// Deps 额外依赖维度（Redis / Nacos 注册状态）；未配置时为 nil，不出现在 JSON 里
	Deps map[string]healthResult `json:"deps,omitempty"`
}

func (h *HealthHandler) ServeHTTP(c *gin.Context) {
	resp := healthResponse{
		Version:    h.build.Version,
		BuildTime:  h.build.BuildTime,
		Downstream: make(map[string]healthResult, len(h.targets)),
	}

	var wg sync.WaitGroup
	var mu sync.Mutex
	for _, t := range h.targets {
		wg.Add(1)
		go func(t DownstreamTarget) {
			defer wg.Done()
			result := h.probe(c.Request.Context(), t)
			mu.Lock()
			resp.Downstream[t.Name] = result
			mu.Unlock()
		}(t)
	}
	wg.Wait()

	// 额外依赖维度（E2E-23 #5/#6）。在下游之后评估，任一不通即降级。
	if h.deps.HasDeps() {
		resp.Deps = make(map[string]healthResult, 2)
	}
	if h.deps.Redis != nil {
		r := healthResult{Status: "ok"}
		if err := h.deps.Redis.Ping(c.Request.Context()); err != nil {
			r = healthResult{Status: "unhealthy", Detail: err.Error()}
		}
		resp.Deps["redis"] = r
	}
	if h.deps.NacosRegistered != nil {
		r := healthResult{Status: "ok"}
		if !h.deps.NacosRegistered() {
			// 进程活着但没注册 ⇒ 网关解析不到本实例（F-137 悖论）
			r = healthResult{Status: "not_registered",
				Detail: "not registered in Nacos: gateway cannot discover this instance"}
		}
		resp.Deps["nacos"] = r
	}

	resp.Status = "ok"
	for _, r := range resp.Downstream {
		if r.Status != "ok" {
			resp.Status = "degraded"
			break
		}
	}
	if resp.Status == "ok" {
		for _, r := range resp.Deps {
			if r.Status != "ok" {
				resp.Status = "degraded"
				break
			}
		}
	}

	// liveness 恒 200（D-29：存量消费方按 200 判定，改 503 会让
	// apisix-seed 的 condition: service_healthy 永不满足）；
	// readiness 则用 503 把下游降级传给 compose 探针。
	code := http.StatusOK
	if h.readiness && resp.Status != "ok" {
		code = http.StatusServiceUnavailable
	}
	c.JSON(code, resp)
}

// probe 探测单个下游 /health
func (h *HealthHandler) probe(ctx context.Context, t DownstreamTarget) healthResult {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.BaseURL+"/health", nil)
	if err != nil {
		return healthResult{Status: "unhealthy", Detail: err.Error()}
	}
	resp, err := h.client.Do(req)
	if err != nil {
		return healthResult{Status: "unhealthy", Detail: err.Error()}
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return healthResult{Status: "unhealthy", Detail: "status " + resp.Status}
	}
	// 可选：解析下游 health body（{"status":"ok"}），仅 status=ok 视为健康
	var body struct {
		Status string `json:"status"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body.Status != "" && body.Status != "ok" {
		return healthResult{Status: "unhealthy", Detail: "downstream status=" + body.Status}
	}
	return healthResult{Status: "ok"}
}
