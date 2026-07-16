package messaging

import "context"

// Producer 抽象消息发布接口。所有实现（in-memory / Kafka / RabbitMQ）都实现它。
type Producer interface {
	// Publish 把消息发到目标 topic。ctx 用于超时/取消控制。
	Publish(ctx context.Context, msg Message) error
	// Close 关闭底层连接；之后 Publish 必须返回 ErrProducerClosed
	Close() error
}
