// Package config — config_test.go
//
// Stage 30 / stage-30-web-bff.md T1.1 RED:
// 断言 config yaml parsing 正确加载默认值 + 下游地址合法。
//
// 跑：go test ./internal/config/...
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sharedconfig "github.com/emotion-echo/shared/pkg/config"

	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"
)

// loadTestConfig 从 etc/web-bff.yaml 加载（测试用）
func loadTestConfig(t *testing.T) Config {
	t.Helper()
	var c Config
	err := sharedconfig.Load(filepath.Join("..", "..", "etc", "web-bff.yaml"), &c, func() { SetDefaults(&c) })
	require.NoError(t, err, "web-bff.yaml 应可被 shared config 加载")
	return c
}

// TestConfig_YamlParsing_Port8894 断言 Port=8894（BFF 服务端口）
func TestConfig_YamlParsing_Port8894(t *testing.T) {
	c := loadTestConfig(t)
	assert.Equal(t, 8894, c.Port, "BFF 端口应 8894")
	assert.Equal(t, "0.0.0.0", c.Host)
	assert.Equal(t, shareddiscovery.ServiceWebBFF, c.Name)
}

// TestConfig_DownstreamDefaults_Valid 断言 5 个下游默认地址非空且端口合法
func TestConfig_DownstreamDefaults_Valid(t *testing.T) {
	c := loadTestConfig(t)

	// 5 个下游 HTTP
	assert.NotEmpty(t, c.UserService.BaseURL, "user-svc BaseURL 应有默认值")
	assert.NotEmpty(t, c.ChatService.BaseURL, "chat-svc BaseURL 应有默认值")
	assert.NotEmpty(t, c.AssessmentService.BaseURL, "assessment-svc BaseURL 应有默认值")
	assert.NotEmpty(t, c.AnalyticsService.BaseURL, "analytics-svc BaseURL 应有默认值")

	// AI 服务双协议
	assert.NotEmpty(t, c.AIService.HTTPAddr, "ai-svc HTTP 地址应有默认值")
	assert.NotEmpty(t, c.AIService.GRPCAddr, "ai-svc gRPC 地址应有默认值")

	// XTTS
	assert.NotEmpty(t, c.XTTS.BaseURL, "XTTS BaseURL 应有默认值")

	// 超时默认值 > 0
	assert.Greater(t, c.UserService.TimeoutMs, 0)
	assert.Greater(t, c.AIService.TimeoutMs, 0)
	assert.Greater(t, c.Health.TimeoutMs, 0)
}

// TestConfig_YamlParsing_AIServiceGRPCAddr 断言 AIService.GRPCAddr 默认值合法
// （T1.1 文档要求：断言 AIService.GRPCAddr 默认值合法）
func TestConfig_YamlParsing_AIServiceGRPCAddr(t *testing.T) {
	c := loadTestConfig(t)
	assert.Equal(t, "localhost:8892", c.AIService.GRPCAddr, "默认 gRPC 地址应为 localhost:8892")
	assert.Equal(t, "http://localhost:8891", c.AIService.HTTPAddr, "默认 HTTP 地址应为 localhost:8891")
}

// TestConfig_AIServiceDefaultTimeoutIs30s 断言 AIService.TimeoutMs 默认 30000。
//
// E2E-F-115（2026-09-22 用户浏览器实测）：dev 模式下首次语音 → /voice/upload → BFF→ai-svc 5s
// DeadlineExceeded。原因 = ai.go:182 newAIHTTPClient 默认 5s + config.go:154-155 默认 5000。
// SenseVoice 转写冷路径（ffmpeg 解码 + 首次张量分配）≈5~15s，5s 必撞 504。
// 修法：默认 30000ms（30s）—— 既覆盖 ASR 冷启动、又给 image 留充足余量。
func TestConfig_AIServiceDefaultTimeoutIs30s(t *testing.T) {
	c := loadTestConfig(t)
	assert.Equal(t, 30000, c.AIService.TimeoutMs,
		"AIService.TimeoutMs 默认应为 30000ms（30s）以容纳 SenseVoice 冷启动转写（E2E-F-115）")
}

