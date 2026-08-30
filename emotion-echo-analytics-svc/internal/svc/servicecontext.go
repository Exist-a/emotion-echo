// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

package svc

import (
	"emotion-echo-analytics-svc/internal/config"
	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/trigger"
)

// ServiceContext analytics-svc 依赖注入容器
//
// Stage 30-A 扩展：除 EventRepo 外，新增 ReportRepo（跨 schema 只读聚合）。
// Round 3 加 MentalHealthRepo + TriggerQueue；Round 4 加 KafkaConsumer。
type ServiceContext struct {
	Config           config.Config
	EventRepo        repository.EventRepo
	ReportRepo       repository.ReportRepo        // Round 1
	MentalHealthRepo repository.MentalHealthRepo // Round 3 part 1
	TriggerQueue     *trigger.TriggerQueue        // Round 3 part 2
}

// NewServiceContext 用最少的依赖构造 svcCtx（向后兼容 Round 0）。
func NewServiceContext(c config.Config, repo repository.EventRepo) *ServiceContext {
	return &ServiceContext{
		Config:    c,
		EventRepo: repo,
	}
}

// NewServiceContextWithReports 完整构造（Round 1 推荐用法）。
func NewServiceContextWithReports(c config.Config, eventRepo repository.EventRepo, reportRepo repository.ReportRepo) *ServiceContext {
	return &ServiceContext{
		Config:     c,
		EventRepo:  eventRepo,
		ReportRepo: reportRepo,
	}
}