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

	// Stage 86：outbox dead 状态机阈值配置化。
	// MaxAttempts=发布重试上限，超出 status=dead（Stage 43 PR-A6.1 语义）；
	// 0 = 关闭 dead 状态机（无限重试，向后兼容）。
	Outbox Outbox

	// Stage 36-A3.2: ai-svc gRPC 地址（dev fallback 同步写中性情绪用）。
	// 空 = 不启用 dev fallback（保持 NoopAIClient），与 KAFKA_ENABLED 组合决定是否调 ai-svc。
	AIService AIService

	// Stage 58 PR-GRPC-3：chat-svc 自身 gRPC server 配置（暴露 ChatService 给 BFF）。
	// Port=0 表示不启动 gRPC server（向后兼容老 yaml）。
	GRPC GRPCServer
}

// Outbox relay dead 状态机配置（Stage 86）+ Round 2.1 §D2 cleanup
type Outbox struct {
	MaxAttempts      int // 重试超阈值 → dead（0 关闭 dead 状态机）
	CleanupEnabled    bool // Round 2.1: 是否启 cleanup ticker（false 时永不清理—— dev 单测场景）
	CleanupIntervalS int  // cleanup ticker 间隔秒数（0 → 默认 3600 = 1h）
	SentRetentionDays  int // sent 行保留天数（0 → 默认 7）
	DeadRetentionDays  int // dead 行保留天数（0 → 默认 30）
}

// AIService ai-svc 客户端配置（Stage 36-A3.2）
type AIService struct {
	GRPCAddr string
}

// GRPCServer chat-svc 暴露的 gRPC server 配置（Stage 58 PR-GRPC-3）
type GRPCServer struct {
	Enabled bool
	Port    int
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
	// Stage 86：outbox dead 阈值默认 100（与 Stage 43 NewRelay 原硬编码值一致）
	if c.Outbox.MaxAttempts == 0 {
		c.Outbox.MaxAttempts = 100
	}
	// Round 2.1 §D2: cleanup ticker 默认值（dev 不启用 — dev 库无百万行；prod 启用 1h 间隔）
	// CleanupEnabled 默认 false（向后兼容 — 单测/dev 不能悄悄启 ticker）；prod profile 显式 yaml true 或 env OUTBOX_CLEANUP_ENABLED=true 启用
	if c.Outbox.CleanupIntervalS == 0 {
		c.Outbox.CleanupIntervalS = 3600 // 1h
	}
	if c.Outbox.SentRetentionDays == 0 {
		c.Outbox.SentRetentionDays = 7
	}
	if c.Outbox.DeadRetentionDays == 0 {
		c.Outbox.DeadRetentionDays = 30
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
	if c.AIService.GRPCAddr == "" {
		c.AIService.GRPCAddr = "emotion-echo-ai-svc:8892"
	}
	// Stage 58 PR-GRPC-3：默认启用 chat-svc gRPC server（:8892），
	// 与 ai-svc :8892 端口对齐（不同 svc，端口命名空间隔离）。
	// 老 yaml 无 GRPC 段时 Port=0 不启动；新 yaml 显式给 :8892 才生效。
	if c.GRPC.Port == 0 && c.GRPC.Enabled {
		c.GRPC.Port = 8892
	}
}
