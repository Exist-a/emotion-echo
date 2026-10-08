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

// Claims 是 BFF 签发的 JWT 载荷
//
// Stage 38-A：APISIX jwt-auth 插件配 key_claim_name="user"（从 token 的 user claim
// 读 credential key）。本包 Claims 同时输出：
//   - "user": APISIX jwt-auth 凭据标识（所有 token 同值 "user"，匹配单一 consumer）
//   - "user_id": go-zero shared jwt_auth.go 用的字段（保留兼容）
//   - "key": APISIX 文档示例里 JWT payload 用的 claim 名（双写保险）
type Claims struct {
	UserID   int64  `json:"user_id,omitempty"`
	User     string `json:"user,omitempty"`    // APISIX jwt-auth key_claim_name=user 读此字段
	Key      string `json:"key,omitempty"`     // APISIX 默认 key_claim_name=key 读此字段
	Username string `json:"username,omitempty"`
	jwt.RegisteredClaims
}

// ErrInvalidToken token 无效或过期
var ErrInvalidToken = errors.New("auth: invalid token")

// Manager 签发/解析 JWT
//
// E2E-29 D-48（双密钥并存窗口）：`secret` 是**当前**签名密钥，`prevSecret`/`prevKeyID`
// 是轮换窗口期仍接受验签的**上一把**。签发一律用当前密钥 + 当前 key id；验签按 token 的
// key claim 选密钥（缺失/未知则依次试），从而"轮换不打断在途会话"。
type Manager struct {
	secret     []byte
	prevSecret []byte
	keyID      string
	prevKeyID  string
	ttl        time.Duration
}

// legacyKeyID 是历史上所有 token 使用的 key claim 值（APISIX consumer 的 key）。
// 不显式配置 key id 时保持该值 ⇒ 向后兼容既有 token 与既有 consumer。
const legacyKeyID = "user"

// NewManager 构造（secret 必须非空）。保持历史语义：key id = "user"、无上一把密钥。
func NewManager(secret string, ttlSeconds int) (*Manager, error) {
	return NewManagerMulti(secret, "", legacyKeyID, "", ttlSeconds)
}

// NewManagerMulti 构造双密钥 Manager（E2E-29 D-48）。
//
// 参数约束（任一不满足即报错，避免"配了一半"的窗口）：
//   - secret 非空、keyID 非空
//   - prevKeyID 与 keyID 必须不同（否则网关无法用 key claim 区分两把密钥）
//   - prevKeyID 与 prevSecret 必须**成对**出现（只给一个 = 配错）
func NewManagerMulti(secret, prevSecret, keyID, prevKeyID string, ttlSeconds int) (*Manager, error) {
	if secret == "" {
		return nil, errors.New("auth: JWT secret must not be empty")
	}
	if keyID == "" {
		return nil, errors.New("auth: JWT key id must not be empty")
	}
	if prevKeyID != "" && prevKeyID == keyID {
		return nil, errors.New("auth: previous JWT key id must differ from current key id")
	}
	if (prevKeyID == "") != (prevSecret == "") {
		return nil, errors.New("auth: previous JWT key id and secret must be provided together")
	}
	ttl := time.Duration(ttlSeconds) * time.Second
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Manager{
		secret:     []byte(secret),
		prevSecret: []byte(prevSecret),
		keyID:      keyID,
		prevKeyID:  prevKeyID,
		ttl:        ttl,
	}, nil
}

// Sign 为指定 user 签发 JWT（HS256）
//
// Stage 38-A：APISIX jwt-auth 插件配 key_claim_name="user"（读 token 的 user claim）。
// 同时保留 user_id（go-zero shared jwt_auth.go 用）。所有 token 把 user="user" 标成同一
// credential key——APISIX 用这一 key 找单一 consumer 验签；用户身份从 user_id claim 取。
func (m *Manager) Sign(userID int64, username string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID:   userID,
		User:     m.keyID, // APISIX jwt-auth key_claim_name=user 读此字段（= 凭据 key）
		Key:      m.keyID, // 同上（兼容 APISIX 默认 key_claim_name=key）
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

// Parse 解析并验证 JWT（返回 user_id）。
//
// E2E-29 D-48：双密钥窗口 —— 按 token 的 key claim 选密钥（命中 prevKeyID 用上一把），
// 未知/缺失则先试当前、再试上一把。**过期语义不变**：无论用哪把密钥，exp 过期一律拒绝。
func (m *Manager) Parse(tokenStr string) (int64, error) {
	if keyID := m.peekKeyID(tokenStr); keyID != "" && keyID == m.prevKeyID && len(m.prevSecret) > 0 {
		if uid, err := m.parseWith(tokenStr, m.prevSecret); err == nil {
			return uid, nil
		}
	}
	if uid, err := m.parseWith(tokenStr, m.secret); err == nil {
		return uid, nil
	}
	if len(m.prevSecret) > 0 {
		if uid, err := m.parseWith(tokenStr, m.prevSecret); err == nil {
			return uid, nil
		}
	}
	return 0, ErrInvalidToken
}

// parseWith 用指定密钥验签并取 user_id。
func (m *Manager) parseWith(tokenStr string, secret []byte) (int64, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method %v", ErrInvalidToken, t.Header["alg"])
		}
		return secret, nil
	})
	if err != nil {
		return 0, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if !token.Valid || claims.UserID == 0 {
		return 0, ErrInvalidToken
	}
	return claims.UserID, nil
}

// peekKeyID 不验签地读出 token 的 key claim（缺失时回落 user claim）。
// 仅用于"选哪把密钥"，不承担任何信任职责 —— 选错密钥只会让验签失败。
func (m *Manager) peekKeyID(tokenStr string) string {
	claims := &Claims{}
	if _, _, err := jwt.NewParser().ParseUnverified(tokenStr, claims); err != nil {
		return ""
	}
	if claims.Key != "" {
		return claims.Key
	}
	return claims.User
}

// TTL 返回 token 有效期（供 handler 计算 expiresIn）
func (m *Manager) TTL() time.Duration { return m.ttl }

// ResetTokenTTL 密保验证 token 有效期（5 分钟）
const ResetTokenTTL = 5 * time.Minute

// SignResetToken 签发密保验证短期 token（E2E-07）
//
// 用途：verify-security-answer 成功后签发，前端传给 reset-password 校验。
// 有效期 5 分钟，只含 username（不含 user_id，因为找回密码时用户未登录）。
func (m *Manager) SignResetToken(username string) (string, error) {
	now := time.Now()
	claims := Claims{
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ResetTokenTTL)),
			Subject:   "reset-password",
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(m.secret)
}

// ParseResetToken 解析密保验证 token（返回 username）
func (m *Manager) ParseResetToken(tokenStr string) (string, error) {
	// E2E-29 D-48：与 Parse 一样接受新旧两把密钥 —— reset token 只有 5 分钟寿命，
	// 若恰逢轮换窗口，用旧密钥签的那张必须仍能兑换（否则用户会莫名看到"重置令牌无效"）。
	secrets := [][]byte{m.secret}
	if len(m.prevSecret) > 0 {
		secrets = append(secrets, m.prevSecret)
	}
	for _, secret := range secrets {
		claims := &Claims{}
		token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("%w: unexpected signing method %v", ErrInvalidToken, t.Header["alg"])
			}
			return secret, nil
		})
		if err != nil {
			continue
		}
		if !token.Valid || claims.Username == "" || claims.Subject != "reset-password" {
			continue
		}
		return claims.Username, nil
	}
	return "", ErrInvalidToken
}
