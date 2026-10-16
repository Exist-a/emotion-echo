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

### 1.0 字母体系（**唯一权威 · 2026-10-16 收口 CL-F-21**）

> **背景（`CL-F-21`）**：本契约历史上并存**两套轨字母**且**没有任何一处说明二者关系**：
> · §1.1 表用的是「**阶段面字母**」A~G（按阶段划主文件面，是**当初的建议轨名**）；
> · §三 占位表/§四 握手区用的是「**会话字母**」即各会话自命的 `Lane-CL-<X>`（按**谁在跑**命名）。
> 两套字母在 A/B 上**恰好一致**，从未暴露；到 C/D 就**完全相反** —— 按"会话字母"去 §1.1 查「C 轨独占列」
> 会读到 `pages/chat/conversation/[id].vue`（那是 **CL-05** 的面），而跑 CL-02 的会话也叫 C ⇒
> 一次误判就能让人**改到别人的文件**或**误以为自己被占**（C 轨 2026-10-15 实际发生过）。

**判定「独占列」的唯一口径**：**认「阶段号」，不认字母**。查 §1.1 时先按下表把字母换成阶段，再读那一行的主文件面。

| 阶段面字母（§1.1） | 阶段 | 会话字母（`Lane-CL-<X>`，§三/§四） | 状态 |
|---|---|---|---|
| A | CL-04 Prompt 组装层 | **A** | ✅ done（#213） |
| B | CL-01 可观测 | **B** | ✅ done（#211） |
| C | CL-05 消息链路 | **D**（⚠️ 与 C 相反） | ✅ done |
| D | CL-00 CI 瘦身 | — | ✅ done（#209） |
| E | CL-02 gRPC 契约 | **C**（⚠️ 与 E 相反） | ✅ done（#214） |
| F | CL-03 Kafka/可靠性 | **F** | ✅ done（#229） |
| G | CL-07 情绪回显+融合消费 | **G / H**（后端切片 / 前端半） | ✅ 后端 #234 · 前端 #243 |

> **新轨开工请只用「阶段号」登记**（如"CL-09 遗留清销"），避免再引入第三套字母。
> §1.1 表头已同步标注为「阶段面字母」。本块只**追加**，未改任何人的行。

### 1.1 各轨主文件面（main face）

| 轨（**阶段面字母**，见 §1.0） | 阶段 | 主文件面 | 备注 |
|---|---|---|---|
| **A** | **CL-04** Prompt 组装层 | BFF `handler/ai_stream_handler.go`、`downstream/llm_grpc.go` + `emotion-llm-service/*.py` + 前端 `composables/{useAIStreamHandler,useAIStream,usePrompt}.ts` | 解锁 CL-06/07；**尽早做** |
| **B** | **CL-01** 可观测（+ **CL-03**） | `shared/pkg/skywalking/**` + 各 svc `main.go` + BFF `downstream/{ai,analytics,assessment}.go` + `deploy/{prometheus,grafana,alertmanager,loki}` + `scripts/smoke_observability.py` | CL-01+CL-03 **同追踪面 ⇒ 建议同轨** |
| **C** | **CL-05** 消息链路 | 前端 `stores/{message,conversation}.ts`、`utils/{db,messageCache}.ts`、`composables/useConversationSender.ts`、`pages/chat/conversation/[id].vue`、`plugins/init.ts` + chat-svc `internal/{types,repository,logic,grpcserver}` + BFF `downstream/chat*.go`、`handler/chat_handler.go` + **`proto/chat.proto`** | 独立于 T1 |
| **D** | **CL-00** CI 瘦身 | `.github/workflows/**` + `scripts/**` | 独立于业务代码 |
| **E** | **CL-02** gRPC 契约 | **`proto/**`** + 各 svc `internal/grpcserver/**` + BFF `internal/downstream/**` + `scripts/check_dead_grpc_inventory.sh` | 独立，可搭任一轮 |
| **F** | **CL-03** Kafka/可靠性闭环 | chat-svc `internal/{config,events,outbox}`、`internal/repository/outbox.go`、`migrations/**`、`main.go` + ai-svc `internal/consumer/**` + analytics-svc `internal/kafka/**` + `emotion-echo-shared/pkg/{messaging,eventrow,grpcinterceptor}` + `deploy/{prometheus,grafana}` + `charts/.../prometheus/templates/configmap-rules.yaml`（内联副本） | 承接 CL-01 追踪面（B 已 done）；**不碰** 前端 / BFF / `proto/**` |
| **G** | **CL-07** 情绪回显+融合消费（**后端切片**：fusion 消费） | ai-svc `migrations/**`（新聚合视图）+ analytics-svc `internal/{repository,logic,svc}` + `proto/metric.proto` + BFF `internal/downstream/analytics{,_grpc}.go`、`internal/handler/analytics_view.go` + `emotion-echo-shared/pkg/emotionanalytics/**`（生成物） | **接手原 C 会话位**（CL-02 已 done、会话已撤）；**只做后端半**，前端半（`pages/chat/conversation/[id].vue` + `stores/message.ts`）**待 D 释放** ⇒ **不碰** |

