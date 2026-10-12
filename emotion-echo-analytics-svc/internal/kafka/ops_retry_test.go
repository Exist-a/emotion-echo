package kafka

import (
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
)

// E2E-23 测试点 #32（D-32）：analytics-svc 的 `Kafka.MaxRetries` 可热更。
//
// 🔴 为什么需要（账本 F-160 的下半段）：P3 已把 `SetDefaults` 的默认值补成 3，
// 但消费判定读的是 `chatEventHandler.maxRetries` —— 那是 `WithMaxRetries`
// 在**构造期拷贝**进去的普通 int 字段。Nacos 推新值若只改它，判定逻辑
// 读到的仍是旧值 ⇒ 又一个"配了不生效"。
//
// 修法：判定改为读 atomic 容器（Nacos 回调写 / 消费循环读）。
// 用 atomic 而非 RWMutex —— 判定在每条消息的热路径上，加锁代价与
// 漏配 RLock 的风险都不划算。
//
// 本测试锁定：更新后判定必须立刻读到新值（不是构造期的拷贝）。

func TestConsumer_UpdateMaxRetries_AffectsHandler(t *testing.T) {
	t.Parallel()

	// 直接构造 consumer+handler 的共享组合（NewConsumer 会真连 broker，
	// 不适合单测；本测试要验的是"两侧共享同一个容器"这一结构性质）。
	c, h := newSharedRetries(3)

	assert.Equal(t, 3, c.currentMaxRetries(), "初始值")
	assert.Equal(t, 3, h.currentMaxRetries(), "handler 初始读到同一值")

	c.WithMaxRetries(7)
	assert.Equal(t, 7, c.currentMaxRetries(), "WithMaxRetries 应立即生效")
	assert.Equal(t, 7, h.currentMaxRetries(), "handler 同步可见")

	// Nacos 回调路径（构造之后）更新 —— 这才是热更场景
	c.UpdateMaxRetries(11)
	assert.Equal(t, 11, c.currentMaxRetries(),
		"运行期更新必须对消费判定可见，否则热更无效")
	assert.Equal(t, 11, h.currentMaxRetries(),
		"handler 侧也必须读到新值 —— 两处若是各存一份，热更只改到其中之一")

	c.UpdateMaxRetries(2)
	assert.Equal(t, 2, h.currentMaxRetries(), "再次更新后 handler 仍须同步")
}

// TestConsumer_UpdateMaxRetries_IgnoresNonPositive 非正值必须被忽略。
func TestConsumer_UpdateMaxRetries_IgnoresNonPositive(t *testing.T) {
	t.Parallel()

	c, _ := newSharedRetries(3)
	c.WithMaxRetries(5)

	for _, bad := range []int{0, -1} {
		c.UpdateMaxRetries(bad)
		assert.Equal(t, 5, c.currentMaxRetries(),
			"非正值必须被忽略（0 会变成'永不重试'，与 main.go 的 >0 守卫语义一致）")
	}
}

// TestConsumer_WithMaxRetries_AlsoUpdatesSharedHolder 保留既有 builder 语义。
func TestConsumer_WithMaxRetries_AlsoUpdatesSharedHolder(t *testing.T) {
	t.Parallel()

	c, h := newSharedRetries(3)
	c.WithMaxRetries(9)

	assert.Equal(t, 9, h.currentMaxRetries(),
		"WithMaxRetries 后 handler 与 consumer 应一致")
}

// TestConsumer_WithMaxRetries_NilReceiver_DoesNotPanic 空 receiver 必须安全。
//
// 🔴 为什么需要（账本 F-216）：main.go 的装配顺序是
//
//	kc, err := kafka.NewConsumer(...)   // broker 不可达 ⇒ (nil, err)
//	if c.Kafka.MaxRetries > 0 {
//	    kc.WithMaxRetries(c.Kafka.MaxRetries)  // ← 在任何 err 检查之前
//	}
//	if err != nil { log.Printf("consumer init failed ...") }
//
// Kafka 未就绪而 analytics-svc 先起时（容器冷启动竞态）NewConsumer 返回
// nil，`nil.WithMaxRetries(...)` 直接 SIGSEGV ⇒ 容器 crash-loop ⇒
// `docker compose up` 整条依赖链以 EXIT=1 中止。
// nil 在本代码库是**受支持的语义**（main.go: "Kafka 未启用时保持 nil，
// BootNacos 的 applyOps 会退化为只记录不应用"）——builder 必须同样 nil-safe。
func TestConsumer_WithMaxRetries_NilReceiver_DoesNotPanic(t *testing.T) {
	t.Parallel()

	var c *Consumer
	assert.NotPanics(t, func() { c.WithMaxRetries(3) },
		"NewConsumer 失败返回 nil 时 main.go 仍会调 WithMaxRetries —— 必须 nil-safe，"+
			"否则 Kafka 未就绪会把瞬时依赖延迟放大成永久 crash-loop")
	assert.Nil(t, c.WithMaxRetries(3), "nil receiver 原样返回 nil")
}

// newSharedRetries 构造共享同一 atomic 容器的 consumer + handler，
// 复现生产装配（Consumer.retries 与 chatEventHandler.retries 同一实例）。
func newSharedRetries(initial int32) (*Consumer, *chatEventHandler) {
	var retries atomic.Int32
	retries.Store(initial)
	h := &chatEventHandler{retries: &retries}
	c := &Consumer{retries: &retries, consumer: h}
	return c, h
}
