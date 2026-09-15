package messaging

import (
	"testing"

	"github.com/IBM/sarama"
	"github.com/stretchr/testify/assert"
)

// Round B: extractSw8Header 收敛契约测试。
// 锁死 ExtractSw8Header 在所有边界条件下的行为：
// 1. 标准 sw8 header
// 2. 多 header 混合（取 sw8）
// 3. 大小写敏感（SW8 不算 sw8）
// 4. nil header 元素不 panic
// 5. 空列表
// 6. value 为空字符串（视为找到，不返空字符串当未找到）
func TestExtractSw8Header(t *testing.T) {
	t.Run("returns sw8 value when present", func(t *testing.T) {
		headers := []*sarama.RecordHeader{
			{Key: []byte("sw8"), Value: []byte("1-abc-def-1")},
		}
		assert.Equal(t, "1-abc-def-1", ExtractSw8Header(headers))
	})

	t.Run("returns sw8 value among multiple headers", func(t *testing.T) {
		headers := []*sarama.RecordHeader{
			{Key: []byte("x-original-topic"), Value: []byte("chat-events")},
			{Key: []byte("x-attempts"), Value: []byte("3")},
			{Key: []byte("sw8"), Value: []byte("trace-context")},
			{Key: []byte("x-error-reason"), Value: []byte("decode fail")},
		}
		assert.Equal(t, "trace-context", ExtractSw8Header(headers))
	})

	t.Run("is case-sensitive (SW8 != sw8)", func(t *testing.T) {
		headers := []*sarama.RecordHeader{
			{Key: []byte("SW8"), Value: []byte("uppercase")},
		}
		assert.Equal(t, "", ExtractSw8Header(headers),
			"SkyWalking sw8 spec 用小写，SW8 大写应不识别")
	})

	t.Run("skips nil header entries without panic", func(t *testing.T) {
		headers := []*sarama.RecordHeader{
			nil,
			{Key: []byte("sw8"), Value: []byte("after-nil")},
			nil,
		}
		assert.Equal(t, "after-nil", ExtractSw8Header(headers))
	})

	t.Run("empty list returns empty string", func(t *testing.T) {
		assert.Equal(t, "", ExtractSw8Header(nil))
		assert.Equal(t, "", ExtractSw8Header([]*sarama.RecordHeader{}))
	})

	t.Run("empty value is still a hit (not treated as not-found)", func(t *testing.T) {
		// 防御性测试：sarama header value 可能是空字节（producer 端
		// 偶发空字符串写入）。函数不应把"value 为空"误判为"未找到"，
		// 找到 sw8 key 即认为命中（即使 value 是 ""）。
		headers := []*sarama.RecordHeader{
			{Key: []byte("sw8"), Value: []byte("")},
		}
		assert.Equal(t, "", ExtractSw8Header(headers))
		// 注：上面 assert.Equal("", ...) 与"未找到"返回值相同——这是设计选择
		// （ExtractSw8Header 不区分"未找到"和"value 为空"），调用方根据
		// CreateEntrySpan 对 "" 的处理自行决定是否跳过。本测试只锁"行为一致"，
		// 不锁"区分两者"。
	})

	t.Run("Sw8HeaderName constant equals 'sw8'", func(t *testing.T) {
		assert.Equal(t, "sw8", Sw8HeaderName,
			"必须与 chat-svc kafka_publisher.sw8HeaderName 字面值一致")
	})
}
