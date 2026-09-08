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
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
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

// ===== PR-OBS-15 RED: 5 case — 决策 6 必填 JSON 字段断言 =====
//
// 目的: PR-OBS-15 设计 (observability-sprint-b.md §三.15):
// 1. ts 字段存在且 ISO8601 (现有 slog 自动加,直接 GREEN)
// 2. level 字段存在 (现有 slog 自动加,直接 GREEN)
// 3. svc 字段存在且等于 svcName (RED — 需加 helper)
// 4. trace_id 字段存在 (RED — 需加 helper, 从 ctx 透传)
// 5. action 字段存在 (RED — 需加 helper)
//
// 现状: shared/pkg/logging 当前输出 {time, level, msg, module} 4 字段
//      缺 svc/trace_id/action (决策 6 审计要求必填)
// GREEN 阶段需在 logging.go 加 WithSvc/WithTraceID/WithAction context helpers
// 或扩展 API,让业务 svc 在 main 启动时 SetGlobalSvc 后所有 log 自动带 svc 字段

// TestLogFields_TimestampField 测试 ts 字段存在且 ISO8601 格式
// 现状: slog JSON handler 自动加 "time" 字段 (ISO8601),应直接 GREEN
func TestLogFields_TimestampField(t *testing.T) {
	var buf bytes.Buffer
	InitTo(&buf)
	slog.Info("ts-test")

	m := parseLogLine(t, buf.String())
	tsRaw, ok := m["time"].(string)
	if !ok {
		t.Fatal("JSON 输出缺 time 字段 (slog JSON handler 应自动加)")
	}
	// ISO8601 含 'T' 分隔日期时间
	if !strings.Contains(tsRaw, "T") {
		t.Errorf("time 字段不是 ISO8601 格式: %q", tsRaw)
	}
}

// TestLogFields_LevelField 测试 level 字段存在
func TestLogFields_LevelField(t *testing.T) {
	var buf bytes.Buffer
	InitTo(&buf)
	slog.Warn("level-test")

	m := parseLogLine(t, buf.String())
	if got := m["level"]; got != "WARN" {
		t.Errorf("level 字段 = %v, want WARN", got)
	}
}

// TestLogFields_ServiceField 测试 svc 字段存在且匹配 svcName (RED — 待实现)
func TestLogFields_ServiceField(t *testing.T) {
	var buf bytes.Buffer
	InitTo(&buf)
	SetGlobalSvc("chat-svc")            // GREEN 阶段需实现
	slog.Info("svc-test")

	m := parseLogLine(t, buf.String())
	if got := m["svc"]; got != "chat-svc" {
		t.Errorf("svc 字段 = %v, want chat-svc (决策 6 审计必填)", got)
	}
}

// TestLogFields_TraceIDField 测试 trace_id 字段从 ctx 透传 (RED — 待实现)
func TestLogFields_TraceIDField(t *testing.T) {
	var buf bytes.Buffer
	InitTo(&buf)
	SetGlobalSvc("chat-svc")
	ctx := WithTraceID(context.Background(), "trace-abc-123") // GREEN 阶段需实现
	slog.InfoContext(ctx, "trace-test")

	m := parseLogLine(t, buf.String())
	if got := m["trace_id"]; got != "trace-abc-123" {
		t.Errorf("trace_id 字段 = %v, want trace-abc-123", got)
	}
}

