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
[![Stage](https://img.shields.io/badge/Stage-60--PR--TTS--VENDOR-blueviolet?style=flat-square)](/docs/stages/stage-60-pr-tts-vendor-landing.md)

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

### 1. 完整的微服务化演进（Stage 0 → 30）

从最初的 **Gin 单体**到 **5 个 Go 微服务 + Python gRPC + BFF 网关（替代原 APISIX）+ Kafka 异步管线 + 真实 LLM 接入（DeepSeek 兼容）**的完整迁移过程，每一步都有独立的文档、commit 与验证记录：

```
Gin 单体 → 微服务拆分 → gRPC 同步通信 → Kafka 异步管线
         → Prometheus metrics → mTLS 安全 → AI 模型容器化
         → 端到端冒烟测试 → 多模态分析 → BFF 网关（替代 APISIX）→ 真实 LLM 接入
```

📚 完整路线图：[`docs/distributed-roadmap.md`](/docs/architecture/roadmap.md) · 30+ 篇 stage 演进文档

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
- **BFF**（`emotion-echo-web-bff`）替代原 APISIX — 鉴权透传 + 5 下游聚合 + SSE 编排 + CORS
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
├── emotion-echo-web/             # Nuxt 3 前端
├── legacy/emotion-echo-gin/      # 遗留单体（已归档）
├── emotion-echo-models/
│   ├── FER/                      # 人脸情绪（Stage 22 容器化）
│   ├── sensevoice-small/         # 语音情绪
│   └── XTTS/                     # 语音合成（含 TTS/ 核心）
├── proto/                        # protobuf 契约
├── deploy/                       # 基础设施编排
│   ├── docker-compose.infra.yml  # PG + Redis + Kafka + SkyWalking（Stage 30 移除 APISIX/etcd）
│   └── docker-compose.apps.yml   # 5 个 Go 微服务 + BFF + 3 个 AI 模型服务
├── docs/                         # 架构决策 + 30+ 篇 stage 文档
└── scripts/                      # 自检 / 验证脚本
```

📐 详细布局规范：[`docs/git-layout.md`](/docs/deployment/git-layout.md)

---

## 快速启动

> 完整启动流程请参考 [`QUICKSTART.md`](QUICKSTART.md)。

### 方式一：Docker Compose（推荐）

```bash
# Stage 30：APISIX 退役。infra 只剩 PG / Redis / Kafka / SkyWalking（5 容器）
cd deploy
docker compose -f docker-compose.infra.yml up -d
# 等待 30~60 秒各容器健康

# 2. 启动 5 个 Go 微服务
docker compose -f docker-compose.dev.yml -f docker-compose.apps.yml up -d

# 3. （可选）启动 AI profile（TTS / 人脸识别 / 语音识别）
# 镜像已在 docker.io / 阿里云 ACR（推荐）
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml --profile ai up -d emotion-echo-xtts emotion-echo-fer emotion-echo-sensevoice
# 首次拉镜像 + build（耗时长 + 受国内镜像影响，见 stage-60 §五）
# docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml --profile ai up -d --build emotion-echo-xtts emotion-echo-fer emotion-echo-sensevoice

# ⚠️ 5 秒必读 · AI profile 不启用 = TTS 按钮无声 / 多模态不可用
# 总镜像约 18GB（XTTS 11.3GB + SenseVoice 4.16GB + FER 538MB），
# dev 默认不起避免拖慢启动；prod 部署时按需启用（详见 docs/stages/stage-60-pr-tts-vendor-landing.md）。
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
| [`docs/README.md`](/docs/stages/README.md) | **文档系统目录页**（按需找文档） |
| [`docs/architecture/decisions.md`](docs/architecture/decisions.md) | ADR 单一事实源 |
| [`docs/architecture/distributed.md`](docs/architecture/distributed.md) | 分布式架构总览 |
| [`docs/architecture/roadmap.md`](docs/architecture/roadmap.md) | 分布式改造路线图（执行版） |
| [`docs/deployment/git-layout.md`](/docs/deployment/git-layout.md) | 仓库布局规范 |
| [`docs/stages/`](docs/stages/) | Stage 0 → 36 共 81 篇演进文档 |
| [`docs/architecture/adr/`](docs/architecture/adr/) | 6 份编号 ADR |
| [`docs/plans/`](docs/plans/) | 当前有效的未来排期计划 |
| [`docs/legacy-plans/`](docs/legacy-plans/) | 历史计划归档（仅作回顾） |

---

## 学习路径建议

如果你是来学习这个项目的，推荐按以下顺序阅读：

1. **[`QUICKSTART.md`](QUICKSTART.md)** — 5 分钟了解如何启动
2. **[`docs/architecture/distributed.md`](docs/architecture/distributed.md)** — 架构总览
3. **[`docs/stages/`](docs/stages/)** — 跟随 Stage 0 → 36 演进
4. **[`docs/architecture/decisions.md`](docs/architecture/decisions.md)** — 工程协作规范 + ADR 单一事实源
4. **[`AGENTS.md`](AGENTS.md)** — 工程协作规范
5. **看代码**：`emotion-echo-ai-svc/`（最复杂的微服务，含 gRPC + Kafka）

---

## 状态

- ✅ Stage 0~28 全部完成（微服务化 + AI 容器化 + 端到端验证 + K8s 化 + 可观测性）
- ✅ Stage 29-A / 29-A.5：cert-manager + Grafana Ingress TLS（render + live smoke 已绿）
- ✅ Stage 29-D：5-family TLS retrofit for the 15 business ApisixRoutes（render-assert 已绿；live smoke 待集群验证）
- ✅ Stage 30-A/B/C：analytics 9 端点 + Kafka pipeline + 消费幂等/DLQ/Outbox（全绿）
- ✅ **Stage 30 Web BFF**：`emotion-echo-web-bff`（:8894）BFF 聚合层 — 聚合 5 下游 + SSE 编排 + 自有 mock 鉴权（`docs/stage-30-web-bff.md`）。
  > 🔧 **2026-09-04 措辞就地更正**（决策 18 §4.4）：原"唯一入口"与决策 11（APISIX = 网关层）/
  > 决策 12（BFF 宿主机不再直接映射）字面冲突。准确说法：BFF 是 APISIX 的 upstream，
  > **APISIX 才是唯一业务入口**；BFF 端口 8894 仅在 dev 调试保留（cf. `apps.yml:602-604` 注释）。
- ✅ **Phase D 接 DeepSeek**：BFF ai_stream 改造为 OpenAI 兼容真实 LLM（env 注入 key，无 key 降级 mock）
- ✅ **APISIX 退役**：Stage 30 BFF 替代网关职责后，compose + helm apisix-routes + etcd 全清；历史保留在 `docs/`（`stage-29-D-tls-all-routes.md`）
  > 🔧 **2026-09-04 措辞就地更正**（决策 18 §4.4）：此条是 Stage 30 的**演进记录**（APISIX 在 Stage 30 暂时退场）；Stage 32 已重新引入 APISIX 网关层（决策 11），
  > cf1c798 后 dev compose 也已恢复。当前实际状态：**APISIX 是 dev / prod 唯一业务入口**（决策 11/12）。
- ✅ **Stage 31**：Nacos 注册中心 + 配置中心演进（PR-01..12）
- ✅ **Stage 32**：APISIX 网关层回归 + JWT 真实验证 + X-User-Id 透传（PR-13..16）
- ✅ **Stage 33**：P0 修复 + BFF 净化（PR-17..22，7 个 PR 收口）
- ✅ **Stage 34**：多模态情绪融合（18 个 PR + merge 收口，docker smoke 验证）— 见 [stage-34-landing.md](/docs/stages/stage-34-landing.md)
- ✅ **Stage 35**：LLM Fusion 生产加固 + 业务端到端验证（ADR-15，14 个 PR + 41 个新测试）— 见 [stage-35-landing.md](/docs/stages/stage-35-landing.md) + [stage-35-smoke-validation.md](/docs/stages/stage-35-smoke-validation.md) + [stage-35-system-feasibility.md](/docs/stages/stage-35-system-feasibility.md)
- 🚧 **Stage 36**（进行中）：ADR-16 列出的 **8 项系统缺口**全部纳入修复日程 — 见 [stage-36-fixes-roadmap.md](/docs/stages/stage-36-fixes-roadmap.md) + [adr-2026-09-known-gaps.md](/docs/architecture/adr/adr-2026-09-known-gaps.md)
  - 36-A：G1（yaml 占位符 4 svc）+ G3（BFF 路由）
  - 36-B：G2（chat list）+ G4（消息自动情绪分析）
  - 36-C：G5（真实 LLM）+ G6（FER/SenseVoice）
  - 36-D：G7（APISIX 镜像）+ G8（Nacos 全栈）

> **🔧 2026-09-10 决策 18 §4.4 就地更正块（README 顶部徽章已同步）**：
>
> 上面 Status 段（Stage 36 之前）与实际进度**已严重失真**——按 git log 实际已达 **Stage 60**。
> 完整实际阶段记录如下（原行保留作为历史快照）：
>
> | Stage | 范围 | 状态 | landing 文档 |
> |---|---|---|---|
> | **Stage 36** | ADR-16 8 项缺口修复（chart contract alignment / msg_list / dev fallback） | ✅ landed | [stage-36-landing.md](/docs/stages/stage-36-landing.md) |
> | **Stage 37** | fixes-roadmap 收口（A/B 两批）| ✅ landed | [stage-37-A-landing.md](/docs/stages/stage-37-A-landing.md) + [stage-37-B-landing.md](/docs/stages/stage-37-B-landing.md) |
> | **Stage 38** | dev apisix path 端到端打通 + 系统状态盘点 | ✅ landed | [stage-38-A-landing.md](/docs/stages/stage-38-A-landing.md) + [stage-38-system-status.md](/docs/stages/stage-38-system-status.md) |
> | **Stage 39** | Nacos dev enablement 落地 | ✅ landed | [stage-39-nacos-enablement.md](/docs/stages/stage-39-nacos-enablement.md) |
> | **Stage 40** | doc drift rectification 集中清理 | ✅ landed | [stage-40-doc-drift-rectification.md](/docs/stages/stage-40-doc-drift-rectification.md) |
> | **Stage 41** | 7 个 Go svc go-zero 全量移除（决策 1 收口） | ✅ landed | [stage-41-gozero-removal.md](/docs/stages/stage-41-gozero-removal.md) + [stage-41-smoke-2026-09-07.txt](/docs/stages/stage-41-smoke-2026-09-07.txt) |
> | **Stage 42** | 容器 TZ 修复 | ✅ landed | [stage-42-container-tz-fix.md](/docs/stages/stage-42-container-tz-fix.md) |
> | **Stage 43** | Kafka reliability sprint A | ✅ landed | [stage-43-kafka-reliability-sprint-a.md](/docs/stages/stage-43-kafka-reliability-sprint-a.md) |
> | **Stage 44~49** | observability sprint B（OTel / gin entry span / logging helper / err tag / grpc rpc tag / 回归） | ✅ landed | [stage-44~49](/docs/stages/) |
> | **Stage 50~55** | 端到端验证 / Nacos 修复 / port mismatch / roadmap Z 状态 | ✅ landed | [stage-50~55](/docs/stages/) |
> | **Stage 56~57** | 批 2/3 合并（Kafka fallback / mentalhealth trigger） | ✅ landed | [stage-56-b2-merged.md](/docs/stages/stage-56-b2-merged.md) + [stage-57-b3-merged.md](/docs/stages/stage-57-b3-merged.md) |
> | **Stage 58** | Q3 后续 8 工作面 24 commit（ENV-1~4 / UP-1~3 / TTS-1~4 / GRPC-1~6） | ✅ landed | [stage-58-q3-followups.md](/docs/stages/stage-58-q3-followups.md) |
> | **Stage 59** | vendor 镜像实测（ai4all/coqui ✅ / serengil/deepface ⚠ / yiminger/sensevoice ❌） | ✅ landed | [stage-59-vendor-image-verification.md](/docs/stages/stage-59-vendor-image-verification.md) |
> | **Stage 60** | PR-TTS-VENDOR 三 AI 模型落地（XTTS vendor 133KB WAV / FER-tflite 538MB / SV-fastbuild 5.88GB） | ✅ landed | [stage-60-pr-tts-vendor-landing.md](/docs/stages/stage-60-pr-tts-vendor-landing.md) |
> | **Stage 60.1** | SV-fastbuild 真实验证 + 仓库分层 base/app Dockerfile + 阿里云 ACR 推送 | ✅ landed | [stage-60-1-pr-tts-vendor-sv-landing.md](/docs/stages/stage-60-1-pr-tts-vendor-sv-landing.md) |
> | **Stage 61~63** | Sprint C/D/E/F1/F2 + gRPC 化决策 4 收口 + Sprint G BFF 全局 gRPC error 映射 | ✅ landed | [stage-61~63](/docs/stages/) + [stage-63-grpc-closure-report.md](/docs/stages/stage-63-grpc-closure-report.md) |
> | **Stage 64** | bug 类未做事项集中收口（chat-svc 中间件分层 + 决策 18 #32 关闭 + 杂项） | ✅ landed | [stage-64-bug-cleanup-2026-09-11.md](/docs/stages/stage-64-bug-cleanup-2026-09-11.md) |
> | **决策 20** | 环境配置分层策略（dev 本地 vs prod 远端）owner sign-off（PR-ENV-1~4 已落地）| ✅ Accepted 2026-09-11 | [adr-2026-09-env-profile-strategy.md](/docs/architecture/adr/adr-2026-09-env-profile-strategy.md) |
> | **Stage 65~71** | dev dashboard 空数据根因/seed + bug 集中收口 + DevEventPublisher + ai-svc metadata + Kafka P1 retry/DLQ + BFF 路由三方对齐 | ✅ landed | [stage-65~71](/docs/stages/) |
> | **Stage 72** | chat Pin/Update RPC 全链路 + Nacos PR-1 | ✅ landed | [stage-72-chat-rpc-nacos-fixture-2026-09-12.md](/docs/stages/stage-72-chat-rpc-nacos-fixture-2026-09-12.md) |
> | **Stage 73** | Kafka §1.5 Protobuf 迁移（eventrow 共享包 + JSON 双写窗口） | ✅ landed | [stage-73-kafka-protobuf-migration-2026-09-12.md](/docs/stages/stage-73-kafka-protobuf-migration-2026-09-12.md) |
> | **Stage 74** | 技术债收尾（Grafana lag 面板 / dev web 容器首通 / Helm 残余冻结 = 决策 23） | ✅ landed | [stage-74-tech-debt-closure-2026-09-12.md](/docs/stages/stage-74-tech-debt-closure-2026-09-12.md) |
> | **Stage 75~76** | Nacos PR-2（BFF gRPC 拨号 Nacos 优先）+ PR-3 dev e2e 验收（契约测试 23/23 + §契约 7 6/6） | ✅ landed | [stage-75](/docs/stages/stage-75-nacos-pr2-grpc-discovery-2026-09-12.md) + [stage-76](/docs/stages/stage-76-nacos-pr3-e2e-acceptance-2026-09-12.md) |
> | **Stage 77** | Postgres 启动重试 + 5 svc gRPC nil-repo → Unavailable 守卫 | ✅ landed | [stage-77-postgres-retry-nilrepo-guard-2026-09-12.md](/docs/stages/stage-77-postgres-retry-nilrepo-guard-2026-09-12.md) |
> | **Stage 78** | 业务计划排期核查（销账/失效盘点 + 新增 llm-chat-real-pipeline 计划） | ✅ landed | [stage-78-business-plans-audit-2026-09-12.md](/docs/stages/stage-78-business-plans-audit-2026-09-12.md) |
> | **Stage 79** | file-upload 收口（前端装配层接线 + contentType 响应链 5 处丢失全链修复） | ✅ landed | [stage-79-post-file-upload-wiring-2026-09-12.md](/docs/stages/stage-79-post-file-upload-wiring-2026-09-12.md) |
> | **Stage 80~83** | llm-chat-real-pipeline PR-1/2/3a/3b（llm-service 流式 ChatCompletion RPC → BFF gRPC 上游 → 规则式 6 类意图分类/风格化回复 → intent 落库 + 日报意图饼图全链） | ✅ landed | [stage-80~83](/docs/stages/) |
> | **Stage 84** | chat-svc events 包 3 个存量测试适配 Stage 73 Protobuf 契约（`go test ./...` 合并门槛恢复） | ✅ landed | [stage-84-kafka-publisher-test-contract-2026-09-12.md](/docs/stages/stage-84-kafka-publisher-test-contract-2026-09-12.md) |
> | **Stage 85** | 趋势报告意图维度（weekly/monthly/annual 饼图全链；真实容器 e2e + DB 交叉实证） | ✅ landed | [stage-85-trend-report-intent-2026-09-12.md](/docs/stages/stage-85-trend-report-intent-2026-09-12.md) |
>
> **失真类型**（决策 18 §三）：类型 2 "陈旧结论" — README 顶部徽章与 Status 段长期未跟随 git log 更新。
> **当前最新 commit**：`40f9f73 feat(analytics,bff,web): Stage 85 — 趋势报告意图维度全链落地`（2026-09-12 更新本表至 Stage 85）

---

## 贡献

本项目为个人作品集，主要由我自己开发。如果你发现 bug 或有建议：

- 📮 提 Issue（描述清晰 + 复现步骤）
- 🔀 提 PR（请遵循 [`AGENTS.md`](AGENTS.md) 的 TDD 流程：先写测试，再写实现）

---

## License

[MIT](LICENSE) © 2026 Emotion-Echo Contributors