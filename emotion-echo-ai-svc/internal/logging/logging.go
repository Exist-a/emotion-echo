// Package logging 是 ai-svc 的 thin re-export,实际定义在 shared/pkg/logging。
//
// Stage 41 PR-5: 原 internal/logging/logger.go (144 行,clone 自 web-bff/internal/logging)
// 已删除,改为本文件 re-export 共享包,保留 import 路径兼容。
//
// 业务代码继续用 'emotion-echo-ai-svc/internal/logging' 即可,无需改 import。
package logging

import "github.com/emotion-echo/shared/pkg/logging"

// Re-export shared/pkg/logging 全部公开 API
var (
	Init      = logging.Init
	InitTo    = logging.InitTo
	Printf    = logging.Printf
	Infof     = logging.Infof
	Warnf     = logging.Warnf
	Fatalf    = logging.Fatalf
)
