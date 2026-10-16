// record.go — CL-03 Phase 4c（CL-D-18=B）：把每次回放的结果落进 PG。
//
// 为什么需要：回放工具是**人工 CLI**（没有 /metrics 端点）⇒ "这次回了多少条、成没成功"
// 若只活在当次终端输出里，事后无法查、无法被 Grafana 消费。
// 落 PG 后：① 运维/事后排查可直接 SQL 查；② Grafana 用 PostgreSQL 数据源出"最近回放"面板。
//
// 设计取舍：
//   - 记录**失败不影响回放结果**（best-effort + 明确 warn）：回放本身是补救动作，
//     不能因为"记账失败"就让回放报错。
//   - DSN 从 PG* 环境变量拼（与 migrate.sh / backup_db.sh 同一套口径），便于容器内/宿主通用。
package main

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // pgx 的 database/sql 驱动
)

// pgDSNFromEnv 由 PG* 环境变量拼 DSN（缺省值 = 本仓 dev 口径）。
// getenv 可注入，便于单测（不碰真环境）。
func pgDSNFromEnv(getenv func(string) string) string {
	pick := func(k, def string) string {
		if v := getenv(k); v != "" {
			return v
		}
		return def
	}
	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		pick("PGUSER", "postgres"),
		pick("PGPASSWORD", "postgres"),
		pick("PGHOST", "localhost"),
		pick("PGPORT", "5432"),
		pick("PGDATABASE", "emotion_echo"),
	)
}

// runRecord 是一次回放运行的记录（对应 emotion_echo_analytics.dlq_replay_runs 一行）。
type runRecord struct {
	StartedAt   time.Time
	FinishedAt  time.Time
	DLQTopic    string
	TargetTopic string
	Replayed    int
	Failed      int
	DryRun      bool
	Operator    string
}

// recordRun 把一次回放结果写进 emotion_echo_analytics.dlq_replay_runs。
//
// 表由 analytics-svc 的迁移 a011 建（analytics 拥有该 schema）。
// 连接/写入失败一律返回 error 由调用方 warn（不 fatal）—— 记账失败不该让回放失败。
func recordRun(ctx context.Context, dsn string, rec runRecord) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("open pg: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping pg: %w", err)
	}
	_, err = db.ExecContext(ctx, `
		INSERT INTO emotion_echo_analytics.dlq_replay_runs
			(started_at, finished_at, dlq_topic, target_topic, replayed, failed, dry_run, operator)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		rec.StartedAt, rec.FinishedAt, rec.DLQTopic, rec.TargetTopic,
		rec.Replayed, rec.Failed, rec.DryRun, rec.Operator)
	if err != nil {
		return fmt.Errorf("insert dlq_replay_runs: %w", err)
	}
	return nil
}
