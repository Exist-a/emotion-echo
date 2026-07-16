// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package svc

import (
	"emotion-echo-assessment-svc/internal/config"
	"emotion-echo-assessment-svc/internal/repository"
)

type ServiceContext struct {
	Config     config.Config
	SurveyRepo repository.SurveyRepo
}

func NewServiceContext(c config.Config, repo repository.SurveyRepo) *ServiceContext {
	return &ServiceContext{
		Config:     c,
		SurveyRepo: repo,
	}
}