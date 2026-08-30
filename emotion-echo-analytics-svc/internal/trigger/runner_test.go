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

// stubAssessmentGetter 最小 fake：只实现 GetLatestAssessment
type stubAssessmentGetter struct {
	assessment *repository.MentalAssessment
	err        error
}

func (s *stubAssessmentGetter) GetLatestAssessment(_ context.Context, _ int64, _ repository.AssessmentType) (*repository.MentalAssessment, error) {
	return s.assessment, s.err
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
