// Package auth — jwt_test.go
//
// Stage 30 / stage-30-web-bff.md T4.32 RED: JWT 签发/解析契约测试
package auth

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestManager_SignParse_RoundTrip(t *testing.T) {
	m, err := NewManager("test-secret", 3600)
	require.NoError(t, err)

	token, err := m.Sign(42, "alice")
	require.NoError(t, err)
	require.NotEmpty(t, token)

	uid, err := m.Parse(token)
	require.NoError(t, err)
	assert.Equal(t, int64(42), uid)
}

func TestManager_Parse_ExpiredToken_ReturnsError(t *testing.T) {
	m, err := NewManager("test-secret", 1) // 1 秒 TTL
	require.NoError(t, err)

	token, err := m.Sign(1, "bob")
	require.NoError(t, err)

	// 等 token 过期
	time.Sleep(1200 * time.Millisecond)

	_, err = m.Parse(token)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidToken)
}

func TestManager_Parse_Garbage_ReturnsError(t *testing.T) {
	m, _ := NewManager("test-secret", 3600)
	_, err := m.Parse("not-a-jwt")
	require.Error(t, err)
}

func TestManager_Parse_WrongSecret_ReturnsError(t *testing.T) {
	m1, _ := NewManager("secret-1", 3600)
	m2, _ := NewManager("secret-2", 3600)

	token, _ := m1.Sign(7, "carol")

	_, err := m2.Parse(token)
	require.Error(t, err, "错误密钥应解析失败")
}

func TestManager_EmptySecret_ReturnsError(t *testing.T) {
	_, err := NewManager("", 3600)
	require.Error(t, err)
}

func TestManager_TTL_Default(t *testing.T) {
	m, _ := NewManager("test-secret", 0) // TTL <= 0 → 默认 24h
	assert.Equal(t, 24*time.Hour, m.TTL())
}
