// Package ops — ai-svc 运营参数的并发安全容器（E2E-23 E 组 / D-32）
//
// 9 个参数来自 plan §2.4 的实测盘点，全部取自各自 yaml 已有配置项，
// 不新编业务概念。分两类消费点：
//
//	① 构造期拷贝（四个 Timeout / 两个熔断阈值）—— 此前值被拷进普通字段，
//	   改配置不生效；现由消费点直接读容器。
//	② 已每请求/每消息现读（XTTS.Language / Speed / Kafka.MaxRetries）——
//	   只需让底层值可变。
//
// 一律用 sync/atomic：超时判定在请求热路径、MaxRetries 在每条消息上，
// RWMutex 需要每个读点都记得配 RLock，漏一处就退化回 data race
// （E2E-21 教训：单测全绿也照样是 bug）。
//
// 零值策略：**一律忽略**。超时的 0 会让调用立即超时、语种的空串会让 TTS
// 用错语言、速度的 0 会让音频失真、MaxRetries 的 0 是"永不重试"——
// 都不属于"安全的默认值"，因此宁可保持不变（运维误推空配置不应致障）。
package ops

import (
	"sync/atomic"
	"time"
)

// Config 是 ops dataId（`emotion-echo-ai-svc.ops.yaml`）的**白名单**字段集。
//
// 刻意用固定结构体而非 map[string]any：未知 key 被 yaml 静默忽略，
// 这本身就是一层防御（P1 的内容层清洗见 shared/pkg/configcenter/ops_sanitize.go）。
//
// 单位：四个超时用 yaml 的**秒**（与 etc/ai-api.yaml 一致），
// OpenSeconds 同为秒；Speed 为倍率。
type Config struct {
	LLMTimeout           int     `yaml:"llm_timeout"`
	FERTimeout           int     `yaml:"fer_timeout"`
	SenseVoiceTimeout    int     `yaml:"sensevoice_timeout"`
	XTTSTimeout          int     `yaml:"xtts_timeout"`
	XTSLanguage          string  `yaml:"xtts_language"`
	XTSSpeed             float64 `yaml:"xtts_speed"`
	KafkaMaxRetries      int     `yaml:"kafka_max_retries"`
	BreakerFailThreshold int     `yaml:"breaker_fail_threshold"`
	BreakerOpenSeconds   int     `yaml:"breaker_open_seconds"`
}

// Initial 是启动时的初始值（来自 config / env，已在 main 里就绪）。
type Initial struct {
	LLMTimeout           time.Duration
	FERTimeout           time.Duration
	SenseVoiceTimeout    time.Duration
	XTTSTimeout          time.Duration
	XTSLanguage          string
	XTSSpeed             float64
	KafkaMaxRetries      int
	BreakerFailThreshold int
	BreakerOpenSeconds   time.Duration
}

// Snapshot 是读取侧的不可变副本。
type Snapshot struct {
	LLMTimeout           time.Duration
	FERTimeout           time.Duration
	SenseVoiceTimeout    time.Duration
	XTTSTimeout          time.Duration
	XTSLanguage          string
	XTSSpeed             float64
	KafkaMaxRetries      int
	BreakerFailThreshold int
	BreakerOpenSeconds   time.Duration
}

// Ops 是并发安全的运营参数容器。
type Ops struct {
	llmTimeout           atomic.Int64 // time.Duration 纳秒
	ferTimeout           atomic.Int64
	senseVoiceTimeout    atomic.Int64
	xttsTimeout          atomic.Int64
	xttsLanguage         atomic.Value // string
	xttsSpeed            atomic.Value // float64
	kafkaMaxRetries      atomic.Int64
	breakerFailThreshold atomic.Int64
	breakerOpenSeconds   atomic.Int64
}

// New 用启动值构造。
func New(init Initial) *Ops {
	o := &Ops{}
	o.setAll(Init2Config(init))
	return o
}

// Init2Config 把启动值转成 Config 形态（复用同一套合法性判定）。
func Init2Config(i Initial) Config {
	return Config{
		LLMTimeout:           int(i.LLMTimeout / time.Second),
		FERTimeout:           int(i.FERTimeout / time.Second),
		SenseVoiceTimeout:    int(i.SenseVoiceTimeout / time.Second),
		XTTSTimeout:          int(i.XTTSTimeout / time.Second),
		XTSLanguage:          i.XTSLanguage,
		XTSSpeed:             i.XTSSpeed,
		KafkaMaxRetries:      i.KafkaMaxRetries,
		BreakerFailThreshold: i.BreakerFailThreshold,
		BreakerOpenSeconds:   int(i.BreakerOpenSeconds / time.Second),
	}
}

func (o *Ops) setAll(c Config) {
	if c.LLMTimeout > 0 {
		o.llmTimeout.Store(int64(time.Duration(c.LLMTimeout) * time.Second))
	}
	if c.FERTimeout > 0 {
		o.ferTimeout.Store(int64(time.Duration(c.FERTimeout) * time.Second))
	}
	if c.SenseVoiceTimeout > 0 {
		o.senseVoiceTimeout.Store(int64(time.Duration(c.SenseVoiceTimeout) * time.Second))
	}
	if c.XTTSTimeout > 0 {
		o.xttsTimeout.Store(int64(time.Duration(c.XTTSTimeout) * time.Second))
	}
	if c.XTSLanguage != "" {
		o.xttsLanguage.Store(c.XTSLanguage)
	}
	if c.XTSSpeed > 0 {
		o.xttsSpeed.Store(c.XTSSpeed)
	}
	if c.KafkaMaxRetries > 0 {
		o.kafkaMaxRetries.Store(int64(c.KafkaMaxRetries))
	}
	if c.BreakerFailThreshold > 0 {
		o.breakerFailThreshold.Store(int64(c.BreakerFailThreshold))
	}
	if c.BreakerOpenSeconds > 0 {
		o.breakerOpenSeconds.Store(int64(time.Duration(c.BreakerOpenSeconds) * time.Second))
	}
}

// Apply 应用来自 Nacos 的新值；零值/非法值被忽略。
func (o *Ops) Apply(c Config) { o.setAll(c) }

// Snapshot 返回当前值的副本。
func (o *Ops) Snapshot() Snapshot {
	if o == nil {
		return Snapshot{}
	}
	lang, _ := o.xttsLanguage.Load().(string)
	speed, _ := o.xttsSpeed.Load().(float64)
	return Snapshot{
		LLMTimeout:           time.Duration(o.llmTimeout.Load()),
		FERTimeout:           time.Duration(o.ferTimeout.Load()),
		SenseVoiceTimeout:    time.Duration(o.senseVoiceTimeout.Load()),
		XTTSTimeout:          time.Duration(o.xttsTimeout.Load()),
		XTSLanguage:          lang,
		XTSSpeed:             speed,
		KafkaMaxRetries:      int(o.kafkaMaxRetries.Load()),
		BreakerFailThreshold: int(o.breakerFailThreshold.Load()),
		BreakerOpenSeconds:   time.Duration(o.breakerOpenSeconds.Load()),
	}
}
