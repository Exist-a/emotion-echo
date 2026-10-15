// internal/repository/outbox_trace_sw8_test.go
//
// CL-03 Phase 1：outbox_events.trace_sw8 的 model 映射契约（ADR 决策 22 checklist 第 3 步
// 「更新 chat-svc model struct + 加 unit test 验证映射」）。
//
// 为什么需要这一列（CL-03 实测）：relay 跑在后台根 ctx 上（main.go relayCtx）⇒
// kafka_publisher.Publish 的 CreateExitSpan 建的是「根」span，异步链路在 outbox→relay
// 处断掉。写入 outbox 行时身处请求 ctx（有入口 span）⇒ 把当前 sw8 存进列，
// relay 发布前用它恢复父 ctx，trace 才能连成一条。
package repository

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestOutboxEvent_HasTraceSw8Field_WithColumnTag 契约：model 有 trace_sw8 且列名显式。
//
// 显式 gorm column tag 是刻意的：GORM 的命名策略对 "TraceSW8" 这类
// 「大写缩写结尾」字段的推导不可靠，列名写死避免迁移列名与 ORM 列名不一致。
func TestOutboxEvent_HasTraceSw8Field_WithColumnTag(t *testing.T) {
	ft, ok := reflect.TypeOf(OutboxEvent{}).FieldByName("TraceSW8")
	require.True(t, ok, "OutboxEvent 缺 TraceSW8 字段 —— CL-03 Phase 1 要求它能承载写入时的 sw8")

	assert.Equal(t, reflect.String, ft.Type.Kind(),
		"TraceSW8 应为 string（sw8 是 base64 文本；空 = 无 trace 上下文）")
	assert.Contains(t, ft.Tag.Get("gorm"), "column:trace_sw8",
		"TraceSW8 必须显式声明 gorm:\"column:trace_sw8\"，与迁移列名严格一致")
}

// TestOutboxEvent_TraceSw8_RoundTripInMemory 内存实现须原样保留该列。
//
// 若 InMemory 实现丢了这一列，任何走内存 repo 的单测都验不出 trace 往返，
// 会掩盖"写入侧没存下来"这类缺陷。
func TestOutboxEvent_TraceSw8_RoundTripInMemory(t *testing.T) {
	repo := NewInMemoryOutboxRepo()
	const sw8 = "1-abc-1-2-0-chat-svc-inst-endpoint-1.2.3.4:80"

	err := repo.CreateInTx(nil, &OutboxEvent{
		EventID:   "evt-1",
		EventType: "message.created",
		Topic:     "chat-events",
		Payload:   []byte(`{}`),
		TraceSW8:  sw8,
	})
	require.NoError(t, err)

	got, err := repo.Get(1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, sw8, got.TraceSW8, "InMemory repo 必须原样保留 trace_sw8")
}

// TestOutboxEvent_TraceSw8_EmptyIsAllowed 空值是合法状态（历史行 / tracer=nil）。
func TestOutboxEvent_TraceSw8_EmptyIsAllowed(t *testing.T) {
	repo := NewInMemoryOutboxRepo()
	require.NoError(t, repo.CreateInTx(nil, &OutboxEvent{
		EventID: "evt-2", EventType: "message.created", Topic: "chat-events", Payload: []byte(`{}`),
	}))

	got, err := repo.Get(1)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Empty(t, got.TraceSW8, "未设置 sw8 时必须为空字符串（不得报错、不得造默认值）")
}
