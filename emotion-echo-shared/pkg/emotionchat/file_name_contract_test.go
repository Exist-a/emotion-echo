package emotionchat

import "testing"

// Stage 89 PR-1 契约：SendMessageRequest / Message 携带 file_name（文件原始名）。
// 用途：LLM prompt 上下文 + 前端 ChatFile 展示（此前对象 key 只有 uid+hash+ext，原始名丢失）。
// 字段缺失 = proto 未升级，本测试编译失败即红。
func TestSendMessageRequestAndMessage_HaveFileName(t *testing.T) {
	req := &SendMessageRequest{FileName: "report.pdf"}
	if req.GetFileName() != "report.pdf" {
		t.Fatalf("SendMessageRequest.GetFileName() = %q, want %q", req.GetFileName(), "report.pdf")
	}
	msg := &Message{FileName: "report.pdf"}
	if msg.GetFileName() != "report.pdf" {
		t.Fatalf("Message.GetFileName() = %q, want %q", msg.GetFileName(), "report.pdf")
	}
}
