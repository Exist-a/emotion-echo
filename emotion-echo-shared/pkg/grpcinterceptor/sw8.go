package grpcinterceptor

import (
	"encoding/base64"
	"strings"
)

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
//
// E2E-26 #17 / F-185：go2sky 的 EncodeSW8 对 traceId 做了 base64 ——
// 真实 Kafka header 实抓（2026-10-03）第2段形如
// NWNhYWIwM2RiNjIwMTFmMWIxMjczZTVlMWQ5OTkxNzE=（明文 59cab03d...）。
// 不解码则消费侧日志 trace_id 是 b64 串，与 OAP traceId（明文 hex）永远
// join 不上（F-146 半修的根因：单测按明文假设编写，与编码端脱节）。
// 解码后须为十六进制形态才采纳；否则回退原文（兼容历史明文形态）。
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
	if decoded, err := base64.StdEncoding.DecodeString(parts[1]); err == nil {
		if s := string(decoded); isHexTraceID(s) {
			return s
		}
	}
	return parts[1]
}

// isHexTraceID 判定 base64 解码结果是否为 traceId 形态（≥16 位十六进制）。
// 用于区分"真 b64 编码的 traceId"与"历史明文恰好能被 base64 解出的乱字节"。
func isHexTraceID(s string) bool {
	if len(s) < 16 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}
