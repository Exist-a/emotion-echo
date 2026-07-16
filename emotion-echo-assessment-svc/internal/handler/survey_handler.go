package handler

import (
	"errors"
	"net/http"
	"strconv"

	"emotion-echo-assessment-svc/internal/logic"
	"emotion-echo-assessment-svc/internal/repository"
	"emotion-echo-assessment-svc/internal/svc"
	"emotion-echo-assessment-svc/internal/types"

	"github.com/gin-gonic/gin"
)

// ListSurveysHandler GET /api/v1/surveys
func ListSurveysHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := 50
		if v := c.Query("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 200 {
				limit = n
			}
		}
		resp, err := logic.NewListSurveysLogic(c.Request.Context(), svcCtx).ListSurveys(&types.ListSurveysReq{Limit: limit})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// GetSurveyHandler GET /api/v1/surveys/:id
func GetSurveyHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid survey id"})
			return
		}
		resp, err := logic.NewGetSurveyLogic(c.Request.Context(), svcCtx).GetSurvey(&types.GetSurveyReq{Id: id})
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "survey not found"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// SubmitSurveyHandler POST /api/v1/surveys/:id/submit
func SubmitSurveyHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("id")
		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid survey id"})
			return
		}

		var req types.SubmitSurveyReq
		req.SurveyId = id
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		resp, err := logic.NewSubmitSurveyLogic(c.Request.Context(), svcCtx).SubmitSurvey(&req)
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "survey not found"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// GetSurveyResultHandler GET /api/v1/surveys/results/:resultId
func GetSurveyResultHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		idStr := c.Param("resultId")
		id, err := strconv.ParseUint(idStr, 10, 64)
		if err != nil || id == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid result id"})
			return
		}
		resp, err := logic.NewGetSurveyResultLogic(c.Request.Context(), svcCtx).GetSurveyResult(&types.GetSurveyResultReq{ResultId: id})
		if err != nil {
			if errors.Is(err, repository.ErrNotFound) {
				c.JSON(http.StatusNotFound, gin.H{"error": "result not found"})
				return
			}
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}

// ListMyResultsHandler GET /api/v1/surveys/results
func ListMyResultsHandler(svcCtx *svc.ServiceContext) gin.HandlerFunc {
	return func(c *gin.Context) {
		limit := 20
		if v := c.Query("limit"); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
				limit = n
			}
		}
		resp, err := logic.NewGetSurveyResultLogic(c.Request.Context(), svcCtx).ListMyResults(&types.ListMyResultsReq{Limit: limit})
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, resp)
	}
}