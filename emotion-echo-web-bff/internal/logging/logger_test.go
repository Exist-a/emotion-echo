// Package logging — logger_test.go
//
// Stage 30 / stage-30-web-bff.md T1.4 RED:
// 断言 logging.Init 后 slog 输出 JSON 格式（LOG_FORMAT=json 默认）。
//
// 跑：go test ./internal/logging/...
package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

// TestInit_JSONFormat_OutputsJSON LOG_FORMAT=json（默认）→ slog 输出 JSON
func TestInit_JSONFormat_OutputsJSON(t *testing.T) {
	t.Setenv("LOG_FORMAT", "json")
	var buf bytes.Buffer
	InitTo(&buf)

	slog.Info("hello world", "module", "test")

	out := strings.TrimSpace(buf.String())
	if !strings.HasPrefix(out, "{") {
		t.Fatalf("LOG_FORMAT=json 应输出 JSON，got: %q", out)
	}
	if !strings.Contains(out, `"msg":"hello world"`) {
		t.Fatalf("JSON 应含 msg 字段，got: %q", out)
	}
	if !strings.Contains(out, `"module":"test"`) {
		t.Fatalf("JSON 应含附加字段 module，got: %q", out)
	}
}

// TestInit_TextFormat_OutputsText LOG_FORMAT=text → slog 输出文本（非 JSON）
func TestInit_TextFormat_OutputsText(t *testing.T) {
	t.Setenv("LOG_FORMAT", "text")
	var buf bytes.Buffer
	InitTo(&buf)

	slog.Info("plain text log")

	out := strings.TrimSpace(buf.String())
	if strings.HasPrefix(out, "{") {
		t.Fatalf("LOG_FORMAT=text 不应输出 JSON，got: %q", out)
	}
	if !strings.Contains(out, "plain text log") {
		t.Fatalf("text 输出应含 msg，got: %q", out)
	}
}

// TestPrintf_ModuleSplit 兼容 helper：[module] 前缀剥离到 module 字段
func TestPrintf_ModuleSplit(t *testing.T) {
	t.Setenv("LOG_FORMAT", "json")
	var buf bytes.Buffer
	InitTo(&buf)

	Printf("[postgres] connected to host=%s", "pg1")

	out := buf.String()
	if !strings.Contains(out, `"module":"postgres"`) {
		t.Fatalf("Printf 应把 [module] 前缀剥离到 module 字段，got: %q", out)
	}
	if !strings.Contains(out, "connected to host=pg1") {
		t.Fatalf("Printf 应输出剥离前缀后的 body，got: %q", out)
	}
}
