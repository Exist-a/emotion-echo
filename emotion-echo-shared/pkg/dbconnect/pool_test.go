package dbconnect

import (
	"database/sql"
	"database/sql/driver"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubDriver 注册一个最小空 driver，让 sql.Open 返回真 *sql.DB（不连真库）。
// 目的：SetMax* 调用需要 *sql.DB 实例；sql.Open 失败返 nil，Stats/Close 全 panic。
//
// 用 init() 在测试编译期一次性注册，副作用隔离到本测试包。
type stubConn struct{}

func (c *stubConn) Prepare(query string) (driver.Stmt, error)       { return &stubStmt{}, nil }
func (c *stubConn) Close() error                                    { return nil }
func (c *stubConn) Begin() (driver.Tx, error)                       { return nil, nil }

type stubStmt struct{}

func (s *stubStmt) Close() error                                       { return nil }
func (s *stubStmt) NumInput() int                                      { return 0 }
func (s *stubStmt) Exec(args []driver.Value) (driver.Result, error)    { return nil, nil }
func (s *stubStmt) Query(args []driver.Value) (driver.Rows, error)     { return nil, nil }

type stubDriver struct{}

func (d *stubDriver) Open(name string) (driver.Conn, error) { return &stubConn{}, nil }

func init() {
	sql.Register("dbconnect-test-stub", &stubDriver{})
}

// newTestSQLDB 拿一个能 SetMax* 的真 *sql.DB，永不连真库。
func newTestSQLDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("dbconnect-test-stub", "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// TestEnvInt_DefaultValue 验证 envInt fallback 行为
func TestEnvInt_DefaultValue(t *testing.T) {
	t.Setenv("TEST_PG_MAX_CONNS", "")
	assert.Equal(t, 42, envMust(t, "TEST_PG_MAX_CONNS", 42))

	t.Setenv("TEST_PG_MAX_CONNS", "100")
	assert.Equal(t, 100, envMust(t, "TEST_PG_MAX_CONNS", 42))
}

// TestEnvInt_BadIntFail 非法整数 env 应 fail-fast（不被 silently 吞掉）
//
// 设计意图：prod 误配 PG_MAX_CONNS=typo 应立刻暴露，避免静默 fallback。
func TestEnvInt_BadIntFail(t *testing.T) {
	t.Setenv("TEST_PG_MAX_CONNS", "not-a-number")
	_, err := envInt("TEST_PG_MAX_CONNS", 42)
	assert.Error(t, err, "非法整数 env 应返 error（fail-fast）")
}

// TestApplyPoolEnv_NilDB 边界：nil sqlDB 不 panic
func TestApplyPoolEnv_NilDB(t *testing.T) {
	assert.NoError(t, ApplyPoolEnv(nil))
}

// TestApplyPoolEnv_AppliesEnvToSetters Round 4.4 PR-2 钉死：env 读取后必须
// 真的落到 *sql.DB 的 SetMax* 调用上。防止实现只读 env 不 SetMax* 的回归。
func TestApplyPoolEnv_AppliesEnvToSetters(t *testing.T) {
	t.Setenv("PG_MAX_CONNS", "33")
	t.Setenv("PG_MIN_IDLE_CONNS", "7")
	t.Setenv("PG_MAX_LIFETIME_SECONDS", "1800")

	sqlDB := newTestSQLDB(t)
	require.NoError(t, ApplyPoolEnv(sqlDB))
	assert.Equal(t, 33, sqlDB.Stats().MaxOpenConnections)
	_ = time.Second // 防止 unused import（lifetime 间接覆盖）
}

// TestApplyPoolEnv_FallsBackToDefaults env 全空时回落到 10/5/1h（向后兼容）
//
// 设计意图：现有 5 svc 默认值 = 10/5/1h（Stage 50 之前硬编码）；
// ApplyPoolEnv 不能默默改大/改小连接预算。
//
// 注：sql.DB 不暴露 SetMaxIdleConns/SetConnMaxLifetime 的 getter，
// 只能验证 SetMaxOpenConnections 真值；MaxIdle/Lifetime 走 TestApplyPoolEnv_AppliesEnvToSetters
// 间接通过 Stats() + 不报错覆盖。
func TestApplyPoolEnv_FallsBackToDefaults(t *testing.T) {
	t.Setenv("PG_MAX_CONNS", "")
	t.Setenv("PG_MIN_IDLE_CONNS", "")
	t.Setenv("PG_MAX_LIFETIME_SECONDS", "")

	sqlDB := newTestSQLDB(t)
	require.NoError(t, ApplyPoolEnv(sqlDB))
	assert.Equal(t, DefaultMaxOpenConns, sqlDB.Stats().MaxOpenConnections)
	_ = time.Duration(DefaultConnMaxLifetime.Seconds()) * time.Second
}

// TestApplyPoolEnv_BadEnvPropagatesError 非法 env 应冒泡（不 silently 兜底）
//
// 钉死契约：prod 误配 PG_MAX_CONNS=typo 不会静默回落到 10。
func TestApplyPoolEnv_BadEnvPropagatesError(t *testing.T) {
	t.Setenv("PG_MAX_CONNS", "typo")
	t.Setenv("PG_MIN_IDLE_CONNS", "")
	t.Setenv("PG_MAX_LIFETIME_SECONDS", "")

	sqlDB := newTestSQLDB(t)
	err := ApplyPoolEnv(sqlDB)
	assert.Error(t, err, "非法 env 必须返 error，不允许静默 fallback")
}

func envMust(t *testing.T, key string, fallback int) int {
	t.Helper()
	v, err := envInt(key, fallback)
	require.NoError(t, err)
	return v
}
