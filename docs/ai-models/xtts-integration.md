# XTTS 接云端 TTS API 实施指南

**日期**：2026-07-17
**目的**：替换本地 XTTS 容器，改为调云端 TTS API
**预计工作量**：1 小时（实现 + 测试）

---

## 一、为什么放弃本地 XTTS

详细见 `docs/stage-25-final-summary.md`。简述：

- Coqui TTS 0.22 + torch 2.13 + torchaudio 2.11 三方依赖互锁
- 反复 build 4 次失败
- 浪费时间 > 接入云端 API

**改用云端**：减少运维成本 + 更稳定 + 音色多 + 跨语言。

---

## 二、云厂商选择

| 厂商 | 中文 | 英文 | 音色数 | 价格 | 推荐场景 |
|------|------|------|--------|------|---------|
| **阿里云智能语音** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | 60+ | 0.01 元/万字符 | 中文为主 |
| **字节火山引擎** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | 100+ | 免费 30 万字符/月 | 音色多 |
| **OpenAI TTS** | ⭐⭐ | ⭐⭐⭐⭐⭐ | 6 | $15/1M 字符 | 已有 OpenAI key |
| **ElevenLabs** | ⭐⭐ | ⭐⭐⭐⭐⭐ | 20+ | 免费 10K 字符/月 | 英文情绪 |
| **腾讯云 TTS** | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | 30+ | 0.016 元/万字符 | 微信生态 |

**推荐**：中文为主选 **阿里云**；英文为主选 **ElevenLabs**；想省心选 **OpenAI**。

---

## 三、改造范围

只改 1 个文件 + 1 个配置：

1. **`emotion-echo-ai-svc/internal/aiclient/xtts.go`** — 改 `Synthesize` 实现，路由到不同 provider
2. **`emotion-echo-ai-svc/etc/ai-api.yaml`** — 加 provider 配置
3. **`docker-compose.apps.yml`** — 删 `emotion-echo-xtts` 服务（不再需要）
4. **`deploy/docker-compose.apps.yml`** — AI profile 移除 xtts

---

## 四、XTTSClient 改造

### 4.1 新数据结构

```go
// TTSProvider 标识
type TTSProvider string

const (
    TTSProviderAliyun    TTSProvider = "aliyun"
    TTSProviderVolcano   TTSProvider = "volcano"
    TTSProviderOpenAI    TTSProvider = "openai"
    TTSProviderElevenLabs TTSProvider = "elevenlabs"
)

// XTTSClient 改为通用 TTS 客户端
type XTTSClient struct {
    provider TTSProvider
    apiKey   string
    voice    string
    language string
    speed    float64
    hc       *http.Client
    timeout  time.Duration
    // 各 provider 特定字段
    aliyunAKID   string  // aliyun access key id
    aliyunAKS    string  // aliyun access key secret
    aliyunAppKey string  // aliyun 智能语音 app key
    volcanoAppID string
    volcanoToken string
    openAIBase   string  // https://api.openai.com/v1
}
```

### 4.2 工厂函数

```go
// NewXTTSClient 从 Config 构造
func NewXTTSClient(c Config) *XTTSClient {
    if c.Provider == "" {
        return nil
    }
    
    timeout := c.Timeout
    if timeout <= 0 {
        timeout = 60
    }
    
    return &XTTSClient{
        provider:     TTSProvider(c.Provider),
        apiKey:       c.APIKey,
        voice:        c.Voice,
        language:     c.Language,
        speed:        c.Speed,
        hc:           &http.Client{Timeout: time.Duration(timeout) * time.Second},
        timeout:      time.Duration(timeout) * time.Second,
        aliyunAKID:   c.Aliyun.AccessKeyID,
        aliyunAKS:    c.Aliyun.AccessKeySecret,
        aliyunAppKey: c.Aliyun.AppKey,
        volcanoAppID: c.Volcano.AppID,
        volcanoToken: c.Volcano.Token,
        openAIBase:   c.OpenAI.BaseURL,
    }
}
```

### 4.3 Synthesize 路由

