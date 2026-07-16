package service

import (
	"emotion-echo-gin/internal/pkg/password"
)

// HashPassword 对密码进行哈希
func HashPassword(pwd string) (string, error) {
	return password.Hash(pwd)
}

// VerifyPassword 验证密码
func VerifyPassword(pwd, hash string) bool {
	return password.Verify(pwd, hash)
}
