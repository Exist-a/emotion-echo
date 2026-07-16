package models

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// User 用户模型
type User struct {
	ID            int64          `gorm:"primaryKey" json:"id,string"`
	Username      string         `gorm:"uniqueIndex;size:64;not null" json:"username"`
	PasswordHash  string         `gorm:"size:255;not null" json:"-"`
	Nickname      string         `gorm:"size:64;default:'用户'" json:"nickname"`
	Avatar        string         `gorm:"size:500;default:'/imgs/default-avatar.webp'" json:"avatar"`
	Age           *int           `gorm:"check:age >= 0 AND age <= 150" json:"age,omitempty"`
	WechatOpenID  string         `gorm:"index;size:64" json:"-"`
	WechatUnionID string         `gorm:"size:64" json:"-"`
	Config        UserConfig     `gorm:"type:jsonb;default:'{}'" json:"config"`
	CreatedAt     time.Time      `json:"createdAt"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// TableName 表名
func (User) TableName() string {
	return "users"
}

// UserConfig 用户配置
type UserConfig struct {
	FontSize string `json:"fontSize,omitempty"`
	Theme    string `json:"theme,omitempty"`
}

// Value 实现 driver.Valuer 接口
func (c UserConfig) Value() (driver.Value, error) {
	return json.Marshal(c)
}

// Scan 实现 sql.Scanner 接口
func (c *UserConfig) Scan(value interface{}) error {
	if value == nil {
		*c = UserConfig{}
		return nil
	}
	
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, c)
	case string:
		return json.Unmarshal([]byte(v), c)
	default:
		return json.Unmarshal([]byte(v.(string)), c)
	}
}
