package outbox

import (
	"context"
	"errors"
	"testing"
	"time"

	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ===== CL-03 Phase 5（CL-D-19=C / CL-F-15）：Kafka 未就绪时**不假 sent** =====
//
// 语义：relay 拿到 `ErrPublisherUnavailable`（publisher 尚未就绪、后台正在重试）时
//   - 不得 MarkSent（否则事件永久丢失，且 outbox 全 sent 是"假绿信号"）
//   - **也不得 MarkFailed/累加 attempts** —— 否则一次长时间 Kafka 故障会把所有行
//     推到 attempts ≥ MaxAttempts ⇒ 全变 dead（dead 行不再被扫描 = 同样丢）
//   - 行原样留 pending，Kafka 恢复后由真 publisher 自然重发

func TestRelay_PublisherUnavailable_KeepsRowPendingWithoutAttempts(t *testing.T) {
	repo := repository.NewInMemoryOutboxRepo()
	mustCreateOutboxRow(t, repo, "evt-unavail-1", "1-abc-1-2-0-svc-inst-ep-1.1.1.1:80")

	unavail := events.NewUnavailablePublisher(errors.New("kafka not ready"))
	r := NewRelay(repo, unavail, 100*time.Millisecond, 10)

	require.NoError(t, r.FlushOnce(context.Background()))

	pending, err := repo.ListPending(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, pending, 1, "publisher 未就绪时行必须留 pending（不得 MarkSent）")
	assert.Equal(t, 0, pending[0].Attempts,
		"未就绪**不得**累加 attempts —— 否则长时间故障会把行推到 dead（dead 不再被扫描 = 丢消息）")
	assert.Empty(t, pending[0].LastError)
}

func TestRelay_PublisherFn_ResolvedPerFlush(t *testing.T) {
	repo := repository.NewInMemoryOutboxRepo()
	mustCreateOutboxRow(t, repo, "evt-unavail-2", "1-abc-1-2-0-svc-inst-ep-1.1.1.1:80")

	// 第一轮：未就绪；第二轮：就绪（模拟后台重试成功后的热替换）
	ready := false
	real := events.NewInMemoryEventPublisher()
	r := NewRelay(repo, events.NewUnavailablePublisher(nil), 100*time.Millisecond, 10)
	r.PublisherFn = func() events.EventPublisher {
		if ready {
			return real
		}
		return events.NewUnavailablePublisher(nil)
	}

	require.NoError(t, r.FlushOnce(context.Background()))
	pending, _ := repo.ListPending(context.Background(), 10)
	require.Len(t, pending, 1, "第一轮未就绪 ⇒ 留 pending")

	ready = true
	require.NoError(t, r.FlushOnce(context.Background()))
	pending, _ = repo.ListPending(context.Background(), 10)
	assert.Empty(t, pending, "就绪后下一轮应自然重发并 MarkSent")
	assert.Len(t, real.Events("chat-events"), 1, "事件应真的发到 publisher")
}

func TestRelay_PublisherFn_ReturnsNil_TreatedAsUnavailable(t *testing.T) {
	repo := repository.NewInMemoryOutboxRepo()
	mustCreateOutboxRow(t, repo, "evt-unavail-3", "")

	r := NewRelay(repo, nil, 100*time.Millisecond, 10)
	r.PublisherFn = func() events.EventPublisher { return nil }

	require.NoError(t, r.FlushOnce(context.Background()))
	pending, _ := repo.ListPending(context.Background(), 10)
	require.Len(t, pending, 1, "PublisherFn 返回 nil = 未就绪 ⇒ 留 pending")
	assert.Equal(t, 0, pending[0].Attempts)
}

// TestRelay_RealPublishFailure_StillCountsAttempts 反向保证：**真的发布失败**
// （非"未就绪"）仍要 MarkFailed 累加 attempts 并按 MaxAttempts 转 dead —— 语义不能被本改动冲掉。
func TestRelay_RealPublishFailure_StillCountsAttempts(t *testing.T) {
	repo := repository.NewInMemoryOutboxRepo()
	mustCreateOutboxRow(t, repo, "evt-unavail-4", "")

	r := NewRelay(repo, &failingPublisher{err: errors.New("forced publish err")}, 100*time.Millisecond, 10)
	require.NoError(t, r.FlushOnce(context.Background()))

	pending, _ := repo.ListPending(context.Background(), 10)
	require.Len(t, pending, 1)
	assert.Equal(t, 1, pending[0].Attempts, "真发布失败必须累加 attempts（走既有 dead 状态机）")
	assert.NotEmpty(t, pending[0].LastError)
}
