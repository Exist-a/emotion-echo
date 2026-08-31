// emotion-echo-web-bff main 入口
//
// Stage 32 PR-16: BFF 退化为纯聚合层。
//
// 职责（Stage 32 之后）：
//   - APISIX 网关层之后的纯聚合层（不再做鉴权 / CORS / 限流 / 熔断）
//   - 信任 APISIX 注入的 X-User-Id header（shared GinAuthMiddleware 解析）
//   - 聚合 5 个下游 + XTTS 直连 + ai-svc gRPC
//   - SSE 流式（/api/v1/ai/stream + /api/v1/tts/stream）
//   - 自有 mock 鉴权仅保留 login 端点（Stage 33 净化）
//
// 装配链：config → clients → ServiceContext → handlers → Gin router
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

	"emotion-echo-web-bff/internal/auth"
	"emotion-echo-web-bff/internal/config"
	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/handler"
	"emotion-echo-web-bff/internal/logging"
	"emotion-echo-web-bff/internal/svc"

	"github.com/SkyAPM/go2sky"
	"github.com/SkyAPM/go2sky/reporter"
	"github.com/gin-gonic/gin"
	sharedmetrics "github.com/emotion-echo/shared/pkg/metrics"
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
	"github.com/zeromicro/go-zero/core/conf"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var configFile = flag.String("f", "etc/web-bff.yaml", "the config file")

func main() {
	flag.Parse()
	logging.Init()

	var c config.Config
	conf.MustLoad(*configFile, &c)
	config.ApplyEnvOverrides(&c)

	// 1. 下游 client 装配
	svcCtx := buildServiceContext(&c)

	// 2. SkyWalking（可选）
	var tracer *go2sky.Tracer
	if c.SkyWalking.Enabled {
		rep, err := reporter.NewGRPCReporter(c.SkyWalking.OAPAddr)
		if err == nil {
			tracer, _ = go2sky.NewTracer(c.SkyWalking.ServiceName, go2sky.WithReporter(rep))
		}
	}

	// 3. Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(sharedmetrics.GinMetricsMiddleware("web-bff"))
	if tracer != nil {
		r.Use(sharedmw.GinSkywalkingMiddleware(tracer))
	}
	// Stage 32 PR-16: 鉴权由 APISIX jwt-auth 统一处理（注入 X-User-Id header），
	// BFF 信任 shared GinAuthMiddleware（解析 X-User-Id 注入 ctx）。
	// CORS 由 APISIX cors 插件统一配；BFF 不再回显 Origin。
	// /api/v1/auth/* 白名单：login/register/refresh 不需要 X-User-Id（用户未登录）。
	if c.TrustAPISIX {
		// 生产路径：APISIX 已注入 X-User-Id
		r.Use(authPathBypass(sharedmw.GinAuthMiddleware()))
	} else {
		// Dev fallback：本地直连 BFF 调试（不经过 APISIX），从 Authorization 解析 JWT
		log.Println("[warn] BFF_TRUST_APISIX=false: dev fallback to Authorization JWT parsing")
		r.Use(authPathBypass(sharedmw.GinAuthMiddleware()))
	}

	// 4. 路由（handler 装配）
	registerRoutes(r, svcCtx, &c)

	// Stage 31 PR-09: Nacos 注册 + 配置
	// BFF 也注册到 Nacos；Stage 32 APISIX nacos-discovery 插件会自动发现 BFF
	bootCtx, bootCancel := context.WithCancel(context.Background())
	defer bootCancel()
	nacosRuntime, err := BootNacos(bootCtx, &c, defaultBootDeps())
	if err != nil {
		log.Printf("[nacos] boot failed (continuing): %v", err)
	}
	defer func() {
		if nacosRuntime != nil {
			nacosRuntime.Close(context.Background(), c.Name, c.Host, c.Port)
		}
	}()

	log.Printf("Starting web-bff at %s:%d...", c.Host, c.Port)
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh
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

// authPathBypass 让 /api/v1/auth/* 路径跳过鉴权（BFF 自有 mock 登录端点）
// 白名单路径直接 c.Next()，否则执行传入的鉴权中间件。
func authPathBypass(authMW gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/auth/") {
			c.Next()
			return
		}
		authMW(c)
	}
}

