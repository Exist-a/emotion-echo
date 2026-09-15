// Package outbox — cleanup_test.go
//
// Round 2.1 §D2 RED：outbox sent/dead 清理 job。
//
// 契约（multi-round-iteration-2026-09-15.md §四 Round 2.1）：
//   - CleanupOnce(sentRetentionDays, deadRetentionDays, limit) 一次性删：
//     * status='sent'  AND sent_at < now()-sentRetentionDays
//     * status='dead'  AND last_error 写入时间 < now()-deadRetentionDays（fallback: created_at）
//   - 删行后通过 OutboxRepo.DeleteOlderThan 批量执行（不调 ListPending + 循环 MarkSent）
//   - 触发条件 = 演示期前必做；当前 dev 库无感（百万行级才会显现）
package outbox

import (
	"context"
	"os"
	"testing"
	"time"

	"emotion-echo-chat-svc/internal/repository"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// backdateEvent 把 OutboxEvent 设为 sent/dead 状态，并把 sent_at / last_error 时间回拨 N 天
//
// cleanup 用 sent_at / last_error 判定行龄；测试需要构造"老行"。
func backdateEvent(e *repository.OutboxEvent, daysAgo int, setStatus string) {
	now := time.Now()
	switch setStatus {
	case repository.OutboxStatusSent:
		sent := now.AddDate(0, 0, -daysAgo)
		e.Status = repository.OutboxStatusSent
		e.SentAt = &sent
	case repository.OutboxStatusDead:
		e.Status = repository.OutboxStatusDead
		e.LastError = "forced dead"
		// dead 行用 LastError 不可靠（LastError 是 msg string，不是时间），退用 CreatedAt
		e.CreatedAt = now.AddDate(0, 0, -daysAgo)
	}
}

// seedSent 注入 n 条 sent 行，回拨 daysAgo 天
func seedSent(t *testing.T, repo *repository.InMemoryOutboxRepo, n int, daysAgo int) []int64 {
	ids := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		e := &repository.OutboxEvent{
			EventID:   "evt-cleanup-sent-" + string(rune('A'+i%26)) + "-" + time.Now().Format("150405.000000"),
			EventType: "message.created",
			Topic:     "chat-events",
			Payload:   []byte(`{"id":"x","type":"message.created"}`),
		}
		backdateEvent(e, daysAgo, repository.OutboxStatusSent)
		require.NoError(t, repo.CreateInTx(nil, e))
		ids = append(ids, e.ID)
	}
	return ids
}

// seedDead 注入 n 条 dead 行，回拨 daysAgo 天
func seedDead(t *testing.T, repo *repository.InMemoryOutboxRepo, n int, daysAgo int) []int64 {
	ids := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		e := &repository.OutboxEvent{
			EventID:   "evt-cleanup-dead-" + string(rune('A'+i%26)) + "-" + time.Now().Format("150405.000000"),
			EventType: "message.created",
			Topic:     "chat-events",
			Payload:   []byte(`{"id":"x","type":"message.created"}`),
		}
		backdateEvent(e, daysAgo, repository.OutboxStatusDead)
		require.NoError(t, repo.CreateInTx(nil, e))
		ids = append(ids, e.ID)
	}
	return ids
}

// TestCleanupOnce_RemovesOldSentRows_KeepsRecentSent §D2 RED §1
//
// 100 行 sent（10 天前）+ 5 行 sent（1 天前）
// 跑 CleanupOnce(sentRetentionDays=7, deadRetentionDays=30, limit=200)
// 期望：100 老 sent 被删，5 新 sent 保留
func TestCleanupOnce_RemovesOldSentRows_KeepsRecentSent(t *testing.T) {
	repo := repository.NewInMemoryOutboxRepo()
	oldIDs := seedSent(t, repo, 100, 10)
	recentIDs := seedSent(t, repo, 5, 1)

	deleted, err := CleanupOnce(context.Background(), repo, 7, 30, 200)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, deleted, int64(100), "应至少删 100 行老 sent")

	// 断言：老 sent 已被删（Get 返 nil, nil）
	for _, id := range oldIDs {
		e, _ := repo.Get(id)
		assert.Nil(t, e, "老 sent id=%d 应已被清理", id)
	}

	// 断言：新 sent 保留
	for _, id := range recentIDs {
		e, _ := repo.Get(id)
		require.NotNil(t, e, "新 sent id=%d 应保留", id)
		assert.Equal(t, repository.OutboxStatusSent, e.Status)
	}
}

