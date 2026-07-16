package messaging

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/IBM/sarama"
)

// KafkaProducerOptions 配置 sarama producer
type KafkaProducerOptions struct {
	ClientID  string         // 标识自己
	TimeoutMs int            // broker dial/operation 超时
	Config    *sarama.Config // 可选：完整配置覆盖
}

// KafkaProducer 是 sarama SyncProducer 之上的薄壳，适配 messaging.Producer 接口
type KafkaProducer struct {
	prod    sarama.SyncProducer
	timeout time.Duration
	once    sync.Once
	closed  bool
	mu      sync.Mutex
}

// NewKafkaProducer 构造一个 Kafka producer
func NewKafkaProducer(brokers []string, opts KafkaProducerOptions) (*KafkaProducer, error) {
	if len(brokers) == 0 {
		return nil, fmt.Errorf("messaging: brokers must not be empty")
	}

	cfg := opts.Config
	if cfg == nil {
		cfg = sarama.NewConfig()
		cfg.ClientID = opts.ClientID
		if opts.ClientID == "" {
			cfg.ClientID = "emotion-echo-gin"
		}
		// 同步 producer + 等待 ACK ack=1（默认值）
		cfg.Producer.RequiredAcks = sarama.WaitForLocal
		cfg.Producer.Return.Successes = true
		cfg.Producer.Retry.Max = 3
		cfg.Net.DialTimeout = 5 * time.Second
		cfg.Producer.Timeout = 5 * time.Second
	}

	if cfg.Net.DialTimeout == 0 {
		cfg.Net.DialTimeout = 5 * time.Second
	}
	prod, err := sarama.NewSyncProducer(brokers, cfg)
	if err != nil {
		return nil, fmt.Errorf("messaging: kafka producer init: %w", err)
	}

	timeout := time.Duration(opts.TimeoutMs) * time.Millisecond
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &KafkaProducer{
		prod:    prod,
		timeout: timeout,
	}, nil
}

// Publish 把消息发出。返回 err 时不重试（调用方决定）
func (p *KafkaProducer) Publish(ctx context.Context, msg Message) error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return ErrProducerClosed
	}
	p.mu.Unlock()

	// ctx 检查
	if err := ctx.Err(); err != nil {
		return err
	}

	if msg.Topic == "" {
		return ErrEmptyTopic
	}
	if len(msg.Value) == 0 {
		return ErrEmptyValue
	}

	pm := &sarama.ProducerMessage{
		Topic: msg.Topic,
		Key:   sarama.ByteEncoder(msg.Key),
		Value: sarama.ByteEncoder(msg.Value),
	}
	for k, v := range msg.Headers {
		pm.Headers = append(pm.Headers, sarama.RecordHeader{
			Key:   []byte(k),
			Value: []byte(v),
		})
	}

	// sarama 的 SendMessage 同步无 ctx；要尊重 ctx 必须用 goroutine + select
	done := make(chan error, 1)
	go func() {
		_, _, sendErr := p.prod.SendMessage(pm)
		done <- sendErr
	}()

	select {
	case <-ctx.Done():
		// ⚠️ 注意：goroutine 仍在写 channel，但 buffer=1 不会阻塞
		// 真实的 broker write 不可能取消，只能等 goroutine 完成
		return ctx.Err()
	case sendErr := <-done:
		if sendErr != nil {
			return fmt.Errorf("messaging: kafka send: %w", sendErr)
		}
		return nil
	case <-time.After(p.timeout):
		return errors.New("messaging: kafka send timeout")
	}
}

// Close 关闭
func (p *KafkaProducer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	var err error
	p.once.Do(func() {
		err = p.prod.Close()
	})
	return err
}
