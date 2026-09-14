// migrations/008_p0r27_ddl_drift_test.go
//
// P0-R2-7 字面量断言：deploy/db 01 + 02 DDL 漂移修复锁定。
//
// 风险：原 deploy/db/01-create-schemas.sql 同时建表（与 02 重叠），02 的
//   richer 定义（pinned / intent 列、FK、event_id UNIQUE）被 CREATE TABLE IF
//   NOT EXISTS 静默跳过。initdb 顺序跑 01 → 02 → 03 → 04，最后视图建在错列
//   缺的表上。
//
// 修复（已落地）：
//   1. 01 仅 CREATE SCHEMA（删表定义块），表定义唯一源 = 02
//   2. 02 emotion_echo_chat.messages.conversation_id 加 FK + ON DELETE CASCADE
//      （之前 BIGINT NOT NULL，无 FK → 孤儿消息可存在）
//
// 本测试钉死两条：
//   - 01 不再 CREATE TABLE（仅 CREATE SCHEMA）
//   - 02 messages.conversation_id 含 REFERENCES ... ON DELETE CASCADE
package migrations

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot returns the absolute path of the Emotion-Echo repo root.
//
// migrations live under emotion-echo-chat-svc/migrations/, deploy/db/ is a
// sibling from repo root. We walk up to repo root by counting ".."s relative
// to the test file's location.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// file is emotion-echo-chat-svc/migrations/<file>.go
	// walk up 2 levels to repo root
	root := filepath.Join(filepath.Dir(file), "..", "..")
	abs, err := filepath.Abs(root)
	if err != nil {
		t.Fatalf("filepath.Abs failed: %v", err)
	}
	return abs
}

// stripSQLComments -- / /* */ 简化版，与 a004 测试同款，保持一致性。
func stripSQLComments(src string) string {
	var out strings.Builder
	inBlock := false
	for _, line := range strings.Split(src, "\n") {
		processed := line
		if !inBlock && strings.Contains(processed, "/*") {
			if idx := strings.Index(processed, "/*"); idx >= 0 {
				if end := strings.Index(processed[idx:], "*/"); end >= 0 {
					processed = processed[:idx] + processed[idx+end+2:]
				} else {
					processed = processed[:idx]
					inBlock = true
				}
			}
		} else if inBlock {
			if end := strings.Index(processed, "*/"); end >= 0 {
				processed = processed[end+2:]
				inBlock = false
			} else {
				processed = ""
			}
		}
		if idx := strings.Index(processed, "--"); idx >= 0 {
			processed = processed[:idx]
		}
		out.WriteString(processed)
		out.WriteByte('\n')
	}
	return out.String()
}

// TestDeployDB01_NoCreateTable_P0R2_7 P0-R2-7 字面量断言：
//
// deploy/db/01-create-schemas.sql 不应再含 CREATE TABLE（仅 CREATE SCHEMA）。
// 原因：01 与 02 同时建同名表 → 02 IF NOT EXISTS 空转 → 02 的 richer
// 定义（pinned / intent / FK / event_id UNIQUE）被静默跳过。
func TestDeployDB01_NoCreateTable_P0R2_7(t *testing.T) {
	path := filepath.Join(repoRoot(t), "deploy", "db", "01-create-schemas.sql")
	srcBytes, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("cannot read 01-create-schemas.sql: %v", err)
	}
	src := stripSQLComments(string(srcBytes))

	if strings.Contains(src, "CREATE TABLE") {
		t.Errorf("deploy/db/01-create-schemas.sql 仍含 CREATE TABLE —— P0-R2-7 修复要求：\n" +
			"01 仅 CREATE SCHEMA，建表定义统一收敛到 02-create-tables-in-schemas.sql\n" +
			"否则 01/02 顺序跑时 02 的 richer 定义被 IF NOT EXISTS 静默跳过")
	}
	// 正向：必须有 CREATE SCHEMA
	if !strings.Contains(src, "CREATE SCHEMA") {
		t.Error("deploy/db/01-create-schemas.sql 缺 CREATE SCHEMA —— 不应再含 CREATE TABLE")
	}
}

// TestDeployDB02_MessagesFK_P0R2_7 P0-R2-7 字面量断言：
//
// deploy/db/02-create-tables-in-schemas.sql 中 emotion_echo_chat.messages
// 表的 conversation_id 列必须包含 REFERENCES ... ON DELETE CASCADE。
//
// 风险：原 BIGINT NOT NULL 无 FK → conversation 被删时 messages 孤儿化，
// 报表端聚合跨 FK 缺失的孤儿消息，得双计数。
func TestDeployDB02_MessagesFK_P0R2_7(t *testing.T) {
	path := filepath.Join(repoRoot(t), "deploy", "db", "02-create-tables-in-schemas.sql")
	srcBytes, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("cannot read 02-create-tables-in-schemas.sql: %v", err)
	}
	src := stripSQLComments(string(srcBytes))

	// 找 messages 表的 CREATE TABLE 块
	idx := strings.Index(src, "emotion_echo_chat.messages")
	if idx < 0 {
		t.Fatal("deploy/db/02-create-tables-in-schemas.sql 缺 emotion_echo_chat.messages CREATE TABLE")
	}
	// 截取 messages 表块到下一个 CREATE TABLE 之前
	rest := src[idx:]
	next := strings.Index(rest[len("emotion_echo_chat.messages"):], "CREATE TABLE")
	if next > 0 {
		rest = rest[:len("emotion_echo_chat.messages")+next]
	}

	if !strings.Contains(rest, "conversation_id BIGINT NOT NULL REFERENCES") {
		t.Errorf("deploy/db/02 emotion_echo_chat.messages.conversation_id 缺 FK —— P0-R2-7 修复要求：\n" +
			"列定义必须含 REFERENCES emotion_echo_chat.conversations(id) ON DELETE CASCADE\n" +
			"否则 conversation 被删时 messages 孤儿化，报表双计数")
	}
	if !strings.Contains(rest, "ON DELETE CASCADE") {
		t.Errorf("deploy/db/02 emotion_echo_chat.messages.conversation_id 缺 ON DELETE CASCADE ——\n" +
			"必须 cascade，否则 conversations.id 删除后 messages 仍残留")
	}
}