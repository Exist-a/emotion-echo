# Emotion-Echo · 情绪倾诉与心理健康助手

> 一个端到端的多模态情绪 AI 应用，从单体 Gin 到 5 服务微服务的完整演进。

[![Go](https://img.shields.io/badge/Go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white)](https://golang.org)
[![Nuxt](https://img.shields.io/badge/Nuxt-3-00DC82?style=flat-square&logo=nuxtdotjs&logoColor=white)](https://nuxt.com)
[![TypeScript](https://img.shields.io/badge/TypeScript-5.x-3178C6?style=flat-square&logo=typescript&logoColor=white)](https://www.typescriptlang.org)
[![Python](https://img.shields.io/badge/Python-3.10+-3776AB?style=flat-square&logo=python&logoColor=white)](https://www.python.org)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-14-336791?style=flat-square&logo=postgresql&logoColor=white)](https://www.postgresql.org)
[![Redis](https://img.shields.io/badge/Redis-7-DC382D?style=flat-square&logo=redis&logoColor=white)](https://redis.io)
[![Kafka](https://img.shields.io/badge/Kafka-3.x-231F20?style=flat-square&logo=apachekafka&logoColor=white)](https://kafka.apache.org)
[![gRPC](https://img.shields.io/badge/gRPC-1.x-244c5a?style=flat-square&logo=grpc&logoColor=white)](https://grpc.io)
[![Docker](https://img.shields.io/badge/Docker-Compose-2496ED?style=flat-square&logo=docker&logoColor=white)](https://www.docker.com)
[![License](https://img.shields.io/badge/License-MIT-green?style=flat-square)](LICENSE)
[![Stage](https://img.shields.io/badge/Stage-29--A-blueviolet?style=flat-square)](docs/stage-29-A-https-grafana.md)

---

## 项目简介

**Emotion-Echo** 是一个面向 C 端的「情绪倾诉与心理健康」应用，提供：

- 🤖 **AI 情绪疏导对话**：流式输出 + 情绪标签识别
- 🎙️ **多模态情绪识别**：文本（LLM）/ 语音（SenseVoice）/ 人脸（FER）
- 🔊 **语音合成回复**：XTTS 流式 TTS
- 📊 **情绪分析报表**：日报 / 周报 / 月报趋势可视化
- 📋 **心理测验**：SDS 等专业量表
- 👤 **3D 数字人**：Three-VRM 数字人形象与语音同步

## 技术亮点

### 1. 完整的微服务化演进（Stage 0 → 25）

从最初的 **Gin 单体**到 **5 个 Go 微服务 + Python gRPC + APISIX 网关 + Kafka 异步管线**的完整迁移过程，每一步都有独立的文档、commit 与验证记录：

```
Gin 单体 → 微服务拆分 → gRPC 同步通信 → Kafka 异步管线
         → Prometheus metrics → mTLS 安全 → AI 模型容器化
         → 端到端冒烟测试 → 多模态分析
```

📚 完整路线图：[`docs/distributed-roadmap.md`](docs/distributed-roadmap.md) · 30+ 篇 stage 演进文档

### 2. 严谨的 TDD 工程实践

- 🔴🟢♻️ **Red-Green-Refactor** 严格循环
- Go：`stretchr/testify` + 表驱动 + `t.Run` 子测试
- Frontend：Vitest + Vue Test Utils + Pinia Testing
- Python：pytest + pytest-asyncio + httpx
- 覆盖率底线：核心包 80% / pkg 工具包 90%

📖 强约束协作约定：[`AGENTS.md`](AGENTS.md)

### 3. 可观测性 + 生产化基座

- **SkyWalking** 链路追踪（HTTP / gRPC / Kafka 全链路）
- **Prometheus** metrics 采集
- **APISIX** 网关（路由 / 鉴权 / 限流 / CORS）
- **etcd** 配置中心
- **Kafka** 异步消息队列
- 完整 **docker-compose** 编排（`deploy/`）

### 4. AI 多模态集成

| 模型 | 用途 | 技术 |
|------|------|------|
| Kimi / OpenAI 兼容 LLM | 文本情绪分析 + 情绪疏导 | gRPC |
| SenseVoice-small | 语音情绪识别（多语种） | FastAPI |
| FER | 人脸情绪识别（7 类） | FastAPI + OpenCV |
| XTTS-v2 | 语音合成（TTS） | FastAPI + Coqui |

所有模型已 Docker 化，与 Go 微服务通过 HTTP/JSON 解耦。

---

## 仓库结构

```
emotion-echo/
├── emotion-echo-shared/          # Go 共享库（pkg + proto stubs）
├── emotion-echo-ai-svc/          # AI 编排：gRPC server + Kafka consumer
├── emotion-echo-chat-svc/        # 会话与消息
├── emotion-echo-analytics-svc/   # 情绪分析报表
├── emotion-echo-assessment-svc/  # 心理测验/量表
├── emotion-echo-user-svc/        # 用户认证
├── emotion-llm-service/          # Python gRPC：LLM 文本情绪
├── Emotion-Echo-Web/             # Nuxt 3 前端
├── legacy/emotion-echo-gin/      # 遗留单体（已归档）
├── Emotion-Echo-LLM/
│   ├── FER/                      # 人脸情绪（Stage 22 容器化）
│   ├── sensevoice-small/         # 语音情绪
│   └── XTTS/                     # 语音合成（含 TTS/ 核心）
├── proto/                        # protobuf 契约
├── deploy/                       # 基础设施编排
│   ├── docker-compose.infra.yml  # PG + Redis + Kafka + APISIX + etcd + SkyWalking
│   ├── docker-compose.apps.yml   # 5 个 Go 微服务
│   └── apisix/                   # 路由 / upstream 配置
├── docs/                         # 架构决策 + 30+ 篇 stage 文档
└── scripts/                      # 自检 / 验证脚本
```

📐 详细布局规范：[`docs/git-layout.md`](docs/git-layout.md)

---

## 快速启动

> 完整启动流程请参考 [`QUICKSTART.md`](QUICKSTART.md)。

### 方式一：Docker Compose（推荐）

```bash
# 1. 启动基础设施（PG / Redis / Kafka / APISIX / SkyWalking / etcd）
cd deploy
docker compose -f docker-compose.infra.yml up -d
# 等待 30~60 秒各容器健康

# 2. 启动 5 个 Go 微服务
docker compose -f docker-compose.apps.yml up -d

# 3. （可选）启动 AI profile
docker compose --profile ai up -d --build
```

### 方式二：本地开发

各服务可独立 `go run` / `npm run dev` / `python server.py`，详见 [QUICKSTART.md](QUICKSTART.md)。

### 端到端验证

```bash
# 仓库布局自检
python scripts/check_git_layout.py

# Stage 23 endpoint 冒烟测试
python scripts/verify_stage23_endpoints.py --ai-svc http://localhost:8891
```

---

## 文档导航

| 文档 | 用途 |
|------|------|
| [`AGENTS.md`](AGENTS.md) | **强约束**：TDD 协作约定 + 测试栈 + 可测试性设计 |
| [`QUICKSTART.md`](QUICKSTART.md) | 完整启动 + 测试流程 |
| [`docs/git-layout.md`](docs/git-layout.md) | 仓库布局规范 |
| [`docs/distributed-roadmap.md`](docs/distributed-roadmap.md) | 分布式改造路线图（执行版） |
| [`docs/distributed-architecture.md`](docs/distributed-architecture.md) | 分布式架构总览 |
| [`docs/architecture-decisions.md`](docs/architecture-decisions.md) | 架构决策记录（ADR） |
| [`docs/stage-0` ~ `stage-25`](docs/) | 25 篇演进文档 |
| [`docs/stage-25-roadmap.md`](docs/stage-25-roadmap.md) | 接下来要做的事 |

---

## 学习路径建议

如果你是来学习这个项目的，推荐按以下顺序阅读：

1. **[`QUICKSTART.md`](QUICKSTART.md)** — 5 分钟了解如何启动
2. **[`docs/distributed-architecture.md`](docs/distributed-architecture.md)** — 架构总览
3. **[`docs/stage-0-learnings.md`](docs/stage-0-learnings.md)** → **`stage-25-roadmap.md`** — 跟随演进
4. **[`AGENTS.md`](AGENTS.md)** — 工程协作规范
5. **看代码**：`emotion-echo-ai-svc/`（最复杂的微服务，含 gRPC + Kafka）

---

## 状态

- ✅ Stage 0~28 全部完成（微服务化 + AI 容器化 + 端到端验证 + K8s 化 + 可观测性）
- ✅ Stage 29-A：cert-manager + Grafana Ingress TLS（render-assert 已绿）
- 🚧 Stage 29-A.5：live cluster smoke 进行中（见 [landing](docs/stage-29-A-https-grafana.md)）
- 📝 长期路线：29-B/C/D/E（告警/Secrets/全部路由 TLS/Let's Encrypt）→ 30 ArgoCD → 31 ACK 迁移

---

## 贡献

本项目为个人作品集，主要由我自己开发。如果你发现 bug 或有建议：

- 📮 提 Issue（描述清晰 + 复现步骤）
- 🔀 提 PR（请遵循 [`AGENTS.md`](AGENTS.md) 的 TDD 流程：先写测试，再写实现）

---

## License

[MIT](LICENSE) © 2026 Emotion-Echo Contributors