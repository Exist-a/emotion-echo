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
	bffdiscovery "emotion-echo-web-bff/internal/discovery"
	"emotion-echo-web-bff/internal/config"
	"emotion-echo-web-bff/internal/downstream"
	"emotion-echo-web-bff/internal/handler"
	"emotion-echo-web-bff/internal/logging"
	"emotion-echo-web-bff/internal/storage"
	"emotion-echo-web-bff/internal/svc"

	"github.com/SkyAPM/go2sky"
	"github.com/gin-gonic/gin"
	sharedconfig "github.com/emotion-echo/shared/pkg/config"
	sharedmetrics "github.com/emotion-echo/shared/pkg/metrics"
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
	sharedgrpc "github.com/emotion-echo/shared/pkg/grpcinterceptor"
	shareddiscovery "github.com/emotion-echo/shared/pkg/discovery"
	sharedbootstrap "github.com/emotion-echo/shared/pkg/bootstrap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var configFile = flag.String("f", "etc/web-bff.yaml", "the config file")

// grpcDialer 是 gRPC 连接构造函数，提取为包级变量以便测试覆盖（bufconn）。
// 生产默认用 grpc.NewClient + insecure（内部服务间 mTLS 由基础设施层保障）。
var grpcDialer = func(addr string) (*grpc.ClientConn, error) {
	return grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

// dialGRPC 尝试 dial 下游 gRPC 地址，失败时打日志并返 nil（调用方走 HTTP fallback）。
//
// Stage 63: 修复 PR-3 "gRPC 建好了但 BFF 没接线" 的 bug。
// 之前 buildServiceContext 构造 4 个下游 client 时只传 BaseURL，不传 GRPCConn，
// 下游工厂静默 fallback 到 HTTP → 生产路径全走 HTTP，gRPC 等于没上线。
func dialGRPC(addr, name string) *grpc.ClientConn {
	if addr == "" {
		return nil
	}
	conn, err := grpcDialer(addr)
	if err != nil {
		log.Printf("[grpc] %s dial %s failed: %v (fallback to HTTP)", name, addr, err)
		return nil
	}
	log.Printf("[grpc] %s connected: %s", name, addr)
	return conn
}

// resolveGRPCAddr 决定下游 gRPC 拨号地址：Nacos 优先（Discover 有实例），env/config
// 的 GRPCAddr 兜底（Nacos 抖动或 NACOS_ENABLED=false 时不阻断启动）。
//
// Stage 75 前置条件：各 svc 注册时把 gRPC 端口写进 metadata.grpc_port（见各 svc
// nacos_boot.go）；resolver 侧用 WithPortHint("grpc_port") 读它。
// envOr Stage 81 PR-2：env 优先，缺省兜底（ai-svc analyzer 同款）
func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func resolveGRPCAddr(resolver bffdiscovery.Resolver, fallbackAddr, svcName string) string {
	if resolver == nil {
		return fallbackAddr
	}
	host, port, err := resolver.Resolve(context.Background(), svcName)
	if err != nil || host == "" {
		log.Printf("[nacos] resolve %s failed (%v), grpc addr fallback to %s", svcName, err, fallbackAddr)
		return fallbackAddr
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	log.Printf("[nacos] resolve %s -> %s (grpc)", svcName, addr)
	return addr
}

func main() {
	flag.Parse()
	logging.Init()
	// PR-OBS-15: SetGlobalSvc 让所有 slog 日志自动带 svc="web-bff" 字段(决策 6 必填)
	logging.SetGlobalSvc("web-bff")

	var c config.Config
	sharedconfig.MustLoad(*configFile, &c, func() { config.SetDefaults(&c) })
	config.ApplyEnvOverrides(&c)

	// PR-2: Nacos 启动在 buildServiceContext 之前——这样 Resolver 可以用 nacosRuntime.Registry。
	bootCtx, bootCancel := context.WithCancel(context.Background())
	defer bootCancel()
	// PR-4: HotReloadLimiter 由 main 持有，注入 bootDeps；web-bff.ops.yaml 解析后 Update 进来。
	opsLimiter := handler.NewHotReloadLimiter(60, 100)
	nacosDeps := defaultBootDeps()
	nacosDeps.opsLimiter = opsLimiter
	nacosRuntime, err := BootNacos(bootCtx, &c, nacosDeps)
	if err != nil {
		log.Printf("[nacos] boot failed (continuing): %v", err)
	}
	defer func() {
		if nacosRuntime != nil {
			nacosRuntime.Close(context.Background(), c.Name, c.Host, c.Port)
		}
	}()

	// 1. 下游 client 装配（PR-2: Resolver 由 nacosRuntime.Registry 派生）
	// Stage 75: grpcResolver 带 grpc_port portHint，gRPC 拨号地址 Nacos 优先（env 兜底）。
	var resolver, grpcResolver bffdiscovery.Resolver
	if nacosRuntime != nil && nacosRuntime.Registry != nil {
		resolver = bffdiscovery.NewNacosResolver(nacosRuntime.Registry, c.Nacos.Namespace)
		grpcResolver = bffdiscovery.NewNacosResolver(nacosRuntime.Registry, c.Nacos.Namespace).WithPortHint("grpc_port")
	}
	svcCtx := buildServiceContext(&c, resolver, grpcResolver)

	// 2. SkyWalking（可选, PR-OBS-2: 用 shared BootstrapSkyWalkingTracer 统一 7 svc 行为）
	var tracer *go2sky.Tracer
	if c.SkyWalking.Enabled {
		t, err := sharedbootstrap.BootstrapSkyWalkingTracer(context.Background(), c.SkyWalking.ServiceName, c.SkyWalking.OAPAddr, 2*time.Second)
		if err != nil {
			sharedmetrics.IncSkyWalkingInitFailed(c.SkyWalking.ServiceName)
			if sharedbootstrap.ShouldFailFast() && sharedbootstrap.IsRequired("skywalking") {
				log.Fatalf("[skywalking] strict mode + required dep, refusing to start: %v", err)
			}
			log.Printf("[skywalking] tracer init failed (warn mode, continue): %v", err)
		} else {
			tracer = t
			log.Printf("[skywalking] tracer initialized (PR-OBS-2 helper)")
		}
	}

	// 3. Gin
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(sharedmetrics.GinMetricsMiddleware("web-bff"))
	// Stage 38-A: dev 模式前端发 Authorization: Bearer <token>，但 sharedmw GinAuthMiddleware
	// 只信任 APISIX 注入的 X-User-Id header（APISIX 已配 jwt-auth 插件解析 token → 注 header）。
	// dev 模式前端走 APISIX :19080 → BFF :8894，APISIX 负责 JWT 验签 + X-User-Id 注入。
	if tracer != nil {
		r.Use(sharedmw.GinSkywalkingMiddleware(sharedgrpc.NewGo2SkyTracer(tracer)))
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

	// 3.5 Stage 81 PR-2：llm-service ChatCompletion gRPC 上游（LLM_SVC_GRPC_ADDR 非空时启用）
	var llmStreamer downstream.LLMChatStreamer
	var intentClassifier downstream.LLMIntentClassifier
	if c.LLM.GRPCAddr != "" {
		llmGRPC, err := downstream.NewLLMGRPCClient(downstream.LLMGRPCOptions{
			Addr:           c.LLM.GRPCAddr,
			TLSEnabled:     os.Getenv("TLS_ENABLED") == "1",
			CACertPath:     envOr("TLS_CA_CERT", "/app/etc/tls/ca.crt"),
			ClientCertPath: envOr("TLS_CLIENT_CERT", "/app/etc/tls/ai-client.crt"),
			ClientKeyPath:  envOr("TLS_CLIENT_KEY", "/app/etc/tls/ai-client.key"),
			TLSServerName:  envOr("TLS_SERVER_NAME", "emotion-llm-service"),
			InternalAPIKey: os.Getenv("INTERNAL_API_KEY"),
		})
		if err != nil {
			// 降级链兜底在 handler（mock / HTTP 直连），这里只告警不阻断启动
			log.Printf("[llm-grpc] client init failed (ai/stream will fall back): %v", err)
		} else {
			llmStreamer = llmGRPC
			intentClassifier = llmGRPC
			defer func() { _ = llmGRPC.Close() }()
			log.Printf("[llm-grpc] ChatCompletion upstream enabled: %s", c.LLM.GRPCAddr)
		}
	}

	// 4. 路由（handler 装配）
	registerRoutes(r, svcCtx, &c, llmStreamer, intentClassifier)

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

// authPathBypass 让 /api/v1/auth/* 路径跳过鉴权（login/register/refresh/logout/verification-code
// 端点拿不到 X-User-Id，必须白名单）。Stage 33 PR-19b/21：仅剩这 5 条白名单路径，
// 其他 /api/v1/* 全部走 sharedmw.GinAuthMiddleware。
func authPathBypass(authMW gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/v1/auth/") {
			c.Next()
			return
		}
		authMW(c)
	}
}

// devCorsOrigins 与 devCORSMiddleware 已回滚（Stage 38-A 改为走 APISIX 路径）。
// 历史注释保留为参考：原 BFF dev 兼容 CORS 中间件已不再使用。
//
// CORS / JWT 注入 / 限流 / 熔断全由 APISIX 统一处理（见 deploy/apisix/seed.sh
// 的 jwt-auth + cors + limit-count + api-breaker 插件链）。
// BFF 回到"纯聚合层"原始定位。

// buildServiceContext 装配全部下游 client + auth manager
//
// PR-2：三个目标 client（ai / chat / analytics）支持 Resolver 兜底。
// 当 env 注入 BaseURL 为空时，从 Nacos Discover 拉实例。
//
// Stage 75：grpcResolver（带 grpc_port portHint）非 nil 时，5 处 gRPC 拨号地址
// Nacos 优先、env GRPCAddr 兜底——dev 默认 Transport=grpc（决策 4），此前 gRPC
// 流量从未经过 Nacos。grpcResolver 为 nil 时行为与 Stage 63 一致。
func buildServiceContext(c *config.Config, resolver, grpcResolver bffdiscovery.Resolver) *svc.ServiceContext {
	svcCtx := svc.NewServiceContext(*c)

	// auth manager（自有 JWT 签发）
	mgr, err := auth.NewManager(c.Auth.JWTSecret, c.Auth.TokenTTLSeconds)
	if err != nil {
		log.Fatalf("[auth] JWT manager init failed: %v", err)
	}
	svcCtx.SetAuth(mgr)

	// HTTP clients（5 个下游 + XTTS）。
	// 三个高优（ai/chat/analytics）走 Resolver 兜底；其他保持 env-only。
	//
	// Stage 63: dial 4 个下游 gRPC 连接，传入 client 构造。
	// Transport=grpc（默认）+ GRPCConn!=nil → 走 gRPC；否则静默 HTTP fallback。
	// dial 失败也降级 HTTP（不阻塞启动）。
	userGRPCConn := dialGRPC(resolveGRPCAddr(grpcResolver, c.UserService.GRPCAddr, shareddiscovery.ServiceUser), "user-svc")
	chatGRPCConn := dialGRPC(resolveGRPCAddr(grpcResolver, c.ChatService.GRPCAddr, shareddiscovery.ServiceChat), "chat-svc")
	assessmentGRPCConn := dialGRPC(resolveGRPCAddr(grpcResolver, c.AssessmentService.GRPCAddr, shareddiscovery.ServiceAssessment), "assessment-svc")
	analyticsGRPCConn := dialGRPC(resolveGRPCAddr(grpcResolver, c.AnalyticsService.GRPCAddr, shareddiscovery.ServiceAnalytics), "analytics-svc")
	aiGRPCConn := dialGRPC(resolveGRPCAddr(grpcResolver, c.AIService.GRPCAddr, shareddiscovery.ServiceAI), "ai-svc")

	svcCtx.SetUser(downstream.NewUserClient(downstream.UserClientOptions{
		BaseURL: c.UserService.BaseURL, TimeoutMs: c.UserService.TimeoutMs,
		Transport: downstream.UserTransport(c.UserService.Transport),
		GRPCConn:  userGRPCConn,
	}))
	svcCtx.SetChat(downstream.NewChatClient(downstream.ChatClientOptions{
		BaseURL: c.ChatService.BaseURL, TimeoutMs: c.ChatService.TimeoutMs,
		Resolver: resolver,
		Transport: downstream.ChatTransport(c.ChatService.Transport),
		GRPCConn:  chatGRPCConn,
	}))
	svcCtx.SetAssessment(downstream.NewAssessmentClient(downstream.AssessmentClientOptions{
		BaseURL: c.AssessmentService.BaseURL, TimeoutMs: c.AssessmentService.TimeoutMs,
		Transport: downstream.AssessmentTransport(c.AssessmentService.Transport),
		GRPCConn:  assessmentGRPCConn,
	}))
	svcCtx.SetAnalytics(downstream.NewAnalyticsClient(downstream.AnalyticsClientOptions{
		BaseURL: c.AnalyticsService.BaseURL, TimeoutMs: c.AnalyticsService.TimeoutMs,
		Resolver: resolver,
		Transport: downstream.AnalyticsTransport(c.AnalyticsService.Transport),
		GRPCConn:  analyticsGRPCConn,
	}))
	svcCtx.SetAI(downstream.NewAIClient(downstream.AIClientOptions{
		BaseURL: c.AIService.HTTPAddr, TimeoutMs: c.AIService.TimeoutMs,
		Resolver: resolver,
		GRPCConn:  aiGRPCConn,
		Transport: downstream.AITransport(c.AIService.Transport),
	}))
	svcCtx.SetXTTS(downstream.NewXTTSClient(downstream.XTTSClientOptions{
		BaseURL: c.XTTS.BaseURL, TimeoutMs: c.XTTS.TimeoutMs,
	}))

	// ai-svc gRPC（EmotionQueryService）— Stage 75 起统一走 resolveGRPCAddr。
	// 修复潜伏 bug：原实现用无 portHint 的 HTTP resolver 解析 ai-svc，拿到 HTTP
	// 端口 8891 覆盖正确的 env AI_SVC_GRPC_ADDR(:8892)。
	grpcAddr := resolveGRPCAddr(grpcResolver, c.AIService.GRPCAddr, shareddiscovery.ServiceAI)
	conn, err := grpc.NewClient(grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Printf("[grpc] ai-svc dial failed: %v (emotion query disabled)", err)
	} else {
		svcCtx.SetEmotionQ(downstream.NewEmotionQueryClient(conn))
	}

	// Sprint 1 PR-4b: MinIO 对象存储装配
	storageCli, storageErr := storage.NewMinIOClient(storage.MinIOConfig{
		Endpoint:       c.MinIO.Endpoint,
		AccessKey:      c.MinIO.AccessKey,
		SecretKey:      c.MinIO.SecretKey,
		Bucket:         c.MinIO.Bucket,
		UseSSL:         c.MinIO.UseSSL,
		PublicBaseURL:  c.MinIO.PublicBaseURL,
	})
	if storageErr != nil {
		log.Printf("[minio] client init failed: %v (storage disabled, avatar upload will 500)", storageErr)
	} else {
		svcCtx.Storage = storageCli
		log.Printf("[minio] connected to %s bucket=%s", c.MinIO.Endpoint, c.MinIO.Bucket)
	}

	return svcCtx
}

// registerRoutes 注册全部 BFF 路由
//
// 路径契约（路由清单）：main_test.go 的 wantRoutes + wantRoutesWithEmotionQ 切片。
// 改路由必须同步更新测试文件 + 在 PR 描述里说明（决策 18 §四.1 结论须附证据）。
// 调试时临时增减路由也行——但合 PR 前 main_test.go 必须绿。
func registerRoutes(r *gin.Engine, s *svc.ServiceContext, c *config.Config, llmStreamer downstream.LLMChatStreamer, llmIntent downstream.LLMIntentClassifier) {
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
	handler.NewChatHandlerWithIntent(s.Chat, llmIntent).Register(r)
	handler.NewSurveyHandler(s.Assessment).Register(r)
	handler.NewAnalyticsHandler(s.Analytics).Register(r)
	handler.NewMultimodalHandler(s.AI).Register(r)
	handler.NewTTSHandler(s.AI, s.XTTS).Register(r)
	handler.NewUploadHandler(s.Storage).Register(r)
	// Sprint 1 PR-4c-1: voice upload (multipart → ai-svc multimodal kind=audio)
	handler.NewVoiceHandler(s.AI).Register(r)
	// Sprint 1 PR-4c-2: user avatar upload (multipart → MinIO → user-svc UpdateMe)
	// 总是注册：handler 内部 nil 检查；缺 Storage 时 503
	handler.NewAvatarHandler(s.User, s.Storage).Register(r)
	// Sprint F2（2026-09-11）：ai-svc gRPC AIHealth RPC 接入
	// /api/v1/ai/health 探针路由（之前未注册，pre-existing 缺失；本次 Sprint 顺手补）
	r.GET("/api/v1/ai/health", handler.NewAIHealthHandler(s.AI).Health)
	if s.EmotionQ != nil {
		handler.NewEmotionQueryHandler(s.EmotionQ).Register(r)
	}
	// SSE 流式
	// Stage 81 PR-2：llm-service ChatCompletion gRPC 上游优先（llmStreamer 非 nil 时）
	r.POST("/api/v1/ai/stream", handler.NewAIStreamHandlerWithLLM(*c, llmStreamer))
	// 未匹配 → 404（不误伤基础设施 probe）
	r.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	})
}
