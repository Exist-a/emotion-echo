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
	"emotion-echo-user-svc/internal/repository"
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

// Sprint 1 PR-4c-3: ResetPasswordHandler POST /api/v1/users/reset-password
// BFF 校验 verification-code 后调此端点（必须在 noAuth group 注册，因为调用者未登录）
func ResetPasswordHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.ResetPasswordReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.AuthErrorResp{Error: "validation: invalid body"})
			return
		}

		l := logic.NewAuthLogic(c.Request.Context(), svcCtx)
		resp, err := l.ResetPassword(&req)
		if err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, logic.ErrValidation):
				status = http.StatusBadRequest
			case errors.Is(err, logic.ErrInvalidCredentials), errors.Is(err, logic.ErrInvalidVerifyCode):
				// 合并返 401 防用户名枚举
				status = http.StatusUnauthorized
			}
			c.JSON(status, types.AuthErrorResp{Error: err.Error()})
			return
		}

		c.JSON(http.StatusOK, resp)
	}
}

// R-01 #1: VerifySecurityAnswerHandler POST /api/v1/users/verify-security-answer
// 供 BFF 找回密码流程验证密保答案（必须在 noAuth group 注册，因为调用者未登录）
func VerifySecurityAnswerHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req struct {
			Username      string `json:"username"`
			QuestionOrder int    `json:"questionOrder"`
			Answer        string `json:"answer"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, types.AuthErrorResp{Error: "validation: invalid body"})
			return
		}
		if req.Username == "" || req.Answer == "" {
			c.JSON(http.StatusBadRequest, types.AuthErrorResp{Error: "validation: username and answer are required"})
			return
		}
		if req.QuestionOrder < 1 || req.QuestionOrder > 2 {
			c.JSON(http.StatusBadRequest, types.AuthErrorResp{Error: "validation: questionOrder must be 1 or 2"})
			return
		}

		l := logic.NewAuthLogic(c.Request.Context(), svcCtx)
		err := l.VerifySecurityAnswerByUsername(req.Username, req.QuestionOrder, req.Answer)
		if err != nil {
			status := http.StatusInternalServerError
			switch {
			case errors.Is(err, logic.ErrValidation):
				status = http.StatusBadRequest
			case errors.Is(err, repository.ErrNotFound):
				status = http.StatusUnauthorized // 防枚举：用户不存在也返 401
			case errors.Is(err, logic.ErrSecurityAnswerMismatch):
				status = http.StatusUnauthorized
			}
			c.JSON(status, types.AuthErrorResp{Error: "security answer verification failed"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"success": true})
	}
}

// E2E-07: GetSecurityQuestionsHandler GET /api/v1/users/security-questions
// 返回用户的密保问题列表（不含答案）。防枚举：用户不存在返回空列表。
func GetSecurityQuestionsHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		username := c.Query("username")
		if username == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "username is required"})
			return
		}

		l := logic.NewAuthLogic(c.Request.Context(), svcCtx)
		questions, err := l.GetSecurityQuestionsByUsername(username)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
			return
		}

		c.JSON(http.StatusOK, gin.H{"questions": questions})
	}
}
