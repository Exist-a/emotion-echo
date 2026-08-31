// Package config 提供 ai-svc 的配置结构
package config

type SkyWalking struct {
	OAPAddr     string `json:",default=localhost:11800"`
	ServiceName string
	Enabled     bool `json:",default=false"`
}

type Postgres struct {
	DSN          string
	MaxOpenConns int `json:",default=10"`
	MaxIdleConns int `json:",default=5"`
}

type Kafka struct {
	// BrokersCSV 逗号分隔的 broker 列表（容器内通过 KAFKA_BROKERS env 注入）
	//   例如 "emotion-echo-kafka:9092" 或 "kafka1:9092,kafka2:9092"
	// 启动时在 main.go 解析成 []string
	BrokersCSV string `json:",default=localhost:9092"`
	GroupID    string `json:",default=ai-svc"`
	Enabled    bool   `json:",default=false"`
	Topics     []string `json:",default=[\"chat-events\"]"`
	// DLQTopic Stage 30-C A2：死信队列 topic。空 = 不启用 DLQ（退化原行为）。
	DLQTopic string `json:",optional"`
	// MaxRetries Stage 30-C A2：失败最大重试次数。0 = 默认 3。
	MaxRetries int `json:",optional"`
}

type LLM struct {
	BaseURL         string `json:",default=http://localhost:8000"`
	GRPCAddr        string `json:",default=localhost:50051"`
	InternalAPIKey  string `json:",default="` // empty = auth disabled
	Enabled         bool   `json:",default=false"`
	Timeout         int    `json:",default=3"`
}

// GRPCServer ai-svc 暴露的 gRPC server 配置（Stage 19）
type GRPCServer struct {
	Enabled bool `json:",default=true"`
	Port    int  `json:",default=8892"`
}

// Stage 22-A: 多模态 AI 模型服务配置
//
// 三个服务都是可选的：URL 为空时客户端直接返回 ErrNotConfigured，
// 调用方（analyzer / consumer）应降级到文本 LLM。
//
// 容器内通过 FER_BASE_URL / SENSEVOICE_BASE_URL / XTTS_BASE_URL env 注入。

type FER struct {
	BaseURL string `json:",default="` // empty = FER disabled
	Timeout int    `json:",default=10"`
}

type SenseVoice struct {
	BaseURL string `json:",default="`
	Timeout int    `json:",default=30"`
}

type XTTS struct {
	BaseURL  string `json:",default="`
	Timeout  int    `json:",default=60"`
	Language string `json:",default=zh-cn"`
	Speed    float64 `json:",default=0.75"`
}

type Config struct {
	Name        string `json:",default=emotion-echo-ai-svc"`
	Host        string `json:",default=0.0.0.0"`
	Port        int    `json:",default=8891"`
	SkyWalking  SkyWalking
	Postgres    Postgres
	Kafka       Kafka
	LLM         LLM
	GRPC        GRPCServer
	FER         FER
	SenseVoice  SenseVoice
	XTTS        XTTS
	Nacos       Nacos
}

// Nacos 注册中心 + 配置中心配置（Stage 31 PR-09）
//
// ai-svc 同时暴露 HTTP :8891 与 gRPC :8892；注册时仅注册 HTTP（SDK 实例元数据
// 会携带 grpc_port=8892 供 Stage 32 APISIX upstream 决定是否双注册）。
type Nacos struct {
	Enabled   bool   `json:",default=true"`
	Addr      string `json:",default=emotion-echo-nacos:8848"`
	Namespace string `json:",default=emotion-echo-dev"`
	GroupName string `json:",default=DEFAULT_GROUP"`
	HotReload bool   `json:",default=false"`
}