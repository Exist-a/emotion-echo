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
	"emotion-echo-user-svc/internal/handler"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"

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
	conf.MustLoad(*configFile, &c) // 继续使用 go-zero 的 conf 库读 yaml（仅 IO 工具）
	applyEnvOverrides(&c)

	// === 1. Postgres 连接 ===
	userRepo, err := openPostgres(c.Postgres.DSN, c.Postgres.MaxOpenConns, c.Postgres.MaxIdleConns)
	if err != nil {
		log.Printf("[postgres] connect failed: %v", err)
		// dev 阶段不阻断：让 svc 起，但 health 接口会显示 dbOk=false
	}
	if userRepo != nil {
		log.Printf("[postgres] connected, dsn=%s", maskDSN(c.Postgres.DSN))
	}

	// === 2. SkyWalking tracer ===
	var tracer *go2sky.Tracer
	if c.SkyWalking.Enabled {
		rep, err := reporter.NewGRPCReporter(c.SkyWalking.OAPAddr)
		if err != nil {
			log.Printf("[skywalking] reporter init failed: %v", err)
		} else {
			svcName := c.SkyWalking.ServiceName
			if svcName == "" {
				svcName = c.Name
			}
			tracer, err = go2sky.NewTracer(svcName, go2sky.WithReporter(rep))
			if err != nil {
				log.Printf("[skywalking] tracer init failed: %v", err)
			} else {
				log.Printf("[skywalking] tracer initialized, oap=%s service=%s", c.SkyWalking.OAPAddr, svcName)
			}
		}
	}

	// === 3. ServiceContext（依赖注入容器） ===
	svcCtx := svc.NewServiceContext(c, userRepo)

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
		r.Use(sharedmw.GinSkywalkingMiddleware(tracer))
	}

	// === 4.5 Stage 33 PR-19a：无 auth 中间件的路由组 ===
	// login/register 在认证前调用，必须在 GinAuthMiddleware 之前注册。
	// （Gin 的路由组注册顺序决定中间件作用范围）
	noAuth := r.Group("/api/v1/users")
	{
		noAuth.POST("/login", handler.LoginHandler(svcCtx))
		noAuth.POST("/register", handler.RegisterHandler(svcCtx))
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

	if err := r.Run(fmt.Sprintf("%s:%d", c.Host, c.Port)); err != nil {
		log.Fatalf("[gin] server crashed: %v", err)
	}
}

func openPostgres(dsn string, maxOpen, maxIdle int) (repository.UserRepo, error) {
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
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetConnMaxLifetime(time.Hour)
	if err := sqlDB.Ping(); err != nil {
		return nil, fmt.Errorf("db ping failed: %w", err)
	}
	return repository.NewPostgresUserRepo(db), nil
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
