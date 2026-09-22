// Package logic — multimodalanalyzelogic_test.go
//
// Sibling test for multimodalanalyzelogic.go (per AGENTS.md §1.1).
//
// Stage 26-T backlog §三 3.2: cover the missing multimodalanalyzelogic
// test surface. The implementation gates on (a) svcCtx.MultiModal != nil
// and (b) kind ∈ {image, audio, text}, then delegates to the embedded
// analyzer. We construct a real *analyzer.MultiModalAnalyzer with the
// fallback Analyzer being a small in-package stub (no snapshot-copy of
// keyword dictionaries). FER/SenseVoice/XTTS are nil — image/audio
// paths will fail at the inner layer; we only exercise the text path
// which uses the fallback.
//
// Coverage:
//
//   - happy path (text kind) → fallback returns result → wrapper maps
//     to MultiModalAnalyzeResp correctly.
//   - svcCtx.MultiModal == nil → ErrMultiModalNotInit.
//   - kind normalization: "  IMAGE " → "image" (lower + trim).
//   - unknown kind → validation error.
//   - image kind with nil FER client → inner error surfaces verbatim.
//   - analyzer returns nil → "analyzer returned nil" error.
package logic

import (
	"context"
	"errors"
	"testing"

	"emotion-echo-ai-svc/internal/analyzer"
	"emotion-echo-ai-svc/internal/svc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubMultimodalAnalyzer satisfies analyzer.Analyzer for the text path
// without snapshot-copying any keyword dictionary.
type stubMultimodalAnalyzer struct {
	got     string
	result  *analyzer.EmotionResult
	err     error
	returns bool
	called  bool
}

func (s *stubMultimodalAnalyzer) Analyze(ctx context.Context, text string) (*analyzer.EmotionResult, error) {
	s.called = true
	s.got = text
	if s.err != nil {
		return nil, s.err
	}
	if s.returns {
		return nil, nil
	}
	if s.result == nil {
		// safe default so the wrapper doesn't nil-deref.
		return &analyzer.EmotionResult{PrimaryEmotion: "neutral", Confidence: 0.5, Model: "stub"}, nil
	}
	return s.result, nil
}

var _ analyzer.Analyzer = (*stubMultimodalAnalyzer)(nil)

// TestMultiModalAnalyzeLogic_TextKind_FallbackUsed covers the canonical
// path: svcCtx.MultiModal wired with fallback-only → text input routes
// to fallback.Analyze, response fields are mapped correctly.
func TestMultiModalAnalyzeLogic_TextKind_FallbackUsed(t *testing.T) {
	t.Parallel()

	stub := &stubMultimodalAnalyzer{
		result: &analyzer.EmotionResult{
			PrimaryEmotion: "happy",
			Confidence:     0.91,
			SentimentScore: 0.6,
			Model:          "stub-v1",
		},
	}
	mma := analyzer.NewMultiModalAnalyzer(stub, nil, nil, nil)
	svcCtx := &svc.ServiceContext{MultiModal: mma}

	l := NewMultiModalAnalyzeLogic(svcCtx)
	resp, err := l.Analyze(context.Background(), "text", nil, "", "我今天很开心")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "text", resp.Kind)
	assert.Equal(t, "happy", resp.Emotion)
	assert.InDelta(t, 0.91, resp.Confidence, 0.001)
	assert.InDelta(t, 0.6, resp.Sentiment, 0.001)
	assert.Equal(t, "stub-v1", resp.Model)
	assert.True(t, stub.called, "fallback should have been invoked for text kind")
}

// TestMultiModalAnalyzeLogic_KindNormalization_LowerAndTrim covers the
// input sanitization: "  IMAGE " → "image". The implementation lower-
// cases and trims before the switch. We verify by sending "  IMAGE  "
// with a non-empty text body so the image branch (with nil FER) falls
// back to keyword analysis on text — which goes through the stub.
//
// Note: with FER==nil, MultiModalAnalyzer.analyzeImage actually calls
// m.Fallback.Analyze("[no text available, image only]") — NOT an error.
// So we route through "text" by passing only text content (no bytes),
// but with image kind normalization we expect the wrapper to NOT
// return the "kind must be one of" error.
func TestMultiModalAnalyzeLogic_KindNormalization_LowerAndTrim(t *testing.T) {
	t.Parallel()

	stub := &stubMultimodalAnalyzer{
		result: &analyzer.EmotionResult{PrimaryEmotion: "calm", Confidence: 0.5, Model: "stub"},
	}
	mma := analyzer.NewMultiModalAnalyzer(stub, nil, nil, nil)
	svcCtx := &svc.ServiceContext{MultiModal: mma}

	l := NewMultiModalAnalyzeLogic(svcCtx)
	// Pass a kind with mixed case + whitespace; image path with nil FER
	// falls back to keyword — should NOT error with "kind must be".
	_, err := l.Analyze(context.Background(), "  IMAGE  ", nil, "f.png", "fallback text")
	require.NoError(t, err, "normalized image kind should be accepted; got %v", err)
	assert.True(t, stub.called, "fallback should have been invoked after normalization")
}

