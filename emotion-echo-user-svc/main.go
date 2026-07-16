// user-svc main 入口（Gin 版本）
//
// 改造记录（2026-07-14）：
//   - HTTP server 从 go-zero rest 迁移到 Gin v1.x
//   - 鉴权从 svc mock 改为 shared/pkg/middleware.GinAuthMiddleware（信任 APISIX JWT）
//   - 链路追踪从 go-zero middleware 改为 shared/pkg/middleware.GinSkywalkingMiddleware
//   - 业务 logic 不变（保持稳定）
package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"emotion-echo-user-svc/internal/config"
	"emotion-echo-user-svc/internal/handler"
	"emotion-echo-user-svc/internal/repository"
	"emotion-echo-user-svc/internal/svc"

	"github.com/SkyAPM/go2sky"
	"github.com/SkyAPM/go2sky/reporter"
	"github.com/gin-gonic/gin"
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
	"github.com/zeromicro/go-zero/core/conf"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

var configFile = flag.String("f", "etc/user-api.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c) // 继续使用 go-zero 的 conf 库读 yaml（仅 IO 工具）

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

	// === 4. Gin router ===
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())

	// 中间件顺序：auth 必须在 trace 之后（trace 数据应包含 auth 后的 ctx）
	if tracer != nil {
		r.Use(sharedmw.GinSkywalkingMiddleware(tracer))
	}
	r.Use(sharedmw.GinAuthMiddleware())

	// === 5. 路由注册 ===
	// health 不需要鉴权（中间件内已跳过 /health）
	r.GET("/health", handler.HealthHandler(svcCtx))
	r.GET("/api/v1/users/me", handler.GetMeHandler(svcCtx))
	r.PATCH("/api/v1/users/me", handler.UpdateProfileHandler(svcCtx))
	r.GET("/api/v1/users/:id", handler.GetUserByIdHandler(svcCtx))

	// === 6. 启动 ===
	log.Printf("Starting server at %s:%d...", c.Host, c.Port)
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
