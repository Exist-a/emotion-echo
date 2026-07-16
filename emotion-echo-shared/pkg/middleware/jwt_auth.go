// Package middleware 提供 Emotion-Echo 各 Go svc 的共享 HTTP 中间件
//
// AuthMiddleware 从 Authorization header 解析 JWT（已被 APISIX jwt-auth 验过），
// 提取 user_id claim，注入到 ctx。
//
// 流程：
//   浏览器 → APISIX jwt-auth 验证 token → 通过后透传到 svc
//          → svc 信任 APISIX（不再次验证 signature）
//          → svc base64 解码 JWT payload，取 user_id claim
//
// 这样 svc 端不需要共享 JWT secret，符合"边界信任"原则。
package middleware

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/zeromicro/go-zero/rest"
)

// RestMiddleware 是 go-zero REST 框架的中间件类型别名
// （shared 包依赖 go-zero，所有 svc 也都用 go-zero）
type RestMiddleware = rest.Middleware

// CtxUserIDKey 是 context 中 user id 的 key
type CtxUserIDKey struct{}

// AuthMiddleware 信任 APISIX 已验证的 JWT，从 Authorization 头解析 user_id
func AuthMiddleware() rest.Middleware {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// 跳过 health 端点（monitoring 不需要鉴权）
			if r.URL.Path == "/health" {
				next(w, r)
				return
			}

			uid, err := extractUserIDFromJWT(r.Header.Get("Authorization"))
			if err != nil || uid <= 0 {
				http.Error(w, `{"error":"unauthorized: invalid or missing JWT"}`, http.StatusUnauthorized)
				return
			}
			ctx := context.WithValue(r.Context(), CtxUserIDKey{}, uid)
			next.ServeHTTP(w, r.WithContext(ctx))
		}
	}
}

// UserIDFromContext 从 context 取出 user_id
func UserIDFromContext(ctx context.Context) (int64, bool) {
	uid, ok := ctx.Value(CtxUserIDKey{}).(int64)
	return uid, ok
}

// extractUserIDFromJWT 从 "Bearer xxx" 头提取 user_id claim
// 不验证签名（APISIX 已验过），只解析 payload
func extractUserIDFromJWT(authHeader string) (int64, error) {
	if authHeader == "" {
		return 0, errMissingJWT
	}
	// 提取 "Bearer <token>"
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return 0, errInvalidJWT
	}
	token := parts[1]

	// JWT 格式：header.payload.signature
	segs := strings.Split(token, ".")
	if len(segs) != 3 {
		return 0, errInvalidJWT
	}

	// 解码 payload（base64 url-safe）
	payload, err := base64.RawURLEncoding.DecodeString(segs[1])
	if err != nil {
		return 0, errInvalidJWT
	}

	var claims struct {
		UserID int64  `json:"user_id"`
		Sub    string `json:"sub"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return 0, errInvalidJWT
	}

	// 兼容：如果 user_id 是字符串（sub 字段），转 int64
	if claims.UserID == 0 && claims.Sub != "" {
		if id, err := strconv.ParseInt(claims.Sub, 10, 64); err == nil {
			claims.UserID = id
		}
	}
	return claims.UserID, nil
}

var (
	errMissingJWT = &jwtError{"missing Authorization header"}
	errInvalidJWT = &jwtError{"invalid JWT format"}
)

type jwtError struct{ msg string }

func (e *jwtError) Error() string { return e.msg }