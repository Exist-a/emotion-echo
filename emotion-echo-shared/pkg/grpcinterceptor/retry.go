// Package grpcinterceptor 的 client retry 实现

package grpcinterceptor

import (
	"context"
	"math/rand"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// RetryOptions 配置 ClientRetryInterceptor 的退避策略
//
// 字段：
//   - MaxAttempts:       总尝试次数（含首次），默认 3
//   - InitialBackoff:    第一次重试前等待时间，默认 100ms
//   - MaxBackoff:        单次 backoff 上限（指数退避达到后封顶），默认 2s
//   - BackoffMultiplier: 每次 backoff 乘子，默认 2.0
//   - Jitter:            是否加随机抖动（避免雪崩），默认 true
//   - RetryableCodes:    触发重试的 gRPC code 列表
type RetryOptions struct {
	MaxAttempts       int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
	Jitter            bool
	RetryableCodes    []codes.Code
}

// DefaultRetryOptions 返回默认 retry 配置
//
// 3 次尝试，100ms→200ms 指数退避，启用 jitter
// 重试 Unavailable / DeadlineExceeded / Aborted / ResourceExhausted
func DefaultRetryOptions() RetryOptions {
	return RetryOptions{
		MaxAttempts:       3,
		InitialBackoff:    100 * time.Millisecond,
		MaxBackoff:        2 * time.Second,
		BackoffMultiplier: 2.0,
		Jitter:            true,
		RetryableCodes: []codes.Code{
			codes.Unavailable,
			codes.DeadlineExceeded,
			codes.Aborted,
			codes.ResourceExhausted,
		},
	}
}

// isRetryable 判断 err 是否应该触发重试
//
// 规则：
//   - 非 status error（如 TCP RST）视为 transient → 重试
//   - status error 在 RetryableCodes 内 → 重试
//   - 其他 status error → 不重试（业务错误）
func isRetryable(err error, codes []codes.Code) bool {
	if err == nil {
		return false
	}
	st, ok := status.FromError(err)
	if !ok {
		// 非 gRPC status error（网络层错误）默认重试
		return true
	}
	for _, c := range codes {
		if st.Code() == c {
			return true
		}
	}
	return false
}

// nextBackoff 计算下一次重试前的等待时间
//
// 公式：backoff = min(initial * multiplier^(attempt-1), max)
// 可选 jitter：backoff *= rand(0.5, 1.0)
func (o RetryOptions) nextBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	// attempt 1: initial * 1
	// attempt 2: initial * multiplier
	// attempt n: initial * multiplier^(n-1)
	backoff := float64(o.InitialBackoff)
	for i := 1; i < attempt; i++ {
		backoff *= o.BackoffMultiplier
	}
	if time.Duration(backoff) > o.MaxBackoff {
		backoff = float64(o.MaxBackoff)
	}
	if o.Jitter {
		// jitter 50%-100%（避免雪崩）
		// rand.Float64() ∈ [0, 1)，所以 0.5+rand*0.5 ∈ [0.5, 1.0)
		backoff *= 0.5 + rand.Float64()*0.5
	}
	return time.Duration(backoff)
}

// ClientRetryInterceptor 返回带退避重试的 unary client interceptor
//
// 行为：
//   - 第 1 次失败若是 retryable → backoff → 重试
//   - 达到 MaxAttempts → 返回最后一次错误
//   - ctx 取消 → 立即停止（不重试）
//   - 非 retryable 错误 → 立即返回（不重试）
//
// 适用：
//   - transient 网络错误（conn reset / Unavailable）
//   - DeadlineExceeded（对端临时过载）
//   - 不适用：业务错误（Unauthenticated / NotFound / InvalidArgument）
func ClientRetryInterceptor(retryOpts RetryOptions) grpc.UnaryClientInterceptor {
	return func(
		ctx context.Context,
		method string,
		req, reply interface{},
		cc *grpc.ClientConn,
		invoker grpc.UnaryInvoker,
		callOpts ...grpc.CallOption,
	) error {
		var lastErr error
		for attempt := 1; attempt <= retryOpts.MaxAttempts; attempt++ {
			// ctx 取消检查（每次重试前）
			if err := ctx.Err(); err != nil {
				if lastErr != nil {
					return lastErr
				}
				return err
			}

			err := invoker(ctx, method, req, reply, cc, callOpts...)
			lastErr = err

			// 成功 → 返回
			if err == nil {
				return nil
			}

			// 不可重试 → 立即返回
			if !isRetryable(err, retryOpts.RetryableCodes) {
				return err
			}

			// 最后一次尝试后不再等
			if attempt == retryOpts.MaxAttempts {
				break
			}

			// 等 backoff（ctx 可中断等待）
			backoff := retryOpts.nextBackoff(attempt)
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return lastErr
			case <-timer.C:
				// 继续重试
			}
		}
		return lastErr
	}
}
