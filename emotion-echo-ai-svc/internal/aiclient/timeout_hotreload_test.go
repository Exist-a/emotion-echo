package aiclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E-23 测试点 #31（D-32）：三个 aiclient 的超时可热更。
//
// 🔴 形态：`NewFERClient` / `NewSenseVoiceClient` / `NewXTTSClient` 都把
// `c.Timeout` **在构造期拷贝**进 struct 的 `timeout` 字段，请求路径用
// `context.WithTimeout(ctx, c.timeout)`。构造之后改配置对它没有任何影响
// ⇒ 与 F-159（LLM.Timeout 没传）同型的"配了不生效"，只是这次是"传了但冻结"。
//
// 修法：加 `TimeoutFn func() time.Duration` 钩子；非 nil 时**每请求**调它取
// 当前值，nil 时退回构造期值（向后兼容，既有调用方不受影响）。
//
// 用函数钩子而非直接依赖 ops 包：aiclient 是被 logic 层依赖的底层，
// 让它反向 import ops 会形成不必要的耦合；钩子把"值从哪来"留给装配层。

func TestFERClient_TimeoutFnOverridesFrozenValue(t *testing.T) {
	t.Parallel()

	var cur atomic.Int64
	cur.Store(int64(10 * time.Second))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 故意慢：若超时真的变成 1s 以下，请求会被取消
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewFERClient(Config{BaseURL: srv.URL, Timeout: 10})
	require.NotNil(t, c)
	c.TimeoutFn = func() time.Duration { return time.Duration(cur.Load()) }

	// 基线：10s 超时，请求应成功
	_, err := c.AnalyzeImage(context.Background(), []byte("x"), "a.jpg")
	assert.NoError(t, err, "10s 超时下应成功")

	// 热更到极短：下一次请求必须用新值（200ms 服务端 + 1ms 超时 ⇒ 超时）
	cur.Store(int64(1 * time.Millisecond))
	_, err = c.AnalyzeImage(context.Background(), []byte("x"), "a.jpg")
	assert.Error(t, err, "热更到 1ms 后请求必须超时 —— 证明读到的是新值而非构造期拷贝")

	// 再热更回长值 ⇒ 恢复成功（证明不是一次性生效）
	cur.Store(int64(5 * time.Second))
	_, err = c.AnalyzeImage(context.Background(), []byte("x"), "a.jpg")
	assert.NoError(t, err, "热更回长超时后应恢复成功")
}

func TestSenseVoiceClient_TimeoutFnOverridesFrozenValue(t *testing.T) {
	t.Parallel()

	var cur atomic.Int64
	cur.Store(int64(30 * time.Second))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"text":"hi"}`))
	}))
	defer srv.Close()

	c := NewSenseVoiceClient(Config{BaseURL: srv.URL, Timeout: 30})
	require.NotNil(t, c)
	c.TimeoutFn = func() time.Duration { return time.Duration(cur.Load()) }

	cur.Store(int64(1 * time.Millisecond))
	_, err := c.Analyze(context.Background(), []byte("x"), "a.wav")
	assert.Error(t, err, "热更到 1ms 后必须超时")

	cur.Store(int64(5 * time.Second))
	_, err = c.Analyze(context.Background(), []byte("x"), "a.wav")
	assert.NoError(t, err, "热更回长超时后应恢复")
}

func TestXTTSClient_TimeoutFnOverridesFrozenValue(t *testing.T) {
	t.Parallel()

	var cur atomic.Int64
	cur.Store(int64(60 * time.Second))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"audio":"AAECAw==","sample_rate":24000}`))
	}))
	defer srv.Close()

	c := NewXTTSClient(Config{BaseURL: srv.URL, Timeout: 60}, "zh-cn", 0.75)
	require.NotNil(t, c)
	c.TimeoutFn = func() time.Duration { return time.Duration(cur.Load()) }

	cur.Store(int64(1 * time.Millisecond))
	_, _, err := c.Synthesize(context.Background(), "你好")
	assert.Error(t, err, "热更到 1ms 后必须超时")

	cur.Store(int64(5 * time.Second))
	_, _, err = c.Synthesize(context.Background(), "你好")
	assert.NoError(t, err, "热更回长超时后应恢复")
}

// TestClients_TimeoutFnNil_KeepsFrozenValue 向后兼容：未装钩子时行为不变。
func TestClients_TimeoutFnNil_KeepsFrozenValue(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	f := NewFERClient(Config{BaseURL: srv.URL, Timeout: 10})
	require.NotNil(t, f)
	assert.Equal(t, 10*time.Second, f.effectiveTimeout(),
		"未装 TimeoutFn 时应返回构造期值")

	s := NewSenseVoiceClient(Config{BaseURL: srv.URL, Timeout: 30})
	require.NotNil(t, s)
	assert.Equal(t, 30*time.Second, s.effectiveTimeout())

	x := NewXTTSClient(Config{BaseURL: srv.URL, Timeout: 60}, "zh-cn", 0.75)
	require.NotNil(t, x)
	assert.Equal(t, 60*time.Second, x.effectiveTimeout())
}
