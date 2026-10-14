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
| A | CL-04 Prompt 组装层 | `feat/cl-04-prompt-assembly`（worktree `../Emotion-Echo-cl04`） | Lane-CL-A | 2026-10-14 | **in-progress**（draft PR [#213](https://github.com/Exist-a/emotion-echo/pull/213)；**持 dev 栈锁至 12:00**） |
| B | CL-01 可观测闭环 | `feat/cl-01-observability`（worktree `../Emotion-Echo-cl01`） | Lane-CL-B | 2026-10-14 | **in-progress**（draft PR [#211](https://github.com/Exist-a/emotion-echo/pull/211)；dev 栈**排队在 A 之后**） |
| C | CL-02 gRPC 契约闭环 | `feat/cl-02-grpc-contract`（worktree `../Emotion-Echo-cl02`） | Lane-CL-C | 2026-10-14 | **in-progress**（draft PR [#214](https://github.com/Exist-a/emotion-echo/pull/214)；🟢 Phase 先行，🔴 需栈 Phase 排队在 B 之后） |

> **登记真实性（2026-10-14 C 轨补记）**：本表在 main 上此前只剩 A 一行 `open` —— CL-00 收口（#210）清过占位后，A/B 各自把登记写在**自己的分支**上、尚未合并。
> C 轨按 §五 第 1 步"读本文件确认无人占用"时，照 main 读会得到**错误的占用状态**，故此处按**事实**回填 A/B 两行
> （依据：`feat/cl-04-prompt-assembly` / `feat/cl-01-observability` 分支上的登记 + 各自 draft PR）。**合并时请保留三行**（§四 已约定"冲突时保留双方行"）。

> 示例（不要留真数据）：`| D | CL-00 CI 瘦身 | feat/cl-00-ci-slim | Lane-CL-D | 2026-10-13 | in-progress |`

### 3.1 dev 栈时间片排队（**并行度的真实瓶颈，不是"轨数"**）

> 19 容器一套栈（稳态 ≈6GB，宿主内存上限 ≈8–10GB）⇒ **不可复制第二套**（试过：整栈复制会击穿内存）。
> 所以并行度由**栈时间片**决定：一条持锁、其余排队。**需栈的 Phase 开工前**必须在下面登记；**不得强占**持锁方。

| 顺序 | 轨 | 需要的栈窗口 | 状态 |
|---|---|---|---|
| 1 | A（CL-04） | 至 **2026-10-14 15:00**（`deploy/.devmode-session` owner=`lane-cl-a`） | **持锁中**（用户裁定：A 阶段**未收尾**，继续持有；余 T-13 契约对齐 + T-14 IAB 人可见钉，收口后删锁。B 轨已停工，C 轨若急用请先与本轨握手） |
| 2 | B（CL-01） | A 释放后（Phase 0 基线 / 8 采集 / 9 负向对照） | 排队中 |
| 3 | C（CL-02） | B 之后（Phase 2 清除验证 / 4 接真 / 6 收口冒烟） | 排队中 |

> **新轨准入规则（2026-10-14 用户口径）**：想加第 4 条轨，先回答"**你的 🟢（不需栈）部分有多大**"。
> 三条"待验证"轨 ≈ 极限；若新轨全程需栈 ⇒ **等**，不要加。**纯 🟢 轨（文档/静态守卫）不受此限**。

> **栈使用铁律**（继承 AGENTS §四 与 RUNBOOK §2）：`--env-file .env.local --profile dev` 不可省；**严禁**删除/清空/覆盖 `.env.local`；
> 用完**删除** `.devmode-session`；改完 Go 必须重建镜像再验收。

## 四、握手区（共享文件改动）

| 时间 | owner | 文件 | 缘由 | 状态 |
|------|-------|------|------|------|
| 2026-10-14 | Lane-CL-A | `docs/closure-roadmap/{parallel-contract,roadmap}.md` | A 轨开工：§三 登记占位 + CL-04 翻 in-progress + 补 PR #213 | ✅ |
| 2026-10-14 | Lane-CL-B | `docs/closure-roadmap/{roadmap,parallel-contract}.md` | B 轨开工：§三 登记占位 + CL-01 翻 in-progress + §2 补并行语义注 | ✅ |
| 2026-10-14 | Lane-CL-C | `docs/closure-roadmap/{parallel-contract,roadmap}.md`、`docs/plans/grpc-deadlink-actualization.md` | C 轨开工：§三 登记占位（**并回填 A/B 两行**）+ CL-02 翻 in-progress + 来源计划 front-matter 同步 | ✅ |
| 2026-10-14 | Lane-CL-C | **`proto/**`**（本 PR 暂未改；Phase 2/3 将改） | C 轨**预占声明**：本阶段是当前唯一计划改 `proto/**` 的轨（A/B 均不改）。其他轨若需改 proto，**先与本轨握手** | ⏳ 预占 |
| 2026-10-14 | Lane-CL-A | `AGENTS.md`（§8.1 新增）、`docs/_meta/multi-track-parallelism.md`（新文件）、`docs/_meta/parallel-tracks.md`（加指针） | A 轨收口轮：把三轨并行暴露的**六类问题**（登记只活分支 / 多轨同改登记表 / **基于旧 main 的分支会回退别人已合内容** / worktree 缺 gitignored 运行态文件 / 两 worktree 共用镜像 tag 互相覆盖 / 本地守卫假红）落成**硬约束 7~10**（只在自己的分支与 PR 上工作 · 登记独立落地且 append-only · 合并前三查 · 共享 docker 资源隔离）+ 新增细则文档（含业界做法与出处）。按硬约束 8，本行**只追加**、不改他人行 | ✅ |

> **C 轨另注**：本轨与 A/B 在 `emotion-echo-web-bff/internal/downstream/` **同目录不同文件**（A 占 `llm*.go`、B 占 `ai.go` 等），
> 依 §1.2"同名文件串行、不同文件可并"**不构成阻塞**；仅 `proto/**` 为独占脊柱（上表已预占）。

## 五、开工前置检查（照做）

1. 读本文件 → 确认目标轨**无人占用**（占位表无同轨 in-progress）。
2. **文件面隔离**：`git worktree add -b <分支> ../Emotion-Echo-<slug> <base>`（**不在共享目录切分支**）。
3. 查 `deploy/.devmode-session`（dev mode 归属锁，有人登记不得强占）。
4. **登记占位**（§三）。
5. 按 `RUNBOOK.md` 执行（起环境 / 六步循环 / 收口契约）。

## 六、收工（照做）

1. **删除本轨占位**（§三）+ 清理握手行（§四）。
2. 跑 AGENTS §2.5 三连：`git status` 干净 / `git status -sb` 与 origin 无 ahead-behind / `git branch --merged main` 无残留。
3. 删 `.devmode-session`（若是你登记的）。
4. 阶段状态回写 `roadmap.md`。

## 七、与治理体系的关系

- **AGENTS §八 双轨并行约束**（`docs/_meta/parallel-tracks.md`）是**跨大阶段**的协议（Lane E / Lane O）；本文件是 **CL 阶段内**的并行契约。两者叠加适用，冲突时以更严格者为准。
- 每条轨的**执行协议**仍以 `RUNBOOK.md` 为准；本文件只管**并行隔离与占位**。
