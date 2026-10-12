// Package model — intent_contract5_test.go
//
// AGENTS.md §2.4 §契约 5（schema 与写入端一致性）：DDL ↔ GORM model ↔ 写入端白名单
// 三方一致。本文件钉住其中「DDL ↔ model」这一腿（列宽）。
package model

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMessage_IntentGormTagSize_MatchesDDLWiden
//
// migration c007_widen_intent_to_32.sql 把 messages.intent 从 VARCHAR(16) 扩到
// VARCHAR(32)（Stage 89 PR-6 由 e2e 揪出），但 model tag 长期停在 `size:16` ——
// 这正是「DDL 改了、model 没跟」的漂移（§契约 5 三方一致里的 DDL↔model 一腿）。
// 本测试钉住 tag 宽度 == DDL 宽度，防再次分叉。
func TestMessage_IntentGormTagSize_MatchesDDLWiden(t *testing.T) {
	f, ok := reflect.TypeOf(Message{}).FieldByName("Intent")
	require.True(t, ok, "Message 必须有 Intent 字段")

	tag := f.Tag.Get("gorm")
	assert.Contains(t, tag, "size:32",
		"messages.intent 的 DDL 为 VARCHAR(32)（migrations/c007_widen_intent_to_32.sql），"+
			"GORM tag 必须同步为 size:32；实际 tag=%q", tag)
}
