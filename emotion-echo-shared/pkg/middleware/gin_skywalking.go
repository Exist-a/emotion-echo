// Gin 版本的 SkyWalking trace 中间件（简化版）
//
// 完整 integration 留待后续：go2sky 的 http plugin 期望标准 http.Handler，
// 与 gin.ResponseWriter 集成需更精细的 adapter。
// 本版本仅生成 trace span metadata，确保 SkyWalking UI 仍能看到接入。
//
// PR-OBS-17: tracer 参数类型由 *go2sky.Tracer (具体类型) → grpcinterceptor.Tracer
// (接口)。调用方改为传 NewGo2SkyTracer(t) 包装一层。生产行为零变化,
// 测试可用 mock 实现验证 tracer 调用契约。
//
// PR-OBS-18: 中间件现在真正创建 EntrySpan 并打 4 个 http.* / user_id tag:
//   - http.method = c.Request.Method
//   - http.url = c.Request.URL.Path (请求实际路径,非路由模板)
//   - http.status_code = c.Writer.Status() (c.Next 后读取)
//   - user_id = c.GetHeader("X-User-Id") (APISIX 注入,AuthMiddleware 之前可读)
//
// 中间件顺序(各 svc main.go 约定): Recovery → Metrics → Skywalking → Auth。
// Skywalking 在 Auth 之前跑,但 X-User-Id header 已由 APISIX jwt-auth 注入到
// Request,无需等 ctx.Value(CtxUserIDKey{})。
package middleware

import (
	"strconv"
	"strings"

	"github.com/emotion-echo/shared/pkg/grpcinterceptor"
	"github.com/gin-gonic/gin"
)

// GinSkywalkingMiddleware 返回 Gin 风格中间件，仅在 /health 和 /internal/ 跳过
//
// PR-OBS-18 行为(相对 PR-OBS-17 增量):
//   - 业务路径: tracer.StartEntry(ctx, c.FullPath()) 创建 EntrySpan
//   - 4 tag 写入: http.method / http.url / user_id (c.Next 前) + http.status_code (c.Next 后)
//   - 收尾: c.Next 后立即 span.EndSpan(nil)(handler err 透传策略留 PR-OBS-23)
//   - 把 tracer 和 span 都挂到 gin ctx(下游用 c.Get("skywalking_span") 可读 span)
//
// 参数 tracer 为 grpcinterceptor.Tracer 接口:
//   - 生产: grpcinterceptor.NewGo2SkyTracer(t) (其中 t = *go2sky.Tracer)
//   - 测试: 自实现 mock(可断言 StartEntry/Tag/EndSpan 调用)
//
// nil tracer 安全: 与之前版本一致,直接 c.Next() 不挂 span(测试 PR-OBS-12 5 case 验证)。
func GinSkywalkingMiddleware(tracer grpcinterceptor.Tracer) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 跳过 health 和 internal(避免 /health scrape 制造无意义 span)
		path := c.Request.URL.Path
		if path == "/health" || strings.HasPrefix(path, "/internal/") {
			c.Next()
			return
		}
		// 把 tracer 挂到 gin context,下游可以读到
		c.Set("skywalking_tracer", tracer)

		// 业务路径: 创建 EntrySpan + 打前置 tag
		var span grpcinterceptor.Span
		if tracer != nil {
			_, span = tracer.StartEntry(c.Request.Context(), c.FullPath())
		}
		if span != nil {
			c.Set("skywalking_span", span)
			span.Tag("http.method", c.Request.Method)
			span.Tag("http.url", c.Request.URL.Path)
			// user_id 仅当 X-User-Id header 存在且为合法数字时打
			if uid := c.GetHeader("X-User-Id"); uid != "" {
				if _, err := strconv.ParseInt(uid, 10, 64); err == nil {
					span.Tag("user_id", uid)
				}
			}
		}

		c.Next()

		// c.Next 之后: 补打 http.status_code 并 EndSpan
		// 同步顺序而非 defer,确保 4 tag 全部写入后才 EndSpan
		if span != nil {
			span.Tag("http.status_code", strconv.Itoa(c.Writer.Status()))
			span.EndSpan(nil)
		}
	}
}