---
status: active
priority: high
created: 2026-10-13
type: parallel-execution-contract
related:
  - roadmap.md（阶段与依赖）
  - RUNBOOK.md（执行协议）
  - ../_meta/parallel-tracks.md（AGENTS §八 的双轨协议；本文件是 CL 阶段内的并行契约）
---

# 「闭环」阶段 · 并行执行契约（parallel-contract）

> 多个会话 / 人**并行推进** CL 阶段时，靠本文件隔离、避免互相踩。
> 场景：剩余时间紧，需要多条轨同时开工。

## 🔴 硬规则（三条，缺一不可）

1. **每次开工前必读本文件** —— 先看"谁正在动什么"（下方占位登记表），再决定能不能开。
2. **开始执行时必须在本文件登记占位** —— 写清：**哪条轨 / 哪个阶段 / 分支 / owner / 开始时间 / 状态**。
3. **收工必须删除自己的占位** + 跑 AGENTS §2.5 收口三连自检。

> 一句话：**读契约 → 看占位 → 登记 → 干活 → 删占位。**

## 一、轨道定义（**主文件面** + 共享 spine）

> **现实核对（2026-10-13）**：各阶段的**大部分文件不同**，但有一层「**脊柱文件**」多轨都会碰（§1.2）。**这些脊柱文件必须串行 + 握手**，不能真并行改。
> 冲突时按 AGENTS §八 走"文件面硬隔离 + 共享文件握手行"。

### 1.1 各轨主文件面（main face）

| 轨 | 阶段 | 主文件面 | 备注 |
|---|---|---|---|
| **A** | **CL-04** Prompt 组装层 | BFF `handler/ai_stream_handler.go`、`downstream/llm_grpc.go` + `emotion-llm-service/*.py` + 前端 `composables/{useAIStreamHandler,useAIStream,usePrompt}.ts` | 解锁 CL-06/07；**尽早做** |
| **B** | **CL-01** 可观测（+ **CL-03**） | `shared/pkg/skywalking/**` + 各 svc `main.go` + BFF `downstream/{ai,analytics,assessment}.go` + `deploy/{prometheus,grafana,alertmanager,loki}` + `scripts/smoke_observability.py` | CL-01+CL-03 **同追踪面 ⇒ 建议同轨** |
| **C** | **CL-05** 消息链路 | 前端 `stores/{message,conversation}.ts`、`utils/{db,messageCache}.ts`、`composables/useConversationSender.ts`、`pages/chat/conversation/[id].vue`、`plugins/init.ts` + chat-svc `internal/{types,repository,logic,grpcserver}` + BFF `downstream/chat*.go`、`handler/chat_handler.go` + **`proto/chat.proto`** | 独立于 T1 |
| **D** | **CL-00** CI 瘦身 | `.github/workflows/**` + `scripts/**` | 独立于业务代码 |
| **E** | **CL-02** gRPC 契约 | **`proto/**`** + 各 svc `internal/grpcserver/**` + BFF `internal/downstream/**` + `scripts/check_dead_grpc_inventory.sh` | 独立，可搭任一轮 |

> **不能并行**：CL-06 / CL-07（依赖 CL-04）；CL-08a~e（自身有序）。

### 1.2 共享 spine 文件（**多轨都碰 ⇒ 必须串行 + 握手登记**）

| spine 文件 | 哪些轨会碰 | 处置 |
|---|---|---|
| **`proto/**`**（尤其 `chat.proto`） | C（分页字段）、E（死链处理） | **串行**：谁先占谁改，另一轨等 |
| **各 `<svc>/main.go` + BFF `main.go`** | B（tracing 接线）、C（chat 接线）、E（downstream/grpc 接线） | **串行 / 握手** |
| **`emotion-echo-web-bff/internal/downstream/**`** | A（llm_grpc）、B（ai/analytics/assessment HTTP 埋点）、C（chat）、E（多数） | **同名文件串行**（不同文件可并） |
| **`scripts/**`** | D（CI 全量）、B（smoke_observability）、E（dead_grpc guard） | **同名脚本串行**；D 重组前先看有没有别的轨在改脚本 |
| **`go.mod` / `package.json`** | A/C（新依赖如 `fake-indexeddb`）、B（redis client） | **追加式**（各自加行，冲突时 rebase） |

> **一句话**：主文件面基本隔离，但 **`proto` / `main.go` / `downstream` / `scripts` / `go.mod|package.json` 是脊柱**——改它们 = **先握手、后串行**。

## 二、共享文件（改动前必须在下"握手区"登记一行）

`deploy/**docker-compose*.yml**` · `AGENTS.md` · `docs/closure-roadmap/decisions.md` · 顶层 `README.md` · 其他轨独占列以外的公共脚本。

> 触碰共享文件 = 先在本文件 §四 握手区登记（谁/哪个文件/为什么），改完标 ✅。

## 三、占位登记表（**正在执行** —— 开工写、收工删）

| 轨 | 阶段 | 分支 / worktree | owner | 开始 | 状态 |
|---|------|----------------|-------|------|------|
| A | CL-04 Prompt 组装层 | 待认领 | — | — | **open**（待另一会话） |

> 示例（不要留真数据）：`| D | CL-00 CI 瘦身 | feat/cl-00-ci-slim | Lane-CL-D | 2026-10-13 | in-progress |`

## 四、握手区（共享文件改动）

| 时间 | owner | 文件 | 缘由 | 状态 |
|------|-------|------|------|------|
| _（空）_ | | | | |

## 五、开工前置检查（照做）

1. 读本文件 → 确认目标轨**无人占用**（占位表无同轨 in-progress）。
2. **文件面隔离**：`git worktree add -b <分支> ../Emotion-Echo-<slug> <base>`（**不在共享目录切分支**）。
3. **先开 PR（draft）再开工**（**2026-10-14 起强制**）：分支 push → 建指向 `main` 的 **draft PR** → 在 §三 占位行**一并填 PR 链接**。**无 PR 不得开工**。
4. 查 `deploy/.devmode-session`（dev mode 归属锁，有人登记不得强占）。
5. **登记占位**（§三，含 PR 链接）。
6. 按 `RUNBOOK.md` 执行（起环境 / 六步循环 / 收口契约）。

## 六、收工（照做）

1. **删除本轨占位**（§三）+ 清理握手行（§四）。
2. 跑 AGENTS §2.5 三连：`git status` 干净 / `git status -sb` 与 origin 无 ahead-behind / `git branch --merged main` 无残留。
3. 删 `.devmode-session`（若是你登记的）。
4. 阶段状态回写 `roadmap.md`。
5. **PR 处理**：阶段收口时把 PR 转 ready，按 §四 握手**串行合并**，合并后删源分支（AGENTS §2.5）。

## 七、与治理体系的关系

- **AGENTS §八 双轨并行约束**（`docs/_meta/parallel-tracks.md`）是**跨大阶段**的协议（Lane E / Lane O）；本文件是 **CL 阶段内**的并行契约。两者叠加适用，冲突时以更严格者为准。
- 每条轨的**执行协议**仍以 `RUNBOOK.md` 为准；本文件只管**并行隔离与占位**。
