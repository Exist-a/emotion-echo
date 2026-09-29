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
	"errors"

	"github.com/emotion-echo/shared/pkg/logging"
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
	// MaxAttempts ADR-19 PR-A6.1: 失败最大重试次数，超出则 status=dead。
	// 默认 100；0 视作关闭 dead 状态机（保留原行为,向后兼容）。
	//
	// E2E-23 E 组：设了 Ops 时**以 Ops 的当前值为准**（可热更），
	// 未设 Ops 才回落到本字段 —— 见 currentMaxAttempts()。
	MaxAttempts int
	// Ops 运营参数容器（Nacos 推送写入）。nil 时行为与热更前完全一致。
	Ops *Ops
}

// currentMaxAttempts 返回本轮 flush 应当使用的重试上限。
//
// 每次判定都重新读 Ops 快照：热更后**下一轮**立即生效，无需重启 relay。
func (r *Relay) currentMaxAttempts() int {
	if r.Ops != nil {
		if v := r.Ops.Snapshot().MaxAttempts; v > 0 {
			return v
		}
	}
	return r.MaxAttempts
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
		repo:        repo,
		publisher:   publisher,
		interval:    interval,
		batchSize:   batchSize,
		MaxAttempts: 100,
	}
}

// Run 阻塞循环；ctx 取消时退出
//
// 注意：panic recovery + log；单个 entry 失败不影响其他 entry
func (r *Relay) Run(ctx context.Context) error {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	logging.PrintfContext(ctx, "[outbox-relay] started: interval=%s batchSize=%d", r.interval, r.batchSize)
	for {
		select {
		case <-ctx.Done():
			logging.PrintfContext(ctx, "[outbox-relay] stopped: %v", ctx.Err())
			return ctx.Err()
		case <-ticker.C:
			if err := r.FlushOnce(ctx); err != nil {
				logging.PrintfContext(ctx, "[outbox-relay] flush err: %v", err)
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
			// ADR-19 PR-A6.1: 失败先 MarkFailed（attempts+1），
			// 再判断 attempts 是否超阈值 → MarkDead 把行置为 dead 状态
			// 不再被 ListPending 扫描,避免永久无限重试毒消息。
			if mfErr := r.repo.MarkFailed(ctx, e.ID, err.Error()); mfErr != nil {
				logging.PrintfContext(ctx, "[outbox-relay] MarkFailed err id=%d: %v", e.ID, mfErr)
			}
			newAttempts := e.Attempts + 1
			logging.PrintfContext(ctx, "[outbox-relay] publish failed id=%d attempts=%d: %v", e.ID, newAttempts, err)
			maxAttempts := r.currentMaxAttempts()
			if maxAttempts > 0 && newAttempts >= maxAttempts {
				if mdErr := r.repo.MarkDead(ctx, e.ID, err.Error()); mdErr != nil {
					logging.PrintfContext(ctx, "[outbox-relay] MarkDead err id=%d: %v", e.ID, mdErr)
				} else {
					// Stage 86（§3.6）: dead 行计数器，Prometheus 规则 outbox-dead.yml 据此告警
					IncDead()
					logging.PrintfContext(ctx, "[outbox-relay] row marked dead id=%d attempts=%d max=%d (will NOT retry)",
						e.ID, newAttempts, maxAttempts)
				}
			}
			continue
		}
		if err := r.repo.MarkSent(ctx, e.ID); err != nil {
			logging.PrintfContext(ctx, "[outbox-relay] MarkSent err id=%d: %v", e.ID, err)
		}
	}
	return nil
}

// publishOne 反序列化 payload + 调 EventPublisher.Publish
//
// Stage 73：必须用 UnmarshalChatEventJSON（按 Type 反序列化 data 到具体 struct）。
// 直接 json.Unmarshal 会让 Data 落成 map[string]interface{}，
// MarshalChatEvent 拒绝 map → outbox 行重试 100 次后 dead（docker e2e 实测）。
func (r *Relay) publishOne(ctx context.Context, e repository.OutboxEvent) error {
	evt, err := events.UnmarshalChatEventJSON(e.Payload)
	if err != nil {
		return err
	}
	if r.publisher == nil {
		return errors.New("outbox-relay: publisher is nil")
	}
	return r.publisher.Publish(ctx, e.Topic, evt)
}
