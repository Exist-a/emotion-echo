// Package main is the entrypoint for emotion-echo-ai-svc.
//
// Responsibilities:
//   - HTTP API (Gin) for emotion queries + Stage 23 multimodal endpoints
//   - gRPC server for emotion.AI
//   - Kafka consumer that listens to chat-events and runs emotion analysis
//   - Optional clients for FER / SenseVoice / XTTS (Stage 22 multimodal)
//   - SkyWalking tracing, Prometheus metrics, structured logging
//   - Graceful shutdown with 10s drain timeout
//
// Stage 22-B: env-var overrides because go-zero conf does NOT parse
// ${VAR:-default} style envsubst. We read OS env after conf.MustLoad
// and patch c.* fields so container-side DNS works.
//
// Stage 23: AI gateway endpoints (multimodal analyze, TTS synthesize,
// AI service health).
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"emotion-echo-ai-svc/internal/analyzer"
	"emotion-echo-ai-svc/internal/bootstrap"
	"emotion-echo-ai-svc/internal/config"
	"emotion-echo-ai-svc/internal/consumer"
	"emotion-echo-ai-svc/internal/events"
	"emotion-echo-ai-svc/internal/grpcserver"
	"emotion-echo-ai-svc/internal/handler"
	"emotion-echo-ai-svc/internal/logging"
	"emotion-echo-ai-svc/internal/logic"
	"emotion-echo-ai-svc/internal/repository"
	"emotion-echo-ai-svc/internal/svc"

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

var configFile = flag.String("f", "etc/ai-api.yaml", "the config file")

// failFastIfRequired logs an error and aborts only when STARTUP_STRICT=true
// AND the dependency is in the required-deps list. Otherwise it just logs.
func failFastIfRequired(dep string, err error, addr string) {
	if err == nil {
		return
	}
	if bootstrap.ShouldFailFast() && bootstrap.IsRequired(dep) {
		logging.Errorf(err, "[startup-strict] required dependency unavailable, refusing to start: dep=%s addr=%s", dep, addr)
		logging.Fatalf("[startup-strict] exit code=1 (dep=%s)", dep)
	}
	logging.Errorf(err, "[startup] dependency check failed (non-strict): dep=%s addr=%s", dep, addr)
}

// applyEnvOverrides reads OS env vars and patches c.* fields.
//
// go-zero conf does NOT parse ${VAR:-default} bash-style substitution.
// ai-api.yaml therefore embeds those literals and they are NOT resolved
// at runtime. This function manually looks up env vars and overrides
// the corresponding fields. Precedence: env > yaml.
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
	if v := os.Getenv("LLM_BASE_URL"); v != "" {
		c.LLM.BaseURL = v
	}
	if v := os.Getenv("LLM_GRPC_ADDR"); v != "" {
		c.LLM.GRPCAddr = v
	}
	if v := os.Getenv("LLM_INTERNAL_API_KEY"); v != "" {
		c.LLM.InternalAPIKey = v
	}
	if v := os.Getenv("FER_BASE_URL"); v != "" {
		c.FER.BaseURL = v
	}
	if v := os.Getenv("SENSEVOICE_BASE_URL"); v != "" {
		c.SenseVoice.BaseURL = v
	}
	if v := os.Getenv("XTTS_BASE_URL"); v != "" {
		c.XTTS.BaseURL = v
	}
}

// applyDefaultFallbacks fills in safe defaults when both yaml and env are empty.
// This happens for fields that ai-api.yaml expresses via ${VAR:-default}
// (which go-zero ignores). Without fallbacks the service would crash on
// startup when running outside compose.
func applyDefaultFallbacks(c *config.Config) {
	if c.Postgres.DSN == "" {
		c.Postgres.DSN = "host=localhost port=5432 user=postgres password=postgres dbname=emotion_echo sslmode=disable search_path=emotion_echo_ai"
	}
	if c.Kafka.BrokersCSV == "" {
		c.Kafka.BrokersCSV = "localhost:9092"
	}
	if c.SkyWalking.OAPAddr == "" {
		c.SkyWalking.OAPAddr = "localhost:11800"
	}
	if c.LLM.BaseURL == "" {
		c.LLM.BaseURL = "http://localhost:8000"
	}
	if c.LLM.GRPCAddr == "" {
		c.LLM.GRPCAddr = "localhost:50051"
	}
}

