// Package sse — stream_test.go
//
// Stage 30 / stage-30-web-bff.md T3.27 RED: ai_stream orchestrator pipe
//
// 验证 unary 分析结果 → SSE 事件序列：
//   event: analysis → event: done
package sse

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamAnalysis_EmitsAnalysisThenDone(t *testing.T) {
	var buf bytes.Buffer
	err := StreamAnalysis(&buf, AnalysisResult{
		MessageID:      42,
		ConversationID: 10,
		PrimaryEmotion: "happy",
		SentimentScore: 0.7,
		Confidence:     0.9,
		Model:          "keyword-stub-v1",
	})
	require.NoError(t, err)

	out := buf.String()
	// 两个事件，各自空行结束
	events := strings.Split(strings.TrimSuffix(out, "\n\n"), "\n\n")
	require.Len(t, events, 2, "应恰好 2 个事件：analysis + done")

	assert.Contains(t, events[0], "event: analysis\n", "第一个事件应为 analysis")
	assert.Contains(t, events[0], `data: {"messageId":42,"conversationId":10,"primaryEmotion":"happy","sentimentScore":0.7,"confidence":0.9,"model":"keyword-stub-v1"}`, "analysis 数据应含完整结果")

	assert.Contains(t, events[1], "event: done\n", "第二个事件应为 done")
	assert.Contains(t, events[1], `data: {"status":"ok"}`, "done 数据应为 status ok")
}

func TestStreamAnalysis_OutputIsValidSSE(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, StreamAnalysis(&buf, AnalysisResult{MessageID: 1, PrimaryEmotion: "calm"}))

	out := buf.String()
	// 每条事件以空行结束（\n\n）
	assert.True(t, strings.HasSuffix(out, "\n\n"), "输出应以空行结束")
	// 无裸 JSON（data 前必须有 "data: " 前缀）
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "{") {
			t.Fatalf("裸 JSON 不应出现，got line: %q", line)
		}
	}
}
