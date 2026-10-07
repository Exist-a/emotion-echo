// Package metrics — E2E-F-198 T5（docs/plans/tts-api-cosyvoice2-f198-2026-10-06.md §D）
//
// TTS provider 降级计数 collector。与 fusion_metrics.go 同范式：service 专属
// 指标集中放 shared/pkg/metrics，避免 shared → 业务包反向依赖。
//
// 1 个 collector：
//   - TTSFallbackTotal: CounterVec{from: cloud}
//     cloud TTS 失败降级 local 的次数（OnFallback 钩子递增）——
//     持续上涨 = cloud 上游持续不可用，值得告警（E2E-22 静默失效教训）
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// TTSFallbackTotal from label 合法值。
const (
	TTSFallbackFromCloud = "cloud"
)

// TTSFallbackTotal cloud TTS 降级 local 计数。
var TTSFallbackTotal = promauto.NewCounterVec(
	prometheus.CounterOpts{
		Name: "emotion_echo_tts_fallback_total",
		Help: "Total number of TTS cloud-to-local degradations, labeled by source provider.",
	},
	[]string{"from"},
)
