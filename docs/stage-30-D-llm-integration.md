# Stage 30-D：接真实 LLM（Phase D 文档落地）

> **对应 commit**：
> - `73a3b1a` feat(web-bff): 接 DeepSeek/OpenAI 兼容 LLM（ai_stream 真实对话流）
> - 关联：`1bff49d` OpenAI 兼容 SSE 格式 + `7fe66a4` 双格式兼容（见 `stage-30-C-browser-testing.md`）
>
> **背景**：项目原用 mock 共情回复（`mockEmpathyReply` 按关键词产出固定话术），无法覆盖真实 LLM 互动。本阶段把 ai_stream handler 改造为调真实 LLM，APIKey 通过 env 注入（**key 不进 git**）。

---

## 一、架构

```
浏览器 /api/v1/ai/stream
  └─→ emotion-echo-web-bff :8894 (ai_stream handler)
        ├─ APIKey 非空 → downstream.LLMClient → POST {BFF_LLM_BASE_URL}/v1/chat/completions
        │                       (OpenAI 兼容流式 SSE)
        │                       → DeepSeek / OpenAI / Kimi / Ollama 任何 OpenAI 兼容服务
        └─ APIKey 空 → mock 共情回复（dev fallback，保留开发友好性）
```

---

## 二、新增/修改文件

| 文件 | 内容 |
|------|------|
| `internal/downstream/llm.go` (150 行) | `LLMClient`（POST `/v1/chat/completions` + SSE 解析 + onDelta 回调） |
| `internal/downstream/llm_test.go` (170 行) | 5 个单测（流式 / 4xx / 无 key / 空 model / ctx cancel） |
| `internal/config/config.go` | `LLM{BaseURL/Model/Timeout}` + ApplyEnvOverrides 加 `BFF_LLM_{API_KEY,BASE_URL,MODEL}` |
| `etc/web-bff.yaml` | LLM 段（BaseURL 默认 `https://api.deepseek.com`，Model `deepseek-chat`） |
| `internal/handler/ai_stream_handler.go` | APIKey 非空 → 调真实 LLM（system prompt 温柔共情 2-3 句）；空 → mock fallback；LLM 失败 → 降级提示 |
| `main.go` | `NewAIStreamHandler(*c)` 注入 config |

---

## 三、LLMClient 关键设计

### 3.1 接口契约

```go
type LLMChatReq struct {
    Model    string    `json:"model"`
    Messages []Message `json:"messages"`
    Stream   bool      `json:"stream,omitempty"`
}

type Message struct {
    Role    string `json:"role"`    // "system" | "user" | "assistant"
    Content string `json:"content"`
}

// 流式回调：LLMClient 把每块 content 推给 caller
type LLMClient interface {
    ChatStream(ctx context.Context, req LLMChatReq, onDelta func(content string)) error
}
```

### 3.2 SSE 解析

OpenAI 兼容 SSE 格式：

```
data: {"choices":[{"delta":{"content":"你"}}]}
data: {"choices":[{"delta":{"content":"好"}}]}
...
data: {"choices":[{"delta":{"content":"世界"},"finish_reason":"stop"}]}
data: [DONE]
```

实现：

```go
scanner := bufio.NewScanner(resp.Body)
for scanner.Scan() {
    line := scanner.Text()
    if !strings.HasPrefix(line, "data: ") { continue }
    payload := strings.TrimSpace(strings.TrimPrefix(line, "data: "))
    if payload == "" || payload == "[DONE]" { break }
    var d LLMDelta
    json.Unmarshal([]byte(payload), &d)
    for _, c := range d.Choices {
        if c.Delta.Content != "" {
            onDelta(c.Delta.Content)
        }
        if c.FinishReason == "stop" { return nil }
    }
}
```

### 3.3 配置选项

```go
type LLMOptions struct {
    BaseURL string  // e.g. "https://api.deepseek.com" / "https://api.openai.com" / "http://localhost:11434"（Ollama）
    APIKey  string  // sk-...（Ollama 留空）
    Model   string  // "deepseek-chat" / "gpt-4o-mini" / "qwen-turbo" / "llama3"
    Timeout time.Duration
}
```

默认值（NewLLMClient）：
- BaseURL = "https://api.deepseek.com"
- Model = "deepseek-chat"
- Timeout = 60s

### 3.4 错误处理

```go
if resp.StatusCode >= 400 {
    body, _ := io.ReadAll(resp.Body)
    return &APIError{StatusCode: resp.StatusCode, Msg: fmt.Sprintf("llm: upstream %d: %s", resp.StatusCode, string(body))}
}
```

