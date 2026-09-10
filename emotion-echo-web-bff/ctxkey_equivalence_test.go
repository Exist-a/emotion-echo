// Package main — ctxkey_equivalence_test.go
//
// Sprint C RED: ctx key 等价契约测试（跨包）。
//
// 行为契约（ctxkey 重构完成后必须成立）：
//   - sharedmw.CtxUserIDKey{} 与 grpcinterceptor.CtxUserIDKeyType{} 是同一类型
//     （类型别名 / 指向同一 struct{}），保证 ctx.Value() 在两边读同一 value
//
// 修复前：两个类型各自定义 struct{}（userid.go:30 与 jwt_auth.go:33），编译期不同，
// ctx.Value 永远 miss → 业务 401 "missing user id in context"。Stage 63 端到端
// 验证发现（决策 18 #26）。
//
// 测试位置说明：本测试放 BFF 根包测试，BFF 已同时引 middleware + grpcinterceptor，
// 不形成 import cycle。
package main

import (
	"context"
	"reflect"
	"testing"

	grpcinterceptor "github.com/emotion-echo/shared/pkg/grpcinterceptor"
	sharedmw "github.com/emotion-echo/shared/pkg/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCtxUserIDKey_TypeAlias 断言两个类型别名指向同一底层类型。
func TestCtxUserIDKey_TypeAlias(t *testing.T) {
	mwType := reflect.TypeOf(sharedmw.CtxUserIDKey{})
	giType := reflect.TypeOf(grpcinterceptor.CtxUserIDKeyType{})

	assert.Equal(t, mwType, giType,
		"sharedmw.CtxUserIDKey 与 grpcinterceptor.CtxUserIDKeyType 必须是同一类型（类型别名指向同一底层类型）")
}

// TestCtxUserIDKey_CrossReadWrite 跨包 ctx 读写一致性。
//
// 模拟场景：gRPC server 拦截器注入 user_id 到 ctx → svc logic 从 ctx 读。
// 修复前（两个独立 struct{}）：logic 永远 miss → 业务 401。
// 修复后（类型别名）：logic 读到 uid。
func TestCtxUserIDKey_CrossReadWrite(t *testing.T) {
	ctx := context.Background()

	// 模拟 gRPC userid 拦截器：写 user_id 到 ctx
	ctx = context.WithValue(ctx, grpcinterceptor.CtxUserIDKeyType{}, int64(42))

	// svc logic 从 ctx 读（同一类型才能取到）
	uid, ok := ctx.Value(sharedmw.CtxUserIDKey{}).(int64)
	require.True(t, ok, "logic 必须能从拦截器写入的 ctx 读出 uid")
	assert.Equal(t, int64(42), uid, "读出的 uid 必须等于写入的 uid")
}

// TestCtxUserIDKey_ReverseDirection 反向也通。
//
// 场景：HTTP middleware (sharedmw) 注入 → gRPC server 拦截器读
func TestCtxUserIDKey_ReverseDirection(t *testing.T) {
	ctx := context.Background()

	// HTTP middleware 注入
	ctx = context.WithValue(ctx, sharedmw.CtxUserIDKey{}, int64(99))

	// gRPC 拦截器读
	uid, ok := grpcinterceptor.UserIDFromGRPCContext(ctx)
	require.True(t, ok, "拦截器必须能从 HTTP middleware 写入的 ctx 读出 uid")
	assert.Equal(t, int64(99), uid)
}
