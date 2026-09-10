// Package config 提供 emotion-echo-web-bff 的配置结构（Stage 41 PR-7）
//
// Stage 41 改造点:去掉所有 json:",default=X" tag + json:",optional" tag,
// 改为 SetDefaults 函数(shared/pkg/config 契约);yaml 字段大小写映射沿用
// shared/pkg/config 的 lowercaseYAMLKeys 预处理(go-zero 不展开 ${VAR:-default},
// 占位符字面值原样保留)。
package config

import "os"

// SkyWalking 链路追踪配置（与 chat-svc 同构）
type SkyWalking struct {
	OAPAddr     string
	ServiceName string
	Enabled     bool
}

// HTTPService 是下游 HTTP 服务的通用配置
//
// Stage 63: 新增 GRPCAddr + Transport 字段，让 BFF 能 dial 下游 gRPC server。
// GRPCAddr 为空 → 不 dial，下游工厂静默走 HTTP fallback。
// Transport 为空 → 下游工厂按 "grpc" 处理（需同时 GRPCConn 非 nil）；
// Transport="http" → 强制 HTTP（回滚开关）。
type HTTPService struct {
	BaseURL   string
	TimeoutMs int
	GRPCAddr  string
	Transport string
}

// AIService 是 ai-svc 的双协议配置（HTTP + gRPC）
type AIService struct {
	HTTPAddr  string
	GRPCAddr  string
	TimeoutMs int
}

// Config 是 BFF 总配置
//
// 字段名与 etc/web-bff.yaml 一一对应。
type Config struct {
	Name       string
	Host       string
	Port       int
	SkyWalking SkyWalking
	Nacos      Nacos

	UserService       HTTPService
	ChatService       HTTPService
	AssessmentService HTTPService
	AnalyticsService  HTTPService

	AIService AIService

	XTTS HTTPService

	Health struct {
		TimeoutMs int
	}

	// Auth 是 BFF 自己的 JWT 配置（Stage 33 PR-19b 真实登录）
	Auth struct {
		JWTSecret        string
		TokenTTLSeconds  int
	}

	// TrustAPISIX 控制 BFF 是否信任 APISIX 注入的 X-User-Id header
	TrustAPISIX bool

	// LLM 是 BFF ai_stream 调用的真实 LLM（OpenAI 兼容）
	LLM struct {
		BaseURL string
		APIKey  string
		Model   string
		Timeout int
	}

	// Sprint 1 PR-4b: MinIO 对象存储
	MinIO struct {
		Endpoint      string
		AccessKey     string
		SecretKey     string
		Bucket        string
		UseSSL        bool
		PublicBaseURL string
	}
}

// Nacos 注册中心 + 配置中心配置（Stage 31 PR-09）
type Nacos struct {
	Enabled   bool
	Addr      string
	Namespace string
	GroupName string
	HotReload bool
}

