package svc

import (
	"emotion-echo-user-svc/internal/config"
	"emotion-echo-user-svc/internal/repository"
)

type ServiceContext struct {
	Config             config.Config
	UserRepo           repository.UserRepo
	SecurityAnswerRepo repository.SecurityAnswerRepo
	// AgentRepo CL-06（用户自定义 Agent·多角色）：角色档案仓储
	AgentRepo repository.UserAgentProfileRepo
}

func NewServiceContext(
	c config.Config,
	userRepo repository.UserRepo,
	securityAnswerRepo repository.SecurityAnswerRepo,
	agentRepo repository.UserAgentProfileRepo,
) *ServiceContext {
	return &ServiceContext{
		Config:             c,
		UserRepo:           userRepo,
		SecurityAnswerRepo: securityAnswerRepo,
		AgentRepo:          agentRepo,
	}
}