func main() {
	flag.Parse()

	// Stage 20-4: structured slog JSON to stdout.
	logging.Init()
	logging.Printf("[startup] ai-svc starting (strict=%v deps=%s)",
		bootstrap.ShouldFailFast(), os.Getenv("STARTUP_STRICT_DEPS"))

	var c config.Config
	conf.MustLoad(*configFile, &c)

	// Stage 22-B: env override + default fallbacks (see doc comments above).
	applyEnvOverrides(&c)
	applyDefaultFallbacks(&c)

	// BrokersCSV -> []string once.
	kafkaBrokersList := kafkaBrokers(c.Kafka.BrokersCSV)

	// Stage 20-P0-3: startup dependency probes (fail-fast).
	depCtx, depCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer depCancel()
	checks := map[string]string{}
	if c.Postgres.DSN != "" {
		host := "localhost"
		port := "5432"
		for _, kv := range strings.FieldsFunc(c.Postgres.DSN, func(r rune) bool { return r == ' ' }) {
			if strings.HasPrefix(kv, "host=") {
				host = strings.TrimPrefix(kv, "host=")
			} else if strings.HasPrefix(kv, "port=") {
				port = strings.TrimPrefix(kv, "port=")
			}
		}
		checks["postgres"] = host + ":" + port
	}
	if len(kafkaBrokersList) > 0 {
		checks["kafka"] = kafkaBrokersList[0]
	}
	if c.SkyWalking.OAPAddr != "" {
		checks["skywalking"] = c.SkyWalking.OAPAddr
	}
	if c.LLM.GRPCAddr != "" {
		checks["llm"] = c.LLM.GRPCAddr
	}
	if len(checks) > 0 {
		results := bootstrap.CheckMultiple(depCtx, checks, 3*time.Second)
		for name, err := range results {
			failFastIfRequired(name, err, checks[name])
		}
	}

	// 1. Postgres
	emoRepo, err := openPostgres(c.Postgres.DSN, c.Postgres.MaxOpenConns, c.Postgres.MaxIdleConns)
	if err != nil {
		logging.Errorf(err, "[postgres] connect failed")
		if bootstrap.ShouldFailFast() && bootstrap.IsRequired("postgres") {
			logging.Fatalf("[postgres] strict mode + required dep, refusing to start")
		}
	} else {
		logging.Printf("[postgres] connected")
	}

	// 2. SkyWalking
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
				logging.Printf("[skywalking] tracer initialized")
			}
		} else {
			logging.Errorf(err, "[skywalking] reporter init failed (will not trace)")
			if bootstrap.ShouldFailFast() && bootstrap.IsRequired("skywalking") {
				logging.Fatalf("[skywalking] strict mode + required dep, refusing to start")
			}
		}
	}

	// 3. Kafka Consumer
	if c.Kafka.Enabled && len(kafkaBrokersList) > 0 {
		kc, err := consumer.NewKafkaConsumer(kafkaBrokersList, c.Kafka.GroupID)
		if err != nil {
			logging.Errorf(err, "[kafka] consumer init failed")
			if bootstrap.ShouldFailFast() && bootstrap.IsRequired("kafka") {
				logging.Fatalf("[kafka] strict mode + required dep, refusing to start")
			}
		} else {
			var an analyzer.Analyzer
			if c.LLM.Enabled {
				grpcAddr := c.LLM.GRPCAddr
				if grpcAddr == "" {
					grpcAddr = "localhost:50051"
				}
				grpcAn, err := analyzer.NewGRPCAnalyzer(grpcAddr)
				if err != nil {
					logging.Errorf(err, "[llm] gRPC dial failed, fallback to HTTP")
					an = analyzer.NewChainedAnalyzer(
						analyzer.NewHTTPAnalyzer(c.LLM.BaseURL),
						analyzer.NewKeywordAnalyzer(),
					)
				} else {
					apiKey := c.LLM.InternalAPIKey
					authWrapped := analyzer.NewAuthWrappedAnalyzer(grpcAn, apiKey)
					logging.Printf("[llm] using gRPC analyzer (target=%s, auth=%s) + keyword fallback",
						grpcAddr, apiKeyStatus(apiKey))
					an = analyzer.NewChainedAnalyzer(authWrapped, analyzer.NewKeywordAnalyzer())
					defer grpcAn.Close()
				}
			} else {
				an = analyzer.NewKeywordAnalyzer()
				logging.Printf("[llm] using keyword analyzer (LLM disabled)")
			}
			createdHandler := logic.NewMessageCreatedHandler(emoRepo, an)

			go func() {
				logging.Printf("[kafka] consumer started: brokers=%v topics=%v groupID=%s",
					kafkaBrokersList, c.Kafka.Topics, c.Kafka.GroupID)
				if err := kc.Consume(context.Background(), c.Kafka.Topics,
					func(ctx context.Context, evt *events.Event) error {
						return createdHandler.Handle(ctx, evt)
					},
					events.EventTypeMessageCreated,
					tracer); err != nil {
					logging.Errorf(err, "[kafka] consume err")
				}
			}()
			defer func() { _ = kc.Close() }()
		}
	}

	// 4. ServiceContext
	svcCtx := svc.NewServiceContext(c, emoRepo)

	// Stage 22-A.5: build 3 AI model clients + MultiModalAnalyzer.
	svcCtx.InitMultiModal()
	if svcCtx.FER != nil {
		logging.Printf("[ai] FER client active: %s", c.FER.BaseURL)
	} else {
		logging.Printf("[ai] FER disabled (FER.BaseURL empty)")
	}
	if svcCtx.SenseVoice != nil {
		logging.Printf("[ai] SenseVoice client active: %s", c.SenseVoice.BaseURL)
	} else {
		logging.Printf("[ai] SenseVoice disabled (SenseVoice.BaseURL empty)")
	}
	if svcCtx.XTTS != nil {
		logging.Printf("[ai] XTTS client active: %s lang=%s speed=%.2f",
			c.XTTS.BaseURL, c.XTTS.Language, c.XTTS.Speed)
	} else {
		logging.Printf("[ai] XTTS disabled (XTTS.BaseURL empty)")
	}

	// 5. Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(sharedmetrics.GinMetricsMiddleware("ai-svc"))
	if tracer != nil {
		r.Use(sharedmw.GinSkywalkingMiddleware(tracer))
	}
	r.Use(sharedmw.GinAuthMiddleware())

	// 6. routes
	r.GET("/health", handler.HealthHandler(svcCtx))
	r.GET("/api/v1/emotion/message/:messageId", handler.GetEmotionByMessageHandler(svcCtx))
	r.GET("/api/v1/emotion/conversation/:conversationId", handler.ListEmotionByConversationHandler(svcCtx))
	// Stage 23: AI multimodal / TTS / AI health endpoints
	r.POST("/api/v1/multimodal/analyze", handler.MultiModalAnalyzeHandler(svcCtx))
	r.POST("/api/v1/tts/synthesize", handler.SynthesizeSpeechHandler(svcCtx))
	r.GET("/api/v1/ai/health", handler.AIHealthHandler(svcCtx))
	r.GET("/metrics", gin.WrapH(sharedmetrics.PromHTTPHandler()))

	logging.Printf("Starting ai-svc at %s:%d...", c.Host, c.Port)

	// Stage 20-3: graceful shutdown context.
	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	if c.GRPC.Enabled {
		gs := grpcserver.New(emoRepo, c.GRPC.Port)
		go func() {
			if err := gs.Start(rootCtx); err != nil {
				logging.Errorf(err, "[grpc] server failed")
			}
		}()
	}

	httpAddr := fmt.Sprintf("%s:%d", c.Host, c.Port)
	srv := &http.Server{
		Addr:              httpAddr,
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	logging.Printf("[http] ai-svc HTTP server listening on %s", httpAddr)

	httpErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			httpErr <- err
		}
		close(httpErr)
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-httpErr:
		if err != nil {
			logging.Fatalf("[http] server crashed: %v", err)
		}
	case sig := <-quit:
		logging.Printf("[shutdown] received %s, starting graceful shutdown (10s timeout)", sig)
		rootCancel()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			logging.Errorf(err, "[shutdown] http Shutdown error")
		} else {
			logging.Printf("[shutdown] http server stopped gracefully")
		}
		logging.Printf("[shutdown] ai-svc exited")
	}
}

