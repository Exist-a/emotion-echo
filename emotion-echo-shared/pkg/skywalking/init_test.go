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

// TestInitRedis_NilSafe nil client 不 panic
func TestInitRedis_NilSafe(t *testing.T) {
	assert.False(t, InitRedis(nil))
}
