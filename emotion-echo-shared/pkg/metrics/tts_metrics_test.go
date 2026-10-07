// Package metrics — E2E-F-198 T5（docs/plans/tts-api-cosyvoice2-f198-2026-10-06.md §D）
//
// TTS cloud→local 降级计数器：plan §B.5 可观测硬要求——
// "云端一直挂但没人知道" = E2E-22 静默失效同型，必须计数进 Prometheus。
package metrics

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTTSFallbackTotal_IncrementVisibleInRegistry(t *testing.T) {
	before, err := RegistryGatherCounter("emotion_echo_tts_fallback_total", map[string]string{"from": "cloud"})
	require.NoError(t, err)

	TTSFallbackTotal.WithLabelValues("cloud").Inc()

	after, err := RegistryGatherCounter("emotion_echo_tts_fallback_total", map[string]string{"from": "cloud"})
	require.NoError(t, err)
	assert.Equal(t, before+1, after, "Inc 后 registry 必须可见 +1（相对断言，-count=N 重跑亦成立）")
}
