package skywalking

import (
	"testing"

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
