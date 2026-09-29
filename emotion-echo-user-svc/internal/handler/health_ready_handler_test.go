package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"emotion-echo-user-svc/internal/config"
	"emotion-echo-user-svc/internal/model"
	"emotion-echo-user-svc/internal/svc"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E-23 A 组 · 测试点 #8：D-29 决议的 readiness 端点契约。
//
// 契约（三条，缺一不可）：
//  1. 依赖可用   ⇒ HTTP 200 + status="ok"
//  2. 依赖不可用 ⇒ HTTP 503 + status="degraded"   ← 与 /health 的差异只在码
//  3. /health 依赖不可用时仍返 200                 ← liveness 恒 200，保兼容
//
// 第 3 条是 D-29 选方案 A 的核心：**不能为了"报得准"而让存量探针全挂**。
// 若 /health 也改成 503，apisix-seed 的 condition: service_healthy 会永不满足。

// pingErrRepo 只关心 Ping，其余方法 panic。
type pingErrRepo struct {
	pingErr error
}

func (f *pingErrRepo) Ping(ctx context.Context) error { return f.pingErr }
func (f *pingErrRepo) GetByID(ctx context.Context, id int64) (*model.User, error) {
	panic("健康检查测试不应调用 GetByID")
}
func (f *pingErrRepo) GetByUsername(ctx context.Context, u string) (*model.User, error) {
	panic("健康检查测试不应调用 GetByUsername")
}
func (f *pingErrRepo) Create(ctx context.Context, u *model.User) error {
	panic("健康检查测试不应调用 Create")
}
func (f *pingErrRepo) UpdateProfile(ctx context.Context, id int64, nickname *string, gender *int16, birthday *time.Time, avatarURL *string, cfg *model.JSONMap) error {
	panic("健康检查测试不应调用 UpdateProfile")
}
func (f *pingErrRepo) UsernameExists(ctx context.Context, u string) (bool, error) {
	panic("健康检查测试不应调用 UsernameExists")
}
func (f *pingErrRepo) UpdatePassword(ctx context.Context, id int64, h string) error {
	panic("健康检查测试不应调用 UpdatePassword")
}

func newHealthRouter(svcCtx *svc.ServiceContext) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/health", HealthHandler(svcCtx))
	r.GET("/health/ready", HealthReadyHandler(svcCtx))
	return r
}

func TestHealthHandlers_D29ReadinessContract(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		pingErr       error
		wantLiveCode  int
		wantReadyCode int
		wantStatus    string
	}{
		{
			name:          "dependency available: both endpoints 200 and ok",
			pingErr:       nil,
			wantLiveCode:  http.StatusOK,
			wantReadyCode: http.StatusOK,
			wantStatus:    "ok",
		},
		{
			// 🔴 核心断言：readiness 必须 503，liveness 必须仍 200。
			name:          "dependency down: readiness 503 degraded while liveness stays 200",
			pingErr:       errors.New("dial tcp 127.0.0.1:5432: connect: connection refused"),
			wantLiveCode:  http.StatusOK,
			wantReadyCode: http.StatusServiceUnavailable,
			wantStatus:    "degraded",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svcCtx := &svc.ServiceContext{
				Config:   config.Config{},
				UserRepo: &pingErrRepo{pingErr: tt.pingErr},
			}
			router := newHealthRouter(svcCtx)

			// --- liveness：恒 200 ---
			liveRec := httptest.NewRecorder()
			router.ServeHTTP(liveRec, httptest.NewRequest(http.MethodGet, "/health", nil))
			assert.Equal(t, tt.wantLiveCode, liveRec.Code,
				"/health 是 liveness，无论依赖如何都必须 200（D-29 保兼容的前提）")

			// --- readiness：承载真实判定 ---
			readyRec := httptest.NewRecorder()
			router.ServeHTTP(readyRec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
			assert.Equal(t, tt.wantReadyCode, readyRec.Code,
				"/health/ready 必须用状态码表达依赖状态，否则 compose 探针无法判定")

			// --- 两者响应体的 status 字段必须一致 ---
			var readyBody map[string]any
			require.NoError(t, json.Unmarshal(readyRec.Body.Bytes(), &readyBody),
				"readiness 响应必须是合法 JSON，实际：%s", readyRec.Body.String())
			assert.Equal(t, tt.wantStatus, readyBody["status"],
				"readiness 的 status 必须说真话")

			var liveBody map[string]any
			require.NoError(t, json.Unmarshal(liveRec.Body.Bytes(), &liveBody))
			assert.Equal(t, tt.wantStatus, liveBody["status"],
				"两个端点的 status 字段必须一致 —— 只有 HTTP 码该有差别")
		})
	}
}

// TestHealthReadyHandler_NotFoundOnUnknownSubPath 防止将来误加通配路由。
func TestHealthReadyHandler_NotFoundOnUnknownSubPath(t *testing.T) {
	t.Parallel()

	gin.SetMode(gin.TestMode)
	router := newHealthRouter(&svc.ServiceContext{Config: config.Config{}})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health/live", nil))

	assert.Equal(t, http.StatusNotFound, rec.Code,
		"/health/live 是未定义路径，不应命中 readiness 处理")
}
