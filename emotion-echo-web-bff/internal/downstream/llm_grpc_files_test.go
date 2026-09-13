// Package downstream — llm_grpc_files_test.go
//
// Stage 89 PR-3 RED：LLMGRPCClient.StreamChat 必须把 handler 侧 FileAttachment
// 映射到 proto ChatCompletionRequest.files（文件理解，字节不过 gRPC）。
package downstream

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLLMGRPCClient_StreamChat_MapsFiles(t *testing.T) {
	fake := &fakeLLMServiceServicer{deltas: []string{"ok"}}
	c := startFakeLLMServer(t, fake)

	err := c.StreamChat(context.Background(), LLMStreamRequest{
		Messages: []Message{{Role: "user", Content: "看看这个文件"}},
		Files: []FileAttachment{
			{URL: "http://emotion-echo-minio:9000/avatars/uploads/u1-ab12cd34.pdf", Name: "report.pdf"},
		},
	}, func(delta, model string) {})
	require.NoError(t, err)

	require.Len(t, fake.gotReq.GetFiles(), 1, "Files 必须映射到 proto files")
	f := fake.gotReq.GetFiles()[0]
	assert.Equal(t, "report.pdf", f.GetName())
	assert.Equal(t, "http://emotion-echo-minio:9000/avatars/uploads/u1-ab12cd34.pdf", f.GetUrl())
}

func TestLLMGRPCClient_StreamChat_NoFiles_LeavesEmpty(t *testing.T) {
	fake := &fakeLLMServiceServicer{deltas: []string{"ok"}}
	c := startFakeLLMServer(t, fake)

	err := c.StreamChat(context.Background(), LLMStreamRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(delta, model string) {})
	require.NoError(t, err)
	assert.Empty(t, fake.gotReq.GetFiles())
}
