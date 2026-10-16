package consumer

import (
	"context"
	"errors"
	"testing"

	"github.com/IBM/sarama"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

// TestPublishToDLQ_Unconfigured_IncrementsCounter ★ CL-03 T-07（Noop 静默）★
//
// DLQ 未配置（dev 常见）时毒消息**无处可去、不可回放** —— 此前只打一条与"已投 DLQ"
// 同款的日志，读者无法区分"进了 DLQ"还是"被丢弃"（静默丢弃）。本用例钉住：
// 未配置时必须递增 emotion_echo_dlq_unconfigured_total，让丢弃成为可观测事实。
func TestPublishToDLQ_Unconfigured_IncrementsCounter(t *testing.T) {
	before := testutil.ToFloat64(DLQUnconfiguredTotal)

	h := &ConsumerGroupHandler{DLQ: nil} // 未配置 DLQ
	msg := &sarama.ConsumerMessage{Topic: "chat-events", Key: []byte("evt-no-dlq"), Value: []byte("x")}

	err := h.publishToDLQ(context.Background(), msg, errors.New("handler boom"), 4)

	assert.NoError(t, err, "未配置 DLQ 仍返回 nil（调用方照旧 Mark 前进，不得卡死 dev 消费）")
	assert.Equal(t, before+1, testutil.ToFloat64(DLQUnconfiguredTotal),
		"DLQ 未配置时必须递增 emotion_echo_dlq_unconfigured_total")
}

// TestPublishToDLQ_Configured_DoesNotIncrementUnconfigured 反向保证：
// DLQ **已配置**时不得动这个 counter（否则它就不是"未配置"的信号了）。
func TestPublishToDLQ_Configured_DoesNotIncrementUnconfigured(t *testing.T) {
	before := testutil.ToFloat64(DLQUnconfiguredTotal)

	h := &ConsumerGroupHandler{DLQ: NewInMemoryDLQPublisher()}
	msg := &sarama.ConsumerMessage{Topic: "chat-events", Key: []byte("evt-with-dlq"), Value: []byte("x")}

	assert.NoError(t, h.publishToDLQ(context.Background(), msg, errors.New("boom"), 2))

	assert.Equal(t, before, testutil.ToFloat64(DLQUnconfiguredTotal),
		"已配置 DLQ 时不得递增 unconfigured counter")
}
