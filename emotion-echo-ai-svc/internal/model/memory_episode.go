// Package model 定义 ai-svc 拥有的领域实体。
//
// CL-08c（记忆 · 情节 + 图记忆检索）Phase 1：本文件新增两个实体 ——
//   - memory_episode：情节 = 对来源（message / note）的一次可重建存档
//   - memory_entity：实体注册表 = 图节点在 PG 的镜像（消解后的规范名）
//
// 与 CL-08a 的 memory_fact / memory_note / memory_extract_run（memory_fact.go）
// 合起来构成 ADR 决策 45 的「PG 为系统记录」：图丢了能从 PG 重放重建（决策 8）。
package model

import "time"

// 情节 / 实体的生命周期状态（与 i012 的 CHECK 约束一一对应）。
//
// 没有 superseded：情节是对"说过什么"的不可变存档；实体的"最新"语义
// 落在 summary 刷新上。生命周期只有 active / deleted（软删）。
const (
	// MemoryNodeStatusActive 当前有效
	MemoryNodeStatusActive = "active"
	// MemoryNodeStatusDeleted 用户删除（PG 软删；图侧物理删除归 Phase 5 / CL-08d）
	MemoryNodeStatusDeleted = "deleted"
)

// MemoryEpisode 一条情节（emotion_echo_ai.memory_episode）。
//
// 双时间轴：
//   - valid_at：事件发生时间（来源消息的 send_time / 自述的 created_at）；
//   - created_at：摄入时间（抽取 worker 写入的时刻）。
//
// content 是图丢失时重放重建的原料（ADR 决策 8：图必须可从 PG 重建）；
// (source_kind, source_id) 是幂等键 —— 一条来源至多一个情节。
type MemoryEpisode struct {
	ID             int64      `gorm:"column:id;primaryKey;autoIncrement"`
	UserID         int64      `gorm:"column:user_id;not null"`
	ConversationID int64      `gorm:"column:conversation_id;not null"`
	SourceKind     string     `gorm:"column:source_kind;not null"`
	SourceID       int64      `gorm:"column:source_id;not null"`
	Name           string     `gorm:"column:name;not null"`
	Content        string     `gorm:"column:content;not null"`
	ValidAt        time.Time  `gorm:"column:valid_at;not null"`
	Status         string     `gorm:"column:status;not null;default:active"`
	DeletedAt      *time.Time `gorm:"column:deleted_at"`
	CreatedAt      time.Time  `gorm:"column:created_at;autoCreateTime"`
}

// TableName 归属 ai-svc 的 schema（跨 schema 写入被迁移守卫禁止）。
func (MemoryEpisode) TableName() string { return "emotion_echo_ai.memory_episode" }

// MemoryEntity 一个实体（emotion_echo_ai.memory_entity）。
//
// (user_id, name) 唯一 = 实体消解的落库锚点（"我妈"/"母亲"的合并由 LLM 消解负责，
// PG 只保证消解结果不重复落行）。summary 随最新一次抽取刷新；
// 软删后再次抽取由仓储层复活该行（新证据应生效），不新建。
type MemoryEntity struct {
	ID        int64      `gorm:"column:id;primaryKey;autoIncrement"`
	UserID    int64      `gorm:"column:user_id;not null"`
	Name      string     `gorm:"column:name;not null"`
	Summary   string     `gorm:"column:summary;not null;default:''"`
	Status    string     `gorm:"column:status;not null;default:active"`
	DeletedAt *time.Time `gorm:"column:deleted_at"`
	CreatedAt time.Time  `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time  `gorm:"column:updated_at;autoUpdateTime"`
}

// TableName 归属 ai-svc 的 schema。
func (MemoryEntity) TableName() string { return "emotion_echo_ai.memory_entity" }
