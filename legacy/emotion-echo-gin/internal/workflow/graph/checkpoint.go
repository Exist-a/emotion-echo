package graph

import (
	"context"
	"time"
)

// Checkpointer 检查点持久化接口
// 用于支持断点续跑和审计回溯
type Checkpointer interface {
	// Save 保存状态快照
	Save(ctx context.Context, runID string, step int, state State) error
	// LoadLatest 加载最新状态
	LoadLatest(ctx context.Context, runID string) (int, State, error)
	// GetHistory 获取执行历史（用于审计回溯）
	GetHistory(ctx context.Context, runID string) ([]CheckpointRecord, error)
}

// CheckpointRecord 检查点记录
type CheckpointRecord struct {
	RunID     string    `json:"run_id"`
	Step      int       `json:"step"`
	NodeID    string    `json:"node_id,omitempty"`
	StateJSON []byte    `json:"state_json"`
	Timestamp time.Time `json:"timestamp"`
}

// NoOpCheckpointer 空实现（用于测试或关闭检查点时）
type NoOpCheckpointer struct{}

func (n *NoOpCheckpointer) Save(ctx context.Context, runID string, step int, state State) error {
	return nil
}

func (n *NoOpCheckpointer) LoadLatest(ctx context.Context, runID string) (int, State, error) {
	return 0, nil, nil
}

func (n *NoOpCheckpointer) GetHistory(ctx context.Context, runID string) ([]CheckpointRecord, error) {
	return nil, nil
}
