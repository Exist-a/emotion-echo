// Package handler — auth_handler.go
//
// Stage 33 PR-19a：user-svc 真实 login / register handler。
//
// 路由（在 main.go 注册时不挂 GinAuthMiddleware）：
//   - POST /api/v1/users/login    → LoginHandler
//   - POST /api/v1/users/register → RegisterHandler
package handler

import (
	"errors"
	"net/http"

	"emotion-echo-user-svc/internal/logic"
	"emotion-echo-user-svc/internal/svc"
	"emotion-echo-user-svc/internal/types"

	"github.com/gin-gonic/gin"
)

// LoginHandler POST /api/v1/users/login
func LoginHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.LoginReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.AuthErrorResp{Error: "validation: invalid body"})
			return
		}

		l := logic.NewAuthLogic(c.Request.Context(), svcCtx)
		resp, err := l.Login(&req)
		if err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, logic.ErrValidation):
				status = http.StatusBadRequest
			case errors.Is(err, logic.ErrInvalidCredentials):
				status = http.StatusUnauthorized
			}
			c.JSON(status, types.AuthErrorResp{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}

// RegisterHandler POST /api/v1/users/register
func RegisterHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.RegisterReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.AuthErrorResp{Error: "validation: invalid body"})
			return
		}

		l := logic.NewAuthLogic(c.Request.Context(), svcCtx)
		resp, err := l.Register(&req)
		if err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, logic.ErrValidation):
				status = http.StatusBadRequest
			case errors.Is(err, logic.ErrUsernameTaken):
				status = http.StatusConflict
			}
			c.JSON(status, types.AuthErrorResp{Error: err.Error()})
			return
		}

		c.JSON(http.StatusCreated, resp)
	}
}
