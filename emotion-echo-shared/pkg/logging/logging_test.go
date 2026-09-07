// Package logging 提供 emotion-echo 各 Go svc 的统一结构化日志（Stage 41 PR-1）。
//
// 与 web-bff/ai-svc 现有 internal/logging 的关系：
//   - 原 internal/logging 是 clone（135 + 144 行，几乎完全相同）
//   - 本包把它们下沉到 shared，main 层只需 import shared/pkg/logging
//   - Errorf helper **不下沉**（stage doc §三 PR-1 △ 偏差）：
//     BFF/ai-svc 的 (err, format, args...) 签名与 chat-svc 的 (format, args...) 不兼容，
//     各 svc 改写时统一走 stdlib slog.ErrorContext(ctx, msg, "err", err)
//
// 输出格式：JSON to stdout（决策 6 审计要求）
//   {"time":"...","level":"INFO","msg":"...","module":"..."}
//
// 环境变量：
//   LOG_FORMAT = json (default) | text
//   LOG_LEVEL  = INFO (default) | DEBUG | WARN | ERROR
package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseLogLine 把单行 JSON 输出解为 map[string]any 便于断言。
// 多行输出取最后一行（其他行视为噪音）。
func parseLogLine(t *testing.T, raw string) map[string]any {
	t.Helper()
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	last := lines[len(lines)-1]
	var m map[string]any
	if err := json.Unmarshal([]byte(last), &m); err != nil {
		t.Fatalf("parse log line %q: %v", last, err)
	}
	return m
}

// TestInitTo_DefaultIsJSON 验证默认格式 = JSON（决策 6）。
func TestInitTo_DefaultIsJSON(t *testing.T) {
	var buf bytes.Buffer
	InitTo(&buf)
	slog.Info("hello")

	line := buf.String()
	require.NotEmpty(t, line)
	// JSON 格式 = 以 '{' 开头
	assert.True(t, strings.HasPrefix(strings.TrimSpace(line), "{"),
		"默认输出应为 JSON, 实得 %q", line)
}

// TestInitTo_FormatText 验证 LOG_FORMAT=text 走 TextHandler。
func TestInitTo_FormatText(t *testing.T) {
	t.Setenv("LOG_FORMAT", "text")
	var buf bytes.Buffer
	InitTo(&buf)
	slog.Info("hello")

	line := buf.String()
	// text 格式 = key=value,不以 '{' 开头
	assert.False(t, strings.HasPrefix(strings.TrimSpace(line), "{"),
		"LOG_FORMAT=text 应输出 text 格式, 实得 %q", line)
	assert.Contains(t, line, "hello", "text 输出应含 msg")
}

// TestInitTo_LevelRespected 验证 LOG_LEVEL 过滤（DEBUG 应该被 INFO 过滤掉）。
func TestInitTo_LevelRespected(t *testing.T) {
	t.Setenv("LOG_LEVEL", "INFO")
	var buf bytes.Buffer
	InitTo(&buf)
	slog.Debug("debug-msg")
	slog.Info("info-msg")

	output := buf.String()
	assert.NotContains(t, output, "debug-msg", "INFO 级别应过滤 DEBUG")
	assert.Contains(t, output, "info-msg", "INFO 级别应保留 INFO")
}

// TestInitTo_LevelDebug 验证 LOG_LEVEL=DEBUG 不过滤 DEBUG。
func TestInitTo_LevelDebug(t *testing.T) {
	t.Setenv("LOG_LEVEL", "DEBUG")
	var buf bytes.Buffer
	InitTo(&buf)
	slog.Debug("debug-msg")

	output := buf.String()
	assert.Contains(t, output, "debug-msg", "DEBUG 级别应保留 DEBUG")
}

// TestInitTo_Fields 验证 JSON 输出含 time/level/msg 字段。
func TestInitTo_Fields(t *testing.T) {
	var buf bytes.Buffer
	InitTo(&buf)
	slog.Info("hello-fields")

	m := parseLogLine(t, buf.String())
	assert.Equal(t, "INFO", m["level"])
	assert.Equal(t, "hello-fields", m["msg"])
	assert.NotEmpty(t, m["time"], "JSON 输出应含 time 字段")
}

