// Package outbox — relay.go
//
// Stage 30-C A3: Outbox relay goroutine。
//
// 职责：
//   - 周期性扫描 outbox_events.pending 行
//   - 解析 payload（JSON 反序列化为 *events.Event）
//   - 调 EventPublisher.Publish 发到指定 topic
//   - 成功 → MarkSent（status=sent, sent_at=now）
//   - 失败 → MarkFailed（attempts++, last_error=<err>；status 保留 pending，下次再试）
//
// 启动：
//   - chat-svc main.go 在 Kafka.Enabled 时启动一个 goroutine
//   - ctx 取消时退出
//
// 重发幂等：
//   - relay 重发天然会重复（A3 残留 bug 或 DLQ 回放）
//   - 靠 A1 消费者侧 event_id UNIQUE 兜底（chat-svc 端 unique 兜底 → consumer 端二次 unique 兜底）
package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"time"

	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/repository"
)

// Relay 周期性发送 outbox pending 行
type Relay struct {
	repo      repository.OutboxRepo
	publisher events.EventPublisher
	interval  time.Duration
	batchSize int
}

// NewRelay 构造
func NewRelay(repo repository.OutboxRepo, publisher events.EventPublisher, interval time.Duration, batchSize int) *Relay {
	if interval <= 0 {
		interval = 1 * time.Second
	}
	if batchSize <= 0 {
		batchSize = 100
	}
	return &Relay{
		repo:      repo,
		publisher: publisher,
		interval:  interval,
		batchSize: batchSize,
	}
}

// Run 阻塞循环；ctx 取消时退出
//
// 注意：panic recovery + log；单个 entry 失败不影响其他 entry
func (r *Relay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	log.Printf("[outbox-relay] started: interval=%s batchSize=%d", r.interval, r.batchSize)
	for {
		select {
		case <-ctx.Done():
			log.Printf("[outbox-relay] stopped: %v", ctx.Err())
			return ctx.Err()
		case <-ticker.C:
			if err := r.FlushOnce(ctx); err != nil {
				log.Printf("[outbox-relay] flush err: %v", err)
			}
		}
	}
}

// FlushOnce 单轮拉取 + 发布 + 标记（测试与 main 都用）
func (r *Relay) FlushOnce(ctx context.Context) error {
	entries, err := r.repo.ListPending(ctx, r.batchSize)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	for _, e := range entries {
		if err := r.publishOne(ctx, e); err != nil {
			if mfErr := r.repo.MarkFailed(ctx, e.ID, err.Error()); mfErr != nil {
				log.Printf("[outbox-relay] MarkFailed err id=%d: %v", e.ID, mfErr)
			}
			log.Printf("[outbox-relay] publish failed id=%d attempts=%d: %v", e.ID, e.Attempts+1, err)
			continue
		}
		if err := r.repo.MarkSent(ctx, e.ID); err != nil {
			log.Printf("[outbox-relay] MarkSent err id=%d: %v", e.ID, err)
		}
	}
	return nil
}

// publishOne 反序列化 payload + 调 EventPublisher.Publish
func (r *Relay) publishOne(ctx context.Context, e repository.OutboxEvent) error {
	var evt events.Event
	if err := json.Unmarshal(e.Payload, &evt); err != nil {
		return err
	}
	if r.publisher == nil {
		return errors.New("outbox-relay: publisher is nil")
	}
	return r.publisher.Publish(ctx, e.Topic, &evt)
}