func openPostgres(dsn string, maxOpen, maxIdle int) (repository.EmotionRepo, error) {
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
	return repository.NewPostgresEmotionRepo(db), nil
}

// apiKeyStatus returns "enabled" or "disabled" for log output.
func apiKeyStatus(key string) string {
	if key == "" {
		return "disabled"
	}
	return "enabled"
}

// kafkaBrokers parses KAFKA_BROKERS env (string) into []string.
//
// Accepts any of:
//   - "emotion-echo-kafka:9092"                  single broker
//   - "kafka1:9092,kafka2:9092"                  CSV list
//   - `["kafka1:9092","kafka2:9092"]`            JSON array (compose env single-quoted)
//
// go-zero conf does not natively support list env var substitution, so
// we keep BrokersCSV as a string and parse it ourselves.
func kafkaBrokers(csv string) []string {
	if csv == "" {
		return nil
	}
	// 1) JSON array form
	csv = strings.TrimSpace(csv)
	if strings.HasPrefix(csv, "[") {
		var arr []string
		if err := json.Unmarshal([]byte(csv), &arr); err == nil {
			return arr
		}
		// fall through to CSV split
	}
	// 2) CSV form (strip possible stray quotes/brackets)
	csv = strings.Trim(csv, `"[]`)
	parts := strings.Split(csv, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(strings.Trim(p, `"`)); p != "" {
			out = append(out, p)
		}
	}
	return out
}