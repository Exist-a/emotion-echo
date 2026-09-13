package emotionllm

import "testing"

// Stage 89 PR-1 契约：ChatCompletionRequest.files 携带附件引用（url/name）。
// 字节不过 gRPC（默认 4MiB 帧限制），文件拉取与文本抽取在 llm-service 内完成。
// 字段缺失 = proto 未升级，本测试编译失败即红。
func TestChatCompletionRequest_HasFiles(t *testing.T) {
	req := &ChatCompletionRequest{
		Files: []*FileAttachment{
			{Url: "http://emotion-echo-minio:9000/avatars/uploads/u1-ab12cd34.pdf", Name: "report.pdf"},
		},
	}
	if len(req.GetFiles()) != 1 {
		t.Fatalf("GetFiles() len = %d, want 1", len(req.GetFiles()))
	}
	f := req.GetFiles()[0]
	if f.GetName() != "report.pdf" {
		t.Fatalf("FileAttachment.GetName() = %q, want %q", f.GetName(), "report.pdf")
	}
	if f.GetUrl() == "" {
		t.Fatal("FileAttachment.GetUrl() is empty")
	}
}
