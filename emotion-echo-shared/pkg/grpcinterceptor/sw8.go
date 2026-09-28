package grpcinterceptor

import "strings"

// SW8HeaderName SkyWalking 跨进程传播头名（gRPC 走 metadata，Kafka 走 record header）。
const SW8HeaderName = "sw8"

// TraceIDFromSW8 从 sw8 头里解出 trace_id，供**异步链路**（Kafka 消费侧）把
// trace 关联回业务日志。
//
// 为什么需要（E2E-F-146）：Kafka 消息没有 ctx 可言，consumer 里的 handler
// 只收 *sarama.ConsumerMessage。若不解析消息头，消费者侧日志就与生产侧链路
// 彻底断开——而异步恰恰是最需要 trace 的场景（"这条消息是谁发的、后来怎么了"）。
//
// sw8 是 8 段短横线分隔，trace_id 在**第 2 段**：
//
//	sample-traceId-segmentId-spanId-parentSpanId-parentService-parentServiceInstance-parentEndpoint-address
//
// 段数不足或采样标志非法时返回 ""（调用方据此不写 trace_id 字段，
// 绝不返回半截字符串——半截 ID 查不到任何东西，比没有更糟）。
func TraceIDFromSW8(sw8 string) string {
	if sw8 == "" {
		return ""
	}
	parts := strings.Split(sw8, "-")
	if len(parts) < 8 {
		return ""
	}
	if parts[0] != "0" && parts[0] != "1" {
		return ""
	}
	if parts[1] == "" {
		return ""
	}
	return parts[1]
}
