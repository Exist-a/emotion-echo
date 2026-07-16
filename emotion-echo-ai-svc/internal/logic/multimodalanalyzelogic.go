// Stage 23-A: 多模态分析对外 endpoint 的业务逻辑。
//
// 入口：multipart/form-data，字段：
//   - kind:    "image" | "audio"
//   - file:    二进制载荷（图像或音频）
//   - filename: 文件名（可选）
//   - text:    文本（可选，与 file 二选一或同时）
//
// 返回：analyzer.EmotionResult + 可选 transcript（音频路径）

package logic

import (
	"context"
	"errors"
	"strings"

	"emotion-echo-ai-svc/internal/analyzer"
	"emotion-echo-ai-svc/internal/svc"
)

// MultiModalAnalyzeResp 标准响应结构
type MultiModalAnalyzeResp struct {
	Kind        string             `json:"kind"`
	Emotion     string             `json:"emotion"`
	Confidence  float64            `json:"confidence"`
	Sentiment   float64            `json:"sentimentScore"`
	Model       string             `json:"model"`
	Transcript  string             `json:"transcript,omitempty"`
	AllScores   map[string]float64 `json:"allScores,omitempty"`
}

// MultiModalAnalyzeLogic 多模态情绪分析的对外网关逻辑
type MultiModalAnalyzeLogic struct {
	svcCtx *svc.ServiceContext
}

func NewMultiModalAnalyzeLogic(svcCtx *svc.ServiceContext) *MultiModalAnalyzeLogic {
	return &MultiModalAnalyzeLogic{svcCtx: svcCtx}
}

// Analyze 是 handler 调用的入口。kind 必填，bytes 按 kind 决定什么 binary。
func (l *MultiModalAnalyzeLogic) Analyze(ctx context.Context, kind string, fileBytes []byte, filename string, textContent string) (*MultiModalAnalyzeResp, error) {
	if l.svcCtx.MultiModal == nil {
		return nil, errors.New("multi-modal analyzer not initialised")
	}
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind != "image" && kind != "audio" && kind != "text" {
		return nil, errors.New("kind must be one of: text, image, audio")
	}

	input := analyzer.MultiModalInput{
		Kind:     kind,
		Text:     textContent,
		Bytes:    fileBytes,
		Filename: filename,
	}
	result, err := l.svcCtx.MultiModal.Analyze(ctx, input)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("analyzer returned nil")
	}

	resp := &MultiModalAnalyzeResp{
		Kind:       kind,
		Emotion:    result.PrimaryEmotion,
		Confidence: result.Confidence,
		Sentiment:  result.SentimentScore,
		Model:      result.Model,
	}
	// 音频路径：把转写文本回给调用方，便于前端展示
	if kind == "audio" && textContent == "" {
		// fallback 路径返回空文本是正常的
		resp.Transcript = ""
	} else {
		resp.Transcript = textContent
	}
	return resp, nil
}
