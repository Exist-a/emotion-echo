package analyzer

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"emotion-echo-ai-svc/internal/aiclient"
)

// MultiModalInput 描述一段待分析的多模态输入
//
// Kind 决定分析路径：
//   - "text"  → 直接调用 fallback（关键词 / 外部 LLM）
//   - "image" → 优先走 FER；不可用则降级
//   - "audio" → 优先走 SenseVoice → 拿到文本后再走文本分析
type MultiModalInput struct {
	Kind     string // "text" | "image" | "audio"
	Text     string
	Bytes    []byte // image or audio payload
	Filename string
}

// MultiModalAnalyzer 优先用 AI 模型服务，按需降级到 fallback analyzer
//
// 设计：
//   - FER / SenseVoice / XTTS 都是可选（New* 可能返回 nil）
//   - 当用户没有启用 AI profile（BaseURL 未配置），三个 client 都为 nil，
//     自动降级到 fallback.Analyze(text)
type MultiModalAnalyzer struct {
	Fallback   Analyzer                                // 必填：纯文本分析兜底
	FER        *aiclient.FERClient                     // 可选
	SenseVoice *aiclient.SenseVoiceClient              // 可选
	XTTS       *aiclient.XTTSClient                    // 暂不参与 Analyze（Synthesize 在其他方法里）
	Logger     func(msg string, args ...any)           // info log；nil 时静默
}

// NewMultiModalAnalyzer 构造器；fallback 必填，三个 client 可空
func NewMultiModalAnalyzer(fallback Analyzer, fer *aiclient.FERClient,
	sv *aiclient.SenseVoiceClient, xtts *aiclient.XTTSClient) *MultiModalAnalyzer {
	return &MultiModalAnalyzer{
		Fallback:   fallback,
		FER:        fer,
		SenseVoice: sv,
		XTTS:       xtts,
	}
}

func (m *MultiModalAnalyzer) log(msg string, args ...any) {
	if m.Logger != nil {
		m.Logger(msg, args...)
	}
}

// Analyze 输入可以是文本、图像、音频（按 Kind 分派）
func (m *MultiModalAnalyzer) Analyze(ctx context.Context, in MultiModalInput) (*EmotionResult, error) {
	switch strings.ToLower(in.Kind) {
	case "text":
		return m.Fallback.Analyze(ctx, in.Text)

	case "image":
		return m.analyzeImage(ctx, in)
	case "audio":
		return m.analyzeAudio(ctx, in)

	default:
		return m.Fallback.Analyze(ctx, in.Text)
	}
}

func (m *MultiModalAnalyzer) analyzeImage(ctx context.Context, in MultiModalInput) (*EmotionResult, error) {
	if m.FER == nil {
		m.log("FER disabled; image input falls back to keyword analyzer")
		return m.Fallback.Analyze(ctx, "[no text available, image only]")
	}
	if len(in.Bytes) == 0 {
		return nil, errors.New("image bytes required")
	}
	m.log("analyzing image via FER bytes=%d", len(in.Bytes))
	res, err := m.FER.AnalyzeImage(ctx, in.Bytes, in.Filename)
	if err != nil {
		m.log("FER call failed: %v; fallback", err)
		return m.Fallback.Analyze(ctx, "[image analysis failed]")
	}
	return &EmotionResult{
		PrimaryEmotion: res.Emotion,
		SentimentScore: sentimentFromEmotion(res.Emotion),
		Confidence:     res.Confidence,
		Model:          "fer:" + res.Source,
	}, nil
}

func (m *MultiModalAnalyzer) analyzeAudio(ctx context.Context, in MultiModalInput) (*EmotionResult, error) {
	if m.SenseVoice == nil {
		m.log("SenseVoice disabled; audio falls back to keyword analyzer")
		return m.Fallback.Analyze(ctx, "")
	}
	if len(in.Bytes) == 0 {
		return nil, errors.New("audio bytes required")
	}
	m.log("transcribing + analyzing audio via SenseVoice bytes=%d", len(in.Bytes))
	svRes, err := m.SenseVoice.Analyze(ctx, in.Bytes, in.Filename)
	if err != nil {
		m.log("SenseVoice failed: %v; fallback", err)
		return m.Fallback.Analyze(ctx, "")
	}

	// 用音频里的情感类别作为 primary 优先；再让 fallback 基于转写文本算 sentiment。
	if svRes.Emotion != "" && svRes.Emotion != "unk" {
		fb, _ := m.Fallback.Analyze(ctx, svRes.Text)
		return &EmotionResult{
			PrimaryEmotion: svRes.Emotion,
			SentimentScore: avgFloat(sentimentFromEmotion(svRes.Emotion), fb.SentimentScore),
			Confidence:     svRes.Confidence,
			Model:          "sensevoice:" + svRes.Source,
		}, nil
	}
	return m.Fallback.Analyze(ctx, svRes.Text)
}

// SynthesizeText 文本转语音（XTTS）。XTTS 未配置时返回 ErrNotConfigured
//
// 用在"回复用户时一起返回语音"的场景；与情绪分析无关。
func (m *MultiModalAnalyzer) SynthesizeText(ctx context.Context, text string) ([]byte, int, error) {
	if m.XTTS == nil {
		return nil, 0, aiclient.ErrNotConfigured
	}
	return m.XTTS.Synthesize(ctx, text)
}

// Helpers
func sentimentFromEmotion(emotion string) float64 {
	switch emotion {
	case "happy":
		return 0.7
	case "sad", "angry", "anxious":
		return -0.5
	case "neutral":
		return 0
	case "calm":
		return 0.3
	default:
		return 0
	}
}

func avgFloat(a, b float64) float64 {
	return (a + b) / 2
}

// unused — 保留以备扩展
var _ = fmt.Sprintf
