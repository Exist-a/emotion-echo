package models

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// EmotionScoresMap 自定义Map类型，支持PostgreSQL jsonb类型扫描
type EmotionScoresMap map[string]float64

// Scan 实现Scanner接口，用于从数据库读取jsonb
func (e *EmotionScoresMap) Scan(value interface{}) error {
	if value == nil {
		*e = nil
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return errors.New("invalid type for EmotionScoresMap")
	}

	return json.Unmarshal(bytes, e)
}

// Value 实现 Valuer 接口，用于写入数据库
func (e EmotionScoresMap) Value() (driver.Value, error) {
	if e == nil {
		return nil, nil
	}
	return json.Marshal(e)
}

// EmotionAnalysis 情绪分析模型
type EmotionAnalysis struct {
	ID              int64           `gorm:"primaryKey" json:"-"`
	ConversationID string          `gorm:"index;size:32;not null" json:"-"`
	UserID          int64           `gorm:"index;not null" json:"-"`
	AnalyzedAt      time.Time       `json:"analyzedAt"`
	EmotionScores  EmotionScoresMap `gorm:"type:jsonb" json:"emotionScores"`
	DominantEmotion string          `gorm:"size:20" json:"dominantEmotion"`
	Summary        string          `gorm:"type:text" json:"summary"`
	CreatedAt       time.Time       `json:"-"`
}

// TableName 表名
func (EmotionAnalysis) TableName() string {
	return "emotion_analyses"
}