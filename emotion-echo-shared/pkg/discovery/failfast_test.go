package discovery

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClassifyBootNacosError_Hard：当 WaitForNacos / Register 失败 → hard error。
// 调用方应 fail-fast（os.Exit(1)），因为 dev 模式下 Nacos 不可达意味着整套
// 服务发现不可信——继续跑等于"假装一切正常"。
func TestClassifyBootNacosError_Hard(t *testing.T) {
	// Sentinel error 用错误信息前缀约定（不引入 errors.Is 链以避免改动 SDK 接口）。
	hardPrefixes := []string{
		"[nacos] WaitForNacos:",
		"[nacos] NewNacosRegistry:",
		"[nacos] Register:",
		"[nacos] NewNacosConfig:",
	}
	for _, p := range hardPrefixes {
		assert.True(t, IsHardBootError(p), "prefix %q should be classified hard", p)
	}
}

// TestClassifyBootNacosError_Soft：GetConfig / ListenConfig 失败 → soft。
// 这两类首次启动 GetConfig 没数据是正常的；ListenConfig 失败时 BFF 仍能跑。
func TestClassifyBootNacosError_Soft(t *testing.T) {
	softPrefixes := []string{
		"[nacos] GetConfig(...): config rpc timeout",
		"[nacos] ListenConfig failed (continuing): conn refused",
	}
	for _, p := range softPrefixes {
		assert.False(t, IsHardBootError(p), "prefix %q should be classified soft", p)
	}
}

// TestIsHardBootError_Compiles：契约——保留 hard/soft 分类函数，
// 6 个 svc main.go 共用，避免各 svc 自己写 if-else。
func TestIsHardBootError_Compiles(t *testing.T) {
	require.NotPanics(t, func() {
		_ = IsHardBootError("dummy")
		_ = context.Background()
	})
}