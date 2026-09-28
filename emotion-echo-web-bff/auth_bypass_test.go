package main

import (
	"testing"
)

// TestNoAuthPathPrefixes_ClientErrorMustStayOpen 锁定"未登录也要能上报前端错误"
//
// 2026-09-29 实测踩过两层坑：
//  1. 只在 BFF 注册路由 → 落到 APISIX catch-all `/api/v1/*`，被 jwt-auth 401
//     ⇒ 补了 seed.sh 白名单路由 119
//  2. 只补 APISIX 白名单 → BFF 自身的 GinAuthMiddleware 仍 401
//     ⇒ 必须同时加进 authPathBypass
// 两处缺一不可，此测试锁第 2 处；第 1 处由 seed_test.js 的路由断言锁。
func TestNoAuthPathPrefixes_ClientErrorMustStayOpen(t *testing.T) {
	hit := func(path string) bool {
		for _, p := range noAuthPathPrefixes {
			if len(path) >= len(p) && path[:len(p)] == p {
				return true
			}
		}
		return false
	}

	cases := []struct {
		path string
		want bool
	}{
		{"/api/v1/client-error", true},
		{"/api/v1/auth/login", true},
		{"/api/v1/users/me", false},
		{"/api/v1/ai/stream", false},
	}
	for _, c := range cases {
		if got := hit(c.path); got != c.want {
			t.Errorf("noAuthPathPrefixes(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}
