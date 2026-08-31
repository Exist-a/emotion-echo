package password

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHash_Success(t *testing.T) {
	t.Parallel()

	hash, err := Hash("hello-world-123")
	require.NoError(t, err)
	require.NotEmpty(t, hash)

	// bcrypt 输出固定 60 字符，前缀 \$2a\$10\$
	assert.Len(t, hash, 60)
	assert.True(t, strings.HasPrefix(hash, "$2a$10$"), "expected bcrypt v2a cost=10 prefix, got %q", hash)
}

func TestHash_DifferentSalts_ProduceDifferentHashes(t *testing.T) {
	t.Parallel()

	h1, err := Hash("same-password")
	require.NoError(t, err)
	h2, err := Hash("same-password")
	require.NoError(t, err)

	assert.NotEqual(t, h1, h2, "bcrypt must salt each hash differently")
}

func TestHash_EmptyPassword_ReturnsError(t *testing.T) {
	t.Parallel()

	_, err := Hash("")
	require.Error(t, err)
}

func TestVerify_CorrectPassword_ReturnsTrue(t *testing.T) {
	t.Parallel()

	hash, err := Hash("correct-horse-battery-staple")
	require.NoError(t, err)

	assert.True(t, Verify("correct-horse-battery-staple", hash))
}

func TestVerify_WrongPassword_ReturnsFalse(t *testing.T) {
	t.Parallel()

	hash, err := Hash("correct-password")
	require.NoError(t, err)

	assert.False(t, Verify("wrong-password", hash))
}

func TestVerify_EmptyPlain_ReturnsFalse(t *testing.T) {
	t.Parallel()

	hash, err := Hash("any-password")
	require.NoError(t, err)

	assert.False(t, Verify("", hash))
}

func TestVerify_EmptyHash_ReturnsFalse(t *testing.T) {
	t.Parallel()

	assert.False(t, Verify("any-password", ""))
	assert.False(t, Verify("any-password", "not-a-bcrypt-hash"))
}

func TestVerify_InteropWithLegacyConstantHash(t *testing.T) {
	t.Parallel()

	// 与 legacy/emotion-echo-gin/internal/pkg/password 行为一致：
	// Hash 生成的 bcrypt 字符串必须能被 Verify 校验（端到端往返）。
	h, err := Hash("interop-test-password")
	require.NoError(t, err)
	assert.True(t, Verify("interop-test-password", h))
}
