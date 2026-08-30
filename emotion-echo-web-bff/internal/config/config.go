// Package config 提供 emotion-echo-web-bff 的配置结构。
//
// 参考 chat-svc / ai-svc 的 config 模式（go-zero conf 加载 + 容器 env 覆盖）。
package config

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
	Name       string `json:",default=web-bff"`
	Host       string `json:",default=0.0.0.0"`
	Port       int    `json:",default=8894"`
	SkyWalking SkyWalking

	UserService       HTTPService `json:",optional"`
	ChatService       HTTPService `json:",optional"`
	AssessmentService HTTPService `json:",optional"`
	AnalyticsService  HTTPService `json:",optional"`

	AIService AIService `json:",optional"`

	XTTS HTTPService `json:",optional"`

	Health struct {
		TimeoutMs int `json:",default=2000"`
	} `json:",optional"`
}
