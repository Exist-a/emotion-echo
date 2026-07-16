// Package handler 提供 user-svc 的 HTTP handler（Gin 版本）
//
// handler 层只做参数解析 + 错误转译，业务逻辑全部委托给 internal/logic。
package handler

import (
	"errors"
	"net/http"
	"strconv"

	"emotion-echo-user-svc/internal/logic"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"
	"emotion-echo-user-svc/internal/types"

	"github.com/gin-gonic/gin"
)

// GetMeHandler 返回当前登录用户
//
// 鉴权由共享中间件 middleware.GinAuthMiddleware 完成（注入 user_id 到 ctx）
func GetMeHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := logic.NewGetMeLogic(c.Request.Context(), svcCtx).GetMe(&types.GetMeReq{})
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// UpdateProfileHandler PATCH /api/v1/users/me 修改当前用户资料
func UpdateProfileHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		var req types.UpdateProfileReq
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		resp, err := logic.NewUpdateProfileLogic(c.Request.Context(), svcCtx).UpdateProfile(&req)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// GetUserByIdHandler 按 ID 查询用户
func GetUserByIdHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
			return
		}

		resp, err := logic.NewGetUserByIdLogic(c.Request.Context(), svcCtx).GetUserById(&types.GetUserByIdReq{Id: id})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// HealthHandler 健康检查（无鉴权）
func HealthHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := logic.NewHealthLogic(c.Request.Context(), svcCtx).Health()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}