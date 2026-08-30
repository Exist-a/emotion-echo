// Package logging 提供 emotion-echo-web-bff 的结构化日志（clone 自 ai-svc）。
//
// 使用 Go 1.21+ stdlib slog，统一输出 JSON 到 stdout：
//   {"time":"...","level":"INFO","msg":"...","module":"...","err":"..."}
//
// 用法：
//   logging.Init()          // main 入口
//   logging.Info(...)        // 或 slog.Info / Printf / Errorf 便捷函数
//   logging.Errorf(err, "kafka consume failed", "topic", topic)
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

// ============ Convenience helpers（兼容 log.Printf 调用风格） ============

// Printf 等价于 log.Printf，但走 slog。module 前缀自动剥离到 "module" 字段。
func Printf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	module, body := splitModule(msg)
	if module != "" {
		slog.Info(body, "module", module)
	} else {
		slog.Info(msg)
	}
}

// Infof 同 Printf，显式 INFO level。
func Infof(format string, args ...any) { Printf(format, args...) }

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

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
