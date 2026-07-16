package jwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Claims JWT 声明
type Claims struct {
	UserID int64  `json:"userId"`
	JTI    string `json:"jti"`
	jwt.RegisteredClaims
}

// JWT JWT 工具
type JWT struct {
	secret            []byte
	accessTokenExpire time.Duration
}

// New 创建 JWT 实例
func New(secret string, accessTokenExpire time.Duration) *JWT {
	return &JWT{
		secret:            []byte(secret),
		accessTokenExpire: accessTokenExpire,
	}
}

// GenerateAccessToken 生成访问令牌
func (j *JWT) GenerateAccessToken(userID int64) (string, error) {
	now := time.Now()
	jti := uuid.New().String()
	claims := Claims{
		UserID: userID,
		JTI:    jti,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        jti,
			ExpiresAt: jwt.NewNumericDate(now.Add(j.accessTokenExpire)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "emotion-echo",
			Audience:  jwt.ClaimStrings{"emotion-echo-client"},
			Subject:   fmt.Sprintf("%d", userID),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(j.secret)
}

// ParseToken 解析令牌
func (j *JWT) ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return j.secret, nil
	})

	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, fmt.Errorf("invalid token")
}

// IsTokenExpired 检查令牌是否过期
func IsTokenExpired(err error) bool {
	if err == nil {
		return false
	}
	// 使用 errors.Is 支持 wrapped error
	return errors.Is(err, jwt.ErrTokenExpired)
}
