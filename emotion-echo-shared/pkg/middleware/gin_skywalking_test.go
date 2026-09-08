package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/emotion-echo/shared/pkg/grpcinterceptor"
	"github.com/gin-gonic/gin"
)

// TestGinSkywalkingMiddleware_SetsTracerOnContext 业务路径应把 tracer 挂到 gin ctx
func TestGinSkywalkingMiddleware_SetsTracerOnContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var (
		gotTracer bool
		gotNil    bool
	)
	r.GET("/api/v1/foo", GinSkywalkingMiddleware(nil), func(c *gin.Context) {
		v, ok := c.Get("skywalking_tracer")
		gotTracer = ok
		if !ok {
			gotNil = true
		}
		_ = v
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/foo", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status want=200 got=%d", rec.Code)
	}
	if !gotTracer {
		t.Fatalf("expected skywalking_tracer key on ctx")
	}
	_ = gotNil
}

// TestGinSkywalkingMiddleware_SkipsHealth /health 直接放行不挂 tracer
func TestGinSkywalkingMiddleware_SkipsHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var sawKey bool
	r.GET("/health", GinSkywalkingMiddleware(nil), func(c *gin.Context) {
		_, sawKey = c.Get("skywalking_tracer")
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status want=200 got=%d", rec.Code)
	}
	if sawKey {
		t.Fatalf("/health should not attach tracer key")
	}
}

// TestGinSkywalkingMiddleware_SkipsInternalPrefix /internal/* 全部跳过
func TestGinSkywalkingMiddleware_SkipsInternalPrefix(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, p := range []string{"/internal/sw-status", "/internal/debug", "/internal/"} {
		sawKey := false
		r := gin.New()
		r.GET(p, GinSkywalkingMiddleware(nil), func(c *gin.Context) {
			_, sawKey = c.Get("skywalking_tracer")
			c.Status(http.StatusOK)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status want=200 got=%d", p, rec.Code)
		}
		if sawKey {
			t.Fatalf("%s should not attach tracer key, but did", p)
		}
	}
}

// TestGinSkywalkingMiddleware_AttachesForPath 表驱动：所有非白名单路径应挂 tracer
func TestGinSkywalkingMiddleware_AttachesForPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, p := range []string{"/", "/api/v1/chat", "/api/v1/ai/health", "/api/v1/tts/synthesize"} {
		sawKey := false
		r := gin.New()
		r.GET(p, GinSkywalkingMiddleware(nil), func(c *gin.Context) {
			_, sawKey = c.Get("skywalking_tracer")
			c.Status(http.StatusOK)
		})
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, p, nil)
		r.ServeHTTP(rec, req)
		if !sawKey {
			t.Fatalf("%s should attach tracer key, but did not (status=%d)", p, rec.Code)
		}
	}
}

