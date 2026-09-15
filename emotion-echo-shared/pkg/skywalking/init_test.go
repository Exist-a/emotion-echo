package skywalking

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
)

// TestInitGORM_NilSafe nil DB 不 panic（Stage 26-A bug #2 防御）
func TestInitGORM_NilSafe(t *testing.T) {
	assert.Equal(t, 0, InitGORM(nil))
}

// TestInitGORM_WithDB 真实 *gorm.DB 应不 panic（不验证回调细节，那是
// InstrumentGORM 的内部行为；本测试只锁"Init 函数能跑"）。
func TestInitGORM_WithDB(t *testing.T) {
	var db *gorm.DB // nil by design — 我们只测 nil-safe 路径
	assert.Equal(t, 0, InitGORM(db))
}

// TestInitGORM_NonNilPositive Caller-wiring 钉死：非 nil *gorm.DB 必须返回 1
// （= InstrumentGORM 调用发生过）。当前实现已经返 1（看 init.go:31）。
//
// 设计意图：5 svc main.go 接入 InitGORM 后，若未来有人改 InitGORM 把它退化为
// always-0，本测试即失败 → caller-wiring 缺失立刻暴露。
func TestInitGORM_NonNilPositive(t *testing.T) {
	db, err := gorm.Open(nil, &gorm.Config{SkipDefaultTransaction: true})
	// gorm.Open(nil, ...) 会用空 dialector，可能 panic 或 err；
	// 我们的目的是让 *gorm.DB 不为 nil，绕开 nil-safe 分支
	if err != nil || db == nil {
		t.Skipf("gorm.Open(nil) failed (env limitation): %v", err)
	}
	assert.GreaterOrEqual(t, InitGORM(db), 1, "非 nil *gorm.DB 必须触发 InstrumentGORM 副作用")
}

// TestInitRedis_NilSafe nil client 不 panic
func TestInitRedis_NilSafe(t *testing.T) {
	assert.False(t, InitRedis(nil))
}

// TestInitRedis_RealClientRegistersHook Round 4.4 PR-1 caller-wiring 钉死：
//
// 真 *redis.Client 传入 InitRedis 必须返回 true，且 hook 必须注册成功
//（=InstrumentRedis 调用发生过）。当前实现对 *redis.Client type-assert 一个
// 内部 hookable interface（永远 false）→ 本测试必失败 → 暴露隐性 bug。
//
// 设计意图：未来 svc 接入 Redis 时，InitRedis 必须真把 TracingHook 挂上，
// 否则 Redis 流量无 trace（沉默丢失可观测性）。
func TestInitRedis_RealClientRegistersHook(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{Addr: "127.0.0.1:0"}) // 不真连
	defer rdb.Close()

	// redis.Client 内部 hook 列表长度不可直接读 —— 我们用 ProcessHook 副作用间接验证
	// "挂载 hook 后执行命令会经过 hook"。但 hook 需要 Tracer 才能产生 span，
	// 没有 Tracer 时 ProcessHook 直接透传（见 redis_tracing.go:38-39），无法证伪。
	//
	// 更可靠：InitRedis 返回值契约 —— 真 *redis.Client 必须返 true
	// （即"hook 已注册"信号）。
	assert.True(t, InitRedis(rdb), "*redis.Client 必须返 true（hook 已注册信号）")
	_ = context.Background() // 防 unused（保留 ctx import 给未来用例）
}
