// migrations/c010_trace_sw8_migration_test.go
//
// CL-03 Phase 1 字面量契约：outbox_events.trace_sw8 列的迁移与"双事实源"一致性。
//
// 背景（CL-03 实测）：relay 用后台根 ctx 发布（main.go 的 relayCtx）⇒
// kafka_publisher.go 的 CreateExitSpan 建的是「根」span，一条消息的 trace 在
// outbox→relay 处断掉。修法 = outbox 表存下写入时的 sw8，relay 发布前恢复父 ctx。
//
// 本测试钉死四条（对应 ADR-2026-09-chat-svc-table-deps「决策 22」§四 字段变更 checklist）：
//  1. 迁移文件存在且命名符合契约（c0NN_<desc>.sql）
//  2. 加列必须幂等（ADD COLUMN IF NOT EXISTS）且**可空**（历史行 / tracer=nil 场景）
//  3. 注释必须写明：变更原因 + 跨域影响 + 回滚方法（checklist 第 1 步）
//  4. 双事实源一致：main.go 的 runOutboxMigration 内联 DDL 不得漏列（否则
//     "内联先建表"的路径上会缺列 → relay 查询报错；checklist 第 8 步精神）
package migrations

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const c010MigrationFile = "c010_add_trace_sw8_to_outbox.sql"

// readMigration 读 chat-svc 的迁移文件；读不到直接 Fatal（不用 Skip —— 缺文件即缺陷）。
func readMigration(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(repoRoot(t), "emotion-echo-chat-svc", "migrations", name)
	bs, err := os.ReadFile(path)
	require.NoErrorf(t, err, "迁移文件缺失：%s", name)
	return string(bs)
}

// TestC010_TraceSw8Migration_ExistsAndNamed 契约：文件存在且命名合规。
func TestC010_TraceSw8Migration_ExistsAndNamed(t *testing.T) {
	src := readMigration(t, c010MigrationFile)
	assert.NotEmpty(t, src)
	assert.Regexp(t, `^c0\d\d_[a-z0-9_]+\.sql$`, c010MigrationFile,
		"迁移文件命名须符合 c0NN_<snake_case_desc>.sql（scripts/test_migrations_contract.sh 契约）")
}

// TestC010_TraceSw8Migration_IdempotentAddColumn 幂等 + 可空。
func TestC010_TraceSw8Migration_IdempotentAddColumn(t *testing.T) {
	src := stripSQLComments(readMigration(t, c010MigrationFile))

	assert.Contains(t, src, "emotion_echo_chat.outbox_events",
		"必须显式带 schema 名（chat-svc 表在 emotion_echo_chat schema 下）")
	assert.Contains(t, src, "ADD COLUMN IF NOT EXISTS",
		"加列必须 IF NOT EXISTS —— migrate.sh 按 checksum 跳过，但重放/新建环境需幂等")
	assert.Regexp(t, `(?i)ADD COLUMN IF NOT EXISTS\s+trace_sw8\s+(VARCHAR|TEXT)`, src,
		"列名必须是 trace_sw8，类型 VARCHAR/TEXT")

	// 必须可空：历史 outbox 行没有 trace 上下文；tracer=nil（dev / 单测）也不写。
	assert.NotRegexp(t, `(?i)trace_sw8\s+\S+\s+NOT NULL`, src,
		"trace_sw8 必须可空 —— 历史行与 tracer=nil 场景都不写该列")
}

// TestC010_TraceSw8Migration_CommentContract checklist 第 1 步：注释含原因/跨域影响/回滚。
func TestC010_TraceSw8Migration_CommentContract(t *testing.T) {
	src := readMigration(t, c010MigrationFile)

	for _, kw := range []string{"回滚", "跨域", "CL-03"} {
		assert.Containsf(t, src, kw,
			"迁移注释必须写明「变更原因 / 跨域影响 / 回滚方法」，缺关键字 %q", kw)
	}
}

// TestC010_TraceSw8_InlineDDLNotDrifted 双事实源一致性（R3）。
//
// chat-svc main.go 的 runOutboxMigration 内联了一份 outbox DDL。若它漏了 trace_sw8，
// 则"没有 db-migrate 容器、仅靠 svc 自建表"的路径上表会缺列。
func TestC010_TraceSw8_InlineDDLNotDrifted(t *testing.T) {
	path := filepath.Join(repoRoot(t), "emotion-echo-chat-svc", "main.go")
	bs, err := os.ReadFile(path)
	require.NoError(t, err, "读不到 chat-svc main.go")

	src := string(bs)
	idx := strings.Index(src, "func runOutboxMigration")
	require.GreaterOrEqual(t, idx, 0, "main.go 里找不到 runOutboxMigration")
	rest := src[idx:]
	if end := strings.Index(rest, "\nfunc "); end > 0 {
		rest = rest[:end]
	}

	assert.Contains(t, rest, "trace_sw8",
		"main.go 内联 outbox DDL 与 c010 迁移漂移了：内联版必须同样带 trace_sw8 列（或改为不再自建表）")
}

// TestC010_TraceSw8_ADRTableSynced checklist 第 9 步：ADR §二 表清单同步。
func TestC010_TraceSw8_ADRTableSynced(t *testing.T) {
	path := filepath.Join(repoRoot(t), "docs", "architecture", "adr",
		"adr-2026-09-chat-svc-table-deps.md")
	bs, err := os.ReadFile(path)
	require.NoError(t, err, "读不到 chat-svc 表依赖 ADR")

	assert.Contains(t, string(bs), "trace_sw8",
		"决策 22 §四 第 9 步要求同步本 ADR 的表字段清单：outbox_events 需登记 trace_sw8")
}
