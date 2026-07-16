package nanoid

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
)

const (
	alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	size     = 8
)

// Generate 生成短 ID
func Generate() string {
	bytes := make([]byte, size)
	rand.Read(bytes)
	
	// 使用 base64 URL 编码并替换特殊字符
	id := base64.RawURLEncoding.EncodeToString(bytes)
	if len(id) > size {
		id = id[:size]
	}
	
	// 替换非字母数字字符
	result := make([]byte, size)
	for i := 0; i < size; i++ {
		if i < len(id) {
			c := id[i]
			if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') {
				result[i] = c
			} else {
				// 使用字母表替换
				result[i] = alphabet[c%62]
			}
		} else {
			result[i] = alphabet[0]
		}
	}
	
	return strings.ToLower(string(result))
}

// GenerateWithPrefix 带前缀的 ID
func GenerateWithPrefix(prefix string) string {
	return prefix + "_" + Generate()
}