> **不能并行**：CL-06（依赖 CL-04，且整阶段动 `[id].vue`/`stores/message.ts`）；CL-08a~e（自身有序）。
> **CL-07 例外（2026-10-16，G 轨）**：CL-07 **整阶段**与 D 轨前端面直接冲突（`[id].vue`/`stores/message.ts`）⇒ 不可并行；
> 但 **CL-07 后端切片**（融合产物消费：聚合视图 → analytics-svc 报表 → proto/BFF 透传）**不碰** 那些文件，文件面与 D/F 隔离 ⇒ 作为 **G 轨**单独开（理由见 `roadmap.md §2` 末注）。
>
> **轨字母对照纠偏（2026-10-16，F 轨补记）**：本表 A~E 是"建议轨"，实际各会话按 **Lane-CL-<字母>** 自命名，**与阶段号不一一对应** ——
> 实测 **Lane-CL-C 跑的是 CL-02（本表 E 行）**、**Lane-CL-D 跑的是 CL-05（本表 C 行）**。新增 **F = CL-03**（与前缀无歧义）。
> B 行备注写的"CL-01 + CL-03 同追踪面 ⇒ 建议同轨"**已不适用**：CL-01 已收口（#211），CL-03 独立成 F 轨开（理由见 `roadmap.md §2` 末注）。
> 该问题已由 D 轨登账 **`CL-F-21`**（两套轨字母并存致「独占列」误判，归 CL-00）；**本注只补具体对照**，不重复记账。

### 1.2 共享 spine 文件（**多轨都碰 ⇒ 必须串行 + 握手登记**）

| spine 文件 | 哪些轨会碰 | 处置 |
|---|---|---|
| **`proto/**`**（尤其 `chat.proto`） | C（分页字段）、E（死链处理） | **串行**：谁先占谁改，另一轨等 |
| **各 `<svc>/main.go` + BFF `main.go`** | B（tracing 接线）、C（chat 接线）、E（downstream/grpc 接线） | **串行 / 握手** |
| **`emotion-echo-web-bff/internal/downstream/**`** | A（llm_grpc）、B（ai/analytics/assessment HTTP 埋点）、C（chat）、E（多数） | **同名文件串行**（不同文件可并） |
| **`scripts/**`** | D（CI 全量）、B（smoke_observability）、E（dead_grpc guard） | **同名脚本串行**；D 重组前先看有没有别的轨在改脚本 |
| **`go.mod` / `package.json`** | A/C（新依赖如 `fake-indexeddb`）、B（redis client） | **追加式**（各自加行，冲突时 rebase） |

> **一句话**：主文件面基本隔离，但 **`proto` / `main.go` / `downstream` / `scripts` / `go.mod|package.json` 是脊柱**——改它们 = **先握手、后串行**。

### 1.3 共享 docker 资源（**镜像 tag / 容器** —— 2026-10-16 收口 `CL-F-09`）

> **背景（`CL-F-09`）**：`git worktree` **只隔离工作区，不隔离 docker daemon**。
> 而 compose 里自建服务用的是**固定 tag**（`emotion-echo/web-bff:v0.1.37` 这类，正是"自建镜像豁免 digest 门禁"的来源）
> ⇒ 两个 worktree 先后 build **同一个 tag**，谁后构建 tag 就指向谁；下一个人 `up -d --force-recreate`
> **就跑上了"最后构建者"的代码**。实测：A 轨 10:10 的构建把 web-bff tag 指向 A 轨代码（运行中的容器仍是旧镜像 ID）。

**四条硬规则**（worktree / 多会话并行时必须遵守）：

| # | 规则 | 做法 |
|---|---|---|
| R1 | **构建用带轨后缀的 tag**，不覆盖公共 tag | 用 `deploy/docker-compose.<track>-verify.yml` override 把 `image:` 改成 `emotion-echo/<svc>:<track>`（先例：`docker-compose.cl02-verify.yml` / `cl03` / `cl05` / `cl07`） |
| R2 | **`up` 只指名自己的服务** | `docker compose -f ... -f <override> up -d <svc> ...`，**不要**整栈 `up -d`（会把别人的服务也换成你的镜像） |
| R3 | **用前在 §3.1 播报**、并确认自己跑的是自己的镜像 | `docker ps --format '{{.Names}}	{{.Image}}'`；★ 改 Go 的验收 = **「worktree 构建镜像 + 共享目录 up 容器」两步** |
| R4 | **用毕删锁 + 说明栈上跑的是谁的镜像** | 删 `deploy/.devmode-session`，并在 §3.1 那行写清"栈仍开着，跑的是 `:<track>` 镜像" |

> **镜像不是"我的文件"，是"共享可变资源"** —— 与 `proto/**`、`main.go` 同等对待：**先声明、后使用、用完交代清楚**。

## 二、共享文件（改动前必须在下"握手区"登记一行）

`deploy/**docker-compose*.yml**` · `AGENTS.md` · `docs/closure-roadmap/decisions.md` · 顶层 `README.md` · 其他轨独占列以外的公共脚本。

> 触碰共享文件 = 先在本文件 §四 握手区登记（谁/哪个文件/为什么），改完标 ✅。

## 三、占位登记表（**正在执行** —— 开工写、收工删）