```go
func (c *XTTSClient) Synthesize(ctx context.Context, text string) ([]byte, int, error) {
    if c == nil {
        return nil, 0, ErrNotConfigured
    }
    if strings.TrimSpace(text) == "" {
        return nil, 0, fmt.Errorf("empty text")
    }
    
    switch c.provider {
    case TTSProviderAliyun:
        return c.synthesizeAliyun(ctx, text)
    case TTSProviderVolcano:
        return c.synthesizeVolcano(ctx, text)
    case TTSProviderOpenAI:
        return c.synthesizeOpenAI(ctx, text)
    case TTSProviderElevenLabs:
        return c.synthesizeElevenLabs(ctx, text)
    default:
        return nil, 0, fmt.Errorf("unknown TTS provider: %s", c.provider)
    }
}
```

### 4.4 阿里云实现（重点）

阿里云智能语音 NUI 平台 RESTful API（短文本 ≤ 300 字符）：

```go
func (c *XTTSClient) synthesizeAliyun(ctx context.Context, text string) ([]byte, int, error) {
    // 1. 构造请求体
    payload := map[string]any{
        "appkey":     c.aliyunAppKey,
        "token":      c.fetchAliyunToken(), // 见 4.5
        "text":       text,
        "format":     "wav",
        "sample_rate": 16000,
        "voice":      c.voice,  // e.g. "zhitian_emo"
        "volume":     50,
        "speech_rate": 0,
        "pitch_rate": 0,
    }
    
    body, _ := json.Marshal(payload)
    
    ctx, cancel := context.WithTimeout(ctx, c.timeout)
    defer cancel()
    
    req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
        "https://nls-gateway-cn-shanghai.aliyuncs.com/stream/v1/tts",
        bytes.NewReader(body))
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := c.hc.Do(req)
    if err != nil {
        return nil, 0, fmt.Errorf("call aliyun TTS: %w", err)
    }
    defer resp.Body.Close()
    
    wav, err := io.ReadAll(resp.Body)
    if err != nil {
        return nil, 0, fmt.Errorf("read aliyun response: %w", err)
    }
    
    return wav, 16000, nil  // 阿里云返回的是 WAV 字节流
}
```

### 4.5 阿里云 Token 获取

阿里云智能语音用临时 token（24h 过期），需要先用 AK/SK 换 token：

```go
// AliyunTokenResp NUI token 响应
type AliyunTokenResp struct {
    Token   string `json:"Token"`
    Expire  int64  `json:"ExpireTime"`
}

// fetchAliyunToken 缓存 token（24h 内不重复获取）
func (c *XTTSClient) fetchAliyunToken() string {
    // 简单实现：启动时获取一次，缓存到 c.aliyunAppKey
    // 进阶：使用 sync.Once + 过期自动刷新
    
    // POST https://nls-meta-cn-shanghai.aliyuncs.com/token
    //  body: {"AccessKeyId":"...", "AccessKeySecret":"..."}
    //  返回: {"Token":"...", "ExpireTime":1700000000}
    
    // 详见：https://help.aliyun.com/zh/isi/getting-started/upgrade-to-the-new-version-of-sdk
}
```

实际用阿里云官方 Go SDK 简化：

```go
import "github.com/aliyun/alibaba-cloud-sdk-go/services/nls_cloud_meta"
```

### 4.6 OpenAI 实现

```go
func (c *XTTSClient) synthesizeOpenAI(ctx context.Context, text string) ([]byte, int, error) {
    payload := map[string]any{
        "model": "tts-1",
        "input": text,
        "voice": c.voice,  // alloy / echo / fable / onyx / nova / shimmer
        "response_format": "wav",
    }
    body, _ := json.Marshal(payload)
    
    req, _ := http.NewRequestWithContext(ctx, "POST",
        c.openAIBase+"/audio/speech", bytes.NewReader(body))
    req.Header.Set("Authorization", "Bearer "+c.apiKey)
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := c.hc.Do(req)
    if err != nil {
        return nil, 0, fmt.Errorf("call openai TTS: %w", err)
    }
    defer resp.Body.Close()
    
    wav, _ := io.ReadAll(resp.Body)
    return wav, 24000, nil  // OpenAI 默认 24kHz
}
```

### 4.7 ElevenLabs 实现

