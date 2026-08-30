// analytics-svc main 入口（Gin 版本，2026-07-14 迁移自 go-zero）
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"emotion-echo-analytics-svc/internal/config"
	"emotion-echo-analytics-svc/internal/events"
	"emotion-echo-analytics-svc/internal/handler"
	"emotion-echo-analytics-svc/internal/kafka"
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

	// v1 启动时刷新 mv_daily_emotion（migration 003；pg_cron 调度留 Stage-2）。
	// 非致命：MV 失败不影响实时 VIEW 查询（reports 端点走 daily_emotion_v）。
	if db != nil {
		if err := db.Exec("REFRESH MATERIALIZED VIEW emotion_echo_analytics.mv_daily_emotion").Error; err != nil {
			log.Printf("[postgres] refresh mv_daily_emotion failed (non-fatal): %v", err)
		} else {
			log.Printf("[postgres] refreshed mv_daily_emotion")
		}
	}

	// Round 3：MentalHealthRepo（跨 schema 只读）
	var mhRepo repository.MentalHealthRepo
	if db != nil {
		mhRepo = repository.NewPostgresMentalHealthRepo(db)
	}

	// Round 3 part 2：TriggerQueue（异步评估）— worker body 用真实 runner
	queueCap := 64
	if c.TriggerQueueCap > 0 {
		queueCap = c.TriggerQueueCap
	}
	workers := 4
	var runner *trigger.MentalHealthRunner
	if db != nil {
		runner = trigger.NewMentalHealthRunner(mhRepo, trigger.NewPostgresJobStore(db))
	}
	tq := trigger.NewTriggerQueue(context.Background(), workers, queueCap, func(ctx context.Context, req trigger.Request) {
		// worker body：真实 mental-health 评估执行（migration 005 任务状态机）
		if runner == nil {
			log.Printf("[trigger] runner not configured (no postgres), skip task %s", req.TraceID)
			return
		}
		if err := runner.Run(ctx, req); err != nil {
			log.Printf("[trigger] task %s failed (user=%d type=%s): %v", req.TraceID, req.UserID, req.AssessmentType, err)
			return
		}
		log.Printf("[trigger] task %s done (user=%d type=%s)", req.TraceID, req.UserID, req.AssessmentType)
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

	// 3.5 Kafka consumer（chat-events → user_behavior_events）
	// 依赖 evtRepo（Postgres 可达）才启动；Enabled 显式开启（opt-in）。
	// topic 不存在 / broker 不可达 → log warn 不 crash（Consumer.Run 自带 5s 重试）。
	appCtx, stopConsumer := context.WithCancel(context.Background())
	defer stopConsumer()
	if c.Kafka.Enabled && evtRepo != nil {
		brokers := splitBrokersCSV(c.Kafka.BrokersCSV)
		topic := events.TopicChatEvents
		if len(c.Kafka.Topics) > 0 {
			topic = c.Kafka.Topics[0]
		}
		kc, err := kafka.NewConsumer(brokers, c.Kafka.GroupID, topic, evtRepo)
		if err != nil {
			log.Printf("[kafka] consumer init failed: %v (behavior events disabled)", err)
		} else {
			go func() {
				if err := kc.Run(appCtx); err != nil && err != context.Canceled {
					log.Printf("[kafka] consumer exited: %v", err)
				}
			}()
			log.Printf("[kafka] consumer started (topic=%s group=%s brokers=%v)", topic, c.Kafka.GroupID, brokers)
		}
	}

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

// splitBrokersCSV 把 "broker1:9092,broker2:9092" 切分成 []string
//
// 与 chat-svc / ai-svc 的 kafkaBrokers 行为一致（Stage 26-P 引入）。
func splitBrokersCSV(csv string) []string {
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// suppress unused import in some build configs
var _ = strconv.Itoa
