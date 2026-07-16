// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package svc

import (
	"emotion-echo-user-svc/internal/config"
	"emotion-echo-user-svc/internal/repository"
)

type ServiceContext struct {
	Config  config.Config
	UserRepo repository.UserRepo
}

func NewServiceContext(c config.Config, userRepo repository.UserRepo) *ServiceContext {
	return &ServiceContext{
		Config:   c,
		UserRepo: userRepo,
	}
}