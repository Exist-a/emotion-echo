// Stage 23-B: TTS 文本转语音 endpoint 的业务逻辑
//
// 请求 JSON：{ "text": "...", "language": "zh-cn", "speed": 0.75 }
// 响应：{ "audio": "<base64 WAV>", "sampleRate": 24000, "mime": "audio/wav", "bytes": <length> }

package logic

import (
	"context"
	"encoding/base64"
	"errors"

	"emotion-echo-ai-svc/internal/svc"
)

// Sentinel errors used by handler to map to HTTP 503.
var (
	ErrMultiModalNotInit = errors.New("multi-modal analyzer not initialised")
	ErrXTTSUnavailable   = errors.New("XTTS service not configured (XTTS_BASE_URL empty)")
)

type SynthesizeSpeechResp struct {
	Audio      string `json:"audio"`         // base64-encoded WAV
	SampleRate int    `json:"sampleRate"`
	MIME       string `json:"mime"`
	Bytes      int    `json:"bytes"`
	Text       string `json:"text"`
	Language   string `json:"language"`
}

type SynthesizeSpeechLogic struct {
	svcCtx *svc.ServiceContext
}

func NewSynthesizeSpeechLogic(svcCtx *svc.ServiceContext) *SynthesizeSpeechLogic {
	return &SynthesizeSpeechLogic{svcCtx: svcCtx}
}

func (l *SynthesizeSpeechLogic) Synthesize(ctx context.Context, text, language string, speed float64) (*SynthesizeSpeechResp, error) {
	if l.svcCtx.MultiModal == nil {
		return nil, ErrMultiModalNotInit
	}
	if l.svcCtx.XTTS == nil {
		return nil, ErrXTTSUnavailable
	}
	if text == "" {
		return nil, errors.New("text is empty")
	}
	// 默认参数（与 config.YAML 对齐）
	if language == "" {
		language = l.svcCtx.Config.XTTS.Language
		if language == "" {
			language = "zh-cn"
		}
	}
	if speed <= 0 {
		speed = l.svcCtx.Config.XTTS.Speed
		if speed <= 0 {
			speed = 0.75
		}
	}

	wav, sr, err := l.svcCtx.MultiModal.SynthesizeText(ctx, text)
	if err != nil {
		return nil, err
	}
	if len(wav) == 0 {
		return nil, errors.New("empty audio returned")
	}
	return &SynthesizeSpeechResp{
		Audio:      base64.StdEncoding.EncodeToString(wav),
		SampleRate: sr,
		MIME:       "audio/wav",
		Bytes:      len(wav),
		Text:       text,
		Language:   language,
	}, nil
}