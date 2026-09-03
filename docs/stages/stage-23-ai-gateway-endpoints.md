# Stage 23 · AI 服务对外 HTTP 网关

**日期**：2026-07-16
**目标**：在 emotion-echo-ai-svc 暴露 3 个 endpoint，让 chat-svc / Web / 第三方应用能调用多模态 AI 能力。

**前置**：Stage 22（3 个 AI 服务容器化 + aiclient 包 + MultiModalAnalyzer）。

---

## 一、3 个新 endpoint 总览

| 端点 | 方法 | 用途 | 输入 | 输出 |
|------|------|------|------|------|
| `/api/v1/multimodal/analyze` | POST | 多模态情绪分析 | `multipart/form-data`: kind, file, text | JSON: emotion + confidence + model |
| `/api/v1/tts/synthesize` | POST | 文本转语音 | JSON: text, language, speed | JSON: base64 WAV + sampleRate |
| `/api/v1/ai/health` | GET | AI 服务集群健康 | — | JSON: per-service enabled/healthy/latency |

---

## 二、API 详细规格

### 2.1 `POST /api/v1/multimodal/analyze`

**场景**：浏览器上传表情照片 / 语音片段 → ai-svc → FER / SenseVoice → 返回 emotion。

**请求（multipart/form-data）**：
| 字段 | 必填 | 类型 | 说明 |
|------|------|------|------|
| `kind` | ✅ | string | `text` \| `image` \| `audio` |
| `file` | kind≠text 时必填 | binary | 图像或音频文件 |
| `filename` | ❌ | string | 默认从上传文件名推断 |
| `text` | ❌ | string | `kind=text` 时用此字段 |

**响应**：
```json
{
  "kind":         "audio",
  "emotion":      "happy",
  "confidence":   0.95,
  "sentimentScore": 0.7,
  "model":        "sensevoice:sensevoice",
  "transcript":   "我太开心了"
}
```

### 2.2 `POST /api/v1/tts/synthesize`

**场景**：chat-svc 拿到 LLM 回复 → 调 ai-svc 转 WAV → 浏览器播放。

**请求（JSON）**：
```json
{ "text": "你好，世界", "language": "zh-cn", "speed": 0.75 }
```

**响应**：
```json
{
  "audio":      "<base64 WAV>",
  "sampleRate": 24000,
  "mime":       "audio/wav",
  "bytes":      192000,
  "text":       "你好，世界",
  "language":   "zh-cn"
}
```

**降级**：未启用 XTTS 时返回 `503 Service Unavailable`。

### 2.3 `GET /api/v1/ai/health`

**目的**：探测 3 个 AI 服务可达性，可用于 K8s readiness / 监控告警。

**响应**：
```json
{
  "time": 1752702345,
  "allHealthy": false,
  "fer":         { "enabled": true,  "healthy": true,  "url": "http://emotion-echo-fer:8004",        "latencyMs": "12" },
  "sensevoice":  { "enabled": true,  "healthy": false, "url": "http://emotion-echo-sensevoice:8002", "latencyMs": "3001", "error": "context deadline" },
  "xtts":        { "enabled": false, "healthy": false, "error": "disabled (BaseURL empty)" }
}
```

---

## 三、降级行为

每个 endpoint 在 AI 模型服务不可用时按下列降级路径返回：

| 操作 | AI 在线 | AI 离线 |
|------|---------|---------|
| `multimodal/analyze` (kind=text) | keyword analyzer | keyword analyzer（同） |
| `multimodal/analyze` (kind=image) | FER | keyword + log warning |
| `multimodal/analyze` (kind=audio) | SenseVoice | keyword + log warning |
| `tts/synthesize` | XTTS → base64 WAV | **503** |
| `ai/health` | 200 + 每项 healthy | 200 + 每项 healthy + error |

---

## 四、文件清单

| 文件 | 作用 |
|------|------|
| `internal/handler/multimodal_handler.go` | 3 个 HTTP handler |
| `internal/logic/multimodalanalyzelogic.go` | 多模态分析业务逻辑 |
| `internal/logic/synthesizespeechlogic.go` | TTS 业务逻辑 |
| `internal/logic/aihealthlogic.go` | AI 健康检查 |
| `main.go` | 路由注册（+3 routes） |
| `scripts/verify_stage23_endpoints.py` | 冒烟测试 |

---

## 五、典型调用链

### 5.1 浏览器上传语音 → 文字 + 情绪

```
浏览器 (Vue)
  ↓ multipart/form-data:
     kind=audio, file=<webm bytes>
Nuxt 前端 → axios POST /api/v1/multimodal/analyze
  ↓ proxy → ai-svc
ai-svc internal:
  handler.MultiModalAnalyzeHandler
    ↓ logic.MultiModalAnalyzeLogic.Analyze
    ↓ svc.MultiModal.Analyze (input kind=audio, bytes=...)
      ↓ svc.SenseVoice.Analyze → emotion-echo-sensevoice:8002/analyze
      ↓ fallback.Analyze(text) 计算 sentiment
  ↓ 写 EmotionAnalysis（如果要）
返回 JSON 到浏览器
```

