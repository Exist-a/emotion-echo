// Package configcenter 提供配置中心的抽象。
//
// Stage 31 PR-04（🔴 RED）：定义 ConfigCenter interface。
// PR-05（nacos_config.go）提供 Nacos 实现。
//
// 安全约束（ADR 决策 10）：本接口范围**仅放运营参数**——
// feature flag、限流阈值、模型路由表、Kafka 重试、A/B 分组等。
// JWT_SECRET / DATABASE_DSN / KAFKA_BROKERS / LLM_API_KEY / POSTGRES_PASSWORD
// 等敏感配置必须走 etc/*.yaml 或环境变量，不允许通过本接口发布。
//
// 实现方（NacosConfig 等）须在 ListenConfig callback 中拒绝（return error）
// 以下前缀的 dataId：
//
//   - jwt.*（任何 JWT 相关字段）
//   - database.* / db.*（DSN / password）
//   - kafka.*（broker 地址、ACL、密码）
//   - llm.* / openai.* / deepseek.*（API key）
//   - *.secret / *.password / *.token（任何后缀匹配）
//
// 这是契约级安全约束，调用方与测试共同保证。
package configcenter

import "context"

// ConfigChangeHandler 监听配置变更的回调。
//
// 当 dataId 对应的配置被服务端推送新版本时，handler 会被调用一次。
// 返回非 nil error 时，调用方应记录告警（不应阻塞后续推送）。
type ConfigChangeHandler func(dataId, group, content string) error

// ConfigCenter 是配置中心的抽象。
//
// 设计原则：
//   - GetConfig 返回配置内容（YAML / JSON / 纯文本，由调用方解析）
//   - ListenConfig 注册热更新回调；多次注册到同一 (dataId, group) 应替换前一个
//   - PublishConfig 仅供运维脚本使用（详见 PR-11 bootstrap_nacos.sh）；
//     业务代码不应调用此方法（敏感配置应通过 etc/*.yaml）
//   - Close 释放底层连接；调用后所有方法应返回 error
type ConfigCenter interface {
	// GetConfig 拉取一次配置内容；返回的字符串可能为空（首次启动无默认）。
	GetConfig(ctx context.Context, dataId, group string) (string, error)

	// ListenConfig 注册热更新回调；返回 nil 表示订阅成功。
	ListenConfig(ctx context.Context, dataId, group string, handler ConfigChangeHandler) error

	// PublishConfig 推送配置；通常仅在运维脚本中调用。
	// 实现方可选择拒绝 dataId 匹配敏感前缀的请求（参见包级安全约束）。
	PublishConfig(ctx context.Context, dataId, group, content string) error

	// Close 关闭客户端，释放连接。Close 后所有方法应返回 error。
	Close() error
}
