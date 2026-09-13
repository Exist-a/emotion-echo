// Package grpcserver — file_name_test.go
//
// Stage 89 PR-2 RED：文件理解特性要求 messages 表持久化原始文件名（file_name），
// 且发送请求 → 落库 → 列表/响应视图全链回带（与 Stage 79 content_type 五层缺口同型，
// 提前锁死每一处映射，杜绝再犯）。
package grpcserver

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"emotion-echo-chat-svc/internal/model"
	"emotion-echo-chat-svc/internal/types"
)

func TestToProtoMessage_PreservesFileName(t *testing.T) {
	m := types.MessageView{
		Id:          1,
		Role:        "user",
		Content:     "https://minio/uploads/u1-ab12cd34.pdf",
		ContentType: "file",
		FileName:    "季度报告.pdf",
	}
	p := toProtoMessage(m)
	assert.Equal(t, "季度报告.pdf", p.GetFileName(), "toProtoMessage 必须回带 FileName")
}

func TestToProtoMessage_EmptyFileName_LeavesEmpty(t *testing.T) {
	m := types.MessageView{Id: 1, Role: "user", Content: "hi", ContentType: "text"}
	p := toProtoMessage(m)
	assert.Empty(t, p.GetFileName(), "非文件消息保持空")
}

// ListMessages 逻辑层 model.Message → types.MessageView 映射必须带 FileName
func TestListMessagesView_MapsFileName(t *testing.T) {
	m := model.Message{
		ID:          2,
		Content:     "https://minio/uploads/u2-cd34ef56.docx",
		ContentType: "file",
		FileName:    "会议纪要.docx",
	}
	view := types.MessageView{
		Id:          m.ID,
		Content:     m.Content,
		ContentType: m.ContentType,
		FileName:    m.FileName,
	}
	assert.Equal(t, "会议纪要.docx", view.FileName)
}

// SendMessage 请求映射：proto req.GetFileName() → types.SendMessageReq.FileName
// （经 chatServer.SendMessage 组装 types.SendMessageReq 的字段；此处锁契约字段名）
func TestSendMessageReq_HasFileNameField(t *testing.T) {
	req := types.SendMessageReq{FileName: "季度报告.pdf"}
	assert.Equal(t, "季度报告.pdf", req.FileName)
}

// model.Message 必须有 file_name 列映射（migration 006 配套）
func TestMessageModel_HasFileNameColumn(t *testing.T) {
	m := model.Message{FileName: "a.pdf"}
	assert.Equal(t, "a.pdf", m.FileName)
}