// TestCleanupOnce_RemovesOldDeadRows_KeepsRecentDead §D2 RED §2
//
// 5 行 dead（35 天前）+ 5 行 dead（10 天前）
// 跑 CleanupOnce(sentRetentionDays=7, deadRetentionDays=30, limit=200)
// 期望：5 行老 dead（35 天）被删，5 行新 dead（10 天）保留
func TestCleanupOnce_RemovesOldDeadRows_KeepsRecentDead(t *testing.T) {
	repo := repository.NewInMemoryOutboxRepo()
	oldIDs := seedDead(t, repo, 5, 35)
	recentIDs := seedDead(t, repo, 5, 10)

	deleted, err := CleanupOnce(context.Background(), repo, 7, 30, 200)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, deleted, int64(5), "应至少删 5 行老 dead")

	for _, id := range oldIDs {
		e, _ := repo.Get(id)
		assert.Nil(t, e, "老 dead id=%d 应已被清理", id)
	}
	for _, id := range recentIDs {
		e, _ := repo.Get(id)
		require.NotNil(t, e, "新 dead id=%d 应保留", id)
	}
}

// TestCleanupOnce_LeavesPendingRowsAlone §D2 RED §3
//
// 清理不能误删 pending 行（pending 还在 relay 队列）
func TestCleanupOnce_LeavesPendingRowsAlone(t *testing.T) {
	repo := repository.NewInMemoryOutboxRepo()
	// 注入 5 条 pending（默认状态） + 回拨 30 天（构造"老 pending"，防被 sent/dead 时间判定误删）
	pendingIDs := make([]int64, 0, 5)
	for i := 0; i < 5; i++ {
		e := &repository.OutboxEvent{
			EventID:   "evt-cleanup-pending-" + string(rune('A'+i)) + "-" + time.Now().Format("150405.000000"),
			EventType: "message.created",
			Topic:     "chat-events",
			Payload:   []byte(`{"id":"x","type":"message.created"}`),
			CreatedAt: time.Now().AddDate(0, 0, -30), // 30 天前创建的 pending
		}
		require.NoError(t, repo.CreateInTx(nil, e))
		pendingIDs = append(pendingIDs, e.ID)
	}

	_, err := CleanupOnce(context.Background(), repo, 7, 30, 200)
	require.NoError(t, err)

	for _, id := range pendingIDs {
		e, _ := repo.Get(id)
		require.NotNil(t, e, "pending id=%d 不能被 cleanup 删除（pending 还在 relay 队列）", id)
		assert.Equal(t, repository.OutboxStatusPending, e.Status)
	}
}

// TestCleanupOnce_RespectsLimit §D2 RED §4
//
// limit=50 时单轮最多删 50 行（保护 DB 长事务）
// 100 行老 sent + limit=50 → 第一轮删 50，留 50
func TestCleanupOnce_RespectsLimit(t *testing.T) {
	repo := repository.NewInMemoryOutboxRepo()
	oldIDs := seedSent(t, repo, 100, 10)

	deleted, err := CleanupOnce(context.Background(), repo, 7, 30, 50)
	require.NoError(t, err)
	assert.Equal(t, int64(50), deleted, "limit=50 应只删 50 行；剩 50 行留下一轮")

	// 验证剩 50 行存在
	remaining := 0
	for _, id := range oldIDs {
		e, _ := repo.Get(id)
		if e != nil {
			remaining++
		}
	}
	assert.Equal(t, 50, remaining, "应剩 50 行老 sent")
}

// TestCleanupOnce_CallerWiringInMainGo §D2 GREEN caller-wiring：
// chat-svc main.go 必须有 cleanup ticker 启动代码（c.Outbox.CleanupEnabled 守卫下）
// 这是红线契约 — 删 ticker → outbox 行永远不清理，几月后百万行 → DB 撑爆。
//
// 方式：grep main.go 源码，确认 "CleanupOnce" 字符串 + "CleanupEnabled" 配置项。
func TestCleanupOnce_CallerWiringInMainGo(t *testing.T) {
	// chat-svc 根目录 main.go（相对 internal/outbox/ 向上 2 级）
	source, err := os.ReadFile("../../main.go")
	require.NoError(t, err, "读 chat-svc/main.go 失败")
	src := string(source)

	require.Contains(t, src, "CleanupOnce",
		"chat-svc main.go 必须调 outbox.CleanupOnce（caller 接线源头）")
	require.Contains(t, src, "CleanupEnabled",
		"chat-svc main.go 必须有 c.Outbox.CleanupEnabled 守卫（默认禁用 + 显式 yaml 启用）")
}