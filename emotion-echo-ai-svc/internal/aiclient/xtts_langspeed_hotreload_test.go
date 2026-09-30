package aiclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// E2E-23 #31：XTTS 的语种与语速可热更。
//
// 这两个参数的消费点形态与超时不同：`synthesizespeechlogic.go` **每请求**
// 读 `l.svcCtx.Config.XTTS.Language`，看似"已现读"，但 Config 是**值类型**
// （`servicecontext.go` 里 `Config config.Config`），main 那份改不到它
// ⇒ 实际仍是冻结的。故同样需要钩子。
//
// 本测试通过**抓服务端收到的请求体**来验证真值，而不是只看客户端字段 ——
// 后者只能证明"字段被改了"，证明不了"请求里用的是新值"。

func TestXTTSClient_LanguageAndSpeedHotReload(t *testing.T) {
	t.Parallel()

	type reqBody struct {
		Text     string  `json:"text"`
		Language string  `json:"language"`
		Speed    float64 `json:"speed"`
	}
	var got atomic.Value // reqBody

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b reqBody
		_ = json.NewDecoder(r.Body).Decode(&b)
		got.Store(b)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"audio":"AAECAw==","sample_rate":24000}`))
	}))
	defer srv.Close()

	var lang atomic.Value
	lang.Store("zh-cn")
	var speed atomic.Value
	speed.Store(0.75)

	c := NewXTTSClient(Config{BaseURL: srv.URL, Timeout: 30}, "zh-cn", 0.75)
	require.NotNil(t, c)
	c.LanguageFn = func() string { return lang.Load().(string) }
	c.SpeedFn = func() float64 { return speed.Load().(float64) }

	// 基线
	_, _, err := c.Synthesize(context.Background(), "你好")
	require.NoError(t, err)
	b := got.Load().(reqBody)
	assert.Equal(t, "zh-cn", b.Language)
	assert.InDelta(t, 0.75, b.Speed, 1e-9)

	// 热更语种与语速
	lang.Store("en")
	speed.Store(1.25)

	_, _, err = c.Synthesize(context.Background(), "hello")
	require.NoError(t, err)
	b = got.Load().(reqBody)
	assert.Equal(t, "en", b.Language, "服务端收到的语种必须是热更后的值")
	assert.InDelta(t, 1.25, b.Speed, 1e-9, "服务端收到的语速必须是热更后的值")
}

// TestXTTSClient_NilHooks_KeepConstructorValues 向后兼容。
func TestXTTSClient_NilHooks_KeepConstructorValues(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"audio":"AAECAw==","sample_rate":24000}`))
	}))
	defer srv.Close()

	c := NewXTTSClient(Config{BaseURL: srv.URL, Timeout: 30}, "ja", 0.9)
	require.NotNil(t, c)

	assert.Equal(t, "ja", c.effectiveLanguage())
	assert.InDelta(t, 0.9, c.effectiveSpeed(), 1e-9)
	_ = time.Second
}
