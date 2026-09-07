// Package config 提供 chat-svc 的配置结构（Stage 41 PR-4）
//
// 改造点：去掉所有 json:",default=X" tag（go-zero conf 才需要）；
// 改为 SetDefaults 函数，在 main.go 加载 yaml 之前调用（shared/pkg/config 契约）。
package config

type SkyWalking struct {
	OAPAddr     string
	ServiceName string
	Enabled     bool
}

type Postgres struct {
	DSN          string
	MaxOpenConns int
	MaxIdleConns int
}

// Kafka.BrokersCSV (string) — Stage 26-P 改造。
// list 字段无法走 go-zero ${ENV} 占位展开,与 ai-svc 范式统一:
// 容器内由 compose env KAFKA_BROKERS 注入,main.go 启动时 split(',')。
//
// Stage 36-A1.2：Kafka.Enabled 默认值改为 true——chat-svc 的 outbox relay
//（Stage 30-C A3）依赖 Kafka，否则 message.created 事件不会发出，
// ai-svc 永远不会消费情绪分析请求。
type Kafka struct {
	BrokersCSV string
	GroupID    string
	Enabled    bool
}

// Nacos 注册中心 + 配置中心配置（Stage 31 PR-08 引入）
type Nacos struct {
	Enabled   bool
	Addr      string
	Namespace string
	GroupName string
	HotReload bool
}

type Config struct {
	Name       string
	Host       string
	Port       int
	SkyWalking SkyWalking
	Postgres   Postgres
	Kafka      Kafka
	Nacos      Nacos

	// Stage 36-A3.2: ai-svc gRPC 地址（dev fallback 同步写中性情绪用）。
	// 空 = 不启用 dev fallback（保持 NoopAIClient），与 KAFKA_ENABLED 组合决定是否调 ai-svc。
	AIService AIService
}

// AIService ai-svc 客户端配置（Stage 36-A3.2）
type AIService struct {
	GRPCAddr string
}

// SetDefaults 填零值字段的默认值（shared/pkg/config.MustLoad 契约）。
//
// 调用顺序：SetDefaults → yaml.Unmarshal（yaml 显式值覆盖默认；R3 反向已钉死）。
func SetDefaults(c *Config) {
	if c.Name == "" {
		c.Name = "emotion-echo-chat-svc"
	}
	if c.Host == "" {
		c.Host = "0.0.0.0"
	}
	if c.Port == 0 {
		c.Port = 8890
	}
	if c.SkyWalking.OAPAddr == "" {
		c.SkyWalking.OAPAddr = "localhost:11800"
	}
	if c.Postgres.MaxOpenConns == 0 {
		c.Postgres.MaxOpenConns = 10
	}
	if c.Postgres.MaxIdleConns == 0 {
		c.Postgres.MaxIdleConns = 5
	}
	if c.Kafka.BrokersCSV == "" {
		c.Kafka.BrokersCSV = "localhost:9092"
	}
	if c.Kafka.GroupID == "" {
		c.Kafka.GroupID = "chat-svc"
	}
	// Kafka.Enabled 默认 true（R3 高风险:yaml 显式 false 必须覆盖默认 true——已由 TestLoad_DefaultsBeforeYaml 钉死）
	if c.Nacos.Addr == "" {
		c.Nacos.Addr = "emotion-echo-nacos:8848"
	}
	if c.Nacos.Namespace == "" {
		c.Nacos.Namespace = "emotion-echo-dev"
	}
	if c.Nacos.GroupName == "" {
		c.Nacos.GroupName = "DEFAULT_GROUP"
	}
	if c.AIService.GRPCAddr == "" {
		c.AIService.GRPCAddr = "emotion-echo-ai-svc:8892"
	}
}
