// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package svc

import (
	"emotion-echo-chat-svc/internal/config"
	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/repository"
)

// ServiceContext 是 chat-svc 的依赖注入容器
//
// 加新依赖时在这里加字段，构造函数加参数
// 所有 logic 通过 l.svcCtx.X 访问
type ServiceContext struct {
	Config           config.Config
	ConversationRepo repository.ConversationRepo
	EventPublisher   events.EventPublisher
}

// NewServiceContext 构造容器
//
// publisher 必传（chat-svc 强依赖事件总线）
// repo 必传（DB 是基础）
func NewServiceContext(c config.Config, repo repository.ConversationRepo, pub events.EventPublisher) *ServiceContext {
	return &ServiceContext{
		Config:           c,
		ConversationRepo: repo,
		EventPublisher:   pub,
	}
}