// TestConfig_ApplyEnvOverrides_OverridesDefaults T1.3 REFACTOR:
// 容器 env 注入后应覆盖 yaml 默认值（指向容器 DNS）。
func TestConfig_ApplyEnvOverrides_OverridesDefaults(t *testing.T) {
	c := loadTestConfig(t)

	t.Setenv("USER_SVC_URL", "http://emotion-echo-user-svc:8888")
	t.Setenv("CHAT_SVC_URL", "http://emotion-echo-chat-svc:8890")
	t.Setenv("ASSESSMENT_SVC_URL", "http://emotion-echo-assessment-svc:8889")
	t.Setenv("ANALYTICS_SVC_URL", "http://emotion-echo-analytics-svc:8904")
	t.Setenv("AI_SVC_HTTP_URL", "http://emotion-echo-ai-svc:8891")
	t.Setenv("AI_SVC_GRPC_ADDR", "emotion-echo-ai-svc:8892")
	t.Setenv("XTTS_BASE_URL", "http://emotion-echo-xtts:8003")
	t.Setenv("SKYWALKING_OAP_ADDR", "emotion-echo-sw-oap:11800")

	ApplyEnvOverrides(&c)

	assert.Equal(t, "http://emotion-echo-user-svc:8888", c.UserService.BaseURL)
	assert.Equal(t, "http://emotion-echo-chat-svc:8890", c.ChatService.BaseURL)
	assert.Equal(t, "http://emotion-echo-assessment-svc:8889", c.AssessmentService.BaseURL)
	assert.Equal(t, "http://emotion-echo-analytics-svc:8904", c.AnalyticsService.BaseURL)
	assert.Equal(t, "http://emotion-echo-ai-svc:8891", c.AIService.HTTPAddr)
	assert.Equal(t, "emotion-echo-ai-svc:8892", c.AIService.GRPCAddr)
	assert.Equal(t, "http://emotion-echo-xtts:8003", c.XTTS.BaseURL)
	assert.Equal(t, "emotion-echo-sw-oap:11800", c.SkyWalking.OAPAddr)
}

// TestConfig_ApplyEnvOverrides_EmptyEnv_KeepsDefault 空 env 不应覆盖默认值
func TestConfig_ApplyEnvOverrides_EmptyEnv_KeepsDefault(t *testing.T) {
	c := loadTestConfig(t)
	t.Setenv("USER_SVC_URL", "") // 空串视为未设置

	ApplyEnvOverrides(&c)

	assert.Equal(t, "http://localhost:8888", c.UserService.BaseURL, "空 env 不应覆盖默认值")
}

// TestConfig_ApplyEnvOverrides_SkyWalkingEnabled Stage 57 B3 修复：
// SKYWALKING_ENABLED=true 必须让 c.SkyWalking.Enabled 翻 true（防止 stage-36-A1.1
// "默认 false" 误关 dev 模式 SkyWalking 链路——批 1 漏掉，stage-57 OAP 实测空 service
// 才暴露）。
func TestConfig_ApplyEnvOverrides_SkyWalkingEnabled(t *testing.T) {
	c := loadTestConfig(t)
	t.Setenv("SKYWALKING_ENABLED", "true")

	ApplyEnvOverrides(&c)

	assert.True(t, c.SkyWalking.Enabled, "SKYWALKING_ENABLED=true 应让 SkyWalking.Enabled=true")
}

// TestConfig_ApplyEnvOverrides_SkyWalkingEnabled_Empty 兜底：空 env 不应翻 enabled。
func TestConfig_ApplyEnvOverrides_SkyWalkingEnabled_Empty(t *testing.T) {
	c := loadTestConfig(t)
	t.Setenv("SKYWALKING_ENABLED", "") // 空串视为未设置

	ApplyEnvOverrides(&c)

	assert.False(t, c.SkyWalking.Enabled, "空 env 不应让 SkyWalking.Enabled=true,保留 yaml 默认 false")
}

// --- Stage 63: BFF gRPC 接线启用（RED）---

