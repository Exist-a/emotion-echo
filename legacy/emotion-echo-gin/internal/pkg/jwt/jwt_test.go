package jwt

import (
	"testing"
	"time"
)

func TestJWT(t *testing.T) {
	j := New("test-secret-key-32-chars-long!!", 15*time.Minute)
	userID := int64(12345)

	// 生成 Token
	token, err := j.GenerateAccessToken(userID)
	if err != nil {
		t.Fatalf("GenerateAccessToken failed: %v", err)
	}
	if token == "" {
		t.Fatal("Token should not be empty")
	}

	// 解析 Token
	claims, err := j.ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken failed: %v", err)
	}
	if claims.UserID != userID {
		t.Errorf("UserID mismatch: got %d, want %d", claims.UserID, userID)
	}
	if claims.JTI == "" {
		t.Error("JTI should not be empty")
	}

	// 验证过期 Token
	expiredJWT := New("test-secret-key-32-chars-long!!", -1*time.Minute)
	expiredToken, _ := expiredJWT.GenerateAccessToken(userID)
	_, err = j.ParseToken(expiredToken)
	if err == nil {
		t.Error("Expired token should fail")
	}
	if !IsTokenExpired(err) {
		t.Error("Should detect token expired")
	}

	// 验证错误签名
	wrongJWT := New("wrong-secret-key-32-chars-long!", 15*time.Minute)
	_, err = wrongJWT.ParseToken(token)
	if err == nil {
		t.Error("Wrong secret should fail")
	}
}
