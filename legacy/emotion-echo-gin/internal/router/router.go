package router

import (
	"github.com/gin-gonic/gin"
	"emotion-echo-gin/internal/config"
	"emotion-echo-gin/internal/handler"
	"emotion-echo-gin/internal/middleware"
	"emotion-echo-gin/internal/pkg/jwt"
	"emotion-echo-gin/internal/repository"
)

// Router 路由
type Router struct {
	engine              *gin.Engine
	cfg                 *config.Config
	authHandler         *handler.AuthHandler
	oauthHandler        *handler.OAuthHandler
	userHandler         *handler.UserHandler
	convHandler         *handler.ConversationHandler
	msgHandler          *handler.MessageHandler
	aiHandler          *handler.AIHandler
	voiceHandler        *handler.VoiceHandler
	surveyHandler       *handler.SurveyHandler
	reportHandler       *handler.ReportHandler
	mentalHealthHandler *handler.MentalHealthHandler
	userBehaviorHandler *handler.UserBehaviorHandler
	jwt                 *jwt.JWT
	redisRepo           *repository.RedisRepository
}

// New 创建路由
func New(
	cfg *config.Config,
	authHandler *handler.AuthHandler,
	oauthHandler *handler.OAuthHandler,
	userHandler *handler.UserHandler,
	convHandler *handler.ConversationHandler,
	msgHandler *handler.MessageHandler,
	aiHandler *handler.AIHandler,
	voiceHandler *handler.VoiceHandler,
	surveyHandler *handler.SurveyHandler,
	reportHandler *handler.ReportHandler,
	mentalHealthHandler *handler.MentalHealthHandler,
	userBehaviorHandler *handler.UserBehaviorHandler,
	jwt *jwt.JWT,
	redisRepo *repository.RedisRepository,
) *Router {
	return &Router{
		engine:              gin.New(),
		cfg:                 cfg,
		authHandler:         authHandler,
		oauthHandler:        oauthHandler,
		userHandler:         userHandler,
		convHandler:         convHandler,
		msgHandler:          msgHandler,
		aiHandler:           aiHandler,
		voiceHandler:        voiceHandler,
		surveyHandler:       surveyHandler,
		reportHandler:       reportHandler,
		mentalHealthHandler: mentalHealthHandler,
		userBehaviorHandler: userBehaviorHandler,
		jwt:                 jwt,
		redisRepo:           redisRepo,
	}
}

// Setup 设置路由
func (r *Router) Setup() *gin.Engine {
	// 全局中间件
	r.engine.Use(middleware.Recovery())
	r.engine.Use(middleware.Logger())
	r.engine.Use(middleware.CORS())

	// 静态文件服务
	r.engine.Static("/uploads", "./uploads")
	
	// 限流中间件（根据配置启用）
	if r.cfg.RateLimit.Enabled {
		r.engine.Use(middleware.RateLimit(r.redisRepo, r.cfg.RateLimit.RequestsPerSecond, r.cfg.RateLimit.Burst))
	}

	// 健康检查
	r.engine.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// API v1
	v1 := r.engine.Group("/api/v1")

	// 公开接口（无需认证）
	public := v1.Group("")
	{
		auth := public.Group("/auth")
		{
			auth.POST("/register", r.authHandler.Register)
			auth.POST("/login", r.authHandler.Login)
			auth.POST("/refresh", r.authHandler.Refresh)
			auth.POST("/logout", r.authHandler.Logout)
			auth.POST("/verification-code", r.authHandler.SendVerifyCode)
			auth.POST("/reset-password", r.authHandler.ResetPassword)

			// 微信 OAuth
			auth.GET("/oauth/wechat/url", r.oauthHandler.GetWechatAuthURL)
			auth.POST("/oauth/wechat/login", r.oauthHandler.WechatLogin)
		}
	}

	// 需要认证的接口
	authorized := v1.Group("")
	authorized.Use(middleware.JWTAuth(r.jwt))
	{
		// 用户模块（使用单数 user，与 v2.0 兼容）
		user := authorized.Group("/user")
		{
			user.GET("/profile", r.userHandler.GetProfile)
			user.PUT("/profile", r.userHandler.UpdateProfile)
			user.POST("/avatar", r.userHandler.UploadAvatar)
		}

		// 会话模块
		conversations := authorized.Group("/conversations")
		{
			conversations.GET("", r.convHandler.List)
			conversations.POST("", r.convHandler.Create)
			conversations.PUT("/:id", r.convHandler.Update)
			conversations.DELETE("/:id", r.convHandler.Delete)
			conversations.POST("/:id/pin", r.convHandler.Pin)

			// 消息
			conversations.GET("/:id/messages", r.msgHandler.List)
			conversations.POST("/:id/messages", r.msgHandler.Send)
		}

		// AI 模块
		ai := authorized.Group("/ai")
		{
			ai.POST("/stream", r.aiHandler.Stream)
		}

		// 语音模块
		voice := authorized.Group("/voice")
		{
			voice.POST("/upload", r.voiceHandler.Upload)
		}

		// 测验模块
		surveys := authorized.Group("/surveys")
		{
			surveys.GET("", r.surveyHandler.List)
			surveys.GET("/:id", r.surveyHandler.Get)
			surveys.POST("/:id/submit", r.surveyHandler.Submit)
			surveys.GET("/result/:resultId", r.surveyHandler.GetResult)
		}

		// 报表模块
		reports := authorized.Group("/reports")
		{
			reports.GET("/daily", r.reportHandler.GetDaily)
			reports.GET("/trend", r.reportHandler.GetTrend)
		}

		// 心理健康模块
		mentalHealth := authorized.Group("/mental-health")
		{
			mentalHealth.GET("/assessment", r.mentalHealthHandler.GetAssessment)
			mentalHealth.GET("/history", r.mentalHealthHandler.GetHistory)
			mentalHealth.POST("/trigger", r.mentalHealthHandler.TriggerAssessment)
			mentalHealth.GET("/trend", r.mentalHealthHandler.GetTrend)
		}

		// 用户行为分析模块
		behavior := authorized.Group("/user-behavior")
		{
			behavior.GET("/day-night", r.userBehaviorHandler.GetDayNightPattern)
			behavior.GET("/depth", r.userBehaviorHandler.GetInteractionDepth)
			behavior.GET("/frequency", r.userBehaviorHandler.GetFrequencyTrend)
		}
	}

	return r.engine
}
