package kafka

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/IBM/sarama"
)

// ===== CL-03 Phase 4b（CL-D-17=A）analytics-svc 侧：DLQ 投递失败 ⇒ 不 Mark =====
//
// 修复前：`consumer.go` 无条件 `sess.MarkMessage` ⇒ 业务 + DLQ 双失败 = 消息永久丢失。
// 修复后：只有 DLQ.Publish 真返回错误时才"不 Mark + 结束本次 claim"（原因见 ai-svc
// 同名测试文件的注释：sarama 提交的是"已 Mark 的最高 offset"，仅跳过本条继续 Mark
// 后续会让这条被隐式跳过）。
//
// 重投安全性（已核实）：本 handler 的唯一变更面是 `event_repository.Create` →
// `ON CONFLICT (event_id, occurred_at) DO NOTHING`（分区表 UNIQUE，见
// event_repository_partition_conflict_test.go）⇒ 幂等。

// failingDLQ93 永远投递失败。
type failingDLQ93 struct{ calls int }

func (p *failingDLQ93) Publish(context.Context, DLQEntry) error {
	p.calls++
	return errors.New("dlq broker unreachable")
}

// driveClaim93Capture 与既有 driveConsumeClaim93 同形，但**返回 session** 以便断言 Mark。
func driveClaim93Capture(t *testing.T, h *chatEventHandler, msgs ...*sarama.ConsumerMessage) *fakeSession93 {
	t.Helper()
	claim := &fakeClaim93{msgs: make(chan *sarama.ConsumerMessage, len(msgs))}
	sess := &fakeSession93{}
	for _, m := range msgs {
		claim.msgs <- m
	}
	close(claim.msgs)

	done := make(chan error, 1)
	go func() { done <- h.ConsumeClaim(sess, claim) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ConsumeClaim err: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("ConsumeClaim timeout")
	}
	return sess
}

// newPoisonHandler93 构造"写入必失败 + 预算 1 次重试"的消费者，直达 DLQ 分支。
func newPoisonHandler93(dlq DLQPublisher) *chatEventHandler {
	return &chatEventHandler{
		repo:      &captureEventRepo{err: errors.New("forced err")},
		topic:     "chat-events",
		dlq:       dlq,
		retries:   newInt32Ptr(1),
		backoffFn: func(int) time.Duration { return 0 },
	}
}

func poisonMsg93(id string) *sarama.ConsumerMessage {
	return &sarama.ConsumerMessage{
		Topic: "chat-events",
		Key:   []byte(id),
		Value: []byte(`{"type":"message.created","id":"` + id + `","data":{"userId":1}}`),
	}
}

// TestConsumeClaim93_DLQPublishFails_DoesNotMarkMessage ★ 本次修复的核心断言 ★
func TestConsumeClaim93_DLQPublishFails_DoesNotMarkMessage(t *testing.T) {
	t.Parallel()
	dlq := &failingDLQ93{}
	h := newPoisonHandler93(dlq)

	sess := driveClaim93Capture(t, h, poisonMsg93("evt93-dlq-fail-1"))

	if dlq.calls == 0 {
		t.Fatal("应尝试过 DLQ 投递")
	}
	if len(sess.marked) != 0 {
		t.Errorf("DLQ 投递失败时**不得** Mark（否则消息永久丢失），marked=%v", sess.marked)
	}
}

// TestConsumeClaim93_DLQPublishFails_DoesNotSkipLaterMessages 首条 DLQ 失败必须停在本条，
// 不得继续 Mark 后续消息（否则 committed offset 前进 ⇒ 本条被隐式跳过）。
func TestConsumeClaim93_DLQPublishFails_DoesNotSkipLaterMessages(t *testing.T) {
	t.Parallel()
	h := newPoisonHandler93(&failingDLQ93{})

	sess := driveClaim93Capture(t, h, poisonMsg93("evt93-dlq-fail-2a"), poisonMsg93("evt93-dlq-fail-2b"))

	if len(sess.marked) != 0 {
		t.Errorf("首条 DLQ 失败后必须停在本条，marked=%v", sess.marked)
	}
}

// TestConsumeClaim93_NoopDLQ_StillMarks NoopDLQPublisher 返回 nil = "成功语义"，
// **不属于**投递失败 ⇒ 行为不变（防止把 Noop 误判为失败而卡死 dev 消费）。
func TestConsumeClaim93_NoopDLQ_StillMarks(t *testing.T) {
	t.Parallel()
	h := newPoisonHandler93(NoopDLQPublisher{})

	sess := driveClaim93Capture(t, h, poisonMsg93("evt93-noop-1"))

	if len(sess.marked) != 1 {
		t.Errorf("NoopDLQPublisher 应视为成功 ⇒ Mark 前进，marked=%v", sess.marked)
	}
}
