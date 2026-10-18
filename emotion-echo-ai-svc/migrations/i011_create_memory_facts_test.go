// migrations/i011_create_memory_facts_test.go
//
// CL-08a · A1 · RED：记忆事实表的**字面量契约测试**（不连 DB）。
//
// 背景：跨会话记忆的系统记录落 PG（ADR 决策 45「PG 为系统记录、图为可重建索引」）。
// 本阶段（a 切片）只有 PG，没有图；表结构一旦写错，后续 c/d 的删除闭环与
// 图重建都会建立在不一致的记录上。
//
// 本测试钉死的契约：
//   - 归属 schema = emotion_echo_ai（ai-svc 拥有；跨 schema 被守卫禁止）
//   - 两张表：memory_fact（事实）/ memory_extract_run（抽取审计 + 幂等）
//   - 双时间轴：valid_from / valid_to（旧值不删除，只关区间）
//   - 软删除：deleted_at（用户裁定 D3：PG 软删）
//   - 幂等：UNIQUE(source_message_id, subject, predicate, object)
//   - 可重跑：CREATE TABLE/INDEX IF NOT EXISTS
//
// 与 i010 测试同款纪律：**先剥离 SQL 注释**再断言，防止注释里的关键词伪装成契约
// （CL-F-17 假绿钉的教训）。

package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestI011_MemoryFacts_Contract(t *testing.T) {
	raw, err := os.ReadFile("i011_create_memory_facts.sql")
	if err != nil {
		t.Fatalf("cannot read i011_create_memory_facts.sql: %v\n"+
			"—— CL-08a A1 要求新增记忆事实表（PG 系统记录）", err)
	}
	src := stripSQLComments(string(raw))

	// 1) 可重跑（compose up 每次都会跑迁移；migrate.sh 另有 checksum 追踪）
	for _, marker := range []string{
		"CREATE TABLE IF NOT EXISTS emotion_echo_ai.memory_fact",
		"CREATE TABLE IF NOT EXISTS emotion_echo_ai.memory_extract_run",
	} {
		if !strings.Contains(src, marker) {
			t.Errorf("i011 必须含 %q（幂等建表）", marker)
		}
	}

	// 2) memory_fact 必需列（双时间轴 / 来源 / 软删 / 状态）
	for _, col := range []string{
		"user_id", "conversation_id", "source_message_id",
		"subject", "predicate", "object", "fact_text", "confidence",
		"valid_from", "valid_to", "status", "deleted_at",
		"created_at", "updated_at",
	} {
		if !strings.Contains(src, col) {
			t.Errorf("memory_fact 缺列 %q（抽取/注入/删除闭环都依赖它）", col)
		}
	}

	// 3) 双时间轴语义：valid_from NOT NULL；valid_to 可空（NULL = 仍然有效）
	//    空白归一化后比对（列对齐用的多空格不应影响契约判定）
	norm := strings.Join(strings.Fields(src), " ")
	if !strings.Contains(norm, "valid_from TIMESTAMPTZ NOT NULL") {
		t.Errorf("memory_fact.valid_from 必须是 TIMESTAMPTZ NOT NULL（事实生效时间）")
	}
	if !strings.Contains(src, "valid_to") {
		t.Errorf("memory_fact 必须有 valid_to（NULL = 仍然有效；被取代时关区间而非删除）")
	}

	// 4) 幂等索引：同一条消息重复消费不得重复落库
	if !strings.Contains(src, "CREATE UNIQUE INDEX IF NOT EXISTS") {
		t.Errorf("i011 必须有 CREATE UNIQUE INDEX IF NOT EXISTS（抽取幂等）")
	}
	for _, col := range []string{"source_message_id", "subject", "predicate", "object"} {
		if !strings.Contains(src, col) {
			t.Errorf("唯一索引/事实列缺 %q（幂等键 = message_id + 三元组）", col)
		}
	}

	// 5) 状态枚举必须被 CHECK 约束住（与写入端常量一致，抓「DDL 对了但写入用错值」）
	for _, v := range []string{"'active'", "'superseded'", "'deleted'"} {
		if !strings.Contains(src, v) {
			t.Errorf("memory_fact.status 的 CHECK 必须包含 %s（双时间轴/软删的三态）", v)
		}
	}

	// 6) 抽取审计表：message_id 作幂等主键 + 失败可观测
	for _, col := range []string{"message_id", "status", "facts_count", "error"} {
		if !strings.Contains(src, col) {
			t.Errorf("memory_extract_run 缺列 %q（幂等登记 + 失败可观测）", col)
		}
	}
	if !strings.Contains(src, "PRIMARY KEY") {
		t.Errorf("memory_extract_run 需要主键（message_id 作幂等键）")
	}

	// 7) 用户隔离的查询路径必须走索引（注入/摘要按 user 取）
	if !strings.Contains(src, "user_id") {
		t.Errorf("memory_fact 必须有 user_id 索引路径（所有查询强制带 user_id）")
	}
}
