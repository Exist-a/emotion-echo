// Sprint 1 PR-4b: storage 包单元测试
//
// 测试范围（纯逻辑 + 接口契约，不连真 MinIO）：
//   1. ObjectKey 命名约定
//   2. GetObjectURL 格式（http/https + trailing slash）
//   3. NewMinIOClient 工厂默认 Bucket

package storage

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestObjectKey_Format(t *testing.T) {
	tests := []struct {
		name     string
		uid      int64
		filename string
		wantExt  string
	}{
		{"jpg", 1, "avatar.jpg", ".jpg"},
		{"png", 42, "PHOTO.PNG", ".png"}, // ext 小写
		{"no ext defaults to .bin", 7, "weird", ".bin"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ObjectKey(tc.uid, tc.filename)
			// 断言 1: prefix = avatars/<uid>-
			assert.True(t, strings.HasPrefix(got, "avatars/"+itoa(tc.uid)+"-"),
				"key %q 应以 avatars/<uid>- 起", got)
			// 断言 2: suffix = 8 hex + ext
			rest := strings.TrimPrefix(got, "avatars/"+itoa(tc.uid)+"-")
			assert.Len(t, rest, 8+len(tc.wantExt), "rest = 8 hex + ext")
			assert.True(t, strings.HasSuffix(got, tc.wantExt), "应以下以 %s 结尾", tc.wantExt)
		})
	}
}

func TestObjectKey_StableForSameFilename(t *testing.T) {
	a := ObjectKey(1, "avatar.jpg")
	b := ObjectKey(1, "avatar.jpg")
	assert.Equal(t, a, b, "同 uid + filename 必须产生 stable key")
}

func TestObjectKey_DifferentUIDIsolates(t *testing.T) {
	a := ObjectKey(1, "avatar.jpg")
	b := ObjectKey(2, "avatar.jpg")
	assert.NotEqual(t, a, b, "不同 uid 应产生不同 key")
}

func TestGetObjectURL_HTTP(t *testing.T) {
	m := &MinIOClient{
		cfg: MinIOConfig{
			Bucket:        "avatars",
			UseSSL:        false,
			PublicBaseURL: "http://localhost:9000",
		},
	}
	got := m.GetObjectURL("avatars/1-abc.jpg")
	assert.Equal(t, "http://localhost:9000/avatars/avatars/1-abc.jpg", got)
}

func TestGetObjectURL_HTTPS(t *testing.T) {
	m := &MinIOClient{
		cfg: MinIOConfig{
			Bucket:        "avatars",
			UseSSL:        true,
			PublicBaseURL: "https://cdn.example.com",
		},
	}
	got := m.GetObjectURL("avatars/1-abc.jpg")
	assert.Equal(t, "https://cdn.example.com/avatars/avatars/1-abc.jpg", got)
}

func TestGetObjectURL_StripsTrailingSlash(t *testing.T) {
	m := &MinIOClient{
		cfg: MinIOConfig{
			Bucket:        "avatars",
			PublicBaseURL: "http://localhost:9000/", // trailing slash
		},
	}
	got := m.GetObjectURL("avatars/1-x.png")
	// 必须不会出 http://localhost:9000//avatars/...
	assert.NotContains(t, got, "//avatars", "不能有双斜杠")
	assert.Equal(t, "http://localhost:9000/avatars/avatars/1-x.png", got)
}

func TestNewMinIOClient_DefaultBucket(t *testing.T) {
	c, err := NewMinIOClient(MinIOConfig{
		Endpoint:  "127.0.0.1:1", // 不拨号，minio.New 纯客户端构造
		AccessKey: "test",
		SecretKey: "test",
		// Bucket 故意留空
	})
	require.NoError(t, err, "工厂不应失败——minio.New 不拨号")
	assert.Equal(t, "avatars", c.cfg.Bucket, "空 Bucket 应默认 avatars")
}

func TestNewMinIOClient_CustomBucket(t *testing.T) {
	c, err := NewMinIOClient(MinIOConfig{
		Endpoint: "127.0.0.1:1",
		Bucket:   "custom-bucket",
	})
	require.NoError(t, err)
	assert.Equal(t, "custom-bucket", c.cfg.Bucket)
}

// helper
func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}