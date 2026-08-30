// Package sse — encoder_test.go
//
// Stage 30 / stage-30-web-bff.md T3.25 RED: SSE encoder golden output
//
// 断言 encoder 输出与 testdata/sse_expected.txt 逐字节一致（event/data/id/retry 字段格式）。
package sse

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEncode_GoldenOutput 两条事件（analysis + done）输出与 golden 文件一致
func TestEncode_GoldenOutput(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, Encode(&buf, Event{
		Name: "analysis",
		ID:   "evt-1",
		Data: map[string]any{"emotion": "happy", "score": 0.7},
	}))
	require.NoError(t, Encode(&buf, Event{
		Name: "done",
		Data: map[string]any{"status": "ok"},
	}))

	golden, err := os.ReadFile(filepath.Join("..", "..", "testdata", "sse_expected.txt"))
	require.NoError(t, err)

	assert.Equal(t, string(golden), buf.String(), "encoder 输出应与 golden 文件逐字节一致")
}

// TestEncode_RetryLine retry 字段格式
func TestEncode_RetryLine(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, Encode(&buf, Event{
		Name:  "ping",
		Data:  map[string]any{"ok": true},
		Retry: 3000,
	}))

	out := buf.String()
	assert.Contains(t, out, "retry: 3000\n", "retry 字段应输出")
	assert.Contains(t, out, "event: ping\n")
	assert.Contains(t, out, "data: {\"ok\":true}\n")
	assert.True(t, strings.HasSuffix(out, "\n\n"), "事件应以空行结束")
}

// TestEncode_EmptyName_DefaultMessage 空 name → 无 event: 行（默认 message 事件）
func TestEncode_EmptyName_DefaultMessage(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, Encode(&buf, Event{Data: "hi"}))

	out := buf.String()
	assert.NotContains(t, out, "event:", "空 name 不应输出 event 行")
	assert.Contains(t, out, "data: \"hi\"\n")
}

// TestEncode_NoData_StillTerminates 无 data 仍输出空行结束
func TestEncode_NoData_StillTerminates(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, Encode(&buf, Event{Name: "keepalive"}))

	assert.Equal(t, "event: keepalive\n\n", buf.String())
}

// TestEncodeRaw_RawLine 原始文本写入（已序列化 data）
func TestEncodeRaw_RawLine(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, EncodeRaw(&buf, "done", "", `{"status":"ok"}`))

	assert.Equal(t, "event: done\ndata: {\"status\":\"ok\"}\n\n", buf.String())
}
