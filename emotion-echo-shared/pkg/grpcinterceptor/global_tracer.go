// Package grpcinterceptor — global_tracer.go
//
// CL-F-14：**可后置注册的全局 tracer**。
//
// 背景（2026-10-14 起栈实测，非推断）：svc 与 SkyWalking OAP 同时启动时，svc 侧
// tracer 初始化会失败（OAP 的 :11800 尚未 listen）。各 svc 的既有行为是
// 「打 warn 继续启动，进程余生不再重试」，而 **gin 中间件 / gRPC server 拦截器
// 在构造期就把 tracer 快照进了闭包** ⇒ 之后即使 OAP 起来、全局 tracer 被补上，
// 入口 span 仍永不创建；入口 span 不建 ⇒ 下游 exit span 因「无父不建」守卫
// 全部 noop ⇒ 该服务在 SkyWalking 服务列表里**整段生命周期缺席**
// （实测：`getAllServices` 5 个 vs 应有 7 个；`docker restart` 即愈）。
//
// 修法：把「谁持有 tracer」从**构造期快照**改为**请求期现读**——
// 入口路径在自身 tracer 为 nil 时回落读本全局；配合
// `skywalking.StartTracerBackgroundRetry` 的后台补挂，服务最终一定能出 span，
// 不依赖编排层的重启策略。
package grpcinterceptor

import "sync"

var (
	globalTracerMu sync.RWMutex
	globalTracer   Tracer
)

// SetGlobalTracer 注册全局 tracer（传 nil 表示清空）。
//
// 与 `skywalking.SetTracer` 的区别：后者持有 *go2sky.Tracer（出站埋点/HTTP/GORM 用），
// 本函数持有 Tracer 接口（入口中间件/拦截器用）。两者由 skywalking.SetTracer 一并维护。
func SetGlobalTracer(t Tracer) {
	globalTracerMu.Lock()
	globalTracer = t
	globalTracerMu.Unlock()
}

// GlobalTracer 返回当前全局 tracer；未注册时为 nil（调用方须自行判空）。
func GlobalTracer() Tracer {
	globalTracerMu.RLock()
	defer globalTracerMu.RUnlock()
	return globalTracer
}
