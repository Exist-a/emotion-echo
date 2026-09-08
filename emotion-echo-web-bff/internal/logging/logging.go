// Package logging 是 web-bff 的 thin re-export,实际定义在 shared/pkg/logging。
//
// Stage 41 PR-7: 原 internal/logging/logger.go (135 行,clone 自 ai-svc)
// 已删除,改为本文件 re-export 共享包,保留 import 路径兼容。
package logging

import "github.com/emotion-echo/shared/pkg/logging"

// Re-export shared/pkg/logging 全部公开 API
var (
	Init         = logging.Init
	InitTo       = logging.InitTo
	Printf       = logging.Printf
	Infof        = logging.Infof
	Warnf        = logging.Warnf
	Fatalf       = logging.Fatalf
	// PR-OBS-15: 决策 6 必填字段 helper(PR-OBS-15 GREEN commit 已落地 shared 包)
	SetGlobalSvc = logging.SetGlobalSvc
	WithTraceID  = logging.WithTraceID
	WithAction   = logging.WithAction
)
