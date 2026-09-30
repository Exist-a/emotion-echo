// Package discovery — Nacos 续约可观测性回归钉（E2E-23 F-107 / F-156）
//
// 这组测试锁的是**账本 F-107 / F-156 记的三处观测盲区**：
//
//	① `nacos_beat.go` 的 `if failCount <= 3` —— 连续失败**第 4 次起连 WARN 都不打**。
//	   而 BFF 的 HTTP beat 在 Nacos 3.x 下**实测 100% 失败**（`beat HTTP 501: no such api`
//	   在日志里出现过 ×9），即这条通道是**恒死**的。恒死 + 3 次后静默
//	   = 运维看板上永远看不到"这条通道已经废了一年"。
//	② `nacos_register.go` 的 `Heartbeat` watcher 里 `_, _ = r.client.UpdateInstance(...)`
//	   —— 续约错误被**完全丢弃**。SDK 真死透时无人知晓。
//	③ 由此产生的"观测盲区"：既没有"失败了"的可观测信号，也没有"它已长期不可用"的结论。
//
// 为什么用行为测试而不是源码扫描：本包已有源码级契约测试的先例
// （见 nacos_register_host_test.go，理由是 INamingClient 有 20+ 方法）。
// 但那处的 bug 本质是"调用点漏用 helper"，而这里的 bug 本质是**运行时有没有打日志**
// —— 只有真跑 goroutine、接真日志 sink 才测得到。
// INamingClient 用「嵌入接口 + 只覆写用到的方法」解决：
// 未覆写的方法一被调用就 panic，这正是测试想要的安全网。
package discovery

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	nacosnaming "github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	nacosvo "github.com/nacos-group/nacos-sdk-go/v2/vo"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------- 日志捕获 sink

// captureHandler 把 slog 记录收进内存，供断言用。
type captureHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *captureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *captureHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *captureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *captureHandler) WithGroup(string) slog.Handler      { return h }

// countAtLeast 返回「级别 >= want 且消息包含 substr」的记录条数。
func (h *captureHandler) countAtLeast(want slog.Level, substr string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, r := range h.records {
		if r.Level >= want && contains(r.Message, substr) {
			n++
		}
	}
	return n
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// installCapture 把 slog 默认 logger 换成捕获 sink，返回恢复函数与 handler。
func installCapture(t *testing.T) (*captureHandler, func()) {
	t.Helper()
	h := &captureHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	return h, func() { slog.SetDefault(prev) }
}

// ---------------------------------------------------------------- SDK fake

// fakeNamingClient 嵌入 INamingClient 接口：只覆写本测试用到的方法，
// 其余方法一旦被调用即 panic（测试即网）。
type fakeNamingClient struct {
	nacosnaming.INamingClient
	updateErr   error
	updateCalls int
	mu          sync.Mutex
}

func (f *fakeNamingClient) UpdateInstance(nacosvo.UpdateInstanceParam) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.updateCalls++
	if f.updateErr != nil {
		return false, f.updateErr
	}
	return true, nil
}

func (f *fakeNamingClient) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.updateCalls
}

// ---------------------------------------------------------------- ① beat 失败不可永久静默

// TestBeatHeartbeat_KeepsReportingAfterThirdFailure：
// 服务端恒返 501（复刻 Nacos 3.x 的 `no such api`），跑够多次心跳后，
// **第 4 次及以后的失败仍然必须有日志**。
//
// 修复前：`if failCount <= 3` ⇒ 第 4 次起一条不打 ⇒ 断言失败。
func TestBeatHeartbeat_KeepsReportingAfterThirdFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Nacos 3.x 的真实响应：端点已不存在
		w.WriteHeader(http.StatusNotImplemented)
		_, _ = w.Write([]byte(`{"timestamp":"","status":501,"error":"Not Implemented","message":"no such api:POST:/nacos/v1/ns/instance/beat"}`))
	}))
	defer srv.Close()

	fake := &fakeNamingClient{updateErr: errors.New("nacos unreachable")}
	reg := &NacosRegistry{
		cfg:    NacosConfig{ServerAddr: srv.URL, Namespace: "ns-test", GroupName: "DEFAULT_GROUP"},
		client: fake,
	}

	h, restore := installCapture(t)
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	reg.BeatHeartbeat(ctx, Instance{ServiceName: "svc-x", Host: "127.0.0.1", Port: 9999}, 10*time.Millisecond)

	// 跑够 ~50 次心跳（间隔 10ms ⇒ 实际 500ms，远低于本测试 5s 护栏）。
	// 必须跨过多个节流点：renewalFailureLogEvery=12 ⇒ 日志落在第 1/12/24/36/48 次。
	time.Sleep(500 * time.Millisecond)
	cancel()
	time.Sleep(30 * time.Millisecond) // 让最后一次 tick 落地

	warns := h.countAtLeast(slog.LevelWarn, "BeatInstance failed")
	calls := fake.calls()
	assert.GreaterOrEqual(t, calls, 40,
		"兜底 UpdateInstance 应被持续调用（这是本项目实际的心跳通道）")
	assert.Greater(t, warns, 3,
		"连续失败的第 4 次起仍必须有日志。修复前 `if failCount <= 3` 使第 4 次起**永久静默**，"+
			"而该通道在 Nacos 3.x 下恒 501 ⇒ 运维永远看不到它已经死了。"+
			"实测 %d 次失败只打了 %d 条 WARN", calls, warns)
}

