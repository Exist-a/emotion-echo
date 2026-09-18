
package svc

import (
	"emotion-echo-user-svc/internal/config"
	"emotion-echo-user-svc/internal/repository"
)

type ServiceContext struct {
	Config            config.Config
	UserRepo          repository.UserRepo
	SecurityAnswerRepo repository.SecurityAnswerRepo
}

func NewServiceContext(c config.Config, userRepo repository.UserRepo, securityAnswerRepo repository.SecurityAnswerRepo) *ServiceContext {
	return &ServiceContext{
		Config:            c,
		UserRepo:          userRepo,
		SecurityAnswerRepo: securityAnswerRepo,
	}
}