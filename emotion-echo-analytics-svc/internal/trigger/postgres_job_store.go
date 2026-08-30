// Package trigger — postgres_job_store.go
//
// Stage 30-A SQL 落地 Part 5 GREEN: JobStore 的 Postgres 实现，
// 写 emotion_echo_analytics.assessment_jobs（analytics 自有表）。
package trigger

import (
	"context"

	"gorm.io/gorm"
)

// PostgresJobStore JobStore 的 Postgres 实现
type PostgresJobStore struct {
	db *gorm.DB
}

// NewPostgresJobStore 构造
func NewPostgresJobStore(db *gorm.DB) *PostgresJobStore {
	return &PostgresJobStore{db: db}
}

// InsertJob 创建 job（status=running）
func (s *PostgresJobStore) InsertJob(ctx context.Context, taskID string, userID int64, assessmentType string) error {
	return s.db.WithContext(ctx).Exec(
		`INSERT INTO emotion_echo_analytics.assessment_jobs
		 (task_id, user_id, assessment_type, status, created_at)
		 VALUES ($1, $2, $3, 'running', NOW())`,
		taskID, userID, assessmentType,
	).Error
}

// CompleteJob 标记 done + 写入 result JSON
func (s *PostgresJobStore) CompleteJob(ctx context.Context, taskID string, result []byte) error {
	return s.db.WithContext(ctx).Exec(
		`UPDATE emotion_echo_analytics.assessment_jobs
		 SET status = 'done', result = $2::jsonb, completed_at = NOW()
		 WHERE task_id = $1`,
		taskID, string(result),
	).Error
}

// FailJob 标记 failed + 写入错误信息
func (s *PostgresJobStore) FailJob(ctx context.Context, taskID, errMsg string) error {
	return s.db.WithContext(ctx).Exec(
		`UPDATE emotion_echo_analytics.assessment_jobs
		 SET status = 'failed', error = $2, completed_at = NOW()
		 WHERE task_id = $1`,
		taskID, errMsg,
	).Error
}
