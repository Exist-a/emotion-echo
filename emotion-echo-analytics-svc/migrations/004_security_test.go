// migrations/004_security_test.go
//
// Stage 94 PR-5 §P0-9 字面量断言: analytics_reader role 不应再含 LOGIN PASSWORD
// 'CHANGE_ME_AT_DEPLOY' 硬编码默认密码(code-review-2026-09-14.md §P0-9)。
//
// 风险:原 CREATE ROLE analytics_reader LOGIN PASSWORD 'CHANGE_ME_AT_DEPLOY'
// 在生产环境若漏 env 替换,任何持有默认密码的攻击者可直接 psql 登录读所有 *_v
// 视图(对话/情绪/评估/用户行为事件),违反最小权限原则。
//
// 修复:role 改为 NOLOGIN(仅作 schema/GRANT 定义存在,应用层用 SET LOCAL ROLE
// analytics_reader 临时切换权限)。这是最干净的"非 login 角色"语义,
// 避免默认空密码同样风险。
//
// 测试策略:
//   1) 源码字面量断言:004 migration 不应再出现 LOGIN PASSWORD 默认值
//   2) 正向断言:必须用 NOLOGIN 关键字

package migrations

import (
	"os"
	"strings"
	"testing"
)

// TestAnalyticsReaderRole_HasNoHardcodedPassword §P0-9 字面量断言:
//
// migrations/004_create_analytics_reader_role.sql 不应再含 'LOGIN PASSWORD'
// + 默认字符串的组合。
func TestAnalyticsReaderRole_HasNoHardcodedPassword(t *testing.T) {
	srcBytes, err := os.ReadFile("004_create_analytics_reader_role.sql")
	if err != nil {
		t.Skipf("cannot read 004: %v", err)
	}
	// 剥离行注释(行首 --) 和块注释(简单版),避免 self-referential 误命中
	src := stripSQLComments(string(srcBytes))

	badPatterns := []string{
		// 原 §P0-9 bug 模式:硬编码 dev 密码
		`LOGIN PASSWORD 'CHANGE_ME_AT_DEPLOY'`,
		`LOGIN PASSWORD 'dev-password'`,
		`LOGIN PASSWORD 'change-me'`,
		`LOGIN PASSWORD 'password'`,
		`LOGIN PASSWORD 'default'`,
		// 任何 LOGIN + PASSWORD 组合都不应出现(role 不应可登录)
		`LOGIN PASSWORD`,
	}
	for _, bad := range badPatterns {
		if strings.Contains(src, bad) {
			t.Errorf("migrations/004 仍含 %q —— §P0-9 修复要求 role 改 NOLOGIN,\n"+
				"避免硬编码默认密码泄露攻击面", bad)
		}
	}

	// 正向断言:必须用 NOLOGIN(role 不可登录,仅作 schema/GRANT 定义)
	if !strings.Contains(src, "NOLOGIN") {
		t.Error("migrations/004 缺 NOLOGIN —— §P0-9 修复要求 role 改 NOLOGIN\n" +
			"(LOGIN 标志 + PASSWORD 同时设会让 PG 接受默认空密码,同样风险)")
	}
}

// TestAnalyticsReaderRole_DailyEmotionByModalityV_P0R2_6 P0-R2-6: 字面量断言
// analytics_reader role 持有 emotion_echo_ai.daily_emotion_by_modality_v 的
// SELECT 权限。
//
// 风险：原 a004 缺这条 GRANT → dashboard EmotionDistributionByModality 报表
// SELECT 该视图 → permission denied，整页 500（§契约 3 必抓 bug）。
// 修复：a004 第 54 行加 GRANT SELECT ON ... daily_emotion_by_modality_v ...
// 本测试钉死：以后重构此 migration 时必须保留这条 GRANT。
func TestAnalyticsReaderRole_DailyEmotionByModalityV_P0R2_6(t *testing.T) {
	srcBytes, err := os.ReadFile("a004_create_analytics_reader_role.sql")
	if err != nil {
		t.Skipf("cannot read a004: %v", err)
	}
	src := stripSQLComments(string(srcBytes))

	// 正向断言：a004 必须对 daily_emotion_by_modality_v 显式 GRANT SELECT
	if !strings.Contains(src, "emotion_echo_ai.daily_emotion_by_modality_v") {
		t.Error("a004_create_analytics_reader_role.sql 缺 GRANT ON " +
			"emotion_echo_ai.daily_emotion_by_modality_v —— §P0-R2-6 必抓 bug：\n" +
			"dashboard EmotionDistributionByModality 必报 permission denied")
	}
	if !strings.Contains(src, "GRANT SELECT") {
		t.Error("a004_create_analytics_reader_role.sql 缺 GRANT SELECT 关键字")
	}
}

// stripSQLComments 剥离 SQL 行注释 (--) 与块注释 (/* ... */),
// 简化版用于字面量断言避免 self-referential 误命中。
func stripSQLComments(src string) string {
	var out strings.Builder
	inBlock := false
	for _, line := range strings.Split(src, "\n") {
		processed := line
		// 块注释处理:这一行含 /* ... */,先剥
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
		// 行注释
		if idx := strings.Index(processed, "--"); idx >= 0 {
			processed = processed[:idx]
		}
		out.WriteString(processed)
		out.WriteByte('\n')
	}
	return out.String()
}