package consumer

import (
	"context"
	"errors"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/events"

	"github.com/IBM/sarama"
)

// ===== CL-03 Phase 4b（CL-D-17=A）：DLQ 投递失败 ⇒ 不 Mark、不丢消息 =====
//
// 修复前：`consumer.go` 无条件 `sess.MarkMessage` ⇒ **业务 + DLQ 双失败 = 消息彻底丢失**
// （只有 counter 可见，属"可观测的黑洞"）。
//
// 修复后：只有 `DLQ.Publish` 真的返回错误时才"不 Mark + 结束本次 claim"。
// **为什么必须结束 claim 而不是仅跳过本条**：sarama 提交的是"已 Mark 的最高 offset"
// ⇒ 若继续消费并 Mark 后续消息，这条未 Mark 的会被**隐式跳过**（committed offset 前进），
// 重投保证失效。结束 claim 会让分区在下次 session 从上次提交位点重投。
//
// 重投安全性（已核实，见 commit message）：本 handler 的唯一变更面是
// `logic/consumehandler.go:88 h.repo.Create(...)` → `ON CONFLICT (event_id) DO NOTHING`
// （`repository/emotion_repository.go:220-228`）⇒ 幂等，重投不产生重复数据。
//
// 注意区分三种情形（后两种行为不变）：
//   - **DLQ 未配置**（`DLQ == nil`，dev）：无处投递 ⇒ 照旧 Mark 前进（否则 dev 直接卡死）
//   - **DLQ 投递成功**：照旧 Mark（消息已被安全保存）
//   - **DLQ 投递失败**：本条不 Mark（本次修复）

// failingDLQPublisher 永远投递失败。
type failingDLQPublisher struct{ calls int }

func (p *failingDLQPublisher) Publish(context.Context, DLQEntry) error {
	p.calls++
	return errors.New("dlq broker unreachable")
}

// newPoisonHandler 构造"handler 必失败 + 预算 0 重试"的消费者，便于直达 DLQ 分支。
func newPoisonHandler(dlq DLQPublisher) *ConsumerGroupHandler {
	return &ConsumerGroupHandler{
		Ready:      make(chan bool),
		Handler:    func(context.Context, *events.Event) error { return errors.New("forced handler err") },
		DLQ:        dlq,
		MaxRetries: 1,
		BackoffFn:  func(int) time.Duration { return 0 },
	}
}

func poisonMsg(id string) *sarama.ConsumerMessage {
	return &sarama.ConsumerMessage{
		Topic: "chat-events",
		Key:   []byte(id),
		Value: []byte(`{"type":"message.created","id":"` + id + `"}`),
	}
}

// TestConsumeClaim_DLQPublishFails_DoesNotMarkMessage ★ 本次修复的核心断言 ★
func TestConsumeClaim_DLQPublishFails_DoesNotMarkMessage(t *testing.T) {
	t.Parallel()
	dlq := &failingDLQPublisher{}
	h := newPoisonHandler(dlq)

	marked := driveConsumeClaim(t, h, []*sarama.ConsumerMessage{poisonMsg("evt-dlq-fail-1")})

	if dlq.calls == 0 {
		t.Fatal("应尝试过 DLQ 投递")
	}
	if len(marked) != 0 {
		t.Errorf("DLQ 投递失败时**不得** Mark（否则消息永久丢失），marked=%v", marked)
	}
}

// TestConsumeClaim_DLQPublishFails_DoesNotSkipLaterMessages 修复前若"只跳过本条但继续 Mark 后续"，
// sarama 会因 committed offset 前进而**隐式跳过**这条未 Mark 的消息 ⇒ 必须结束本次 claim。
func TestConsumeClaim_DLQPublishFails_DoesNotSkipLaterMessages(t *testing.T) {
	t.Parallel()
	h := newPoisonHandler(&failingDLQPublisher{})

	marked := driveConsumeClaim(t, h, []*sarama.ConsumerMessage{
		poisonMsg("evt-dlq-fail-2a"),
		poisonMsg("evt-dlq-fail-2b"),
	})

	if len(marked) != 0 {
		t.Errorf("首条 DLQ 失败后必须停在本条（后续消息留给下次 session 重投），marked=%v", marked)
	}
}

// TestConsumeClaim_DLQUnconfigured_StillMarks 未配置 DLQ（dev）时行为**不变**：
// 无处可投但也不能卡死消费 ⇒ 照旧 Mark 前进。
func TestConsumeClaim_DLQUnconfigured_StillMarks(t *testing.T) {
	t.Parallel()
	h := newPoisonHandler(nil)

	marked := driveConsumeClaim(t, h, []*sarama.ConsumerMessage{poisonMsg("evt-no-dlq-1")})

	if len(marked) != 1 {
		t.Errorf("DLQ 未配置时应照旧 Mark 前进（dev 不得卡死），marked=%v", marked)
	}
}

// TestConsumeClaim_NoopDLQ_StillMarks NoopDLQPublisher（返回 nil 的成功语义）不属于"投递失败"
// ⇒ 行为不变（本用例防止把 Noop 误判成失败而卡死消费）。
func TestConsumeClaim_NoopDLQ_StillMarks(t *testing.T) {
	t.Parallel()
	h := newPoisonHandler(NoopDLQPublisher{})

	marked := driveConsumeClaim(t, h, []*sarama.ConsumerMessage{poisonMsg("evt-noop-dlq-1")})

	if len(marked) != 1 {
		t.Errorf("NoopDLQPublisher 返回 nil（成功语义）时应 Mark 前进，marked=%v", marked)
	}
}
