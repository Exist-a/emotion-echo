package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

// TestHealthHandler_IncludesBuildInfo 钉住 /health 返回包含 version/build_time 字段
//
// E2E-F-130 同族 + F-99 经验（2026-09-21 IAB 实测 dev 跑旧代码）：
// 单看 /health 200 无法判定"容器跑的是不是修复后代码"，必须能从响应里读到 git SHA/时间。
// F-130 候选修法 ② 提到「版本端点自检」，本测试落地该版本端。
func TestHealthHandler_IncludesBuildInfo(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// main.go 入口走 NewHealthHandlerWithBuild；零参数版兼容但 version=空（开发态）
	r.GET("/health", NewHealthHandlerWithBuild(nil, 100*time.Millisecond, BuildInfo{
		Version:   "test-v1.2.3",
		BuildTime: "2026-09-24T12:00:00Z",
	}))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, 200, w.Code, "无下游时 /health 应 200 ok")
	body := w.Body.String()
	var resp map[string]any
	assert.NoError(t, json.Unmarshal([]byte(body), &resp))

	// 必须含 version/build_time 字段（即便下游列表为空）
	_, hasVersion := resp["version"]
	_, hasBuildTime := resp["build_time"]
	assert.True(t, hasVersion, "/health 必须含 version 字段；当前 body="+body)
	assert.True(t, hasBuildTime, "/health 必须含 build_time 字段；当前 body="+body)

	// version 应是非空字符串
	if v, ok := resp["version"].(string); ok {
		assert.NotEmpty(t, strings.TrimSpace(v), "version 必须非空")
		assert.Equal(t, "test-v1.2.3", v)
	}
	if t2, ok := resp["build_time"].(string); ok {
		assert.Equal(t, "2026-09-24T12:00:00Z", t2)
	}
}