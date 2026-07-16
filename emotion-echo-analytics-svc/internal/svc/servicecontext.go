// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package svc

import (
	"emotion-echo-analytics-svc/internal/config"
	"emotion-echo-analytics-svc/internal/repository"
)

type ServiceContext struct {
	Config   config.Config
	EventRepo repository.EventRepo
}

func NewServiceContext(c config.Config, repo repository.EventRepo) *ServiceContext {
	return &ServiceContext{
		Config:    c,
		EventRepo: repo,
	}
}