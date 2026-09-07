// Package config 提供 assessment-svc 的配置结构（Stage 41 PR-2）
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

type Config struct {
	Name       string
	Host       string
	Port       int
	SkyWalking SkyWalking
	Postgres   Postgres
	Nacos      Nacos
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
// 调用顺序：SetDefaults → yaml.Unmarshal（yaml 显式值覆盖默认；R3 反向已钉死）。
func SetDefaults(c *Config) {
	if c.Name == "" {
		c.Name = "emotion-echo-assessment-svc"
	}
	if c.Host == "" {
		c.Host = "0.0.0.0"
	}
	if c.Port == 0 {
		c.Port = 8889
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
	// Nacos 默认值：Enabled=true（dev compose 依赖 Nacos 注册）
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
