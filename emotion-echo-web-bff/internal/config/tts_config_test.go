// Package config — tts_config_test.go
//
// E2E-F-198（docs/plans/tts-api-cosyvoice2-f198-2026-10-06.md §D T6）RED：
// TTS 配置段默认值 / env 覆盖 / secret 不进 yaml 三契约。
//
// 范式与纪律同 LLM 段（config.go 注释）：
//   - yaml 不写 APIKey（dev 留空 → provider=auto 直接走 XTTS，无需 key）
//   - 真 key 只进 gitignored .env.local（TTS_API_KEY），经 ApplyEnvOverrides 注入
package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestConfig_TTSDefaults 断言 TTS 段默认值（plan §C yaml 设计）。
func TestConfig_TTSDefaults(t *testing.T) {
	c := loadTestConfig(t)
	assert.Equal(t, "auto", c.TTS.Provider, "默认 auto：key 空→XTTS，key 有→cloud+回退")
	assert.Equal(t, "https://api.siliconflow.cn/v1", c.TTS.BaseURL)
	assert.Equal(t, "FunAudioLLM/CosyVoice2-0.5B", c.TTS.Model)
	assert.NotEmpty(t, c.TTS.Voice, "默认预置音色（官方约定带模型名前缀）")
	assert.Contains(t, c.TTS.Voice, c.TTS.Model, "预置音色必须带模型名前缀（官方文档约定）")
	assert.Equal(t, 24000, c.TTS.SampleRate, "必须 24000 与 XTTS SAMPLE_RATE 对齐（plan §B.4，官方默认 44100 不能用）")
	assert.Equal(t, "wav", c.TTS.ResponseFormat, "必须 wav 容器（前端不建 WAV 头，plan §B.3）")
	assert.Equal(t, 30, c.TTS.Timeout, "秒；cloud 实测 33 字 ~1.5s，30s 宽裕")
	assert.Empty(t, c.TTS.APIKey, "yaml 加载后 key 必须为空——secret 只进 env")
}

// TestConfig_TTSSectionHasNoKeyField 钉住「secret 不进 yaml」：etc/web-bff.yaml
// 的 TTS 段内不得出现任何 Key 字段行（含历史漂移回归——BFF_JWT_SECRET 先例）。
func TestConfig_TTSSectionHasNoKeyField(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "etc", "web-bff.yaml"))
	require.NoError(t, err)

	inTTS := false
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(line, "TTS:") {
			inTTS = true
			continue
		}
		if inTTS {
			// 顶层键（无缩进且带冒号）= TTS 段结束
			if line != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.Contains(line, ":") {
				inTTS = false
				continue
			}
			if strings.Contains(trimmed, "Key") {
				t.Fatalf("TTS 段出现 key 字段行 %q——secret 只准进 env/gitignored .env.local（ADR-2026-10 §Decision.2）", trimmed)
			}
		}
	}
}

// TestConfig_ApplyEnvOverrides_TTSKeys env 注入覆盖（TTS_API_KEY 等五个变量）。
func TestConfig_ApplyEnvOverrides_TTSKeys(t *testing.T) {
	c := loadTestConfig(t)

	t.Setenv("TTS_PROVIDER", "cloud")
	t.Setenv("TTS_API_KEY", "sk-test-tts")
	t.Setenv("TTS_API_BASE_URL", "https://api.example-tts.cn/v1")
	t.Setenv("TTS_MODEL", "Example/TTS-1")
	t.Setenv("TTS_VOICE", "Example/TTS-1:bob")

	ApplyEnvOverrides(&c)

	assert.Equal(t, "cloud", c.TTS.Provider)
	assert.Equal(t, "sk-test-tts", c.TTS.APIKey)
	assert.Equal(t, "https://api.example-tts.cn/v1", c.TTS.BaseURL)
	assert.Equal(t, "Example/TTS-1", c.TTS.Model)
	assert.Equal(t, "Example/TTS-1:bob", c.TTS.Voice)
}

// TestConfig_ApplyEnvOverrides_TTSEmptyKeepsDefault 空 env 不覆盖（与全局纪律一致）。
func TestConfig_ApplyEnvOverrides_TTSEmptyKeepsDefault(t *testing.T) {
	c := loadTestConfig(t)
	t.Setenv("TTS_API_KEY", "")
	t.Setenv("TTS_PROVIDER", "")

	ApplyEnvOverrides(&c)

	assert.Equal(t, "auto", c.TTS.Provider)
	assert.Empty(t, c.TTS.APIKey)
}
