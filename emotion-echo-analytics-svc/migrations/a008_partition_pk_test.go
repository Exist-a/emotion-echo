// migrations/a008_partition_pk_test.go
//
// a008 分区表 PK bug 字面量断言（P2-R2-11 关联）
//
// 背景：a008_partition_user_behavior_events.sql 把 user_behavior_events 改为
// 按 occurred_at 月分区，但用 `LIKE ... INCLUDING ALL` 复制原表结构时把
// 原 PK (id BIGSERIAL) 也复制过去。PG 在分区表上要求 PRIMARY KEY 必须包含
// 分区键（occurred_at），否则 RENAME 交换表名后 INSERT/SELECT 路径报：
//   "unique constraint on partitioned table must include all partitioning columns"
// 直接阻塞整个 migrate.sh（set -eu 中止），导致后续 a009 及任何下游 migration
// 全部跳过。
//
// 修复策略：a008 必须显式 DROP 继承的 PK，再 ADD 含 (id, occurred_at) 的复合 PK。
// 本测试钉住："以后重构 a008 时必须保留这一步"。
//
// 关联决策：决策 18 §P2-R2-11（user_behavior_events 月分区）
// 关联文档：emotion-echo-analytics-svc/migrations/a008_partition_user_behavior_events.sql

package migrations

import (
	"os"
	"strings"
	"testing"
)

// TestA008_PartitionTable_PKIncludesPartitionKey §P2-R2-11 字面量断言
//
// a008 把 user_behavior_events 改为按 occurred_at 月分区。PG 强制要求：
//   1) 分区表 PRIMARY KEY 必须包含分区键 occurred_at
//   2) 分区表 UNIQUE 约束（如 event_id）必须包含分区键 occurred_at
// 否则 INSERT/SELECT 路径报：
//   "unique constraint on partitioned table must include all partitioning columns"
//
// 修复策略：a008 不用 LIKE INCLUDING ALL（会复制原表 PK），改用显式列定义 +
// 显式 ADD PRIMARY KEY (id, occurred_at) + UNIQUE (event_id, occurred_at)。
func TestA008_PartitionTable_PKIncludesPartitionKey(t *testing.T) {
	srcBytes, err := os.ReadFile("a008_partition_user_behavior_events.sql")
	if err != nil {
		t.Skipf("cannot read a008: %v", err)
	}
	src := stripSQLComments(string(srcBytes))

	// 反向断言：不能用 LIKE INCLUDING ALL（会复制原表 PK 触发 PG 拒绝）
	if strings.Contains(src, "INCLUDING ALL") {
		t.Errorf("a008 含 'INCLUDING ALL' —— §P2-R2-11 修复：\n" +
			"LIKE INCLUDING ALL 会复制原表 PK (id BIGSERIAL)，分区表 PK 必须含分区键 occurred_at，\n" +
			"PG 会在 PARTITION BY 子句处拒绝建表。改用显式列定义 + 显式 ADD PRIMARY KEY (id, occurred_at)。")
	}

	// 正向断言 1：必须显式 ADD PRIMARY KEY 含 occurred_at
	addPKMarkers := []string{
		"ADD PRIMARY KEY", "id", "occurred_at",
	}
	if !containsAllOrdered(src, addPKMarkers) {
		t.Errorf("a008 缺 'ADD PRIMARY KEY (id, occurred_at)' —— §P2-R2-11 修复：分区表 PK 必须含分区键 occurred_at")
	}

	// 正向断言 2：event_id UNIQUE 约束也必须含 occurred_at
	uniqueEventIDMarkers := []string{
		"UNIQUE", "event_id", "occurred_at",
	}
	if !containsAllOrdered(src, uniqueEventIDMarkers) {
		t.Errorf("a008 缺 UNIQUE (event_id, occurred_at) —— §P2-R2-11 修复：\n" +
			"分区表 event_id 唯一约束同样必须含分区键 occurred_at，否则 INSERT 报同样的错。")
	}
}

// containsAllOrdered 检查 src 是否按顺序包含所有 markers（允许中间有其它字符）
func containsAllOrdered(src string, markers []string) bool {
	idx := 0
	for _, m := range markers {
		pos := strings.Index(src[idx:], m)
		if pos < 0 {
			return false
		}
		idx += pos + len(m)
	}
	return true
}

// TestA008_PartitionTable_AllColumnsPresent §P2-R2-11 字面量断言
//
// a008 显式列定义必须与 deploy/db/02-create-tables-in-schemas.sql:215 对齐，
// 包含全部 10 列。原表已由 a006 ADD event_id + 02-create-tables 加
// properties/ip/user_agent，a008 漏列会导致 INSERT ... SELECT 报
// "INSERT has more expressions than target columns"，阻塞 migrate。
func TestA008_PartitionTable_AllColumnsPresent(t *testing.T) {
	srcBytes, err := os.ReadFile("a008_partition_user_behavior_events.sql")
	if err != nil {
		t.Skipf("cannot read a008: %v", err)
	}
	src := stripSQLComments(string(srcBytes))

	requiredCols := []string{
		"id", "user_id", "event_type", "target", "properties",
		"session_id", "ip", "user_agent", "occurred_at", "event_id",
	}
	for _, col := range requiredCols {
		if !strings.Contains(src, col) {
			t.Errorf("a008 缺列 '%s' —— §P2-R2-11 修复：\n"+
				"a008 显式列定义必须与 deploy/db/02-create-tables-in-schemas.sql:215 对齐（10 列）\n"+
				"漏列导致 INSERT ... SELECT 'INSERT has more expressions than target columns'", col)
		}
	}

	// INSERT 必须用显式列名（防御后续列顺序漂移再次触发不匹配）
	if !strings.Contains(src, "INSERT INTO emotion_echo_analytics.user_behavior_events_partitioned") ||
		!containsAllOrdered(src, []string{"INSERT INTO emotion_echo_analytics.user_behavior_events_partitioned",
			"id", "user_id", "event_type", "target", "properties", "session_id", "ip",
			"user_agent", "occurred_at", "event_id", "SELECT"}) {
		t.Errorf("a008 的 INSERT 必须显式列出全部 10 列（防御列顺序漂移）")
	}
}
