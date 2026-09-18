// user-svc main 入口（Gin 版本）
//
// 改造记录（2026-07-14）：
//   - HTTP server 从 go-zero rest 迁移到 Gin v1.x
//   - 鉴权从 svc mock 改为 shared/pkg/middleware.GinAuthMiddleware（信任 APISIX JWT）
//   - 链路追踪从 go-zero middleware 改为 shared/pkg/middleware.GinSkywalkingMiddleware
//   - 业务 logic 不变（保持稳定）
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"emotion-echo-user-svc/internal/config"
	"emotion-echo-user-svc/internal/grpcserver"
	"emotion-echo-user-svc/internal/handler"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"

	"github.com/SkyAPM/go2sky"
	"github.com/gin-gonic/gin"
	sharedbootstrap "github.com/emotion-echo/shared/pkg/bootstrap"
	dbconnect "github.com/emotion-echo/shared/pkg/dbconnect"
	sharedconfig "github.com/emotion-echo/shared/pkg/config"
	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"
	sharedlogging "github.com/emotion-echo/shared/pkg/logging"
	sharedmetrics "github.com/emotion-echo/shared/pkg/metrics"
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
	sharedgrpc "github.com/emotion-echo/shared/pkg/grpcinterceptor"
	sharedskywalking "github.com/emotion-echo/shared/pkg/skywalking"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var configFile = flag.String("f", "etc/user-api.yaml", "the config file")

