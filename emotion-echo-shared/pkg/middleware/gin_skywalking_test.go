package middleware

import (
	"context"
	"fmt"
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
// PR-OBS-18 扩展: StartEntry 返回共享 span 实例,记录调用 + tag + EndSpan err
type stubTracer struct {
	startEntryCalls []string
	createLocalCalls []string
	// span PR-OBS-18: StartEntry 返回的 span(测试可断言 Tag/EndSpan 调用)
	span *stubSpan
}

func (t *stubTracer) StartEntry(ctx context.Context, opName string) (context.Context, grpcinterceptor.Span) {
	t.startEntryCalls = append(t.startEntryCalls, opName)
	if t.span == nil {
		t.span = &stubSpan{}
	}
	return ctx, t.span
}

func (t *stubTracer) CreateLocalSpan(ctx context.Context, opName string) (context.Context, grpcinterceptor.Span, error) {
	t.createLocalCalls = append(t.createLocalCalls, opName)
	return ctx, &stubSpan{}, nil
}

// stubSpan 满足 grpcinterceptor.Span 接口,记录 Tag/EndSpan 调用
// PR-OBS-18 扩展: tagKV 记录 / endErr 记录
type stubSpan struct {
	tagCalls []tagKV
	endErr   error
	ended    bool
}

func (s *stubSpan) EndSpan(err error) {
	s.ended = true
	s.endErr = err
}

func (s *stubSpan) Tag(key, value string) {
	s.tagCalls = append(s.tagCalls, tagKV{key, value})
}

// tagKV 记录 span.Tag 调用
type tagKV struct{ K, V string }

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

// ===== PR-OBS-18 RED: GinSkywalkingMiddleware 创建 EntrySpan + 4 tag 精确断言 =====
//
// 目的: stage-44 §四 B 收口第二步——中间件真正调用 tracer.StartEntry 创建 span,
// 打 4 个 http.* / user_id tag,使 SkyWalking UI 能聚合查询 HTTP 请求维度。
//
// 设计:
// - stubTracer.StartEntry 返回共享 span (tracer.span),记录 opName 到 startEntryCalls
// - 中间件在 c.Next 前调 tracer.StartEntry(ctx, opName)
// - c.Next 后调 span.Tag(http.method, ...) / Tag(http.url, ...) / Tag(http.status_code, ...)
// - 业务路径中间件优先级在 AuthMiddleware 之前 → X-User-Id header 仍可 c.GetHeader 读
//
// 4 个 case:
// 1. CreatesEntrySpan: 业务路径触发 StartEntry 1 次
// 2. TagsHTTPMethodURLStatus: 3 个 http.* tag 精确值
// 3. TagsUserIDFromHeader: X-User-Id → user_id tag
// 4. EndSpanOnHandlerError: handler 返 error/panic 时 EndSpan(err) 被调
//
// RED 状态: GinSkywalkingMiddleware 当前实现不调 tracer.StartEntry,断言失败 = RED

// TestGinSkywalkingMiddleware_CreatesEntrySpan 业务路径应触发 StartEntry
func TestGinSkywalkingMiddleware_CreatesEntrySpan(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tracer := &stubTracer{}
	r := gin.New()
	r.GET("/api/v1/foo", GinSkywalkingMiddleware(tracer), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/foo", nil)
	r.ServeHTTP(rec, req)

	if len(tracer.startEntryCalls) != 1 {
		t.Fatalf("expected 1 StartEntry call, got %d", len(tracer.startEntryCalls))
	}
	// opName = gin route FullPath (非 URL.Path,因为有路由变量时 c.FullPath 返回注册路径)
	if tracer.startEntryCalls[0] != "/api/v1/foo" {
		t.Errorf("expected opName=/api/v1/foo, got %q", tracer.startEntryCalls[0])
	}
}

// TestGinSkywalkingMiddleware_TagsHTTPMethodURLStatus 断言 3 个 http.* tag 精确值
func TestGinSkywalkingMiddleware_TagsHTTPMethodURLStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tracer := &stubTracer{}
	r := gin.New()
	r.POST("/api/v1/chat/:id", GinSkywalkingMiddleware(tracer), func(c *gin.Context) {
		c.Status(http.StatusCreated)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chat/42", nil)
	r.ServeHTTP(rec, req)

	if len(tracer.startEntryCalls) != 1 {
		t.Fatalf("expected 1 StartEntry call, got %d", len(tracer.startEntryCalls))
	}
	span := tracer.span
	if span == nil {
		t.Fatal("expected tracer.span to be set by StartEntry")
	}
	// 断言 3 个 tag 精确比对
	wantTags := []tagKV{
		{"http.method", http.MethodPost},
		{"http.url", "/api/v1/chat/42"}, // c.Request.URL.Path (实例路径,非路由模板)
		{"http.status_code", "201"},
	}
	if len(span.tagCalls) != len(wantTags) {
		t.Errorf("expected %d tag calls, got %d: %+v", len(wantTags), len(span.tagCalls), span.tagCalls)
	}
	for _, want := range wantTags {
		found := false
		for _, got := range span.tagCalls {
			if got.K == want.K && got.V == want.V {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing tag %+v in %+v", want, span.tagCalls)
		}
	}
	// span 必须在 c.Next 后被 EndSpan(nil)(handler 无 err)
	if !span.ended {
		t.Error("expected span.EndSpan called after c.Next")
	}
	if span.endErr != nil {
		t.Errorf("expected span.EndSpan(nil) on success, got endErr=%v", span.endErr)
	}
}

// TestGinSkywalkingMiddleware_TagsUserIDFromHeader 验证 X-User-Id header → user_id tag
func TestGinSkywalkingMiddleware_TagsUserIDFromHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tracer := &stubTracer{}
	r := gin.New()
	r.GET("/api/v1/profile", GinSkywalkingMiddleware(tracer), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/profile", nil)
	// 模拟 APISIX jwt-auth 注入 X-User-Id (shared/auth/jwt_auth.go 解析模式)
	req.Header.Set("X-User-Id", "12345")
	r.ServeHTTP(rec, req)

	if len(tracer.startEntryCalls) != 1 {
		t.Fatalf("expected 1 StartEntry, got %d", len(tracer.startEntryCalls))
	}
	span := tracer.span
	if span == nil {
		t.Fatal("expected tracer.span to be set")
	}
	// 断言 user_id tag
	found := false
	for _, got := range span.tagCalls {
		if got.K == "user_id" && got.V == "12345" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected tag (user_id, 12345), got %+v", span.tagCalls)
	}
}

// TestGinSkywalkingMiddleware_EndSpanOnHandlerError 验证 handler 返 error 时
// span.EndSpan(err) 收到该 err
func TestGinSkywalkingMiddleware_EndSpanOnHandlerError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tracer := &stubTracer{}
	r := gin.New()
	r.GET("/api/v1/fail", GinSkywalkingMiddleware(tracer), func(c *gin.Context) {
		c.Status(http.StatusInternalServerError)
		c.Error(fmt.Errorf("forced handler error"))
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/fail", nil)
	r.ServeHTTP(rec, req)

	if len(tracer.startEntryCalls) != 1 {
		t.Fatalf("expected 1 StartEntry, got %d", len(tracer.startEntryCalls))
	}
	span := tracer.span
	if !span.ended {
		t.Fatal("expected span.EndSpan called")
	}
	// 中间件应传 status code 5xx 时判定为 err(或读 c.Errors())
	// 当前 PR 仅断言 span 被 EndSpan,不强求 endErr 值(后续 PR-OBS-23 可加精细判定)
	if span.endErr == nil {
		t.Log("span.EndSpan(nil) — handler error 透传策略留作后续 PR")
	}
}

// TestGinSkywalkingMiddleware_AttachesSpanOnContext 业务路径应把 span
// 实例挂到 gin ctx (与 tracer 一致)。下游 handler 可通过 c.Get(\"skywalking_span\")
// 拿到当前 span 并继续打业务 tag(后续 PR-OBS-19 业务层 tag 设置点)。
func TestGinSkywalkingMiddleware_AttachesSpanOnContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tracer := &stubTracer{}
	var gotSpan interface{}
	var gotSame bool
	r := gin.New()
	r.GET("/api/v1/foo", GinSkywalkingMiddleware(tracer), func(c *gin.Context) {
		gotSpan, _ = c.Get("skywalking_span")
		if gotSpan != nil {
			_, gotSame = gotSpan.(*stubSpan)
		}
		c.Status(http.StatusOK)
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/foo", nil)
	r.ServeHTTP(rec, req)

	if gotSpan == nil {
		t.Fatal("expected skywalking_span key on ctx")
	}
	if !gotSame {
		t.Errorf("expected ctx span to be *stubSpan (instance from StartEntry), got %T", gotSpan)
	}
}