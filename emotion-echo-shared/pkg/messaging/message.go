package messaging

import "errors"

// 业务错误：调用方可通过 errors.Is 判断
var (
	ErrEmptyTopic       = errors.New("messaging: topic must not be empty")
	ErrEmptyValue       = errors.New("messaging: value must not be empty")
	ErrProducerClosed   = errors.New("messaging: producer is closed")
	ErrPublishCancelled = errors.New("messaging: publish cancelled")
)

// Message 是消息载体，跨 producer/consumer 共享
type Message struct {
	Topic   string
	Key     []byte // 可选：用于 Kafka partition routing
	Value   []byte // 必填
	Headers map[string]string
}
