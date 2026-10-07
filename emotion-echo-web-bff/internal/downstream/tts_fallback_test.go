// Package downstream — tts_fallback_test.go
//
// E2E-F-198（docs/plans/tts-api-cosyvoice2-f198-2026-10-06.md §D T4/T5）RED：
// 回退链与装配语义（plan §B.5）。
//
// 语义矩阵：
//   mode=auto + key 空   → 直接 local（dev 常态），不打降级日志
//   mode=auto + key 有   → cloud 优先；cloud 任何错误 → 降级 local + Warn(reason)
//   mode=cloud           → 只用 cloud，失败即失败（fail-loud，实测甄别用）
//   mode=local           → 只用 XTTS（回滚开关 / 离线环境）
//
// 降级必须可观测：Warn 带 reason + OnFallback 钩子（供 main 接 metrics 计数，
// 防"云端一直挂但没人知道"——E2E-22 静默失效同型）。
package downstream

import (
	"bytes"
	"context"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCloudProvider 可编程的假 cloud provider（记录调用次数，按脚本返回）。
type fakeCloudProvider struct {
	calls    atomic.Int32
	response func() (*XTTSPhonemesResp, error)
}

func (f *fakeCloudProvider) Synthesize(ctx context.Context, req TTSPhonemesReq) (*XTTSPhonemesResp, error) {
	f.calls.Add(1)
	return f.response()
}
func (f *fakeCloudProvider) Name() string { return "cloud" }

// fakeLocalProvider 假 local provider（记录调用次数，恒返回可辨别的响应）。
type fakeLocalProvider struct {
	calls atomic.Int32
}

func (f *fakeLocalProvider) Synthesize(ctx context.Context, req TTSPhonemesReq) (*XTTSPhonemesResp, error) {
	f.calls.Add(1)
	return &XTTSPhonemesResp{
		Audio:      "bG9jYWwtbWFya2Vy", // base64("local-marker")
		SampleRate: 24000,
		Text:       req.Text,
		Phonemes:   []XTTSPhoneme{{Char: "测", Start: 0, Duration: 0.5}},
		Duration:   0.5,
	}, nil
}
func (f *fakeLocalProvider) Name() string { return "local" }

// newCaptureLogger 返回写入 buffer 的 logger（断言降级日志）。
func newCaptureLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewTextHandler(&buf, nil)), &buf
}

func TestNewTTSProvider_AutoNoKey_UsesLocalDirectlyWithoutWarn(t *testing.T) {
	logger, buf := newCaptureLogger()
	cloud := &fakeCloudProvider{response: func() (*XTTSPhonemesResp, error) {
		t.Error("key 为空时不得调用 cloud provider")
		return nil, nil
	}}
	local := &fakeLocalProvider{}

	p := NewTTSProvider(TTSProviderConfig{
		Mode: "auto", APIKey: "", Cloud: cloud, Local: local, Logger: logger,
	})
	resp, err := p.Synthesize(context.Background(), TTSPhonemesReq{Text: "测"})
	require.NoError(t, err)
	assert.Equal(t, "bG9jYWwtbWFya2Vy", resp.Audio, "base64(local-marker)；key 空 → 直接走 local（不构造/不调用 cloud）")
	assert.NotContains(t, buf.String(), "degraded",
		"dev 无 key 是常态，不是异常——不得打降级 Warn（plan §B.5）")
}

func TestNewTTSProvider_AutoWithKey_CloudFirst(t *testing.T) {
	logger, buf := newCaptureLogger()
	cloud := &fakeCloudProvider{response: func() (*XTTSPhonemesResp, error) {
		return &XTTSPhonemesResp{Audio: "Y2xvdWQ=", SampleRate: 44100, Duration: 1.0,
			Phonemes: []XTTSPhoneme{{Char: "测", Start: 0, Duration: 1.0}}}, nil
	}}
	local := &fakeLocalProvider{}

	p := NewTTSProvider(TTSProviderConfig{
		Mode: "auto", APIKey: "k", Cloud: cloud, Local: local, Logger: logger,
	})
	resp, err := p.Synthesize(context.Background(), TTSPhonemesReq{Text: "测"})
	require.NoError(t, err)
	assert.Equal(t, 44100, resp.SampleRate, "cloud 成功 → 直接返回 cloud 响应")
	assert.EqualValues(t, 1, cloud.calls.Load())
	assert.EqualValues(t, 0, local.calls.Load(), "cloud 成功不得触碰 local")
	assert.NotContains(t, buf.String(), "degraded")
}

