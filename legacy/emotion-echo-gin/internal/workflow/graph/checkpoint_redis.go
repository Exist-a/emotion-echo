package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisCheckpointer Redis 检查点实现
type RedisCheckpointer struct {
	client *redis.Client
	prefix string // key 前缀，默认 "checkpoint"
}

// NewRedisCheckpointer 创建 Redis 检查点
func NewRedisCheckpointer(client *redis.Client, prefix string) *RedisCheckpointer {
	if prefix == "" {
		prefix = "checkpoint"
	}
	return &RedisCheckpointer{
		client: client,
		prefix: prefix,
	}
}

// Save 保存状态快照
// Key: checkpoint:{run_id}:latest
// Key: checkpoint:{run_id}:history（List）
func (r *RedisCheckpointer) Save(ctx context.Context, runID string, step int, state State) error {
	timestamp := time.Now()

	// 序列化状态
	stateJSON, err := state.MarshalJSON()
	if err != nil {
		return fmt.Errorf("marshal state failed: %w", err)
	}

	record := CheckpointRecord{
		RunID:     runID,
		Step:      step,
		StateJSON: stateJSON,
		Timestamp: timestamp,
	}

	recordJSON, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("marshal record failed: %w", err)
	}

	pipe := r.client.Pipeline()

	// 保存最新状态（Hash）
	latestKey := fmt.Sprintf("%s:%s:latest", r.prefix, runID)
	pipe.HSet(ctx, latestKey, map[string]interface{}{
		"step":       step,
		"state":      string(stateJSON),
		"timestamp":  timestamp.Format(time.RFC3339),
		"record":     string(recordJSON),
	})
	// 设置过期时间（7天）
	pipe.Expire(ctx, latestKey, 7*24*time.Hour)

	// 追加到历史记录（List）
	historyKey := fmt.Sprintf("%s:%s:history", r.prefix, runID)
	pipe.RPush(ctx, historyKey, string(recordJSON))
	// 限制历史记录长度（保留最近 100 条）
	pipe.LTrim(ctx, historyKey, -100, -1)
	pipe.Expire(ctx, historyKey, 7*24*time.Hour)

	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("redis pipeline failed: %w", err)
	}

	return nil
}

// LoadLatest 加载最新状态
func (r *RedisCheckpointer) LoadLatest(ctx context.Context, runID string) (int, State, error) {
	latestKey := fmt.Sprintf("%s:%s:latest", r.prefix, runID)

	result, err := r.client.HGetAll(ctx, latestKey).Result()
	if err != nil {
		return 0, nil, fmt.Errorf("redis get failed: %w", err)
	}
	if len(result) == 0 {
		return 0, nil, nil // 无检查点
	}

	step := 0
	if s, ok := result["step"]; ok {
		fmt.Sscanf(s, "%d", &step)
	}

	stateJSON := result["state"]
	state, err := UnmarshalMemoryState([]byte(stateJSON))
	if err != nil {
		return 0, nil, fmt.Errorf("unmarshal state failed: %w", err)
	}

	return step, state, nil
}

// GetHistory 获取执行历史
func (r *RedisCheckpointer) GetHistory(ctx context.Context, runID string) ([]CheckpointRecord, error) {
	historyKey := fmt.Sprintf("%s:%s:history", r.prefix, runID)

	recordsJSON, err := r.client.LRange(ctx, historyKey, 0, -1).Result()
	if err != nil {
		return nil, fmt.Errorf("redis lrange failed: %w", err)
	}

	records := make([]CheckpointRecord, 0, len(recordsJSON))
	for _, recordJSON := range recordsJSON {
		var record CheckpointRecord
		if err := json.Unmarshal([]byte(recordJSON), &record); err != nil {
			continue // 跳过损坏的记录
		}
		records = append(records, record)
	}

	return records, nil
}

// DeleteRun 删除指定运行 ID 的检查点
func (r *RedisCheckpointer) DeleteRun(ctx context.Context, runID string) error {
	latestKey := fmt.Sprintf("%s:%s:latest", r.prefix, runID)
	historyKey := fmt.Sprintf("%s:%s:history", r.prefix, runID)
	return r.client.Del(ctx, latestKey, historyKey).Err()
}
