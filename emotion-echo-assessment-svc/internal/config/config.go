// Package config 提供 assessment-svc 的配置结构
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

type Config struct {
	Name       string `json:",default=emotion-echo-assessment-svc"`
	Host       string `json:",default=0.0.0.0"`
	Port       int    `json:",default=8889"`
	SkyWalking SkyWalking
	Postgres   Postgres
}