func TestNewTTSProvider_CloudError_FallsBackToLocalWithReason(t *testing.T) {
	logger, buf := newCaptureLogger()
	cloud := &fakeCloudProvider{response: func() (*XTTSPhonemesResp, error) {
		return nil, assert.AnError
	}}
	local := &fakeLocalProvider{}
	var hookReason atomic.Value
	onFallback := func(reason string) { hookReason.Store(reason) }

	p := NewTTSProvider(TTSProviderConfig{
		Mode: "auto", APIKey: "k", Cloud: cloud, Local: local,
		Logger: logger, OnFallback: onFallback,
	})
	resp, err := p.Synthesize(context.Background(), TTSPhonemesReq{Text: "测"})
	require.NoError(t, err, "cloud 失败必须降级成功（对前端不可见）")
	assert.Equal(t, "bG9jYWwtbWFya2Vy", resp.Audio, "base64(local-marker)")

	logText := buf.String()
	assert.Contains(t, logText, "degraded", "降级必须打 Warn（可观测硬要求）")
	assert.Contains(t, logText, assert.AnError.Error(), "Warn 必须带 reason")
	assert.Contains(t, hookReason.Load(), assert.AnError.Error(),
		"OnFallback 钩子必须收到 reason（main 侧接 metrics 计数）")
}

func TestNewTTSProvider_CloudConnRefused_FallsBack(t *testing.T) {
	// 连接层错误（等价超时/断网路径：http.Client.Do 返回 error）→ 降级
	cloud := &fakeCloudProvider{response: func() (*XTTSPhonemesResp, error) {
		return nil, context.DeadlineExceeded
	}}
	local := &fakeLocalProvider{}

	p := NewTTSProvider(TTSProviderConfig{
		Mode: "auto", APIKey: "k", Cloud: cloud, Local: local, Logger: newCaptureLoggerPtr(),
	})
	resp, err := p.Synthesize(context.Background(), TTSPhonemesReq{Text: "测"})
	require.NoError(t, err)
	assert.Equal(t, "bG9jYWwtbWFya2Vy", resp.Audio, "base64(local-marker)")
}

func TestNewTTSProvider_CloudMode_FailsLoudNoFallback(t *testing.T) {
	// mode=cloud：失败必须上抛（实测甄别/排障用），静默降级会掩盖问题
	logger, _ := newCaptureLogger()
	cloud := &fakeCloudProvider{response: func() (*XTTSPhonemesResp, error) {
		return nil, assert.AnError
	}}
	local := &fakeLocalProvider{}

	p := NewTTSProvider(TTSProviderConfig{
		Mode: "cloud", APIKey: "k", Cloud: cloud, Local: local, Logger: logger,
	})
	_, err := p.Synthesize(context.Background(), TTSPhonemesReq{Text: "测"})
	require.Error(t, err, "mode=cloud 失败不得降级")
	assert.EqualValues(t, 0, local.calls.Load(), "mode=cloud 不得触碰 local")
}

func TestNewTTSProvider_LocalMode_AlwaysLocal(t *testing.T) {
	// mode=local：回滚开关——有 key 也不用 cloud（离线环境 / cloud 故障根因定位）
	cloud := &fakeCloudProvider{response: func() (*XTTSPhonemesResp, error) {
		t.Error("mode=local 不得调用 cloud provider")
		return nil, nil
	}}
	local := &fakeLocalProvider{}

	p := NewTTSProvider(TTSProviderConfig{
		Mode: "local", APIKey: "k", Cloud: cloud, Local: local, Logger: newCaptureLoggerPtr(),
	})
	resp, err := p.Synthesize(context.Background(), TTSPhonemesReq{Text: "测"})
	require.NoError(t, err)
	assert.Equal(t, "bG9jYWwtbWFya2Vy", resp.Audio, "base64(local-marker)")
	assert.EqualValues(t, 1, local.calls.Load())
}

func TestNewTTSProvider_EmptyMode_DefaultsToAuto(t *testing.T) {
	// config 未设 Provider（空串）= auto（plan §C yaml 默认 auto）
	local := &fakeLocalProvider{}
	p := NewTTSProvider(TTSProviderConfig{
		Mode: "", APIKey: "", Cloud: nil, Local: local, Logger: newCaptureLoggerPtr(),
	})
	assert.Equal(t, "local", p.Name(), "空 mode + 空 key ⇒ local（Name 可直接断言）")
}

// newCaptureLoggerPtr 便捷包装（不关心 buffer 内容时用）。
func newCaptureLoggerPtr() *slog.Logger {
	l, _ := newCaptureLogger()
	return l
}
