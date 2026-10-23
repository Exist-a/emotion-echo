// migrations/i012_create_memory_episode_entity_test.go
//
// CL-08c · Phase 1 · RED：情节（episode）与实体（entity）表的**字面量契约测试**（不连 DB）。
//
// 背景：CL-08a 已落地 memory_fact / memory_note / memory_extract_run（i011）；
// 本迁移补齐 ADR 决策 45「PG 为系统记录」缺的最后两块：
//   - memory_episode：情节 = 对来源（message / note）的一次"可重建原料"存档。
//     图丢了能从它重放重建（ADR 决策 8）；管理页删除以 PG 为准 ⇒ 必须先有它。
//   - memory_entity：实体注册表 = 图节点在 PG 的镜像（消解后的规范名）。
//     同一用户内同名实体只允许一行（消解锚点）；摘要随抽取刷新。
//
// 本测试钉死的契约：
//   - 归属 schema = emotion_echo_ai（ai-svc 拥有；跨 schema 被守卫禁止）
//   - 双时间轴：episode.valid_at（事件发生时间）NOT NULL + created_at（摄入时间）
//     —— 与 fact 的 valid_from/valid_to 同一族语义（Graphiti 双时间轴）
//   - 幂等：UNIQUE(source_kind, source_id) —— 一条来源至多一个 episode（与
//     memory_extract_run 的复合主键同键，Phase 2 抽取 worker 直接复用）
//   - 实体唯一：UNIQUE(user_id, name) —— 同用户同名实体只一行
//   - 软删除：status + deleted_at（PG 软删；图侧物理删除归 Phase 5）
//   - 可重跑：CREATE TABLE/INDEX IF NOT EXISTS
//
// 与 i010/i011 测试同款纪律：**先剥离 SQL 注释**再断言，防止注释里的关键词
// 伪装成契约（CL-F-17 假绿钉的教训）。

package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestI012_MemoryEpisodeEntity_Contract(t *testing.T) {
	raw, err := os.ReadFile("i012_create_memory_episode_entity.sql")
	if err != nil {
		t.Fatalf("cannot read i012_create_memory_episode_entity.sql: %v\n"+
			"—— CL-08c Phase 1 要求新增情节与实体表（PG 系统记录的最后两块）", err)
	}
	src := stripSQLComments(string(raw))

	// 1) 可重跑（compose up 每次都会跑迁移；migrate.sh 另有 checksum 追踪）
	for _, marker := range []string{
		"CREATE TABLE IF NOT EXISTS emotion_echo_ai.memory_episode",
		"CREATE TABLE IF NOT EXISTS emotion_echo_ai.memory_entity",
	} {
		if !strings.Contains(src, marker) {
			t.Errorf("i012 必须含 %q（幂等建表）", marker)
		}
	}

	// 2) memory_episode 必需列（来源引用 / 内容 / 双时间轴 / 软删）
	for _, col := range []string{
		"id", "user_id", "conversation_id", "source_kind", "source_id",
		"name", "content", "valid_at", "status", "deleted_at", "created_at",
	} {
		if !strings.Contains(src, col) {
			t.Errorf("memory_episode 缺列 %q（图重建与管理页删除都依赖它）", col)
		}
	}

	// 3) 双时间轴语义：valid_at NOT NULL（事件发生时间）；created_at（摄入时间）。
	//    空白归一化后比对（列对齐用的多空格不应影响契约判定）
	norm := strings.Join(strings.Fields(src), " ")
	if !strings.Contains(norm, "valid_at TIMESTAMPTZ NOT NULL") {
		t.Errorf("memory_episode.valid_at 必须是 TIMESTAMPTZ NOT NULL（事件发生时间；摄入时间另由 created_at 承载）")
	}

	// 4) 幂等：一条来源至多一个 episode（与 memory_extract_run 复合主键同键）
	if !strings.Contains(src, "CREATE UNIQUE INDEX IF NOT EXISTS") {
		t.Errorf("i012 必须有 CREATE UNIQUE INDEX IF NOT EXISTS（抽取幂等）")
	}
	for _, col := range []string{"source_kind", "source_id"} {
		if !strings.Contains(src, col) {
			t.Errorf("唯一索引缺 %q（幂等键 = 来源身份）", col)
		}
	}

	// 5) memory_entity 必需列（规范名 / 摘要 / 软删）
	for _, col := range []string{"id", "user_id", "name", "summary", "status", "deleted_at", "created_at", "updated_at"} {
		if !strings.Contains(src, col) {
			t.Errorf("memory_entity 缺列 %q（图节点镜像 / 管理页读取依赖它）", col)
		}
	}
	// 实体唯一锚点：同一用户内同名实体只允许一行（"我妈"/"母亲"的合并由
	// LLM 消解负责；PG 只保证消解结果不重复落行）
	if !strings.Contains(norm, "UNIQUE INDEX") || !strings.Contains(norm, "user_id, name") {
		t.Errorf("memory_entity 必须有 (user_id, name) 唯一索引（实体消解的落库锚点）")
	}

	// 6) 状态枚举必须被 CHECK 约束住（与写入端常量一致，抓「DDL 对了但写入用错值」；
	//    episode/entity 没有 superseded —— 情节不可变、实体只刷新摘要，生命周期只有 active/deleted）
	for _, v := range []string{"'active'", "'deleted'"} {
		if !strings.Contains(src, v) {
			t.Errorf("status 的 CHECK 必须包含 %s（软删二态）", v)
		}
	}

	// 7) 来源类型必须被 CHECK 约束住（与 i011 的 memory_fact/memory_extract_run 同口径）
	for _, v := range []string{"'message'", "'note'"} {
		if !strings.Contains(src, v) {
			t.Errorf("source_kind 的 CHECK 必须包含 %s", v)
		}
	}

	// 8) 用户隔离的查询路径必须走索引（注入/管理页/删除全部强制带 user_id）
	if !strings.Contains(src, "user_id") {
		t.Errorf("两张表都必须有 user_id 索引路径（记忆绝不能跨用户）")
	}
}
