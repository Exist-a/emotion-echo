// Package downstream — ai_grpc_deadline_test.go
//
// E2E-F-115（2026-09-22 用户浏览器实测）：dev 模式下首次语音 → /voice/upload → BFF→ai-svc
// 5s DeadlineExceeded。修法：ai_grpc.go 的 3 个 RPC 必须带 30s deadline（与 HTTP 路径
// 默认 timeout 对齐）—— 防 SenseVoice 冷启动转写（ffmpeg + 首次张量分配）撞死限。
//
// 测法：bufconn + fakeEmotionQuerySrv 记录 ctx.Deadline() → 断言 deadline 距 now 在
// [25s, 35s] 区间（即 30s ± 抖动）。
package downstream

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAIGRPCClient_MultiModalAnalyze_Has30sDeadline 断言 BFF→ai-svc gRPC 调用带 30s deadline。
//
// E2E-F-115：原 ai_grpc.go:57 cli.MultiModalAnalyze(withUserID(ctx), grpcReq) 不设 deadline，
// gRPC 客户端 default deadline 通常无（几十分钟），看似不撞死限；但 ai-svc 内部有自身的
// 5s server-side timeout ⇒ 首请求冷启动仍 5s 内被 ai-svc 拒掉（返 DeadlineExceeded）。
// 修法：BFF 客户端主动设 30s deadline，告知 server 给足冷启动窗口。
func TestAIGRPCClient_MultiModalAnalyze_Has30sDeadline(t *testing.T) {
	conn, fake := startFakeGRPCServerWithSrv(t)
	c := NewAIGRPCClient(conn)
	require.NotNil(t, c, "GRPCConn 非 nil 必须返非 nil client")

	before := time.Now()
	_, err := c.MultiModalAnalyze(context.Background(), MultiModalAnalyzeReq{
		Kind:     "image",
		File:     strings.NewReader("fake-image-bytes"),
		FileName: "test.jpg",
	})
	require.NoError(t, err)

	fake.lastMultiModalMu.Lock()
	dl := fake.lastMultiModalDeadline
	fake.lastMultiModalMu.Unlock()

	require.False(t, dl.IsZero(),
		"ctx 必须带 deadline（IsZero=true 表示未设 = 修法未落地，E2E-F-115 未修复）")

	delta := dl.Sub(before)
	assert.GreaterOrEqual(t, delta, 25*time.Second,
		"deadline 距 now ≥25s（30s - 5s 抖动容差）；实际 = %v", delta)
	assert.LessOrEqual(t, delta, 35*time.Second,
		"deadline 距 now ≤35s（30s + 5s 抖动容差）；实际 = %v", delta)
}

// TestAIGRPCClient_SynthesizeSpeech_Has30sDeadline 同上，钉 TTS RPC 也带 30s deadline。
//
// XTTS 冷启动 ≈ 3~8s（首次模型加载），原无 deadline 风险稍低，但与其他 RPC 保持一致
// 避免特殊化导致后续回归。
func TestAIGRPCClient_SynthesizeSpeech_Has30sDeadline(t *testing.T) {
	conn, fake := startFakeGRPCServerWithSrv(t)
	c := NewAIGRPCClient(conn)

	before := time.Now()
	_, err := c.SynthesizeSpeech(context.Background(), SynthesizeSpeechReq{Text: "hi"})
	require.NoError(t, err)

	fake.lastMultiModalMu.Lock()
	dl := fake.lastMultiModalDeadline
	fake.lastMultiModalMu.Unlock()

	require.False(t, dl.IsZero(), "ctx 必须带 deadline")
	delta := dl.Sub(before)
	assert.GreaterOrEqual(t, delta, 25*time.Second)
	assert.LessOrEqual(t, delta, 35*time.Second)
}
