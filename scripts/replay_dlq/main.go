// replay_dlq —— chat-events-dlq 死信回放工具（E2E-24 / 决策 D-33）
//
// 背景（账本 E2E-F-12 / F-150）：chat-events-dlq 只进不出，死信堆积 25+ 条且
// 无任何处置机制；告警注释里的"回放"是手工 psql UPDATE。本工具把死信读出、
// 剥掉 DLQ 诊断 headers（保留 sw8 以延续 trace）、按 x-original-topic 重发布回
// 原 topic，交给已修复的 consumer（F-149/F-174）正常消费或按契约进 DLQ/落库。
//
// 形态说明（D-33）：工具用 Go + 仓库既有 sarama 依赖、以 `go run` 方式运行，
// 而非 bash + kafka console 文本管道 —— **DLQ payload 是 Protobuf 二进制**
// （Stage 73 起），console consumer/producer 的文本处理会损坏字节流。
// 工具本身零服务代码改动，单测覆盖转换逻辑（main_test.go）。
//
// 用法：
//
//	go run ./replay_dlq -brokers localhost:9092 -limit 0          # 全量 dry-run 预览
//	go run ./replay_dlq -limit 0                                  # 全量回放（默认 -to chat-events）
//	go run ./replay_dlq -limit 5                                  # 只回放前 5 条
//
// 幂等性：回放消息被 consumer 重新处理时受 ON CONFLICT (event_id, occurred_at)
// DO NOTHING 兜底（F-149 修复），重复回放不产生重复行。
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/IBM/sarama"
)

type replayEntry struct {
	key    []byte
	value  []byte
	target string // x-original-topic 优先，否则回退 -to
	headers []sarama.RecordHeader
}

// transformDLQEntry 从一条 DLQ 消息构造回放消息：
//   - 目标 topic = header[x-original-topic]，缺失时回退 fallbackTopic
//   - 剥掉 DLQ 诊断 headers（x-original-topic / x-error-reason / x-attempts）
//   - 保留其余 headers（sw8 等，trace 连续性）
func transformDLQEntry(msg *sarama.ConsumerMessage, fallbackTopic string) (replayEntry, error) {
	e := replayEntry{key: msg.Key, value: msg.Value, target: fallbackTopic}
	for _, h := range msg.Headers {
		switch string(h.Key) {
		case "x-original-topic":
			if len(h.Value) > 0 {
				e.target = string(h.Value)
			}
		case "x-error-reason", "x-attempts":
			continue // DLQ 诊断信息，不回传业务 topic
		default:
			e.headers = append(e.headers, *h)
		}
	}
	if e.target == "" {
		return e, fmt.Errorf("target topic 为空（无 x-original-topic 且未给 -to）")
	}
	return e, nil
}

func main() {
	brokers := flag.String("brokers", "localhost:9092", "Kafka brokers（逗号分隔）")
	dlqTopic := flag.String("dlq", "chat-events-dlq", "DLQ topic")
	fallbackTopic := flag.String("to", "chat-events", "x-original-topic 缺失时的回放目标")
	limit := flag.Int("limit", 0, "最多回放条数，0 = 全部")
	dryRun := flag.Bool("dry-run", false, "只打印回放计划，不实际发布")
	timeout := flag.Int("timeout-sec", 30, "读取 DLQ 的空闲超时（秒）")
	flag.Parse()

	brokerList := splitBrokers(*brokers)

	// ---- 1. 读 DLQ（sarama partition consumer，OffsetOldest → 最新，无消费组）----
	consumer, err := sarama.NewConsumer(brokerList, saramaConfig())
	if err != nil {
		fatal("连接 Kafka 失败: %v", err)
	}
	defer consumer.Close()

	partitions, err := consumer.Partitions(*dlqTopic)
	if err != nil {
		fatal("取 DLQ 分区失败: %v", err)
	}

	var entries []*replayEntry
	for _, p := range partitions {
		pc, err := consumer.ConsumePartition(*dlqTopic, p, sarama.OffsetOldest)
		if err != nil {
			fatal("ConsumePartition %s-%d 失败: %v", *dlqTopic, p, err)
		}
	idle:
		for {
			select {
			case msg, ok := <-pc.Messages():
				if !ok {
					break idle
				}
				e, tErr := transformDLQEntry(msg, *fallbackTopic)
				if tErr != nil {
					fmt.Fprintf(os.Stderr, "[replay] 跳过不可回放消息 (offset=%d): %v\n", msg.Offset, tErr)
					continue
				}
				entries = append(entries, &e)
				if *limit > 0 && len(entries) >= *limit {
					break idle
				}
			case <-time.After(time.Duration(*timeout) * time.Second):
				break idle // 空闲超时 = 已读到末尾
			}
		}
		pc.Close()
	}

	fmt.Printf("[replay] DLQ %s 共读到 %d 条可回放消息\n", *dlqTopic, len(entries))

	// ---- 2. dry-run 预览 ----
	if *dryRun {
		for i, e := range entries {
			fmt.Printf("  #%d → topic=%s key=%q bytes=%d headers=%d\n",
				i+1, e.target, string(e.key), len(e.value), len(e.headers))
		}
		fmt.Println("[replay] dry-run：未实际发布")
		return
	}

	// ---- 3. 回放 ----
	producer, err := sarama.NewSyncProducer(brokerList, saramaConfig())
	if err != nil {
		fatal("创建 producer 失败: %v", err)
	}
	defer producer.Close()

	ok, fail := 0, 0
	for i, e := range entries {
		msg := &sarama.ProducerMessage{
			Topic:   e.target,
			Key:     sarama.ByteEncoder(e.key),
			Value:   sarama.ByteEncoder(e.value),
			Headers: append([]sarama.RecordHeader(nil), e.headers...),
		}
		if _, _, pErr := producer.SendMessage(msg); pErr != nil {
			fail++
			fmt.Fprintf(os.Stderr, "[replay] #%d 发布失败 → %s: %v\n", i+1, e.target, pErr)
			continue
		}
		ok++
	}
	fmt.Printf("[replay] 完成：成功 %d / 失败 %d（目标=%s）。死信在 DLQ topic 中保留为历史（offset 累计），处置账面见 report\n", ok, fail, *fallbackTopic)
}

func saramaConfig() *sarama.Config {
	cfg := sarama.NewConfig()
	cfg.Version = sarama.V2_8_0_0
	cfg.Consumer.Return.Errors = true
	cfg.Producer.RequiredAcks = sarama.WaitForAll
	cfg.Producer.Retry.Max = 3
	cfg.Producer.Return.Successes = true
	return cfg
}

func splitBrokers(s string) []string {
	var out []string
	cur := ""
	for _, r := range s {
		if r == ',' {
			if cur != "" {
				out = append(out, cur)
			}
			cur = ""
			continue
		}
		cur += string(r)
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[replay] FATAL: "+format+"\n", args...)
	os.Exit(1)
}
