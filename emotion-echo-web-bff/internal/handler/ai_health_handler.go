// Package handler — ai_health_handler.go
//
// Sprint F2（2026-09-11）：ai-svc gRPC AIHealth 路由 handler
//
// 与 HTTP `GET /api/v1/ai/health` 语义对齐（ai-svc HTTP 端已存在 handler，
// 但 BFF 路由之前缺失，本次 Sprint 顺手补 + 接 gRPC client）。
package handler

import (
	"net/http"

	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/session"

	"github.com/gin-gonic/gin"
)

// AIHealthHandler 暴露 ai-svc AIHealth RPC（BFF 端）
type AIHealthHandler struct {
	ai downstream.AIClient
}

// NewAIHealthHandler 构造
func NewAIHealthHandler(ai downstream.AIClient) *AIHealthHandler {
	return &AIHealthHandler{ai: ai}
}

// Health GET /api/v1/ai/health
//
// 鉴权：BFF 路由层白名单（与 auth 白名单同源），调用方需带 X-User-Id
// （与 ai-svc gRPC userid 拦截器要求一致）。
//
// 返回 ai-svc 真实 health resp（FER/SenseVoice/XTTS 各自 enabled/healthy），
// HTTP status 跟随 resp.All：all=true → 200；all=false → 200 + body 标 unhealthy
// （避免 K8s liveness 把整个 svc kill；readiness 看 resp.All）。
func (h *AIHealthHandler) Health(c *gin.Context) {
	if h.ai == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "ai client not initialized"})
		return
	}
	resp, err := h.ai.AIHealth(session.WithRequestAuth(c))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	_ = resp
	status := http.StatusOK
	if !resp.AllHealthy {
		// 部分 AI 不可用 → 200 + body unhealthy（与 HTTP handler 行为一致）
	}
	c.JSON(status, resp)
}
