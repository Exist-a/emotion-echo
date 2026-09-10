// Package ctxkey — userid_test.go
//
// Sprint C GREEN: ctxkey 包自身单元测试（只测本包，不跨包）。
//
// 跨包等价测试放在 emotion-echo-web-bff/ctxkey_equivalence_test.go
// （grpcinterceptor 与 middleware 同时引会形成 cycle，不能放 shared 内部）。
package ctxkey

import (
	"context"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWithUserID_RoundTrip(t *testing.T) {
	ctx := WithUserID(context.Background(), 42)

	uid, ok := UserIDFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, int64(42), uid)
}

func TestUserIDFromContext_Empty(t *testing.T) {
	uid, ok := UserIDFromContext(context.Background())
	assert.False(t, ok)
	assert.Equal(t, int64(0), uid)
}

func TestWithUserID_OverwriteLast(t *testing.T) {
	// 标准库 context 行为：后写覆盖前写
	ctx := WithUserID(context.Background(), 1)
	ctx = WithUserID(ctx, 2)

	uid, ok := UserIDFromContext(ctx)
	require.True(t, ok)
	assert.Equal(t, int64(2), uid)
}

func TestUserID_TypeEmptyStruct(t *testing.T) {
	// 防御性断言：UserID 是 struct{}（零字节）。未来若有人误改成有字段的类型，本测试会失败。
	var u UserID
	assert.Equal(t, 0, int(unsafe.Sizeof(u)), "UserID 必须是零字节 struct{}")
}
