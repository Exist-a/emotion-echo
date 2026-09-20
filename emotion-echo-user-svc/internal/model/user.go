// Package model 定义 user-svc 拥有的领域实体。
//
// 注意：这些 struct 同时对应数据库表（GORM），
// 与 emotion_echo_user schema 中的表一一对应。
package model

import (
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// JSONMap 是 map[string]any 的 GORM JSONB 别名。
// Scan 从数据库读 JSONB → map；Value 写回 JSONB。
// nil 表示 NULL（未设置），空 map {} 表示用户主动清空。
type JSONMap map[string]any

func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		bytes = []byte(value.(string))
	}
	m := make(map[string]any)
	if err := json.Unmarshal(bytes, &m); err != nil {
		return err
	}
	*j = m
	return nil
}

func (j JSONMap) Value() (interface{}, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// User 与 emotion_echo_user.users 表对应。
// 表是 user-svc 的"唯一拥有者"，其他 svc 不得直接读写。
// E2E-06: DeletedAt 改用 gorm.DeletedAt，GORM 自动给查询加 WHERE deleted_at IS NULL
type User struct {
	ID           int64          `gorm:"column:id;primaryKey;autoIncrement"`
	Username     string         `gorm:"column:username;size:64;uniqueIndex"`
	PasswordHash *string        `gorm:"column:password_hash;size:255"`
	Nickname     *string        `gorm:"column:nickname;size:64"`
	AvatarURL    *string        `gorm:"column:avatar_url"`
	Gender       int16          `gorm:"column:gender;default:0"`
	Birthday     *time.Time     `gorm:"column:birthday"`
	Config       JSONMap        `gorm:"column:config;type:jsonb"`
	CreatedAt    time.Time      `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt    time.Time      `gorm:"column:updated_at;autoUpdateTime"`
	DeletedAt    gorm.DeletedAt `gorm:"column:deleted_at;index"`
}

// TableName 显式指定 schema + 表名
func (User) TableName() string { return "emotion_echo_user.users" }