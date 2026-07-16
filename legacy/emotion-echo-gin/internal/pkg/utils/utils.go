package utils

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/google/uuid"
)

// GenerateRandomToken 生成随机 Token（32字节，64字符十六进制）
func GenerateRandomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// 如果加密随机数生成失败，使用 UUID 作为回退
		return uuid.New().String()
	}
	return hex.EncodeToString(b)
}

// SHA256Hash 计算 SHA256 哈希
func SHA256Hash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

// GenerateUUID 生成 UUID
func GenerateUUID() string {
	return uuid.New().String()
}

// GenerateRandomState 生成随机 state（16字符十六进制）
func GenerateRandomState() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// 回退方案
		n, _ := rand.Int(rand.Reader, big.NewInt(1<<32))
		return fmt.Sprintf("%08x", n.Int64())
	}
	return hex.EncodeToString(b)
}
