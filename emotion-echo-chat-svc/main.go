// chat-svc main 入口（Gin 版本，2026-07-14 迁移自 go-zero）
//
// 4 路由：POST /api/v1/conversations + POST/GET messages + /health
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"emotion-echo-chat-svc/internal/config"
	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/handler"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"

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

var configFile = flag.String("f", "etc/chat-api.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	// 1. Postgres
	convRepo, err := openPostgres(c.Postgres.DSN, c.Postgres.MaxOpenConns, c.Postgres.MaxIdleConns)
	if err != nil {
		log.Printf("[postgres] connect failed: %v", err)
	} else {
		log.Printf("[postgres] connected")
	}

	// 2. Kafka publisher
	var pub events.EventPublisher = events.NewInMemoryEventPublisher()
	if c.Kafka.Enabled && len(c.Kafka.Brokers) > 0 {
		kp, err := events.NewKafkaEventPublisher(c.Kafka.Brokers)
		if err != nil {
			log.Printf("[kafka] producer init failed: %v (fallback to in-memory)", err)
		} else {
			pub = kp
			log.Printf("[kafka] producer connected, brokers=%v", c.Kafka.Brokers)
			defer func() { _ = kp.Close() }()
		}
	}

	// 3. SkyWalking
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

	// 4. ServiceContext
	svcCtx := svc.NewServiceContext(c, convRepo, pub)

	// 5. Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(sharedmetrics.GinMetricsMiddleware("chat-svc"))
	if tracer != nil {
		r.Use(sharedmw.GinSkywalkingMiddleware(tracer))
	}
	r.Use(sharedmw.GinAuthMiddleware())

	// 6. routes
	r.GET("/health", handler.HealthHandler(svcCtx))
	r.GET("/metrics", gin.WrapH(sharedmetrics.PromHTTPHandler()))
	r.POST("/api/v1/conversations", handler.CreateConversationHandler(svcCtx))
	r.POST("/api/v1/conversations/:id/messages", handler.SendMessageHandler(svcCtx))
	r.GET("/api/v1/conversations/:id/messages", handler.ListMessagesHandler(svcCtx))

	log.Printf("Starting chat-svc at %s:%d...", c.Host, c.Port)
	if err := r.Run(fmt.Sprintf("%s:%d", c.Host, c.Port)); err != nil {
		log.Fatalf("[gin] server crashed: %v", err)
	}
}

func openPostgres(dsn string, maxOpen, maxIdle int) (repository.ConversationRepo, error) {
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
	return repository.NewPostgresConversationRepo(db), nil
}
