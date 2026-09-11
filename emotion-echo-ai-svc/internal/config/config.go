// Package config 提供 ai-svc 的配置结构（Stage 41 PR-5）
//
// 改造点：去掉所有 json:",default=X" tag + json:",optional" tag（go-zero conf 才需要）；
// 改为 SetDefaults 函数（shared/pkg/config 契约）。
// 含复杂字段：Kafka.Topics 切片默认 + Nacos.Enabled 默认值反转（与其他 svc 不同）。
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

type Kafka struct {
	// BrokersCSV 逗号分隔的 broker 列表（容器内通过 KAFKA_BROKERS env 注入）
	//   例如 "emotion-echo-kafka:9092" 或 "kafka1:9092,kafka2:9092"
	// 启动时在 main.go 解析成 []string
	BrokersCSV string
	GroupID    string
	// Enabled: dev 默认 true（本地启动消费者）；生产 compose 注入 KAFKA_ENABLED=false 时 main.go applyEnvOverrides 覆盖。
	Enabled bool
	Topics  []string
	// DLQTopic Stage 30-C A2：死信队列 topic。空 = 不启用 DLQ（退化原行为）。
	DLQTopic string
	// MaxRetries Stage 30-C A2：失败最大重试次数。0 = 默认 3。
	MaxRetries int
}

type LLM struct {
	BaseURL        string
	GRPCAddr       string
	InternalAPIKey string // empty = auth disabled
	Enabled        bool
	Timeout        int
	// Model Stage 35 PR-8：模型名（DeepSeek / OpenAI / 兼容实现各异），env 注入
	Model string
}

// GRPCServer ai-svc 暴露的 gRPC server 配置（Stage 19）
type GRPCServer struct {
	Enabled bool
	Port    int
}

// Stage 22-A: 多模态 AI 模型服务配置
//
// 三个服务都是可选的：URL 为空时客户端直接返回 ErrNotConfigured，
// 调用方（analyzer / consumer）应降级到文本 LLM。
//
// 容器内通过 FER_BASE_URL / SENSEVOICE_BASE_URL / XTTS_BASE_URL env 注入。

type FER struct {
	BaseURL string // empty = FER disabled
	Timeout int
}

type SenseVoice struct {
	BaseURL string
	Timeout int
}

type XTTS struct {
	BaseURL  string
	Timeout  int
	Language string
	Speed    float64
}

type Config struct {
	Name       string
	Host       string
	Port       int
	SkyWalking SkyWalking
	Postgres   Postgres
	Kafka      Kafka
	LLM        LLM
	GRPC       GRPCServer
	FER        FER
	SenseVoice SenseVoice
	XTTS       XTTS
	Nacos      Nacos
}

// Nacos 注册中心 + 配置中心配置（Stage 31 PR-09）
//
// ai-svc 同时暴露 HTTP :8891 与 gRPC :8892；注册时仅注册 HTTP。
//
// Stage 35 PR-7：compose 注入 NACOS_ENABLED=true 时 main.go applyEnvOverrides 覆盖。
type Nacos struct {
	Enabled   bool
	Addr      string
	Namespace string
	GroupName string
	HotReload bool
}

// SetDefaults 填零值字段的默认值（shared/pkg/config.MustLoad 契约）。
//
// 重要语义：
//   - Kafka.Enabled / LLM.Enabled / Nacos.Enabled / GRPCServer.Enabled 等 bool 字段
//     **不在 SetDefaults 里强制覆盖**——因为 Go bool 零值 = false 与"显式 false"无法区分,
//     若强制覆盖会破坏 R3 反向(plan §六 R3:yaml 显式 false 必须保留)。
//     这些字段由 yaml 显式给值或 main.go applyEnvOverrides 通过 env 注入覆盖。
//   - Kafka.Topics 默认 ["chat-events"]（go-zero conf 字面量带引号怪癖,这里直接 []string 赋值）
//   - MaxRetries=0 时填 3（保持原行为）
func SetDefaults(c *Config) {
	if c.Name == "" {
		c.Name = "emotion-echo-ai-svc"
	}
	if c.Host == "" {
		c.Host = "0.0.0.0"
	}
	if c.Port == 0 {
		c.Port = 8891
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
		c.Kafka.GroupID = "ai-svc"
	}
	if len(c.Kafka.Topics) == 0 {
		c.Kafka.Topics = []string{"chat-events"}
	}
	// Kafka.Enabled 不在 SetDefaults 默认（bool 零值 = false 与"显式 false"无法区分）。
	// dev compose env KAFKA_ENABLED=true 必须在 applyEnvOverrides 显式覆盖。
	// 详见 main.go applyEnvOverrides 函数注释。
	if c.Kafka.MaxRetries == 0 {
		c.Kafka.MaxRetries = 3
	}
	if c.LLM.BaseURL == "" {
		c.LLM.BaseURL = "http://localhost:8000"
	}
	if c.LLM.GRPCAddr == "" {
		c.LLM.GRPCAddr = "localhost:50051"
	}
	if c.LLM.Timeout == 0 {
		c.LLM.Timeout = 3
	}
	if c.LLM.Model == "" {
		c.LLM.Model = "deepseek-chat"
	}
	if c.GRPC.Port == 0 {
		c.GRPC.Port = 8892
	}
	if c.FER.Timeout == 0 {
		c.FER.Timeout = 10
	}
	if c.SenseVoice.Timeout == 0 {
		c.SenseVoice.Timeout = 30
	}
	if c.XTTS.Timeout == 0 {
		c.XTTS.Timeout = 60
	}
	if c.XTTS.Language == "" {
		c.XTTS.Language = "zh-cn"
	}
	if c.XTTS.Speed == 0 {
		c.XTTS.Speed = 0.75
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
