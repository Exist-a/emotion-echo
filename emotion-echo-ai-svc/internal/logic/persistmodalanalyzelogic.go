// Package logic — Stage 34 · PR-8 GREEN
//
// PersistMultiModalAnalyzeLogic 是 MultiModalAnalyzeLogic 的"持久化包装器"：
// 在原有 Analyze 行为之上，根据 kind 与 persist 开关决定是否落库到
// face_emotion_results / voice_emotion_results。
//
// 设计原则：
//   - 与原 MultiModalAnalyzeLogic 共存，不改其签名 → handler 可渐进迁移
//   - persist=false 时行为与原 MultiModalAnalyzeLogic 完全一致（不写库）
//   - persist=true 时按 kind 选择 repo 写入
//   - 分析失败时不写库（事务安全）
package logic

import (
	"context"
	"errors"
	"strings"

	"emotion-echo-ai-svc/internal/analyzer"
	"emotion-echo-ai-svc/internal/model"
	"emotion-echo-ai-svc/internal/svc"
)

// PersistRequest 是 PersistMultiModalAnalyzeLogic 的入参结构体。
//
// 比原 Analyze 多了：UploadID / Persist / MessageID / UserID / ConversationID
// / Transcript / DurationMs / Language。Handler 从 multipart form 透传。
type PersistRequest struct {
	Kind           string
	Bytes          []byte
	Filename       string
	TextContent    string
	UploadID       string // 前端 nonce
	Persist        bool   // 是否落库
	MessageID      int64
	UserID         int64
	ConversationID int64
	// audio-only 字段
	Transcript string
	DurationMs int
	Language   string
}

// PersistMultiModalAnalyzeLogic 多模态分析 + 可选持久化包装器。
type PersistMultiModalAnalyzeLogic struct {
	svcCtx *svc.ServiceContext
}

func NewPersistMultiModalAnalyzeLogic(svcCtx *svc.ServiceContext) *PersistMultiModalAnalyzeLogic {
	return &PersistMultiModalAnalyzeLogic{svcCtx: svcCtx}
}

// Analyze 走 MultiModalAnalyzer；persist=true 时按 kind 写库。
//
// 返回的 *MultiModalAnalyzeResp 字段复用原结构体（见 multimodalanalyzelogic.go），
// 不引入新 view-model，避免重复定义。
func (l *PersistMultiModalAnalyzeLogic) Analyze(ctx context.Context, req PersistRequest) (*MultiModalAnalyzeResp, error) {
	if l.svcCtx.MultiModal == nil {
		return nil, errors.New("multi-modal analyzer not initialised")
	}

	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	if kind != "image" && kind != "audio" && kind != "text" {
		return nil, errors.New("kind must be one of: text, image, audio")
	}

	input := analyzer.MultiModalInput{
		Kind:     kind,
		Text:     req.TextContent,
		Bytes:    req.Bytes,
		Filename: req.Filename,
	}
	result, err := l.svcCtx.MultiModal.Analyze(ctx, input)
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, errors.New("analyzer returned nil")
	}

	// 持久化：分析成功后按 kind 写库（失败回退：不写）
	if req.Persist {
		if err := l.persistResult(ctx, req, result); err != nil {
			return nil, err
		}
	}

	return &MultiModalAnalyzeResp{
		Kind:       kind,
		Emotion:    result.PrimaryEmotion,
		Confidence: result.Confidence,
		Sentiment:  result.SentimentScore,
		Model:      result.Model,
	}, nil
}

// persistResult 按 kind 写库。UploadID 为空时退化为"非幂等"路径（不阻塞功能）。
//
// 设计：写库失败返回 error，调用方决定是否回退。Analyzer 已成功 → 写库失败
// 应当被 caller 记录 + 告警（不影响用户感知，但需要可观测）。
func (l *PersistMultiModalAnalyzeLogic) persistResult(ctx context.Context, req PersistRequest, result *analyzer.EmotionResult) error {
	switch req.Kind {
	case "image":
		if l.svcCtx.FaceEmotionRepo == nil {
			return nil // repo 未装配 → 静默跳过（向后兼容）
		}
		row := &model.FaceEmotionResult{
			UploadID:       req.UploadID,
			MessageID:      req.MessageID,
			UserID:         req.UserID,
			ConversationID: req.ConversationID,
			PrimaryEmotion: result.PrimaryEmotion,
			Confidence:     result.Confidence,
			Model:          result.Model,
		}
		return l.svcCtx.FaceEmotionRepo.Create(ctx, row)

	case "audio":
		if l.svcCtx.VoiceEmotionRepo == nil {
			return nil
		}
		row := &model.VoiceEmotionResult{
			UploadID:       req.UploadID,
			MessageID:      req.MessageID,
			UserID:         req.UserID,
			ConversationID: req.ConversationID,
			Transcript:     req.Transcript,
			PrimaryEmotion: result.PrimaryEmotion,
			Confidence:     result.Confidence,
			Model:          result.Model,
			DurationMs:     req.DurationMs,
			Language:       req.Language,
		}
		return l.svcCtx.VoiceEmotionRepo.Create(ctx, row)

	case "text":
		// text 走 chat-events Kafka 异步链路，不在 multimodal 端点写。
		return nil

	default:
		return nil
	}
}
