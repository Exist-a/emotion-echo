// Package auth — jwt.go
//
// Stage 30 / stage-30-web-bff.md T4.32-34: BFF 自有 mock 鉴权。
//
// 背景：前端调用 /auth/login、/auth/register 等端点，但所有 Go svc 都没有
// auth 路由（user-svc 仅有 users/me 等，鉴权 mock 且无登录）。BFF 作为
// 前端唯一入口，需自己签发 JWT，让前端登录 → 拿 token → 调 BFF → 透传下游。
//
// JWT 格式与 shared GinAuthMiddleware 兼容：
//   payload 含 user_id claim（GinAuthMiddleware 从 base64 解码取 user_id）
//
// 生产替换：接入真实认证（OAuth/DB 用户）时替换本包签发逻辑，handler 不变。
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// Claims 是 BFF 签发的 JWT 载荷（与 shared jwt_auth 兼容）
type Claims struct {
	UserID   int64  `json:"user_id"`
	Username string `json:"username,omitempty"`
	jwt.RegisteredClaims
}

// ErrInvalidToken token 无效或过期
var ErrInvalidToken = errors.New("auth: invalid token")

// Manager 签发/解析 JWT
type Manager struct {
	secret []byte
	ttl    time.Duration
}

// NewManager 构造（secret 必须非空）
func NewManager(secret string, ttlSeconds int) (*Manager, error) {
	if secret == "" {
		return nil, errors.New("auth: JWT secret must not be empty")
	}
	ttl := time.Duration(ttlSeconds) * time.Second
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Manager{secret: []byte(secret), ttl: ttl}, nil
}

// Sign 为指定 user 签发 JWT（HS256）
func (m *Manager) Sign(userID int64, username string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
			Subject:   fmt.Sprintf("%d", userID),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// Parse 解析并验证 JWT（返回 user_id）
func (m *Manager) Parse(tokenStr string) (int64, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method %v", ErrInvalidToken, t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if !token.Valid || claims.UserID == 0 {
		return 0, ErrInvalidToken
	}
	return claims.UserID, nil
}

// TTL 返回 token 有效期（供 handler 计算 expiresIn）
func (m *Manager) TTL() time.Duration { return m.ttl }
