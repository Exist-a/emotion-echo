// analytics-svc main 入口（Gin 版本，2026-07-14 迁移自 go-zero）
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strconv"
	"time"

	"emotion-echo-analytics-svc/internal/config"
	"emotion-echo-analytics-svc/internal/handler"
	"emotion-echo-analytics-svc/internal/repository"
	"emotion-echo-analytics-svc/internal/svc"
	"emotion-echo-analytics-svc/internal/trigger"

	"github.com/SkyAPM/go2sky"
	"github.com/SkyAPM/go2sky/reporter"
	"github.com/gin-gonic/gin"
	sharedmetrics "github.com/emotion-echo/shared/pkg/metrics"
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
	"github.com/zeromicro/go-zero/core/conf"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var configFile = flag.String("f", "etc/analytics-api.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	// 1. Postgres（单一 gorm.DB — Round 5 migrations 落地后驱动 search_path）
	db, dbErr := openPostgres(c.Postgres.DSN, c.Postgres.MaxOpenConns, c.Postgres.MaxIdleConns)
	var evtRepo repository.EventRepo
	if dbErr != nil {
		log.Printf("[postgres] connect failed: %v (EventRepo = nil)", dbErr)
		evtRepo = nil
	} else {
		log.Printf("[postgres] connected")
		evtRepo = repository.NewPostgresEventRepo(db)
	}

	// Round 1：ReportRepo（跨 schema 只读聚合）
	var reportRepo repository.ReportRepo
	if db != nil {
		reportRepo = repository.NewPostgresReportRepo(db)
	}

	// Round 3：MentalHealthRepo（跨 schema 只读）
	var mhRepo repository.MentalHealthRepo
	if db != nil {
		mhRepo = repository.NewPostgresMentalHealthRepo(db)
	}

	// Round 3 part 2：TriggerQueue（异步评估）
	queueCap := 64
	if c.TriggerQueueCap > 0 {
		queueCap = c.TriggerQueueCap
	}
	workers := 4
	tq := trigger.NewTriggerQueue(context.Background(), workers, queueCap, func(_ context.Context, _ trigger.Request) {
		// Round 4 GREEN 占位：worker body 真实逻辑在 Round 5
		// （mental_health_service.TriggerAssessment 调用）
		log.Printf("[trigger] worker received request (no-op in Round 4)")
	})
	// 在 main 退出时优雅关闭
	defer tq.Close(context.Background())

	// 2. SkyWalking tracer
	var tracer *go2sky.Tracer
	if c.SkyWalking.Enabled {
		rep, err := reporter.NewGRPCReporter(c.SkyWalking.OAPAddr)
		if err == nil {
			svcName := c.SkyWalking.ServiceName
			if svcName == "" {
				svcName = c.Name
			}
			tracer, _ = go2sky.NewTracer(svcName, go2sky.WithReporter(rep))
			if tracer != nil {
				log.Printf("[skywalking] tracer initialized")
			}
		}
	}

	// 3. ServiceContext（Round 4 扩展：ReportRepo + MentalHealthRepo + TriggerQueue）
	svcCtx := svc.NewServiceContext(c, evtRepo)
	svcCtx.ReportRepo = reportRepo
	svcCtx.MentalHealthRepo = mhRepo
	svcCtx.TriggerQueue = tq

	// 4. Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(sharedmetrics.GinMetricsMiddleware("analytics-svc"))
	if tracer != nil {
		r.Use(sharedmw.GinSkywalkingMiddleware(tracer))
	}
	r.Use(sharedmw.GinAuthMiddleware())

	// 5. routes — Stage 30-A 9 个业务端点 + 健康 + metrics
	registerRoutes(r, svcCtx)

	log.Printf("Starting analytics-svc at %s:%d...", c.Host, c.Port)
	if err := r.Run(fmt.Sprintf("%s:%d", c.Host, c.Port)); err != nil {
		log.Fatalf("[gin] server crashed: %v", err)
	}
}

// registerRoutes 注册 Stage 30-A 9 个业务端点 + /health + /metrics
//
// Stage 30-A §二 HTTP 契约（前后端共同）：
//   GET  /api/v1/reports/daily
//   GET  /api/v1/reports/trend
//   GET  /api/v1/user-behavior/day-night
//   GET  /api/v1/user-behavior/depth
//   GET  /api/v1/user-behavior/frequency
//   GET  /api/v1/mental-health/assessment
//   GET  /api/v1/mental-health/history
//   POST /api/v1/mental-health/trigger
//   GET  /api/v1/mental-health/trend
func registerRoutes(r *gin.Engine, svcCtx *svc.ServiceContext) {
	// 基础设施
	r.GET("/health", handler.HealthHandler(svcCtx))
	r.GET("/metrics", gin.WrapH(sharedmetrics.PromHTTPHandler()))

	// Stage 30-A 业务路由
	r.GET("/api/v1/reports/daily", handler.ReportsDailyHandler(svcCtx))
	r.GET("/api/v1/reports/trend", handler.ReportsTrendHandler(svcCtx))

	r.GET("/api/v1/user-behavior/day-night", handler.UserBehaviorDayNightHandler(svcCtx))
	r.GET("/api/v1/user-behavior/depth", handler.UserBehaviorDepthHandler(svcCtx))
	r.GET("/api/v1/user-behavior/frequency", handler.UserBehaviorFrequencyHandler(svcCtx))

	r.GET("/api/v1/mental-health/assessment", handler.MentalHealthAssessmentHandler(svcCtx))
	r.GET("/api/v1/mental-health/history", handler.MentalHealthHistoryHandler(svcCtx))
	r.POST("/api/v1/mental-health/trigger", handler.MentalHealthTriggerHandler(svcCtx))
	r.GET("/api/v1/mental-health/trend", handler.MentalHealthTrendHandler(svcCtx))
}

func openPostgres(dsn string, maxOpen, maxIdle int) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(time.Hour)
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("db ping failed: %w", err)
	}
	return db, nil
}

// suppress unused import in some build configs
var _ = strconv.Itoa
