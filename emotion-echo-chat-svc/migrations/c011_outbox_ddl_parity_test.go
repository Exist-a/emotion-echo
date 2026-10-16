// migrations/c011_outbox_ddl_parity_test.go
//
// CL-F-22：outbox_events 的**双事实源**（migrations/*.sql vs chat-svc main.go 内联 DDL）
// 必须**逐项等价** —— 不只列，**索引也算**。
//
// 现场（2026-10-16 建档实读，账本 CL-F-22）：
//   · `migrations/c001_create_outbox_events.sql` 建了 **2** 个索引
//     （`idx_outbox_pending` + `idx_outbox_attempts`）；
//   · `emotion-echo-chat-svc/main.go` 的 `runOutboxMigration` 内联 DDL **只有 1** 个
//     （缺 `idx_outbox_attempts`）。
//   ⇒ 哪份生效取决于有没有跑过 `db-migrate` 容器：**自建表路径上少了那个索引**。
//
// 既有守卫 `c010_trace_sw8_migration_test.go` 只钉「内联版不得漏**列**」，
// 索引漂移不在覆盖内 ⇒ 本次把守卫从"列"扩到"**索引集合双向相等**"。
//
// 为什么是"集合相等"而不是"包含"：包含式断言挡不住"迁移里删了索引、内联还在"
// 的反向漂移（那种情况下两边的**优化器行为**又不一样了）。
package migrations

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var indexNameRe = regexp.MustCompile(`(?i)CREATE\s+(?:UNIQUE\s+)?INDEX\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z0-9_]+)`)

// readInlineOutboxDDL 取 main.go 里 runOutboxMigration 函数体内的内联 DDL 文本。
func readInlineOutboxDDL(t *testing.T) string {
	t.Helper()
	path := filepath.Join(repoRoot(t), "emotion-echo-chat-svc", "main.go")
	bs, err := os.ReadFile(path)
	require.NoError(t, err, "读不到 chat-svc main.go")
	src := string(bs)

	idx := strings.Index(src, "func runOutboxMigration")
	require.GreaterOrEqual(t, idx, 0, "main.go 里找不到 runOutboxMigration")
	body := src[idx:]
	if end := strings.Index(body, "\nfunc "); end > 0 {
		body = body[:end]
	}
	return body
}

// outboxMigrationIndexNames 汇总**所有**触及 outbox_events 的迁移文件里的索引名。
//
// 扫 `migrations/c*.sql` 而不是硬编码 c001 —— 后续新增迁移（如给 outbox 补索引）
// 会自动纳入比对，避免"守卫只认某一份文件"的假绿。
func outboxMigrationIndexNames(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join(repoRoot(t), "emotion-echo-chat-svc", "migrations")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err, "读不到 migrations 目录")

	found := map[string]bool{}
	scanned := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		bs, err := os.ReadFile(filepath.Join(dir, name))
		require.NoError(t, err)
		text := stripSQLComments(string(bs))
		if !strings.Contains(text, "outbox_events") {
			continue
		}
		scanned++
		for _, m := range indexNameRe.FindAllStringSubmatch(text, -1) {
			found[strings.ToLower(m[1])] = true
		}
	}
	require.Greaterf(t, scanned, 0, "没有任何迁移文件提到 outbox_events —— 目录/命名变了？请同步本钉")

	out := make([]string, 0, len(found))
	for k := range found {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TestOutbox_InlineDDL_IndexParity 索引集合双向相等（CL-F-22）。
//
// 回退实现（把内联 DDL 里的 idx_outbox_attempts 删掉）会让本钉如期变红。
func TestOutbox_InlineDDL_IndexParity(t *testing.T) {
	inline := readInlineOutboxDDL(t)

	found := map[string]bool{}
	for _, m := range indexNameRe.FindAllStringSubmatch(inline, -1) {
		found[strings.ToLower(m[1])] = true
	}
	inlineIdx := make([]string, 0, len(found))
	for k := range found {
		inlineIdx = append(inlineIdx, k)
	}
	sort.Strings(inlineIdx)

	migIdx := outboxMigrationIndexNames(t)

	assert.Equalf(t, migIdx, inlineIdx,
		"双事实源索引漂移：migrations=%v 内联=%v —— 两侧必须逐项一致"+
			"（缺索引 ⇒ 自建表路径的查询计划与迁移路径不同；多索引 ⇒ 反向漂移）",
		migIdx, inlineIdx)
}

// TestOutbox_InlineDDL_ColumnParity 列集合双向相等（把 c010 的"只查 trace_sw8"扩成全列比对）。
func TestOutbox_InlineDDL_ColumnParity(t *testing.T) {
	inline := readInlineOutboxDDL(t)
	inlineCols := outboxColumnsFromCreateTable(inline)
	inlineCols = appendUnique(inlineCols, addColumnNames(inline)...)

	// 迁移侧：c001 的 CREATE TABLE + 后续迁移的 ADD COLUMN
	dir := filepath.Join(repoRoot(t), "emotion-echo-chat-svc", "migrations")
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)

	migCols := []string{}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		bs, err := os.ReadFile(filepath.Join(dir, e.Name()))
		require.NoError(t, err)
		text := stripSQLComments(string(bs))
		if !strings.Contains(text, "outbox_events") {
			continue
		}
		migCols = appendUnique(migCols, outboxColumnsFromCreateTable(text)...)
		migCols = appendUnique(migCols, addColumnNames(text)...)
	}
	sort.Strings(migCols)
	sort.Strings(inlineCols)

	assert.Equalf(t, migCols, inlineCols,
		"双事实源列漂移：migrations=%v 内联=%v", migCols, inlineCols)
}

// outboxColumnsFromCreateTable 从 `CREATE TABLE ... outbox_events ( ... )` 里取列名。
func outboxColumnsFromCreateTable(sql string) []string {
	re := regexp.MustCompile(`(?is)CREATE\s+TABLE\s+(?:IF\s+NOT\s+EXISTS\s+)?\S*outbox_events\s*\((.*?)\)\s*;`)
	m := re.FindStringSubmatch(sql)
	if m == nil {
		return nil
	}
	var cols []string
	for _, line := range strings.Split(m[1], "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") ||
			strings.HasPrefix(strings.ToUpper(line), "PRIMARY KEY") ||
			strings.HasPrefix(strings.ToUpper(line), "UNIQUE") ||
			strings.HasPrefix(strings.ToUpper(line), "CONSTRAINT") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			cols = append(cols, strings.ToLower(strings.Trim(fields[0], ",\"")))
		}
	}
	return cols
}

var addColumnRe = regexp.MustCompile(`(?i)ADD\s+COLUMN\s+(?:IF\s+NOT\s+EXISTS\s+)?([a-z0-9_]+)`)

func addColumnNames(sql string) []string {
	var out []string
	for _, m := range addColumnRe.FindAllStringSubmatch(sql, -1) {
		out = append(out, strings.ToLower(m[1]))
	}
	return out
}

func appendUnique(dst []string, items ...string) []string {
	seen := map[string]bool{}
	for _, d := range dst {
		seen[d] = true
	}
	for _, it := range items {
		if !seen[it] {
			seen[it] = true
			dst = append(dst, it)
		}
	}
	return dst
}