`APIError` 已有，handler 层 `statusFor(err)` 自动透传状态码给前端。

---

## 四、ai_stream handler 改造

```go
func (h *AIStreamHandler) ServeHTTP(c *gin.Context) {
    c.Header("Content-Type", "text/event-stream")
    c.Header("X-Accel-Buffering", "no")
    c.Header("Cache-Control", "no-cache")
    c.Header("Connection", "keep-alive")

    var req aiStreamReq  // 双格式（OpenAI messages 或 {message, emotion, conversationId}）
    json.NewDecoder(c.Request.Body).Decode(&req)

    userContent := req.Message  // 优先用 req.Message（前端发消息格式）
    if userContent == "" {
        for i := len(req.Messages) - 1; i >= 0; i-- {
            if req.Messages[i].Role == "user" {
                userContent = req.Messages[i].Content
                break
            }
        }
    }

    flusher, _ := c.Writer.(http.Flusher)
    writeDelta := func(content string) error { /* OpenAI SSE */ }

    // 真实 LLM 路径
    if h.cfg.LLM.APIKey != "" {
        llmMessages := []downstream.Message{
            {Role: "system", Content: "你是一个温柔、共情的情绪疏导陪伴者。用中文简短回应（2-3 句话），表达理解、不评判、鼓励继续说。"},
            {Role: "user", Content: userContent},
        }
        llmClient := downstream.NewLLMClient(downstream.LLMOptions{
            BaseURL: h.cfg.LLM.BaseURL, APIKey: h.cfg.LLM.APIKey,
            Model: h.cfg.LLM.Model, Timeout: time.Duration(h.cfg.LLM.Timeout) * time.Second,
        })
        ctx, cancel := context.WithCancel(c.Request.Context())
        defer cancel()
        err := llmClient.ChatStream(ctx, downstream.LLMChatReq{Model: h.cfg.LLM.Model, Messages: llmMessages},
            func(content string) { if writeErr := writeDelta(content); writeErr != nil { cancel() } })
        if err != nil { writeDelta("\n\n[抱歉，AI 服务暂时不可用]") }
        io.WriteString(c.Writer, "data: [DONE]\n\n")
        return
    }

    // Fallback：mock 共情回复（APIKey 为空）
    reply := mockEmpathyReply(userContent)
    // ... rune 分块输出
}
```

---

## 五、配置：env 注入（key 安全）

`deploy/docker-compose.apps.yml`：

```yaml
emotion-echo-web-bff:
  environment:
    # Stage 30-D：真实 LLM（DeepSeek/OpenAI 兼容）。dev 留空 → ai_stream 走 mock。
    # 生产部署：写 deploy/env/.env.local（**git ignore**），不要 commit 真实 key
    BFF_LLM_API_KEY:     ${BFF_LLM_API_KEY:-}
    BFF_LLM_BASE_URL:    ${BFF_LLM_BASE_URL:-https://api.deepseek.com}
    BFF_LLM_MODEL:       ${BFF_LLM_MODEL:-deepseek-chat}
```

**关键**：yaml 仅占位（`${:-}` 默认空），真实 key 通过部署时 env 注入。`git grep` 历史中无 key（部署时设临时 env，跑完立即 unset）。

部署命令：
```bash
BFF_LLM_API_KEY=sk-dd6f7d... docker compose up -d emotion-echo-web-bff
```

---

## 六、5 个 LLMClient 单测

| 测试 | 验证 |
|------|------|
| `TestLLMClient_ChatStream_DeltaContent` | SSE 流解析（多块拼接 + [DONE]）+ 请求路径 + Authorization |
| `TestLLMClient_ChatStream_Upstream4xx_ReturnsAPIError` | 401 → 返回 APIError{StatusCode:401}，handler 透传状态码 |
| `TestLLMClient_ChatStream_NoAPIKey_NoAuth` | Ollama 模式：APIKey 空 → 不发 Authorization（前端可直连本地 LLM） |
| `TestLLMClient_ChatStream_EmptyModel_UsesClientDefault` | 请求 model 空 → 用 client 默认 model（前端 useAIStream 没传 model 时） |
| `TestLLMClient_ChatStream_CancelledContext` | ctx cancel 时函数返回（不死循环） |

---

## 七、浏览器实测（真实 DeepSeek）

用 Playwright 端到端验证（Phase C 同套脚本）：

