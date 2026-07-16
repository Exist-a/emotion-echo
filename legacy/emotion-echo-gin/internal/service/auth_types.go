package service

import (
	"crypto/sha256"
	"encoding/hex"
)

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username         string `json:"username" binding:"required"`
	Password         string `json:"password" binding:"required,min=6"`
	VerificationCode string `json:"verificationCode" binding:"required,len=6"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username   string `json:"username" binding:"required"`
	Password   string `json:"password" binding:"required"`
	RememberMe bool   `json:"rememberMe"`
}

// AuthResponse 认证响应
type AuthResponse struct {
	AccessToken string `json:"accessToken"`
	ExpiresIn   int    `json:"expiresIn"`
	RememberMe  bool   `json:"rememberMe"`
	UserID     int64  `json:"userId,omitempty"`
}

// ResetPasswordRequest 重置密码请求
type ResetPasswordRequest struct {
	Username         string `json:"username" binding:"required"`
	VerificationCode string `json:"verificationCode" binding:"required,len=6"`
	NewPassword      string `json:"newPassword" binding:"required,min=6"`
}

// RefreshTokenRequest 刷新Token请求
type RefreshTokenRequest struct {
	RefreshToken string `json:"refreshToken" binding:"required"`
}

// sha256Hash 计算SHA256哈希
func sha256Hash(input string) string {
	hash := sha256.Sum256([]byte(input))
	return hex.EncodeToString(hash[:])
}
