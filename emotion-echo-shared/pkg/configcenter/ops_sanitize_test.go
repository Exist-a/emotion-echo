package configcenter

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E-23 P1 · 账本 E2E-F-158：ops 配置的敏感字段保护缺失。
//
// 🔴 缺陷形态（保护机制与实际用法模型不匹配）：
//   `sensitivePrefixes`（本文件）只在 `isSensitiveDataId` 里生效，而后者
//   **仅被 PublishConfig 调用**（nacos_config.go:191）。包级文档也明说：
//   "GetConfig / ListenConfig 不做前缀过滤"。
//   而 E2E-23 D-32 决议的 ops 方案是**单 dataId 打包全部运营参数**
//   （`emotion-echo-<svc>.ops.yaml`）—— 于是往这个文件里写
//   `llm.api_key: sk-xxx` 时，dataId 是 `...ops.yaml`（不匹配任何前缀），
//   一次都不会被拦。
//
// 修法：在**内容层**再挡一道 —— ops 解析前把敏感 key 剔除。
// 这样即便有人绕过 PublishConfig 直接往 Nacos 塞，读取侧也不会应用。
//
// 本测试是负向断言的核心：证明"敏感 key 进不来、普通 key 照常通过"。

func TestSanitizeOpsContent_RejectsSensitiveKeys(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		content     string
		wantDropped []string
		wantKept    []string
	}{
		{
			name: "llm api key is dropped",
			content: "limit_count: 100\n" +
				"burst: 20\n" +
				"llm.api_key: sk-should-never-be-here\n",
			wantDropped: []string{"llm.api_key"},
			wantKept:    []string{"limit_count", "burst"},
		},
		{
			name:        "postgres password is dropped",
			content:     "limit_count: 5\npostgres_password: hunter2\n",
			wantDropped: []string{"postgres_password"},
			wantKept:    []string{"limit_count"},
		},
		{
			name:        "token suffix is dropped",
			content:     "limit_count: 5\nauth_token: abc\n",
			wantDropped: []string{"auth_token"},
			wantKept:    []string{"limit_count"},
		},
		{
			name:        "datasource dsn is dropped",
			content:     "limit_count: 5\nmain_dsn: postgres://u:p@h/db\n",
			wantDropped: []string{"main_dsn"},
			wantKept:    []string{"limit_count"},
		},
		{
			name:        "nested sensitive key is dropped",
			content:     "limit_count: 5\ndb:\n  password: x\n",
			wantDropped: []string{"db.password"},
			wantKept:    []string{"limit_count"},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cleaned, dropped := SanitizeOpsContent(tt.content)

			for _, k := range tt.wantDropped {
				assert.Contains(t, dropped, k, "敏感 key 应被识别并剔除")
				assert.NotContains(t, cleaned, "sk-should-never-be-here",
					"清洗后不得残留敏感值原文")
			}
			for _, k := range tt.wantKept {
				assert.Contains(t, cleaned, k, "正常运营参数必须原样保留")
			}
			// 关键：敏感值本身绝不能出现在结果里
			for _, secret := range []string{"hunter2", "abc", "postgres://u:p@h/db"} {
				if strings.Contains(tt.content, secret) {
					assert.NotContains(t, cleaned, secret,
						"清洗后的内容里绝不能残留敏感值")
				}
			}
		})
	}
}

func TestSanitizeOpsContent_AllCleanCaseNoDropped(t *testing.T) {
	t.Parallel()

	content := "limit_count: 100\nburst: 20\nxtts_timeout_s: 60\n"
	cleaned, dropped := SanitizeOpsContent(content)

	assert.Empty(t, dropped, "无敏感 key 时不应报告任何剔除")
	assert.Equal(t, content, cleaned, "无敏感 key 时内容应原样返回（不做无谓改写）")
}

func TestSanitizeOpsContent_CommentsPreserved(t *testing.T) {
	t.Parallel()

	content := "# 运营参数\nlimit_count: 100\n"
	cleaned, dropped := SanitizeOpsContent(content)

	assert.Empty(t, dropped)
	assert.Contains(t, cleaned, "# 运营参数", "注释应保留（运维可读性）")
}

func TestIsSensitiveOpsKey(t *testing.T) {
	t.Parallel()

	require.True(t, IsSensitiveOpsKey("llm.api_key"), "llm. 前缀应判敏感")
	require.True(t, IsSensitiveOpsKey("LLM.API_KEY"), "大小写不敏感")
	require.True(t, IsSensitiveOpsKey("auth_token"), ".token 后缀应判敏感")
	require.True(t, IsSensitiveOpsKey("db.password"), "嵌套 key 逐段判定")
	require.False(t, IsSensitiveOpsKey("limit_count"), "普通运营参数不应误判")
	require.False(t, IsSensitiveOpsKey("xtts_timeout_s"), "超时参数不应误判")
	require.False(t, IsSensitiveOpsKey(""), "空 key 不应判敏感")
}