// SetDefaults 填零值字段的默认值（shared/pkg/config.MustLoad 契约）。
//
// R3 反向:Kafka.Enabled / SkyWalking.Enabled / Nacos.Enabled / Nacos.HotReload /
// TrustAPISIX / MinIO.UseSSL 等 bool 不在 SetDefaults 里覆盖（yaml 显式 false
// 必须保留）。
func SetDefaults(c *Config) {
	if c.Name == "" {
		c.Name = "emotion-echo-web-bff"
	}
	if c.Host == "" {
		c.Host = "0.0.0.0"
	}
	if c.Port == 0 {
		c.Port = 8894
	}
	if c.SkyWalking.OAPAddr == "" {
		c.SkyWalking.OAPAddr = "localhost:11800"
	}
	// 下游服务默认
	setHTTPServiceDefaults(&c.UserService, "http://localhost:8888")
	setHTTPServiceDefaults(&c.ChatService, "http://localhost:8890")
	setHTTPServiceDefaults(&c.AssessmentService, "http://localhost:8889")
	setHTTPServiceDefaults(&c.AnalyticsService, "http://localhost:8904")
	// Stage 63: 下游 gRPC 地址默认值（与各 svc grpcserver 监听端口对齐）
	if c.UserService.GRPCAddr == "" {
		c.UserService.GRPCAddr = "localhost:8887"
	}
	if c.ChatService.GRPCAddr == "" {
		c.ChatService.GRPCAddr = "localhost:8892"
	}
	if c.AssessmentService.GRPCAddr == "" {
		c.AssessmentService.GRPCAddr = "localhost:8886"
	}
	if c.AnalyticsService.GRPCAddr == "" {
		c.AnalyticsService.GRPCAddr = "localhost:8885"
	}
	if c.AIService.HTTPAddr == "" {
		c.AIService.HTTPAddr = "http://localhost:8891"
	}
	if c.AIService.GRPCAddr == "" {
		c.AIService.GRPCAddr = "localhost:8892"
	}
	if c.AIService.TimeoutMs == 0 {
		c.AIService.TimeoutMs = 5000
	}
	setHTTPServiceDefaults(&c.XTTS, "http://localhost:8003")
	if c.XTTS.TimeoutMs == 0 {
		c.XTTS.TimeoutMs = 30000
	}
	if c.Health.TimeoutMs == 0 {
		c.Health.TimeoutMs = 2000
	}
	if c.Auth.JWTSecret == "" {
		c.Auth.JWTSecret = "dev-bff-secret"
	}
	if c.Auth.TokenTTLSeconds == 0 {
		c.Auth.TokenTTLSeconds = 86400
	}
	if c.LLM.BaseURL == "" {
		c.LLM.BaseURL = "https://api.deepseek.com"
	}
	if c.LLM.Model == "" {
		c.LLM.Model = "deepseek-chat"
	}
	if c.LLM.Timeout == 0 {
		c.LLM.Timeout = 60
	}
	if c.MinIO.Endpoint == "" {
		c.MinIO.Endpoint = "emotion-echo-minio:9000"
	}
	if c.MinIO.AccessKey == "" {
		c.MinIO.AccessKey = "minioadmin"
	}
	if c.MinIO.SecretKey == "" {
		c.MinIO.SecretKey = "minioadmin"
	}
	if c.MinIO.Bucket == "" {
		c.MinIO.Bucket = "avatars"
	}
	if c.MinIO.PublicBaseURL == "" {
		c.MinIO.PublicBaseURL = "http://localhost:9000"
	}
	if c.Nacos.Addr == "" {
		c.Nacos.Addr = "emotion-echo-nacos:8848"
	}
	if c.Nacos.Namespace == "" {
		c.Nacos.Namespace = "emotion-echo-dev"
	}
	if c.Nacos.GroupName == "" {
		c.Nacos.GroupName = "DEFAULT_GROUP"
	}
}

func setHTTPServiceDefaults(s *HTTPService, defaultBaseURL string) {
	if s.BaseURL == "" {
		s.BaseURL = defaultBaseURL
	}
	if s.TimeoutMs == 0 {
		s.TimeoutMs = 5000
	}
	// Transport 默认空 → 下游工厂按 "grpc" 处理（与决策 4 "内部 svc-to-svc = gRPC" 对齐）。
	//
	// Sprint C（2026-09-11）历史：曾因 grpcinterceptor.CtxUserIDKeyType 与 middleware.CtxUserIDKey
	// 两个类型不一致（决策 18 #26）临时默认 "http" 绕路。Sprint C ctxkey 重构后两类型别名指向
	// ctxkey.UserID，跨包 ctx 读写一致，可恢复 grpc 默认。
	//
	// Sprint D（chat-svc 6 RPC 实现）未做前，chat-svc gRPC 仍返 Unimplemented；
	// 届时 BFF 端 conversations 仍 500，需要 Sprint D 解决。
}