// buildServiceContext 装配全部下游 client + auth manager
func buildServiceContext(c *config.Config) *svc.ServiceContext {
	svcCtx := svc.NewServiceContext(*c)

	// auth manager（自有 JWT 签发）
	mgr, err := auth.NewManager(c.Auth.JWTSecret, c.Auth.TokenTTLSeconds)
	if err != nil {
		log.Fatalf("[auth] JWT manager init failed: %v", err)
	}
	svcCtx.SetAuth(mgr)

	// HTTP clients（5 个下游 + XTTS）
	svcCtx.SetUser(downstream.NewUserClient(downstream.UserClientOptions{
		BaseURL: c.UserService.BaseURL, TimeoutMs: c.UserService.TimeoutMs,
	}))
	svcCtx.SetChat(downstream.NewChatClient(downstream.ChatClientOptions{
		BaseURL: c.ChatService.BaseURL, TimeoutMs: c.ChatService.TimeoutMs,
	}))
	svcCtx.SetAssessment(downstream.NewAssessmentClient(downstream.AssessmentClientOptions{
		BaseURL: c.AssessmentService.BaseURL, TimeoutMs: c.AssessmentService.TimeoutMs,
	}))
	svcCtx.SetAnalytics(downstream.NewAnalyticsClient(downstream.AnalyticsClientOptions{
		BaseURL: c.AnalyticsService.BaseURL, TimeoutMs: c.AnalyticsService.TimeoutMs,
	}))
	svcCtx.SetAI(downstream.NewAIClient(downstream.AIClientOptions{
		BaseURL: c.AIService.HTTPAddr, TimeoutMs: c.AIService.TimeoutMs,
	}))
	svcCtx.SetXTTS(downstream.NewXTTSClient(downstream.XTTSClientOptions{
		BaseURL: c.XTTS.BaseURL, TimeoutMs: c.XTTS.TimeoutMs,
	}))

	// ai-svc gRPC（EmotionQueryService）
	conn, err := grpc.NewClient(c.AIService.GRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Printf("[grpc] ai-svc dial failed: %v (emotion query disabled)", err)
	} else {
		svcCtx.SetEmotionQ(downstream.NewEmotionQueryClient(conn))
	}

	return svcCtx
}

// registerRoutes 注册全部 BFF 路由
func registerRoutes(r *gin.Engine, s *svc.ServiceContext, c *config.Config) {
	// health（聚合下游探测）— 免鉴权（GinAuthMiddleware 白名单已含 /health）
	r.GET("/health", handler.NewHealthHandler([]handler.DownstreamTarget{
		{Name: "user", BaseURL: c.UserService.BaseURL},
		{Name: "chat", BaseURL: c.ChatService.BaseURL},
		{Name: "assessment", BaseURL: c.AssessmentService.BaseURL},
		{Name: "analytics", BaseURL: c.AnalyticsService.BaseURL},
		{Name: "ai", BaseURL: c.AIService.HTTPAddr},
		{Name: "xtts", BaseURL: c.XTTS.BaseURL},
	}, time.Duration(c.Health.TimeoutMs)*time.Millisecond))
	r.GET("/metrics", gin.WrapH(sharedmetrics.PromHTTPHandler()))

	// auth（Stage 33 PR-19b：真实登录，注入 UserClient）
	r.POST("/api/v1/auth/:action", handler.NewAuthHandler(s.Auth, s.User))

	// 业务 handler（各自 Register）
	handler.NewUserHandler(s.User).Register(r)
	handler.NewChatHandler(s.Chat).Register(r)
	handler.NewSurveyHandler(s.Assessment).Register(r)
	handler.NewAnalyticsHandler(s.Analytics).Register(r)
	handler.NewMultimodalHandler(s.AI).Register(r)
	handler.NewTTSHandler(s.AI, s.XTTS).Register(r)
	handler.NewUploadHandler().Register(r)
	if s.EmotionQ != nil {
		handler.NewEmotionQueryHandler(s.EmotionQ).Register(r)
	}
	// SSE 流式
	r.POST("/api/v1/ai/stream", handler.NewAIStreamHandler(*c))
	// 未匹配 → 404（不误伤基础设施 probe）
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	})
}
