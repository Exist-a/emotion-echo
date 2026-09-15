package messaging

import "github.com/IBM/sarama"

// Sw8HeaderName 是 SkyWalking v3 sw8 跨进程 trace 透传 header 名常量。
//
// 写入侧（chat-svc internal/events/kafka_publisher.go sw8HeaderName = "sw8"）与
// 读取侧（ai-svc / analytics-svc consumer extractSw8Header）共用此常量。
//
// 收敛到 shared/pkg/messaging 是因为 D8-2（kafka-pipeline-pending-decisions.md
// 状态盘点 §D8-2 登记 "extractSw8Header 双份未收敛"）。
const Sw8HeaderName = "sw8"

// ExtractSw8Header 从 sarama RecordHeader 列表抽 sw8 header value。
//
// 返回值约定：
//   - 找到 sw8 header（含 nil-safe 跳过）→ 返回对应 string value
//   - 未找到 / 列表为空 / 所有 header 均为 nil → 返回 ""
//
// 与 ai-svc internal/consumer/consumer_failure.go:98-108 + analytics-svc
// internal/kafka/consumer.go:259-272 函数体完全等价。Round B 收敛到 shared 后
// 两个 svc 直接 import 此函数即可，零业务逻辑变化。
func ExtractSw8Header(headers []*sarama.RecordHeader) string {
	for _, h := range headers {
		if h == nil {
			continue
		}
		if string(h.Key) == Sw8HeaderName {
			return string(h.Value)
		}
	}
	return ""
}