// ApplyEnvOverrides 用容器环境变量覆盖 config 字段（Stage 22-B 范式）。
//
// 覆盖项与 etc/web-bff.yaml 注释的 env 名一一对应：
//   USER_SVC_URL / CHAT_SVC_URL / ASSESSMENT_SVC_URL / ANALYTICS_SVC_URL
//   AI_SVC_HTTP_URL / AI_SVC_GRPC_ADDR / XTTS_BASE_URL / SKYWALKING_OAP_ADDR
//
// 仅覆盖非空 env（空串视为未设置，保持 yaml 默认值）。
func ApplyEnvOverrides(c *Config) {
	if v := os.Getenv("USER_SVC_URL"); v != "" {
		c.UserService.BaseURL = v
	}
	if v := os.Getenv("CHAT_SVC_URL"); v != "" {
		c.ChatService.BaseURL = v
	}
	if v := os.Getenv("ASSESSMENT_SVC_URL"); v != "" {
		c.AssessmentService.BaseURL = v
	}
	if v := os.Getenv("ANALYTICS_SVC_URL"); v != "" {
		c.AnalyticsService.BaseURL = v
	}
	// Stage 63: 下游 gRPC 地址 env 覆盖（容器 DNS）
	if v := os.Getenv("USER_SVC_GRPC_ADDR"); v != "" {
		c.UserService.GRPCAddr = v
	}
	if v := os.Getenv("CHAT_SVC_GRPC_ADDR"); v != "" {
		c.ChatService.GRPCAddr = v
	}
	if v := os.Getenv("ASSESSMENT_SVC_GRPC_ADDR"); v != "" {
		c.AssessmentService.GRPCAddr = v
	}
	if v := os.Getenv("ANALYTICS_SVC_GRPC_ADDR"); v != "" {
		c.AnalyticsService.GRPCAddr = v
	}
	// Stage 63: 传输协议回滚开关（默认空 → grpc；设 "http" 强制走 HTTP fallback）
	if v := os.Getenv("USER_TRANSPORT"); v != "" {
		c.UserService.Transport = v
	}
	if v := os.Getenv("CHAT_TRANSPORT"); v != "" {
		c.ChatService.Transport = v
	}
	if v := os.Getenv("ASSESSMENT_TRANSPORT"); v != "" {
		c.AssessmentService.Transport = v
	}
	if v := os.Getenv("ANALYTICS_TRANSPORT"); v != "" {
		c.AnalyticsService.Transport = v
	}
	if v := os.Getenv("AI_SVC_HTTP_URL"); v != "" {
		c.AIService.HTTPAddr = v
	}
	if v := os.Getenv("AI_SVC_GRPC_ADDR"); v != "" {
		c.AIService.GRPCAddr = v
	}
	if v := os.Getenv("XTTS_BASE_URL"); v != "" {
		c.XTTS.BaseURL = v
	}
	if v := os.Getenv("SKYWALKING_OAP_ADDR"); v != "" {
		c.SkyWalking.OAPAddr = v
	}
	if v := os.Getenv("SKYWALKING_ENABLED"); v != "" {
		c.SkyWalking.Enabled = v == "true" || v == "1"
	}
	if v := os.Getenv("BFF_JWT_SECRET"); v != "" {
		c.Auth.JWTSecret = v
	}
	if v := os.Getenv("BFF_TRUST_APISIX"); v != "" {
		c.TrustAPISIX = v == "true" || v == "1"
	}
	if v := os.Getenv("BFF_LLM_API_KEY"); v != "" {
		c.LLM.APIKey = v
	}
	if v := os.Getenv("BFF_LLM_BASE_URL"); v != "" {
		c.LLM.BaseURL = v
	}
	if v := os.Getenv("BFF_LLM_MODEL"); v != "" {
		c.LLM.Model = v
	}
	// Stage 31 PR-09: Nacos
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
	// Sprint 1 PR-4b: MinIO
	if v := os.Getenv("MINIO_ENDPOINT"); v != "" {
		c.MinIO.Endpoint = v
	}
	if v := os.Getenv("MINIO_ACCESS_KEY"); v != "" {
		c.MinIO.AccessKey = v
	}
	if v := os.Getenv("MINIO_SECRET_KEY"); v != "" {
		c.MinIO.SecretKey = v
	}
	if v := os.Getenv("MINIO_BUCKET"); v != "" {
		c.MinIO.Bucket = v
	}
	if v := os.Getenv("MINIO_PUBLIC_BASE_URL"); v != "" {
		c.MinIO.PublicBaseURL = v
	}
}
