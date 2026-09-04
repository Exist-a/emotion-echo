package handler

import (
	"sync"
)

// OpsConfig 是 web-bff.ops.yaml 反序列化结构。
//
// PR-4: HotReload 启用时，Nacos 推送 web-bff.ops.yaml 触发本结构更新，
// BFF 用最新值调整 limit-count / api-breaker 阈值。
type OpsConfig struct {
	// LimitCount 每 IP 每分钟请求上限
	LimitCount int `yaml:"limit_count" json:"limit_count"`
	// Burst 全局突发上限
	Burst int `yaml:"burst" json:"burst"`
}

// HotReloadLimiter 持有可热更新的限流参数。
//
// 线程安全：Update 由 Nacos 推送回调触发，Get 由请求中间件读。
type HotReloadLimiter struct {
	mu    sync.RWMutex
	limit int
	burst int
}

// NewHotReloadLimiter 构造默认。
func NewHotReloadLimiter(defaultLimit, defaultBurst int) *HotReloadLimiter {
	return &HotReloadLimiter{limit: defaultLimit, burst: defaultBurst}
}

// Update 热更新（来自 ListenConfig 回调）。
func (h *HotReloadLimiter) Update(cfg OpsConfig) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if cfg.LimitCount > 0 {
		h.limit = cfg.LimitCount
	}
	if cfg.Burst > 0 {
		h.burst = cfg.Burst
	}
}

// Snapshot 读当前值（中间件调用）。
func (h *HotReloadLimiter) Snapshot() (int, int) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.limit, h.burst
}