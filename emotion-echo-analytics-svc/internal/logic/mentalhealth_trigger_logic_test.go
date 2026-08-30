// Package logic — mentalhealth_trigger_logic_test.go
//
// Sibling test for mentalhealth_trigger_logic.go (per AGENTS.md §1.1).
//
// Stage 30-A Round 3 part 2 RED: cover POST /api/v1/mental-health/trigger.
//
// POST 是 async（per stage-30-A §三.2）：logic 校验 → 提交到
// TriggerQueue → 立即返回 task_id + "accepted" status。
//
// Coverage matrix:
//
//   - HappyPath_ReturnsAcceptedAndTaskID
//   - TaskIDLooksLikeUUID
//   - InvalidUserID
//   - InvalidAssessmentType
//   - QueueFull_Backpressure
//   - QueueClosed
//   - NilQueue_InternalError
//
// 注意：本测试 RED 阶段只引用 TriggerQueue 的 *类型*（编译即
// 失败在 NewMentalHealthTriggerLogic 未定义）。Round 3 GREEN 提交
// 会把 trigger package + ServiceContext 扩展一起带上。
package logic

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"emotion-echo-analytics-svc/internal/config"
	"emotion-echo-analytics-svc/internal/svc"
	"emotion-echo-analytics-svc/internal/trigger"
	"emotion-echo-analytics-svc/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMentalHealthTriggerLogic_HappyPath_ReturnsAccepted(t *testing.T) {
	t.Parallel()
	queue := trigger.NewTriggerQueue(context.Background(), 1, 4, func(_ context.Context, _ trigger.Request) {})
	defer queue.Close(context.Background())

	// 手工构造 ServiceContext（避免新增构造函数 — Round 3 GREEN 才会加）
	ctx := context.Background()
	l := NewMentalHealthTriggerLogic(ctx, &svc.ServiceContext{
		Config:       config.Config{},
		TriggerQueue: queue,
	})

	resp, err := l.TriggerAssessment(&types.TriggerMentalHealthReq{
		UserID: 42, AssessmentType: "daily",
	})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "accepted", resp.Status)
	assert.NotEmpty(t, resp.TaskID, "task id 必须生成（用于后续追踪）")
}

func TestMentalHealthTriggerLogic_TaskIDLooksLikeUUID(t *testing.T) {
	t.Parallel()
	queue := trigger.NewTriggerQueue(context.Background(), 1, 4, func(_ context.Context, _ trigger.Request) {})
	defer queue.Close(context.Background())

	l := NewMentalHealthTriggerLogic(context.Background(), &svc.ServiceContext{
		Config: config.Config{}, TriggerQueue: queue,
	})
	resp, err := l.TriggerAssessment(&types.TriggerMentalHealthReq{
		UserID: 1, AssessmentType: "weekly",
	})
	require.NoError(t, err)
	assert.Len(t, resp.TaskID, 36)
	assert.Equal(t, 4, strings.Count(resp.TaskID, "-"))
}

func TestMentalHealthTriggerLogic_InvalidUserID(t *testing.T) {
	t.Parallel()
	queue := trigger.NewTriggerQueue(context.Background(), 1, 4, func(_ context.Context, _ trigger.Request) {})
	defer queue.Close(context.Background())

	l := NewMentalHealthTriggerLogic(context.Background(), &svc.ServiceContext{
		Config: config.Config{}, TriggerQueue: queue,
	})
	_, err := l.TriggerAssessment(&types.TriggerMentalHealthReq{UserID: 0, AssessmentType: "daily"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "user")
}

func TestMentalHealthTriggerLogic_InvalidAssessmentType(t *testing.T) {
	t.Parallel()
	queue := trigger.NewTriggerQueue(context.Background(), 1, 4, func(_ context.Context, _ trigger.Request) {})
	defer queue.Close(context.Background())

	l := NewMentalHealthTriggerLogic(context.Background(), &svc.ServiceContext{
		Config: config.Config{}, TriggerQueue: queue,
	})
	_, err := l.TriggerAssessment(&types.TriggerMentalHealthReq{UserID: 1, AssessmentType: "bogus"})
	require.Error(t, err)
}

func TestMentalHealthTriggerLogic_QueueFull_Backpressure(t *testing.T) {
	t.Parallel()
	block := make(chan struct{})
	queue := trigger.NewTriggerQueue(context.Background(), 1, 1, func(_ context.Context, _ trigger.Request) {
		<-block
	})
	defer func() {
		close(block)
		queue.Close(context.Background())
	}()

	l := NewMentalHealthTriggerLogic(context.Background(), &svc.ServiceContext{
		Config: config.Config{}, TriggerQueue: queue,
	})

	_, err := l.TriggerAssessment(&types.TriggerMentalHealthReq{UserID: 1, AssessmentType: "daily"})
	require.NoError(t, err)

	_, err = l.TriggerAssessment(&types.TriggerMentalHealthReq{UserID: 2, AssessmentType: "daily"})
	require.Error(t, err)
	assert.ErrorIs(t, err, trigger.ErrQueueFull)
}

func TestMentalHealthTriggerLogic_QueueClosed(t *testing.T) {
	t.Parallel()
	queue := trigger.NewTriggerQueue(context.Background(), 1, 4, func(_ context.Context, _ trigger.Request) {})
	require.NoError(t, queue.Close(context.Background()))

	l := NewMentalHealthTriggerLogic(context.Background(), &svc.ServiceContext{
		Config: config.Config{}, TriggerQueue: queue,
	})
	_, err := l.TriggerAssessment(&types.TriggerMentalHealthReq{UserID: 1, AssessmentType: "daily"})
	require.Error(t, err)
	assert.ErrorIs(t, err, trigger.ErrQueueClosed)
}

func TestMentalHealthTriggerLogic_NilQueue_InternalError(t *testing.T) {
	t.Parallel()
	l := NewMentalHealthTriggerLogic(context.Background(), &svc.ServiceContext{
		Config: config.Config{}, TriggerQueue: nil,
	})
	_, err := l.TriggerAssessment(&types.TriggerMentalHealthReq{UserID: 1, AssessmentType: "daily"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "queue", "expected error to mention queue")
}

// 时间引用确保 import live
var _ = time.Second
var _ sync.Mutex