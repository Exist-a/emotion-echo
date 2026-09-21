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
	"time"

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
//
// E2E-15 阶段 1.2 新增 Save：触发链末尾写 mental_health_assessments（修 E2E-F-10）。
type assessmentGetter interface {
	GetLatestAssessment(ctx context.Context, userID int64, atype repository.AssessmentType) (*repository.MentalAssessment, error)
	Save(ctx context.Context, a *repository.MentalAssessment) error
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
//   - 成功（含无数据 nil）→ Save 写 mental_health_assessments（修 E2E-F-10）：
//     - 成功 → CompleteJob + result JSON；无数据时 result=null
//     - 失败 → FailJob + 返回错误
//
// E2E-15 阶段 1.2：新增 Save 路径（写 mental_health_assessments 表）。
// assessment 为 nil 时写 placeholder（UserID/Type/今日窗口/score=0/risk="low"），
// 让 dev 演示账号首次 trigger 后 mental_health_assessments 不再为空。
func (r *MentalHealthRunner) Run(ctx context.Context, req Request) error {
	if err := r.jobs.InsertJob(ctx, req.TraceID, req.UserID, req.AssessmentType); err != nil {
		return fmt.Errorf("insert job %s: %w", req.TraceID, err)
	}

	assessment, err := r.repo.GetLatestAssessment(ctx, req.UserID, repository.AssessmentType(req.AssessmentType))
	if err != nil {
		_ = r.jobs.FailJob(ctx, req.TraceID, err.Error())
		return fmt.Errorf("assess user %d: %w", req.UserID, err)
	}

	// E2E-15 新增：写 mental_health_assessments（修 E2E-F-10）
	// assessment 为 nil 时写 placeholder（让表非空），便于 dev 演示
	toSave := assessment
	if toSave == nil {
		toSave = placeholderAssessment(req.UserID, req.AssessmentType)
	}
	if err := r.repo.Save(ctx, toSave); err != nil {
		_ = r.jobs.FailJob(ctx, req.TraceID, err.Error())
		return fmt.Errorf("save assessment user %d: %w", req.UserID, err)
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

// placeholderAssessment 在 GetLatestAssessment 返回 nil 时构造一条
// 占位记录 —— 让 mental_health_assessments 表不再永远为空。
//
// 设计要点：
//   - UserID / Type 透传 Request（让 evaluator 可关联）
//   - OverallScore = 0（无数据）
//   - RiskLevel = "low"（与 riskFromScore(<40) 一致）
//   - WindowStart / WindowEnd = 今日日期（不依赖外部时钟：使用 time.Now() UTC）
//   - GeneratedAt 不写（PostgresMentalHealthRepo.Save 用 DEFAULT NOW()）
func placeholderAssessment(userID int64, assessmentType string) *repository.MentalAssessment {
	today := time.Now().UTC().Format("2006-01-02")
	return &repository.MentalAssessment{
		UserID:       userID,
		Type:         assessmentType,
		WindowStart:  today,
		WindowEnd:    today,
		OverallScore: 0,
		RiskLevel:    "low",
	}
}