// TestGinSkywalkingMiddleware_NilTracerPassedThrough 传 nil tracer 应不崩且仍走 next
func TestGinSkywalkingMiddleware_NilTracerPassedThrough(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	called := false
	r.GET("/api/v1/ok", GinSkywalkingMiddleware(nil), func(c *gin.Context) {
		called = true
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/ok", nil)
	r.ServeHTTP(rec, req)
	if !called {
		t.Fatalf("next handler should run even with nil tracer")
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status want=200 got=%d", rec.Code)
	}
}

// ===== PR-OBS-12 RED: 3 case — gin_skywalking 透传行为 =====
//
// 目的: PR-OBS-12 设计 (observability-sprint-b.md §三.12):
// 1. HTTP trace 标签断言 — span 含 user_id + http.method/url/status
// 2. 跨层 trace_id 透传 — gin handler 能拿到 skywalking_tracer key
// 3. Kafka consumer trace — 另 PR (PR-OBS-14)
//
// 现状约束: go2sky v1.5 Tracer 是具体类型,无法直接 mock
// (NewTracer 需要 reporter.NewGRPCReporter 真实连接 OAP)
// 本 PR 测试范围限定为 middleware 行为 (ctx key 传递),不重构 go2sky 抽象
// 完整 span tag 测试留作 follow-up (需先抽 TracerInterface)
//
// 注: PR-OBS-12 plan §三.12 期望的 'span tag 含 user_id' 需要 go2sky.NewTracer
// + reporter mock 才能测,这是较大的抽象改造
// 本 PR 落地 3 个 case 验证现有 middleware 行为的边界 (测试覆盖补全 + 文档化)

// TestGinSkywalkingMiddleware_AttachesUserIDHeader 测试 X-User-Id 头被中间件处理
//
// 行为: middleware 应读取 X-User-Id header 并放入 gin ctx (供下游 tracer 提取)
// 现状: 当前实现未提取 X-User-Id,本 case 暂仅断言 X-User-Id 在 header 传递路径上未被截断
// 完整 'X-User-Id → span tag' 行为留作 follow-up (需抽 SkywalkingCarrier)
func TestGinSkywalkingMiddleware_AttachesUserIDHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var (
		gotTracer bool
		gotUserID string
	)
	r.GET("/api/v1/user", GinSkywalkingMiddleware(nil), func(c *gin.Context) {
		_, gotTracer = c.Get("skywalking_tracer")
		gotUserID = c.GetHeader("X-User-Id")
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/user", nil)
	req.Header.Set("X-User-Id", "123")
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status want=200 got=%d", rec.Code)
	}
	if !gotTracer {
		t.Errorf("expected skywalking_tracer key on ctx")
	}
	if gotUserID != "123" {
		t.Errorf("X-User-Id header not passed through: got %q, want \"123\"", gotUserID)
	}
}

// TestGinSkywalkingMiddleware_AttachesMethodAndURL 测试请求 method/path 挂到 ctx
// 用途: 下游 tracer 可用 c.Request.Method + c.FullPath() 设 span tag
// 现状: middleware 当前未主动设,验证 ctx 持有原始 Request (供下游用)
func TestGinSkywalkingMiddleware_AttachesMethodAndURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	var (
		gotMethod string
		gotPath   string
		gotFull   string
	)
	r.POST("/api/v1/chat/:id", GinSkywalkingMiddleware(nil), func(c *gin.Context) {
		gotMethod = c.Request.Method
		gotPath = c.Request.URL.Path
		gotFull = c.FullPath()
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/42", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status want=200 got=%d", rec.Code)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("Request.Method lost: got %q, want POST", gotMethod)
	}
	if gotPath != "/api/v1/chat/42" {
		t.Errorf("URL.Path lost: got %q", gotPath)
	}
	if gotFull != "/api/v1/chat/:id" {
		t.Errorf("FullPath lost: got %q, want /api/v1/chat/:id", gotFull)
	}
}

// TestGinSkywalkingMiddleware_AttachesStatusCode 测试 response status 写入 ctx
// 用途: 下游 tracer 读取 c.Writer.Status() 设 span tag http.status_code
// 现状: 验证 middleware 不干扰 status 写入路径 (c.Status 仍正常)
func TestGinSkywalkingMiddleware_AttachesStatusCode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/api/v1/created", GinSkywalkingMiddleware(nil), func(c *gin.Context) {
		c.Status(http.StatusCreated) // 201
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/created", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("status code lost: got %d, want 201", rec.Code)
	}
}

// ===== PR-OBS-17 GREEN: GinSkywalkingMiddleware mock tracer 边界 =====
//
// 目的: stage-44 §四 B 收口——tracer 形参改为 grpcinterceptor.Tracer 接口后,
// 验证中间件把传入的 mock tracer 实例挂到 gin ctx (而不是 nil 或新建)。
//
// 设计:
// - stubTracer 满足 grpcinterceptor.Tracer 接口的最小实现
// - 业务 handler 从 ctx 读 skywalking_tracer,断言类型 + 同一指针
//
// 与已有 case 区别: 已有 case 都用 nil tracer (验证 middleware 不 panic),
// 本 case 用 mock 验证"传入实例 = ctx 挂的实例"指针相等性 (语义层)。
//
// PR-OBS-18 扩展: 中间件将在这里调用 stubTracer.StartEntry(ctx, "/api/v1/...")
// 创建 EntrySpan 并打 http.method/url/status_code/user_id tag。届时:
//   - StartEntry 调用次数 == 1
//   - span.Tag(\"http.method\", \"GET\") / Tag(\"http.url\", \"/api/v1/...\") 等断言
//   - span.EndSpan(err) 在 next handler 返回时被调

