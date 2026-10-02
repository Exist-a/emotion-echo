// E2E-26 #3：真实 grpc.ClientConn + ClientDialOptions 全链的 sw8 注入实测（bufconn，
// 无网络/磁盘依赖）。与 client_sw8_wire_test 的区别：那边手动调拦截器且 cc=nil
// （peer 空 → go2sky errParameter 假红），这里走 grpc.Dial 真链 —— peer/target
// 与生产同形。capture 拦截器挂链尾，读到的是 tracing→timeout→traceid→logging
// 四层处理完的**最终** outgoing metadata。
package grpcinterceptor

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/SkyAPM/go2sky"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
)

func TestClientDialOptions_FullChain_InjectsSw8(t *testing.T) {
	lis := bufconn.Listen(1 << 20)
	t.Cleanup(func() { _ = lis.Close() })

	cliTr, err := go2sky.NewTracer("bufconn-cli", go2sky.WithReporter(fakeReporter{}))
	require.NoError(t, err)

	var finalMD metadata.MD
	capture := func(ctx context.Context, method string, req, reply interface{},
		cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		finalMD, _ = metadata.FromOutgoingContext(ctx)
		return invoker(ctx, method, req, reply, cc, opts...)
	}

	opts := append(ClientDialOptions(NewGo2SkyTracer(cliTr), time.Second),
		grpc.WithChainUnaryInterceptor(capture),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	conn, err := grpc.Dial("passthrough:///bufconn-target", opts...)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// 无 server：连接会失败，但拦截器链已全部执行（transport 在链尾之后）
	_ = conn.Invoke(ctx, "/diag.v1.Svc/Do", []byte{}, new([]byte))

	require.NotNil(t, finalMD, "capture 必须取到 outgoing metadata")
	sw8Vals := finalMD.Get("sw8")
	require.NotEmpty(t, sw8Vals,
		"全链最终 outgoing metadata 缺 sw8 —— tracing 层注入失败或被后续层丢弃")
	parts := strings.Split(sw8Vals[0], "-")
	require.GreaterOrEqual(t, len(parts), 8, "sw8 应 8 段: %s", sw8Vals[0])
	require.Equal(t, "1", parts[0])
	t.Logf("sw8 OK: %s", sw8Vals[0][:48]+"...")
}
