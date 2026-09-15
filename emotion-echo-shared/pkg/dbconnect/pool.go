// Package dbconnect — PG 连接池配置化（Round 4.4 PR-2）
//
// 历史：5 svc 全部硬编码默认 MaxOpenConns=10 / MaxIdleConns=5，
// PG `max_connections=200` 但总连接预算无规划，多副本部署会爆。
//
// 修复：ApplyPoolEnv 从 env 读 PG_MAX_CONNS / PG_MIN_IDLE / PG_MAX_LIFETIME_SECONDS，
// 缺省保持向后兼容（10/5/1h），prod 模式必须显式注入。
//
// 适用：所有 5 svc 的 openPostgres 调用 SetMax* 前调用本函数。
package dbconnect

import (
	"database/sql"
	"os"
	"strconv"
	"time"
)

// DefaultMaxOpenConns / DefaultMaxIdleConns / DefaultConnMaxLifetime ——
// 历史硬编码值，作为 env 缺失时的 fallback。
const (
	DefaultMaxOpenConns    = 10
	DefaultMaxIdleConns    = 5
	DefaultConnMaxLifetime = time.Hour
)

// ApplyPoolEnv Round 4.4 PR-2：从 env 读 PG 池配置并 SetMax* 到 sql.DB。
//
// env：
//   - PG_MAX_CONNS（int；缺省 DefaultMaxOpenConns）
//   - PG_MIN_IDLE_CONNS（int；缺省 DefaultMaxIdleConns）
//   - PG_MAX_LIFETIME_SECONDS（int；缺省 DefaultConnMaxLifetime.Seconds()）
//
// 各 svc main.go 在 openPostgres 拿到 sqlDB 后立即调一次：
//   sqlDB, _ := db.DB()
//   dbconnect.ApplyPoolEnv(sqlDB)
//
// prod 部署建议：5 svc × 20 conns = 100 < PG max_connections=200，留 100 给
// 其他客户端（如 psql 调试、migration 工具）。
func ApplyPoolEnv(sqlDB *sql.DB) error {
	if sqlDB == nil {
		return nil
	}
	maxOpen, err := envInt("PG_MAX_CONNS", DefaultMaxOpenConns)
	if err != nil {
		return err
	}
	maxIdle, err := envInt("PG_MIN_IDLE_CONNS", DefaultMaxIdleConns)
	if err != nil {
		return err
	}
	lifetimeSecs, err := envInt("PG_MAX_LIFETIME_SECONDS", int(DefaultConnMaxLifetime.Seconds()))
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(time.Duration(lifetimeSecs) * time.Second)
	return nil
}

func envInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	return strconv.Atoi(v)
}
