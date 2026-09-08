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
	"strings"
	"time"
)

// EventType 常量镜像 chat-svc/internal/events.EventType* 三个值
//
// 这里重复定义是因为 shared 不能 import chat-svc（避免反向依赖）。
// 任何 EventType 字符串变更必须同时改这两处；PR-A1.3 仅做"映射函数复用"，
// 不强制两路径 EventType 字符串统一（见包注释）。
//
// Sprint A 收口（PR-A1.3 v2）: 这些常量保留供外部 reference，
// 实际 mapper 用 classifyEventType 基于前缀匹配,允许两种命名风格混用。
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
//   - 未知 eventType（不属于 message/conversation 两大类）→ zero, ErrUnknownEventType
//   - 缺关键字段（ConversationID==0 对所有类型;message 类要求 MessageID>0）→ error
//
// eventType 分类（基于前缀匹配,兼容两种命名风格）:
//   - "message" / "message.created"        → message 类 → target="msg:N"
//   - "conversation_created" / "conversation.created" /
//     "conversation_closed"  / "conversation.closed"
//     → conversation 类 → target="conv:N"
//   - 其他 → ErrUnknownEventType
//
// eventType 字段值原样透传到 row.EventType（调用方决定落库 enum 字面值）。
//
// Sprint A 收口（PR-A1.3 v2）:
//   - target / session_id 格式统一 chat-svc 风格(msg:N/conv:N)
//   - session_id 全用 conv:N(同一会话聚合语义清晰)
func MapEventToUserBehaviorRow(eventID, eventType string, data DataShape, occurredAt time.Time) (UserBehaviorRow, error) {
	if eventID == "" {
		return UserBehaviorRow{}, errors.New("eventrow: eventID is empty")
	}

	// 先分类,unknown eventType 优先返 ErrUnknownEventType
	// (避免错误信息混乱:unknown + missing field 同时报)
	class := classifyEventType(eventType)
	if class == eventClassUnknown {
		return UserBehaviorRow{}, fmt.Errorf("%w: eventType=%q", ErrUnknownEventType, eventType)
	}

	if data.ConversationID == 0 {
		return UserBehaviorRow{}, fmt.Errorf("eventrow: ConversationID is 0 (eventType=%s, eventID=%s)",
			eventType, eventID)
	}

	occurredUTC := occurredAt.UTC()
	sessionID := fmt.Sprintf("conv:%d", data.ConversationID)

	switch class {
	case eventClassMessage:
		if data.MessageID == 0 {
			return UserBehaviorRow{}, fmt.Errorf("eventrow: message event missing MessageID (eventID=%s, eventType=%s)",
				eventID, eventType)
		}
		return UserBehaviorRow{
			EventID:    eventID,
			UserID:     data.UserID,
			EventType:  eventType,
			Target:     fmt.Sprintf("msg:%d", data.MessageID),
			SessionID:  sessionID,
			OccurredAt: occurredUTC,
		}, nil
	case eventClassConversation:
		return UserBehaviorRow{
			EventID:    eventID,
			UserID:     data.UserID,
			EventType:  eventType,
			Target:     sessionID, // conv:N 同样作为 target
			SessionID:  sessionID,
			OccurredAt: occurredUTC,
		}, nil
	default:
		return UserBehaviorRow{}, fmt.Errorf("%w: eventType=%q", ErrUnknownEventType, eventType)
	}
}

// eventClass 内部枚举,用于按 message/conversation 大类决定 target 格式
type eventClass int

const (
	eventClassUnknown eventClass = iota
	eventClassMessage
	eventClassConversation
)

// classifyEventType 把 eventType 字符串分类
//
// 兼容两种命名风格:
//   - chat-svc 原值:        "message.created" / "conversation.created" / "conversation.closed"
//   - analytics-svc normalize: "message" / "conversation_created" / "conversation_closed"
//   - 历史 Stage 30-C A3 之前: "conversation" (已被 Stage 37-A 拆掉,这里 reject)
//
// 匹配规则:前缀 "message" 或 "conversation" (含 . / _ 边界)。
func classifyEventType(eventType string) eventClass {
	switch {
	case eventType == "":
		return eventClassUnknown
	case eventType == "message" || strings.HasPrefix(eventType, "message."):
		return eventClassMessage
	case eventType == "message_created",
		eventType == "conversation.created" || strings.HasPrefix(eventType, "conversation."),
		eventType == "conversation_created", eventType == "conversation_closed":
		return eventClassConversation
	default:
		return eventClassUnknown
	}
}

// ErrUnknownEventType 未知事件类型错误
//
// 调用方可用 errors.Is(err, ErrUnknownEventType) 识别。
var ErrUnknownEventType = errors.New("eventrow: unknown event type")
