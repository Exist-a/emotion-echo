//go:build windows
// +build windows

package console

import (
	"syscall"
)

var (
	kernel32                  = syscall.NewLazyDLL("kernel32.dll")
	setConsoleOutputCPProc    = kernel32.NewProc("SetConsoleOutputCP")
	getConsoleOutputCPProc    = kernel32.NewProc("GetConsoleOutputCP")
)

const (
	CP_UTF8 = 65001
)

// SetUTF8 设置控制台输出为 UTF-8 编码
func SetUTF8() bool {
	// 获取当前编码
	currentCP, _, _ := getConsoleOutputCPProc.Call()
	if currentCP == CP_UTF8 {
		return true
	}

	// 设置为 UTF-8
	ret, _, _ := setConsoleOutputCPProc.Call(uintptr(CP_UTF8))
	return ret != 0
}

// GetCP 获取当前控制台编码
func GetCP() uint {
	cp, _, _ := getConsoleOutputCPProc.Call()
	return uint(cp)
}
