package middleware

import (
	"testing"

	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
)

// TestAuthMiddleware_ReturnsRestMiddleware 适配层调 shared 并返回 rest.Middleware
func TestAuthMiddleware_ReturnsRestMiddleware(t *testing.T) {
	mw := AuthMiddleware()
	if mw == nil {
		t.Fatalf("AuthMiddleware() should return non-nil")
	}
	// 验证返回类型与 shared 一致
	var _ sharedmw.RestMiddleware = mw
}

// TestCtxUserIDKey_Alias CtxUserIDKey 是 shared CtxUserIDKey 的别名（type alias）
func TestCtxUserIDKey_Alias(t *testing.T) {
	// 静态声明，证明 import 路径与类型别名可用
	var _ sharedmw.CtxUserIDKey
	var _ = CtxUserIDKey{}
}
