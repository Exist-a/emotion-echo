// Package handler — viewmodel_contenttype_test.go
//
// Stage 79 RED：toMessageItemVM 硬编码 ContentType:"text"（viewmodel.go:81），
// 下游无论返回什么都渲染成 text——文件消息（image/file/video）无法在前端正确渲染。
// 契约：透传下游 ContentType；空值兜底 "text"（向后兼容旧消息）。
package handler

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"emotion-echo-web-bff/internal/downstream"
)

func TestToMessageItemVM_PreservesDownstreamContentType(t *testing.T) {
	m := &downstream.MessageView{ID: 1, Role: "user", Content: "https://minio/x.png", ContentType: "file"}
	vm := toMessageItemVM(m)
	assert.Equal(t, "file", vm.ContentType, "下游 contentType 必须透传，不得硬编码 text")
}

func TestToMessageItemVM_EmptyContentType_FallsBackToText(t *testing.T) {
	m := &downstream.MessageView{ID: 1, Role: "user", Content: "hi"}
	vm := toMessageItemVM(m)
	assert.Equal(t, "text", vm.ContentType, "下游为空时兜底 text（兼容旧消息）")
}
