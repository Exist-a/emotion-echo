// chat-svc main 入口（Gin 版本，2026-07-14 迁移自 go-zero）
//
// 4 路由：POST /api/v1/conversations + POST/GET messages + /health
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"emotion-echo-chat-svc/internal/config"
	"emotion-echo-chat-svc/internal/events"
	"emotion-echo-chat-svc/internal/grpcclient"
	"emotion-echo-chat-svc/internal/handler"
	"emotion-echo-chat-svc/internal/outbox"
	"emotion-echo-chat-svc/internal/repository"
	"emotion-echo-chat-svc/internal/svc"

	"github.com/SkyAPM/go2sky"
	"github.com/SkyAPM/go2sky/reporter"
	"github.com/gin-gonic/gin"
	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"
	sharedmetrics "github.com/emotion-echo/shared/pkg/metrics"
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
	"github.com/zeromicro/go-zero/core/conf"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var configFile = flag.String("f", "etc/chat-api.yaml", "the config file")

// applyEnvOverrides 让容器内的 ${POSTGRES_DSN} / ${KAFKA_BROKERS} /
// ${SKYWALKING_OAP_ADDR} env 在 go-zero conf.MustLoad 之后覆盖 config struct,
// 避免 go-zero 1.10 conf bug — 它不识别 "${VAR:default}" bash default 语法,
// 原样保留字面字符串,所以需要在 main 显式兜底 (Stage 26-Q 修)。
// 模式与 ai-svc 内部 applyEnvOverrides() 一致 (Stage 20-P0-1)。
func applyEnvOverrides(c *config.Config) {
	if v := os.Getenv("POSTGRES_DSN"); v != "" {
		c.Postgres.DSN = v
	}
	if v := os.Getenv("KAFKA_BROKERS"); v != "" {
		c.Kafka.BrokersCSV = v
	}
	if v := os.Getenv("SKYWALKING_OAP_ADDR"); v != "" {
		c.SkyWalking.OAPAddr = v
	}
	if v := os.Getenv("NACOS_ENABLED"); v != "" {
		c.Nacos.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("NACOS_ADDR"); v != "" {
		c.Nacos.Addr = v
	}
	if v := os.Getenv("NACOS_NAMESPACE"); v != "" {
		c.Nacos.Namespace = v
	}
	if v := os.Getenv("NACOS_HOT_RELOAD"); v != "" {
		c.Nacos.HotReload = v == "true" || v == "1"
	}
}

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)
	applyEnvOverrides(&c)

	// 1. Postgres
	convRepo, db, err := openPostgres(c.Postgres.DSN, c.Postgres.MaxOpenConns, c.Postgres.MaxIdleConns)
	if err != nil {
		log.Printf("[postgres] connect failed: %v", err)
	} else {
		log.Printf("[postgres] connected")
	}

	// 1.1 Stage 30-C A3: Outbox 迁移 + 表初始化（如果 DB 可达）
	var outboxRepo repository.OutboxRepo
	if db != nil {
		outboxRepo = repository.NewPostgresOutboxRepo(db)
		if err := runOutboxMigration(db); err != nil {
			log.Printf("[outbox] migration failed: %v", err)
		}
	}

	// 2. Kafka publisher
	// Kafka.BrokersCSV 是 Stage 26-P 改造后从 yaml list 改为 CSV 字符串,
	// 因为 go-zero conf 不原生支持 ${ENV} 占位在 list 字段上展开
	// (同 ai-svc 范式)。容器内由 KAFKA_BROKERS env 注入。
	kafkaBrokersList := splitBrokersCSV(c.Kafka.BrokersCSV)
	var pub events.EventPublisher = events.NewInMemoryEventPublisher()
	if c.Kafka.Enabled && len(kafkaBrokersList) > 0 {
		kp, err := events.NewKafkaEventPublisher(kafkaBrokersList)
		if err != nil {
			log.Printf("[kafka] producer init failed: %v (fallback to in-memory)", err)
		} else {
			pub = kp
			log.Printf("[kafka] producer connected, brokers=%v", kafkaBrokersList)
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

	// 4. ServiceContext（Stage 30-C A3: 注入 DB + OutboxRepo + Stage 36-A3.2: AIClient）
	svcCtx := svc.NewServiceContext(c, convRepo, pub)
	if db != nil {
		svcCtx.WithDB(db)
	}
	if outboxRepo != nil {
		svcCtx.WithOutboxRepo(outboxRepo)
	}
	// Stage 36-A3.2: dial ai-svc gRPC 给 dev fallback 用。dial 失败不 hard fail——
	// dev fallback 是 best-effort，dial 失败就回退 NoopAIClient（SendMessageLogic 仍然
	// 工作，只是不调 ai-svc）。这样 compose dev 环境没起 ai-svc 也能启动 chat-svc。
	if c.AIService.GRPCAddr != "" {
		ai, err := grpcclient.NewAIgRPCClient(c.AIService.GRPCAddr)
		if err != nil {
			log.Printf("[ai-grpc] dial %s failed: %v (fallback to noop)", c.AIService.GRPCAddr, err)
		} else {
			svcCtx.WithAIClient(ai)
			log.Printf("[ai-grpc] connected, addr=%s (dev fallback enabled)", c.AIService.GRPCAddr)
			if closer, ok := ai.(interface{ Close() error }); ok {
				defer func() { _ = closer.Close() }()
			}
		}
	}
	if outboxRepo != nil {
		svcCtx.WithOutboxRepo(outboxRepo)
	}

	// 4.1 Stage 30-C A3: 启 Outbox relay goroutine（每 1s 扫一次）
	if db != nil && outboxRepo != nil && c.Kafka.Enabled && len(kafkaBrokersList) > 0 {
		relayCtx, relayCancel := context.WithCancel(context.Background())
		defer relayCancel()
		relay := outbox.NewRelay(outboxRepo, pub, 1*time.Second, 100)
		go func() {
			log.Printf("[outbox] relay started")
			_ = relay.Run(relayCtx)
		}()
		// 监听 SIGTERM/SIGINT 优雅退出
		go func() {
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
			<-sigCh
			relayCancel()
		}()
	}

	// 5. Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	// HandleMethodNotAllowed: true makes gin return 405 (instead of 404)
	// when the path matches a route registered for a different HTTP method.
	// Without this flag, gin collapses all unmatched methods into 404, which
	// obscures real client bugs (e.g. accidental POST vs GET).
	r.HandleMethodNotAllowed = true
	r.NoMethod(func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "method not allowed"})
	})
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	})
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
	r.GET("/api/v1/conversations", handler.ListConversationsHandler(svcCtx)) // Stage 36-A2.1 (G2 upper)
	r.POST("/api/v1/conversations/:id/messages", handler.SendMessageHandler(svcCtx))
	r.GET("/api/v1/conversations/:id/messages", handler.ListMessagesHandler(svcCtx))
	r.DELETE("/api/v1/conversations/:id", handler.DeleteConversationHandler(svcCtx))

	// 6.5 Stage 31 PR-08: Nacos 注册 + 配置
	bootCtx, bootCancel := context.WithCancel(context.Background())
	defer bootCancel()

	nacosRuntime, err := BootNacos(bootCtx, &c, defaultBootDeps())
	if err != nil {
		if shareddiscovery.IsHardBootError(err.Error()) {
			log.Fatalf("[nacos] boot failed (fatal): %v", err)
		}
		log.Printf("[nacos] boot failed (continuing): %v", err)
	}
	defer func() {
		if nacosRuntime != nil {
			nacosRuntime.Close(context.Background(), c.Name, c.Host, c.Port)
		}
	}()

	log.Printf("Starting chat-svc at %s:%d...", c.Host, c.Port)

	// 优雅退出：SIGINT/SIGTERM → Nacos Close
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Printf("[signal] received, shutting down...")
		bootCancel()
		if nacosRuntime != nil {
			nacosRuntime.Close(context.Background(), c.Name, c.Host, c.Port)
		}
		os.Exit(0)
	}()

	if err := r.Run(fmt.Sprintf("%s:%d", c.Host, c.Port)); err != nil {
		log.Fatalf("[gin] server crashed: %v", err)
	}
}

