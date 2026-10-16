// Package events — publisher_unavailable.go
//
// CL-03 Phase 5（CL-D-19=C / CL-F-15）：Kafka publisher 未就绪时的**诚实语义**。
//
// 修复前的行为（B 轨 2026-10-14 起栈实测 CL-F-15）：producer 初始化失败 ⇒
// `pub = InMemoryEventPublisher` ⇒ relay 每轮把 outbox 行标 `sent`，而事件**只进进程内存**：
// `chat-events` topic 里没有该事件、消费端也没收到，"outbox 全 sent"成了假绿信号，
// 唯一暴露它的是一个 counter。链路无声失效。
//
// 修复后的行为：
//   - 初始化失败 ⇒ 装 `UnavailablePublisher`（Publish 一律返回 `ErrPublisherUnavailable`）
//     ⇒ relay **不 MarkSent**（行留 pending），也**不累加 attempts**（避免长时间故障把所有行
//     推成 dead —— dead 行不再被扫描，等于另一种丢）
//   - 同时启动 `StartKafkaPublisherBackgroundRetry`（指数退避）⇒ 成功后由调用方把真 publisher
//     热替换进 `PublisherHolder` ⇒ relay 下一轮自然把 pending 行发出去
package events

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ErrPublisherUnavailable 表示「Kafka publisher 尚未就绪」——
// 与「发布真的失败」是**两种不同语义**：前者应留 pending 等重试，后者才走 attempts/dead 状态机。
var ErrPublisherUnavailable = errors.New("events: kafka publisher unavailable (init failed; background retry in progress)")

// UnavailablePublisher 是"未就绪"占位 publisher：Publish 一律报错，Close 无副作用。
//
// 为什么不用 InMemoryEventPublisher 占位：它返回 nil（成功语义）⇒ relay 会 MarkSent
// ⇒ 就是 CL-F-15 那个黑洞。
type UnavailablePublisher struct{ reason error }

// NewUnavailablePublisher 构造；reason 为 nil 时退化为哨兵错误本身。
func NewUnavailablePublisher(reason error) *UnavailablePublisher {
	if reason == nil {
		reason = ErrPublisherUnavailable
	}
	return &UnavailablePublisher{reason: reason}
}

// Publish 一律返回可被 errors.Is(ErrPublisherUnavailable) 识别的错误（原始原因保留在文本里）。
func (p *UnavailablePublisher) Publish(context.Context, string, *Event) error {
	return fmt.Errorf("%w: %v", ErrPublisherUnavailable, p.reason)
}

// Close 无副作用。
func (p *UnavailablePublisher) Close() error { return nil }

// publisherBox 让 `atomic.Value` 的**具体类型恒定**（恒为 `*publisherBox`）。
//
// 为什么不能直接存接口值：`atomic.Value` 要求同一个 Value 始终存同一具体类型，而生产装配
// 会在 `*UnavailablePublisher`（init 失败）→ `*KafkaEventPublisher`（后台重试成功热替换）
// 之间换类型 ⇒ 直接存会 **panic: sync/atomic: store of inconsistently typed value into Value**
// （起栈实测 2026-10-16 14:02:56：Kafka 恢复瞬间 chat-svc panic 退出、容器重启）。
type publisherBox struct{ p EventPublisher }

// PublisherHolder 线程安全地持有"当前 publisher"，供后台重试成功后**热替换**。
//
// 为什么需要它：relay 与后台重试 goroutine 并发读写同一个 publisher ⇒ 直接改字段是
// data race（E2E-21 教训：单测全绿也照样是 bug）。用 atomic.Value 做无锁读写。
type PublisherHolder struct{ v atomic.Value }

// NewPublisherHolder 构造；p 为 nil 时 holder 处于"未设置"状态（Get 返回 nil）。
func NewPublisherHolder(p EventPublisher) *PublisherHolder {
	h := &PublisherHolder{}
	if p != nil {
		h.Set(p)
	}
	return h
}

// Get 返回当前 publisher；未设置时为 nil（调用方按"未就绪"处理）。
func (h *PublisherHolder) Get() EventPublisher {
	if v := h.v.Load(); v != nil {
		if b, ok := v.(*publisherBox); ok && b != nil {
			return b.p
		}
	}
	return nil
}

// Set 安装 publisher（nil 忽略，避免把 holder 打回未设置）。
//
// 存的是 `*publisherBox`（类型恒定）而非接口值本身 —— 见 publisherBox 的说明：
// 直接存接口值在"换具体类型"时会 panic。
func (h *PublisherHolder) Set(p EventPublisher) {
	if p != nil {
		h.v.Store(&publisherBox{p: p})
	}
}

// KafkaPublisherRetryOptions 控制后台重试节奏与回调。
type KafkaPublisherRetryOptions struct {
	// InitialInterval 首次重试前的等待（默认 5s）
	InitialInterval time.Duration
	// MaxInterval 退避上限（默认 60s）
	MaxInterval time.Duration
	// Try 尝试构造 publisher。nil 时本函数直接返回 no-op stop。
	// 生产 = NewKafkaEventPublisher 的闭包；单测注入假实现以获得确定性。
	Try func() (*KafkaEventPublisher, error)
	// OnSuccess 成功构造后回调（生产在这里挂 tracer + 写进 PublisherHolder）。
	OnSuccess func(*KafkaEventPublisher)
	// Logf 日志（nil = 静默）
	Logf func(format string, args ...any)
}

// StartKafkaPublisherBackgroundRetry 后台指数退避重试构造 Kafka publisher，成功后回调并退出。
//
// 与 CL-F-14 的 tracer 后台补挂同构（svc 先于 Kafka/OAP 起来时不放弃自愈）。
// 返回的 stop 幂等且会等 goroutine 退出（测试可安全调用）。
func StartKafkaPublisherBackgroundRetry(ctx context.Context, opts KafkaPublisherRetryOptions) func() {
	if opts.Try == nil {
		return func() {}
	}
	interval := opts.InitialInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	maxInterval := opts.MaxInterval
	if maxInterval <= 0 {
		maxInterval = 60 * time.Second
	}
	logf := opts.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}

	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	go func() {
		defer close(done)
		attempt := 0
		for {
			attempt++
			kp, err := opts.Try()
			if err == nil && kp != nil {
				logf("[kafka] producer 后台重试成功（第 %d 次）—— outbox relay 自此刻起恢复真实投递", attempt)
				if opts.OnSuccess != nil {
					opts.OnSuccess(kp)
				}
				return
			}
			logf("[kafka] producer 后台重试第 %d 次失败（backoff=%s）：%v", attempt, interval, err)
			select {
			case <-runCtx.Done():
				return
			case <-time.After(interval):
			}
			interval *= 2
			if interval > maxInterval {
				interval = maxInterval
			}
		}
	}()

	var once sync.Once
	return func() {
		once.Do(func() {
			cancel()
			<-done
		})
	}
}
