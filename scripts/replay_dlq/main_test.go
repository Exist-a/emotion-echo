// main_test.go —— replay_dlq 转换逻辑单测（E2E-24 / D-33）
//
// 契约：
//   - 目标 topic = header[x-original-topic] 优先，缺失回退 fallback
//   - DLQ 诊断 headers（x-original-topic/x-error-reason/x-attempts）剥除
//   - 其余 headers（sw8 等）保留（trace 连续性）
//   - 无 x-original-topic 且 fallback 为空 ⇒ 报错（不可静默发错 topic）
package main

import (
	"testing"

	"github.com/IBM/sarama"
)

func TestTransformDLQEntry_UsesOriginalTopicAndStripsDiagnostics(t *testing.T) {
	msg := &sarama.ConsumerMessage{
		Topic: "chat-events-dlq",
		Key:   []byte("evt-1"),
		Value: []byte{0x0a, 0x00, 0x12, 0x05}, // 二进制 payload 原样透传
		Headers: []*sarama.RecordHeader{
			{Key: []byte("x-original-topic"), Value: []byte("chat-events")},
			{Key: []byte("x-error-reason"), Value: []byte("SQLSTATE 42P10")},
			{Key: []byte("x-attempts"), Value: []byte("4")},
			{Key: []byte("sw8"), Value: []byte("1-abc-2-abc-abc-abc-abc")},
		},
	}

	e, err := transformDLQEntry(msg, "fallback-topic")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if e.target != "chat-events" {
		t.Errorf("target = %q, want chat-events（x-original-topic 优先）", e.target)
	}
	for _, h := range e.headers {
		k := string(h.Key)
		if k == "x-original-topic" || k == "x-error-reason" || k == "x-attempts" {
			t.Errorf("诊断 header %q 不应回传业务 topic", k)
		}
	}
	foundSw8 := false
	for _, h := range e.headers {
		if string(h.Key) == "sw8" {
			foundSw8 = true
		}
	}
	if !foundSw8 {
		t.Error("sw8 header 应保留（trace 连续性）")
	}
	if string(e.value) != string(msg.Value) {
		t.Error("payload 必须字节级原样透传（proto 二进制）")
	}
}

func TestTransformDLQEntry_FallsBackWhenNoOriginalTopic(t *testing.T) {
	msg := &sarama.ConsumerMessage{
		Topic: "chat-events-dlq",
		Value: []byte("x"),
		Headers: []*sarama.RecordHeader{
			{Key: []byte("x-attempts"), Value: []byte("2")},
		},
	}
	e, err := transformDLQEntry(msg, "chat-events")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if e.target != "chat-events" {
		t.Errorf("target = %q, want fallback chat-events", e.target)
	}
	if len(e.headers) != 0 {
		t.Errorf("诊断 headers 应剥干净，got %d", len(e.headers))
	}
}

func TestTransformDLQEntry_EmptyTargetFails(t *testing.T) {
	msg := &sarama.ConsumerMessage{Topic: "chat-events-dlq", Value: []byte("x")}
	if _, err := transformDLQEntry(msg, ""); err == nil {
		t.Fatal("无 x-original-topic 且 fallback 为空必须报错（不可静默发错 topic）")
	}
}
