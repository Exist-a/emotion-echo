// cmd/memory-rebuild — CL-08d：「从 PG 重建图」运维工具（ADR 决策 8 配套）。
//
// 用途：图数据丢失/损坏/换图后端/历史行缺 uuid 时，以 PG 为准重放重建。
// 重放 = 清空该用户图（DeleteUserData）→ 按时间序逐条 UpsertEpisode → 回填 uuid。
//
// 用法（容器内或任意能同时访问 PG 与 llm-service 的环境）：
//
//	go run ./cmd/memory-rebuild -user 1            # 重建指定用户
//	go run ./cmd/memory-rebuild -all               # 重建全部有情节的用户
//
// 环境变量（与 compose 同名）：
//
//	MEMORY_REBUILD_PG_DSN   默认 postgres://postgres:postgres@localhost:5432/emotion_echo?sslmode=disable
//	MEMORY_GRAPH_ADDR       默认 emotion-llm-service:50052（本地跑用 localhost:50052）
//	MEMORY_GRAPH_TLS        默认 1（与 ai-svc 同范式；本地明文调试设 0）
//	MEMORY_GRAPH_API_KEY / AI_LLM_INTERNAL_API_KEY / INTERNAL_API_KEY（鉴权回落链）
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"emotion-echo-ai-svc/internal/memory"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type pgEpisodeRow struct {
	SourceKind string
	SourceID   int64
	UserID     int64
	Name       string
	Content    string
	ValidAt    time.Time
}

func main() {
	userID := flag.Int64("user", 0, "重建指定用户（0 = 不指定）")
	all := flag.Bool("all", false, "重建全部有情节的用户")
	flag.Parse()
	if !*all && *userID <= 0 {
		fmt.Fprintln(os.Stderr, "必须 -user <id> 或 -all")
		os.Exit(2)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	dsn := envOr("MEMORY_REBUILD_PG_DSN",
		"postgres://postgres:postgres@localhost:5432/emotion_echo?sslmode=disable")
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		fatal("open pg", err)
	}
	defer func() { _ = db.Close() }()

	writer := memory.NewGraphWriterFromEnv()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	users := []int64{}
	switch {
	case *all:
		rows, qerr := db.QueryContext(ctx,
			`SELECT DISTINCT user_id FROM emotion_echo_ai.memory_episode WHERE deleted_at IS NULL ORDER BY user_id`)
		if qerr != nil {
			fatal("list users", qerr)
		}
		for rows.Next() {
			var id int64
			if serr := rows.Scan(&id); serr != nil {
				fatal("scan user", serr)
			}
			users = append(users, id)
		}
		_ = rows.Close()
	default:
		users = append(users, *userID)
	}

	totalOK, totalFailed := 0, 0
	for _, uid := range users {
		eps, err := loadEpisodes(ctx, db, uid)
		if err != nil {
			fatal("load episodes", err)
		}
		srcs := make([]memory.EpisodeSource, 0, len(eps))
		for _, e := range eps {
			srcs = append(srcs, memory.EpisodeSource{
				SourceKind: e.SourceKind, SourceID: e.SourceID, UserID: e.UserID,
				Name: e.Name, Content: e.Content, ValidAtUnixMs: e.ValidAt.UnixMilli(),
			})
		}
		okN, failedN, rerr := memory.RebuildUserGraph(ctx, writer, &sqlUUIDSetter{db: db}, srcs, uid)
		if rerr != nil {
			fatal(fmt.Sprintf("rebuild user %d", uid), rerr)
		}
		slog.Info("user rebuilt", "user_id", uid, "episodes", len(eps), "ok", okN, "failed", failedN)
		totalOK += okN
		totalFailed += failedN
	}
	slog.Info("rebuild finished", "users", len(users), "ok", totalOK, "failed", totalFailed)
	if totalFailed > 0 {
		os.Exit(1)
	}
}

// loadEpisodes 直接 SQL 读（跨 schema 只读 memory_episode —— 重建工具是运维
// 旁路，不走服务 gRPC；只 SELECT，不写业务表，符合"只碰自己 schema"的守卫精神）。
func loadEpisodes(ctx context.Context, db *sql.DB, uid int64) ([]pgEpisodeRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT source_kind, source_id, user_id, name, content, valid_at
		FROM emotion_echo_ai.memory_episode
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY valid_at ASC, id ASC`, uid)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []pgEpisodeRow{}
	for rows.Next() {
		var r pgEpisodeRow
		if err := rows.Scan(&r.SourceKind, &r.SourceID, &r.UserID, &r.Name, &r.Content, &r.ValidAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// sqlUUIDSetter 用裸 SQL 回填 uuid（同上：运维旁路，只更新自己 schema 的列）。
type sqlUUIDSetter struct{ db *sql.DB }

func (s *sqlUUIDSetter) SetEpisodeGraphUUID(ctx context.Context, sourceKind string, sourceID int64, uuid string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE emotion_echo_ai.memory_episode SET graph_uuid = $1
		 WHERE source_kind = $2 AND source_id = $3 AND deleted_at IS NULL`,
		uuid, sourceKind, sourceID)
	return err
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func fatal(msg string, err error) {
	slog.Error(msg, "err", err)
	os.Exit(1)
}