// TestMultiModalAnalyzeLogic_UnknownKind_ReturnsValidationError pins
// the validation behavior for unsupported kinds.
func TestMultiModalAnalyzeLogic_UnknownKind_ReturnsValidationError(t *testing.T) {
	t.Parallel()

	stub := &stubMultimodalAnalyzer{}
	mma := analyzer.NewMultiModalAnalyzer(stub, nil, nil, nil)
	svcCtx := &svc.ServiceContext{MultiModal: mma}

	l := NewMultiModalAnalyzeLogic(svcCtx)
	_, err := l.Analyze(context.Background(), "video", nil, "x.mp4", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "kind must be one of")
	assert.False(t, stub.called, "unknown kind must not invoke analyzer")
}

// TestMultiModalAnalyzeLogic_NilMultimodal_ReturnsErrMultiModalNotInit
// covers the unwired-service branch: svcCtx.MultiModal == nil.
func TestMultiModalAnalyzeLogic_NilMultimodal_ReturnsErrMultiModalNotInit(t *testing.T) {
	t.Parallel()

	svcCtx := &svc.ServiceContext{MultiModal: nil}
	l := NewMultiModalAnalyzeLogic(svcCtx)
	_, err := l.Analyze(context.Background(), "text", nil, "", "hi")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multi-modal analyzer not initialised")
}

// TestMultiModalAnalyzeLogic_AnalyzerReturnsNil_ReturnsError pins the
// "inner returned nil result" branch — the wrapper must surface a
// recognizable error rather than panic on the next field read.
func TestMultiModalAnalyzeLogic_AnalyzerReturnsNil_ReturnsError(t *testing.T) {
	t.Parallel()

	stub := &stubMultimodalAnalyzer{returns: true}
	mma := analyzer.NewMultiModalAnalyzer(stub, nil, nil, nil)
	svcCtx := &svc.ServiceContext{MultiModal: mma}

	l := NewMultiModalAnalyzeLogic(svcCtx)
	_, err := l.Analyze(context.Background(), "text", nil, "", "hi")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "analyzer returned nil")
}

// TestMultiModalAnalyzeLogic_AnalyzerError_PropagatesAsIs covers the
// "inner layer fails" branch — wrapper propagates verbatim.
func TestMultiModalAnalyzeLogic_AnalyzerError_PropagatesAsIs(t *testing.T) {
	t.Parallel()

	stub := &stubMultimodalAnalyzer{err: errors.New("downstream unavailable")}
	mma := analyzer.NewMultiModalAnalyzer(stub, nil, nil, nil)
	svcCtx := &svc.ServiceContext{MultiModal: mma}

	l := NewMultiModalAnalyzeLogic(svcCtx)
	_, err := l.Analyze(context.Background(), "text", nil, "", "hi")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "downstream unavailable")
}
// TestMultiModalAnalyzeLogic_AudioTranscriptPropagated 是 E2E-F-104 的回归钉。
//
// 背景：analyzer.EmotionResult 原先无 Text 字段 ⇒ ASR 转写文本在 analyzer 内部
// 被丢弃；logic 层又只回填请求侧 textContent（BFF 从不传）⇒ /voice/upload 的
// transcript 恒空，前端即使修好响应消费也上不了屏。
//
// 注意（诚实标注）：本条断言是在 GREEN 实现之后补的**回归钉**，
// 该缺陷的 RED 步骤在 analyzer 层完成 ——
// `TestMultiModalAnalyzer_SenseVoiceSuccess` 因 `r.Text undefined` 编译失败。
func TestMultiModalAnalyzeLogic_AudioTranscriptPropagated(t *testing.T) {
	t.Parallel()

	stub := &stubMultimodalAnalyzer{
		result: &analyzer.EmotionResult{
			PrimaryEmotion: "happy",
			Confidence:     0.95,
			Model:          "sensevoice:sensevoice",
			Text:           "我太开心了",
		},
	}
	mma := analyzer.NewMultiModalAnalyzer(stub, nil, nil, nil)
	svcCtx := &svc.ServiceContext{MultiModal: mma}

	l := NewMultiModalAnalyzeLogic(svcCtx)
	// 关键：请求侧 text（第 5 个参数）为空 —— 与 BFF /voice/upload 的真实调用一致
	resp, err := l.Analyze(context.Background(), "audio", []byte("webm"), "v.webm", "")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "我太开心了", resp.Transcript,
		"音频路径必须回传 ASR 文本（请求侧未提供 text 时更应如此）")
}

// TestMultiModalAnalyzeLogic_AudioTranscriptFallsBackToRequestText 覆盖
// 请求侧 text 作为补充的语义：ASR 文本为空时回落到请求侧 text。
func TestMultiModalAnalyzeLogic_AudioTranscriptFallsBackToRequestText(t *testing.T) {
	t.Parallel()

	stub := &stubMultimodalAnalyzer{
		result: &analyzer.EmotionResult{
			PrimaryEmotion: "neutral",
			Confidence:     0.5,
			Model:          "keyword-v1",
			Text:           "", // ASR 未给文本（降级/静音）
		},
	}
	mma := analyzer.NewMultiModalAnalyzer(stub, nil, nil, nil)
	svcCtx := &svc.ServiceContext{MultiModal: mma}

	l := NewMultiModalAnalyzeLogic(svcCtx)
	resp, err := l.Analyze(context.Background(), "audio", []byte("webm"), "v.webm", "前端补充的文本")
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, "前端补充的文本", resp.Transcript)
}
