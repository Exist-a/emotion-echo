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

// Nacos 注册中心 + 配置中心配置（Stage 31 PR-07 引入）
//
// Enabled=false 时 user-svc 不注册到 Nacos（dev 单机调试可用）；
// dev / prod 默认 true。
type Nacos struct {
	Enabled   bool   `json:",default=true"`             // 是否启用 Nacos
	Addr      string `json:",default=emotion-echo-nacos:8848"` // Nacos server 地址（容器内默认 DNS）
	Namespace string `json:",default=emotion-echo-dev"`        // Nacos namespace（dev/prod 隔离）
	GroupName string `json:",default=DEFAULT_GROUP"`           // Nacos group
	HotReload bool   `json:",default=false"`                   // 是否启用 ListenConfig 热重载（prod 启用，dev 默认关闭避免日志噪音）
}

// Config 是 user-svc 的总配置（手写，不再依赖 go-zero rest.RestConf）
type Config struct {
	Name       string `json:",default=emotion-echo-user-svc"` // 服务名，用于 Nacos 注册 + tracer
	Host       string `json:",default=0.0.0.0"`
	Port       int    `json:",default=8888"`
	SkyWalking SkyWalking
	Postgres   Postgres
	Nacos      Nacos
}
