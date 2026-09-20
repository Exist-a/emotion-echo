// Package handler — viewmodel_config_test.go
//
// E2E-12 RED：toProfileVM 硬编码 Config: map[string]any{}（viewmodel.go:106），
// 下游 user-svc 返回的 config 被丢弃，前端永远读到空 config。
// 契约：透传下游 Config；nil 时兜底空 map（向后兼容无 config 的旧用户）。
package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"emotion-echo-web-bff/internal/downstream"
)

func TestToProfileVM_PreservesDownstreamConfig(t *testing.T) {
	t.Parallel()

	u := &downstream.UserInfo{
		UserID:    1,
		Account:   "echo",
		Nickname:  "Echo",
		AvatarURL: "https://minio/avatar.png",
		CreatedAt: 1700000000,
		Config:    map[string]any{"fontSize": "18px", "theme": "dark"},
	}
	vm := toProfileVM(u)

	assert.Equal(t, "18px", vm.Config["fontSize"], "下游 fontSize 必须透传，不得硬编码空 map")
	assert.Equal(t, "dark", vm.Config["theme"], "下游 theme 必须透传")
}

func TestToProfileVM_NilConfig_FallsBackToEmptyMap(t *testing.T) {
	t.Parallel()

	u := &downstream.UserInfo{
		UserID:    2,
		Account:   "old_user",
		Nickname:  "Old",
		CreatedAt: 1700000000,
		// Config 未设置（旧用户无 config）
	}
	vm := toProfileVM(u)

	assert.NotNil(t, vm.Config, "Config 为 nil 时应兜底空 map，不得返回 nil（前端 JSON 反序列化会出问题）")
	assert.Empty(t, vm.Config, "无 config 时应为空 map")
}