// Package eventrow 提供 chat-svc 业务事件 → user_behavior_events 行的共享映射
//
// ADR-19 PR-A1.3 REFACTOR: 把 chat-svc/internal/events.mapEventToUserBehaviorRow
// 抽到 shared 包，让 chat-svc DevEventPublisher 与 analytics-svc Kafka consumer
// 共用同一份映射函数,消灭两份 schema 漂移。
//
// 包级 API:
//   - UserBehaviorRow: user_behavior_events 表行的内部表示
//   - MapEventToUserBehaviorRow: 纯函数,从 EventID/EventType/Data 三件套映射到行
//
// 不依赖 chat-svc 的 events 包（避免 shared → chat-svc 反向依赖）：
//   - 调用方传入 eventID (string) / eventType (string) / data (any)
//   - data 由调用方先 marshal 成 []byte 再 unmarshal 到目标 DTO,或直接传目标 DTO
//
// EventType 命名差异（PR-A1.3 不修复,留作后续数据迁移 PR）:
//   - chat-svc DevEventPublisher: 用 ev.Type 原值（message.created / conversation.created / conversation.closed）
//   - analytics-svc consumer:     用 normalizeEventType 后（message / conversation_created / conversation_closed）
//   - 两路径最终落库的 event_type 字符串不同 → 同一事件从 dev vs prod 走会有不同落库值
//   - 修复路径（不在 PR-A1.3 范围）：选定一种命名 + 数据迁移脚本,后续 sprint 单独起
package eventrow

import (
	"errors"
	"fmt"
	"time"
)

// EventType 常量镜像 chat-svc/internal/events.EventType* 三个值
//
// 这里重复定义是因为 shared 不能 import chat-svc（避免反向依赖）。
// 任何 EventType 字符串变更必须同时改这两处；PR-A1.3 仅做"映射函数复用"，
// 不强制两路径 EventType 字符串统一（见包注释）。
const (
	EventTypeMessageCreated         = "message.created"
	EventTypeConversationCreated    = "conversation.created"
	EventTypeConversationClosed     = "conversation.closed"
)

// UserBehaviorRow 是 user_behavior_events 表的内部表示
//
// 字段顺序 = SQL INSERT 参数顺序的稳定锚点（PR-A1.2 chat-svc DevEventPublisher
// 用此顺序）；调用方在 INSERT 时按字段顺序展开。
type UserBehaviorRow struct {
	EventID    string
	UserID     int64
	EventType  string
	Target     string
	SessionID  string
	OccurredAt time.Time
}

// DataShape 是 MapEventToUserBehaviorRow 接受的 Data 形状
//
// 真实 chat-svc events.MessageCreatedData / events.ConversationCreatedData /
// events.ConversationClosedData 三个结构字段不同但语义对齐；调用方负责先把
// events.Data (any) 反序列化到这三个目标类型之一（用 json.Marshal+Unmarshal）
// 再传进来。这样 eventrow 不必 import chat-svc/internal/events。
type DataShape struct {
	MessageID      int64 // message.created 用
	ConversationID int64 // 3 类型都用,作为 session_id 锚
	UserID         int64 // 3 类型都用
}

// MapEventToUserBehaviorRow 把事件三件套映射到 user_behavior_events 行
//
// 参数:
//   - eventID: 事件唯一 ID（取自 chat-svc Event.ID,作幂等键）
//   - eventType: 事件类型字符串（chat-svc 用带点的;analytics-svc 用 normalize 后无点的;
//                库内部不 normalize,原样落库,留给调用方决定）
//   - data: 已经 unmarshal 到合适 DTO 的字段值（MessageID/ConversationID/UserID）
//   - occurredAt: 事件产生时间
//
// 返回:
//   - 成功 → UserBehaviorRow, nil
//   - 未知 eventType → zero UserBehaviorRow, ErrUnknownEventType
//   - 缺关键字段（ConversationID==0 对所有类型;MessageID==0 对 message.created）→
//     error with details
//
// 已知限制（PR-A1.3 范围内保留）:
//   - target 字段对 message.created 用 "msg:N",对 conversation.* 用 "conv:N"
//     analytics-svc 旧逻辑用纯数字字符串。PR-A1.3 暂保留 chat-svc 风格,
//     后续数据迁移 PR 决定统一形态。
//   - session_id 用 "conv:N" 前缀格式,与 chat-svc DevEventPublisher 对齐;
//     analytics-svc 旧逻辑用 topic 名（无前缀）。同样待后续 PR 统一。
func MapEventToUserBehaviorRow(eventID, eventType string, data DataShape, occurredAt time.Time) (UserBehaviorRow, error) {
	if eventID == "" {
		return UserBehaviorRow{}, errors.New("eventrow: eventID is empty")
	}
	if data.ConversationID == 0 {
		return UserBehaviorRow{}, fmt.Errorf("eventrow: ConversationID is 0 (eventType=%s, eventID=%s)",
			eventType, eventID)
	}

	occurredUTC := occurredAt.UTC()
	sessionID := fmt.Sprintf("conv:%d", data.ConversationID)

	switch eventType {
	case EventTypeMessageCreated:
		if data.MessageID == 0 {
			return UserBehaviorRow{}, fmt.Errorf("eventrow: message.created event missing MessageID (eventID=%s)", eventID)
		}
		return UserBehaviorRow{
			EventID:    eventID,
			UserID:     data.UserID,
			EventType:  eventType,
			Target:     fmt.Sprintf("msg:%d", data.MessageID),
			SessionID:  sessionID,
			OccurredAt: occurredUTC,
		}, nil
	case EventTypeConversationCreated:
		return UserBehaviorRow{
			EventID:    eventID,
			UserID:     data.UserID,
			EventType:  eventType,
			Target:     sessionID, // conv:N 同样作为 target
			SessionID:  sessionID,
			OccurredAt: occurredUTC,
		}, nil
	case EventTypeConversationClosed:
		return UserBehaviorRow{
			EventID:    eventID,
			UserID:     data.UserID,
			EventType:  eventType,
			Target:     sessionID,
			SessionID:  sessionID,
			OccurredAt: occurredUTC,
		}, nil
	default:
		return UserBehaviorRow{}, fmt.Errorf("%w: eventType=%q", ErrUnknownEventType, eventType)
	}
}

// ErrUnknownEventType 未知事件类型错误
//
// 调用方可用 errors.Is(err, ErrUnknownEventType) 识别。
var ErrUnknownEventType = errors.New("eventrow: unknown event type")
