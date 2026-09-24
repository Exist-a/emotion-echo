// Package handler — health_handler.go
//
// Stage 30 / stage-30-web-bff.md T4.59-60: health handler（聚合下游探测）
//
// 端点：GET /health
// 响应：
//   {
//     "status": "ok",
//     "version": "git-sha 或 dev-build",
//     "build_time": "2026-09-24T12:34:56Z",
//     "downstream": {
//       "user": {"status":"ok"},
//       ...
//     }
//   }
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

// HealthHandler 聚合下游健康探测
type HealthHandler struct {
	targets []DownstreamTarget
	client  *http.Client
	build   BuildInfo
}

// NewHealthHandler 构造
// targets == nil 时不探测下游（仅返回 build 信息）；用于测试 / 启动早期自检。
func NewHealthHandler(targets []DownstreamTarget, timeout time.Duration) gin.HandlerFunc {
	return NewHealthHandlerWithBuild(targets, timeout, BuildInfo{})
}

// NewHealthHandlerWithBuild 含构建信息；main.go 用此入口传 version/build_time。
func NewHealthHandlerWithBuild(targets []DownstreamTarget, timeout time.Duration, build BuildInfo) gin.HandlerFunc {
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	h := &HealthHandler{targets: targets, client: &http.Client{Timeout: timeout}, build: build}
	return h.ServeHTTP
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

	resp.Status = "ok"
	for _, r := range resp.Downstream {
		if r.Status != "ok" {
			resp.Status = "degraded"
			break
		}
	}
	c.JSON(http.StatusOK, resp)
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
