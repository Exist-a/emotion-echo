// Package config 提供 user-svc 的配置结构（去掉 go-zero 依赖版本）
package config

// SkyWalking 链路追踪配置
type SkyWalking struct {
	OAPAddr     string `json:",default=localhost:11800"`
	ServiceName string
	Enabled     bool `json:",default=false"`
}

// Postgres 数据库连接配置
type Postgres struct {
	DSN          string
	MaxOpenConns int `json:",default=10"`
	MaxIdleConns int `json:",default=5"`
}

// Config 是 user-svc 的总配置（手写，不再依赖 go-zero rest.RestConf）
type Config struct {
	Name       string `json:",default=emotion-echo-user-svc"` // 服务名，用于 tracer 等
	Host       string `json:",default=0.0.0.0"`
	Port       int    `json:",default=8888"`
	SkyWalking SkyWalking
	Postgres   Postgres
}