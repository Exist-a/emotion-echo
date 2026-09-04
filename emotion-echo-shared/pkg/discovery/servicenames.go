// Package discovery 服务名常量。
//
// PR-0：所有 svc 用长名（emotion-echo-<svc>）注册到 Nacos，避免与 PR-03 nacos_boot_test.go
// 短名（ai-svc 等）冲突。APISIX nacos-discovery 按 service_name 拉实例，
// 长/短不一致会直接 404。
//
// 调用方约定：
//   - BootNacos 注册时用 ServiceUser / ServiceChat / ...
//   - 业务 config.Name 默认值也用同一份常量
package discovery

const (
	// ServiceUser user-svc 服务名。
	ServiceUser = "emotion-echo-user-svc"
	// ServiceChat chat-svc 服务名。
	ServiceChat = "emotion-echo-chat-svc"
	// ServiceAnalytics analytics-svc 服务名。
	ServiceAnalytics = "emotion-echo-analytics-svc"
	// ServiceAssessment assessment-svc 服务名。
	ServiceAssessment = "emotion-echo-assessment-svc"
	// ServiceAI ai-svc 服务名（仅 HTTP :8891 注册；gRPC :8892 写入 metadata.grpc_port）。
	ServiceAI = "emotion-echo-ai-svc"
	// ServiceWebBFF web-bff 服务名（供 APISIX nacos-discovery 上游拉取）。
	ServiceWebBFF = "emotion-echo-web-bff"
	// ServiceLLM emotion-llm-service 服务名（Python svc；Python 端 BootNacos 尚未实现）。
	ServiceLLM = "emotion-llm-service"
)