package grpcserver

// server_race_test.go —— Addr()/Start() 并发访问的回归钉（E2E-F-63）
//
// 背景：Server.listener 由 Start() 在后台 goroutine 写入，而 Addr() 会被调用方
// （测试轮询、运维探针、`startXxxTestServer` 的等待循环）并发读取。
// 无同步时 -race 实测报 WARNING: DATA RACE，且 5 个服务模块各有一份拷贝。
//
// 契约：Addr() 在 Start() 运行期间可被并发安全调用；Start() 完成后
//      它返回**真实绑定的地址**（而非退化值 :port）。
//
// 注意：本用例的价值在 -race 下才完全体现（无 -race 时不会主动报错），
// 故 CI/dev 必须带 -race 运行本包（go-test.yml 已恢复 -race）。

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
)

func TestServer_Addr_ConcurrentWithStart_NoRace(t *testing.T) {
	// port=0 → 绑定临时端口；直接构造结构体，避免依赖各模块的 New 及其外部依赖
	srv := &Server{grpcServer: grpc.NewServer(), port: 0}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	started := make(chan struct{})
	go func() {
		close(started)
		_ = srv.Start(ctx)
	}()
	<-started

	// 并发轮询 Addr()：这正是原缺陷的触发形态
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 500; j++ {
				_ = srv.Addr()
			}
		}()
	}
	wg.Wait()

	// 等 Start() 完成监听绑定，再断言拿到真实地址（避免时序 flake）
	deadline := time.Now().Add(3 * time.Second)
	addr := ""
	for time.Now().Before(deadline) {
		addr = srv.Addr()
		if addr != "" && addr != ":0" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	assert.NotEqual(t, ":0", addr, "Start() 完成后 Addr() 应返回真实绑定地址")
	assert.NotEmpty(t, addr)
}
