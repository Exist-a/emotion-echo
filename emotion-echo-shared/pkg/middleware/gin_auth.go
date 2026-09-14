// Package middleware 提供 Emotion-Echo 各 Go svc 的共享 HTTP 中间件（Gin 版本）
//
// GinAuthMiddleware 是 jwt_auth.go 中 AuthMiddleware 的 Gin 适配版本。
// 逻辑：从 X-User-Id header 读取 user_id（已被 APISIX jwt-auth 验签后注入），
// 注入到 ctx。
//
// 流程：
//   浏览器 → APISIX jwt-auth 验证 token → 通过后注入 X-User-Id: <uid>
//          → svc 信任 APISIX（不再验证 signature）
//          → svc 读 X-User-Id header，转 int64，注入 ctx
//
// Stage 94 PR-6 §P0-7：APISIX IP 白名单 —— 仅允许可信 APISIX 来源 IP
// 携带 X-User-Id header，避免 svc 端口被外部直连时 header 伪造。
// 默认 RequireAPISIXIP=true（生产安全），dev 环境显式关 false。
package middleware

import (
	"context"
	"fmt"
	"net"
	"strings"

	"github.com/gin-gonic/gin"
)

// AuthOpts GinAuthMiddlewareWithOpts 配置选项
type AuthOpts struct {
	// RequireAPISIXIP 是否要求 X-User-Id 来自 APISIX IP 白名单内
	// 默认 true（生产）。dev 模式（k8s 外 / 本地直连调试）设为 false
	RequireAPISIXIP bool
	// APISIXCIDRs 可信 APISIX IP 段(CIDR),如 ["10.0.0.0/8"] 表示 k8s pod CIDR
	// 仅在 RequireAPISIXIP=true 时生效
	APISIXCIDRs []string
	// TrustedProxies 信任的 reverse proxy IP 段,从 c.Request.RemoteAddr 取 host
	// 时用 X-Forwarded-For(若 cfg.ForwardedByClientIP=true)。这里不做
	// Forwarded 信任 —— 直接看 RemoteAddr(gin 默认 RemoteAddr 是 TCP peer,
	// 不会被 X-Forwarded-For 污染)
	// 留作未来扩展
}

// compiledCIDRs 内部 cache:解析后的 *net.IPNet,避免每次请求重复 ParseCIDR
type compiledCIDRs struct {
	cidrs []*net.IPNet
}

func compileCIDRs(cidrs []string) (*compiledCIDRs, error) {
	out := &compiledCIDRs{}
	for _, c := range cidrs {
		_, ipnet, err := net.ParseCIDR(c)
		if err != nil {
			return nil, err
		}
		out.cidrs = append(out.cidrs, ipnet)
	}
	return out, nil
}

// isFromTrustedAPISIX 判断 remoteAddr 是否在 APISIXCIDRs 白名单内
func (c *compiledCIDRs) isFromTrustedAPISIX(remoteAddr string) bool {
	// RemoteAddr 形如 "host:port" 或 "[ipv6]:port",先剥 port
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		// 已经是 bare host(无 port) —— 直接用
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, ipnet := range c.cidrs {
		if ipnet.Contains(ip) {
			return true
		}
	}
	return false
}

// GinAuthMiddleware 保留向后兼容 helper —— 默认 RequireAPISIXIP=false
// （保留旧 dev mode 行为,任何 X-User-Id 都接受 —— 与 Stage 32 PR-16 一致）。
//
// 生产 BFF 必须用 GinAuthMiddlewareWithOpts 显式开启 RequireAPISIXIP=true
// + 配 APISIXCIDRs。这是 §P0-7 修复的核心：让 svc 端口直接 dial 不可伪造身份。
//
// 注意:此处默认 false 是为了 Stage 32 PR-16 dev 兼容;生产部署必须升级到
// GinAuthMiddlewareWithOpts(AuthOpts{RequireAPISIXIP: true, ...})。
func GinAuthMiddleware() gin.HandlerFunc {
	return GinAuthMiddlewareWithOpts(AuthOpts{RequireAPISIXIP: false})
}

// GinAuthMiddlewareWithOpts §P0-7 修复入口：
//
//   - RequireAPISIXIP=true：必须 X-User-Id 来自 APISIXCIDRs 白名单内 remoteAddr,
//     否则 401 (防止外部直连 svc 端口伪造 header)
//   - RequireAPISIXIP=false：dev 模式,任何来源都接受(Stage 32 PR-16 行为)
//
// 用法示例(生产):
//
//	r.Use(sharedmw.GinAuthMiddlewareWithOpts(sharedmw.AuthOpts{
//	    RequireAPISIXIP: true,
//	    APISIXCIDRs:     []string{"10.0.0.0/8"}, // k8s pod CIDR
//	}))
//
// 用法示例(dev):
//
//	r.Use(sharedmw.GinAuthMiddlewareWithOpts(sharedmw.AuthOpts{RequireAPISIXIP: false}))
func GinAuthMiddlewareWithOpts(opts AuthOpts) gin.HandlerFunc {
	cidrs, _ := compileCIDRs(opts.APISIXCIDRs) // 编译失败时退化空白名单（永远 isFromTrustedAPISIX=false）

	return func(c *gin.Context) {
		// 跳过白名单端点（monitoring / metrics 不需要鉴权）
		path := c.Request.URL.Path
		if path == "/health" || path == "/metrics" {
			c.Next()
			return
		}

		uidHeader := c.GetHeader(XUserIDHeader)
		// §P0-7 修复：即使 X-User-Id 合法,也先校验来源 IP(若开启)
		if opts.RequireAPISIXIP {
			remoteAddr := c.Request.RemoteAddr
			if cidrs == nil || !cidrs.isFromTrustedAPISIX(remoteAddr) {
				logReject(c, "unauthorized: X-User-Id from untrusted remote addr "+
					"(RequireAPISIXIP=true; remoteAddr="+remoteAddr+"; APISIXCIDRs="+
					strings.Join(opts.APISIXCIDRs, ",")+")")
				return
			}
		}

		uid, ok := parseXUserID(uidHeader)
		if !ok || uid <= 0 {
			c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized: missing or invalid X-User-Id"})
			return
		}
		// 注入 user_id 到 ctx（与 rest 版本共享 CtxUserIDKey 类型）
		ctx := context.WithValue(c.Request.Context(), CtxUserIDKey{}, uid)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

// logReject 在 RequireAPISIXIP 拒绝路径上记日志(供 Sentry/alertmanager 追查),
// 但响应文案对外不暴露白名单细节(返回通用 unauthorized 文案)
func logReject(c *gin.Context, internalDetail string) {
	// 内部 log:运维可追查;外部响应:不泄露白名单 / remoteAddr
	c.AbortWithStatusJSON(401, gin.H{"error": "unauthorized"})
	// Gin's c.Error() 链挂载 internalDetail 供 logger middleware / Sentry 取
	// 但不直接 print 到 stdout(避免污染测试输出 + 防止 log injection)
	_ = c.Error(fmt.Errorf("%s", internalDetail))
}

// parseXUserID 解析 X-User-Id header 值
func parseXUserID(h string) (int64, bool) {
	if h == "" {
		return 0, false
	}
	var uid int64
	for _, c := range h {
		if c < '0' || c > '9' {
			return 0, false
		}
		uid = uid*10 + int64(c-'0')
	}
	if uid <= 0 {
		return 0, false
	}
	return uid, true
}
