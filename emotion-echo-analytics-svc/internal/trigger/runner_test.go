// Package trigger — runner_test.go
//
// Stage 30-A SQL 落地 Part 5 RED: MentalHealthRunner 单元测试。
// 用 InMemoryJobStore + stub assessmentGetter，不依赖真实 DB。
package trigger

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"emotion-echo-analytics-svc/internal/repository"
)

// stubAssessmentGetter 最小 fake：实现 GetLatestAssessment + Save（E2E-15）
type stubAssessmentGetter struct {
	assessment *repository.MentalAssessment
	err        error

	// Save 行为控制（E2E-15 阶段 1.2 测试用）
	saveCalls   int
	lastSaved   *repository.MentalAssessment
	saveErr     error
}

func (s *stubAssessmentGetter) GetLatestAssessment(_ context.Context, _ int64, _ repository.AssessmentType) (*repository.MentalAssessment, error) {
	return s.assessment, s.err
}

// Save 记录调用 + 返 saveErr
func (s *stubAssessmentGetter) Save(_ context.Context, a *repository.MentalAssessment) error {
	s.saveCalls++
	s.lastSaved = a
	return s.saveErr
}

// inMemoryJobStore 记录 job 状态机（测试替身）
type inMemoryJobStore struct {
	mu      sync.Mutex
	rows    map[string]jobRow
	lastErr error
}

type jobRow struct {
	status string
	result []byte
	errMsg string
}

func newInMemoryJobStore() *inMemoryJobStore {
	return &inMemoryJobStore{rows: map[string]jobRow{}}
}

func (s *inMemoryJobStore) InsertJob(_ context.Context, taskID string, _ int64, _ string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastErr != nil {
		return s.lastErr
	}
	s.rows[taskID] = jobRow{status: JobRunning}
	return nil
}

func (s *inMemoryJobStore) CompleteJob(_ context.Context, taskID string, result []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastErr != nil {
		return s.lastErr
	}
	row := s.rows[taskID]
	row.status = JobDone
	row.result = result
	s.rows[taskID] = row
	return nil
}

func (s *inMemoryJobStore) FailJob(_ context.Context, taskID, errMsg string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastErr != nil {
		return s.lastErr
	}
	row := s.rows[taskID]
	row.status = JobFailed
	row.errMsg = errMsg
	s.rows[taskID] = row
	return nil
}

func (s *inMemoryJobStore) get(taskID string) (jobRow, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rows[taskID]
	return r, ok
}

func newTestRunner(assessment *repository.MentalAssessment, repoErr error) (*MentalHealthRunner, *inMemoryJobStore) {
	store := newInMemoryJobStore()
	runner := NewMentalHealthRunner(&stubAssessmentGetter{assessment: assessment, err: repoErr}, store)
	return runner, store
}

func TestMentalHealthRunner_Run_HappyPath_JobDoneWithResult(t *testing.T) {
	runner, store := newTestRunner(&repository.MentalAssessment{
		UserID:       42,
		Type:         "daily",
		OverallScore: 55,
		RiskLevel:    "moderate",
	}, nil)

	err := runner.Run(context.Background(), Request{
		UserID:         42,
		AssessmentType: "daily",
		TraceID:        "task-1",
	})
	require.NoError(t, err)

	row, ok := store.get("task-1")
	require.True(t, ok, "job 应已创建")
	assert.Equal(t, JobDone, row.status)

	var got repository.MentalAssessment
	require.NoError(t, json.Unmarshal(row.result, &got))
	assert.Equal(t, int64(42), got.UserID)
	assert.Equal(t, float64(55), got.OverallScore)
	assert.Equal(t, "moderate", got.RiskLevel)
}

