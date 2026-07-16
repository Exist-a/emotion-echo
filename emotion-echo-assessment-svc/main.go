// assessment-svc main 入口（Gin 版本，2026-07-14 迁移自 go-zero）
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"emotion-echo-assessment-svc/internal/config"
	"emotion-echo-assessment-svc/internal/handler"
	"emotion-echo-assessment-svc/internal/repository"
	"emotion-echo-assessment-svc/internal/svc"

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

var configFile = flag.String("f", "etc/assessment-api.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	surveyRepo, err := openPostgres(c.Postgres.DSN, c.Postgres.MaxOpenConns, c.Postgres.MaxIdleConns)
	if err != nil {
		log.Printf("[postgres] connect failed: %v", err)
	} else {
		log.Printf("[postgres] connected")
	}

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

	svcCtx := svc.NewServiceContext(c, surveyRepo)

	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(sharedmetrics.GinMetricsMiddleware("assessment-svc"))
	if tracer != nil {
		r.Use(sharedmw.GinSkywalkingMiddleware(tracer))
	}
	r.Use(sharedmw.GinAuthMiddleware())

	r.GET("/health", handler.HealthHandler(svcCtx))
	r.GET("/metrics", gin.WrapH(sharedmetrics.PromHTTPHandler()))
	r.GET("/api/v1/surveys", handler.ListSurveysHandler(svcCtx))
	r.GET("/api/v1/surveys/:id", handler.GetSurveyHandler(svcCtx))
	r.POST("/api/v1/surveys/:id/submit", handler.SubmitSurveyHandler(svcCtx))
	r.GET("/api/v1/surveys/results", handler.ListMyResultsHandler(svcCtx))
	r.GET("/api/v1/surveys/results/:resultId", handler.GetSurveyResultHandler(svcCtx))

	log.Printf("Starting assessment-svc at %s:%d...", c.Host, c.Port)
	if err := r.Run(fmt.Sprintf("%s:%d", c.Host, c.Port)); err != nil {
		log.Fatalf("[gin] server crashed: %v", err)
	}
}

func openPostgres(dsn string, maxOpen, maxIdle int) (repository.SurveyRepo, error) {
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
	return repository.NewPostgresSurveyRepo(db), nil
}
