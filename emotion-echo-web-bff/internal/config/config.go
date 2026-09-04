// Package config 提供 emotion-echo-web-bff 的配置结构。
//
// 参考 chat-svc / ai-svc 的 config 模式（go-zero conf 加载 + 容器 env 覆盖）。
package config

import "os"

// SkyWalking 链路追踪配置（与 chat-svc 同构）
type SkyWalking struct {
	OAPAddr     string `json:",default=localhost:11800"`
	ServiceName string
	Enabled     bool `json:",default=false"`
}

// HTTPService 是下游 HTTP 服务的通用配置
type HTTPService struct {
	BaseURL   string `json:",default=http://localhost:8888"`
	TimeoutMs int    `json:",default=5000"`
}

// AIService 是 ai-svc 的双协议配置（HTTP + gRPC）
type AIService struct {
	HTTPAddr  string `json:",default=http://localhost:8891"`
	GRPCAddr  string `json:",default=localhost:8892"`
	TimeoutMs int    `json:",default=5000"`
}

// Config 是 BFF 总配置
//
// 字段名与 etc/web-bff.yaml 一一对应。
// go-zero conf 加载时 `${VAR:-default}` 占位符不展开（Stage 22-B 已确认），
// 容器内地址由 main.go applyEnvOverrides 覆盖（T1.3 REFACTOR 拆分）。
type Config struct {
	Name       string `json:",default=emotion-echo-web-bff"`
	Host       string `json:",default=0.0.0.0"`
	Port       int    `json:",default=8894"`
	SkyWalking SkyWalking
	Nacos      Nacos

	UserService       HTTPService `json:",optional"`
	ChatService       HTTPService `json:",optional"`
	AssessmentService HTTPService `json:",optional"`
	AnalyticsService  HTTPService `json:",optional"`

	AIService AIService `json:",optional"`

	XTTS HTTPService `json:",optional"`

	Health struct {
		TimeoutMs int `json:",default=2000"`
	} `json:",optional"`

	// Auth 是 BFF 自己的 JWT 配置（Stage 33 PR-19b 真实登录）
	//
	// JWTSecret 用于 /api/v1/auth/login 端点给前端签发 JWT；
	// 真正的下游鉴权由 APISIX jwt-auth 验签后注入 X-User-Id header 完成（决策 12）。
	Auth struct {
		// JWTSecret：login 端点 sign JWT 用，必须与 APISIX jwt-auth secret 同源
		JWTSecret string `json:",default=dev-bff-secret"`
		// TokenTTLSeconds token 有效期（秒）
		TokenTTLSeconds int `json:",default=86400"`
	} `json:",optional"`

	// TrustAPISIX 控制 BFF 是否信任 APISIX 注入的 X-User-Id header
	// true（默认）：直接读 X-User-Id，不再解析 Authorization（生产路径）
	// false：本地直连 BFF 调试时回退到解析 Authorization（dev fallback）
	// Stage 32 PR-16: 由 env BFF_TRUST_APISIX 注入
	TrustAPISIX bool `json:",default=true"`

	// LLM 是 BFF ai_stream 调用的真实 LLM（OpenAI 兼容）
	// 生产部署填真实 key；dev 留空则 ai_stream 走 mock 共情回复
	LLM struct {
		BaseURL string `json:",default=https://api.deepseek.com"`
		APIKey  string `json:",optional"`
		Model   string `json:",default=deepseek-chat"`
		Timeout int    `json:",default=60"`
	} `json:",optional"`
}

// Nacos 注册中心 + 配置中心配置（Stage 31 PR-09）
//
// web-bff 也注册到 Nacos（service-name=web-bff）；Stage 32 APISIX 通过
// nacos-discovery 插件自动发现 web-bff 作为上游，无需静态 upstream 配置。
type Nacos struct {
	Enabled   bool   `json:",default=true"`
	Addr      string `json:",default=emotion-echo-nacos:8848"`
	Namespace string `json:",default=emotion-echo-dev"`
	GroupName string `json:",default=DEFAULT_GROUP"`
	HotReload bool   `json:",default=false"`
}

// ApplyEnvOverrides 用容器环境变量覆盖 config 字段（Stage 22-B 范式）。
//
// 背景：go-zero conf 不解析 `${VAR:-default}` bash 占位符（Stage 22-B 已确认），
// 所以容器地址由 compose environment 注入，main.go 启动时调用本函数覆盖。
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
	if v := os.Getenv("BFF_JWT_SECRET"); v != "" {
		c.Auth.JWTSecret = v
	}
	// Stage 32 PR-16: 移除 BFF_JWT_SECRET 不再由 BFF 用于下游鉴权透传
	// (保留字段：仅用于 mock login 端点签发；Stage 33 净化会彻底删除)
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
	// Stage 31 PR-09: Nacos 注册中心 + 配置中心
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
