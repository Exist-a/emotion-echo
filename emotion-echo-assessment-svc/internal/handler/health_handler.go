package handler

import (
	"net/http"

	"emotion-echo-assessment-svc/internal/logic"
	"emotion-echo-assessment-svc/internal/svc"

	"github.com/gin-gonic/gin"
)

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

// HealthReadyHandler 就绪检查（无鉴权，D-29 新增）
//
// readiness 语义：依赖不可用时返 **503**，供 compose healthcheck 判定。
// 与 /health 共用同一套依赖检查（同一个 HealthLogic），差别仅在 HTTP 码 ——
// 两者的响应体 status 字段必然一致。
func HealthReadyHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		resp, err := logic.NewHealthLogic(c.Request.Context(), svcCtx).Health()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		code := http.StatusOK
		if resp.Status != logic.StatusOk {
			code = http.StatusServiceUnavailable
		}
		c.JSON(code, resp)
	}
}