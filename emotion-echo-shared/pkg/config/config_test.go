// Package config 提供 emotion-echo 各 Go svc 的统一配置加载（Stage 41 PR-0）。
//
// 行为契约（必须由测试钉死，与 go-zero conf 兼容）：
//
//   - Load(path, dst, defaults): 读取 yaml → 先填 defaults()（零值字段被填默认）
//     → 再 yaml.Unmarshal（yaml 显式值覆盖默认，slice 字段亦然）
//   - MustLoad(path, dst, defaults): 同 Load，失败即 panic
//   - LoadBytes(b, dst, defaults): 同 Load 但接收 []byte（供测试）
//   - 必须行为：yaml 缺失 → 用 defaults；yaml 显式 false → 覆盖默认 true（R3 反向）
//   - 必须行为：yaml 大写风格（`Name: emotion-echo-...`）能正确加载到小写 struct 字段（A2 假设）
//
// 与 go-zero conf 的差异（plan §四 4.1）：
//   - 不解析 `${VAR:-default}` 占位（语义不变，由 applyEnvOverrides 接管）
//   - 不校验"必填"字段（提供 Validate() 钩子，业务自行决定）
//   - 字段名大小写不敏感匹配（A2 假设，需 golden 测试验证）
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfig 是 golden 测试和 RED 测试共用的最小 struct。
// 字段全小写，验证 yaml 大写风格能正确映射（A2 假设）。
//
// 注意：不加 yaml tag（yaml.v3 默认行为 = 大小写不敏感匹配导出字段名）。
// 若加 `yaml:"-"` 表示"跳过此字段"，会让所有测试读到零值。
type TestConfig struct {
	Name string // 测试专用
	Host string // 0.0.0.0
	Port int    // 8888

	// bool 字段覆盖测试（R3 高风险）
	BoolTrueDefault  bool // 默认 true；yaml 显式 false 必须覆盖默认
	BoolFalseDefault bool // 默认 false；yaml 显式 true 必须覆盖默认
	BoolZeroDefault  bool // 默认 false；yaml 省略走默认

	// slice 字段（plan §二 2.1 slice 默认值 case）
	Brokers []string // 默认 ["chat-events"]；yaml 显式覆盖

	// 嵌套 struct（验证 yaml.v3 大小写映射对嵌套同样生效）
	Postgres struct {
		DSN          string
		MaxOpenConns int
	}
}

// TestLoad_DefaultsBeforeYaml 验证核心契约：defaults() 先于 yaml.Unmarshal 执行。
// 这条契约若写反（先 yaml 再 defaults），yaml 显式 false 会被默认 true 覆盖，触发 R3。
func TestLoad_DefaultsBeforeYaml(t *testing.T) {
	yamlContent := []byte(`
Port: 9000
BoolTrueDefault: false
BoolFalseDefault: true
Brokers:
  - "kafka-prod:9092"
`)
	var c TestConfig
	err := LoadBytes(yamlContent, &c, func() {
		c.Name = "test-svc"
		c.Host = "0.0.0.0"
		c.Port = 8888
		c.BoolTrueDefault = true // 默认 true；yaml 显式 false 必须覆盖
		c.BoolFalseDefault = false
		c.BoolZeroDefault = false
		c.Brokers = []string{"chat-events"}
		c.Postgres.DSN = "default-dsn"
		c.Postgres.MaxOpenConns = 10
	})
	require.NoError(t, err)

	assert.Equal(t, "test-svc", c.Name, "defaults() 填的字段保留")
	assert.Equal(t, "0.0.0.0", c.Host, "defaults() 填的字段保留")
	assert.Equal(t, 9000, c.Port, "yaml 显式值覆盖 default")
	assert.Equal(t, false, c.BoolTrueDefault, "R3 反向：yaml false 必须覆盖默认 true")
	assert.Equal(t, true, c.BoolFalseDefault, "yaml true 覆盖默认 false")
	assert.Equal(t, []string{"kafka-prod:9092"}, c.Brokers, "yaml slice 覆盖默认 slice")
	assert.Equal(t, "default-dsn", c.Postgres.DSN, "嵌套 struct 默认值保留")
	assert.Equal(t, 10, c.Postgres.MaxOpenConns, "嵌套 struct 默认值保留")
}

// TestLoad_DefaultsOnly 验证：yaml 完全不写某字段 → 走 defaults。
func TestLoad_DefaultsOnly(t *testing.T) {
	var c TestConfig
	err := LoadBytes([]byte(""), &c, func() {
		c.Name = "minimal-svc"
		c.Port = 7777
		c.BoolTrueDefault = true
	})
	require.NoError(t, err)
	assert.Equal(t, "minimal-svc", c.Name)
	assert.Equal(t, 7777, c.Port)
	assert.Equal(t, true, c.BoolTrueDefault, "yaml 缺省字段走 default")
	assert.Equal(t, false, c.BoolFalseDefault, "零值字段未被 defaults 覆盖 → 保持零值")
}

