// scripts/grpc_smoke/main.go
//
// Stage 62 PR-3.4: BFF → 3 svc gRPC 端到端冒烟客户端
//
// 用途：对 user-svc / assessment-svc / analytics-svc 的 gRPC server 做
// 真实 RPC 调用验证（health check + 业务 RPC）。
//
// 用法：
//   go run ./scripts/grpc_smoke \
//     -user=localhost:18887 -assessment=localhost:18886 -analytics=localhost:18885
//
// 前置：3 svc 容器已起 + gRPC 端口映射到宿主（tmp/grpc-smoke-override.yml）
//
// 为什么独立程序而不写 _test.go：
//   _test.go 需要真实 TCP 环境（compose 网络），不属于 unit test；
//   独立 main 包既可宿主跑也可容器内跑，便于 PR-3.4 冒烟。

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	emotionanalytics "github.com/emotion-echo/shared/pkg/emotionanalytics"
	emotionassessment "github.com/emotion-echo/shared/pkg/emotionassessment"
	emotionuser "github.com/emotion-echo/shared/pkg/emotionuser"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

var (
	userAddr       = flag.String("user", "localhost:18887", "user-svc gRPC addr")
	assessmentAddr = flag.String("assessment", "localhost:18886", "assessment-svc gRPC addr")
	analyticsAddr  = flag.String("analytics", "localhost:18885", "analytics-svc gRPC addr")
	testUserID     = flag.Int64("user-id", 1, "x-user-id metadata for authed RPCs")
)

func main() {
	flag.Parse()

	pass, fail := 0, 0
	check := func(name string, err error) {
		if err != nil {
			fmt.Printf("  ❌ %s: %v\n", name, err)
			fail++
			return
		}
		fmt.Printf("  ✅ %s\n", name)
		pass++
	}

	dial := func(addr string) *grpc.ClientConn {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		conn, err := grpc.DialContext(ctx, addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock())
		if err != nil {
			fmt.Printf("FATAL: dial %s: %v\n", addr, err)
			os.Exit(1)
		}
		return conn
	}

	// authCtx 返回 (ctx, cancel)：caller 必须 defer cancel()
	// （不能在此函数内 defer cancel —— 函数返回即取消，RPC 会 context canceled）
	authCtx := func() (context.Context, context.CancelFunc) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		return metadata.AppendToOutgoingContext(ctx, "x-user-id", fmt.Sprintf("%d", *testUserID)), cancel
	}

	fmt.Println("=== §契约 10 · BFF → user-svc gRPC ===")
	userConn := dial(*userAddr)
	defer userConn.Close()

	// 10.1 health check
	{
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resp, err := healthpb.NewHealthClient(userConn).Check(ctx, &healthpb.HealthCheckRequest{})
		if err != nil {
			check("10.1 health check", err)
		} else {
			check(fmt.Sprintf("10.1 health check (status=%s)", resp.Status), nil)
		}
	}

	// 10.2 missing x-user-id → Unauthenticated
	{
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := emotionuser.NewUserServiceClient(userConn).GetMe(ctx, &emotionuser.GetMeRequest{})
		st, _ := status.FromError(err)
		if st.Code().String() == "Unauthenticated" {
			check("10.2 GetMe 无 x-user-id → Unauthenticated", nil)
		} else {
			check("10.2 GetMe 无 x-user-id → Unauthenticated", fmt.Errorf("got %s: %v", st.Code(), err))
		}
	}

	// 10.3 GetUserById with x-user-id
	{
		ctx, cancel := authCtx()
		defer cancel()
		resp, err := emotionuser.NewUserServiceClient(userConn).GetUserById(ctx, &emotionuser.GetUserByIdRequest{UserId: *testUserID})
		if err != nil {
			st, _ := status.FromError(err)
			// NOT_FOUND 也算链路通（证明拦截器 + logic 层都走通了）
			if st.Code().String() == "NotFound" {
				check("10.3 GetUserById → NotFound（链路通，DB 无此 user）", nil)
			} else {
				check("10.3 GetUserById", fmt.Errorf("got %s: %v", st.Code(), err))
			}
		} else {
			check(fmt.Sprintf("10.3 GetUserById → id=%d username=%q", resp.Id, resp.Username), nil)
		}
	}

	fmt.Println("=== §契约 11 · BFF → assessment-svc gRPC ===")
	assessmentConn := dial(*assessmentAddr)
	defer assessmentConn.Close()

	// 11.1 health check
	{
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resp, err := healthpb.NewHealthClient(assessmentConn).Check(ctx, &healthpb.HealthCheckRequest{})
		if err != nil {
			check("11.1 health check", err)
		} else {
			check(fmt.Sprintf("11.1 health check (status=%s)", resp.Status), nil)
		}
	}

	// 11.2 ListSurveys
	{
		ctx, cancel := authCtx()
		defer cancel()
		resp, err := emotionassessment.NewAssessmentServiceClient(assessmentConn).ListSurveys(ctx, &emotionassessment.ListSurveysRequest{Limit: 10})
		if err != nil {
			st, _ := status.FromError(err)
			check("11.2 ListSurveys", fmt.Errorf("got %s: %v", st.Code(), err))
		} else {
			check(fmt.Sprintf("11.2 ListSurveys → %d items (total=%d)", len(resp.Items), resp.Total), nil)
		}
	}

	fmt.Println("=== §契约 12 · BFF → analytics-svc gRPC ===")
	analyticsConn := dial(*analyticsAddr)
	defer analyticsConn.Close()

	// 12.1 health check
	{
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resp, err := healthpb.NewHealthClient(analyticsConn).Check(ctx, &healthpb.HealthCheckRequest{})
		if err != nil {
			check("12.1 health check", err)
		} else {
			check(fmt.Sprintf("12.1 health check (status=%s)", resp.Status), nil)
		}
	}

	// 12.2 ReportsDaily（空数据也算链路通）
	{
		ctx, cancel := authCtx()
		defer cancel()
		resp, err := emotionanalytics.NewAnalyticsServiceClient(analyticsConn).ReportsDaily(ctx, &emotionanalytics.ReportsDailyRequest{
			UserId: *testUserID,
		})
		if err != nil {
			st, _ := status.FromError(err)
			check("12.2 ReportsDaily", fmt.Errorf("got %s: %v", st.Code(), err))
		} else {
			check(fmt.Sprintf("12.2 ReportsDaily → msgCount=%d convCount=%d emotions=%d",
				resp.MessageCount, resp.ConversationCount, len(resp.EmotionDistribution)), nil)
		}
	}

	fmt.Printf("\n=== 结果: %d PASS / %d FAIL ===\n", pass, fail)
	if fail > 0 {
		os.Exit(1)
	}
}