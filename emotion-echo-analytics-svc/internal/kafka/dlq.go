// Package kafka — dlq.go
//
// Stage 30-C A2: analytics-svc 死信队列抽象。
//
// 与 ai-svc 同构但放在本包内（不抽到 shared 包），原因：
//   - analytics-svc 用 sarama 包直接 + handleOne 风格，与 ai-svc 的 ConsumerGroupHandler 风格不同
//   - DLQPublisher 接口和 InMemoryDLQPublisher 重复实现可以接受（2 个 svc 共 60 行）
//   - 共享 future 阶段可走 B2 Schema Registry 合并
//
// 失败语义（与 ai-svc 对齐）：
//   - attempt < MaxRetries：返回 error 让 sarama 不 MarkMessage（自动重投）
//   - attempt >= MaxRetries：调 DLQ.Publish + MarkMessage + 重置计数
//   - DLQ 为 nil：原行为是"静默丢"，新行为改为"返回 error 触发重投 + 计数"
package kafka

import (
	"context"
	"sync"

	"github.com/IBM/sarama"
)

// DLQEntry 是 DLQ 消息的载体
type DLQEntry struct {
	Topic         string
	Key           []byte
	Value         []byte
	Attempts      int
	LastError     string
	OriginalTopic string
}

// DLQPublisher 抽象：把毒消息投到 DLQ topic
type DLQPublisher interface {
	Publish(ctx context.Context, entry DLQEntry) error
}

// NoopDLQPublisher 不做任何事（DLQ 未配置时）
type NoopDLQPublisher struct{}

func (NoopDLQPublisher) Publish(_ context.Context, _ DLQEntry) error { return nil }

// InMemoryDLQPublisher 线程安全的测试替身
type InMemoryDLQPublisher struct {
	mu       sync.Mutex
	captured []DLQEntry
	err      error
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

func (p *InMemoryDLQPublisher) Captured() []DLQEntry {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]DLQEntry, len(p.captured))
	copy(out, p.captured)
	return out
}

func (p *InMemoryDLQPublisher) Reset() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.captured = nil
	p.err = nil
}

func (p *InMemoryDLQPublisher) SetError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.err = err
}

// KafkaDLQPublisher sarama 实现的 DLQ 生产者
type KafkaDLQPublisher struct {
	producer sarama.SyncProducer
	topic    string
}

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

func (p *KafkaDLQPublisher) Publish(_ context.Context, entry DLQEntry) error {
	headers := []sarama.RecordHeader{
		{Key: []byte("x-original-topic"), Value: []byte(entry.OriginalTopic)},
		{Key: []byte("x-error-reason"), Value: []byte(entry.LastError)},
		{Key: []byte("x-attempts"), Value: []byte(itoa(entry.Attempts))},
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

func (p *KafkaDLQPublisher) Close() error { return p.producer.Close() }

// itoa 同 ai-svc，避免引入 strconv（attempts 计数通常 ≤10）
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
