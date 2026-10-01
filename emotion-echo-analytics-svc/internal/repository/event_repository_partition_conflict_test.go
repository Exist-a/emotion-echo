// event_repository_partition_conflict_test.go —— E2E-F-149 静态契约钉
//
// 背景：a008 分区表化后 user_behavior_events 的 UNIQUE 约束是
// (event_id, occurred_at)（分区表强制含分区键）。PostgresEventRepo.Create
// 的 ON CONFLICT 冲突目标必须包含 occurred_at，否则 42P10——每条消息
// 消费必失败（F-149，2026-10-01 运行时复现并修复）。
//
// 为什么用字面量契约测试而非行为测试：行为验证（真 PG）在
// integration_test/event_create_partition_integration_test.go（build tag
// integration，CI 不跑）。本钉保证 CI 上任何把冲突目标改回单列的改动
// 会立刻红（模式同 migrations/a008_partition_pk_test.go）。
package repository

import (
	"os"
	"strings"
	"testing"
)

func TestPostgresEventRepo_Create_ConflictTargetIncludesPartitionKey(t *testing.T) {
	srcBytes, err := os.ReadFile("event_repository.go")
	if err != nil {
		t.Fatalf("读不到 event_repository.go: %v", err)
	}
	src := string(srcBytes)

	// 定位 Create 方法体内的 OnConflict 子句
	start := strings.Index(src, "func (r *PostgresEventRepo) Create(")
	if start < 0 {
		t.Fatal("找不到 PostgresEventRepo.Create 方法")
	}
	end := strings.Index(src[start:], "\nfunc ")
	if end < 0 {
		end = len(src) - start
	}
	body := src[start : start+end]

	if !strings.Contains(body, `"event_id"`) {
		t.Fatal("Create 的 OnConflict 缺 event_id 列")
	}
	if !strings.Contains(body, `"occurred_at"`) {
		t.Fatal("Create 的 OnConflict 必须含分区键 occurred_at（分区表 UNIQUE = (event_id, occurred_at)，见 a008；缺它 ⇒ 42P10，F-149 复发）")
	}
}
