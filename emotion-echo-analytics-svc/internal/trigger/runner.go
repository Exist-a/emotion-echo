// Package trigger — runner.go
//
// Stage 30-A SQL 落地 Part 5 GREEN: MentalHealthRunner 执行异步
// mental-health 评估任务（TriggerQueue worker 的真实 worker body）。
//
// 生命周期（analytics 自有表 assessment_jobs，不违反跨 schema 只读边界）：
//   InsertJob(running) → GetLatestAssessment（只读跨 schema 查询）
//   → CompleteJob(done, result JSONB) | FailJob(failed, error)
package trigger

import (
	"context"
	"encoding/json"
	"fmt"

	"emotion-echo-analytics-svc/internal/repository"
)

// Job 状态机取值（assessment_jobs.status）
const (
	JobRunning = "running"
	JobDone    = "done"
	JobFailed  = "failed"
)

// JobStore 持久化 trigger job 状态（接口 — 依赖注入便于测试）
type JobStore interface {
	// InsertJob 创建 job（status=running）
	InsertJob(ctx context.Context, taskID string, userID int64, assessmentType string) error
	// CompleteJob 标记 done + 写入 result JSON
	CompleteJob(ctx context.Context, taskID string, result []byte) error
	// FailJob 标记 failed + 写入错误信息
	FailJob(ctx context.Context, taskID, errMsg string) error
}

// assessmentGetter MentalHealthRunner 对评估仓库的最小依赖
//
// repository.MentalHealthRepo 满足此接口（编译期由 NewMentalHealthRunner 保证）。
type assessmentGetter interface {
	GetLatestAssessment(ctx context.Context, userID int64, atype repository.AssessmentType) (*repository.MentalAssessment, error)
}

// MentalHealthRunner 异步评估任务执行器
type MentalHealthRunner struct {
	repo assessmentGetter
	jobs JobStore
}

// NewMentalHealthRunner 构造
//
// repo 接收 assessmentGetter（repository.MentalHealthRepo 满足之），
// 测试可用只实现 GetLatestAssessment 的 stub。
func NewMentalHealthRunner(repo assessmentGetter, jobs JobStore) *MentalHealthRunner {
	return &MentalHealthRunner{repo: repo, jobs: jobs}
}

// Run 执行一个评估任务（TriggerQueue worker 调用）。
//
//   - InsertJob 失败 → 直接返回（不评估）
//   - GetLatestAssessment 失败 → FailJob + 返回错误
//   - 成功（含无数据 nil）→ CompleteJob + result JSON；无数据时 result=null
func (r *MentalHealthRunner) Run(ctx context.Context, req Request) error {
	if err := r.jobs.InsertJob(ctx, req.TraceID, req.UserID, req.AssessmentType); err != nil {
		return fmt.Errorf("insert job %s: %w", req.TraceID, err)
	}

	assessment, err := r.repo.GetLatestAssessment(ctx, req.UserID, repository.AssessmentType(req.AssessmentType))
	if err != nil {
		_ = r.jobs.FailJob(ctx, req.TraceID, err.Error())
		return fmt.Errorf("assess user %d: %w", req.UserID, err)
	}

	result, err := json.Marshal(assessment) // 无数据 assessment=nil → "null"
	if err != nil {
		_ = r.jobs.FailJob(ctx, req.TraceID, err.Error())
		return fmt.Errorf("marshal assessment: %w", err)
	}
	if err := r.jobs.CompleteJob(ctx, req.TraceID, result); err != nil {
		return fmt.Errorf("complete job %s: %w", req.TraceID, err)
	}
	return nil
}
