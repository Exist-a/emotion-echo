// Package consumer — dlq.go
//
// Stage 30-C A2: 死信队列（DLQ）抽象。
//
// 背景：
//   ai-svc 当前 consumer.go:98-101 行为是 Handler 返 error → 不 MarkMessage，
//   让 sarama 无限重投。注释说"最终入 DLQ"但无实现 → 毒消息卡死 partition。
//
// A2 方案：
//   重试 N 次（默认 3）后 → 调 DLQPublisher.Publish 发到 chat-events-dlq
//   → MarkMessage 让消费继续。DLQ 消息保留原 payload + 错误原因（sarama headers），
//   便于运营事后回放与告警。
//
// 设计：
//   - DLQPublisher 是接口，InMemory / Kafka 两个实现
//   - NoopDLQPublisher 是 nil-safe 的退化（DLQ 未配置时不发布，但 MarkMessage 仍发生）
//   - 与 Producer 解耦：DLQ 是"消费失败的兜底"，主流程 Producer 失败走另一路径
package consumer

import (
	"context"
	"sync"

	"github.com/IBM/sarama"
)

// DLQEntry 是 DLQ 消息的载体。
//
// Key 沿用原 Kafka message 的 key（事件 ID），保证重放时仍按同 id 同 partition。
// Headers 携带 x-original-topic / x-error-reason / x-attempts 便于排查。
type DLQEntry struct {
	Topic         string            // 原 topic（写 DLQ headers 用）
	Key           []byte            // 事件 ID（同 key 同 partition）
	Value         []byte            // 原 payload（不解码，原样转发）
	Attempts      int               // 重试次数（最后一次失败时累计）
	LastError     string            // 最后一次错误信息
	OriginalTopic string            // 来源 topic（chat-events）
	Headers       map[string]string // 透传的原 headers（若有）
}

// DLQPublisher 把消费失败的毒消息投到 DLQ topic。
//
// Stage 30-C A2 接口契约：
//   - 失败不应阻塞主流程（即使 Kafka 不可达，DLQ Publish 返 error 也由 caller 选择 log/skip）
//   - 实现必须是线程安全的（多 partition 并发投递）
type DLQPublisher interface {
	Publish(ctx context.Context, entry DLQEntry) error
}

// NoopDLQPublisher 是不做任何事的实现。
//
// 用于：
//   - 单元测试（不需要真实 broker）
//   - 生产环境 DLQ 未配置时的退化（与 plan 中 "DLQNilIsSafe" 一致）
//
// 注意：NoopDLQPublisher 不会报错，但调用方仍需 MarkMessage 让消费继续。
type NoopDLQPublisher struct{}

// Publish no-op
func (NoopDLQPublisher) Publish(_ context.Context, _ DLQEntry) error { return nil }

// =====================================================
// InMemoryDLQPublisher（测试替身）
// =====================================================

// InMemoryDLQPublisher 捕获 DLQ 调用，便于断言"收到了哪条毒消息"。
// 线程安全（用 mutex 保护 captured）。
type InMemoryDLQPublisher struct {
	mu       sync.Mutex
	captured []DLQEntry
	err      error // 模拟 Publish 失败
}

func NewInMemoryDLQPublisher() *InMemoryDLQPublisher {
	return &InMemoryDLQPublisher{}
}

func (p *InMemoryDLQPublisher) Publish(_ context.Context, entry DLQEntry) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.err != nil {
		return p.err
	}
	p.captured = append(p.captured, entry)
	return nil
}

// Captured 返回所有收到的 DLQ entry（测试用）
func (p *InMemoryDLQPublisher) Captured() []DLQEntry {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]DLQEntry, len(p.captured))
	copy(out, p.captured)
	return out
}

// SetError 模拟 DLQ publish 失败（测试 DLQ 自身的失败路径）
func (p *InMemoryDLQPublisher) SetError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.err = err
}

// Reset 清空捕获（子测试间复用同一 publisher 时）
func (p *InMemoryDLQPublisher) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.captured = nil
	p.err = nil
}

// =====================================================
// KafkaDLQPublisher（生产实现 — commit 6 才填实现）
// =====================================================

// KafkaDLQPublisher sarama 实现的 DLQ 生产者。
// 真实实现在 commit 6 落地；此处仅声明类型占位。
type KafkaDLQPublisher struct {
	producer sarama.SyncProducer
	topic    string
}

// NewKafkaDLQPublisher 构造 Kafka DLQ 生产者。
// 实现见 commit 6。
func NewKafkaDLQPublisher(brokers []string, dlqTopic string) (*KafkaDLQPublisher, error) {
	cfg := sarama.NewConfig()
	cfg.Producer.RequiredAcks = sarama.WaitForAll
	cfg.Producer.Retry.Max = 3
	cfg.Producer.Return.Successes = true
	cfg.Producer.Return.Errors = true
	cfg.Version = sarama.V2_8_0_0

	prod, err := sarama.NewSyncProducer(brokers, cfg)
	if err != nil {
		return nil, err
	}
	return &KafkaDLQPublisher{producer: prod, topic: dlqTopic}, nil
}

// Publish 把 DLQ entry 发到 DLQ topic。
func (p *KafkaDLQPublisher) Publish(_ context.Context, entry DLQEntry) error {
	headers := []sarama.RecordHeader{
		{Key: []byte("x-original-topic"), Value: []byte(entry.OriginalTopic)},
		{Key: []byte("x-error-reason"), Value: []byte(entry.LastError)},
		{Key: []byte("x-attempts"), Value: []byte(itoa(entry.Attempts))},
	}
	for k, v := range entry.Headers {
		headers = append(headers, sarama.RecordHeader{Key: []byte(k), Value: []byte(v)})
	}
	msg := &sarama.ProducerMessage{
		Topic:   p.topic,
		Key:     sarama.ByteEncoder(entry.Key),
		Value:   sarama.ByteEncoder(entry.Value),
		Headers: headers,
	}
	_, _, err := p.producer.SendMessage(msg)
	return err
}

// Close 关闭 producer。
func (p *KafkaDLQPublisher) Close() error { return p.producer.Close() }

// itoa 避免引入 strconv 包以保持 dlq.go 轻量；attempts 计数通常 ≤10
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var b [20]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}
