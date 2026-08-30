// Package config 提供 analytics-svc 的配置结构
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

// Kafka chat-events consumer 配置（与 ai-svc 同模式）
type Kafka struct {
	// BrokersCSV 逗号分隔的 broker 列表（容器内通过 KAFKA_BROKERS env 注入）
	BrokersCSV string `json:",default=localhost:9092"`
	GroupID    string `json:",default=analytics-svc"`
	Enabled    bool   `json:",default=false"` // opt-in：显式 true 才启动 consumer
	Topics     []string `json:",default=[\"chat-events\"]"`
}

type Config struct {
	Name       string `json:",default=emotion-echo-analytics-svc"`
	Host       string `json:",default=0.0.0.0"`
	Port       int    `json:",default=8892"`
	SkyWalking SkyWalking
	Postgres   Postgres
	Kafka      Kafka

	// TriggerQueueCap Round 3 part 2: async trigger queue buffer size.
	// <=0 用 trigger.DefaultQueueCap (64).
	TriggerQueueCap int `json:",default=64"`
}