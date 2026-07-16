// Package model 定义 user-svc 拥有的领域实体。
//
// 注意：这些 struct 同时对应数据库表（GORM），
// 与 emotion_echo_user schema 中的表一一对应。
package model

import "time"

// User 与 emotion_echo_user.users 表对应。
// 表是 user-svc 的"唯一拥有者"，其他 svc 不得直接读写。
type User struct {
	ID           int64      `gorm:"column:id;primaryKey;autoIncrement"`
	Username     string     `gorm:"column:username;size:64;uniqueIndex"`
	Phone        *string    `gorm:"column:phone;size:20;uniqueIndex"`
	Email        *string    `gorm:"column:email;size:128;uniqueIndex"`
	PasswordHash *string    `gorm:"column:password_hash;size:255"`
	Nickname     *string    `gorm:"column:nickname;size:64"`
	AvatarURL    *string    `gorm:"column:avatar_url"`
	Gender       int16      `gorm:"column:gender;default:0"`
	Birthday     *time.Time `gorm:"column:birthday"`
	Status       int16      `gorm:"column:status;default:1"`
	CreatedAt    time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time  `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt    *time.Time `gorm:"column:deleted_at;index"`
}

// TableName 显式指定 schema + 表名
func (User) TableName() string { return "emotion_echo_user.users" }