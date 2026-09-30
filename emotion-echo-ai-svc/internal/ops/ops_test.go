package ops

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// E2E-23 测试点 #31（D-32）：ai-svc 的 9 个运营参数接入 Nacos 热更。
//
// 盘点（plan §2.4）出的 9 个候选，及其消费点形态并不统一：
//
//	| 参数 | 消费点 | 现状形态 |
//	|---|---|---|
//	| LLM.Timeout | fusion/llm_fuser.go:65 | 构造期拷贝（已修接线 F-159，但值仍不可变） |
//	| FER.Timeout | aiclient/fer.go:41 | 构造期拷贝进 struct 字段 |
//	| SenseVoice.Timeout | aiclient/sensevoice.go:37 | 同上 |
//	| XTTS.Timeout | aiclient/xtts.go:49 | 同上 |
//	| XTTS.Language | logic/synthesizespeechlogic.go:51 | **每请求现读**，但 Config 是值拷贝 |
//	| XTTS.Speed | logic/synthesizespeechlogic.go:57 | 同上 |
//	| Kafka.MaxRetries | internal/consumer/consumer.go:99 | 每条消息现读 handler 字段 |
//	| Breaker.FailThreshold | fusion/llm_breaker.go:163 | 构造期存入普通字段 |
//	| Breaker.OpenSeconds | fusion/llm_breaker.go:163 | 同上 |
//
// 两种形态都要覆盖：
//   1. **已每请求现读**的（Language/Speed/MaxRetries）—— 只需让底层值可变；
//   2. **构造期拷贝**的（四个 Timeout / 两个熔断阈值）—— 必须显式改成读容器。
//
// 一律用 atomic 而非 RWMutex：Timeout 判定在请求热路径、MaxRetries 在每条
// 消息上，RWMutex 需要每个读点都记得配 RLock，漏一处就退化回 race。

func TestOps_ApplyUpdatesAllNineParams(t *testing.T) {
	t.Parallel()

	o := New(Initial{
		LLMTimeout:           3 * time.Second,
		FERTimeout:           10 * time.Second,
		SenseVoiceTimeout:    30 * time.Second,
		XTTSTimeout:          60 * time.Second,
		XTSLanguage:          "zh-cn",
		XTSSpeed:             0.75,
		KafkaMaxRetries:      3,
		BreakerFailThreshold: 5,
		BreakerOpenSeconds:   30 * time.Second,
	})

	snap := o.Snapshot()
	assert.Equal(t, 3*time.Second, snap.LLMTimeout)
	assert.Equal(t, 10*time.Second, snap.FERTimeout)
	assert.Equal(t, 30*time.Second, snap.SenseVoiceTimeout)
	assert.Equal(t, 60*time.Second, snap.XTTSTimeout)
	assert.Equal(t, "zh-cn", snap.XTSLanguage)
	assert.Equal(t, 0.75, snap.XTSSpeed)
	assert.Equal(t, 3, snap.KafkaMaxRetries)
	assert.Equal(t, 5, snap.BreakerFailThreshold)
	assert.Equal(t, 30*time.Second, snap.BreakerOpenSeconds)

	// 模拟 Nacos 推送
	o.Apply(Config{
		LLMTimeout:           8,
		FERTimeout:           20,
		SenseVoiceTimeout:    45,
		XTTSTimeout:          90,
		XTSLanguage:          "en",
		XTSSpeed:             1.2,
		KafkaMaxRetries:      5,
		BreakerFailThreshold: 8,
		BreakerOpenSeconds:   60,
	})

	snap = o.Snapshot()
	assert.Equal(t, 8*time.Second, snap.LLMTimeout, "LLM 超时应热更")
	assert.Equal(t, 20*time.Second, snap.FERTimeout)
	assert.Equal(t, 45*time.Second, snap.SenseVoiceTimeout)
	assert.Equal(t, 90*time.Second, snap.XTTSTimeout)
	assert.Equal(t, "en", snap.XTSLanguage)
	assert.Equal(t, 1.2, snap.XTSSpeed)
	assert.Equal(t, 5, snap.KafkaMaxRetries)
	assert.Equal(t, 8, snap.BreakerFailThreshold)
	assert.Equal(t, 60*time.Second, snap.BreakerOpenSeconds)
}

// TestOps_ApplyIgnoresInvalidValues 零值/非法值必须被忽略：
// 超时 0 会让每次调用立即超时；语种空串会让 TTS 用错误语言；
// 速度 0 会让音频失真 —— 都不该被"推了个空配置"覆盖。
func TestOps_ApplyIgnoresInvalidValues(t *testing.T) {
	t.Parallel()

	o := New(Initial{
		LLMTimeout: 3 * time.Second, FERTimeout: 10 * time.Second,
		SenseVoiceTimeout: 30 * time.Second, XTTSTimeout: 60 * time.Second,
		XTSLanguage: "zh-cn", XTSSpeed: 0.75,
		KafkaMaxRetries: 3, BreakerFailThreshold: 5, BreakerOpenSeconds: 30 * time.Second,
	})

	o.Apply(Config{}) // 全零：应保持原值

	snap := o.Snapshot()
	assert.Equal(t, 3*time.Second, snap.LLMTimeout, "0 超时必须被忽略")
	assert.Equal(t, "zh-cn", snap.XTSLanguage, "空语种必须被忽略")
	assert.Equal(t, 0.75, snap.XTSSpeed, "0 速度必须被忽略")
	assert.Equal(t, 3, snap.KafkaMaxRetries, "0 重试必须被忽略")
	assert.Equal(t, 5, snap.BreakerFailThreshold, "0 阈值必须被忽略")
}

// TestOps_ApplyIsConcurrencySafe 供 CI 的 -race 复核：
// Nacos 回调写 + 业务热路径读必须无竞争。
func TestOps_ApplyIsConcurrencySafe(t *testing.T) {
	t.Parallel()

	o := New(Initial{
		LLMTimeout: 3 * time.Second, FERTimeout: 10 * time.Second,
		SenseVoiceTimeout: 30 * time.Second, XTTSTimeout: 60 * time.Second,
		XTSLanguage: "zh-cn", XTSSpeed: 0.75,
		KafkaMaxRetries: 3, BreakerFailThreshold: 5, BreakerOpenSeconds: 30 * time.Second,
	})

	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 1; i <= 300; i++ {
			o.Apply(Config{
				LLMTimeout:      i, // Config 里超时单位是秒
				FERTimeout:      i,
				KafkaMaxRetries: i,
				XTSSpeed:        0.5 + float64(i)/100,
			})
		}
		close(stop)
	}()

	for r := 0; r < 4; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = o.Snapshot()
				}
			}
		}()
	}

	wg.Wait()
	assert.NotNil(t, o)
}
