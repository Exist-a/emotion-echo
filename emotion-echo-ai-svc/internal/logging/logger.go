// Package logging 提供 ai-svc 的结构化日志（Stage 20-4）
//
// 使用 Go 1.21+ stdlib slog，统一输出 JSON 到 stdout：
//   {"time":"2026-07-15T13:00:00Z","level":"INFO","msg":"...","module":"postgres",...}
//
// 用法：
//   import "emotion-echo-ai-svc/internal/logging"
//   logging.Init()  // main 入口
//   logging.Info("postgres connected", "dsn_host", host)
//   logging.Errorf(err, "kafka consume failed", "topic", topic)
//
// 兼容层：保留 log.Printf 的便利函数 logging.Printf / Infof。
package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

// Init 初始化全局 slog JSON handler。
// 读取环境变量：
//   LOG_FORMAT = json (default) | text
//   LOG_LEVEL  = INFO (default) | DEBUG | WARN | ERROR
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
		// AddSource: true 加 caller 信息（生产可选，会增加日志体积）
	}

	var handler slog.Handler
	if logFormat == "text" {
		handler = slog.NewTextHandler(w, opts)
	} else {
		handler = slog.NewJSONHandler(w, opts)
	}

	slog.SetDefault(slog.New(handler))
}

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

// ============ Convenience helpers（兼容旧 log.Printf 调用风格） ============

// Printf 等价于 log.Printf，但走 slog。
// 旧代码里的 "[module] msg" 风格：module 前缀会被自动剥离到 "module" 字段。
func Printf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	module, body := splitModule(msg)
	if module != "" {
		slog.Info(body, "module", module)
	} else {
		slog.Info(msg)
	}
}

// Infof 同 Printf，但显式标识 INFO level。
func Infof(format string, args ...any) {
	Printf(format, args...)
}

// Warnf 走 slog.Warn。
func Warnf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	module, body := splitModule(msg)
	if module != "" {
		slog.Warn(body, "module", module)
	} else {
		slog.Warn(msg)
	}
}

// Errorf 走 slog.Error，自动把 err 加到 "err" 字段。
func Errorf(err error, format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	module, body := splitModule(msg)
	if module != "" {
		slog.Error(body, "module", module, "err", errString(err))
	} else {
		slog.Error(msg, "err", errString(err))
	}
}

// Fatalf 记录后退出（兼容 log.Fatalf 用法）。
// 注意：slog 没有 Fatal 级别，ERROR 后 os.Exit(1)。
func Fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	module, body := splitModule(msg)
	if module != "" {
		slog.Error(body, "module", module)
	} else {
		slog.Error(msg)
	}
	os.Exit(1)
}

// splitModule 把 "[postgres] connected" 拆成 ("postgres", "connected")。
// 没有 [xxx] 前缀时返回 ("", msg)。
func splitModule(msg string) (module, body string) {
	if !strings.HasPrefix(msg, "[") {
		return "", msg
	}
	end := strings.Index(msg, "]")
	if end < 0 || end > 32 { // 限制 module 名长度，避免误判
		return "", msg
	}
	module = msg[1:end]
	body = strings.TrimSpace(msg[end+1:])
	return module, body
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