| 轨 | 阶段 | 分支 / worktree | owner | 开始 | 状态 |
|---|------|----------------|-------|------|------|
| B | CL-01 可观测闭环 | `feat/cl-01-observability`（worktree `../Emotion-Echo-cl01`） | Lane-CL-B | 2026-10-14 | ✅ **done 并已合并**（squash → `b1f13dc`，PR [#211](https://github.com/Exist-a/emotion-echo/pull/211)，**源分支已删**）；dev 栈已用毕释放（`.devmode-session` 已删） |
| C | CL-02 gRPC 契约闭环 | `feat/cl-02-grpc-contract`（worktree `../Emotion-Echo-cl02`） | Lane-CL-C | 2026-10-14 | **in-progress**（draft PR [#214](https://github.com/Exist-a/emotion-echo/pull/214)；🟢 部分已完成（Phase 1/2/3/5 + 守卫接 CI）；**🔵 2026-10-15 06:36 起持 dev 栈锁**做 Phase 2/6 运行时验证） |
| D | CL-05 消息链路闭环 | `feat/cl-05-message-pipeline`（worktree `../Emotion-Echo-cl05`） | Lane-CL-D | 2026-10-14 | ✅ **done 并已收口（2026-10-16）** —— 22 测试点全落四值、Playwright 人可见钉 4 passed/1 skipped（跳过项由 IAB 补齐）、ADR + 决策 42、`E2E-F-205/206` 已置 ✅、来源计划已迁 landed、dev 栈锁已释放。**依据**：用户裁定 `CL-D-15`。⚠️ 残留见 report §6.3（`CL-F-11` **未修复**仅关闭、`CL-F-20`、Postgres keyset SQL 无测试直接执行） |
| F | CL-03 Kafka/可靠性闭环 | `feat/cl-03-kafka-reliability`（worktree `../Emotion-Echo-cl03`，**已删**） | Lane-CL-F | 2026-10-16 | ✅ **done 并已合并**（阶段 PR [#229](https://github.com/Exist-a/emotion-echo/pull/229) **squash 合并 → `c88653b`**，源分支已删、worktree 已删）；16/16 测试点 PASS；dev 栈已用毕释放（`.devmode-session` 已删，**队列下一位 = G（CL-07）**）；第二方核对**五轮**（首轮 FAIL 9 → 复核轮 FAIL 13 → 第三轮 FAIL 7 → 第四轮 FAIL 1+3 → 第五轮 PASS；前四轮抓出 2 项真缺陷并修复）见 `stages/cl-03-kafka-reliability/report.md §6` |
| G | CL-07 情绪回显+融合消费（**后端切片**） | `feat/cl-07-fusion-consumption`（worktree `../Emotion-Echo-cl07`） | Lane-CL-G（**接手原 C 会话位**） | 2026-10-16 | ✅ **后端切片 done 并已合并**（squash → `bf401350`，PR [#234](https://github.com/Exist-a/emotion-echo/pull/234)，**源分支已删**）；**9/9 测试点 PASS + 第二方核对 PASS**（含 4 组突变实验）。⚠️ **CL-07 整体仍 in-progress** —— **前端半**（`[id].vue` 顶部状态条 / 气泡标签 / 数字人情绪）**待 D 释放该页后另开**；本轨已不再占用 dev 栈与 `proto/**` |
| H | CL-07 情绪回显（**前端半**） | `feat/cl-07-emotion-display`（worktree `../Emotion-Echo-cl07`） | Lane-CL-G（**接手原 C 会话位**） | 2026-10-16 | ✅ **done 并已合并**（squash → `349c9615`，PR [#243](https://github.com/Exist-a/emotion-echo/pull/243)，**源分支已删**）；9/10 PASS（T-10 待 CL-F-17）+ 第二方核对 PASS；**不再占用** `[id].vue` / dev 栈 / `proto` |

| 2026-10-16 | Lane-CL-P | `deploy/docker-compose.clp2.yml`（新增）、`deploy/docker-compose.clp2-grafana.yml`（新增） | P2 轨起栈验证用 override（§1.3 R1：**带轨后缀 tag `:clp2`，不覆盖公共 tag**）。前者给 user/chat/analytics-svc 换 `:clp2` 镜像 + **实验用** `STARTUP_STRICT=true`；后者**故意置空** `GF_PG_*` 做 CL-F-24 的负向对照。二者均为**实验/验证产物**，非生产配置 | ✅ |
> **登记真实性（2026-10-14 C 轨补记）**：本表在 main 上此前只剩 A 一行 `open` —— CL-00 收口（#210）清过占位后，A/B 各自把登记写在**自己的分支**上、尚未合并。
> C 轨按 §五 第 1 步"读本文件确认无人占用"时，照 main 读会得到**错误的占用状态**，故此处按**事实**回填 A/B 两行
> （依据：`feat/cl-04-prompt-assembly` / `feat/cl-01-observability` 分支上的登记 + 各自 draft PR）。**合并时请保留三行**（§四 已约定"冲突时保留双方行"）。

> 示例（不要留真数据）：`| D | CL-00 CI 瘦身 | feat/cl-00-ci-slim | Lane-CL-D | 2026-10-13 | in-progress |`

### 3.1 dev 栈时间片排队（**并行度的真实瓶颈，不是"轨数"**）

> 19 容器一套栈（稳态 ≈6GB，宿主内存上限 ≈8–10GB）⇒ **不可复制第二套**（试过：整栈复制会击穿内存）。
> 所以并行度由**栈时间片**决定：一条持锁、其余排队。**需栈的 Phase 开工前**必须在下面登记；**不得强占**持锁方。

| 顺序 | 轨 | 需要的栈窗口 | 状态 |
|---|---|---|---|
| 1 | ~~A（CL-04）~~ | 已于 2026-10-14 阶段收口时**释放**（`deploy/.devmode-session` 已删） | ✅ **已释放**（Phase 0~7 全部完成：15/16 PASS + T-14 IAB 钉绿；B/C 排队位次不变） |
| 2 | B（CL-01） | A 释放后（Phase 0 基线 / 8 采集 / 9 负向对照） | ✅ **已用毕释放（2026-10-15 06:25）** —— 收口契约 7 条全满足、第二轮独立核对 PASS、状态翻 `done`；`deploy/.devmode-session` 已删 |
| 3 | C（CL-02） | B 之后（Phase 2 清除验证 / 6 收口冒烟；"4 接真"已由裁定改判 N/A） | ✅ **已用毕释放（2026-10-15 07:40）** —— 06:36 取锁 → 7/7 healthy + Nacos count:6 + smoke 24/24 → 删 `deploy/.devmode-session`；**队列下一位 = D（CL-05）**。注意：栈仍开着，跑的是本轨 `:cl02-verify` 镜像，D 接手时按 H5 用自己后缀 tag 重建 |
| 4 | D（CL-05） | C 之后（Phase 0 基线取证 / 5 会话游标运行验证 / 7 端到端+人可见钉） | ✅ **已用毕释放（2026-10-16 07:4x）** —— 取锁 2026-10-15 20:00 → 本轨镜像 `:cl05`（H5 带轨后缀，不覆盖公共 tag）→ Phase 0 基线 / Phase 5 会话游标运行验证（gRPC 与 `CHAT_TRANSPORT=http` 两条路径各 17/17）/ Phase 7 Playwright 人可见钉（4 passed / 1 skipped）→ **已删 `deploy/.devmode-session`**。⚠️ 本轨**阶段未 done**（`CL-F-11` 未复现/根因未定位；来源计划迁移与最终签署待办）⇒ 下一位可用锁。**注意**：栈仍开着，跑的是本轨 `:cl05` 镜像（chat-svc / web-bff）。**【补记 2026-10-16 08:0x–08:4x】又取了**一次短窗口**做 IAB（内置浏览器）补验（T-20① 上翻更早页 / CL-F-11 复现尝试），**已再次释放**（`deploy/.devmode-session` 已删）；本轮结束态：锁为空、栈仍开、前端 dev 已停 |
| — | A（CL-04 **收口后追加** T-18 标题） | 2026-10-14 11:5x–12:0x 短窗口（运行时验证 T-18：重建 web-bff + 建会话首轮 → 查标题） | ✅ **已用毕释放**（`deploy/.devmode-session` 已删；位次不影响 B/C 排队） |
| 5 | ~~F（CL-03）~~ | 已于 2026-10-16 用毕**释放**（Phase 0/7 运行时取证 + 告警 firing + Phase 5 停机/恢复 A/B + 收口复核轮的证据重采；`deploy/.devmode-session` 已删） | ✅ **已释放**（栈上跑的是本轨 `:cl03` 镜像的 chat/ai/analytics 三服务；**队列下一位 = G（CL-07）**。注：栈在 2026-10-16 15:1x 已被整体停掉，与 F 轨无关） |
| 6 | G（CL-07 **后端切片**） | D → F 之后（数据契约实拍） | ✅ **已用毕释放（2026-10-16）** —— 取锁 → worktree 构建 `:cl07` 镜像（analytics-svc/web-bff）→ 20 容器 healthy + `db-migrate` 应用 `i010`/`a011` OK + Nacos `count:7` → **T-9 端到端实拍 PASS**（登录 → 建会话 705 → 发消息 951 → `emotion_analysis`/`fused_emotions` 各 +1 → 报表 `fusedEmotion.total=27`；空态日期字段缺省）→ **已删 `deploy/.devmode-session`**。栈仍开着（跑本轨 `:cl07` 镜像）。**结论=9/9 测试点 PASS → **第二方核对 PASS** ⇒ 后端切片 done 并已合并（`bf401350`，PR #234）** |

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
| 2026-10-14 | Lane-CL-D | `docs/closure-roadmap/{parallel-contract,roadmap}.md` | D 轨开工：§三 登记占位 + §3.1 排 4 位 + CL-05 翻 in-progress。按 H2/条款 8，登记走**独立小 PR**（本 PR），**不含**阶段产物；本行只追加、不改他人行 | ✅ |
| 2026-10-14 | Lane-CL-D | **`proto/**`（`proto/chat.proto`）** | D 轨**握手申请**：CL-05 Phase 5「会话列表游标化」（用户 2026-10-13 已定 D-B=B1 游标）需在 `ListConversationsRequest` 增游标字段；**与 C 轨 `proto/**` 预占冲突**。请求：① C 轨确认其 Phase 2/3 改 proto 的**时间窗与文件**；② 约定**串行**（谁先谁改）。D 轨在 C 轨答复前**不动 `proto/**`**，先把 🟢 Phase 1~4 做完 | ⏳ **待 C 轨答复** |
| 2026-10-14 | Lane-CL-D | `emotion-echo-chat-svc/internal/grpcserver/chat_server.go`、`emotion-echo-web-bff/internal/downstream/chat{,_grpc}.go` | D 轨握手：与 C 轨**同名文件**（C 轨死链治理已删/归档 `chat_server.go` 的 `StreamMessages` 桩；D 轨要改其中的 `ListMessages`/`ListConversations` 透传 + `chat.go` 的 client 接口）。依 §1.2「同名文件串行」⇒ 声明排队，**D 轨等 C 轨该文件落地后再改** | ⏳ **待排队** |
| 2026-10-14 | Lane-CL-D | **`docs/closure-roadmap/decisions.md`** | D 轨**握手**：登记本阶段 4 项用户裁定（`CL-D-11`~`CL-D-14`：SSE 不落缓存 / 前端 DTO 字段统一只登记 / `MessageWithStatus` 三形态顺延 Phase 3 / `CL-F-10`·`CL-F-11` 按原计划排 Phase 8）。⚠️ **编号从 11 起**：`CL-D-08~10` 已被 **C 轨**在其分支占用（PR #214 未合）⇒ 直接从 08 续会撞号；已在 decisions.md「使用约定」补记该占位事实。本行按 H2 走**独立小 PR**、只追加 | ✅ |

> **C 轨另注**：本轨与 A/B 在 `emotion-echo-web-bff/internal/downstream/` **同目录不同文件**（A 占 `llm*.go`、B 占 `ai.go` 等），
> 依 §1.2"同名文件串行、不同文件可并"**不构成阻塞**；仅 `proto/**` 为独占脊柱（上表已预占）。
>
> **D 轨另注（2026-10-14）**：本轨主文件面（前端 `stores/utils/composables/pages` + chat-svc `internal/{types,repository,logic}` + BFF `handler/chat_handler.go`）
> 与 B/C 两轨**不重叠**；与 C 轨仅在 `proto/**`、`chat_svc/internal/grpcserver/chat_server.go`、BFF `downstream/chat*.go` 三处相交（上表已声明排队）。
> 与 **A 轨（CL-04，已 done）** 同碰前端但**不同文件**（A 占 `composables/{useAIStreamHandler,useAIStream,usePrompt}.ts`，D 占 `stores/**`、`utils/**`、`pages/**`、`plugins/init.ts`）——A 已收口，不再构成并发。
| 2026-10-15 | Lane-CL-C | `deploy/docker-compose.cl02-verify.yml`（新增） | C 轨起栈验证用 override（H5/条款10：把 7 个服务的镜像 tag 换成 `:cl02-verify`，不覆盖公共 tag）。**第二方核对 F-6 指出此前漏登，本行补登** | ✅ |
| 2026-10-15 | Lane-CL-C | `docs/closure-roadmap/decisions.md` | C 轨登记 7 项用户裁定（`CL-D-08`~`CL-D-10`：死链逐条终态 / 模型服务边界 / 守卫形态）。**F-6 指出此前漏登，本行补登** | ✅ |
| 2026-10-15 | Lane-CL-C | **答复 D 轨两条挂起握手**（见上两行 D 轨请求）：① **`proto/**`**：C 轨改动**已落地**（`proto/{chat,metric,user,emotion_llm}.proto`：StreamMessages 与三条 mental-health 标 `deprecated`；Logout/VerifySecurityAnswer/AnalyzeBatch 已删 + 桩重生成）⇒ **C 轨不再占 proto**，D 轨可开始；建议**等 #214 合并后再改**，避免基于旧 main 回退。② **同名文件**（`chat_server.go` / `chat{,_grpc}.go`）：C 轨改动**已落地**（删 StreamMessages 桩与 BFF 死方法、`chat.go` 补 HTTP 出站埋点）⇒ **D 轨可改**，同样建议合并后 | ✅ |
| 2026-10-15 | Lane-CL-C | `emotion-echo-web/app/pages/chat/conversation/[id].vue`（**尚未改**；计划已建档） | C 轨**预占声明**：用户手动测试提出「摄像头悬浮可拖可折叠」（`docs/plans/camera-preview-floating-window.md`，D2=上方中间、D3=折叠成 pill 条已定稿），实现要动该页。该页在 §1.1 属 **C 轨独占列**，但 **D 轨（CL-05）已声明要改它**（见上两行 D 轨握手）⇒ 按 §1.2「同名文件串行」**C 轨先占**（D 轨当前仍在 🟢 Phase 1~4、未动该页）。D 轨如需先改，请在本行下回复协商；C 轨落地后会在此标 ✅ 并释放 | ⏳ **预占（待 D 轨确认）** |
| 2026-10-15 | Lane-CL-D | **`deploy/.devmode-session`（运行时锁）** + 本文件 §3.1 第 4 行 | D 轨**取锁**：队列 1/2/3 位（A/B/C）均已用毕释放，C 于 2026-10-15 07:40 释放后**队首轮到 D**。锁由本轨写入（owner `lane-cl-d`，收工即删）。按 **H2/条款 8**，本行随**独立小 PR** 落地，不含阶段产物 | ✅ |
| 2026-10-15 | Lane-CL-D | **`emotion-echo-web/app/pages/chat/conversation/[id].vue`** —— **回复上一行 C 轨的预占** | **D 轨主张本轮优先，三条理由**：① **字母歧义（关键）**：本契约并存**两套轨字母** —— §1.1 的 A~E 是**阶段面**（轨 C = **CL-05 消息链路**、轨 E = CL-02），而 §三 占位表用的是**会话字母**（C = CL-02、D = CL-05）。C 轨按**会话字母**读 §1.1，把"**C 轨独占列**"读成了自己 ⇒ 该列实为 **CL-05（D 轨）** 的主文件面（§1.1 明文列有 `pages/chat/conversation/[id].vue` 与 `composables/useConversationSender.ts`）；② **先占在先**：D 轨 2026-10-14 已在 §四 声明要改该页（握手行见上），早于 C 轨 2026-10-15 的预占；③ **C 轨尚未动该页**（对 main 及各远端分支 `git log -- [id].vue` 均 0 commit），且 **dev 栈锁在 D 轨手上** ⇒ C 轨此刻也无法开工，让 D 轨先跑**不增加 C 轨等待**，反之则卡住一个**在飞阶段**。**建议**：本轮该页由 D 轨完成 CL-05 收尾（Phase 8：`CL-F-10` 发送后不自动滚底 / `CL-F-11` 刷新后双对话），**落地后立即在本行标 ✅ 释放**给 C 轨做相机悬浮窗。**若 C 轨不同意，请在本行下回复，D 轨让位**。⚠️ 已登账本 **`CL-F-21`**（两套轨字母并存致"独占列"误判，归 CL-00） | ⏳ **待 C 轨确认** |
| 2026-10-15 | Lane-CL-D | **`docs/closure-roadmap/decisions.md`** | D 轨**勘误**：该文件 `CL-D-13` 行内的交叉引用写作 `CL-F-13`，而本轨该条已因与 B 轨同号而**改号为 `CL-F-18`**（见账本 `discovered-unresolved.md`）⇒ 就地改**本轨自己那条**的交叉引用（**不动他人行**）。随本独立小 PR 落地 | ✅ |
| 2026-10-16 | Lane-CL-D | **`deploy/.devmode-session`（运行时锁）+ 本文件 §3.1 第 4 行** | D 轨**用毕释放**：Phase 0/5/7 与 Phase 8 的栈内工作跑完，**已删锁**。⚠️ 同时如实声明：**本轨阶段未 done** —— `CL-F-11`（刷新后双对话）在 `/new` 表单路径上**未复现**、根因未定位（**不宣称修复**）；`CL-05` 的来源计划迁移 / 最终签署仍待办。栈内跑的是本轨 `:cl05` 镜像；下一位取锁者按 **H5** 用自己后缀 tag 重建 | ✅ |
| 2026-10-16 | Lane-CL-D | **`deploy/.devmode-session`（运行时锁）—— IAB 补验短窗口** | D 轨**第二次取锁**（收口后补验短窗口，先例 = A 轨 T-18 短窗口）：补 **Playwright 够不到的两项**（用户 2026-10-16 确认范围）—— ① T-20① 上翻加载更早页（Playwright 因数据前置跳过）：会话 550 滚到顶 `.dialog` **20→22**、首条换成更早一条、视口停在顶部未被拽走 ⇒ **PASS**；② `CL-F-11` 按用户确切步骤（新建页输入 + **点发送按钮** → 刷新）**仍未复现**（只建 1 个会话、总数 +1、刷新后不变；连点两次变体亦然）⇒ **根因未定位、不宣称修复**。**已再次释放**。证据 `docs/closure-roadmap/stages/cl-05-message-pipeline/evidence/iab-round-2026-10-16.md` | ✅ |
| 2026-10-16 | Lane-CL-D | **`deploy/docker-compose.cl05-verify.yml`、`deploy/docker-compose.cl05-http-transport.yml`（均新增）** + `docs/architecture/decisions.md`（ADR 索引追加）+ `docs/e2e-roadmap/discovered-unresolved.md`（`E2E-F-205/206` 状态列） | **补登（第三方核对指出漏登）**：按 §二，`deploy/**docker-compose*.yml**` 属共享文件需登记 —— 本轨起栈用了两个 override（`:cl05` 后缀 tag，H5 不覆盖公共 tag；http-transport 仅用于 T-7 把 BFF 切到降级路径验一次）。另两个是**追加式共享文档**：`decisions.md` 只追加决策 42、`E2E` 账本只改自己归属的两行状态列（append-only，未回退他人行）。**对照**：C 轨同类漏登曾在第二方核对 F-6 被指出（本轨这次是同一类问题的复发） | ✅ |
| 2026-10-16 | Lane-CL-D | **勘误（不改他人行，仅登记事实）**：本文件 §3.1 第 5/6 行（F/G）仍写「排队中（**D 持锁中**——2026-10-16 06:16 取锁）」 | D 轨**已多次释放**锁（第 4 行与 §四 均有记录；`deploy/.devmode-session` 现不存在）。按 **H3** 本轨**不代改他人行**，仅在此登记事实，**请 F/G 轨或 CL-00 刷新这两行** —— 否则下一位取锁者会误以为锁被占。（这正是 C 轨曾记过的「登记表与实际状态脱钩」同型问题） | ⏳ **待 F/G 或 CL-00 处理** |
| 2026-10-16 | Lane-CL-F | `docs/closure-roadmap/{parallel-contract,roadmap}.md` | F 轨开工：§三 登记占位 + §3.1 排 5 位 + §1.1 新增 F 行 + 轨字母对照纠偏 + CL-03 翻 in-progress。按 **H2/条款 8**，登记走**独立小 PR**（本 PR），**不含**阶段产物（阶段 PR = [#229](https://github.com/Exist-a/emotion-echo/pull/229)）；本行只追加、不改他人行 | ✅ |
| 2026-10-16 | Lane-CL-F | **`emotion-echo-chat-svc/main.go`**（脊柱）+ `emotion-echo-shared/pkg/grpcinterceptor/{tracing.go,tracing_go2sky.go}`（**仅在 M1 裁 A 时**） | F 轨**握手申请**：① CL-03 Phase 1~3 要加 outbox `trace_sw8` 列并在 relay 发布前恢复父 ctx ⇒ 落地动作含 `migrations/` 新文件 + `internal/repository/outbox.go` + `internal/outbox/relay.go` + 三条 `internal/logic/*conversationlogic.go` + `main.go`（relay 构造/fallback 段）；② 若裁定给 shared `Tracer` 接口加 sw8 原语，会动 **7 个 svc 共享**的接口与其余实现（含接口契约测试）。**D 轨（CL-05）若也需改 `main.go`，请在本行下回复协商**；F 轨按 §1.2「脊柱串行」，**先做不需 main.go 的 Phase 1 迁移，改 `main.go` 前再确认** | ✅ **已落地（2026-10-16 收口）**：D 轨（CL-05）已收口、全程**未提异议**；本轨 `chat-svc/main.go` 改动随阶段 PR [#229] 进 main（`c88653b`）。按 §六「收口时清理本轨握手行」的先例（D 轨同做法）就地更新本行状态 |
| 2026-10-16 | Lane-CL-F | `deploy/prometheus/rules/*.yml` + `deploy/grafana/dashboards/*.json` + **`charts/emotion-echo/charts/prometheus/templates/configmap-rules.yaml`**（内联副本） | F 轨握手：CL-03 Phase 4 要新增 **DLQ 堆积告警**（DLQ 硬化四问题之四）。该面属 **B 轨（CL-01）文件面**，B 已收口（#211）⇒ 依 §1.2「同名文件串行」声明占用；新增告警**必须同步 k8s 内联副本**，否则 `deploy/obs_monitoring.test.js` group ⑤ 的双向相等断言会红（现成机器守卫）；**已落地**（commit `ddb576c`：obs 契约测试全过 + helm 渲染守卫 8/8） | ✅ |
| 2026-10-16 | Lane-CL-F | **`AGENTS.md`**（§七 bullet + 页脚）、**`docs/closure-roadmap/RUNBOOK.md`**（§1 开工前置检查）、本文件 §五/§六 | **文档治理重新落地**：`#212`（「每阶段开工先建 PR」三处对齐，用户明令）因其 base 过期在 `AGENTS.md` 产生**真内容冲突**；按 **H1「禁止 rebase 他人分支 / 代他人解冲突」**，F 轨不碰其分支，改为**在最新 main 上重新落地同一意图**（本 PR），并在 #212 留评论指向后继 | ✅ |
| 2026-10-16 | Lane-CL-G | `docs/closure-roadmap/{parallel-contract,roadmap}.md` | G 轨**接手原 C 会话位**开工（C/CL-02 已 done、会话已撤）：本次接手 **CL-07 后端切片**。§1.1 新增 G 行 + §三 占位 + §3.1 排 6 位 + CL-07 翻 in-progress（**局部**）。按 **H2/条款 8**，登记走**独立小 PR**（本 PR），不含阶段产物；本行只追加、不改他人行 | ✅ |
| 2026-10-16 | Lane-CL-G | **`proto/metric.proto`**（**仅此文件**；**非** `proto/chat.proto`） | G 轨**握手声明**：Phase 3 要在 `ReportsDailyResponse` 增 `fused_emotion_distribution` + `fusion_method_breakdown`，并**重新生成** `emotion-echo-shared/pkg/emotionanalytics/metric{,_grpc}.pb.go`（protoc 32.1 + protoc-gen-go v1.36.11 + protoc-gen-go-grpc v1.6.2，RUNBOOK §2）。D 轨在改 `proto/chat.proto` ⇒ **不同文件**，依 §1.2「不同文件可并」**不构成阻塞**；此处仅登记，D/F 无需动作 | ⏳ **声明占用** |
| 2026-10-16 | Lane-CL-G | analytics-svc `internal/{repository,logic,svc}/**`（新增 `fused_report_repository.go`；改 `reports_daily_logic.go` / `servicecontext.go`）+ ai-svc `migrations/**`（新视图）+ BFF `internal/downstream/analytics{,_grpc}.go`、`internal/handler/analytics_view.go` | G 轨**握手声明**：与 **F 轨**在 analytics-svc 同服务下**不同子包**（F 占 `internal/kafka/**`；G 占 `internal/{repository,logic,svc}`），ai-svc 下也不同域（F 占 `internal/consumer/**`；G 占 `migrations/**`）；与 **D 轨**在 BFF 同目录**不同文件**（D 占 `downstream/chat*.go`、`handler/chat_handler.go`；G 占 `downstream/analytics*.go`、`handler/analytics_view.go`）。依 §1.2「同名文件串行、不同文件可并」⇒ **不构成阻塞** | ⏳ **声明占用** |

| 2026-10-16 | Lane-CL-D | **本文件 §三 占位（D 轨 · CL-05）+ §四 握手行** | D 轨**收口**：按 §六 删除/关闭本轨占位、清理握手行；阶段状态回写 `roadmap.md`（`CL-05 → done`）。同时按 **H2/条款 8** 把收口依据（用户裁定 `CL-D-15`）与**残留清单**（`CL-F-11` **未修复**仅关闭、`CL-F-20`、Postgres keyset SQL 无测试直接执行 等）一并写明，避免「done」被读成「无债」。本行随**独立小 PR** 落地 | ✅ |
| 2026-10-16 | Lane-CL-F | **本文件 §三 登记行（F 轨 · CL-03）+ §3.1 第 5 位 + 握手行状态** | F 轨**收口登记**（按 H2/条款 8 走**独立小 PR**，不含阶段产物）：CL-03 阶段 PR [#229] **squash 合并 → `c88653b`**，源分支与 worktree 均已删；§三 F 行由 `in-progress` 翻 `done 并已合并`；§3.1 第 5 位标「已用毕释放」（**队列下一位 = G**）；本轨那条 `chat-svc/main.go` 脊柱握手申请标 **已落地**（D 轨 CL-05 已收口、全程未提异议；本轨改动随 #229 进 main）。**本行只追加**，未改他人行 | ✅ |

| 2026-10-16 | Lane-CL-G | `deploy/docker-compose.cl07-verify.yml`（新增） | G 轨起栈验证用 override（H5/条款10：把 analytics-svc / web-bff 的镜像 tag 换成 `:cl07`，不覆盖公共 tag）。**本行补登**（登记 PR #232 时该文件尚未产出） | ✅ |

| 2026-10-16 | Lane-CL-P | `docs/closure-roadmap/{parallel-contract,discovered-unresolved,}.md`、`scripts/check_git_layout.py` + 新增 `scripts/test_check_git_layout.py`、`.github/workflows/doc-drift-check.yml`（**仅加一个 step**）、前端 `app/{utils/{messageCache,db}.ts,stores/message.ts,types/**,components/digital-human/**}` | P 轨（**CL 遗留清销第 1 批**）开工：① §1.0 新增「字母体系（唯一权威）」收口 `CL-F-21`；② §三 登记占位；③ 账本翻正 `CL-F-01/03`、关闭 `CL-F-06/17/18/20`。**无并发持有者**（A~H 轨均已 done 并合并；CL-02 的 `proto/**` 预占已随 #214 释放）。按 §1.2「不同文件可并」不构成阻塞；本行只追加、不改他人行 | ✅ |
| 2026-10-16 | Lane-CL-G | 前端 `pages/chat/conversation/{new.vue,[id].vue}` + `components/emotion/**`（新）+ `composables/useCurrentEmotion.ts`（新）+ `utils/emotion.ts` + `types/api.ts` + `configs/apiRoutes.ts` | G 轨**续 CL-07 前端半**（H 轨登记）：这些文件属 D 轨（CL-05）此前主张的 `[id].vue` 面，**D 轨已 done 并释放该页**（roadmap CL-05 已翻 done）⇒ 现无并发持有者。依 §1.2 声明占用；纯前端 + 新文件为主 | ⏳ **声明占用** |

## 五、开工前置检查（照做）

1. 读本文件 → 确认目标轨**无人占用**（占位表无同轨 in-progress）。
2. **文件面隔离**：`git worktree add -b <分支> ../Emotion-Echo-<slug> <base>`（**不在共享目录切分支**）。
3. **先开 PR（draft）再开工**（**2026-10-14 起强制，用户明令**）：`git push` 分支 → 建指向 `main` 的 **draft PR** → 在 §三 占位行**一并填 PR 链接**。**无 PR 不得开工**。
   （顺序说明：占位登记按 H2 走**独立小 PR 先落 main**，故首次登记的 PR 链接可能后补——以 §三 行为准。）
4. 查 `deploy/.devmode-session`（dev mode 归属锁，有人登记不得强占）。
5. **登记占位**（§三，含 PR 链接）。
6. 按 `RUNBOOK.md` 执行（起环境 / 六步循环 / 收口契约）。

## 六、收工（照做）

1. **删除本轨占位**（§三）+ 清理握手行（§四）。
2. 跑 AGENTS §2.5 三连：`git status` 干净 / `git status -sb` 与 origin 无 ahead-behind / `git branch --merged main` 无残留。
3. 删 `.devmode-session`（若是你登记的）。
4. 阶段状态回写 `roadmap.md`。
5. **PR 处理**：阶段收口时把 PR 转 ready，按 §四 握手**串行合并**（H8），合并后删源分支（AGENTS §2.5）。

## 七、与治理体系的关系

- **AGENTS §八 双轨并行约束**（`docs/_meta/parallel-tracks.md`）是**跨大阶段**的协议（Lane E / Lane O）；本文件是 **CL 阶段内**的并行契约。两者叠加适用，冲突时以更严格者为准。
- 每条轨的**执行协议**仍以 `RUNBOOK.md` 为准；本文件只管**并行隔离与占位**。

## 八、跨轨沟通（2026-10-14 起，用户明令）

> **背景（真实教训）**：2026-10-14 多轨并行时，**C 轨把"并行告知"写在了 B 轨的阶段 PR #211 评论里，而 B 轨从未查看** —— 消息静默丢失，直到用户提醒才发现。跨轨沟通是**好事**，但必须**约定渠道 + 强制查看**，否则等于没发。

### 8.1 渠道（按优先级）

1. **对方阶段 PR 的评论**（首选）：GitHub PR comment —— 天然按轨隔离、**不产生文件冲突**（优于往 §三/§四 表里塞消息）。
2. **本文件 §四 握手区**：涉及**共享文件 / 共享资源**（`proto/**`、镜像 tag、dev 栈位次、`decisions.md` 编号）时，除 PR 评论外**再**在 §四追加一行（**append-only**，只追加自己那行）。
3. 紧急 / 需要用户拍板：由用户转达。

### 8.2 消息格式（写清五件事）

```
### 来自 <轨>（Lane-CL-<X>）的 <告知 | 请求 | 答复>
- 我是谁：<轨 + 阶段，如 B / CL-01>
- 找谁：<目标轨，如 C / CL-02>
- 什么事：<一句话>
- 需要你做什么：<明确动作；或"仅告知，无需动作">
- 期望回应时间：<时间点；或"不阻塞">
```

### 8.3 强制查看义务（**每轮开工第 0 步 + 收工前各一次**）

- **开工第 0 步**：先确认**自己是哪条轨 / 哪个阶段**（读 §三 找到自己那行，别搞错身份），然后：
  1. 查**自己阶段 PR 的评论**（REST：`GET /repos/{owner}/{repo}/issues/<PR>/comments`）—— 有没有发给自己的消息；
  2. 查 §四 握手区**有没有新增行**（尤其"待 XX 轨答复"的行）；
  3. 有发给自己的 ⇒ **当场回应**（回 PR 评论；涉及共享资源再在 §四追加一行）。
- **收工前**：再查一次（防"干完才发现有人在等答复"）。
- **不查看 = 违规**（按 AGENTS §六 处置）：消息静默丢失会让别的轨**停工等待**（D 轨的 `proto/**` 握手就在等 C 轨答复）。

### 8.4 一句话

**开工先报身份 → 查自己 PR 评论 + §四 新行 → 有问必答；收工再查一次。**
