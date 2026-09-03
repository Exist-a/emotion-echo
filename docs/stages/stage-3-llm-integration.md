# Emotion-Echo · Stage 3 LLM 接入完成报告

> ⚠️ **架构决策请看 [architecture-decisions.md](./architecture-decisions.md)（ADR）**。
> 本文档保留为历史过程记录（2026-07-13 当时状态）。
> **未来变更**：HTTP → gRPC（proto 单一事实源，详见 ADR 决策 4-5）。

> 2026-07-13：ai-svc 通过 HTTPAnalyzer 调用 emotion-llm-service（FastAPI/Python），实现跨语言情绪分析。

## 🏆 战果

| 项 | 内容 |
|----|------|
| 新增微服务 | `emotion-llm-service`（FastAPI Python）端口 8000 |
| HTTP 协议 | `POST /analyze` JSON `{text}` → `{primaryEmotion, sentimentScore, confidence, model}` |
| ai-svc 新能力 | `HTTPAnalyzer`（HTTP 客户端）+ `ChainedAnalyzer`（LLM→keyword 兜底） |
| 跨语言互操作 | Go → Python HTTP 跨进程调用 |
| 韧性设计 | LLM 3 秒超时不可达 → 自动 fallback 到本地 keyword analyzer |
| 端到端验证 | message → Kafka → ai-svc → HTTP → LLM service → emotion_analysis 表 |

## 🔴🟢 TDD 闭环（HTTPAnalyzer）

```
🔴 RED：undefined: HTTPAnalyzer / ChainedAnalyzer / NewHTTPAnalyzer / NewChainedAnalyzer
🟢 GREEN：5/5 PASS
  ├─ TestHTTPAnalyzer_CallLLMService_HappyPath         httptest mock 服务返回 happy
  ├─ TestHTTPAnalyzer_LLMServiceDown_ReturnsError      不可达时返回 error
  ├─ TestHTTPAnalyzer_LLMReturns500_ReturnsError        5xx 响应处理
  ├─ TestChainedAnalyzer_LLMFirst_Success              主 LLM 成功用主
  └─ TestChainedAnalyzer_LLMFirst_FallbackKeyword      主失败用 keyword 兜底

KeywordAnalyzer (Stage 2 已有)：4/4 PASS
e2e：emotion_analysis 表的 model 字段从 keyword-stub-v1 → keyword-v1
```

## 🏗 跨语言分布式架构

```
浏览器
  │ POST /api/v1/conversations/1/messages
  ▼
APISIX (9080)
  │
  ▼
chat-svc (8890)
  │ ① DB: emotion_echo_chat.messages
  │ ② Kafka: chat-events topic
  ▼
Kafka broker
  │
  ▼
ai-svc (8891)
  │ ① 消费 message.created
  │ ② ChainedAnalyzer.Analyze()
  │     ├─ HTTPAnalyzer.Analyze() → POST localhost:8000/analyze
  │     │                          ↓
  │     │                     emotion-llm-service (Python FastAPI)
  │     │                          ↓
  │     │                     keyword-v1 / 未来 LLM
  │     │                          ↓
  │     │                     {primaryEmotion, sentimentScore, ...}
  │     │
  │     └─ 若失败 → KeywordAnalyzer.Analyze()（Go 内置兜底）
  │
  │ ③ DB: emotion_echo_ai.emotion_analysis
  ▼
（前端下次 GET 拿结果）
```

## 📁 新增文件

```
emotion-llm-service/                       ← 🆕 Python 微服务
├── main.py                                ← FastAPI app
├── requirements.txt                       ← fastapi/uvicorn/pydantic
├── out.log / err.log                      ← 运行日志

emotion-echo-ai-svc/internal/analyzer/
├── http_analyzer.go                       ← 🆕 HTTP 客户端 + ChainedAnalyzer
└── http_analyzer_test.go                  ← 🆕 5 TDD 测试

emotion-echo-ai-svc/internal/config/config.go  ← 加 LLM struct
emotion-echo-ai-svc/etc/ai-api.yaml            ← 加 LLM 段
emotion-echo-ai-svc/ai.go                      ← ChainedAnalyzer 接线
```

