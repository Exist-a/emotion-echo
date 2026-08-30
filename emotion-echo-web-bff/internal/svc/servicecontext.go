// Package svc — servicecontext.go
//
// Stage 30 / stage-30-web-bff.md §三: BFF 依赖注入容器。
//
// 持有所有下游 client + auth manager + config，handler 从这取依赖。
package svc

import (
	"emotion-echo-web-bff/internal/auth"
	"emotion-echo-web-bff/internal/config"
	"emotion-echo-web-bff/internal/downstream"
)

// ServiceContext 是 BFF 的依赖注入容器
type ServiceContext struct {
	Config config.Config

	// 下游 client（7 个）
	User       downstream.UserClient
	Chat       downstream.ChatClient
	Assessment downstream.AssessmentClient
	Analytics  downstream.AnalyticsClient
	AI         downstream.AIClient
	XTTS       downstream.XTTSClient
	EmotionQ   downstream.EmotionQueryClient

	// 自有鉴权（签发 JWT）
	Auth *auth.Manager
}

// NewServiceContext 构造容器（所有依赖必须已构造好）
func NewServiceContext(c config.Config) *ServiceContext {
	return &ServiceContext{Config: c}
}

// SetUser 注入 user client
func (s *ServiceContext) SetUser(u downstream.UserClient) *ServiceContext { s.User = u; return s }
func (s *ServiceContext) SetChat(c downstream.ChatClient) *ServiceContext  { s.Chat = c; return s }
func (s *ServiceContext) SetAssessment(a downstream.AssessmentClient) *ServiceContext {
	s.Assessment = a
	return s
}
func (s *ServiceContext) SetAnalytics(a downstream.AnalyticsClient) *ServiceContext {
	s.Analytics = a
	return s
}
func (s *ServiceContext) SetAI(a downstream.AIClient) *ServiceContext { s.AI = a; return s }
func (s *ServiceContext) SetXTTS(x downstream.XTTSClient) *ServiceContext {
	s.XTTS = x
	return s
}
func (s *ServiceContext) SetEmotionQ(e downstream.EmotionQueryClient) *ServiceContext {
	s.EmotionQ = e
	return s
}
func (s *ServiceContext) SetAuth(a *auth.Manager) *ServiceContext { s.Auth = a; return s }
