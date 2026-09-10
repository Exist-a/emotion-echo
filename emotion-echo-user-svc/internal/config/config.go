// Package config 提供 user-svc 的配置结构（Stage 41 PR-3）
//
// 改造点：去掉所有 json:",default=X" tag（go-zero conf 才需要）；
// 改为 SetDefaults 函数，在 main.go 加载 yaml 之前调用（shared/pkg/config 契约）。
package config

// SkyWalking 链路追踪配置
type SkyWalking struct {
	OAPAddr     string
	ServiceName string
	Enabled     bool
}

// Postgres 数据库连接配置
type Postgres struct {
	DSN          string
	MaxOpenConns int
	MaxIdleConns int
}

// Nacos 注册中心 + 配置中心配置（Stage 31 PR-07 引入）
//
// Enabled=false 时 user-svc 不注册到 Nacos（dev 单机调试可用）；
// dev / prod 默认 true。
type Nacos struct {
	Enabled   bool
	Addr      string
	Namespace string
	GroupName string
	HotReload bool
}

// GRPCServer user-svc 暴露的 gRPC server 配置（Stage 62 PR-3.2）
//
// Port=0 表示不启动 gRPC server（向后兼容老 yaml）；新 yaml 显式给 :8887 才生效。
type GRPCServer struct {
	Enabled bool
	Port    int
}

// Config 是 user-svc 的总配置
type Config struct {
	Name       string
	Host       string
	Port       int
	SkyWalking SkyWalking
	Postgres   Postgres
	Nacos      Nacos
	GRPC       GRPCServer
}

// SetDefaults 填零值字段的默认值（shared/pkg/config.MustLoad 契约）。
//
// 调用顺序：SetDefaults → yaml.Unmarshal（yaml 显式值覆盖默认；R3 反向已钉死）。
func SetDefaults(c *Config) {
	if c.Name == "" {
		c.Name = "emotion-echo-user-svc"
	}
	if c.Host == "" {
		c.Host = "0.0.0.0"
	}
	if c.Port == 0 {
		c.Port = 8888
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
	if c.Nacos.Addr == "" {
		c.Nacos.Addr = "emotion-echo-nacos:8848"
	}
	if c.Nacos.Namespace == "" {
		c.Nacos.Namespace = "emotion-echo-dev"
	}
	if c.Nacos.GroupName == "" {
		c.Nacos.GroupName = "DEFAULT_GROUP"
	}
	// Stage 62 PR-3.2：默认启用 user-svc gRPC server（:8887），
	// 老 yaml 无 GRPC 段时 Port=0 不启动；新 yaml 显式给 :8887 才生效。
	if c.GRPC.Port == 0 && c.GRPC.Enabled {
		c.GRPC.Port = 8887
	}
}