```go
func (c *XTTSClient) synthesizeElevenLabs(ctx context.Context, text string) ([]byte, int, error) {
    req, _ := http.NewRequestWithContext(ctx, "POST",
        fmt.Sprintf("https://api.elevenlabs.io/v1/text-to-speech/%s", c.voice),  // voice ID
        strings.NewReader(fmt.Sprintf(`{"text":%q}`, text)))
    req.Header.Set("xi-api-key", c.apiKey)
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Accept", "audio/wav")
    
    resp, err := c.hc.Do(req)
    if err != nil {
        return nil, 0, fmt.Errorf("call elevenlabs: %w", err)
    }
    defer resp.Body.Close()
    
    wav, _ := io.ReadAll(resp.Body)
    return wav, 44100, nil  // ElevenLabs 默认 44.1kHz
}
```

### 4.8 Health 改写

```go
func (c *XTTSClient) Health(ctx context.Context) error {
    // 1. 检查 API key 是否配置
    if c == nil || c.provider == "" {
        return ErrNotConfigured
    }
    
    // 2. 探活云端（不同 provider 不同）
    switch c.provider {
    case TTSProviderAliyun:
        // 调 GET /voices 探活
        req, _ := http.NewRequestWithContext(ctx, "GET",
            "https://nls-gateway-cn-shanghai.aliyuncs.com/stream/v1/tts", nil)
        req.Header.Set("appkey", c.aliyunAppKey)
        return c.doHealthReq(req)
    case TTSProviderOpenAI:
        req, _ := http.NewRequestWithContext(ctx, "GET",
            c.openAIBase+"/models", nil)
        req.Header.Set("Authorization", "Bearer "+c.apiKey)
        return c.doHealthReq(req)
    // ... 其他 provider
    }
    return nil
}

func (c *XTTSClient) doHealthReq(req *http.Request) error {
    ctx, cancel := context.WithTimeout(req.Context(), 5*time.Second)
    defer cancel()
    req = req.WithContext(ctx)
    
    resp, err := c.hc.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    if resp.StatusCode >= 500 {
        return fmt.Errorf("TTS provider unhealthy: %d", resp.StatusCode)
    }
    return nil
}
```

---

## 五、Config 配置

### 5.1 新结构（`emotion-echo-ai-svc/etc/ai-api.yaml`）

```yaml
TTS:
  # Provider: aliyun / volcano / openai / elevenlabs
  Provider: aliyun
  Timeout: 60          # seconds
  Language: zh-cn
  Speed: 0.0           # 0 = 默认
  
  Aliyun:
    AppKey: ${ALIYUN_APPKEY}
    AccessKeyID: ${ALIYUN_ACCESS_KEY_ID}
    AccessKeySecret: ${ALIYUN_ACCESS_KEY_SECRET}
    Voice: zhitian_emo   # 智甜·情感女声
  
  Volcano:
    AppID: ${VOLCANO_APP_ID}
    Token: ${VOLCANO_TOKEN}
    Voice: BV001_streaming
  
  OpenAI:
    ApiKey: ${OPENAI_API_KEY}
    BaseURL: https://api.openai.com/v1
    Voice: alloy
  
  ElevenLabs:
    ApiKey: ${ELEVENLABS_API_KEY}
    Voice: 21m00Tcm4TlvDq8ikWAM  # Rachel
```

### 5.2 AI 服务配置同步更新

`internal/config/config.go` 加新结构：

```go
type TTSConfig struct {
    Provider          string         `yaml:"provider"`
    Timeout           int            `yaml:"timeout"`
    Language          string         `yaml:"language"`
    Speed             float64        `yaml:"speed"`
    Aliyun            AliyunConfig   `yaml:"aliyun"`
    Volcano           VolcanoConfig  `yaml:"volcano"`
    OpenAI            OpenAIConfig   `yaml:"openai"`
    ElevenLabs        ElevenLabsConfig `yaml:"elevenlabs"`
}

type AliyunConfig struct {
    AppKey          string `yaml:"appkey"`
    AccessKeyID     string `yaml:"access_key_id"`
    AccessKeySecret string `yaml:"access_key_secret"`
    Voice           string `yaml:"voice"`
}

// ... 其他 provider 配置
```

---

## 六、docker-compose 清理

### 6.1 删 XTTS 服务（`deploy/docker-compose.apps.yml`）

```yaml
# 删掉整个 emotion-echo-xtts service（290-249 行）
```

### 6.2 改 ai-svc 配置

`emotion-echo-ai-svc/etc/ai-api.yaml`：

```yaml
TTS:
  Provider: aliyun
  Aliyun:
    AppKey: ${ALIYUN_APPKEY}
    ...
```

启动后 ai-svc 自动调阿里云 TTS，**无需 XTTS 容器**。

