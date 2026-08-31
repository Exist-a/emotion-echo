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
	Nacos      Nacos
}

// Nacos 注册中心 + 配置中心配置（Stage 31 PR-09）
type Nacos struct {
	Enabled   bool   `json:",default=true"`
	Addr      string `json:",default=emotion-echo-nacos:8848"`
	Namespace string `json:",default=emotion-echo-dev"`
	GroupName string `json:",default=DEFAULT_GROUP"`
	HotReload bool   `json:",default=false"`
}