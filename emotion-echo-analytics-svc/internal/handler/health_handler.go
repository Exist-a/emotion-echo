package handler

import (
	"net/http"

	"emotion-echo-analytics-svc/internal/logic"
	"emotion-echo-analytics-svc/internal/svc"

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