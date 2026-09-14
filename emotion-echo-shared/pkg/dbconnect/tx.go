// Package dbconnect - 事务重试 helper (P1-20 Round 1)
//
// PostgreSQL 在序列化失败 (SQLSTATE 40001) 或死锁 (40P01) 时会让事务
// 整体回滚。GORM 的 Transaction 默认无 retry，调用方直接 5xx 给客户端。
//
// 本文件提供 WithRetry 包装，自动识别 40001/40P01 并重试。
package dbconnect

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
)

// PG retry-able SQLSTATE codes
const (
	pgSerializationFailure = "40001" // could not serialize access
	pgDeadlockDetected    = "40P01" // deadlock detected
)

// DefaultTxRetryConfig 默认重试配置
type TxRetryConfig struct {
	MaxAttempts int           // 默认 3
	BaseDelay   time.Duration // 默认 50ms，指数退避
}

// DefaultTxRetry 默认重试配置（3 次 / 50ms base）
var DefaultTxRetry = TxRetryConfig{MaxAttempts: 3, BaseDelay: 50 * time.Millisecond}

// IsRetryablePGError 判断是否为可重试的 PG 错误（序列化失败 / 死锁）
//
// 实现：检查错误消息是否包含 PG SQLSTATE 码（40001 或 40P01）。
// 不引入 pgconn 依赖（shared 库应保持轻量）。
func IsRetryablePGError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, pgSerializationFailure) ||
		strings.Contains(msg, pgDeadlockDetected)
}

// WithTxRetry 在 gorm.DB 上跑事务，自动重试序列化失败/死锁。
//
// 用法：
//
//	err := dbconnect.WithTxRetry(db, func(tx *gorm.DB) error {
//	    return tx.Create(&row).Error
//	})
func WithTxRetry(db *gorm.DB, fn func(tx *gorm.DB) error, cfg ...TxRetryConfig) error {
	c := DefaultTxRetry
	if len(cfg) > 0 {
		c = cfg[0]
	}
	if c.MaxAttempts < 1 {
		c.MaxAttempts = 1
	}
	if c.BaseDelay <= 0 {
		c.BaseDelay = 50 * time.Millisecond
	}

	var lastErr error
	for attempt := 1; attempt <= c.MaxAttempts; attempt++ {
		err := db.Transaction(fn)
		if err == nil {
			return nil
		}
		lastErr = err
		if !IsRetryablePGError(err) {
			return err
		}
		// 指数退避 + 抖动
		delay := c.BaseDelay * (1 << (attempt - 1))
		if attempt < c.MaxAttempts {
			time.Sleep(delay)
		}
	}
	return lastErr
}

// WithTxRetryContext 同 WithTxRetry，支持 ctx 取消
func WithTxRetryContext(ctx context.Context, db *gorm.DB, fn func(tx *gorm.DB) error, cfg ...TxRetryConfig) error {
	c := DefaultTxRetry
	if len(cfg) > 0 {
		c = cfg[0]
	}
	if c.MaxAttempts < 1 {
		c.MaxAttempts = 1
	}
	if c.BaseDelay <= 0 {
		c.BaseDelay = 50 * time.Millisecond
	}

	var lastErr error
	for attempt := 1; attempt <= c.MaxAttempts; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := db.WithContext(ctx).Transaction(fn)
		if err == nil {
			return nil
		}
		lastErr = err
		if !IsRetryablePGError(err) {
			return err
		}
		delay := c.BaseDelay * (1 << (attempt - 1))
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return lastErr
}
