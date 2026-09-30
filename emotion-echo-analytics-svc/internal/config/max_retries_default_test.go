package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E-23 P3 · 账本 E2E-F-160：`Kafka.MaxRetries` 的默认值是"幽灵契约"。
//
// 🔴 缺陷形态（注释承诺 ≠ 代码实现）：
//   struct 字段注释写着"消费失败最大重试次数，与 ai-svc 对齐（默认 3）"
//   （internal/config/config.go:28-30），但 `SetDefaults` **没有该分支**
//   ⇒ 从 SetDefaults 出来的 Config 里 MaxRetries 恒为 0。
//   实际生效的 3 来自 `internal/kafka/consumer.go:69,71` 的**硬编码**，
//   与配置系统完全无关。
//
// 后果（为什么必须在接热更之前修）：
//   1. 配的值"看起来生效"实则由硬编码决定 —— 改 yaml 改不动；
//   2. main.go 的守卫是 `if c.Kafka.MaxRetries > 0` ⇒ 推 0（想关重试）
//      会被**静默忽略**，且没有任何日志；
//   3. 与 ai-svc 不一致：ai-svc 的 `SetDefaults` 里**有**这个分支
//      （internal/config/config.go:143-145）⇒ 同名字段两个服务两套行为。
//
// 本测试锁定"SetDefaults 之后 MaxRetries == 3"这条契约。

func TestSetDefaults_KafkaMaxRetriesDefaultsTo3(t *testing.T) {
	t.Parallel()

	c := Config{}
	SetDefaults(&c)

	assert.Equal(t, 3, c.Kafka.MaxRetries,
		"SetDefaults 必须把 MaxRetries 填成 3 —— struct 注释已承诺该默认值；"+
			"缺失会让它恒为 0，实际值退化成 consumer.go 的硬编码（配置形同虚设）")
}

// TestSetDefaults_MaxRetriesZeroIsFilled 锁住"零值即未设置"的语义：
// SetDefaults 先跑、yaml 后覆盖（E2E-17 修的顺序），所以 0 会被填成 3。
func TestSetDefaults_MaxRetriesZeroIsFilled(t *testing.T) {
	t.Parallel()

	c := Config{}
	c.Kafka.MaxRetries = 0
	SetDefaults(&c)

	require.Equal(t, 3, c.Kafka.MaxRetries,
		"零值应被填为默认 3（与 SetDefaults 先跑 / yaml 后覆盖的顺序一致）")
}

// TestSetDefaults_MaxRetriesNonZeroPreserved 非零值不得被默认值覆盖。
func TestSetDefaults_MaxRetriesNonZeroPreserved(t *testing.T) {
	t.Parallel()

	for _, v := range []int{1, 5, 10} {
		c := Config{}
		c.Kafka.MaxRetries = v
		SetDefaults(&c)
		assert.Equal(t, v, c.Kafka.MaxRetries,
			"显式配置的值必须原样保留，默认值不得覆盖它")
	}
}

// TestAnalyticsAndAi_MaxRetriesDefaultsAligned 两个服务对同名字段
// 必须有相同默认值（E2E-F-160 记录的不一致隐患）。
//
// 这里只断言 analytics 侧取 3；ai-svc 的同值由其自身包的测试覆盖，
// 本条锁的是"analytics 补齐后与 ai-svc 对齐"这一事实不再漂移。
func TestAnalyticsAndAi_MaxRetriesDefaultsAligned(t *testing.T) {
	t.Parallel()

	c := Config{}
	SetDefaults(&c)
	assert.Equal(t, 3, c.Kafka.MaxRetries,
		"补齐后 analytics 的默认值应与 ai-svc 的 SetDefaults（默认 3）一致")
}
