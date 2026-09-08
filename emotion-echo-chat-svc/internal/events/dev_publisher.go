// Package events — dev_publisher.go
//
// ADR-19 PR-A1.2 GREEN: DevEventPublisher 实现 EventPublisher 接口。
//
// 目的：KAFKA_ENABLED=false 时（dev / CI 不想跑 Kafka 容器），
// 替代 nil publisher，把 chat-svc 业务事件同步写入
// emotion_echo_analytics.user_behavior_events 表。
//
// 与 KafkaEventPublisher 的差异：
//   - KafkaEventPublisher：发到 chat-events topic，由 analytics-svc consumer 落库
//   - DevEventPublisher：直接 INSERT user_behavior_events（dev-only，prod 不命中）
//
// 接口合规：实现 events.EventPublisher（与 KafkaEventPublisher 同接口），
// outbox relay 完全透明，调用方无感知下游是 Kafka 还是 PG。
//
// 同步语义（ADR-19 §C）：
//   - Publish 必须等 INSERT 落库后才返回（relay 据此 MarkSent）
//   - DB 错误 → 返 error（relay MarkFailed，下次重试）
//   - 同一 event_id 二次 Publish → ON CONFLICT DO NOTHING，返 nil（与 Stage 30-C A1
//     幂等去重契约对齐）
//
// 跨服务职责代价（ADR-19 §E）：
//   - chat-svc 知道 user_behavior_events 表结构
//   - 只在 KAFKA_ENABLED=false 启用，prod 不命中
//   - PR-A1.3 会把事件→行的映射抽到 emotion-echo-shared/internal/eventrow 包，
//     与 analytics-svc consumer 共用，消灭两份 schema 漂移
package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/emotion-echo/shared/pkg/eventrow"
)

// dbExecutor 是 DevEventPublisher 需要的最小 DB 接口
//
// 真实 *sql.DB 也实现此接口（database/sql.DB.ExecContext 签名一致）。
// 引入小接口的目的是让单测注入 fake 实现，避免引入 sqlmock 等第三方包。
type dbExecutor interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// DevEventPublisher 把 Event 同步写入 user_behavior_events 表
//
// 与 KafkaEventPublisher 不同的字段：
//   - db dbExecutor（真实 *sql.DB 或测试 fake）
//
// 字段全部私有，构造走 NewDevEventPublisher（避免外部直接 struct literal）。
type DevEventPublisher struct {
	db dbExecutor
}

// NewDevEventPublisher 构造 DevEventPublisher
//
// db 可以是 *sql.DB（生产）或 fakeDBExecutor（单测）。
// 暂不引入 Clock/IDGen（PR-A1.3 refactor 阶段需要再补；当前用 time.Now()
// 满足 PR-A1.2 GREEN，event_id 由 caller 决定）。
func NewDevEventPublisher(db dbExecutor) *DevEventPublisher {
	return &DevEventPublisher{db: db}
}

// Close 关闭 — DevEventPublisher 没有后台 goroutine 需 flush，Close 为 no-op
//
// 与 KafkaEventPublisher.Close（flush sarama producer 缓冲）不同。
// 多次 Close 安全（接口合规）。
func (p *DevEventPublisher) Close() error {
	return nil
}

// Publish 把 Event 写入 user_behavior_events 表
//
//   - Event 为 nil → 返 error（不让 relay 误以为成功）
//   - Event.Type 未知 → 返 error（不打日志表、不返默认 "unknown" 静默丢）
//   - DB 错误 → 透传（relay MarkFailed 重试）
//   - 成功 → 返 nil（relay MarkSent）
//
// event_id 取自 Event.ID（与 Kafka 路径一致 — Stage 30-C A1 幂等去重契约）。
// event_type 细分（message.created / conversation.created / conversation.closed），
// 不再合并成 "conversation"（Stage 37-A A3 修复）。
//
// PR-A1.3 REFACTOR：把 mapEventToUserBehaviorRow 抽到 shared/pkg/eventrow，
// 与 analytics-svc consumer 共用（消灭两份 schema 漂移）。
func (p *DevEventPublisher) Publish(ctx context.Context, topic string, e *Event) error {
	if e == nil {
		return fmt.Errorf("dev_publisher: event is nil")
	}
	if e.ID == "" {
		return fmt.Errorf("dev_publisher: event.ID is empty (type=%s)", e.Type)
	}

	row, err := buildUserBehaviorRow(e)
	if err != nil {
		return err
	}

	const insertSQL = `
INSERT INTO emotion_echo_analytics.user_behavior_events
  (event_id, user_id, event_type, target, session_id, occurred_at)
VALUES
  ($1, $2, $3, $4, $5, $6)
ON CONFLICT (event_id) DO NOTHING`

	if _, err := p.db.ExecContext(ctx, insertSQL,
		row.EventID,
		row.UserID,
		row.EventType,
		row.Target,
		row.SessionID,
		row.OccurredAt,
	); err != nil {
		return fmt.Errorf("dev_publisher: insert user_behavior_events failed (event_id=%s): %w",
			row.EventID, err)
	}
	return nil
}

// buildUserBehaviorRow 把 chat-svc Event 转换为 shared eventrow.UserBehaviorRow
//
// 这是 chat-svc 侧的薄包装：把 events.Data 反序列化成目标 DTO 字段值，
// 再调 eventrow.MapEventToUserBehaviorRow 完成映射。
//
// chat-svc 当前落库 event_type 用 ev.Type 原值（带点），与 analytics-svc
// consumer 用的 normalizeEventType 后值（不带点）不同 — 详见 eventrow 包注释。
// PR-A1.3 不改落库值（避免引入未知回归），等数据迁移 PR 决定统一形态。
func buildUserBehaviorRow(e *Event) (eventrow.UserBehaviorRow, error) {
	shape, err := extractDataShape(e)
	if err != nil {
		return eventrow.UserBehaviorRow{}, err
	}
	return eventrow.MapEventToUserBehaviorRow(e.ID, e.Type, shape, e.Time)
}

// extractDataShape 把 e.Data (any) 反序列化到 eventrow.DataShape
//
// e.Data 在 JSON 序列化时已经是 MessageCreatedData / ConversationCreatedData /
// ConversationClosedData 结构之一。先 marshal 再 unmarshal 到 eventrow.DataShape
// 与 analytics-svc consumer 的 remarshal 模式一致（consumer.go:285）。
func extractDataShape(e *Event) (eventrow.DataShape, error) {
	b, err := json.Marshal(e.Data)
	if err != nil {
		return eventrow.DataShape{}, fmt.Errorf("dev_publisher: marshal data: %w", err)
	}
	// 第一次 unmarshal 到 dynamic map 用于按字段名匹配
	var dyn map[string]any
	if err := json.Unmarshal(b, &dyn); err != nil {
		return eventrow.DataShape{}, fmt.Errorf("dev_publisher: unmarshal data to dyn: %w", err)
	}
	shape := eventrow.DataShape{}
	if v, ok := dyn["messageId"]; ok {
		if f, ok := v.(float64); ok {
			shape.MessageID = int64(f)
		}
	}
	if v, ok := dyn["conversationId"]; ok {
		if f, ok := v.(float64); ok {
			shape.ConversationID = int64(f)
		}
	}
	if v, ok := dyn["userId"]; ok {
		if f, ok := v.(float64); ok {
			shape.UserID = int64(f)
		}
	}
	return shape, nil
}
