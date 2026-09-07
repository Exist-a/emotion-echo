// Package handler — auth_handler_test.go
//
// Sprint 1 PR-4c-4：reset-password 端到端 HTTP 测试。
//
// 背景：PR-4c-3 的 ResetPasswordHandler 单测只覆盖 business logic（authlogic_test.go
// 里 l.ResetPassword() 直接调，跳过 HTTP binding + middleware）。commit msg 声明
// "实测 user-svc 7 包全 PASS"，但跑过 docker e2e 后发现：
//   - POST /api/v1/users/reset-password 在 user-svc 容器内被 GinAuthMiddleware
//     401 拦截，业务逻辑根本进不去（main.go:137-145 注释说"noAuth 必须在
//     auth middleware 之前注册"是错的——Gin 的 r.Use 是全局中间件栈）。
//   - login / register 同 noAuth 分组却能通过——bug 范围比预期窄，仅 reset-password
//     被拦（推测与 Gin 路由树中动态路由 /api/v1/users/:id 的优先级有关）。
//
// 本测试 RED 阶段要做到的：
//   1. ResetPasswordHandler 在 gin.New() + 全局 GinAuthMiddleware 环境下，
//      必须能进 handler 并按预期返回（非 401）。
//   2. 修复方案 RED 描述：要么 main.go 修 noAuth 路由的中间件继承，要么
//      GinAuthMiddleware 加白名单（/api/v1/users/login|/register|/reset-password）。
//
// 测试规模：3 用例
//   - happy path（前置 create user + reset）→ 200
//   - 不存在的 user → 401（业务 logic ErrNotFound）
//   - 缺字段 → 400（handler validation）
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	emotionmw "github.com/emotion-echo/shared/pkg/middleware"
	"emotion-echo-user-svc/internal/model"
	"emotion-echo-user-svc/internal/repository"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newResetPasswordRouter 复刻 main.go 的路由装配（包括 GinAuthMiddleware 全局中间件），
// 用来端到端验证 reset-password HTTP 路径。
//
// 关键点：必须挂 GinAuthMiddleware。否则测试只覆盖了 binding + handler，
// 漏掉了"中间件实际会不会拦"的契约——这正是 PR-4c-3 漏掉的。
func newResetPasswordRouter(repo repository.UserRepo) *gin.Engine {
	r := gin.New()

	// 复刻 main.go 4.5 节：noAuth 组先注册
	noAuth := r.Group("/api/v1/users")
	{
		noAuth.POST("/login", LoginHandler(newUserHandlerSvcCtx(repo)))
		noAuth.POST("/register", RegisterHandler(newUserHandlerSvcCtx(repo)))
		noAuth.POST("/reset-password", ResetPasswordHandler(newUserHandlerSvcCtx(repo)))
	}

	// 复刻 main.go 第 145 行：r.Use 全局中间件
	r.Use(emotionmw.GinAuthMiddleware())

	// 复刻 main.go 第 151-153 行：动态路由（这块才是触发 bug 的关键——与
	// /reset-password 同 namespace 注册了 :id 参数路由，Gin 路由树内部处理
	// 可能影响 noAuth 注册路径的中间件继承）
	r.GET("/health", HealthHandler(newUserHandlerSvcCtx(repo)))
	r.GET("/api/v1/users/me", GetMeHandler(newUserHandlerSvcCtx(repo)))
	r.PATCH("/api/v1/users/me", UpdateProfileHandler(newUserHandlerSvcCtx(repo)))
	r.GET("/api/v1/users/:id", GetUserByIdHandler(newUserHandlerSvcCtx(repo)))

	return r
}

func TestResetPasswordHandler_HTTP_NoAuth_AllowsRequest(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	// 前置：注册一个用户
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, &model.User{
		ID:           42,
		Username:     "alice",
		PasswordHash: strPtr("$2a$10$OLD.HASH.before"),
	}))

	r := newResetPasswordRouter(repo)

	body := `{"username":"alice","verificationCode":"123456","newPassword":"new-pwd-789"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/reset-password",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// RED 期望：200（handler 进了、业务通过、密码改了）
	// 实际会得到什么？
	//   - 如果中间件拦了：401 unauthorized: missing or invalid X-User-Id
	//   - 如果中间件没拦但 handler 报业务错：401 / 400 等
	require.Equal(t, http.StatusOK, w.Code,
		"reset-password must bypass auth middleware (callers have no token yet). got body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), "alice")
}

func TestResetPasswordHandler_HTTP_UserNotFound_Returns401(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	r := newResetPasswordRouter(repo)

	body := `{"username":"nonexistent","verificationCode":"000000","newPassword":"new-pwd-789"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/reset-password",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 期望：业务层 ErrInvalidCredentials（防用户名枚举合并返回）→ handler 映射 401。
	// 这里关键是断言非"missing or invalid X-User-Id"——后者是中间件拦的，
	// 会让 HTTP 路径走不到 handler。修正前的 RED 期望是"user not found"，
	// 实际 logic 用 ErrInvalidCredentials 合并了 not-found 防枚举。
	require.Equal(t, http.StatusUnauthorized, w.Code)
	assert.NotContains(t, w.Body.String(), "X-User-Id",
		"401 必须来自 handler 业务逻辑，不是 GinAuthMiddleware 拦的")
	assert.Contains(t, w.Body.String(), "invalid")
}

func TestResetPasswordHandler_HTTP_EmptyFields_Returns400(t *testing.T) {
	t.Parallel()
	repo := repository.NewInMemoryUserRepo()
	r := newResetPasswordRouter(repo)

	body := `{"username":"","verificationCode":"","newPassword":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/reset-password",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "validation")
}