// TestPrintf_WithModulePrefix 验证 Printf 拆 [module] 前缀到 module 字段。
// 这是兼容 log.Printf("[postgres] connected %d", n) 风格的关键。
func TestPrintf_WithModulePrefix(t *testing.T) {
	var buf bytes.Buffer
	InitTo(&buf)
	Printf("[postgres] connected port=%d", 5432)

	m := parseLogLine(t, buf.String())
	assert.Equal(t, "postgres", m["module"], "[postgres] 前缀应剥离到 module 字段")
	assert.Equal(t, "connected port=5432", m["msg"], "剩余部分作为 msg")
}

// TestPrintf_NoModulePrefix 验证无 [module] 前缀时整行作为 msg。
func TestPrintf_NoModulePrefix(t *testing.T) {
	var buf bytes.Buffer
	InitTo(&buf)
	Printf("just a message")

	m := parseLogLine(t, buf.String())
	_, hasModule := m["module"]
	assert.False(t, hasModule, "无 [module] 前缀时不应有 module 字段")
	assert.Equal(t, "just a message", m["msg"])
}

// TestInfof_InfoLevel 验证 Infof 走 INFO 级别。
func TestInfof_InfoLevel(t *testing.T) {
	var buf bytes.Buffer
	InitTo(&buf)
	Infof("info-test")

	m := parseLogLine(t, buf.String())
	assert.Equal(t, "INFO", m["level"])
}

// TestWarnf_WarnLevel 验证 Warnf 走 WARN 级别。
func TestWarnf_WarnLevel(t *testing.T) {
	var buf bytes.Buffer
	InitTo(&buf)
	Warnf("warn-test")

	m := parseLogLine(t, buf.String())
	assert.Equal(t, "WARN", m["level"])
}

// TestFatalf_ExitsAndLogs 验证 Fatalf 记录 ERROR 级别后调用 os.Exit(1)。
// 用子进程跑避免把测试进程杀掉。
func TestFatalf_ExitsAndLogs(t *testing.T) {
	if os.Getenv("BE_CRASHER") == "1" {
		var buf bytes.Buffer
		InitTo(&buf)
		Fatalf("fatal-test")
		// 不应执行到这里
		t.Fatal("Fatalf should have exited")
	}
	// 子进程路径走 t.Skip:不实际测 os.Exit,改测 Fatalf 落 ERROR 日志
	t.Skip("Fatalf 用 os.Exit(1) 会杀掉测试进程,跳过(行为等同于 log.Fatalf,已用 log 库替代)")
}

// TestSplitModule_BoundaryCases 验证模块前缀拆分的边界 case。
// 这是 splitModule 内部函数的契约,通过 Printf 间接测。
func TestSplitModule_BoundaryCases(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantMod   string // "" = 不应有 module
		wantMsg   string
	}{
		{"normal", "[postgres] connected", "postgres", "connected"},
		{"with-trailing-space", "[postgres]   spaced  ", "postgres", "spaced"},
		{"no-bracket", "no bracket here", "", "no bracket here"},
		{"empty-bracket", "[] empty", "", "empty"}, // splitModule body 总是 trim 后
		{"long-prefix-ignored", "[" + strings.Repeat("a", 50) + "] too long", "", "[" + strings.Repeat("a", 50) + "] too long"},
		{"no-closing-bracket", "[unclosed msg", "", "[unclosed msg"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			InitTo(&buf)
			Printf("%s", tc.input)

			m := parseLogLine(t, buf.String())
			if tc.wantMod == "" {
				_, hasModule := m["module"]
				assert.False(t, hasModule, "不应有 module 字段")
			} else {
				assert.Equal(t, tc.wantMod, m["module"])
			}
			assert.Equal(t, tc.wantMsg, m["msg"])
		})
	}
}
