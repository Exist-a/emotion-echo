// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package svc

import (
	"emotion-echo-chat-svc/internal/config"
	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/repository"

	"gorm.io/gorm"
)

// ServiceContext 是 chat-svc 的依赖注入容器
//
// 加新依赖时在这里加字段，构造函数加参数
// 所有 logic 通过 l.svcCtx.X 访问
//
// Stage 30-C A3: 加 DB 与 OutboxRepo 字段。DB 为 nil 时 logic 走非事务路径
// （InMemory 测试场景）；DB 非 nil 时走事务（生产 Postgres 场景）。
type ServiceContext struct {
	Config           config.Config
	ConversationRepo repository.ConversationRepo
	EventPublisher   events.EventPublisher

	// Stage 30-C A3: 事务性 Outbox
	DB         *gorm.DB            // 生产场景注入；nil = 走非事务路径
	OutboxRepo repository.OutboxRepo // 生产场景注入；nil = 不写 outbox（退化原行为）
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

// WithDB 设置 DB（GORM）+ OutboxRepo（A3 用）
func (s *ServiceContext) WithDB(db *gorm.DB) *ServiceContext {
	s.DB = db
	return s
}

// WithOutboxRepo 设置 OutboxRepo
func (s *ServiceContext) WithOutboxRepo(repo repository.OutboxRepo) *ServiceContext {
	s.OutboxRepo = repo
	return s
}