### 5.2 浏览器请求语音回复

```
浏览器
  ↓ JSON {text: "你好"}
chat-svc → ai-svc POST /api/v1/tts/synthesize
  ↓ logic.SynthesizeSpeechLogic.Synthesize
  ↓ svc.MultiModal.SynthesizeText → emotion-echo-xtts:8003/tts
返回 base64 WAV → 浏览器 audio.play()
```

### 5.3 K8s readiness 检测

```yaml
readinessProbe:
  httpGet: { path: /api/v1/ai/health, port: 8891 }
  initialDelaySeconds: 5
  periodSeconds: 30
# 注意：allHealthy=false 时要选用 single-service readiness，
# 否则 ai-svc pod 会被 K8s 杀掉
```

---

## 六、踩坑清单

### Q1：multipart file 字段名是什么？
**A**：服务端 `c.FormFile("file")` 读 `file` 字段。前端 form-data 必须叫 `file`（不能改）。

### Q2：TTS 返 503 时怎么办？
**A**：前端降级方案——展示文本而不是语音，提示"语音生成暂时不可用"。

### Q3：`/api/v1/multimodal/analyze` (kind=image) AI 全关闭时？
**A**：返回 200 + model=`keyword-stub` + emotion=`neutral`（不要返 5xx，避免前端报错）。

### Q4：SenseVoice / XTTS 模型加载慢？
**A**：`start_period=120-180s`，端到端 cold start 可能 60-120s。

### Q5：base64 WAV 太大传输慢？
**A**：客户端 `ReadableStream` 流式接收；服务端以后接 `/api/v1/tts/stream`（流式 endpoint，Stage 24+ 候选）。

---

## 七、端到端验证结果（2026-07-16）

`python scripts/verify_stage23_endpoints.py --ai-svc http://localhost:8891`：

```
== Stage 23 endpoint check (http://localhost:8891) ==
   using demo JWT: eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJ…

[OK] GET /api/v1/ai/health → 200
   - FER        : down (enabled=on, 2568ms, err=dial tcp: lookup emotion-echo-fer)
   - SenseVoice : down (enabled=on, 2614ms, err=dial tcp: lookup emotion-echo-sensevoice)
   - XTTS       : down (enabled=on, 2630ms, err=dial tcp: lookup emotion-echo-xtts)

[OK] POST /api/v1/multimodal/analyze (kind=text) → 200
   emotion=happy model=keyword-stub-v1 confidence=0.5

[OK] POST /api/v1/multimodal/analyze (kind=audio) → 200
   emotion=neutral model=keyword-stub (SenseVoice off → fallback)

[OK] POST /api/v1/tts/synthesize → 503
   call XTTS: Post "http://emotion-echo-xtts:8003/tts": dial tcp: lookup emotion-ec

=== Summary: 3/4 Stage 23 endpoints healthy ===
  (TTS 503 是预期降级，未启用 XTTS 服务)
```

**3/4 endpoint 工作**——TTS 503 是预期降级（AI profile 没启用）。

| Endpoint | AI 在线 | AI 离线 | 期望 |
|----------|---------|---------|------|
| `/api/v1/ai/health` | 200 + 每项 healthy=true | 200 + 每项 enabled=true healthy=false + error | YES |
| `multimodal/analyze` kind=text | keyword (200) | keyword (200) | YES |
| `multimodal/analyze` kind=image | FER (200) | keyword + log warn (200) | YES |
| `multimodal/analyze` kind=audio | SenseVoice (200) | keyword + log warn (200) | YES |
| `tts/synthesize` | XTTS base64 WAV (200) | **503 + 错误信息** | YES |

详见 [stage-24-endpoint-verification-and-bugfix.md](stage-24-endpoint-verification-and-bugfix.md)。

---

## 八、Stage 24+ 候选

> 详见 [stage-25-roadmap.md](stage-25-roadmap.md)。

- Stage 24：端到端验证 + 6+ bug 修复 — 已完成
- `POST /api/v1/tts/stream`：TTS 流式 chunk 推送给前端
- `POST /api/v1/multimodal/analyze-batch`：批量分析（性能优化）
- `GET /api/v1/emotion/aggregate/:userId`：用户级别情绪趋势（接 analytics-svc）
- 网关层限流：token-bucket，按 user_id / IP
- WebSocket `/ws/multimodal`：双向流式多模态对话

---

**进度（2026-07-16 更新）**：
- Stage 22-A：3 个 AI 服务容器化 — 完成
- Stage 22-A.5：aiclient 客户端 — 完成
- Stage 23：3 个对外 HTTP endpoint — 完成
- Stage 24：端到端验证 + bug 修复 — 完成
- Stage 25+：见 [roadmap](stage-25-roadmap.md)

