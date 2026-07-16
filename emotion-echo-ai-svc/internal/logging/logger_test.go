// Package logging 测试（Stage 20-4）
//
// 覆盖：
//  1. JSON 格式输出（一行一个合法 JSON）
//  2. text 格式输出（可读文本）
//  3. module 字段自动从 "[xxx] msg" 提取
//  4. Errorf 把 err 并到 "err" 字段
package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestJsonFormat_ValidJSON(t *testing.T) {
	var buf bytes.Buffer
	InitTo(&buf)
	// 强制 level 重新设置（InitTo 内部用环境变量）
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	Printf("[postgres] connected")
	Warnf("[kafka] retries exhausted")
	Errorf(ioErr("connection refused"), "[llm] gRPC dial failed")

	lines := splitNonEmptyLines(buf.String())
	if len(lines) != 3 {
		t.Fatalf("expected 3 log lines, got %d: %s", len(lines), buf.String())
	}
	for i, ln := range lines {
		var obj map[string]any
		if err := json.Unmarshal([]byte(ln), &obj); err != nil {
			t.Fatalf("line %d not valid JSON: %v\nline: %s", i, err, ln)
		}
		for _, k := range []string{"time", "level", "msg"} {
			if _, ok := obj[k]; !ok {
				t.Errorf("line %d missing field %q: %s", i, k, ln)
			}
		}
	}
}

func TestJsonFormat_ModuleFieldExtracted(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	Printf("[postgres] connected")

	ln := strings.TrimSpace(buf.String())
	var obj map[string]any
	if err := json.Unmarshal([]byte(ln), &obj); err != nil {
		t.Fatalf("not valid JSON: %v\nline: %s", err, ln)
	}
	if obj["module"] != "postgres" {
		t.Errorf("expected module=postgres, got %v", obj["module"])
	}
	if obj["msg"] != "connected" {
		t.Errorf("expected msg=connected (no prefix), got %v", obj["msg"])
	}
}

func TestJsonFormat_ErrorfPutsErrInField(t *testing.T) {
	var buf bytes.Buffer
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo})))

	Errorf(ioErr("dial tcp: connection refused"), "[llm] gRPC dial failed")

	ln := strings.TrimSpace(buf.String())
	var obj map[string]any
	if err := json.Unmarshal([]byte(ln), &obj); err != nil {
		t.Fatalf("not valid JSON: %v\nline: %s", err, ln)
	}
	if obj["level"] != "ERROR" {
		t.Errorf("expected level=ERROR, got %v", obj["level"])
	}
	if obj["module"] != "llm" {
		t.Errorf("expected module=llm, got %v", obj["module"])
	}
	if obj["err"] != "dial tcp: connection refused" {
		t.Errorf("expected err field, got %v", obj["err"])
	}
}

func TestSplitModule(t *testing.T) {
	cases := []struct {
		in        string
		wantMod   string
		wantBody  string
	}{
		{"[postgres] connected", "postgres", "connected"},
		{"[grpc] ai-svc gRPC server listening on :8892", "grpc", "ai-svc gRPC server listening on :8892"},
		{"[module with extra space]   body", "module with extra space", "body"},
		{"no prefix here", "", "no prefix here"},
		{"[", "", "["}, // 没闭合
		{"", "", ""},
	}
	for _, c := range cases {
		mod, body := splitModule(c.in)
		if mod != c.wantMod || body != c.wantBody {
			t.Errorf("splitModule(%q) = (%q, %q), want (%q, %q)",
				c.in, mod, body, c.wantMod, c.wantBody)
		}
	}
}

// ============ helpers ============

func splitNonEmptyLines(s string) []string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		if strings.TrimSpace(ln) != "" {
			out = append(out, ln)
		}
	}
	return out
}

// ioErr 简单构造 error
type simpleErr string

func (e simpleErr) Error() string { return string(e) }

func ioErr(s string) error { return simpleErr(s) }
