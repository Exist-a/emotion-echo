package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"emotion-echo-gin/internal/config"
	"emotion-echo-gin/internal/database"
	"emotion-echo-gin/internal/handler"
	"emotion-echo-gin/internal/pkg/console"
	"emotion-echo-gin/internal/pkg/jwt"
	"emotion-echo-gin/internal/pkg/llm"
	"emotion-echo-gin/internal/pkg/logger"
	"emotion-echo-gin/internal/pkg/snowflake"
	"emotion-echo-gin/internal/repository"
	"emotion-echo-gin/internal/router"
	"emotion-echo-gin/internal/scheduler"
	"emotion-echo-gin/internal/service"
	"emotion-echo-gin/internal/worker"
	"emotion-echo-gin/internal/workflow/chat"
	"emotion-echo-gin/internal/workflow/graph"
	"go.uber.org/zap"
)

func main() {
	// 设置 Windows 控制台编码为 UTF-8，解决乱码问题
	console.SetUTF8()

	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// 初始化日志
	if err := logger.Init(cfg.Server.Mode); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	// 连接数据库
	db, err := database.NewPostgres(cfg)
	if err != nil {
		logger.Fatal("Failed to connect to postgres", zap.Error(err))
	}

	// 连接 Redis
	redisClient, err := database.NewRedis(cfg)
	if err != nil {
		logger.Fatal("Failed to connect to redis", zap.Error(err))
	}

	// 初始化仓库
	userRepo := repository.NewUserRepository(db)
	tokenRepo := repository.NewTokenRepository(db)
	convRepo := repository.NewConversationRepository(db)
	msgRepo := repository.NewMessageRepository(db)
	surveyRepo := repository.NewSurveyRepository(db)
	surveyResultRepo := repository.NewSurveyResultRepository(db)
	analysisRepo := repository.NewEmotionAnalysisRepository(db)
	redisRepo := repository.NewRedisRepository(redisClient)

	// 初始化服务
	idGen := snowflake.NewGenerator(1)
	jwtInstance := jwt.New(cfg.JWT.Secret, cfg.GetAccessTokenExpire())

	authService := service.NewAuthService(db, userRepo, tokenRepo, redisRepo, jwtInstance, idGen, cfg.GetAccessTokenExpire(), cfg.GetRefreshTokenExpire())
	verifyService := service.NewVerifyService(redisRepo)
	wechatOAuthService := service.NewWechatOAuthService(cfg, userRepo, tokenRepo, jwtInstance, idGen, cfg.GetAccessTokenExpire(), cfg.GetRefreshTokenExpire())
	userService := service.NewUserService(userRepo)
	convService := service.NewConversationService(convRepo)
	msgService := service.NewMessageService(db, msgRepo, convRepo)

	// 初始化LLM客户端（用于工作流）
	llmClient := llm.NewClient(cfg)
	llmCaller := llmClient.Call

	// 初始化 LLM Chain（用于对话上下文管理）
	llmChain, err := llm.NewChain(cfg)
	if err != nil {
		logger.Fatal("Failed to create LLM chain", zap.Error(err))
	}

	// 初始化情绪分析工作流
	emotionWorkflow := chat.BuildEmotionWorkflow(llmCaller)

	// surveyService 需要先创建，因为 aiService 依赖它
	surveyService := service.NewSurveyService(surveyRepo, surveyResultRepo)

	aiService := service.NewAIService(cfg, db, convService, msgService, surveyService, emotionWorkflow, analysisRepo, llmCaller, llmChain)
	reportService := service.NewReportService(convRepo, msgRepo, analysisRepo)
	userBehaviorService := service.NewUserBehaviorService(msgRepo, convRepo)

	// 初始化检查点持久化
	checkpointer := graph.NewRedisCheckpointer(redisClient, "checkpoint")

	// 初始化心理健康评估服务
	mentalHealthRepo := repository.NewMentalHealthRepository(db)
	mentalHealthService := service.NewMentalHealthService(mentalHealthRepo, msgRepo, convRepo, surveyResultRepo, analysisRepo, llmCaller, checkpointer)

	// 初始化情绪分析工作流
	emotionWorker := worker.NewEmotionWorker(cfg, convRepo, msgRepo, analysisRepo, userRepo)
	mentalHealthWorker := worker.NewMentalHealthWorker(mentalHealthService)
	sched := scheduler.NewScheduler(cfg, emotionWorker, mentalHealthWorker)
	sched.Start()

	// 初始化处理器
	authHandler := handler.NewAuthHandler(cfg, authService, verifyService)
	oauthHandler := handler.NewOAuthHandler(wechatOAuthService)
	userHandler := handler.NewUserHandler(userService)
	convHandler := handler.NewConversationHandler(convService)
	msgHandler := handler.NewMessageHandler(msgService)
	aiHandler := handler.NewAIHandler(aiService)
	voiceService := service.NewVoiceService(cfg, msgRepo, convRepo)
	voiceHandler := handler.NewVoiceHandler(voiceService)
	surveyHandler := handler.NewSurveyHandler(surveyService)
	reportHandler := handler.NewReportHandler(reportService)
	mentalHealthHandler := handler.NewMentalHealthHandler(mentalHealthService)
	userBehaviorHandler := handler.NewUserBehaviorHandler(userBehaviorService)

	// 设置路由
	r := router.New(cfg, authHandler, oauthHandler, userHandler, convHandler, msgHandler, aiHandler, voiceHandler, surveyHandler, reportHandler, mentalHealthHandler, userBehaviorHandler, jwtInstance, redisRepo)
	engine := r.Setup()

	// 创建 HTTP server
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: engine,
	}

	// 在后台启动服务
	go func() {
		logger.Info("Server starting", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// 等待中断信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	// 优雅关闭：停止接收新请求，等待现有请求完成
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 关闭 HTTP server
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server forced to shutdown", zap.Error(err))
	}

	// 停止调度器
	sched.Stop()

	// 关闭数据库连接
	sqlDB, err := db.DB()
	if err == nil {
		sqlDB.Close()
	}

	// 关闭 Redis
	redisClient.Close()

	logger.Info("Server exited")
}
