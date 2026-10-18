// Package model 定义 ai-svc 拥有的领域实体。
//
// CL-08a（记忆 · 用户事实切片）：本文件新增两个实体 ——
//   - memory_fact：跨会话记忆的**系统记录**（ADR 决策 45「PG 为系统记录、图为可重建索引」）
//   - memory_extract_run：抽取审计 + 幂等登记（同一 message_id 只抽一次）
package model

import "time"

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
//   - valid_from：事实开始生效的时间（LLM 给出则用，否则回落消息时间）
//   - valid_to：NULL = 仍然有效；被新值取代时置为新值的 valid_from
//     —— **旧行不删除**，历史仍可查（"能说出北京是前值"）。
//
// 软删除（用户裁定 D3）：status='deleted' + deleted_at 置位；
// 图侧物理删除归 CL-08c（本阶段无图）。
type MemoryFact struct {
	ID              int64      `gorm:"column:id;primaryKey;autoIncrement"`
	UserID          int64      `gorm:"column:user_id;not null"`
	ConversationID  int64      `gorm:"column:conversation_id;not null"`
	SourceMessageID int64      `gorm:"column:source_message_id;not null"`
	Subject         string     `gorm:"column:subject;not null"`
	Predicate       string     `gorm:"column:predicate;not null"`
	Object          string     `gorm:"column:object;not null"`
	FactText        string     `gorm:"column:fact_text;not null"`
	Confidence      *float64   `gorm:"column:confidence"`
	ValidFrom       time.Time  `gorm:"column:valid_from;not null"`
	ValidTo         *time.Time `gorm:"column:valid_to"`
	Status          string     `gorm:"column:status;not null;default:active"`
	DeletedAt       *time.Time `gorm:"column:deleted_at"`
	CreatedAt       time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 归属 ai-svc 的 schema（跨 schema 写入被迁移守卫禁止）。
func (MemoryFact) TableName() string { return "emotion_echo_ai.memory_fact" }

// MemoryExtractRun 一条消息的抽取运行记录（emotion_echo_ai.memory_extract_run）。
//
// message_id 作主键 = 幂等键：at-least-once 投递下同一条消息只抽一次 LLM。
type MemoryExtractRun struct {
	MessageID      int64     `gorm:"column:message_id;primaryKey;autoIncrement:false"`
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
