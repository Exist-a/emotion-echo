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
	"fmt"
	"strings"
	"time"
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
func (p *DevEventPublisher) Publish(ctx context.Context, topic string, e *Event) error {
	if e == nil {
		return fmt.Errorf("dev_publisher: event is nil")
	}
	if e.ID == "" {
		return fmt.Errorf("dev_publisher: event.ID is empty (type=%s)", e.Type)
	}

	row, err := mapEventToUserBehaviorRow(e)
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

// userBehaviorRow 是 user_behavior_events 一行的内部表示
//
// 字段顺序 = SQL INSERT 参数顺序；不要随便调（调了要同步调单测的 joinArgs
// 断言语义，虽然单测用 Contains 不绑顺序，但保持稳定便于 reviewer 阅读）。
type userBehaviorRow struct {
	EventID    string
	UserID     int64
	EventType  string
	Target     string
	SessionID  string
	OccurredAt time.Time
}

// mapEventToUserBehaviorRow 把 Event 映射到 user_behavior_events 行
//
// 设计要点（per ADR-19 §D）：
//   - 3 种事件类型显式分支，未知类型返 error（不允许静默丢）
//   - target 字段填 message.id（message.created）或 conversation.id（其他两）
//   - session_id 填 conversation.id（让"同一会话的事件"聚合到一起）
//   - occurred_at 取 Event.Time（事件产生时间，非落库时间）
//
// PR-A1.3 会把这函数抽到 shared/pkg/eventrow.MapEventToUserBehaviorRow，
// 与 analytics-svc consumer 共用（消灭两份映射）。
func mapEventToUserBehaviorRow(e *Event) (userBehaviorRow, error) {
	switch d := e.Data.(type) {
	case MessageCreatedData:
		if d.MessageID == 0 {
			return userBehaviorRow{}, fmt.Errorf("dev_publisher: message.created event missing MessageID")
		}
		if d.ConversationID == 0 {
			return userBehaviorRow{}, fmt.Errorf("dev_publisher: message.created event missing ConversationID")
		}
		return userBehaviorRow{
			EventID:    e.ID,
			UserID:     d.UserID,
			EventType:  EventTypeMessageCreated,
			Target:     fmt.Sprintf("msg:%d", d.MessageID),
			SessionID:  fmt.Sprintf("conv:%d", d.ConversationID),
			OccurredAt: e.Time.UTC(),
		}, nil
	case ConversationCreatedData:
		if d.ConversationID == 0 {
			return userBehaviorRow{}, fmt.Errorf("dev_publisher: conversation.created event missing ConversationID")
		}
		return userBehaviorRow{
			EventID:    e.ID,
			UserID:     d.UserID,
			EventType:  EventTypeConversationCreated,
			Target:     fmt.Sprintf("conv:%d", d.ConversationID),
			SessionID:  fmt.Sprintf("conv:%d", d.ConversationID),
			OccurredAt: e.Time.UTC(),
		}, nil
	case ConversationClosedData:
		if d.ConversationID == 0 {
			return userBehaviorRow{}, fmt.Errorf("dev_publisher: conversation.closed event missing ConversationID")
		}
		return userBehaviorRow{
			EventID:    e.ID,
			UserID:     d.UserID,
			EventType:  EventTypeConversationClosed,
			Target:     fmt.Sprintf("conv:%d", d.ConversationID),
			SessionID:  fmt.Sprintf("conv:%d", d.ConversationID),
			OccurredAt: e.Time.UTC(),
		}, nil
	default:
		return userBehaviorRow{}, fmt.Errorf("dev_publisher: unknown event data type %T (event_type=%s)",
			d, e.Type)
	}
}

// 防 unused import 警告（strings 留给后续 PR-A1.3 refactor 时拼接 SQL 用）
var _ = strings.HasPrefix
