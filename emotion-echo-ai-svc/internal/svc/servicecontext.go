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
	//
	// Stage 26-T: 字段类型改为 interface（aiclient.FERService 等），
	// 让 aihealthlogic / handler 可在测试里替换为 fake，避免
	// 直接依赖具体 struct（具体 struct 的字段不可 mock）。
	// 具体 *aiclient.FERClient 等隐式满足 interface；InitMultiModal
	// 仍然返回具体类型，Go 自动转换。
	FER        aiclient.FERService
	SenseVoice aiclient.SenseVoiceService
	XTTS       aiclient.XTTSService

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
//
// Stage 26-T 重构：构造的具体 *aiclient.FERClient / *SenseVoiceClient /
// *XTTSClient 先保留在局部变量，赋值给 interface 字段后再传给
// MultiModalAnalyzer（后者签名仍要求具体类型）。
func (s *ServiceContext) InitMultiModal() {
	ferClient := aiclient.NewFERClient(aiclient.Config{BaseURL: s.Config.FER.BaseURL, Timeout: s.Config.FER.Timeout})
	svClient := aiclient.NewSenseVoiceClient(aiclient.Config{BaseURL: s.Config.SenseVoice.BaseURL, Timeout: s.Config.SenseVoice.Timeout})
	xttsClient := aiclient.NewXTTSClient(aiclient.Config{BaseURL: s.Config.XTTS.BaseURL, Timeout: s.Config.XTTS.Timeout},
		s.Config.XTTS.Language, s.Config.XTTS.Speed)

	// Assign to interface fields (production) AND keep concrete refs
	// for the analyzer (which still takes concrete *aiclient.*Client).
	s.FER = ferClient
	s.SenseVoice = svClient
	s.XTTS = xttsClient
	s.MultiModal = analyzer.NewMultiModalAnalyzer(
		analyzer.NewKeywordAnalyzer(),
		ferClient, svClient, xttsClient,
	)
}