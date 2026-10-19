// Package model 定义 ai-svc 拥有的领域实体。
//
// CL-08a（记忆 · 用户事实切片）：本文件新增三个实体 ——
//   - memory_fact：跨会话记忆的**系统记录**（ADR 决策 45「PG 为系统记录、图为可重建索引」）
//   - memory_note：「我的记忆」页的自述原文（独立于 conversations/messages）
//   - memory_extract_run：抽取审计 + 幂等登记（每个来源只抽一次）
package model

import "time"

// 事实来源类型。
const (
	// MemorySourceKindMessage 正常会话里的 user 消息
	MemorySourceKindMessage = "message"
	// MemorySourceKindNote 「我的记忆」页的自述条
	MemorySourceKindNote = "note"
)

// 事实生命周期状态（与 i011 的 CHECK 约束一一对应）。
const (
	// MemoryFactStatusActive 当前有效（valid_to IS NULL）
	MemoryFactStatusActive = "active"
	// MemoryFactStatusSuperseded 已被更新值取代（双时间轴关区间，不删除）
	MemoryFactStatusSuperseded = "superseded"
	// MemoryFactStatusDeleted 用户删除（PG 软删；图侧物理删除归 CL-08c）
	MemoryFactStatusDeleted = "deleted"
)

// 抽取运行状态（与 i011 的 CHECK 约束一一对应）。
const (
	MemoryExtractStatusPending = "pending"
	MemoryExtractStatusDone    = "done"
	MemoryExtractStatusFailed  = "failed"
)

// MemoryFact 一条用户事实（emotion_echo_ai.memory_fact）。
//
// 双时间轴（T-3 / T-15）：
//   - valid_from：事实开始生效的时间（LLM 给出则用，否则回落来源时间）
//   - valid_to：NULL = 仍然有效；被新值取代时置为新值的 valid_from
//     —— **旧行不删除**，历史仍可查（"能说出北京是前值"）。
//
// 来源：source_kind + source_id（message / note）—— 与抽取审计表的幂等键一致。
type MemoryFact struct {
	ID             int64      `gorm:"column:id;primaryKey;autoIncrement"`
	UserID         int64      `gorm:"column:user_id;not null"`
	ConversationID int64      `gorm:"column:conversation_id;not null"`
	SourceKind     string     `gorm:"column:source_kind;not null"`
	SourceID       int64      `gorm:"column:source_id;not null"`
	Subject        string     `gorm:"column:subject;not null"`
	Predicate      string     `gorm:"column:predicate;not null"`
	Object         string     `gorm:"column:object;not null"`
	FactText       string     `gorm:"column:fact_text;not null"`
	Confidence     *float64   `gorm:"column:confidence"`
	ValidFrom      time.Time  `gorm:"column:valid_from;not null"`
	ValidTo        *time.Time `gorm:"column:valid_to"`
	Status         string     `gorm:"column:status;not null;default:active"`
	DeletedAt      *time.Time `gorm:"column:deleted_at"`
	CreatedAt      time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 归属 ai-svc 的 schema（跨 schema 写入被迁移守卫禁止）。
func (MemoryFact) TableName() string { return "emotion_echo_ai.memory_fact" }

// MemoryNote 「我的记忆」页的一条自述（emotion_echo_ai.memory_note）。
//
// 独立于 conversations/messages（用户裁定 D6）⇒ 不触发 AI 回复、情绪分析、数字人。
type MemoryNote struct {
	ID            int64      `gorm:"column:id;primaryKey;autoIncrement"`
	UserID        int64      `gorm:"column:user_id;not null"`
	Content       string     `gorm:"column:content;not null"`
	ExtractStatus string     `gorm:"column:extract_status;not null;default:pending"`
	CreatedAt     time.Time  `gorm:"column:created_at;autoCreateTime"`
	DeletedAt     *time.Time `gorm:"column:deleted_at"`
}

// TableName 归属 ai-svc 的 schema。
func (MemoryNote) TableName() string { return "emotion_echo_ai.memory_note" }

// MemoryExtractRun 一个来源的抽取运行记录（emotion_echo_ai.memory_extract_run）。
//
// (source_kind, source_id) 复合主键 = 幂等键：at-least-once 投递下同一来源只抽一次 LLM。
type MemoryExtractRun struct {
	SourceKind     string    `gorm:"column:source_kind;primaryKey"`
	SourceID       int64     `gorm:"column:source_id;primaryKey;autoIncrement:false"`
	UserID         int64     `gorm:"column:user_id;not null"`
	ConversationID int64     `gorm:"column:conversation_id;not null"`
	Status         string    `gorm:"column:status;not null;default:pending"`
	FactsCount     int       `gorm:"column:facts_count;not null;default:0"`
	Error          string    `gorm:"column:error;not null;default:''"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 归属 ai-svc 的 schema。
func (MemoryExtractRun) TableName() string { return "emotion_echo_ai.memory_extract_run" }
