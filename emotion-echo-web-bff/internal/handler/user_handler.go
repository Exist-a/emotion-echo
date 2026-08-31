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
	"fmt"
	"net/http"
	"strconv"
	"time"

	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/session"

	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
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
	r.GET("/api/v1/user/profile", h.profile)
	r.GET("/api/v1/users/me", h.getMe)
	r.PATCH("/api/v1/users/me", h.updateMe)
	r.GET("/api/v1/users/:id", h.getByID)
}

// profile 前端 GET /user/profile 直接返回 UserInfo 形状（types/api.ts）
//
// mock 鉴权设计：BFF 签发 token（含 user_id/username），用户信息由 BFF 从 token
// 提供（user-svc 无 mock 用户记录，返回 401 会造成前端 401 处理循环清 token）。
// 真实用户信息待 user-svc 落地真实用户表后透传。
func (h *UserHandler) profile(c *gin.Context) {
	uid, ok := c.Request.Context().Value(sharedmw.CtxUserIDKey{}).(int64)
	if !ok || uid <= 0 {
		Fail(c, http.StatusUnauthorized, 1, "unauthorized: missing user id")
		return
	}
	OK(c, ProfileVM{
		ID:        fmt.Sprintf("%d", uid),
		Username:  "demo",
		Nickname:  "体验用户",
		Avatar:    "",
		Age:       nil,
		Config:    map[string]any{},
		CreatedAt: time.Now().Format(time.RFC3339),
	})
}

func (h *UserHandler) getMe(c *gin.Context) {
	ctx := session.WithRequestAuth(c)
	u, err := h.user.GetMe(ctx)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"user": u})
}

func (h *UserHandler) updateMe(c *gin.Context) {
	var req downstream.UpdateProfileReq
	if err := json.NewDecoder(c.Request.Body).Decode(&req); err != nil {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid body")
		return
	}
	ctx := session.WithRequestAuth(c)
	u, err := h.user.UpdateMe(ctx, req)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"user": u})
}

func (h *UserHandler) getByID(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		Fail(c, http.StatusBadRequest, 1, "validation: invalid user id")
		return
	}
	ctx := session.WithRequestAuth(c)
	u, err := h.user.GetByID(ctx, id)
	if err != nil {
		Fail(c, statusFor(err), 1, err.Error())
		return
	}
	OK(c, gin.H{"user": u})
}
