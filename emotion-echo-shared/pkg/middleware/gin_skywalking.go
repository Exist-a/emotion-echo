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
// PR-OBS-15: 中间件现在用 logging.WithTraceID 包 c.Request.Context(),
// 让 handler 内 slog.InfoContext(ctx, ...) 自动带 trace_id 字段。
// trace_id 来源优先级:
//   1. X-Trace-Id header (APISIX 路径,可显式注入)
//   2. skywalking span 上下文(若未来 span 内携带 trace id)
//   3. 跳过(无 trace_id,handler 日志无此字段)
//
// PR-OBS-23: handler err 透传判定(buildSpanError),优先级:
//   1. c.Errors() 非空 → EndSpan(c.Errors.Last().Err)  (handler 显式 c.Error)
//   2. status >= 500 → EndSpan(http <code>) 兜底 (handler 现状 c.JSON(500, ...) 不调 c.Error)
//   3. 其他 → EndSpan(nil) (4xx 业务正常 + 200 成功)
// 让 OAP UI 能直接过滤"5xx + error"维度。
//
// 中间件顺序(各 svc main.go 约定): Recovery → Metrics → Skywalking → Auth。
// Skywalking 在 Auth 之前跑,但 X-User-Id / X-Trace-Id header 已由 APISIX jwt-auth 注入到
// Request,无需等 ctx.Value(CtxUserIDKey{})。
package middleware

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/emotion-echo/shared/pkg/grpcinterceptor"
	"github.com/emotion-echo/shared/pkg/logging"
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

		// PR-OBS-15: 把 trace_id (来自 X-Trace-Id header) 注入 Request ctx,
		// 让 handler 内 slog.InfoContext(ctx, ...) 自动带 trace_id 字段(Loki 可查)
		if tid := c.GetHeader("X-Trace-Id"); tid != "" {
			c.Request = c.Request.WithContext(logging.WithTraceID(c.Request.Context(), tid))
		}

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
			span.EndSpan(buildSpanError(c))
		}
	}
}

// buildSpanError PR-OBS-23 决定 span.EndSpan(err) 应传什么 err:
//
// 1. handler 调 c.Error(err) → EndSpan(err) (handler 显式声明 soft error 时
//    即使 status 200 也视为 err,符合业务"部分成功 + 警告"语义)
// 2. handler 未 c.Error + status >= 500 → EndSpan(http 500) 兜底
//    (chat/user/assessment/analytics/ai/web-bff 现状都是 c.JSON(500, ...) 不用 c.Error)
// 3. 其他 → nil (4xx 业务正常如 401/403/404,或 200 成功)
//
// 返回 error 或 nil。span.EndSpan 收到 err 时,go2sky adapter 会调
// span.Error(time.Now(), err.Error()) 让 OAP UI error 列可见。
func buildSpanError(c *gin.Context) error {
	// 1. 优先 handler 显式 c.Error(err)
	if lastErr := c.Errors.Last(); lastErr != nil {
		// gin.Error.Unwrap() 返回底层 error;若底层为 nil (handler 误用) 兜底 gin.Error 自身
		if unwrapped := lastErr.Unwrap(); unwrapped != nil {
			return unwrapped
		}
		return lastErr
	}
	// 2. 兜底 status >= 500
	if status := c.Writer.Status(); status >= 500 {
		return fmt.Errorf("http %d", status)
	}
	// 3. 4xx / 200 + 无 c.Error → 不视为 err
	return nil
}