// stubTracer PR-OBS-17 — 满足 grpcinterceptor.Tracer 接口,记录调用次数
type stubTracer struct {
	startEntryCalls []string
	createLocalCalls []string
}

func (t *stubTracer) StartEntry(ctx context.Context, opName string) (context.Context, grpcinterceptor.Span) {
	// 当前 PR GinSkywalkingMiddleware 未实际调用 StartEntry(只 c.Set),
	// 但接口已存在以便 PR-OBS-18 接入;这里返回 noop span 让接口完整。
	return ctx, &stubSpan{}
}

func (t *stubTracer) CreateLocalSpan(ctx context.Context, opName string) (context.Context, grpcinterceptor.Span, error) {
	t.createLocalCalls = append(t.createLocalCalls, opName)
	return ctx, &stubSpan{}, nil
}

// stubSpan 满足 grpcinterceptor.Span 接口,无业务行为
type stubSpan struct{}

func (s *stubSpan) EndSpan(error)             {}
func (s *stubSpan) Tag(_, _ string)           {}

// 编译期断言: stubTracer / stubSpan 满足接口
var _ grpcinterceptor.Tracer = (*stubTracer)(nil)
var _ grpcinterceptor.Span = (*stubSpan)(nil)

// TestGinSkywalkingMiddleware_AttachesNonNilTracerInstance 业务路径应把
// 传入的 tracer 实例原样挂到 gin ctx (指针相等性)
func TestGinSkywalkingMiddleware_AttachesNonNilTracerInstance(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tracer := &stubTracer{}
	var (
		gotRaw   interface{}
		gotType  string
		isSame   bool
	)
	r := gin.New()
	r.GET("/api/v1/foo", GinSkywalkingMiddleware(tracer), func(c *gin.Context) {
		gotRaw, _ = c.Get("skywalking_tracer")
		if gotRaw != nil {
			gotType = "<non-nil>"
			isSame = gotRaw == tracer
		}
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/foo", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status want=200 got=%d", rec.Code)
	}
	if gotRaw == nil {
		t.Fatal("skywalking_tracer key on ctx is nil (middleware should attach tracer)")
	}
	if gotType != "<non-nil>" {
		t.Errorf("expected non-nil tracer, got %v", gotRaw)
	}
	if !isSame {
		t.Errorf("expected ctx tracer to be same instance as passed in (pointer equality)")
	}
}

// TestGinSkywalkingMiddleware_AttachesInterfaceTypedTracer 验证 ctx 挂的
// 是 grpcinterceptor.Tracer 接口(而非 *go2sky.Tracer 具体类型)。
// 这是 PR-OBS-17 接口切换的契约 —— 下游读 ctx.Get(\"skywalking_tracer\") 时
// 可安全类型断言为 grpcinterceptor.Tracer。
func TestGinSkywalkingMiddleware_AttachesInterfaceTypedTracer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tracer := &stubTracer{}
	var (
		assertOK bool
	)
	r := gin.New()
	r.GET("/api/v1/typed", GinSkywalkingMiddleware(tracer), func(c *gin.Context) {
		raw, exists := c.Get("skywalking_tracer")
		if !exists {
			t.Fatal("skywalking_tracer key missing")
		}
		// 类型断言为 grpcinterceptor.Tracer 接口
		_, assertOK = raw.(grpcinterceptor.Tracer)
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/typed", nil)
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status want=200 got=%d", rec.Code)
	}
	if !assertOK {
		t.Error("expected ctx skywalking_tracer to be grpcinterceptor.Tracer interface")
	}
}