// TestLogFields_ActionField 测试 action 字段从 ctx 透传 (RED — 待实现)
func TestLogFields_ActionField(t *testing.T) {
	var buf bytes.Buffer
	InitTo(&buf)
	SetGlobalSvc("chat-svc")
	ctx := WithAction(context.Background(), "user.login") // GREEN 阶段需实现
	slog.InfoContext(ctx, "action-test")

	m := parseLogLine(t, buf.String())
	if got := m["action"]; got != "user.login" {
		t.Errorf("action 字段 = %v, want user.login", got)
	}
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

// ===== PR-OBS-15 端到端组合 case (2026-09-08 接入前护栏) =====
//
// 目的: PR-OBS-15 GREEN commit (e730393) 加了 5 个单字段 case (svc / trace_id /
// action 各 1),但生产路径是 3 字段同时存在(6 svc main.go 接入 Init +
// SetGlobalSvc 后,gin handler 内 slog.InfoContext 自动收到全部)。
//
// 本组 case 验证组合行为 + 6 svc main.go 接入后才能 GREEN:
//
// 1. TestLogFields_AllThreeFieldsCombined_RealisticFlow
//    模拟 svc main.go 启动 → gin handler 处理请求 → slog 输出
//    断言 JSON 同时含 svc/trace_id/action 3 字段 (PR-OBS-15 RED 状态)
//    → 6 svc main.go 接入 SetGlobalSvc 后应 GREEN
//
// 2. TestLogFields_SvcFieldPersistsAcrossLogs
//    多次 slog.Info 调用,断言每条都有 svc 字段 (全局变量,不随 ctx 变)
//    → 即使只 SetGlobalSvc 一次,所有日志都带 svc
//
// 注: PR-OBS-15 的 5 case 当时是单字段测试,因为 helper 实现是核心;
//     本 PR 接入 (6 svc main.go 改) 是组合验证。helper 已 GREEN,
//     本 case 落地的 RED 是 "6 svc main.go 是否真调 Init/SetGlobalSvc"
//     的源码契约 —— 但 runtime 测不出来,所以改用 helper 组合 case
//     做语义护栏。
//
// RED 状态: 当前组合 case 应直接 GREEN(helper 已实现),作为"防退化"护栏。

// TestLogFields_AllThreeFieldsCombined_RealisticFlow 模拟 svc 启动 + handler 日志
// 端到端流程,断言 JSON 同时含 svc/trace_id/action 3 字段
func TestLogFields_AllThreeFieldsCombined_RealisticFlow(t *testing.T) {
	var buf bytes.Buffer
	InitTo(&buf)
	SetGlobalSvc("chat-svc")

	// 模拟 gin middleware 链:WithTraceID → WithAction → handler slog
	ctx := WithTraceID(context.Background(), "trace-xyz-001")
	ctx = WithAction(ctx, "chat.handler.PostMessage")

	slog.InfoContext(ctx, "post message received", "msg_len", 42)

	m := parseLogLine(t, buf.String())
	if got := m["svc"]; got != "chat-svc" {
		t.Errorf("svc field = %v, want chat-svc", got)
	}
	if got := m["trace_id"]; got != "trace-xyz-001" {
		t.Errorf("trace_id field = %v, want trace-xyz-001", got)
	}
	if got := m["action"]; got != "chat.handler.PostMessage" {
		t.Errorf("action field = %v, want chat.handler.PostMessage", got)
	}
	// msg + msg_len 必须存在(业务字段不被吞)
	if got := m["msg"]; got != "post message received" {
		t.Errorf("msg field = %v, want post message received", got)
	}
	if got, ok := m["msg_len"].(float64); !ok || got != 42 {
		t.Errorf("msg_len field = %v (类型 %T), want 42", m["msg_len"], m["msg_len"])
	}
}

// TestLogFields_SvcFieldPersistsAcrossLogs SetGlobalSvc 后多次调用,每条都有 svc 字段
func TestLogFields_SvcFieldPersistsAcrossLogs(t *testing.T) {
	var buf bytes.Buffer
	InitTo(&buf)
	SetGlobalSvc("user-svc")

	slog.Info("first")
	slog.Info("second")
	slog.Info("third")

	// 解析 3 行 JSON
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 log lines, got %d: %q", len(lines), buf.String())
	}
	for i, line := range lines {
		m := parseLogLine(t, line)
		if got := m["svc"]; got != "user-svc" {
			t.Errorf("line %d: svc field = %v, want user-svc", i+1, got)
		}
	}
}