func TestMentalHealthRunner_Run_RepoError_JobFailed(t *testing.T) {
	runner, store := newTestRunner(nil, errors.New("assessment query failed"))

	err := runner.Run(context.Background(), Request{
		UserID:         42,
		AssessmentType: "weekly",
		TraceID:        "task-2",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "assessment query failed")

	row, ok := store.get("task-2")
	require.True(t, ok)
	assert.Equal(t, JobFailed, row.status)
	assert.Equal(t, "assessment query failed", row.errMsg)
}

func TestMentalHealthRunner_Run_NoData_JobDoneWithNullResult(t *testing.T) {
	// 用户无评估数据 → repo 返 (nil, nil)：job 仍 done，result 为 null
	runner, store := newTestRunner(nil, nil)

	err := runner.Run(context.Background(), Request{
		UserID:         42,
		AssessmentType: "daily",
		TraceID:        "task-3",
	})
	require.NoError(t, err)

	row, ok := store.get("task-3")
	require.True(t, ok)
	assert.Equal(t, JobDone, row.status)
	assert.JSONEq(t, "null", string(row.result), "无数据时 result 序列化为 null")
}

func TestMentalHealthRunner_Run_InsertJobError_NoAssessmentCall(t *testing.T) {
	store := newInMemoryJobStore()
	store.lastErr = errors.New("insert failed")
	runner := NewMentalHealthRunner(&stubAssessmentGetter{assessment: nil, err: nil}, store)

	err := runner.Run(context.Background(), Request{
		UserID:         42,
		AssessmentType: "daily",
		TraceID:        "task-4",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "insert job")
}

// === E2E-15 阶段 1.2 新增：runner.Run 末尾调 Save 写 mental_health_assessments（修 E2E-F-10） ===

// TestMentalHealthRunner_Run_HappyPath_WritesAssessment：
// 成功读到 assessment → runner.Run 应调 Save 一次（写 mental_health_assessments），job 仍 done。
func TestMentalHealthRunner_Run_HappyPath_WritesAssessment(t *testing.T) {
	stub := &stubAssessmentGetter{assessment: &repository.MentalAssessment{
		UserID:       42,
		Type:         "daily",
		WindowStart:  "2026-09-21",
		WindowEnd:    "2026-09-21",
		OverallScore: 55,
		RiskLevel:    "moderate",
	}}
	store := newInMemoryJobStore()
	runner := NewMentalHealthRunner(stub, store)

	err := runner.Run(context.Background(), Request{
		UserID:         42,
		AssessmentType: "daily",
		TraceID:        "task-write-1",
	})
	require.NoError(t, err)

	// Save 应被调 1 次
	assert.Equal(t, 1, stub.saveCalls, "runner.Run 应调 Save 一次写 mental_health_assessments（修 E2E-F-10）")

	// Save 入参应镜像 GetLatestAssessment 返回值
	require.NotNil(t, stub.lastSaved)
	assert.Equal(t, int64(42), stub.lastSaved.UserID)
	assert.Equal(t, "daily", stub.lastSaved.Type)
	assert.Equal(t, float64(55), stub.lastSaved.OverallScore)
	assert.Equal(t, "moderate", stub.lastSaved.RiskLevel)

	// job 仍 done（Save 失败才应 fail）
	row, ok := store.get("task-write-1")
	require.True(t, ok)
	assert.Equal(t, JobDone, row.status, "Save 成功时 job 应 done")
}

// TestMentalHealthRunner_Run_NoData_WritesPlaceholder：
// 无数据时 GetLatestAssessment 返 nil → runner.Run 应调 Save(placeholder) 一次
// （让 mental_health_assessments 表非空，dev 演示账号首次即可见）。
func TestMentalHealthRunner_Run_NoData_WritesPlaceholder(t *testing.T) {
	stub := &stubAssessmentGetter{assessment: nil} // 无数据
	store := newInMemoryJobStore()
	runner := NewMentalHealthRunner(stub, store)

	err := runner.Run(context.Background(), Request{
		UserID:         42,
		AssessmentType: "daily",
		TraceID:        "task-write-2",
	})
	require.NoError(t, err)

	// 即使无数据也写一条（让表非空）
	assert.Equal(t, 1, stub.saveCalls, "无数据时 runner.Run 仍应写 placeholder 一次")

	// placeholder 入参：UserID/Type 来自 Request；score 0；risk "low"
	require.NotNil(t, stub.lastSaved)
	assert.Equal(t, int64(42), stub.lastSaved.UserID)
	assert.Equal(t, "daily", stub.lastSaved.Type)
	assert.Equal(t, float64(0), stub.lastSaved.OverallScore, "无数据 placeholder score=0")
	assert.Equal(t, "low", stub.lastSaved.RiskLevel)

	// job 仍 done（placeholder 写入算成功）
	row, ok := store.get("task-write-2")
	require.True(t, ok)
	assert.Equal(t, JobDone, row.status)
}

// TestMentalHealthRunner_Run_SaveError_FailsJob：
// Save 报错 → runner.Run 应返回 error + job 应 fail（GetLatestAssessment 成功但 Save 失败不能静默）
func TestMentalHealthRunner_Run_SaveError_FailsJob(t *testing.T) {
	stub := &stubAssessmentGetter{
		assessment: &repository.MentalAssessment{
			UserID:       42,
			Type:         "daily",
			OverallScore: 55,
		},
		saveErr: errors.New("save failed"),
	}
	store := newInMemoryJobStore()
	runner := NewMentalHealthRunner(stub, store)

	err := runner.Run(context.Background(), Request{
		UserID:         42,
		AssessmentType: "daily",
		TraceID:        "task-write-3",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "save failed")

	// job 应 fail
	row, ok := store.get("task-write-3")
	require.True(t, ok)
	assert.Equal(t, JobFailed, row.status, "Save 失败时 job 应 fail")
	assert.Equal(t, "save failed", row.errMsg)
}
