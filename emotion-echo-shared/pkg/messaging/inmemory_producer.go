package messaging

import (
	"context"
	"sync"
)

// InMemoryProducer 进程内内存版，用于单元测试与本地调试。
// 数据存到 chan，无外部依赖。
type InMemoryProducer struct {
	mu       sync.Mutex
	closed   bool
	buffer   []Message
	listener chan Message // 测试时可订阅
}

// NewInMemoryProducer 构造一个 in-memory producer
func NewInMemoryProducer() *InMemoryProducer {
	return &InMemoryProducer{
		buffer:   make([]Message, 0, 16),
		listener: make(chan Message, 64),
	}
}

// Publish 把消息写入内存 buffer。先校验输入，再尊重 ctx。
func (p *InMemoryProducer) Publish(ctx context.Context, msg Message) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return ErrProducerClosed
	}
	if msg.Topic == "" {
		return ErrEmptyTopic
	}
	if len(msg.Value) == 0 {
		return ErrEmptyValue
	}
	// ctx 检查（必须在加锁后做，避免 race）
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	p.buffer = append(p.buffer, msg)
	// 异步推送到 listener（非阻塞；buffer 满即丢，不影响 buffer）
	select {
	case p.listener <- msg:
	default:
	}
	return nil
}

// Close 关闭 producer
func (p *InMemoryProducer) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return nil
	}
	p.closed = true
	close(p.listener)
	return nil
}

// Drain 排空当前 buffer（仅用于测试断言）
//
// Stage 26-A 暴露 bug #6 修复：原 copy() 仅 shallow-copy slice header，
// 外部修改消息的 Value 会污染内部 buffer。
//
// 本实现：每条 message 拷贝一份（Value 深拷贝 bytes）。
// 注意：Headers map 是浅拷贝，调用方不应原地修改；若需求更严可
// 改为 json.Marshal/Unmarshal → map[string]any → 重新解析。
func (p *InMemoryProducer) Drain() []Message {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]Message, len(p.buffer))
	for i, m := range p.buffer {
		out[i].Topic = m.Topic
		out[i].Key = m.Key
		if len(m.Value) > 0 {
			out[i].Value = append([]byte(nil), m.Value...)
		} else {
			out[i].Value = m.Value // 保留 nil/[]byte{} 不变
		}
		out[i].Headers = m.Headers
	}
	return out
}
