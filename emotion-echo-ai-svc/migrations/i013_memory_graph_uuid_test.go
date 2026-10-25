// migrations/i013_memory_graph_uuid_test.go
//
// CL-08d · RED：memory_episode 补 graph_uuid 列（字面量契约，先例 i011/i012）。
//
// 背景：Phase 4 的图摄取（graphiti add_episode）会返回 episode uuid，但 PG 没存；
// 删除闭环（T-16）需要按 uuid 走 Graphiti.remove_episode 把图上的情节摘掉，
// 否则"删了 PG 图上还在"（ADR 决策 3：图随删）。

package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestI013_MemoryGraphUUID_Contract(t *testing.T) {
	raw, err := os.ReadFile("i013_memory_graph_uuid.sql")
	if err != nil {
		t.Fatalf("cannot read i013_memory_graph_uuid.sql: %v\n"+
			"—— CL-08d 要求为 memory_episode 补 graph_uuid 列（图删除闭环）", err)
	}
	src := stripSQLComments(string(raw))

	if !strings.Contains(src, "ALTER TABLE emotion_echo_ai.memory_episode") {
		t.Errorf("i013 必须是 ALTER TABLE emotion_echo_ai.memory_episode（增量补列）")
	}
	for _, col := range []string{"graph_uuid"} {
		if !strings.Contains(src, col) {
			t.Errorf("i013 缺列 %q（remove_episode 的定位键）", col)
		}
	}
	norm := strings.Join(strings.Fields(src), " ")
	if !strings.Contains(norm, "IF NOT EXISTS") {
		t.Errorf("i013 必须幂等（ADD COLUMN IF NOT EXISTS）")
	}
}