```bash
# 1. 起全栈 + 注入 key
BFF_LLM_API_KEY=sk-dd6f... docker compose -f deploy/docker-compose.infra.yml \
  -f deploy/docker-compose.apps.yml up -d --build emotion-echo-web-bff
```

```
浏览器: POST /api/v1/ai/stream {"message":"我今天心情特别好，一切都顺利","conversationId":"14"}
BFF: 真实 LLM（DeepSeek deepseek-chat）
  → SSE: data: {"choices":[{"delta":{"content":"听"}}]}
  → SSE: data: {"choices":[{"delta":{"content":"起"}}]}
  → ...
  → SSE: data: [DONE]
浏览器: 流式渲染"听起来你最近心情特别好..."（打字机效果）
```

实测成功（curl + Playwright 双确认）。

---

## 八、设计决策记录

### 决策 1：为什么用 OpenAI 兼容格式而非 Anthropic / Gemini 原生？

- **前端 useAIStream.ts 已用 OpenAI 兼容**（之前是 mock，写法固定）
- **DeepSeek / Kimi / Ollama / OpenAI** 全支持 OpenAI 格式 → 切换 LLM 不改前端
- Antrhopic / Gemini 原生格式需要前端改 → 锁定供应商

### 决策 2：为什么 key 走 env 而非 config 文件？

- **安全**：config 文件进 git 是泄漏面，env 注入不留痕
- **生产部署**：docker-compose / k8s secret / vault 都按 env 注入
- **dev fallback**：APIKey 空 → mock → 无 key 也能跑（开发友好）

### 决策 3：mock fallback 还是 mock + LLM？

保留 mock fallback（APIKey 空时）。理由：
- 开发环境无需配 key 即可跑完整链路
- 集成测试 / e2e 测试不需要真实 LLM（避免 token 成本）
- 部署时按需注入 key，无侵入性

---

## Stage 36-B1 增补：ai-svc 侧 env 注入（2026-09-01）

之前 stage 30-D 主要设计 BFF 的 `BFF_LLM_*` env，但 ai-svc 的 LLM 调用（FusionWorker）走另一套：
- ai-svc main.go:419-432 直接读 `os.Getenv("LLM_BASE_URL") / "LLM_API_KEY" / "LLM_MODEL")`
- Stage 35 PR-7 + PR-8 已加 `LLM_TIMEOUT` 与 `LLM_MODEL` 的覆盖

Stage 36-B1 把 ai-svc env 注入对齐到 docker-compose：
- `deploy/docker-compose.apps.yml` ai-svc env 块：
  - `LLM_BASE_URL: ${LLM_BASE_URL:-}` （空 = fallback）
  - `LLM_API_KEY: ${LLM_API_KEY:-}` （关键！之前从未注入到 ai-svc）
  - `LLM_MODEL: ${LLM_MODEL:-deepseek-chat}`
  - `LLM_TIMEOUT: ${LLM_TIMEOUT:-3}`
- `deploy/env/.env.local.example` 提供 DeepSeek 默认值的模板（gitignored `*.local`）

**为什么不用 `FUSION_LLM_*` 前缀**：ai-svc 历史代码（Stage 19 / Stage 35 PR-8）已用无前缀的 `LLM_*`，改名代价大；与 BFF 的 `BFF_LLM_*` 区分（两 svc 不共享 env 命名空间）。

**真实冒烟**：在 `.env.local` 填入 DeepSeek key 后 `docker compose up -d ai-svc`，看 logs：
- 有 key：`[fusion] LLM fuser active: https://api.deepseek.com model=deepseek-chat breaker=closed`
- 没 key：`[fusion] LLM fuser disabled (LLM_BASE_URL empty); late_fuser is fallback`

同时 `emotion_echo_fusion_llm_call_total{outcome="success"}` Prometheus 指标应增加。

---

## 九、后续可优化（未做）

| 项 | 价值 |
|----|------|
| BFF 加限流中间件（`golang.org/x/time/rate`）按 user_id 限流 | 防止 LLM API 滥用 |
| 缓存常见回复（高频问题）| 减成本 |
| 多 LLM failover（DeepSeek 不可用 → OpenAI fallback） | 提升可用性 |
| BFF 缓存 + 异步写 chat-svc 用户消息（在 LLM 调用前先持久化） | 避免 LLM 慢时丢消息 |

---

## 十、安全检查清单

- [x] `git grep` 工作区 0 处出现真实 key
- [x] `git log -p` 历史 0 处出现真实 key
- [x] yaml 仅占位 `${:-}`，无默认值泄漏
- [x] 部署文档明确警告"不要 commit 真实 key"
