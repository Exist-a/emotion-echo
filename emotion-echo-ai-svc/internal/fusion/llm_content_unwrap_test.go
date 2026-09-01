// Package fusion — Stage 35 · PR-1 RED
//
// LLM 真实返回常常包 ```json...``` markdown 标记。
// unwrapLLMContent 负责把各种包装剥掉，给 json.Unmarshal 一个干净的 JSON 字符串。
//
// 5 个 case 覆盖：
//   1. 纯 JSON（无包装）
//   2. ```json\n{...}\n``` 包裹
//   3. ```\n{...}\n``` 包裹（无语言标记）
//   4. 双重 JSON 编码（content 本身就是 JSON 字符串）
//   5. 前置自然语言 + JSON（LLM 偶发"以下是结果：..."模式）
package fusion

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUnwrapLLMContent_PureJSON 无包装直接返回 trim 后。
func TestUnwrapLLMContent_PureJSON(t *testing.T) {
	t.Parallel()
	in := `{"primary_emotion":"happy","sentiment_score":0.5}`
	out := unwrapLLMContent(in)
	assert.Equal(t, in, out)
}

// TestUnwrapLLMContent_MarkdownJSONFence ```json\n{...}\n```。
func TestUnwrapLLMContent_MarkdownJSONFence(t *testing.T) {
	t.Parallel()
	in := "```json\n{\"primary_emotion\":\"happy\",\"sentiment_score\":0.5}\n```"
	out := unwrapLLMContent(in)
	assert.Equal(t, `{"primary_emotion":"happy","sentiment_score":0.5}`, out)
}

// TestUnwrapLLMContent_MarkdownNoLangFence ```\n{...}\n```（无语言标记）。
func TestUnwrapLLMContent_MarkdownNoLangFence(t *testing.T) {
	t.Parallel()
	in := "```\n{\"primary_emotion\":\"sad\",\"sentiment_score\":-0.4}\n```"
	out := unwrapLLMContent(in)
	assert.Equal(t, `{"primary_emotion":"sad","sentiment_score":-0.4}`, out)
}

// TestUnwrapLLMContent_DoubleJSONEncoded content 本身就是 JSON 字符串（双重编码）。
func TestUnwrapLLMContent_DoubleJSONEncoded(t *testing.T) {
	t.Parallel()
	// 实际 LLM 返回的字符串是 "{\"primary_emotion\":\"happy\",...}" 这种被再序列化一次的字符串
	in := `"{\"primary_emotion\":\"happy\",\"sentiment_score\":0.5,\"modality_contrib\":{\"text\":1.0},\"reasoning\":\"\"}"`
	out := unwrapLLMContent(in)
	require.NotEmpty(t, out)
	assert.Contains(t, out, `"primary_emotion":"happy"`)
	assert.Contains(t, out, `"sentiment_score":0.5`)
}

// TestUnwrapLLMContent_LeadingNaturalLanguage 前置自然语言（DeepSeek 偶发）。
func TestUnwrapLLMContent_LeadingNaturalLanguage(t *testing.T) {
	t.Parallel()
	in := "以下是融合结果：\n{\"primary_emotion\":\"neutral\",\"sentiment_score\":0.0}\n如有疑问请反馈。"
	out := unwrapLLMContent(in)
	assert.Contains(t, out, `"primary_emotion":"neutral"`)
	assert.Contains(t, out, `"sentiment_score":0`)
}

// TestUnwrapLLMContent_WhitespaceTrimmed 前后空白被清掉。
func TestUnwrapLLMContent_WhitespaceTrimmed(t *testing.T) {
	t.Parallel()
	in := "   \n\n  {\"primary_emotion\":\"calm\",\"sentiment_score\":0.1}  \n\n   "
	out := unwrapLLMContent(in)
	assert.Equal(t, `{"primary_emotion":"calm","sentiment_score":0.1}`, out)
}

// TestUnwrapLLMContent_EmptyReturnsEmpty 空字符串 → 空。
func TestUnwrapLLMContent_EmptyReturnsEmpty(t *testing.T) {
	t.Parallel()
assert.Equal(t, "", unwrapLLMContent(""))
assert.Equal(t, "", unwrapLLMContent("   "))
}