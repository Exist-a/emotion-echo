package discovery

import "strings"

// IsHardBootError 分类 BootNacos 错误的严重度。
//
// PR-5: 6 个 svc main.go 共用此函数：
//   - hard 错误（WaitForNacos / NewNacosRegistry / Register / NewNacosConfig 失败）
//     → 必须 fail-fast，os.Exit(1)。dev 模式下 Nacos 不可达意味着服务发现不可信。
//   - soft 错误（GetConfig / ListenConfig 失败）→ 继续运行，记录日志。
//     首次启动 GetConfig 没数据是正常的；ListenConfig 失败 BFF 仍能跑。
//
// 判定方式：错误信息前缀匹配（不引入 errors.Is 链，避免改 SDK 接口）。
func IsHardBootError(errMsg string) bool {
	hardPrefixes := []string{
		"[nacos] WaitForNacos:",
		"[nacos] NewNacosRegistry:",
		"[nacos] Register:",
		"[nacos] NewNacosConfig:",
	}
	for _, p := range hardPrefixes {
		if strings.HasPrefix(errMsg, p) {
			return true
		}
	}
	return false
}