// Package grpcinterceptor 的 stream 实现

package grpcinterceptor

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"

	"github.com/emotion-echo/shared/pkg/logging"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// wrappedServerStream 包装 grpc.ServerStream，统计收发的消息数
type wrappedServerStream struct {
	grpc.ServerStream
	recvCount int
	sendCount int
}

// Recv 计数 + 委托
func (w *wrappedServerStream) RecvMsg(m interface{}) error {
	err := w.ServerStream.RecvMsg(m)
	if err == nil {
		w.recvCount++
	}
	return err
}

// SendMsg 计数 + 委托
func (w *wrappedServerStream) SendMsg(m interface{}) error {
	err := w.ServerStream.SendMsg(m)
	if err == nil {
		w.sendCount++
	}
	return err
}

// =====================================================
// Server Stream Interceptors
// =====================================================

// NewServerStreamLoggingInterceptor 记录 server 端 stream RPC 的开始/结束 + 消息计数
//
// 输出格式：
//
//	[stream-server] method=/.../AnalyzeBatch stream-start
//	[stream-server] method=/.../AnalyzeBatch stream-end duration=120ms sent=3 recv=0 code=OK
func NewServerStreamLoggingInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		start := time.Now()
		// E2E-F-146：ss.Context() 里带着上游透传的 x-trace-id（见 NewServerTracingInterceptor
		// 把它塞进 ctx 的同一路径），所以流式日志也要走带 ctx 的调用，否则查不到链路。
		sctx := ss.Context()
		logging.PrintfContext(sctx, "[stream-server] method=%s stream-start", info.FullMethod)

		wrapped := &wrappedServerStream{ServerStream: ss}
		err := handler(srv, wrapped)

		logging.PrintfContext(
			sctx,
			"[stream-server] method=%s stream-end duration=%dms sent=%d recv=%d code=%v",
			info.FullMethod,
			time.Since(start).Milliseconds(),
			wrapped.sendCount,
			wrapped.recvCount,
			errCode(err),
		)
		return err
	}
}

// NewServerStreamRecoveryInterceptor 捕获 stream handler 内的 panic
//
// panic 会被转成 INTERNAL status 返回给 client，避免 server 进程崩溃
func NewServerStreamRecoveryInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) (err error) {
		defer func() {
			if r := recover(); r != nil {
				logging.PrintfContext(ss.Context(), "[stream-server] PANIC method=%s panic=%v", info.FullMethod, r)
				err = status.Errorf(codes.Internal, "stream handler panic: %v", r)
			}
		}()
		return handler(srv, ss)
	}
}

// =====================================================
// Client Stream Interceptors
// =====================================================

// NewClientStreamLoggingInterceptor 记录 client 端 stream RPC 的开始/结束 + 消息计数
func NewClientStreamLoggingInterceptor() grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		start := time.Now()
		logging.PrintfContext(ctx, "[stream-client] method=%s stream-start desc.ServerStreams=%v",
			method, desc.ServerStreams)

		cs, err := streamer(ctx, desc, cc, method, opts...)
		if err != nil {
			log.Printf(
				"[stream-client] method=%s stream-fail duration=%dms err=%v",
				method, time.Since(start).Milliseconds(), err,
			)
			return nil, err
		}

		// 包装 ClientStream 计数
		return &wrappedClientStream{
			ClientStream: cs,
			method:       method,
			start:        start,
			ctx:          ctx,
		}, nil
	}
}

// NewClientStreamTimeoutInterceptor 给 client 端 stream RPC 加 ctx timeout
func NewClientStreamTimeoutInterceptor(timeout time.Duration) grpc.StreamClientInterceptor {
	return func(
		ctx context.Context,
		desc *grpc.StreamDesc,
		cc *grpc.ClientConn,
		method string,
		streamer grpc.Streamer,
		opts ...grpc.CallOption,
	) (grpc.ClientStream, error) {
		if _, ok := ctx.Deadline(); !ok {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		return streamer(ctx, desc, cc, method, opts...)
	}
}

// wrappedClientStream 包装 grpc.ClientStream，统计消息数
type wrappedClientStream struct {
	grpc.ClientStream
	method string
	start  time.Time
	sent   int
	recvd  int
	// ctx 保存发起 stream 时的 ctx：SendMsg/RecvMsg 是 grpc.ClientStream 的方法，
	// 签名里没有 ctx，而结束日志要能带 trace_id，只能从这儿取（E2E-F-146）。
	ctx context.Context
}

func (w *wrappedClientStream) SendMsg(m interface{}) error {
	err := w.ClientStream.SendMsg(m)
	if err == nil {
		w.sent++
	}
	return err
}

func (w *wrappedClientStream) RecvMsg(m interface{}) error {
	err := w.ClientStream.RecvMsg(m)
	if err == nil {
		w.recvd++
	} else {
		// 收到 error / EOF 时打结束日志
		logging.PrintfContext(
			w.ctx,
			"[stream-client] method=%s stream-end duration=%dms sent=%d recv=%d code=%v",
			w.method, time.Since(w.start).Milliseconds(), w.sent, w.recvd, errCode(err),
		)
	}
	return err
}

func (w *wrappedClientStream) CloseSend() error {
	err := w.ClientStream.CloseSend()
	log.Printf(
		"[stream-client] method=%s close-send duration=%dms sent=%d recv=%d",
		w.method, time.Since(w.start).Milliseconds(), w.sent, w.recvd,
	)
	return err
}

// errCode 把 error 转成 grpc code 字符串（兼容 nil）
func errCode(err error) string {
	if err == nil {
		return "OK"
	}
	st, ok := status.FromError(err)
	if !ok {
		return err.Error()
	}
	return st.Code().String()
}
