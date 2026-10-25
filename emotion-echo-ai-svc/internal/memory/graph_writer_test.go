// CL-08c · Phase 4 · RED→GREEN：图写入端（ai-svc → llm-service MemoryGraphService）。
//
// 失败语义（ADR 决策 8 / R9）：PG 是系统记录 ⇒ 图写 best-effort ——
// 成功抽取后异步投递一次；失败只记日志、绝不阻塞/重试抽取主流程
//（缺口由「从 PG 重建图」工具兜底，CL-08d）。
package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeGraphWriter 记录调用（供 handler 侧接线测试用，见 logic 包）。
type FakeGraphWriter struct {
	Calls []Episode
	Err   error
}

func (f *FakeGraphWriter) UpsertEpisode(ctx context.Context, ep Episode) error {
	f.Calls = append(f.Calls, ep)
	return f.Err
}

func TestEpisodeGraphWriterContract(t *testing.T) {
	// 契约 1：Noop 永不报错（未装配 = 行为零变化）
	var w EpisodeGraphWriter = NoopGraphWriter{}
	assert.NoError(t, w.UpsertEpisode(context.Background(), Episode{UserID: 1}))

	// 契约 2：gRPC 请求映射（字段一一对齐 proto）
	ep := Episode{
		UserID: 42, Name: "msg-100", Content: "我在学萨克斯",
		ValidAtUnixMs: 1790000000000, SourceKind: "message", SourceID: 100,
	}
	req := EpisodeToPB(ep)
	require.NotNil(t, req)
	assert.EqualValues(t, 42, req.UserId)
	assert.Equal(t, "msg-100", req.Name)
	assert.Equal(t, "我在学萨克斯", req.Content)
	assert.EqualValues(t, 1790000000000, req.ValidAtUnixMs)
	assert.Equal(t, "message", req.SourceKind)
	assert.EqualValues(t, 100, req.SourceId)
}

func TestNewGraphWriterFromEnv_Degrades(t *testing.T) {
	t.Setenv("MEMORY_GRAPH_ADDR", "")
	w := NewGraphWriterFromEnv()
	_, isNoop := w.(NoopGraphWriter)
	assert.True(t, isNoop, "未配地址 ⇒ Noop（不装配、不报错）")
}

// grpc.NewClient 是惰性连接：构造必成功，不可达在**首次调用**暴露为 error。
func TestGraphWriter_UnreachableFailsOnCallNotConstruct(t *testing.T) {
	t.Setenv("MEMORY_GRAPH_TLS", "0") // 明文路径：mTLS 证书在容器外不存在
	w, err := NewGraphWriterWithAddr("127.0.0.1:1", "test-key", 300*time.Millisecond)
	require.NoError(t, err, "惰性连接：构造不得失败")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err = w.UpsertEpisode(ctx, Episode{UserID: 1, Name: "n", Content: "c"})
	assert.Error(t, err, "不可达地址必须在调用时报错（best-effort 由 handler 吞掉记日志）")
	_ = errors.New
}