---

## 七、单元测试

加 `emotion-echo-ai-svc/internal/aiclient/xtts_cloud_test.go`：

```go
func TestXTTS_AliyunProvider_Success(t *testing.T) {
    server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "audio/wav")
        w.Write([]byte("RIFF...."))  // 模拟 WAV
    }))
    defer server.Close()
    
    c := &XTTSClient{
        provider: TTSProviderAliyun,
        aliyunAppKey: "test-key",
        hc: &http.Client{Timeout: 5*time.Second},
    }
    // 注入 mock URL
    c.aliyunBase = server.URL  // 需要加一个 baseURL 字段
    
    wav, sr, err := c.Synthesize(context.Background(), "你好")
    assert.NoError(t, err)
    assert.Equal(t, 16000, sr)
    assert.NotEmpty(t, wav)
}
```

按 AGENTS.md 强约束：**先写测试** → 看失败 → 实现 → 看通过。

---

## 八、安全注意事项

### 8.1 API key 管理

**不要把 API key 写进 git**！用环境变量：

```bash
# .env.local (gitignored)
ALIYUN_APPKEY=xxx
ALIYUN_ACCESS_KEY_ID=xxx
ALIYUN_ACCESS_KEY_SECRET=xxx

# docker-compose 引用
environment:
  - ALIYUN_APPKEY=${ALIYUN_APPKEY}
```

### 8.2 HTTPS 必须

所有云端 API 都用 HTTPS，**不要** 调 HTTP（防 MITM）。

### 8.3 Rate limiting

每家云厂商有 QPS 限制：
- 阿里云智能语音：默认 20 QPS（可申请提高）
- OpenAI TTS：50 RPM（每分钟 50 次）

ai-svc 的限流中间件（`shared/pkg/middleware/limiter.go`）已 per-user 限流 10 QPS，应该足够。

### 8.4 内容审核

云端 API 通常**自带内容审核**（违规文本会返 400），不用自己写。

---

## 九、成本估算

| 月活 | 中文 TTS 字符/月 | 阿里云 | 火山 | OpenAI | ElevenLabs |
|------|---------------|--------|------|--------|------------|
| 100 | 10万 | 0.001 元 | 0 | $1.5 | $15 |
| 1000 | 100万 | 0.1 元 | 0 | $15 | $150 |
| 1万 | 1000万 | 10 元 | 0 | $150 | - |
| 10万 | 1亿 | 100 元 | 0 | $1500 | - |

**国内项目推荐阿里云**（中文音色好 + 便宜 + 备案方便）。

---

## 十、回滚方案

如果云端 API 不可用（限流 / 服务商故障），回退方案：

1. **临时禁用 ai-svc 的 TTS endpoint**（前端显示"暂不可用"）
2. **降级到本地 XTTS**：用 `PyTorch 2.0.1 + torchaudio 2.0.2`（与 TTS 0.22 兼容，**不引新 wheel**）—— 镜像大小约 12 GB
3. **多 provider 备份**：在 Config 加 fallback chain（`Primary: aliyun, Fallback: openai`）

---

## 十一、相关文档

- `docs/stage-25-final-summary.md` — Stage 25 总结
- `docs/stage-25-final-landing.md` — 项目当前状态
- `emotion-echo-ai-svc/internal/aiclient/xtts.go` — 当前实现
- `emotion-echo-ai-svc/etc/ai-api.yaml` — 当前配置

---

## 十二、下一步实施清单

按 AGENTS.md TDD 流程：

- [ ] 1. 🔴 RED：写 `xtts_cloud_test.go`，4 个测试（4 provider 各 1 个）
- [ ] 2. 🟢 GREEN：实现 `Synthesize` 路由到 4 个 provider
- [ ] 3. 🟢 GREEN：更新 `Config` 结构 + `ai-api.yaml`
- [ ] 4. ♻️ REFACTOR：删 `docker-compose.apps.yml` 的 XTTS service
- [ ] 5. ♻️ REFACTOR：跑 `python scripts/verify_stage23_endpoints.py` 期望 `4/4 [AI profile LIVE]`
- [ ] 6. 📝 写 `docs/xtts-cloud-api-integration.md` 部署笔记
- [ ] 7. commit + push

**预计时间**：1 小时
**风险**：中（涉及 API key 配置 + 多 provider 兼容性）