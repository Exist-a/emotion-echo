package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"emotion-echo-chat-svc/internal/repository"
	sharedmetrics "github.com/emotion-echo/shared/pkg/metrics"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deadCounter 取全局 dead 计数器当前值（指标不存在时返回 0）。
// 用 delta 断言而非绝对值：relay 测试是 t.Parallel，全局 counter 可能被并发递增。
func deadCounter(t *testing.T) float64 {
	t.Helper()
	v, err := sharedmetrics.RegistryGatherCounter("emotion_echo_outbox_events_dead_total", nil)
	require.NoError(t, err)
	return v
}

// TestRelay_MaxAttemptsReached_MarksDead 回归锁（Stage 43 PR-A6.1 落地时漏了 relay 级测试，
// 只有 repo 级 outbox_test.go）+ dead 指标断言（Stage 86）。
//
// MaxAttempts=2：第 1 轮失败 → 仍 pending（attempts=1）；第 2 轮失败 → status=dead，
// 不再被 ListPending 扫到，且 dead 计数器 +1。
//
// 注意：断言全局 Prometheus counter，不能 t.Parallel（并发测试会互踩 delta）。
func TestRelay_MaxAttemptsReached_MarksDead(t *testing.T) {
	repo := repository.NewInMemoryOutboxRepo()
	pub := &failingPublisher{err: errors.New("forced publish err")}

	entry := &repository.OutboxEvent{
		EventID:   "evt-dead-1",
		EventType: "message.created",
		Topic:     "chat-events",
		Payload:   []byte(`{"id":"evt-dead-1","type":"message.created","source":"chat-svc"}`),
	}
	require.NoError(t, repo.CreateInTx(nil, entry))
	deadID := entry.ID // CreateInTx 原地回填 ID

	before := deadCounter(t)

	r := NewRelay(repo, pub, 100*time.Millisecond, 10)
	r.MaxAttempts = 2

	// 第 1 轮：MarkFailed，仍 pending
	require.NoError(t, r.FlushOnce(context.Background()))
	pending, _ := repo.ListPending(context.Background(), 10)
	require.Len(t, pending, 1)
	assert.Equal(t, 1, pending[0].Attempts)
	assert.Equal(t, repository.OutboxStatusPending, pending[0].Status)

	// 第 2 轮：attempts 达到 MaxAttempts → MarkDead + 计数器 +1
	require.NoError(t, r.FlushOnce(context.Background()))
	pending, _ = repo.ListPending(context.Background(), 10)
	assert.Empty(t, pending, "dead 行不应再被 ListPending 扫到")

	got, err := repo.Get(deadID)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, repository.OutboxStatusDead, got.Status)
	assert.Equal(t, 2, got.Attempts)
	assert.Equal(t, "forced publish err", got.LastError)

	assert.Equal(t, before+1, deadCounter(t), "MarkDead 成功应递增 dead 计数器")
}

// TestRelay_MaxAttemptsZero_DisablesDead MaxAttempts=0 视作关闭 dead 状态机（向后兼容）：
// 反复失败永远 pending，计数器不动。（同上，断言全局 counter，不能 t.Parallel）
func TestRelay_MaxAttemptsZero_DisablesDead(t *testing.T) {
	repo := repository.NewInMemoryOutboxRepo()
	pub := &failingPublisher{err: errors.New("forced publish err")}

	require.NoError(t, repo.CreateInTx(nil, &repository.OutboxEvent{
		EventID:   "evt-nodead-1",
		EventType: "message.created",
		Topic:     "chat-events",
		Payload:   []byte(`{"id":"evt-nodead-1","type":"message.created","source":"chat-svc"}`),
	}))

	before := deadCounter(t)

	r := NewRelay(repo, pub, 100*time.Millisecond, 10)
	r.MaxAttempts = 0
	for i := 0; i < 3; i++ {
		require.NoError(t, r.FlushOnce(context.Background()))
	}

	pending, _ := repo.ListPending(context.Background(), 10)
	require.Len(t, pending, 1)
	assert.Equal(t, 3, pending[0].Attempts, "MaxAttempts=0 应保持原无限重试行为")
	assert.Equal(t, before, deadCounter(t), "未进 dead 状态不应递增计数器")
}
