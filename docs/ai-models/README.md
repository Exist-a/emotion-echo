---
purpose: AI 模型服务文档
status: Round 1 占位 · Round 3 迁入
---

# AI 模型服务

> FER、SenseVoice、XTTS 等 AI 模型相关的部署、构建与决策文档。

## 当前文件（Round 3 之后）

| 文件 | 来源 | 说明 |
|------|------|------|
| `build-guide.md` | `docs/ai-images-build-guide.md` | AI 模型镜像构建指南 |
| `xtts-decision.md` | `docs/xtts-cloud-api-decision.md` | XTTS 决策记录 |
| `xtts-integration.md` | `docs/xtts-cloud-api-integration.md` | XTTS 集成说明 |

## AI profile 端口表

| 服务 | 容器端口 | 路径 |
|------|---------|------|
| SenseVoice | :8002 | `Emotion-Echo-LLM/sensevoice-small/` |
| XTTS | :8003 | `Emotion-Echo-LLM/XTTS/` |
| FER | :8004 | `Emotion-Echo-LLM/FER/` |
