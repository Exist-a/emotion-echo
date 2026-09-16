// migrations/a008_partition_idempotent_test.go
//
// a008 幂等守卫字面量断言（2026-09-16 dev 模式修复）
//
// 背景：a008 第一次跑按设计走 (CREATE TABLE _partitioned + INSERT + RENAME +
// COMMIT)。第二次跑时 RENAME 已完成, user_behavior_events 已指向 partitioned
// 表; "INSERT INTO _partitioned SELECT FROM user_behavior_events" 变成自递归,
// PG 报 "no partition of relation found" (a008 第 3 个 bug)。
//
// 修复策略：a008 开头加 DO $$ ... RAISE EXCEPTION 守卫, 如果 user_behavior_events
// 已是 partitioned table 就直接让整个脚本 set -eu 中止 (避免后续 DDL 在错误状态运行)。

package migrations

import (
	"os"
	"strings"
	"testing"
)

// TestA008_PartitionedTable_IdempotencyGuard §P2-R2-11 幂等守卫字面量断言
//
// a008 必须含幂等守卫: 当 user_behavior_events 已 partitioned 时, 抛 RAISE EXCEPTION
// 让事务回滚 (避免后续 SELECT 自递归 + DDL 副作用)。守卫可在事务 BEGIN 之后 —
// 关键语义是 RAISE EXCEPTION + 事务回滚, 不是位置。
func TestA008_PartitionedTable_IdempotencyGuard(t *testing.T) {
	srcBytes, err := os.ReadFile("a008_partition_user_behavior_events.sql")
	if err != nil {
		t.Skipf("cannot read a008: %v", err)
	}
	src := stripSQLComments(string(srcBytes))

	// 必须含 守卫三要素
	guardMarkers := []string{
		"DO $$",
		"pg_partitioned_table",
		"RAISE EXCEPTION",
	}
	if !containsAllOrdered(src, guardMarkers) {
		t.Errorf("a008 缺幂等守卫: 必须含 DO $$ + 检查 pg_partitioned_table + RAISE EXCEPTION\n" +
			"—— §P2-R2-11 修复：第二次跑 a008 时 user_behavior_events 已指向 partitioned 表, \n" +
			"'SELECT FROM user_behavior_events' 变成自递归, PG 报 'no partition of relation found'")
	}

	// 守卫必须在事务 BEGIN 之后（事务回滚语义要求）
	guardIdx := strings.Index(src, "DO $$")
	beginIdx := strings.Index(src, "BEGIN;")
	if guardIdx < 0 || beginIdx < 0 || guardIdx < beginIdx {
		t.Errorf("a008 幂等守卫应在事务 BEGIN 之后 (RAISE EXCEPTION 需要在事务内回滚, 否则 ON_ERROR_STOP=off 也会落到 COMMIT)\n"+
			"—— 守卫位置: %d, BEGIN 位置: %d", guardIdx, beginIdx)
	}

	// 必须含 ON_ERROR_STOP off (让 RAISE EXCEPTION 不阻塞 psql 继续到下一文件)
	if !strings.Contains(src, "ON_ERROR_STOP off") {
		t.Errorf("a008 缺 ON_ERROR_STOP off: 守卫命中时 RAISE EXCEPTION 需不阻塞 psql 继续到下一文件")
	}
}
