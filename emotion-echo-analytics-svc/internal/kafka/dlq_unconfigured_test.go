package kafka

import (
	"context"
	"errors"
	"testing"

	"github.com/IBM/sarama"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

// TestPublishToDLQ_Noop_IncrementsCounter ★ CL-03 T-07（Noop 静默，analytics 侧）★
//
// analytics-svc 的 NewConsumer 默认装配 NoopDLQPublisher（Publish 返回 nil = "成功"）
// ⇒ 此前"毒消息被丢弃"与"投递成功"在指标上无法区分。本用例钉住：Noop 时必须递增
// emotion_echo_analytics_dlq_unconfigured_total。
func TestPublishToDLQ_Noop_IncrementsCounter(t *testing.T) {
	before := testutil.ToFloat64(DLQUnconfiguredTotal)

	h := &chatEventHandler{dlq: NoopDLQPublisher{}} // 默认装配形态
	msg := &sarama.ConsumerMessage{Topic: "chat-events", Key: []byte("evt93-noop"), Value: []byte("x")}

	assert.NoError(t, h.publishToDLQ(context.Background(), msg, errors.New("boom"), 3))

	assert.Equal(t, before+1, testutil.ToFloat64(DLQUnconfiguredTotal),
		"Noop DLQ 时必须递增 emotion_echo_analytics_dlq_unconfigured_total")
}

// TestPublishToDLQ_RealDLQ_DoesNotIncrementUnconfigured 反向保证：真 DLQ 时不得动该 counter。
func TestPublishToDLQ_RealDLQ_DoesNotIncrementUnconfigured(t *testing.T) {
	before := testutil.ToFloat64(DLQUnconfiguredTotal)

	h := &chatEventHandler{dlq: NewInMemoryDLQPublisher()}
	msg := &sarama.ConsumerMessage{Topic: "chat-events", Key: []byte("evt93-real"), Value: []byte("x")}

	assert.NoError(t, h.publishToDLQ(context.Background(), msg, errors.New("boom"), 3))

	assert.Equal(t, before, testutil.ToFloat64(DLQUnconfiguredTotal))
}

// TestPublishToDLQ_NoopPointer_IncrementsCounter 装配方可能写 &NoopDLQPublisher{}（指针形态）
// —— 与值形态必须同判，否则同型缺陷会从"指针装配"这一侧重新漏回来。
func TestPublishToDLQ_NoopPointer_IncrementsCounter(t *testing.T) {
	before := testutil.ToFloat64(DLQUnconfiguredTotal)

	h := &chatEventHandler{dlq: &NoopDLQPublisher{}}
	msg := &sarama.ConsumerMessage{Topic: "chat-events", Key: []byte("evt93-noop-ptr"), Value: []byte("x")}

	assert.NoError(t, h.publishToDLQ(context.Background(), msg, errors.New("boom"), 3))

	assert.Equal(t, before+1, testutil.ToFloat64(DLQUnconfiguredTotal),
		"指针形态 Noop DLQ 也必须递增 counter")
}
