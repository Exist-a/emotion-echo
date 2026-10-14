// Package kafka — CL-01 T-05 / G4：analytics-svc DLQ 投递建 exit span 回归钉。
//
// 与 ai-svc 同构：DLQ 投递必须建 exit span（opName="kafka-publish-dlq"），
// 并把 sw8 注入 DLQ headers。修前 RED：exitOpCalls 为空。
package kafka

import (
	"context"
	"errors"
	"testing"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublishToDLQ_CreatesExitSpan(t *testing.T) {
	tracer := &mockTracer93{exitSpan: &mockSpan93{}}
	pub := NewInMemoryDLQPublisher()
	h := &chatEventHandler{dlq: pub, Tracer: tracer}

	msg := &sarama.ConsumerMessage{Topic: "chat-events", Key: []byte("1"), Value: []byte("payload")}
	h.publishToDLQ(context.Background(), msg, errors.New("boom"), 4)

	require.Len(t, tracer.exitOpCalls, 1, "DLQ 投递必须建 1 个 exit span（修前 0 ⇒ RED）")
	assert.Equal(t, "kafka-publish-dlq", tracer.exitOpCalls[0])

	captured := pub.Captured()
	require.Len(t, captured, 1, "DLQ 应收到 1 条消息")
	assert.Equal(t, "mock-sw8", captured[0].Headers["sw8"],
		"sw8 必须注入 DLQ headers（trace 续接）")
}

func TestPublishToDLQ_NilDLQ_NoSpan(t *testing.T) {
	tracer := &mockTracer93{}
	h := &chatEventHandler{dlq: nil, Tracer: tracer}

	msg := &sarama.ConsumerMessage{Topic: "chat-events", Key: []byte("1")}
	h.publishToDLQ(context.Background(), msg, errors.New("boom"), 4)

	assert.Empty(t, tracer.exitOpCalls, "DLQ 未配置时不应建 DLQ span")
}
