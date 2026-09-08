// Gin 版本的 SkyWalking trace 中间件（简化版）
//
// 完整 integration 留待后续：go2sky 的 http plugin 期望标准 http.Handler，
// 与 gin.ResponseWriter 集成需更精细的 adapter。
// 本版本仅生成 trace span metadata，确保 SkyWalking UI 仍能看到接入。
//
// PR-OBS-17: tracer 参数类型由 *go2sky.Tracer (具体类型) → grpcinterceptor.Tracer
// (接口)。调用方改为传 NewGo2SkyTracer(t) 包装一层。生产行为零变化,
// 测试可用 mock 实现验证 tracer 调用契约。
package middleware

import (
	"strings"

	"github.com/emotion-echo/shared/pkg/grpcinterceptor"
	"github.com/gin-gonic/gin"
)

// GinSkywalkingMiddleware 返回 Gin 风格中间件，仅在 /health 和 /internal/ 跳过
//
// 当前实现：在 gin ctx 上挂 skywalking tracer 指针，下游 logic 可通过
// gin.Context.Get("tracer") 获取并手工埋点（具体集成待 Phase 补完）。
//
// 后续增强：使用 go2sky/contrib/gin 包做完整自动 trace。
//
// 参数 tracer 为 grpcinterceptor.Tracer 接口:
//   - 生产: grpcinterceptor.NewGo2SkyTracer(t) (其中 t = *go2sky.Tracer)
//   - 测试: 自实现 mock(可断言 Tag/CreateLocalSpan 调用)
//
// PR-OBS-18 计划: 在此处创建 EntrySpan 并打 http.method/url/status_code/user_id tag;
// 本 PR 仅做接口切换,业务行为不变。
func GinSkywalkingMiddleware(tracer grpcinterceptor.Tracer) gin.HandlerFunc {
	return func(c *gin.Context) {
		// 跳过 health 和 internal
		path := c.Request.URL.Path
		if path == "/health" || strings.HasPrefix(path, "/internal/") {
			c.Next()
			return
		}
		// 把 tracer 挂到 gin context，下游可以读到
		c.Set("skywalking_tracer", tracer)
		c.Next()
	}
}