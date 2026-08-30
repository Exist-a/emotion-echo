// Package handler — user_handler.go
//
// Stage 30 / stage-30-web-bff.md T4.35-37: user handler（BFF → user-svc）
//
// 端点：
//   GET   /api/v1/users/me    → {"user": UserInfo}
//   PATCH /api/v1/users/me    → {"user": UserInfo}
//   GET   /api/v1/users/:id   → {"user": UserInfo}
//
// 透传：请求 Authorization 头 → user-svc（WithRequestAuth → UserClient）。
package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/session"

	"github.com/gin-gonic/gin"
)

// UserHandler 处理 /api/v1/users/* 端点
type UserHandler struct {
	user downstream.UserClient
}

// NewUserHandler 构造
func NewUserHandler(user downstream.UserClient) *UserHandler {
	return &UserHandler{user: user}
}

// Register 注册路由
func (h *UserHandler) Register(r *gin.Engine) {
	r.GET("/api/v1/users/me", h.getMe)
	r.PATCH("/api/v1/users/me", h.updateMe)
	r.GET("/api/v1/users/:id", h.getByID)
}

func (h *UserHandler) getMe(c *gin.Context) {
	ctx := session.WithRequestAuth(c)
	u, err := h.user.GetMe(ctx)
	if err != nil {
		c.JSON(statusFor(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": u})
}

func (h *UserHandler) updateMe(c *gin.Context) {
	var req downstream.UpdateProfileReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation: invalid body"})
		return
	}
	ctx := session.WithRequestAuth(c)
	u, err := h.user.UpdateMe(ctx, req)
	if err != nil {
		c.JSON(statusFor(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": u})
}

func (h *UserHandler) getByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "validation: invalid user id"})
		return
	}
	ctx := session.WithRequestAuth(c)
	u, err := h.user.GetByID(ctx, id)
	if err != nil {
		c.JSON(statusFor(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": u})
}
