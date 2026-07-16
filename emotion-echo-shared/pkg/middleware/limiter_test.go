package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// testGetUserID 从 ctx 提取 user_id（与 limiter.go 的回调签名一致）
func testGetUserID(c *gin.Context) int64 {
	v, ok := c.Request.Context().Value(CtxUserIDKey{}).(int64)
	if !ok {
		return 0
	}
	return v
}

// testReqWithUID 构造带 user_id 的 httptest.NewRequest
func testReqWithUID(uid int64) *http.Request {
	req := httptest.NewRequest("GET", "/x", nil)
	if uid > 0 {
		req = req.WithContext(context.WithValue(req.Context(), CtxUserIDKey{}, uid))
	}
	return req
}

func TestUserRateLimitMiddleware_AllowsBelowBurst(t *testing.T) {
	tb := NewTokenBucket(5, 5)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(UserRateLimitMiddleware(tb, testGetUserID))
	r.GET("/x", func(c *gin.Context) { c.String(200, "ok") })

	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, testReqWithUID(1))
		if w.Code != 200 {
			t.Errorf("req %d: want 200, got %d", i, w.Code)
		}
	}
}

func TestUserRateLimitMiddleware_RejectsOverBurst(t *testing.T) {
	// burst 3, rate 1/s
	tb := NewTokenBucket(1.0, 3)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(UserRateLimitMiddleware(tb, testGetUserID))
	r.GET("/x", func(c *gin.Context) { c.String(200, "ok") })

	var ok200, over429 int
	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, testReqWithUID(1))
		if w.Code == 200 {
			ok200++
		} else if w.Code == 429 {
			over429++
		}
	}

	if ok200 != 3 {
		t.Errorf("want exactly 3 OK (burst=3), got %d", ok200)
	}
	if over429 != 7 {
		t.Errorf("want 7 rejected (429), got %d", over429)
	}
}

func TestUserRateLimitMiddleware_PerUserIsolation(t *testing.T) {
	// burst 2 - 每用户独立桶
	tb := NewTokenBucket(0.1, 2)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(UserRateLimitMiddleware(tb, testGetUserID))
	r.GET("/x", func(c *gin.Context) { c.String(200, "ok") })

	// 用户 1 用满 2 个 token
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, testReqWithUID(1))
		if w.Code != 200 {
			t.Errorf("uid 1 req %d: want 200, got %d", i, w.Code)
		}
	}
	// 用户 2 也用满 2 个 token（独立桶，应该 OK）
	for i := 0; i < 2; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, testReqWithUID(2))
		if w.Code != 200 {
			t.Errorf("uid 2 req %d: want 200, got %d", i, w.Code)
		}
	}
	// 用户 1 第 3 次应该被拒
	w := httptest.NewRecorder()
	r.ServeHTTP(w, testReqWithUID(1))
	if w.Code != 429 {
		t.Errorf("uid 1 third req: want 429, got %d", w.Code)
	}
}

func TestUserRateLimitMiddleware_SkipsWhenNoUserID(t *testing.T) {
	tb := NewTokenBucket(1, 1)
	gin.SetMode(gin.TestMode)
	var handlerCalls int32
	r := gin.New()
	r.Use(UserRateLimitMiddleware(tb, testGetUserID))
	r.GET("/x", func(c *gin.Context) { atomic.AddInt32(&handlerCalls, 1); c.String(200, "ok") })

	// 100 个无 user_id 请求：全部放行（不计 user 维度）
	for i := 0; i < 100; i++ {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, testReqWithUID(0)) // uid=0 → 跳过
		if w.Code != 200 {
			t.Errorf("req %d: want 200 (no user), got %d", i, w.Code)
		}
	}
	if atomic.LoadInt32(&handlerCalls) != 100 {
		t.Errorf("handler called %d times, want 100", handlerCalls)
	}
}

func TestTokenBucket_Refills(t *testing.T) {
	tb := NewTokenBucket(100, 2)
	if !tb.Allow("u1") {
		t.Fatal("first should pass")
	}
	if !tb.Allow("u1") {
		t.Fatal("second should pass")
	}
	if tb.Allow("u1") {
		t.Fatal("third should fail (no refill yet)")
	}
	// 等待 50ms：100/s * 0.05 = 5 tokens
	time.Sleep(50 * time.Millisecond)
	if !tb.Allow("u1") {
		t.Fatal("after refill should pass")
	}
}

func TestUserRateLimitMiddleware_RateLimitHeaders(t *testing.T) {
	tb := NewTokenBucket(1, 1)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(UserRateLimitMiddleware(tb, testGetUserID))
	r.GET("/x", func(c *gin.Context) { c.String(200, "ok") })

	// 用满 1 个 token
	r.ServeHTTP(httptest.NewRecorder(), testReqWithUID(1))
	// 第 2 个 → 429
	w := httptest.NewRecorder()
	r.ServeHTTP(w, testReqWithUID(1))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("want 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Errorf("429 response should include Retry-After header")
	}
}