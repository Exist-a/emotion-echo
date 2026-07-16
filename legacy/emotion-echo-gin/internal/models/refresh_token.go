package models

import (
	"time"
)

// RefreshToken 刷新令牌模型
type RefreshToken struct {
	ID         int64     `gorm:"primaryKey" json:"-"`
	UserID     int64     `gorm:"index;not null" json:"-"`
	TokenHash  string    `gorm:"uniqueIndex;size:64;not null" json:"-"`
	RememberMe bool      `gorm:"not null;default:false" json:"-"`
	ExpiresAt  time.Time `json:"-"`
	CreatedAt  time.Time `json:"-"`
}

// TableName 表名
func (RefreshToken) TableName() string {
	return "refresh_tokens"
}
