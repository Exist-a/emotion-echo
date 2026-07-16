// Package bootstrap 提供 ai-svc 启动期的依赖检查（Stage 20-P0-3）
//
// 用法：
//   import "emotion-echo-ai-svc/internal/bootstrap"
//   if err := bootstrap.CheckTCP(ctx, addr, 5*time.Second); err != nil {
//       if bootstrap.ShouldFailFast() && bootstrap.IsRequired("postgres") {
//           return err  // 启动失败 → 容器退出 → orchestrator 重启
//       }
//       // dev mode：仅 warn，继续启动
//   }
//
// 设计取舍：所有 check 都用纯 TCP 探活（端口可达），避免引入额外数据库 driver 依赖。
// 真正的连接 / auth 校验交给后续 gorm / sarama / grpc.Dial 失败处理。
//
// 行为由环境变量控制：
//   STARTUP_STRICT=true|false    默认 false（dev 兼容）
//                                生产设为 true → 关键依赖失败立即退出
//   STARTUP_STRICT_DEPS=postgres,kafka,skywalking,llm
//                                哪些依赖触发 fail-fast（CSV）
//                                默认 "postgres,kafka,skywalking"
package bootstrap

import (
	"context"
	"net"
	"os"
	"strings"
	"time"
)

// ShouldFailFast 根据 STARTUP_STRICT env 返回是否启用 fail-fast
func ShouldFailFast() bool {
	v := strings.ToLower(os.Getenv("STARTUP_STRICT"))
	return v == "1" || v == "true" || v == "yes"
}

// IsRequired 返回 dep 是否在 STARTUP_STRICT_DEPS 列表里
// dep 名称：postgres / kafka / skywalking / llm
func IsRequired(dep string) bool {
	list := os.Getenv("STARTUP_STRICT_DEPS")
	if list == "" {
		list = "postgres,kafka,skywalking"
	}
	for _, d := range strings.Split(list, ",") {
		if strings.TrimSpace(strings.ToLower(d)) == strings.ToLower(dep) {
			return true
		}
	}
	return false
}

// CheckTCP 通用 TCP 探活
// addr 形如 "emotion-echo-postgres:5432" / "emotion-echo-kafka:9092" / "emotion-echo-sw-oap:11800" / "emotion-llm-service:50051"
func CheckTCP(ctx context.Context, addr string, timeout time.Duration) error {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return &ErrDial{Addr: addr, Err: err}
	}
	_ = conn.Close()
	return nil
}

// ErrDial 包装 dial 错误，附带 addr 信息
type ErrDial struct {
	Addr string
	Err  error
}

func (e *ErrDial) Error() string {
	return "dial " + e.Addr + ": " + e.Err.Error()
}

func (e *ErrDial) Unwrap() error { return e.Err }

// CheckMultiple 并行检查多个 addr，全部成功才返回 nil
// 任一失败 → 返回第一个失败的 ErrDial
func CheckMultiple(ctx context.Context, addrs map[string]string, timeout time.Duration) map[string]error {
	type kv struct {
		name string
		addr string
	}
	jobs := make([]kv, 0, len(addrs))
	for k, v := range addrs {
		jobs = append(jobs, kv{k, v})
	}
	results := make(map[string]error, len(jobs))
	done := make(chan struct{}, len(jobs))

	for _, j := range jobs {
		go func(name, addr string) {
			results[name] = CheckTCP(ctx, addr, timeout)
			done <- struct{}{}
		}(j.name, j.addr)
	}

	for range jobs {
		<-done
	}
	return results
}