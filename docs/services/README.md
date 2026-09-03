---
purpose: 微服务说明
status: Round 1 占位 · Round 3 创建各服务子文档
---

# 微服务说明

> 按服务维度组织。每个 Go 微服务一个独立子文件。

## 服务列表（待 Round 3 编写）

| 服务 | 端口 | 说明 |
|------|------|------|
| `user-svc.md` | :8888 | 用户认证 |
| `chat-svc.md` | :8890 | 会话与消息 |
| `analytics-svc.md` | :8893 | 情绪分析报表 |
| `assessment-svc.md` | :8889 | 心理测验 |
| `ai-svc.md` | :8891 (HTTP) / :8892 (gRPC) | AI 编排 |
| `web-bff.md` | :8894 | 唯一 BFF 入口 |
| `llm-service.md` | :8000 (HTTP) / :50051 (gRPC) | Python LLM 推理 |

> Round 1 期间此目录为空文件，Round 3 起按需补齐。
