// Package fusion — CL-01 T-06 / D-03：fusion 两指标接线回归钉。
//
// 背景（CL-F-04 同族）：`RecordFusionCall` / `ObserveFusionDuration` 定义齐全，
// 但**全仓 0 生产调用** ⇒ `emotion_fusion_calls_total` 无任何 series
//（"定义了没数据"）。本测试跑一次 worker.processOne，断言 counter 递增 ——
// **修前 RED**（series 不存在 / 不增长），接线后 GREEN。
//
// 注：不用 t.Parallel —— 与 fusion_metrics_test.go 的并行用例共用同一 series，
// 顺序执行可避免相互干扰（本测试在顺序阶段先跑）。
package fusion

import (
	"context"
	"testing"
	"time"

	"emotion-echo-ai-svc/internal/model"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFusionModalityLabel 低基数标签映射：单模态 → 其名；多模态 → "fused"。
// 取值限定在指标文档定义的 text/face/voice/fused 内（不改指标语义）。
func TestFusionModalityLabel(t *testing.T) {
	text := &ModalityScore{Emotion: "sad"}
	face := &ModalityScore{Emotion: "sad"}
	voice := &ModalityScore{Emotion: "sad"}

	cases := []struct {
		name string
		snap ModalitySnapshot
		want string
	}{
		{"only_text", ModalitySnapshot{Text: text}, "text"},
		{"only_face", ModalitySnapshot{Face: face}, "face"},
		{"only_voice", ModalitySnapshot{Voice: voice}, "voice"},
		{"text_face", ModalitySnapshot{Text: text, Face: face}, "fused"},
		{"all_three", ModalitySnapshot{Text: text, Face: face, Voice: voice}, "fused"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, fusionModalityLabel(tc.snap))
		})
	}
}

// TestProcessOne_RecordsFusionCallMetric processOne 成功融合 → 递增
// emotion_fusion_calls_total{modality="text",result="success"} + duration histogram 有样本。
func TestProcessOne_RecordsFusionCallMetric(t *testing.T) {
	textRepo := newFakeEmotionRepo()
	faceRepo := newFakeFaceRepo()
	voiceRepo := newFakeVoiceRepo()
	fusedRepo := newFakeFusedRepo()

	textRepo.put(100, &model.EmotionAnalysis{
		MessageID: 100, UserID: 7, ConversationID: 50,
		PrimaryEmotion: "sad", SentimentScore: -0.5, Confidence: 0.9, Model: "text-v1",
	})
	fusedRepo.data[100] = &model.FusedEmotion{MessageID: 100}

	llm := &fakeFuser{result: &model.FusedEmotion{
		MessageID: 100, UserID: 7, ConversationID: 50,
		PrimaryEmotion: "sad", FusionMethod: "llm",
	}}
	late := &fakeFuser{result: &model.FusedEmotion{
		MessageID: 100, UserID: 7, ConversationID: 50,
		PrimaryEmotion: "sad", FusionMethod: "late_fusion_weighted",
	}}

	w := NewFusionWorker(FusionWorkerDeps{
		EmotionRepo: textRepo, FaceEmotionRepo: faceRepo, VoiceEmotionRepo: voiceRepo,
		FusedEmotionRepo: fusedRepo, PendingLister: fusedRepo, LLMFuser: llm, LateFuser: late,
		TickInterval: 5 * time.Second,
	})

	labels := map[string]string{"modality": "text", "result": "success"}
	before := readFusionCounter(t, "emotion_fusion_calls_total", labels)
	histBefore := histogramSampleCount(t, "emotion_fusion_duration_seconds", "text")

	require.NoError(t, w.processOne(context.Background(), 100))

	after := readFusionCounter(t, "emotion_fusion_calls_total", labels)
	assert.Greater(t, after, before,
		"processOne 成功后必须递增 emotion_fusion_calls_total{modality=text,result=success}（修前 0 生产调用 ⇒ RED）")

	histAfter := histogramSampleCount(t, "emotion_fusion_duration_seconds", "text")
	assert.Greater(t, histAfter, histBefore,
		"processOne 必须给 emotion_fusion_duration_seconds{modality=text} 记一次观测")
}

// histogramSampleCount 返回指定 histogram 中 label modality=<modality> 的样本数（无则 0）。
func histogramSampleCount(t *testing.T, name, modality string) uint64 {
	t.Helper()
	mfs, err := prometheus.DefaultGatherer.Gather()
	require.NoError(t, err)
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			for _, lp := range m.GetLabel() {
				if lp.GetName() == "modality" && lp.GetValue() == modality {
					return m.GetHistogram().GetSampleCount()
				}
			}
		}
	}
	return 0
}
