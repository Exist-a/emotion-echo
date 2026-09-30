package grpcserver

import (
	"context"
	"testing"
	"time"

	"github.com/emotion-echo/shared/pkg/emotionquery"
	"github.com/emotion-echo/shared/pkg/healthcheck"

	"google.golang.org/grpc"
)

// E2E-23 B 组 · 测试点 #13/#14：gRPC health 状态必须会翻转。
//
// 🔴 计划期实测（plan §0 F-d）：5 个 Go 服务的 SetServingStatus 全在
// New() 构造函数里调一次就结束，**没有任何 NOT_SERVING 写入**。
// 停机过程中 grpc.health.v1.Health/Check 仍报 SERVING ⇒
// 负载均衡器/编排层认为"它还活着"，继续往正在关闭的实例打请求。
//
// 根因不只是"少写一行"：healthSrv 是 New() 里的**局部变量**，
// 没存进 Server 结构 ⇒ Start() 的停机分支拿不到它，无从翻转。

// TestGrpcHealth_ReportsServingWhileRunning 基线：运行期应报 SERVING。
func TestGrpcHealth_ReportsServingWhileRunning(t *testing.T) {
	_, conn, stop := startTestServer(t, nil)
	defer stop()
	client := healthcheck.NewClient(conn)

	for _, name := range []string{"", emotionQueryServiceName} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		status, err := client.Check(ctx, name)
		cancel()

		if err != nil {
			t.Fatalf("service=%q 探活失败：%v", name, err)
		}
		if status != healthcheck.ServingStatusServing {
			t.Errorf("service=%q 运行期应为 SERVING，实际 %v", name, status)
		}
	}
}

// TestGrpcHealth_FlipsToNotServingOnShutdown 核心断言（测试点 #13）：
// 收到停机信号后 health 必须翻成 NOT_SERVING，让上游来得及摘流量。
//
// 这是"先摘流量、再关连接"的正确顺序：GracefulStop 会等待在途 RPC，
// 期间连接仍可问出状态，上游据此把新请求发给别的实例。
func TestGrpcHealth_FlipsToNotServingOnShutdown(t *testing.T) {
	srv, conn, stop := startTestServer(t, nil)
	defer stop()
	client := healthcheck.NewClient(conn)

	// 先确认当前是 SERVING（否则后面的翻转无从谈起）
	ctx0, cancel0 := context.WithTimeout(context.Background(), 3*time.Second)
	before, err := client.Check(ctx0, emotionQueryServiceName)
	cancel0()
	if err != nil {
		t.Fatalf("停机前探活失败：%v", err)
	}
	if before != healthcheck.ServingStatusServing {
		t.Fatalf("前置条件不满足：停机前应为 SERVING，实际 %v", before)
	}

	// 触发停机信号：生产路径是 main 收到 SIGTERM 后走这里。
	srv.MarkShuttingDown()

	// 轮询等待翻转
	deadline := time.Now().Add(3 * time.Second)
	var lastStatus healthcheck.ServingStatus
	var lastErr error
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		lastStatus, lastErr = client.Check(ctx, emotionQueryServiceName)
		cancel()
		if lastErr == nil && lastStatus == healthcheck.ServingStatusNotServing {
			return // 成功翻转
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("MarkShuttingDown 后 3s 内 gRPC health 未翻转为 NOT_SERVING（最后读到 %v，err=%v）\n"+
		"⇒ 上游在实例关闭期间仍认为它可接流量，会把新请求打进正在停机的实例",
		lastStatus, lastErr)
}

// TestGrpcHealth_MarkShuttingDownIsIdempotent 停机信号可能被重复送达
// （SIGTERM 后再 SIGKILL、Restart 重试等），翻转必须幂等且不 panic。
func TestGrpcHealth_MarkShuttingDownIsIdempotent(t *testing.T) {
	srv, conn, stop := startTestServer(t, nil)
	defer stop()
	client := healthcheck.NewClient(conn)

	srv.MarkShuttingDown()
	srv.MarkShuttingDown() // 重复调用不应 panic
	srv.MarkShuttingDown()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	status, err := client.Check(ctx, emotionQueryServiceName)
	if err != nil {
		t.Fatalf("重复调用后探活失败：%v", err)
	}
	if status != healthcheck.ServingStatusNotServing {
		t.Errorf("重复调用后应保持 NOT_SERVING，实际 %v", status)
	}
}

// TestGrpcHealth_UnknownService 守住 grpc.health 的既有语义：
// 未注册 service 不得被误报成 SERVING。
func TestGrpcHealth_UnknownService(t *testing.T) {
	_, conn, stop := startTestServer(t, nil)
	defer stop()
	client := healthcheck.NewClient(conn)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	status, err := client.Check(ctx, "emotion.NeverRegistered")
	if err != nil {
		return // 部分实现对未知 service 返回 NotFound 错误，也算正确
	}
	if status == healthcheck.ServingStatusServing {
		t.Errorf("未注册的 service 不应报 SERVING，实际 %v", status)
	}
}

// 编译期确认 client 参数类型，避免误传。
var _ func(*grpc.ClientConn) *healthcheck.Client = healthcheck.NewClient

// TestGrpcHealth_RegisteredNameMatchesProtoServiceName 钉住"注册名必须是 proto 真名"。
//
// 这条测试存在的理由：E2E-23 复核轮发现注册名是 `emotion.AI` —— proto 里不存在这个名字。
// 后果是 per-service 健康只有测试自己查得到，真实 gRPC 客户端一律 NOT_FOUND；
// 而旧测试用同一个字面量断言，与实现共享错误前提 ⇒ 永远绿（自证循环）。
//
// 现在断言的基准是**独立来源**：emotion-echo-shared 生成的 pb.go 里的 ServiceDesc.ServiceName。
func TestGrpcHealth_RegisteredNameMatchesProtoServiceName(t *testing.T) {
	const wantRealName = "emotion_ai.v1.EmotionQueryService" // = emotionquery.EmotionQueryService_ServiceDesc.ServiceName

	if emotionQueryServiceName != wantRealName {
		t.Fatalf("注册的 per-service 名 = %q，应为 proto 真名 %q；"+
			"写成 proto 里不存在的名字会让真实客户端 Check 时拿到 NOT_FOUND，"+
			"而只被自己写的测试查到（自证循环）",
			emotionQueryServiceName, wantRealName)
	}

	// 反向确认：这个名字确实来自 proto，而不是凭空捏造
	if got := emotionquery.EmotionQueryService_ServiceDesc.ServiceName; got != wantRealName {
		t.Fatalf("基准本身错了：pb.go 里 ServiceDesc.ServiceName = %q，期望 %q", got, wantRealName)
	}

	// 且真实名字必须真的可查（不只是常量对上了）
	_, conn, stop := startTestServer(t, nil)
	defer stop()
	client := healthcheck.NewClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	status, err := client.Check(ctx, wantRealName)
	if err != nil {
		t.Fatalf("用 proto 真名探活失败：%v", err)
	}
	if status != healthcheck.ServingStatusServing {
		t.Fatalf("用 proto 真名探活得到 %v，期望 SERVING", status)
	}
}
