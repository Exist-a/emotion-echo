// migrations/a011_grant_daily_emotion_fused_v_test.go
//
// CL-07（后端切片）Phase 1 · 字面量契约钉：新融合视图必须授权给 analytics_reader。
//
// 为什么单独一个迁移 + 一个钉子：本仓有"GRANT 缺失 ⇒ analytics_reader 读不到视图"
// 的历史缺陷类（AGENTS §2.4 §契约 3；账本 A4）。新视图若漏授权，端到端报表会
// 在 analytics_reader 身份下 permission denied —— 本地跑不出来（应用层可能用特权
// 角色），只有契约测试/换角色才暴露。
//
// 复用同包既有 helper：stripSQLComments（004_security_test.go）。

package migrations

import (
	"os"
	"strings"
	"testing"
)

// TestA011_GrantDailyEmotionFusedV a011 必须把 daily_emotion_fused_v 授权给 analytics_reader。
func TestA011_GrantDailyEmotionFusedV(t *testing.T) {
	raw, err := os.ReadFile("a011_grant_daily_emotion_fused_v.sql")
	if err != nil {
		t.Fatalf("cannot read a011_grant_daily_emotion_fused_v.sql: %v\n"+
			"—— CL-07 新视图 daily_emotion_fused_v 必须授权给 analytics_reader（§契约 3）", err)
	}
	src := stripSQLComments(string(raw))

	if !strings.Contains(src, "GRANT SELECT ON emotion_echo_ai.daily_emotion_fused_v TO analytics_reader") {
		t.Errorf("a011 必须 `GRANT SELECT ON emotion_echo_ai.daily_emotion_fused_v TO analytics_reader`\n"+
			"（否则 analytics_reader 身份读报表会 permission denied；账本 A4 同型缺陷）")
	}

	// 授权目标必须是"只读 role"，不得误授给可写角色（最小权限）。
	if strings.Contains(src, "ALL PRIVILEGES") || strings.Contains(src, "GRANT INSERT") ||
		strings.Contains(src, "GRANT UPDATE") || strings.Contains(src, "GRANT DELETE") {
		t.Errorf("a011 只应 GRANT SELECT（analytics_reader 是只读 role，不得授写权限）")
	}
}