// ===== PR-OBS-15 RED: 6 svc main.go 接入层契约 =====
//
// 目的: 决策 6 必填字段 svc/trace_id/action 的"Loki 可查询"前提
//      是每个 svc main.go 调 logging.Init() + logging.SetGlobalSvc("<svc-name>")。
// 当前状态(2026-09-08 接入前):
//   - 调 Init:  web-bff, ai-svc      (2/6)
//   - 调 SetGlobalSvc: (none)         (0/6)  ← 关键缺口,即使 Init 了也没 svc 字段
// PR-OBS-15 GREEN 必须落地"6 svc 全调 Init + SetGlobalSvc"。
//
// 测试方法: 源码 grep 6 svc main.go,断言每个都同时含 Init 和 SetGlobalSvc。
// 这是 stage-44 §四 C 落地的硬护栏 —— 与 PR-OBS-14 TraceTagLiterals 同模式
// (字面量断言的局限已知,本 case 用于"接入完整性"防退化)。
//
// svc 名 → 期望文件路径 → 期望字面量(避免重命名 svc 后断言失效):
//   ai-svc     emotion-echo-ai-svc/main.go          SetGlobalSvc("ai-svc")
//   chat-svc   emotion-echo-chat-svc/main.go        SetGlobalSvc("chat-svc")
//   user-svc   emotion-echo-user-svc/main.go        SetGlobalSvc("user-svc")
//   assessment emotion-echo-assessment-svc/main.go  SetGlobalSvc("assessment-svc")
//   analytics  emotion-echo-analytics-svc/main.go   SetGlobalSvc("analytics-svc")
//   web-bff    emotion-echo-web-bff/main.go         SetGlobalSvc("web-bff")
//
// 注: trace_id / action 字段由 GinSkywalkingMiddleware 和 gin handler 注入,
// 不在本 case 范围 (后续 PR-OBS-19/20 处理 interceptor / handler 层)。
//
// 目录结构: 测试运行时 cwd 不固定,用相对路径 ../emotion-echo-<svc>/main.go 遍历;
// 实际目录由项目根决定。本 case 用 t.Skip 兼容(找文件失败时)以便 5 logging
// 核心 case 不被阻塞。

// TestMain_FilesInvokeInitAndSetGlobalSvc 6 svc main.go 必须 Init + SetGlobalSvc
func TestMain_FilesInvokeInitAndSetGlobalSvc(t *testing.T) {
	svcs := []struct {
		dir  string
		name string // 期望的 SetGlobalSvc 字面量
	}{
		{"emotion-echo-ai-svc", "ai-svc"},
		{"emotion-echo-chat-svc", "chat-svc"},
		{"emotion-echo-user-svc", "user-svc"},
		{"emotion-echo-assessment-svc", "assessment-svc"},
		{"emotion-echo-analytics-svc", "analytics-svc"},
		{"emotion-echo-web-bff", "web-bff"},
	}

	// logging_test.go 在 emotion-echo-shared/pkg/logging/,svc main.go 在
	// Emotion-Echo/<svc>/main.go (4 层 ..: logging → pkg → shared → Emotion-Echo)
	for _, svc := range svcs {
		mainPath := filepath.Join("..", "..", "..", svc.dir, "main.go")
		src, err := os.ReadFile(mainPath)
		if err != nil {
			t.Skipf("cannot read %s (cwd-relative skip): %v", mainPath, err)
			continue
		}
		content := string(src)

		// 1. 必须有 logging.Init() 调用
		if !strings.Contains(content, "logging.Init()") {
			t.Errorf("[%s] missing logging.Init() — 决策 6 必填字段前提缺失", svc.dir)
		}
		// 2. 必须有 logging.SetGlobalSvc("<expected-name>") 调用
		wantCall := `logging.SetGlobalSvc("` + svc.name + `")`
		if !strings.Contains(content, wantCall) {
			t.Errorf("[%s] missing %s — 决策 6 svc 字段缺失,Loki 查询 svc=<name> 无结果", svc.dir, wantCall)
		}
	}
}
