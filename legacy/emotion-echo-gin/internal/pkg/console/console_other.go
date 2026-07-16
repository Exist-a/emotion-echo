//go:build !windows
// +build !windows

package console

// SetUTF8 在非 Windows 平台上设置控制台输出为 UTF-8 编码（空操作）
func SetUTF8() bool {
	return true
}

// GetCP 获取当前控制台编码（非 Windows 平台返回 0）
func GetCP() uint {
	return 0
}
