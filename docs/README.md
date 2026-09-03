# Emotion-Echo 文档系统

> **目录页**：本文件是整个 `docs/` 的入口。
> 你想找什么 → 看下面"按需找文档"。

## 按需找文档

| 我想…… | 看哪里 |
|--------|-------|
| 跑起来 | 根目录 [QUICKSTART.md](../QUICKSTART.md) |
| 了解架构总览 | [architecture/distributed.md](architecture/distributed.md) + [architecture/positioning.md](architecture/positioning.md) |
| 找一条 ADR | [architecture/adr/](architecture/adr/) |
| 读单一事实源 ADR | [architecture/decisions.md](architecture/decisions.md) |
| 看部署/Compose | [deployment/docker-compose.md](deployment/docker-compose.md) |
| 看 k8s/Helm 历史 | [stages/stage-27-k8s-local-helm.md](/docs/stages/stage-27-k8s-local-helm.md) |
| 看运维手册 | [deployment/runbook/](deployment/runbook/) |
| 看前端设计 | [frontend/design.md](frontend/design.md) |
| 看 AI 模型构建 | [ai-models/build-guide.md](ai-models/build-guide.md) |
| 看 XTTS 决策 | [ai-models/xtts-decision.md](ai-models/xtts-decision.md) |
| 看某次演进完整记录 | [stages/stage-XX-*.md](stages/)（Stage 0 → 36） |
| 看未来要做什么 | [plans/](plans/) |
| 回顾过去的计划 | [legacy-plans/](legacy-plans/)（landed / shifted / historical） |
| 学习 Kubernetes/Docker/TDD | [learn/](learn/) |

## 文档地图

```
docs/
├── README.md                       # 本文件（目录页）
├── _meta/
│   └── doc-migration-map.md        # 文档迁移总表（Round 0 产物）
│
├── architecture/         # 架构决策与定位（单一事实源）
│   ├── README.md
│   ├── decisions.md      # ADR 单一事实源
│   ├── positioning.md    # 分布式架构 + 单机部署
│   ├── distributed.md    # 分布式架构总览
│   ├── roadmap.md        # 演进路线图（历史过程记录）
│   ├── microservices.md
│   ├── decomposition-plan.md
│   ├── audit-2026-08-31.md
│   └── adr/              # 6 份编号 ADR（adr-2026-09-*.md）
│
├── deployment/           # 部署与运维
│   ├── README.md
│   ├── docker-compose.md
│   ├── git-layout.md
│   └── runbook/          # Stage 34 / Stage 35 运维手册
│
├── services/             # 微服务说明（占位）
│   └── README.md         # 待按需补各服务子文件
│
├── frontend/             # 前端文档
│   ├── README.md
│   └── design.md         # Emotion-Echo-Web 设计
│
├── ai-models/            # AI 模型服务（FER / SenseVoice / XTTS）
│   ├── README.md
│   ├── build-guide.md
│   ├── xtts-decision.md
│   └── xtts-integration.md
│
├── stages/               # 系统演进历史（Stage 0 → 36+）
│   └── README.md
│
├── plans/                # 当前有效、未来排期的计划
│   ├── README.md
│   ├── ai-response-structured.md
│   ├── three-vrm-usage-reference.md
│   ├── wechat-qq-login-and-upload.md
│   ├── file-upload-message-extension.md
│   └── intent-classification-6-types.md
│
├── legacy-plans/         # 历史计划归档（仅作回顾）
│   ├── README.md
│   ├── landed/           # 🟢 已落地（18 份）
│   ├── shifted/          # 🟡 已偏移（8 份）
│   └── historical/       # ⚫ 历史价值（1 份）
│
├── learn/                # 学习教程（13 份，原位保留）
│   ├── 00-index.md
│   ├── 01-why-kubernetes.md
│   ├── 02-local-cluster.md
│   └── ...
│
├── env-templates/        # 环境变量模板（仓库原有）
└── flatten-snapshot-20260716-205225.json  # 一次性快照
```

## 当前架构状态（Stage 36+）

| 服务 | 端口 | 类型 | 文档 |
|------|------|------|------|
| emotion-echo-web-bff | :8894 | 唯一入口 | [stages/stage-30-web-bff.md](/docs/stages/stage-30-web-bff.md) |
| emotion-echo-user-svc | :8888 | 用户认证 | [stages/stage-33-p0-fix-bff-purify.md](/docs/stages/stage-33-p0-fix-bff-purify.md) |
| emotion-echo-chat-svc | :8890 | 会话/消息 | [stages/stage-35-landing.md](/docs/stages/stage-35-landing.md) |
| emotion-echo-analytics-svc | :8893 | 报表 | [stages/stage-30-A-analytics-business.md](/docs/stages/stage-30-A-analytics-business.md) |
| emotion-echo-assessment-svc | :8889 | 量表 | [stages/stage-30-A-analytics-business.md](/docs/stages/stage-30-A-analytics-business.md) |
| emotion-echo-ai-svc | :8891(HTTP) / :8892(gRPC) | AI 编排 | [stages/stage-19-ai-svc-grpc-server.md](/docs/stages/stage-19-ai-svc-grpc-server.md) |
| emotion-llm-service | :8000(HTTP) / :50051(gRPC) | Python LLM | [stages/stage-30-D-llm-integration.md](/docs/stages/stage-30-D-llm-integration.md) |
| FER (profile ai) | :8004 | 人脸情绪 | [ai-models/build-guide.md](ai-models/build-guide.md) |
| SenseVoice (profile ai) | :8002 | 语音情绪 | 同上 |
| XTTS (profile ai) | :8003 | 语音合成 | [ai-models/xtts-integration.md](ai-models/xtts-integration.md) |
| Web (Nuxt) | :3000 | 前端 | [frontend/design.md](frontend/design.md) |

## 文档维护约定

- 新增功能计划写到 `docs/plans/`（不再使用过去的 `.trae/` 目录）
- 新增架构决策写一条 ADR 到 `docs/architecture/adr/`，并在 `docs/architecture/decisions.md` 索引中登记
- 完成的 stage 在 commit 完成后追加 `docs/stages/stage-XX-*.md`
- 已有计划落地后，从 `plans/` 迁入 `legacy-plans/landed/` 加 front-matter

> 本目录自 2026-09-03 起按本约定组织；重构过程见 [_meta/doc-migration-map.md](_meta/doc-migration-map.md)。
