// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package svc

import (
	"emotion-echo-ai-svc/internal/aiclient"
	"emotion-echo-ai-svc/internal/analyzer"
	"emotion-echo-ai-svc/internal/config"
	"emotion-echo-ai-svc/internal/repository"
)

type ServiceContext struct {
	Config     config.Config
	EmotionRepo repository.EmotionRepo

	// Stage 22-A: 多模态 AI 模型客户端（任一可为 nil）。
	// 调用方使用前应检查非空，或使用 analyzer.MultiModalAnalyzer 做降级。
	FER        *aiclient.FERClient
	SenseVoice *aiclient.SenseVoiceClient
	XTTS       *aiclient.XTTSClient

	// MultiModalAnalyzer 集成版；外面 handler 可以直接用。
	MultiModal *analyzer.MultiModalAnalyzer
}

func NewServiceContext(c config.Config, repo repository.EmotionRepo) *ServiceContext {
	return &ServiceContext{
		Config:      c,
		EmotionRepo: repo,
	}
}

// InitMultiModal 按 config 构造 3 个 aiclient + 多模态 analyzer
//
// 由 main.go 启动时调用一次。建议在 NewServiceContext 之后立刻调用。
func (s *ServiceContext) InitMultiModal() {
	s.FER = aiclient.NewFERClient(aiclient.Config{BaseURL: s.Config.FER.BaseURL, Timeout: s.Config.FER.Timeout})
	s.SenseVoice = aiclient.NewSenseVoiceClient(aiclient.Config{BaseURL: s.Config.SenseVoice.BaseURL, Timeout: s.Config.SenseVoice.Timeout})
	s.XTTS = aiclient.NewXTTSClient(aiclient.Config{BaseURL: s.Config.XTTS.BaseURL, Timeout: s.Config.XTTS.Timeout},
		s.Config.XTTS.Language, s.Config.XTTS.Speed)
	s.MultiModal = analyzer.NewMultiModalAnalyzer(
		analyzer.NewKeywordAnalyzer(),
		s.FER, s.SenseVoice, s.XTTS,
	)
}