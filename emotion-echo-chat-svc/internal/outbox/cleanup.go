// Package outbox — cleanup.go
//
// Round 2.1 §D2: outbox sent/dead 清理 job。
//
// 背景（kafka-pipeline-pending-decisions.md §D2）：
//   c001_create_outbox_events.sql 无 cleanup 字段，relay.go MarkSent 后永不触碰该行。
//   聊天高频事件 → 几月后百万行级；JSONB payload 让单行更大。
//
// 设计：
//   - CleanupOnce 一次性执行（也作为 ticker 单轮）：分两批删
//     * status='sent' AND sent_at < now()-sentRetentionDays
//     * status='dead' AND created_at < now()-deadRetentionDays
//   - limit > 0 时每批最多删 limit 行（防长事务）
//   - 主流程 main.go 启 goroutine ticker（默认 1h）定期跑 CleanupOnce
//
// 为什么不在 relay.Run 内合并：relay.Run 是热路径，每秒跑一次；
// cleanup 是冷路径（h/级别），混在一起会让 relay 阻塞或与其他 ticker 抢锁。
//
// 安全：
//   - 不删 pending 行（pending 还在 relay 队列，删除会丢事件）
//   - 不删 failed 行（failed 是 transient，下一轮 relay 再试）
//   - dead 行用 CreatedAt 判定（LastError 是 msg string，不能当时间戳）
package outbox

import (
	"context"
	"time"

	"emotion-echo-chat-svc/internal/repository"
)

// DefaultCleanupLimit 单轮最多删多少行（防长事务；大表场景分轮删）
const DefaultCleanupLimit = 500

// CleanupOnce 一次性清理老 sent / dead 行
//
// 参数：
//   - ctx: 主流程 ctx 取消时函数立即返（不再发起新 DELETE）
//   - repo: outbox 仓库（InMemory / Postgres）
//   - sentRetentionDays: sent 行保留天数（> 0；0 时跳过 sent 清理）
//   - deadRetentionDays: dead 行保留天数（> 0；0 时跳过 dead 清理）
//   - limit: 每批上限（0 时用 DefaultCleanupLimit；负数不限）
//
// 返回：本次删除的总行数（sent + dead 之和）。
//
// 错误返回语义：仅在 DELETE SQL 出错或参数非法（status 不支持）时报错。
// "没行可删" 不视为错误，deleted=0。
func CleanupOnce(ctx context.Context, repo repository.OutboxRepo, sentRetentionDays int, deadRetentionDays int, limit int) (int64, error) {
	if limit == 0 {
		limit = DefaultCleanupLimit
	}
	if limit < 0 {
		limit = 0
	}

	var totalDeleted int64
	now := time.Now()

	if sentRetentionDays > 0 {
		cutoff := now.AddDate(0, 0, -sentRetentionDays)
		n, err := repo.DeleteOlderThan(ctx, repository.OutboxStatusSent, cutoff, limit)
		if err != nil {
			return totalDeleted, err
		}
		totalDeleted += n
		IncCleaned(repository.OutboxStatusSent, n)
	}

	if deadRetentionDays > 0 {
		cutoff := now.AddDate(0, 0, -deadRetentionDays)
		n, err := repo.DeleteOlderThan(ctx, repository.OutboxStatusDead, cutoff, limit)
		if err != nil {
			return totalDeleted, err
		}
		totalDeleted += n
		IncCleaned(repository.OutboxStatusDead, n)
	}

	return totalDeleted, nil
}