// TestConfig_DownstreamGRPCAddr_Defaults 断言 4 个下游服务都有 gRPC 地址默认值。
// 这是 BFF→svc gRPC 接线的前提：config 层必须知道每个 svc 的 gRPC 监听地址。
func TestConfig_DownstreamGRPCAddr_Defaults(t *testing.T) {
	var c Config
	SetDefaults(&c)

	assert.Equal(t, "localhost:8887", c.UserService.GRPCAddr, "user-svc gRPC 默认 :8887")
	assert.Equal(t, "localhost:8892", c.ChatService.GRPCAddr, "chat-svc gRPC 默认 :8892")
	assert.Equal(t, "localhost:8886", c.AssessmentService.GRPCAddr, "assessment-svc gRPC 默认 :8886")
	assert.Equal(t, "localhost:8885", c.AnalyticsService.GRPCAddr, "analytics-svc gRPC 默认 :8885")
}

// TestConfig_ApplyEnvOverrides_GRPCAddr 断言容器 env 可覆盖 4 个下游 gRPC 地址。
func TestConfig_ApplyEnvOverrides_GRPCAddr(t *testing.T) {
	c := loadTestConfig(t)

	t.Setenv("USER_SVC_GRPC_ADDR", "emotion-echo-user-svc:8887")
	t.Setenv("CHAT_SVC_GRPC_ADDR", "emotion-echo-chat-svc:8892")
	t.Setenv("ASSESSMENT_SVC_GRPC_ADDR", "emotion-echo-assessment-svc:8886")
	t.Setenv("ANALYTICS_SVC_GRPC_ADDR", "emotion-echo-analytics-svc:8885")

	ApplyEnvOverrides(&c)

	assert.Equal(t, "emotion-echo-user-svc:8887", c.UserService.GRPCAddr)
	assert.Equal(t, "emotion-echo-chat-svc:8892", c.ChatService.GRPCAddr)
	assert.Equal(t, "emotion-echo-assessment-svc:8886", c.AssessmentService.GRPCAddr)
	assert.Equal(t, "emotion-echo-analytics-svc:8885", c.AnalyticsService.GRPCAddr)
}

// TestConfig_ApplyEnvOverrides_GRPCAddr_Empty 空 env 不覆盖默认值。
func TestConfig_ApplyEnvOverrides_GRPCAddr_Empty(t *testing.T) {
	var c Config
	SetDefaults(&c)
	t.Setenv("USER_SVC_GRPC_ADDR", "")

	ApplyEnvOverrides(&c)

	assert.Equal(t, "localhost:8887", c.UserService.GRPCAddr, "空 env 不应覆盖 gRPC 默认值")
}

// TestConfig_Transport_DefaultEmpty 断言 Transport 默认空串（下游工厂空串即 grpc）。
//
// 历史：Stage 63 收口期间曾临时改为 "http" 绕路 ctxkey 不通问题，Sprint C ctxkey 重构后
// 恢复默认空 → grpc（与决策 4 "内部 svc-to-svc = gRPC" 对齐）。
//
// Sprint D 修 chat-svc 6 RPC 之前，chat-svc gRPC 路径仍返 Unimplemented，但其他 3 个 svc
// （user/assessment/analytics）应能正常走 gRPC。
func TestConfig_Transport_DefaultEmpty(t *testing.T) {
	var c Config
	SetDefaults(&c)

	assert.Empty(t, c.UserService.Transport, "Transport 默认空 → 下游工厂按 grpc 处理")
	assert.Empty(t, c.ChatService.Transport)
	assert.Empty(t, c.AssessmentService.Transport)
	assert.Empty(t, c.AnalyticsService.Transport)
}

// TestConfig_ApplyEnvOverrides_Transport 断言 *_TRANSPORT env 可强制 http（回滚开关）。
func TestConfig_ApplyEnvOverrides_Transport(t *testing.T) {
	c := loadTestConfig(t)

	t.Setenv("USER_TRANSPORT", "http")
	t.Setenv("CHAT_TRANSPORT", "http")
	t.Setenv("ASSESSMENT_TRANSPORT", "http")
	t.Setenv("ANALYTICS_TRANSPORT", "http")

	ApplyEnvOverrides(&c)

	assert.Equal(t, "http", c.UserService.Transport)
	assert.Equal(t, "http", c.ChatService.Transport)
	assert.Equal(t, "http", c.AssessmentService.Transport)
	assert.Equal(t, "http", c.AnalyticsService.Transport)
}

