// Package grpcserver — message_contenttype_test.go
//
// Stage 79 RED：SendMessage 请求侧 ContentType 早已透传（chat_server.go:124），
// 但响应视图 toProtoMessage 从未回带 content_type（proto Message 此前无该字段）——
// e2e 实测 DB 落库 file 而前端收到 text。契约：响应 Message 必须回带 ContentType。
package grpcserver

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"emotion-echo-chat-svc/internal/types"
)

func TestToProtoMessage_PreservesContentType(t *testing.T) {
	m := types.MessageView{
		Id:          1,
		Role:        "user",
		Content:     "https://minio/x.png",
		ContentType: "image",
	}
	p := toProtoMessage(m)
	assert.Equal(t, "image", p.GetContentType(), "toProtoMessage 必须回带 ContentType")
}

func TestToProtoMessage_EmptyContentType_LeavesEmpty(t *testing.T) {
	m := types.MessageView{Id: 1, Role: "user", Content: "hi"}
	p := toProtoMessage(m)
	assert.Empty(t, p.GetContentType(), "未设置时保持空（BFF 侧兜底默认 text）")
}
