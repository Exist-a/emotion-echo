package dbconnect

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func envMust(t *testing.T, key string, fallback int) int {
	t.Helper()
	v, err := envInt(key, fallback)
	require.NoError(t, err)
	return v
}
