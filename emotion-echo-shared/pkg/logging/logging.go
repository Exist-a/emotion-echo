// Package logging 提供 emotion-echo 各 Go svc 的统一结构化日志（Stage 41 PR-1）。
//
// 来源：web-bff/internal/logging + ai-svc/internal/logging 是 clone 关系
// （原文件头注释明写"clone 自 ai-svc"），本包把它们下沉到 shared，
// 业务 svc 只需 `import "github.com/emotion-echo/shared/pkg/logging"`。
//
// 设计决策（stage doc §三 PR-1 △ 偏差登记）：
//   - **Errorf helper 不下沉**：BFF/ai-svc 的 (err, format, args...) 签名
//     与 chat-svc 现状 (format, args...) 不兼容。各 svc 改写时统一走
//     stdlib slog.ErrorContext(ctx, msg, "err", err)，err 作结构化字段。
//   - Printf/Infof/Warnf/Fatalf/Init/InitTo 下沉：API 与原 internal/logging 完全兼容。
//   - 模块前缀自动剥离："[postgres] connected" → module="postgres", msg="connected"。
//
// 输出格式：JSON to stdout（决策 6 审计要求）
//   {"time":"...","level":"INFO","msg":"...","module":"..."}
//
// 环境变量：
//   LOG_FORMAT = json (default) | text
//   LOG_LEVEL  = INFO (default) | DEBUG | WARN | ERROR
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Init 初始化全局 slog JSON handler，输出到 stdout。
func Init() {
	InitTo(os.Stdout)
}

// InitTo 同 Init，但允许指定输出（用于 e2e 测试用 bytes.Buffer 捕获）。
func InitTo(w io.Writer) {
	logFormat := strings.ToLower(os.Getenv("LOG_FORMAT"))
	if logFormat == "" {
		logFormat = "json"
	}
	logLevel := strings.ToUpper(os.Getenv("LOG_LEVEL"))
	if logLevel == "" {
		logLevel = "INFO"
	}

	opts := &slog.HandlerOptions{
		Level: parseLevel(logLevel),
	}

	var handler slog.Handler
	if logFormat == "text" {
		handler = slog.NewTextHandler(w, opts)
	} else {
		handler = slog.NewJSONHandler(w, opts)
	}

	slog.SetDefault(slog.New(handler))
}

// parseLevel 把字符串映射到 slog.Level，未识别值走 INFO。
func parseLevel(s string) slog.Level {
	switch s {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Printf 等价于 log.Printf，但走 slog。module 前缀自动剥离到 "module" 字段。
func Printf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	emit(slog.LevelInfo, msg)
}

// Infof 同 Printf，显式 INFO level（与 Printf 同义，保留 API 兼容）。
func Infof(format string, args ...any) {
	Printf(format, args...)
}

// Warnf 走 slog.Warn。
func Warnf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	emit(slog.LevelWarn, msg)
}

// Fatalf 记录后退出（兼容 log.Fatalf 用法）。
// 注：调用 os.Exit(1) 会杀掉当前进程；测试中不可调。
func Fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	emit(slog.LevelError, msg)
	os.Exit(1)
}

// emit 把消息按 [module] 前缀拆分后写到 slog。
// 这是拆分逻辑的唯一入口，方便测试和重构。
func emit(level slog.Level, msg string) {
	module, body := splitModule(msg)
	switch level {
	case slog.LevelWarn:
		if module != "" {
			slog.Warn(body, "module", module)
		} else {
			slog.Warn(body)
		}
	case slog.LevelError:
		if module != "" {
			slog.Error(body, "module", module)
		} else {
			slog.Error(body)
		}
	default: // INFO 及其他走 Info
		if module != "" {
			slog.Info(body, "module", module)
		} else {
			slog.Info(body)
		}
	}
}

// splitModule 把 "[postgres] connected" 拆成 ("postgres", "connected")。
//
// 边界规则（与原 web-bff/ai-svc internal/logging 完全一致）：
//   - 必须以 '[' 开头
//   - ']' 必须在 0..32 字符内（前缀名最长 32 字符）
//   - 拆分后 trim 空格作为 msg
//
// 不匹配的输入原样返回 msg，module = ""。
func splitModule(msg string) (module, body string) {
	if !strings.HasPrefix(msg, "[") {
		return "", msg
	}
	end := strings.Index(msg, "]")
	if end < 0 || end > 32 {
		return "", msg
	}
	module = msg[1:end]
	body = strings.TrimSpace(msg[end+1:])
	return module, body
}
