// pkg/discovery/nacos_register_host_test.go
//
// Stage 62 PR-3.4 冒烟测试中发现的高危 bug 回归护栏（决策 18 失真台账 #28）：
//
// 现象（2026-09-10 docker 实测）：
//   svc 启动 T+3s 时 Nacos 显示真实容器 IP（172.18.0.14:8888），
//   T+6s 起变成 0.0.0.0:8888 —— 全 6 个 svc 均如此。
//
// 根因：
//   Register() 已把 yaml 的 "0.0.0.0" 占位解析为本机 IP（b869ff9 PR-1 修复），
//   但 Heartbeat() 用原始 ins.Host（"0.0.0.0"）调 UpdateInstance，
//   把正确注册覆盖成 0.0.0.0；Unregister() 同样用原始 ins.Host，
//   导致注销不到真正注册的那个实例。
//
// 影响：
//   APISIX 走 nacos-discovery 拉上游实例 → 拿到 0.0.0.0 → connect refused
//   → **dev 网关全链路 502**（curl http://localhost:19080/api/v1/auth/login → 502）。
//
// 契约：Register / Unregister / Heartbeat 三处必须用同一份 IP 解析结果。

package discovery

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestRegisterHost_ResolvesPlaceholderIPs：yaml 占位/空值必须解析为可路由 IP。
func TestRegisterHost_ResolvesPlaceholderIPs(t *testing.T) {
	// "0.0.0.0"（yaml 默认 Host）→ 必须是真实 IP，不能原样返回
	got := registerHost("0.0.0.0")
	assert.NotEqual(t, "0.0.0.0", got, "0.0.0.0 必须被解析为可路由 IP（否则 Nacos 存 0.0.0.0）")
	assert.NotEmpty(t, got)

	// "" → 同样解析
	gotEmpty := registerHost("")
	assert.NotEmpty(t, gotEmpty, "空 Host 必须被解析")
	assert.NotEqual(t, "0.0.0.0", gotEmpty)

	// 显式 IP → 原样返回（不解析）
	assert.Equal(t, "10.1.2.3", registerHost("10.1.2.3"), "显式 IP 应原样透传")
	assert.Equal(t, "172.18.0.14", registerHost("172.18.0.14"))
}

// TestRegisterHost_StableAcrossCalls：同一输入必须返回同一结果。
//
// 为什么关键：Register 解析出 IP_A 注册；Heartbeat 若解析出 IP_B（或不解析=0.0.0.0），
// UpdateInstance 就会操作到另一个实例 → 真实实例被"覆盖"成 0.0.0.0（本次 bug）。
func TestRegisterHost_StableAcrossCalls(t *testing.T) {
	first := registerHost("0.0.0.0")
	for i := 0; i < 5; i++ {
		assert.Equal(t, first, registerHost("0.0.0.0"),
			"第 %d 次调用结果必须与首次一致（否则 Register/Heartbeat 会操作不同实例）", i+1)
	}
}

// TestNacosRegisterSource_AllSitesUseRegisterHost：源码级契约测试。
//
// 断言 nacos_register.go 里 Register / Unregister / Heartbeat 三处都不再把
// 原始 ins.Host 直接塞给 Nacos（那会写进 0.0.0.0）。
//
// 为什么用源码扫描：nacosnaming.INamingClient 是 20+ 方法的 SDK 大接口，
// fake 成本远高于收益；本 bug 的本质是"三个调用点漏用同一个 helper"，
// 源码级断言正好锁住这一点。
func TestNacosRegisterSource_AllSitesUseRegisterHost(t *testing.T) {
	src, err := os.ReadFile("nacos_register.go")
	assert.NoError(t, err)
	code := string(src)

	// 1. 不允许把原始 ins.Host 直接当 Ip 传给 Nacos
	assert.NotContains(t, code, "Ip:          ins.Host",
		"nacos_register.go 不应再把原始 ins.Host 传给 Nacos（会写 0.0.0.0）—— 必须用 registerHost(ins.Host)")

	// 2. registerHost 至少被 3 处使用（Register / Unregister / Heartbeat）
	count := strings.Count(code, "registerHost(ins.Host)")
	assert.GreaterOrEqual(t, count, 3,
		"registerHost(ins.Host) 应出现 ≥3 次（Register/Unregister/Heartbeat 三处），实际 %d 次", count)
}