// TestLoad_CaseInsensitiveMapping 验证 A2 假设：yaml 大写风格能映射到小写 struct 字段。
// 6 份 etc/*.yaml 全是大写（如 `Name: emotion-echo-chat-svc`），本测试必须通过才能落地 PR-2~7。
func TestLoad_CaseInsensitiveMapping(t *testing.T) {
	yamlContent := []byte(`
Name: emotion-echo-chat-svc
Host: 0.0.0.0
Port: 8890
`)
	var c TestConfig
	err := LoadBytes(yamlContent, &c, func() {})
	require.NoError(t, err)
	assert.Equal(t, "emotion-echo-chat-svc", c.Name, "yaml 大写 Name 映射到 struct Name 字段")
	assert.Equal(t, "0.0.0.0", c.Host)
	assert.Equal(t, 8890, c.Port)
}

// TestLoad_NestedStructCaseInsensitive 验证嵌套 struct 也支持大小写不敏感。
func TestLoad_NestedStructCaseInsensitive(t *testing.T) {
	yamlContent := []byte(`
Postgres:
  DSN: "host=pg port=5432"
  MaxOpenConns: 20
`)
	var c TestConfig
	err := LoadBytes(yamlContent, &c, func() {})
	require.NoError(t, err)
	assert.Equal(t, "host=pg port=5432", c.Postgres.DSN)
	assert.Equal(t, 20, c.Postgres.MaxOpenConns)
}

// TestLoad_FileNotFound 验证 Load 文件缺失返回 error（不 panic）。
func TestLoad_FileNotFound(t *testing.T) {
	var c TestConfig
	err := Load("/nonexistent/path/config.yaml", &c, func() {})
	require.Error(t, err, "文件不存在应返回 error")
	assert.Contains(t, err.Error(), "config", "error 信息应包含路径提示")
}

// TestMustLoad_PanicOnError 验证 MustLoad 在加载失败时 panic。
func TestMustLoad_PanicOnError(t *testing.T) {
	defer func() {
		r := recover()
		require.NotNil(t, r, "MustLoad 失败应 panic")
	}()
	var c TestConfig
	MustLoad("/nonexistent/path/config.yaml", &c, func() {})
}

// TestLoad_InvalidYAMLSyntax 验证 yaml 语法错误返回 error。
func TestLoad_InvalidYAMLSyntax(t *testing.T) {
	bad := []byte("Name: : invalid\n  : : :\nPort: [unclosed")
	var c TestConfig
	err := LoadBytes(bad, &c, func() {})
	require.Error(t, err, "yaml 语法错误应返回 error")
}

// TestLoad_EmptyYamlWithDefaults 验证空 yaml 也能正常加载 defaults。
func TestLoad_EmptyYamlWithDefaults(t *testing.T) {
	var c TestConfig
	err := LoadBytes(nil, &c, func() {
		c.Name = "empty-yaml-svc"
		c.Port = 1234
	})
	require.NoError(t, err)
	assert.Equal(t, "empty-yaml-svc", c.Name)
	assert.Equal(t, 1234, c.Port)
}

// TestLoad_FromFile 验证 Load 能从磁盘读取 yaml（用 t.TempDir 写入临时文件）。
func TestLoad_FromFile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.yaml")
	content := []byte("Name: from-disk\nPort: 5555\n")
	require.NoError(t, os.WriteFile(cfgPath, content, 0o600))

	var c TestConfig
	err := Load(cfgPath, &c, func() {})
	require.NoError(t, err)
	assert.Equal(t, "from-disk", c.Name)
	assert.Equal(t, 5555, c.Port)
}

// TestLoad_CommentsIgnored 验证 yaml 注释行被正确忽略（与 etc/*.yaml 实际形态对齐）。
func TestLoad_CommentsIgnored(t *testing.T) {
	yamlContent := []byte(`# Stage 36-A1.2:容器内服务发现
# 外部依赖的 string 地址通过 env 注入 compose environment
Name: with-comments
# 注释行: Port 应仍取 default
Port: 6666
`)
	var c TestConfig
	err := LoadBytes(yamlContent, &c, func() {
		c.Port = 9999 // 默认值
	})
	require.NoError(t, err)
	assert.Equal(t, "with-comments", c.Name)
	assert.Equal(t, 6666, c.Port, "yaml 显式值覆盖 default")
}

// TestLoad_ErrorMessageContainsPath 验证 Load 失败时 error 信息对调试友好。
func TestLoad_ErrorMessageContainsPath(t *testing.T) {
	var c TestConfig
	err := Load("/some/missing/path.yaml", &c, func() {})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "/some/missing/path.yaml"),
		"error 信息应包含文件路径，便于排查")
}