## 🎯 端到端验证证据

```
[chat-svc] → msg_id=3 "我感觉特别轻松，所有事情都变好了"
[ai-svc]   → HTTPAnalyzer → POST localhost:8000/analyze
[llm-svc]  → 匹配 "轻松/好" → primaryEmotion=neutral score=0.50 model=keyword-v1
[ai-svc]   → 写 emotion_analysis (id=3 msg_id=3 emotion=neutral model=keyword-v1)

[chat-svc] → msg_id=4 "最近失眠焦虑，很痛苦"
[ai-svc]   → HTTPAnalyzer → POST localhost:8000/analyze
[llm-svc]  → 匹配 "焦虑" → primaryEmotion=anxious score=0.00 model=keyword-v1
[ai-svc]   → 写 emotion_analysis (id=4 msg_id=4 emotion=anxious model=keyword-v1)
```

emotion_analysis 表：

```
 id | message_id | primary_emotion | score |      model
----+------------+-----------------+-------+----------------
  4 |          4 | anxious         |  0.00 | keyword-v1       ← LLM 服务
  3 |          3 | neutral         |  0.50 | keyword-v1       ← LLM 服务
  2 |          2 | anxious         | -0.40 | keyword-stub-v1  ← Go 内置（旧）
  1 |          1 | happy           |  0.60 | keyword-stub-v1  ← Go 内置（旧）
```

**关键证据**：model 字段从 `keyword-stub-v1`（Stage 2 的 Go 实现）变为 `keyword-v1`（Python LLM 服务的版本），证明请求实际走了 HTTP 链路。

## 🎓 这次的核心认知

| 概念 | 体验 |
|------|------|
| **跨语言契约** | Go ↔ Python 通过 JSON over HTTP 解耦 |
| **ChainedAnalyzer** | 主备 fallback 让业务无感："给我一个 Analyzer，结果是情绪" |
| **超时硬约束** | 3 秒 LLM 超时（不能用默认的无限超时）—— LLM 挂了不能阻塞消费 |
| **httptest mock** | 不启 Python 服务也能跑 Go 单测，单测 < 5 秒 |
| **model 字段溯源** | 在结果里标识用了哪个 analyzer 版本，方便审计 |

## 🎓 白盒审计要点

1. **HTTPAnalyzer** — godoc 全，timeout 显式，err 包装 %w
2. **ChainedAnalyzer** — 接口化（primary/secondary 都是 Analyzer 接口）
3. **ChainedAnalyzer.Analyze** — 失败原因日志记录，primary 错误不隐藏
4. **LLM config** — yaml 配置，不硬编码 URL

## ⚠️ 这一轮踩到的坑

1. **PowerShell 字符串转义 Chinese** — "今天很开心" 通过双引号字符串传到 Python 变乱码
2. **PowerShell Invoke-WebRequest 不支持 --data-binary** — 改用 curl.exe
3. **Copy-Item 同源覆盖** — `Copy-Item x x -Force` 报错
4. **Python 模型字段不一致** — Go 侧 model 字段叫 "keyword-stub-v1"，Python 侧叫 "keyword-v1"（审计友好但要让业务方知道）

## 📊 进度条

```
Phase 0 基础设施    ████████████████████ 100% ✅
Phase 1 go-zero     ████████████████████ 100% ✅
Phase 2 Kafka       ████████████████████ 100% ✅ (Stage 2 完)
Phase 3 韧性         ███░░░░░░░░░░░░░░░░  20%  ← ChainedAnalyzer 兜底
Phase 4 业务深化      ██████████░░░░░░░░  60%  ← LLM 接入完成
Phase 5 K8s          ░░░░░░░░░░░░░░░░░░░░   0%
```

## 🚀 下一步候选

- **A**：DLQ + 重试（Phase 3 韧性补完）
- **B**：接真实的 SnowNLP / jieba 提升中文分析精度
- **C**：接 OpenAI API / 国内大模型 API
- **D**：写 GET /api/v1/emotion/conversation/:id 端点查分析结果
- **E**：docker-compose 把 emotion-llm-service 也加入

走 A/B/C/D/E？