func openPostgres(dsn string, maxOpen, maxIdle int) (repository.ConversationRepo, *gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("db open failed: %w", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(time.Hour)
	if err := sqlDB.Ping(); err != nil {
		return nil, nil, fmt.Errorf("db ping failed: %w", err)
	}
	return repository.NewPostgresConversationRepo(db), db, nil
}

// runOutboxMigration 跑 migrations/001_create_outbox_events.sql（Stage 30-C A3）
//
// 简单实现：用 gorm 直接 Exec 文件内容。生产应该用 migrate 工具；当前学习/开发期 OK。
func runOutboxMigration(db *gorm.DB) error {
	const sql = `
CREATE SCHEMA IF NOT EXISTS emotion_echo_chat;

CREATE TABLE IF NOT EXISTS emotion_echo_chat.outbox_events (
    id          BIGSERIAL PRIMARY KEY,
    event_id    VARCHAR(64) NOT NULL UNIQUE,
    event_type  VARCHAR(64) NOT NULL,
    topic       VARCHAR(64) NOT NULL,
    payload     JSONB NOT NULL,
    status      VARCHAR(16) NOT NULL DEFAULT 'pending',
    attempts    INT NOT NULL DEFAULT 0,
    last_error  TEXT,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_outbox_pending
    ON emotion_echo_chat.outbox_events(created_at)
    WHERE status = 'pending';
`
	return db.Exec(sql).Error
}

// splitBrokersCSV 把 c.Kafka.BrokersCSV (`"broker1:9092,broker2:9092"`) 切分
// 成 []string,作为 events.NewKafkaEventPublisher 的输入。
// 与 ai-svc 内部的 kafkaBrokers() 行为一致;Stage 26-P · Commit P3 引入。
func splitBrokersCSV(csv string) []string {
	csv = strings.TrimSpace(csv)
	if csv == "" {
		return nil
	}
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
