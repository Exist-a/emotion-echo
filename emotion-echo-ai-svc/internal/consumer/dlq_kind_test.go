package consumer

import (
	"context"
	"testing"
)

// fakeRealDLQPublisher 代表"DLQ 真已配置"的实现（非 Noop）。
type fakeRealDLQPublisher struct{}

func (fakeRealDLQPublisher) Publish(_ context.Context, _ DLQEntry) error { return nil }

// TestDLQKind 装配种类的可读名 —— 供启动日志区分"真 DLQ"与"Noop（未配置）"。
//
// 为什么需要：`main.go` 的 `consumer started` 日志此前用 `dlq != nil` 判断，
// 而生产装配在未配 DLQ 时传的是 `NoopDLQPublisher{}`（**非 nil**）⇒ 日志恒打印
// `dlq=true`，与事实相反（CL-03 T-07 运行时验证时实测到：容器 `dlq=true` 但
// 毒消息实际走了"Noop：无处投递"分支）。
func TestDLQKind(t *testing.T) {
	tests := []struct {
		name string
		in   DLQPublisher
		want string
	}{
		{"nil（未装配）→ none", nil, "none"},
		{"Noop 值类型（main.go 生产装配形态）→ noop", NoopDLQPublisher{}, "noop"},
		{"Noop 指针类型 → noop", &NoopDLQPublisher{}, "noop"},
		{"真实 publisher → kafka", fakeRealDLQPublisher{}, "kafka"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := DLQKind(tc.in); got != tc.want {
				t.Errorf("DLQKind(%T) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