// applyEnvOverrides 让容器内的 ${POSTGRES_DSN} / ${SKYWALKING_OAP_ADDR} /
// ${NACOS_*} env 在 go-zero conf.MustLoad 之后覆盖 config struct（go-zero 1.10
// 不展开 ${VAR:-default}，原样保留字面量；与 chat-svc/ai-svc 同模式，Stage 30
// 容器化补充）。
func applyEnvOverrides(c *config.Config) {
	if v := os.Getenv("POSTGRES_DSN"); v != "" {
		c.Postgres.DSN = v
	}
	if v := os.Getenv("SKYWALKING_OAP_ADDR"); v != "" {
		c.SkyWalking.OAPAddr = v
	}
	if v := os.Getenv("SKYWALKING_ENABLED"); v != "" {
		c.SkyWalking.Enabled = v == "true" || v == "1"
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

	// PR-OBS-15: structured slog JSON to stdout + svc 字段(决策 6 必填)
	sharedlogging.Init()
	sharedlogging.SetGlobalSvc("user-svc")

	var c config.Config
	sharedconfig.MustLoad(*configFile, &c, func() { config.SetDefaults(&c) })
	applyEnvOverrides(&c)

	// === 1. Postgres 连接（Stage 77：失败按 500ms×10 退避重试，盖过瞬时 DNS 抖动；
	// 重试耗尽仍失败才降级 nil repo——dev 阶段不阻断，但 health 接口会显示 dbOk=false） ===
	var userRepo repository.UserRepo
	var securityAnswerRepo repository.SecurityAnswerRepo
	db, err := openPostgresDB(c.Postgres.DSN, c.Postgres.MaxOpenConns, c.Postgres.MaxIdleConns)
	if err != nil {
		log.Printf("[postgres] connect failed: %v", err)
	} else {
		userRepo = repository.NewPostgresUserRepo(db)
		securityAnswerRepo = repository.NewPostgresSecurityAnswerRepo(db)
		log.Printf("[postgres] connected, dsn=%s", maskDSN(c.Postgres.DSN))
	}

	// === 2. SkyWalking tracer (PR-OBS-2: 用 shared BootstrapSkyWalkingTracer 统一 7 svc 行为) ===
	var tracer *go2sky.Tracer
	if c.SkyWalking.Enabled {
		svcName := c.SkyWalking.ServiceName
		if svcName == "" {
			svcName = c.Name
		}
		t, err := sharedbootstrap.BootstrapSkyWalkingTracer(context.Background(), svcName, c.SkyWalking.OAPAddr, 2*time.Second)
		if err != nil {
			sharedmetrics.IncSkyWalkingInitFailed(svcName)
			if sharedbootstrap.ShouldFailFast() && sharedbootstrap.IsRequired("skywalking") {
				log.Fatalf("[skywalking] strict mode + required dep, refusing to start: %v", err)
			}
			log.Printf("[skywalking] tracer init failed (warn mode, continue): %v", err)
		} else {
			tracer = t
			log.Printf("[skywalking] tracer initialized, oap=%s service=%s (PR-OBS-2 helper)", c.SkyWalking.OAPAddr, svcName)
		}
	}

	// === 3. ServiceContext（依赖注入容器） ===
	svcCtx := svc.NewServiceContext(c, userRepo, securityAnswerRepo)

	// === 3.5 Nacos 注册中心 + 配置中心（Stage 31 PR-07） ===
	bootCtx, bootCancel := context.WithCancel(context.Background())
	defer bootCancel()

	nacosRuntime, err := BootNacos(bootCtx, &c, defaultBootDeps())
	if err != nil {
		// PR-5: hard error（WaitForNacos / Register 失败）→ fail-fast，dev 不再"假活着"。
		if shareddiscovery.IsHardBootError(err.Error()) {
			log.Fatalf("[nacos] boot failed (fatal): %v", err)
		}
		log.Printf("[nacos] boot failed (continuing): %v", err)
	}
	defer func() {
		if nacosRuntime != nil {
			nacosRuntime.Close(bootCtx, c.Name, c.Host, c.Port)
		}
	}()

	// === 4. Gin router ===
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(sharedmetrics.GinMetricsMiddleware("user-svc"))

	// 中间件顺序：auth 必须在 trace 之后（trace 数据应包含 auth 后的 ctx）
	if tracer != nil {
		r.Use(sharedmw.GinSkywalkingMiddleware(sharedgrpc.NewGo2SkyTracer(tracer)))
	}

	// === 4.5 Stage 33 PR-19a：无 auth 中间件的路由组 ===
	// login/register 在认证前调用，必须在 GinAuthMiddleware 之前注册。
	// （Gin 的路由组注册顺序决定中间件作用范围）
	noAuth := r.Group("/api/v1/users")
	{
		noAuth.POST("/login", handler.LoginHandler(svcCtx))
		noAuth.POST("/register", handler.RegisterHandler(svcCtx))
		// Sprint 1 PR-4c-3: 密码重置（BFF 校验 verification-code 后调）
		noAuth.POST("/reset-password", handler.ResetPasswordHandler(svcCtx))
		// R-01 #1: 密保答案验证（供 BFF 找回密码流程）
		noAuth.POST("/verify-security-answer", handler.VerifySecurityAnswerHandler(svcCtx))
	}

	r.Use(sharedmw.GinAuthMiddleware())

	// === 5. 路由注册 ===
	// health 不需要鉴权（中间件内已跳过 /health）
	r.GET("/health", handler.HealthHandler(svcCtx))
	r.GET("/metrics", gin.WrapH(sharedmetrics.PromHTTPHandler()))
	r.GET("/api/v1/users/me", handler.GetMeHandler(svcCtx))
	r.PATCH("/api/v1/users/me", handler.UpdateProfileHandler(svcCtx))
	r.GET("/api/v1/users/:id", handler.GetUserByIdHandler(svcCtx))

	// === 6. 启动 ===
	log.Printf("Starting server at %s:%d...", c.Host, c.Port)

	// 优雅退出：SIGINT/SIGTERM → Unregister + close
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
		log.Printf("[signal] received, shutting down gracefully...")
		bootCancel()
		if nacosRuntime != nil {
			nacosRuntime.Close(context.Background(), c.Name, c.Host, c.Port)
		}
		os.Exit(0)
	}()

	// Stage 62 PR-3.2：双轨启动 — Gin HTTP (:8888) + gRPC (:8887)
	// gRPC 端口从 yaml GRPC.Port 读；空则不启动（向后兼容老 yaml）
	grpcPort := c.GRPC.Port
	if grpcPort > 0 && svcCtx != nil {
		gs := grpcserver.New(svcCtx, grpcPort)
		go func() {
			if err := gs.Start(bootCtx); err != nil {
				log.Printf("[grpc] user-svc gRPC server failed: %v", err)
			}
		}()
	} else {
		log.Printf("[grpc] user-svc gRPC server 跳过（GRPC.Port=%d, svcCtx=%v）", grpcPort, svcCtx != nil)
	}

	if err := r.Run(fmt.Sprintf("%s:%d", c.Host, c.Port)); err != nil {
		log.Fatalf("[gin] server crashed: %v", err)
	}
}

func openPostgresDB(dsn string, maxOpen, maxIdle int) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Warn),
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	// 第一步：按 yaml 配置设默认值（向后兼容 Stage 50 行为）。
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(time.Hour)
	// 第二步：Round 4.4 PR-2 — env 覆盖 yaml。非法 env 立即 fail-fast。
	if err := dbconnect.ApplyPoolEnv(sqlDB); err != nil {
		return nil, fmt.Errorf("apply pool env failed: %w", err)
	}
	// Round 4.4 PR-1：SkyWalking GORM trace 接入。
	sharedskywalking.InitGORM(db)
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("db ping failed: %w", err)
	}
	return db, nil
}

func maskDSN(dsn string) string {
	at := strings.Index(dsn, "@")
	if at < 0 {
		return dsn
	}
	colon := strings.Index(dsn[:at], ":")
	if colon < 0 {
		return dsn
	}
	prefix := dsn[:colon+1]
	rest := dsn[colon+1 : at]
	if strings.Contains(rest, ":") {
		c2 := strings.Index(rest, ":")
		return prefix + rest[:c2+1] + "***" + dsn[at:]
	}
	return dsn
}
