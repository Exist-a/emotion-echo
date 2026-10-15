// migrations/i010_create_daily_emotion_fused_v_test.go
//
// CL-07（后端切片）Phase 1 · RED：融合情绪日聚合视图的字面量契约钉。
//
// 背景：fused_emotions（三模态收敛后的综合情绪）此前**只写不读**——
// 唯一消费点是一个按消息查的端点，前端零调用；按模态视图
// （i005 daily_emotion_by_modality_v）**主动排除**了它。本视图补上
// "按 user × day × 情绪（+ 融合方法）" 的聚合，供报表消费。
//
// 本测试是**字面量契约测试**（不连 DB）：钉死 SQL 文件必须满足的契约，
// 防止后续编辑悄悄丢掉关键语义（deleted_at 过滤 / fusion_method 维度 /
// 不双计）。真实聚合正确性由集成测试 + 端到端数据契约实拍覆盖。

package migrations

import (
	"os"
	"strings"
	"testing"
)

// stripSQLComments 去掉 -- 行注释与 /* */ 块注释，避免注释里的关键词
// 伪装成契约（与 emotion-echo-chat-svc/migrations/008_p0r27_ddl_drift_test.go 同款）。
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
				continue
			}
		}
		if idx := strings.Index(processed, "--"); idx >= 0 {
			processed = processed[:idx]
		}
		out.WriteString(processed)
		out.WriteString("\n")
	}
	return out.String()
}

// containsAllOrdered 断言 markers 按顺序都出现在 src 中。
func containsAllOrdered(src string, markers []string) bool {
	pos := 0
	for _, m := range markers {
		idx := strings.Index(src[pos:], m)
		if idx < 0 {
			return false
		}
		pos += idx + len(m)
	}
	return true
}

// TestI010_DailyEmotionFusedView_Contract i010 视图字面量契约。
func TestI010_DailyEmotionFusedView_Contract(t *testing.T) {
	raw, err := os.ReadFile("i010_create_daily_emotion_fused_v.sql")
	if err != nil {
		t.Fatalf("cannot read i010_create_daily_emotion_fused_v.sql: %v\n"+
			"—— CL-07 Phase 1 要求新增融合情绪日聚合视图（fusion-result-consumption §2.1）", err)
	}
	src := stripSQLComments(string(raw))

	// 1) 幂等：CREATE OR REPLACE（migrate.sh 会重跑已应用文件之外的每次运行）
	if !strings.Contains(src, "CREATE OR REPLACE VIEW emotion_echo_ai.daily_emotion_fused_v") {
		t.Errorf("i010 必须 `CREATE OR REPLACE VIEW emotion_echo_ai.daily_emotion_fused_v`（幂等，可重复应用）")
	}

	// 2) 数据源 = fused_emotions（融合收敛结果，**不是**单模态表）
	if !strings.Contains(src, "emotion_echo_ai.fused_emotions") {
		t.Errorf("i010 必须从 emotion_echo_ai.fused_emotions 聚合")
	}

	// 3) 软删除过滤：i007 给 fused_emotions 加了 deleted_at，视图必须排除已删行
	if !strings.Contains(src, "deleted_at IS NULL") {
		t.Errorf("i010 必须 `WHERE deleted_at IS NULL`（i007 软删除列；否则已删除的融合行仍计入报表）")
	}

	// 4) 维度：GROUP BY 必须含 fusion_method（"多少比例真 LLM 融合 / 多少降级加权"是核心指标）
	if !strings.Contains(src, "GROUP BY") || !strings.Contains(src, "fusion_method") {
		t.Errorf("i010 必须按 fusion_method 分组（llm vs late_fusion_weighted 占比是融合消费的关键指标）")
	}

	// 5) 多模态占比：face_ratio / voice_ratio 从 available_modalities(JSONB) 判定，需 ::jsonb 转义
	if !strings.Contains(src, "'face'") || !strings.Contains(src, "'voice'") {
		t.Errorf("i010 必须输出 face_ratio / voice_ratio（回答「多模态到底用了多少」）")
	}
	if !strings.Contains(src, "::jsonb") {
		t.Errorf("i010 的 available_modalities 是 JSONB 字符串，`?` 操作符前需 `::jsonb` 转换（见 fusion 计划 §5 风险表）")
	}

	// 6) 不双计：本视图**只**聚合 fused（不与单模态 UNION，不与 emotion_analysis 混入）
	if strings.Contains(src, "UNION") {
		t.Errorf("i010 不应 UNION 单模态表 —— fused 是收敛结果，与单模态并列会双计（i005 注释同款纪律）")
	}
	if strings.Contains(src, "emotion_analysis") {
		t.Errorf("i010 不应直接读 emotion_analysis —— 融合视图只读 fused_emotions")
	}

	// 7) 契约列齐备（下游 analytics-svc repo 按列名解析）
	for _, col := range []string{"user_id", "primary_emotion", "cnt", "avg_sentiment", "avg_confidence"} {
		if !strings.Contains(src, col) {
			t.Errorf("i010 视图缺列/表达式 %q（analytics-svc FusedReportRepo 依赖）", col)
		}
	}
}
