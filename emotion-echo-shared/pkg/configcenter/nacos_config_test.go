package configcenter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// -----------------------------------------------------------------------------
// isSensitiveDataId 单元测试（防敏感配置误推 Nacos）
// -----------------------------------------------------------------------------

func TestIsSensitiveDataId_Prefix(t *testing.T) {
	cases := []struct {
		dataId string
		want   bool
	}{
		{"jwt.secret", true},
		{"JWT.SECRET", true}, // case-insensitive
		{"database.dsn", true},
		{"db.password", true},
		{"kafka.brokers", true},
		{"llm.api_key", true},
		{"openai.key", true},
		{"deepseek.token", true},
		{"postgres_password", true},

		// 正常运营参数 → false
		{"user-svc.ops.yaml", false},
		{"feature_flags", false},
		{"rate_limit", false},
		{"model_router", false},
		{"kafka_retry_max", false}, // kafka_retry 不在敏感前缀（仅 kafka.）
	}
	for _, c := range cases {
		t.Run(c.dataId, func(t *testing.T) {
			assert.Equal(t, c.want, isSensitiveDataId(c.dataId))
		})
	}
}

func TestIsSensitiveDataId_Suffix(t *testing.T) {
	cases := []struct {
		dataId string
		want   bool
	}{
		{"anything.secret", true},
		{"my.SECRET", true}, // case-insensitive
		{"db.password", true},
		{"auth.token", true},
		{"primary.dsn", true},

		{"normal.yaml", false},
		{"feature_flags", false},
	}
	for _, c := range cases {
		t.Run(c.dataId, func(t *testing.T) {
			assert.Equal(t, c.want, isSensitiveDataId(c.dataId))
		})
	}
}

// -----------------------------------------------------------------------------
// buildServerConfigsInline 单元测试（与 discovery 包等价）
// -----------------------------------------------------------------------------

func TestBuildServerConfigsInline_SingleAddr(t *testing.T) {
	out, err := buildServerConfigsInline("127.0.0.1:8848")
	assert.NoError(t, err)
	assert.Len(t, out, 1)
	assert.Equal(t, "127.0.0.1", out[0].IpAddr)
	assert.Equal(t, uint64(8848), out[0].Port)
}

func TestBuildServerConfigsInline_MultiAddr(t *testing.T) {
	out, err := buildServerConfigsInline("n1:8848,n2:8848")
	assert.NoError(t, err)
	assert.Len(t, out, 2)
}

func TestBuildServerConfigsInline_Errors(t *testing.T) {
	_, err := buildServerConfigsInline("")
	assert.Error(t, err)
	_, err = buildServerConfigsInline("not-port")
	assert.Error(t, err)
	_, err = buildServerConfigsInline("127.0.0.1:bad")
	assert.Error(t, err)
}

// -----------------------------------------------------------------------------
// keyOfCC + groupOrDefault 单元测试
// -----------------------------------------------------------------------------

func TestKeyOfCC(t *testing.T) {
	assert.Equal(t, "user-svc.ops.yaml@DEFAULT_GROUP", keyOfCC("user-svc.ops.yaml", "DEFAULT_GROUP"))
}

func TestGroupOrDefault(t *testing.T) {
	assert.Equal(t, "FOO", groupOrDefault("FOO", "DEFAULT_GROUP"))
	assert.Equal(t, "DEFAULT_GROUP", groupOrDefault("", "DEFAULT_GROUP"))
}