// =====================================================
// Stage 94 PR-5 §P0-10 · BFF JWTSecret 硬编码默认修复测试
// =====================================================
//
// code-review-2026-09-14.md §P0-10 原文:"BFF JWTSecret 硬编码默认值
// dev-bff-secret"(0.5d) —— 认证密钥漏 env 注入时,所有 JWT 用 dev 密钥签 →
// 攻击者可伪造任意 user_id token。
//
// 修复目标:删默认值 + main.go 启动 fail-fast。

// TestSetDefaults_JWTSecret_HasNoHardcodedDefault §P0-10 字面量断言:
//
// config.go SetDefaults 内不应给 c.Auth.JWTSecret 设任何默认值(尤其
// "dev-bff-secret" / "change-me" 等弱密钥)。
func TestSetDefaults_JWTSecret_HasNoHardcodedDefault(t *testing.T) {
	srcBytes, err := os.ReadFile("config.go")
	if err != nil {
		t.Skipf("cannot read config.go: %v", err)
	}
	src := string(srcBytes)

	badDefaults := []string{
		`JWTSecret = "dev-bff-secret"`,
		`JWTSecret = "change-me"`,
		`JWTSecret = "secret"`,
		`JWTSecret = "default"`,
		`JWTSecret = "dev-secret"`,
	}
	for _, bad := range badDefaults {
		if strings.Contains(src, bad) {
			t.Errorf("config.go SetDefaults 仍含硬编码默认值 %q —— §P0-10 修复要求删除默认值,\n"+
				"生产环境漏 env 注入时不应有任何兜底密钥(应让 main.go fail-fast)", bad)
		}
	}

	// 必须保留空值检测(if c.Auth.JWTSecret == ""),让 main.go 知道缺
	if !strings.Contains(src, `c.Auth.JWTSecret == ""`) {
		t.Error("config.go 缺空值检测 —— 删默认值后必须保留检测,让 main.go 等上层\n" +
			"知道 JWTSecret 未配置 → fail-fast")
	}
}

// TestWebBffYaml_JWTSecret_HasNoInsecureDefault §P0-10 字面量断言:
//
// etc/web-bff.yaml 不应再含默认 JWTSecret: dev-bff-secret。
func TestWebBffYaml_JWTSecret_HasNoInsecureDefault(t *testing.T) {
	srcBytes, err := os.ReadFile("../../etc/web-bff.yaml")
	if err != nil {
		t.Skipf("cannot read web-bff.yaml: %v", err)
	}
	// 只看 yaml code body（去掉注释）,避免 self-referential 误命中
	src := stripYAMLLines(string(srcBytes))

	badDefaults := []string{
		`JWTSecret: dev-bff-secret`,
		`JWTSecret: change-me`,
		`JWTSecret: secret`,
		`JWTSecret: default`,
	}
	for _, bad := range badDefaults {
		if strings.Contains(src, bad) {
			t.Errorf("web-bff.yaml 仍含默认密钥 %q —— §P0-10 修复要求删除默认值,\n"+
				"yaml 应只展示占位符或留空 + 注释引导 env 注入", bad)
		}
	}
}

// stripYAMLLines 移除 yaml 注释行（行首 #）以及尾部行内注释。
// 简化版:只处理行首 #,因为 dev-bff-secret 字面量只在注释或配置行出现,
// 而我们检查的是配置行 —— 行内 # 后面若误命中 JWTSecret dev-bff-secret,
// 配置行本身的 key/value 不该有 #,所以剥离行首 # 就够。
func stripYAMLLines(src string) string {
	var out strings.Builder
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		// 跳过纯注释行(以 # 开头) 和空行
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		out.WriteString(line)
		out.WriteByte('\n')
	}
	return out.String()
}

// TestMain_JWTSecret_RequiredAtStartup §P0-10 字面量断言:
//
// web-bff main.go 启动时必须校验 Auth.JWTSecret 非空 + log.Fatal,避免
// dev 密钥进生产。
func TestMain_JWTSecret_RequiredAtStartup(t *testing.T) {
	mainBytes, err := os.ReadFile("../../main.go")
	if err != nil {
		t.Skipf("cannot read web-bff/main.go: %v", err)
	}
	src := string(mainBytes)

	if !strings.Contains(src, `c.Auth.JWTSecret`) {
		t.Error("web-bff main.go 缺 c.Auth.JWTSecret 校验 —— §P0-10 修复要求启动时\n" +
			"校验 JWTSecret 非空,缺失则 log.Fatal")
	}
}
