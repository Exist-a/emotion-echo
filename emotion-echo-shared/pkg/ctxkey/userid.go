// Package ctxkey — userid.go
//
// Sprint C: 单一 ctx key 来源，解决 grpcinterceptor 与 middleware 两套 ctx key
// 类型不一致问题（决策 18 #26，Stage 63 端到端验证发现）。
//
// 历史：
//   - middleware.CtxUserIDKey（jwt_auth.go:33）与 grpcinterceptor.CtxUserIDKeyType
//     （userid.go:30）原本各自定义独立 struct{}，注释写"同义"是错的
//   - 4 svc gRPC server userid 拦截器注入 user_id 到 ctx（类型 A），
//     4 svc logic 读 ctx 用的类型 B → ctx.Value 永远 miss → 业务 401
//
// 本包设计：
//   - ctxkey.UserID 是唯一的 ctx key 类型（struct{} 不导出字段，纯标识用）
//   - middleware 与 grpcinterceptor 用类型别名（type A = B）指向 ctxkey.UserID
//   - 本包不引 grpcinterceptor 或 middleware，避免循环依赖
//
// 调用方迁移：
//   - 现有调用 `ctx.Value(sharedmw.CtxUserIDKey{})` 或
//     `ctx.Value(grpcinterceptor.CtxUserIDKeyType{})` 都无须改（别名透明）
//   - 新代码可直接 `ctxkey.UserIDFromContext(ctx)` 取值（见 helper）
package ctxkey

import "context"

// UserID 是 ctx 中存 end user id 的唯一 key 类型。
//
// 为什么是 struct{}：
//   - struct{} 零字节内存开销
//   - 不能有方法（避免误以为是接口），纯标识
//   - 类型别名在编译期等价于 UserID（type A = UserID），跨包读写同一 ctx value
//
// 唯一约束：ctxkey 包**不能**引 grpcinterceptor 或 middleware（会与它们已存在的
// 互相引用形成循环）。
type UserID struct{}

// userIDKey 是 ctx 写入用的 key 实例（避免每处都写 UserID{} 字面量）。
var userIDKey = UserID{}

// WithUserID 返回注入 user_id 的 ctx（handler / interceptor 写入端用）。
func WithUserID(parent context.Context, uid int64) context.Context {
	return context.WithValue(parent, userIDKey, uid)
}

// UserIDFromContext 从 ctx 读 user_id（logic / 任何读取端用）。
// 返回 (0, false) 当 ctx 里没有 UserID 类型的值时。
func UserIDFromContext(ctx context.Context) (int64, bool) {
	v, ok := ctx.Value(userIDKey).(int64)
	return v, ok
}
