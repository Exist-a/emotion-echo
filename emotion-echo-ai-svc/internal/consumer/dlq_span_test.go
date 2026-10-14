// Package consumer — CL-01 T-05 / G4：DLQ 投递建 exit span 回归钉。
//
// 背景：DLQ 投递是"毒消息最后的去向"，此前只调 DLQ.Publish、无任何 span，
// OAP 上看不到 DLQ 投递（黑洞）。且原实现用 sess.Context()（无父 span）——
// 即便加 span 也会被约束③守卫跳过。本测试断言：
//   - publishToDLQ 建 1 个 exit span，opName = "kafka-publish-dlq"
//   - sw8 被注入 DLQ headers（DLQ 消息可续上同一条 trace，不再是孤儿）
//
// 修前 RED：exitOpCalls 为空。
package consumer

import (
	"context"
	"errors"
	"testing"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishToDLQ_CreatesExitSpan(t *testing.T) {
	tracer := &mockTracer{exitSpan: &mockSpan{}}
	pub := NewInMemoryDLQPublisher()
	h := &ConsumerGroupHandler{DLQ: pub, Tracer: tracer}

	msg := &sarama.ConsumerMessage{Topic: "chat-events", Key: []byte("1"), Value: []byte("payload")}
	h.publishToDLQ(context.Background(), msg, errors.New("boom"), 4)

	require.Len(t, tracer.exitOpCalls, 1, "DLQ 投递必须建 1 个 exit span（修前 0 ⇒ RED）")
	assert.Equal(t, "kafka-publish-dlq", tracer.exitOpCalls[0])

	captured := pub.Captured()
	require.Len(t, captured, 1, "DLQ 应收到 1 条消息")
	assert.Equal(t, "mock-sw8", captured[0].Headers["sw8"],
		"sw8 必须注入 DLQ headers（trace 续接，DLQ 消息不再是孤儿）")
}

// DLQ 未配置（nil）时不得建 span / 不得 panic。
func TestPublishToDLQ_NilDLQ_NoSpan(t *testing.T) {
	tracer := &mockTracer{exitSpan: &mockSpan{}}
	h := &ConsumerGroupHandler{DLQ: nil, Tracer: tracer}

	msg := &sarama.ConsumerMessage{Topic: "chat-events", Key: []byte("1")}
	h.publishToDLQ(context.Background(), msg, errors.New("boom"), 4)

	assert.Empty(t, tracer.exitOpCalls, "DLQ 未配置时不应建 DLQ span")
}
