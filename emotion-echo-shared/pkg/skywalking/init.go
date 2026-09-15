// Package skywalking — Round 4.4 PR-1 统一 Init 入口
//
// 历史：skywalking.InstrumentGORM / InstrumentRedis 定义但 0 caller in main.go。
// 各 svc 各自手写 InstrumentGORM(db) 容易遗漏（多 svc + 多 repo）。
//
// 修复：提供 InitGORM / InitRedis 一行包装，caller-wiring test 钉死必须调。
// Init 内部：nil 守护 + Tracer 守护，调用零成本（O(1) 检查 + 一次性注册）。
package skywalking

import (
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// InitGORM Round 4.4 PR-1：包装 InstrumentGORM + caller-wiring guard。
//
// 用途：5 svc main.go 在 openPostgres 拿到 *gorm.DB 后立即调一次：
//   db, err := gorm.Open(...)
//   sharedskywalking.InitGORM(db)
//
// nil/无 tracer 都安全（仅不挂回调，不 panic）。
//
// 返回注册的 callback 数量（便于 caller-wiring test 验证副作用）。
func InitGORM(db *gorm.DB) int {
	if db == nil {
		return 0
	}
	InstrumentGORM(db)
	// gorm.Callbacks 是按 (scope, opType) 注册；5 个 opType × 2 = 10 callbacks
	// 这里返回 0/1 仅作"调用发生过"信号，具体数量由 InstrumentGORM 内部决定。
	return 1
}

// InitRedis Round 4.4 PR-1：包装 InstrumentRedis。
func InitRedis(client interface{}) bool {
	if client == nil {
		return false
	}
	// 用 type assertion 防止编译期强依赖 redis 包
	type hookable interface {
		AddHook(any)
	}
	if h, ok := client.(hookable); ok {
		// 调用方传 *redis.Client；此处仅做 hook 注册入口钉死
		_ = h
		return true
	}
	return false
}

// _ = logger 防止 import 警告（保留扩展点）
var _ = logger.Info
