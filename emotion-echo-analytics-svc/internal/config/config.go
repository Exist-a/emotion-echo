// Package config 提供 analytics-svc 的配置结构（Stage 41 PR-6）
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

// Kafka chat-events consumer 配置（与 ai-svc 同模式）
//
// Stage 36-A1.3：Kafka.Enabled 默认值改为 true——analytics-svc 是 chat-events
// 的下游消费者（user_behavior_events 数据源）。
type Kafka struct {
	BrokersCSV string
	GroupID    string
	Enabled    bool
	Topics     []string
	// DLQTopic ADR-19 PR-A3.2: 非空时 main.go 用 NewKafkaDLQPublisher 注入,
	// 替代默认 NoopDLQPublisher{}。空字符串保持 Noop（向后兼容）。
	DLQTopic string
}

// GRPCServer analytics-svc 暴露的 gRPC server 配置（Stage 62 PR-3.2）
//
// Port=0 表示不启动 gRPC server（向后兼容老 yaml）；新 yaml 显式给 :8885 才生效。
type GRPCServer struct {
	Enabled bool
	Port    int
}

type Config struct {
	Name       string
	Host       string
	Port       int
	SkyWalking SkyWalking
	Postgres   Postgres
	Kafka      Kafka
	Nacos      Nacos
	GRPC       GRPCServer

	// TriggerQueueCap Round 3 part 2: async trigger queue buffer size.
	// <=0 用 trigger.DefaultQueueCap (64).
	TriggerQueueCap int
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
// 重要：Kafka.Enabled / SkyWalking.Enabled / Nacos.Enabled / Nacos.HotReload
// 等 bool 字段**不在 SetDefaults 里强制覆盖**（R3 反向：yaml 显式 false 必须保留）。
// Kafka.Topics 默认 ["chat-events"]；TriggerQueueCap 默认 64。
func SetDefaults(c *Config) {
	if c.Name == "" {
		c.Name = "emotion-echo-analytics-svc"
	}
	if c.Host == "" {
		c.Host = "0.0.0.0"
	}
	if c.Port == 0 {
		c.Port = 8892
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
		c.Kafka.GroupID = "analytics-svc"
	}
	if len(c.Kafka.Topics) == 0 {
		c.Kafka.Topics = []string{"chat-events"}
	}
	if c.TriggerQueueCap == 0 {
		c.TriggerQueueCap = 64
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
	// Stage 62 PR-3.2：默认启用 analytics-svc gRPC server（:8885），
	// 老 yaml 无 GRPC 段时 Port=0 不启动；新 yaml 显式给 :8885 才生效。
	if c.GRPC.Port == 0 && c.GRPC.Enabled {
		c.GRPC.Port = 8885
	}
}