// TestShouldLogConsecutiveFailure_NeverPermanentlySilent：
// 直接锁住"永不永久静默"这条性质 —— 任意长度的时间窗内都必须有日志，
// 而不是"头几条之后就没了"。
//
// 这是上面那条行为测试的**性质化**版本：行为测试受 tick 数与节流常数影响，
// 换句话说它只能证明"这一次跨过了节流点"；本测试证明的是"永远会跨过"。
func TestShouldLogConsecutiveFailure_NeverPermanentlySilent(t *testing.T) {
	const every = renewalFailureLogEvery

	// 第 1 次必打
	assert.True(t, shouldLogConsecutiveFailure(1, every), "第 1 次失败必须打日志")

	// 性质：不存在长度 >= every 的连续失败窗口是完全静默的
	const horizon = 1000
	window := 0
	for n := 1; n <= horizon; n++ {
		if shouldLogConsecutiveFailure(n, every) {
			window = 0
		} else {
			window++
		}
		require.Less(t, window, every,
			"出现了长度 %d 的完全静默窗口（n=%d）—— 违反\"永不永久静默\"", window, n)
	}

	// 边界：n<=0 不打；every<=0 视为 1（每次都打）
	assert.False(t, shouldLogConsecutiveFailure(0, every), "n=0 不是失败")
	assert.False(t, shouldLogConsecutiveFailure(-1, every), "n<0 不是失败")
	for n := 1; n <= 5; n++ {
		assert.True(t, shouldLogConsecutiveFailure(n, 0), "every<=0 应退化为每次都打（n=%d）", n)
	}
}

// TestBeatHeartbeat_AnnouncesLongTermDeadChannel：
// 恒失败足够多次后，必须出现一条**说清楚后果**的日志
// （不是重复的 warn，而是"这条通道已长期不可用、功能由 SDK UpdateInstance 兜底"的结论）。
func TestBeatHeartbeat_AnnouncesLongTermDeadChannel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
	}))
	defer srv.Close()

	fake := &fakeNamingClient{updateErr: errors.New("nacos unreachable")}
	reg := &NacosRegistry{
		cfg:    NacosConfig{ServerAddr: srv.URL, Namespace: "ns-test", GroupName: "DEFAULT_GROUP"},
		client: fake,
	}

	h, restore := installCapture(t)
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	// 间隔 1ms，300ms ⇒ 约 300 次心跳，足够越过"长期不可用"阈值
	reg.BeatHeartbeat(ctx, Instance{ServiceName: "svc-y", Host: "127.0.0.1", Port: 9998}, time.Millisecond)
	time.Sleep(300 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)

	assert.GreaterOrEqual(t, h.countAtLeast(slog.LevelWarn, "unavailable"),
		1,
		"必须有一条明确告知「beat 通道长期不可用、功能已由 SDK UpdateInstance 兜底」的日志 —— "+
			"否则运维只能看到一堆重复 warn，无法得出「这条路已经废了」的结论")
}

// ---------------------------------------------------------------- ② watcher 续约错误不可被吞

// TestHeartbeat_LogsRenewalFailure：SDK 续约恒失败时必须打日志。
//
// 修复前：Heartbeat 里 `_, _ = r.client.UpdateInstance(...)` 丢弃了 error，
//
//	整条链路零日志 —— 断言失败。
func TestHeartbeat_LogsRenewalFailure(t *testing.T) {
	fake := &fakeNamingClient{updateErr: errors.New("dial tcp 10.0.0.1:8848: connect: connection refused")}
	reg := &NacosRegistry{
		cfg:    NacosConfig{ServerAddr: "127.0.0.1:8848", Namespace: "ns-test", GroupName: "DEFAULT_GROUP"},
		client: fake,
	}

	h, restore := installCapture(t)
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	reg.Heartbeat(ctx, Instance{ServiceName: "svc-z", Host: "127.0.0.1", Port: 9997}, 10*time.Millisecond)
	time.Sleep(150 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)

	require.GreaterOrEqual(t, fake.calls(), 5, "watcher 应周期性发起续约")
	assert.GreaterOrEqual(t, h.countAtLeast(slog.LevelWarn, "renew"),
		1,
		"续约失败必须打 WARN。修复前 `_, _ = r.client.UpdateInstance(...)` 把 error 完全丢弃，"+
			"SDK 死透时零日志 —— 账本 F-156 记的\"观测盲区\"即指此处。")
}

// TestHeartbeat_LogsRecovery：失败若干次后恢复，应打一条 INFO 说明已恢复，
// 否则运维无法判断"现在到底好了没有"。
func TestHeartbeat_LogsRecovery(t *testing.T) {
	fake := &fakeNamingClient{updateErr: errors.New("temporary outage")}
	reg := &NacosRegistry{
		cfg:    NacosConfig{ServerAddr: "127.0.0.1:8848", Namespace: "ns-test", GroupName: "DEFAULT_GROUP"},
		client: fake,
	}

	h, restore := installCapture(t)
	defer restore()

	ctx, cancel := context.WithCancel(context.Background())
	reg.Heartbeat(ctx, Instance{ServiceName: "svc-r", Host: "127.0.0.1", Port: 9996}, 10*time.Millisecond)
	time.Sleep(80 * time.Millisecond) // 先失败几次
	fake.mu.Lock()
	fake.updateErr = nil // 恢复
	fake.mu.Unlock()
	time.Sleep(120 * time.Millisecond)
	cancel()
	time.Sleep(20 * time.Millisecond)

	assert.GreaterOrEqual(t, h.countAtLeast(slog.LevelInfo, "renew"),
		1,
		"续约恢复后必须有一条 INFO 记录 —— 只有 warn 没有 recovery，运维无法判断当前是否已恢复")
}
