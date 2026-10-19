---
status: pending
stage: CL-08a
slug: cl-08-memory
type: execution-plan
owner: Lane-CL-08（开工时登记）
created: 2026-10-19
parent-plan: plan.md（CL-08 全量详档；本档是 **a 切片** 的执行版）
source-plan: docs/plans/conversation-memory-pending-decision-2026-09-24.md
related:
  - ../../roadmap.md（§2 CL-08a 行 / §4 CL-08 例外注记）
  - ../../decisions.md（CL-D-01 / CL-D-07 / CL-D-24 / CL-D-31 / CL-D-33 / CL-D-35~37）
  - ../../parallel-contract.md（§三 占位 / §1.2 spine / §1.3 共享 docker）
  - ../../RUNBOOK.md（执行六步 / 证据有效性 / 收口契约）
  - ../../../architecture/adr/adr-2026-10-agent-memory-graph.md（形态权威 · 4 项待实测项）
---

# CL-08a · 记忆（用户事实）— 切片执行计划

> **本档定位**：`plan.md` 是 **CL-08 全量**（a~e）的设计与验收详档；本档是用户 2026-10-19 裁定
> "**一步步做**" 之后，**第一个切片 CL-08a** 的可执行计划。
> **改设计改 `plan.md` / ADR；改本切片的执行改本档。**

---

## §0 本轮裁定（用户 2026-10-19）

| # | 事项 | 裁定 | 落地含义 |
|---|---|---|---|
| D1 | **阶段范围** | **一步步做**：CL-08a~e 各自独立成阶段，不在一个阶段里一次做完 | 本档只覆盖 **a（用户事实：抽取 → 存 → 注入）**。b（情绪模式）/ c（情节+图检索）/ d（管理页）/ e（风险信号）各自另开阶段 |
| D2 | **抽取粒度** | **混合**：每条 user 消息抽取 + 会话收尾（`conversation.closed`）做一次合并/失效判定 | 两条触发都要接；成本 = 每条 user 消息一次 LLM（异步，不进首字路径） |
| D3 | **删除语义** | **PG 软删 + 图物理删** | 本阶段只落 **PG 侧的列与语义**（`status='deleted'` + `deleted_at`）；**图侧删除随 c**（本阶段无图） |
| D4 | **「我的记忆」tab（原"管理页"）** | **并入本阶段**：侧边栏 tab + 自述页（回执 + 列表）+ **PG 事实关系图**；用户级开关与删除闭环仍归 d | 阶段范围从"后端切片"扩为「**自述入口 + 抽取 + 注入**」三条竖切 |
| D5 | **中间验收密度** | 「每步中间不用完整测试，要不然时间太长了」 | **见 §8 口径**：TDD 单测不放松；IAB / Playwright / smoke / 六契约 / 第二方核对**只在收口跑一次** |
| D6 | **「我的记忆」自述流存储** | **独立记忆条**：新表 `memory_note` + `POST /api/v1/memory/notes`。**不进 `conversations` / `messages`** ⇒ 天然不触发 AI 回复、情绪分析、数字人 | 事实管线多一个触发源（`source_kind='note'`）；回执由 BFF 直接返回 |
| D7 | **记忆图渲染的数据源** | **先用 PG 事实渲染**（subject → predicate → object）；c 阶段图后端上线后**只换数据源**，前端组件不动 | 本阶段不依赖 Graphiti/Kuzu |

> ⚠️ **D5 的边界（请当场纠正若理解有误）**：本档把"不用完整测试"理解为**不做全量验收**，
> **不是**不写单测 —— AGENTS §〇 是硬规则（每行新代码先 RED）。若你的意思是连单测也跳过，
> 那与仓规直接冲突，需要你明确授权并同步改 `AGENTS.md`，否则审计/门禁会判违规。

### 0.1 前序文档已定的技术口径（本阶段直接沿用，不重开）

- **拓扑**：正常会话的事实抽取走 **ai-svc 的独立 Kafka consumer group**（`ai-svc-memory`）；
  **「我的记忆」自述条** 走 ai-svc 新增的 **gRPC `MemoryService`**（BFF → ai-svc），
  遵循 `adr-2026-09-incremental-rpc-adoption:41`「新增跨进程调用一律 gRPC + proto」
  ⇒ 本阶段新增 **`proto/memory.proto`**（**不再**推到 c；c 只在此文件上补图检索方法）。
- **PG 为系统记录**：`emotion_echo_ai` schema（写入方 = ai-svc；跨 schema 读被守卫禁止）。
- **注入读路径**：BFF 只读 **Redis 用户级摘要**（`CL-D-07` 同构：用户级键 + TTL + 取不到降级），**不做 Redis+PG 双读**。
- **记忆段位置**：`base → agent → personality → **memory** → emotion → guard`（guard 恒末位；记忆为空则不产生该段）。
- **预算**：记忆段独立 **≤600 token**，与 history 的 2000 分开记账。
- **顺序与幂等**：**不改 topic 分区键**（同会话已天然有序）；幂等键 = **`(source_kind, source_id)`**
  （`message:<id>` / `note:<id>`），抽取与事实去重都按它。
- **测试替身**：单测注入**假 LLM**（不打真 LLM、不依赖网络）。
- **embedding / Graphiti / Kuzu**：**本阶段一律不碰**；图渲染先用 PG 事实（D7）
  ⇒ `plan.md` Phase 0 的 4 项 spike **全部顺延到 c**。

---

## §1 范围与非目标

**本阶段交付什么（两条竖切，共享同一套事实管线）**

> **① 正常会话**：用户在会话 A 说过的事实（"我是后端工程师""我住在北京"）→ **异步抽取**落 PG →
> 更新用户级摘要 → **新会话 B** 里 AI 能复述；事实更新时**只用新值**、旧值留史。
>
> **② 「我的记忆」自述页**：侧边栏新 tab → 用户讲自己的经历 → 落成记忆条 → 抽取进同一套事实 →
> 页面回执"记忆已更新" + 自述列表 + **PG 事实关系图**；**无 AI 回答、无情绪分析、无数字人**。

**范围内**：数据模型与迁移（含 `memory_note`）· 异步抽取 worker（混合粒度）· 自述条 gRPC 入口 ·
双时间轴失效 · 幂等 · Redis 摘要写侧 · BFF 记忆段注入 · **`proto/memory.proto` + BFF 端点** ·
**前端「我的记忆」tab（自述 + 回执 + 列表 + PG 关系图）** · 降级 · 指标 · 跨会话语义的人可见钉。

**非目标（本阶段不做）**

| 不做 | 归属 |
|---|---|
| 图后端（Graphiti/Kuzu）、三层召回、实体消解 | CL-08c |
| embedding 供给（`BAAI/bge-m3`） | CL-08c（Phase 0 spike ①） |
| BFF→llm-service 的 `user_id`（本阶段注入由 BFF 直读 Redis，不经过 llm-service） | CL-08c（届时按 `CL-F-56` 补） |
| 删除交互、用户级"关闭记忆"开关、管理页增强 | CL-08d |
| 情绪模式（趋势/触发/缓解） | CL-08b |
| 风险信号（危机拦截） | CL-08e |
| 会话摘要器 | `CL-D-31` 口径内，另项 |

---

## §2 已读功课（AGENTS §〇）

### 2.1 本阶段新增实读（2026-10-19）

| 文件 | 关键事实（决定本档怎么写） |
|---|---|
| `emotion-echo-chat-svc/internal/events/events.go:28-30` | 事件类型三件套齐备：`message.created` / `conversation.created` / `conversation.closed` ⇒ **混合粒度的"会话收尾"有现成事件**，不必新造 |
| `emotion-echo-ai-svc/internal/events/events.go` | ai-svc 侧**只声明了** `message.created` ⇒ 本阶段要补 `conversation.closed` 常量与载荷结构 |
| `emotion-echo-ai-svc/internal/logic/consumehandler.go:45-97` | 现成模板：类型过滤 → 解析 → 只处理 user → 调 LLM → 写库（`EventID` 幂等） |
| `emotion-echo-ai-svc/main.go:351-410` | 单 consumer 实例 `NewKafkaConsumer(brokers, c.Kafka.GroupID)` ⇒ **独立 group = 再建一个实例**，配置走新 env，不动既有 group |
| `emotion-echo-ai-svc/migrations/i001~i010` | 迁移编号 `iNNN_`；本阶段续 `i011`/`i012` |
| `emotion-echo-web-bff/internal/handler/prompt_assembly.go:29-91` | 段常量 + `systemSegments` 段序；空段不产生 ⇒ **新加 `memory` 段不破坏无记忆时的逐字 golden** |
| `.../prompt_assembly.go:50-66` | `assembleInput` 是加字段的位置 |
| `.../prompt_assembly.go:175-182` | `assembleMessages` 是唯一组装点；**两个调用点在 `ai_stream_handler.go:425-436` / `:497-507`**，都要传 |
| `emotion-echo-web-bff/main.go:294-298,625-634` | BFF 的 Redis client **只在 `LOGIN_LOCK_BACKEND=redis` 时才构建**（compose 默认给 `redis`；`REDIS_ADDR` 为空则回落 `emotion-echo-redis:6379`）⇒ 记忆注入不能绑死在该开关上（见 §6 R-A5） |
| `deploy/docker-compose.infra.yml:48-66` | Redis 服务 `emotion-echo-redis:6379`（无口令、无宿主端口）⇒ ai-svc 直接复用同一容器，**不新增服务** |
| `emotion-echo-ai-svc/internal/events/events.go`（`MessageCreatedData`） | 载荷含 `messageId`/`conversationId`/`userId`/`role`/`content`/`createdAt` ⇒ 抽取所需字段齐备 |
| `deploy/docker-compose.apps.yml`（ai-svc 段） | ai-svc 现有 `KAFKA_*` / `LLM_*` / `LLM_GRPC_ADDR`；**无 Redis env** ⇒ 本阶段要加 |

### 2.2 已查 ADR / 决策

`adr-2026-10-agent-memory-graph.md`（决策 45）· `CL-D-01` / `CL-D-07` / `CL-D-24` / `CL-D-31` / `CL-D-33` / `CL-D-35~37` · 架构决策 39（内存红线）/ 40（SiliconFlow 纪律）/ 45。

### 2.3 未跑 smoke 声明

本阶段**尚无代码**，且当前栈只有 4 个模型容器在跑（全栈未起）、`deploy/.devmode-session` 是 CL-06 遗留的**过期锁**（owner `lane-cl`，until 2026-10-18 23:59）。
⇒ 未跑 `scripts/smoke_data_layer.py`。**A5 收口时必须先按 §1.3 R1 用自己的后缀 tag 起栈，再跑一次**。

---

## §3 数据模型（PG · `emotion_echo_ai`）

**迁移**：`emotion-echo-ai-svc/migrations/i011_create_memory_facts.sql`（+ 必要时 `i012` 拆抽取审计表）。
**硬要求**：幂等（`compose up` 每次都会跑）· checksum 追踪 · **不跨 schema**。

### 3.1 `memory_fact`（系统记录）

| 列 | 类型 | 说明 |
|---|---|---|
| `id` | BIGSERIAL PK | |
| `user_id` | BIGINT NOT NULL | 隔离键（**所有查询必须带**） |
| `conversation_id` | BIGINT NOT NULL | 来源会话（自述条为 0） |
| `source_kind` | VARCHAR(16) NOT NULL | `message`（正常会话）/ `note`（自述条） |
| `source_id` | BIGINT NOT NULL | 来源 id（消息 id / 自述条 id）—— 幂等键之一 |
| `subject` / `predicate` / `object` | TEXT NOT NULL | 结构化三元组（抽取产物） |
| `fact_text` | TEXT NOT NULL | 注入用的可读句（如"用户是后端工程师"） |
| `confidence` | REAL | 抽取置信度（可空） |
| `valid_from` | TIMESTAMPTZ NOT NULL | 双时间轴：事实生效时间（LLM 给出则用，否则=消息时间） |
| `valid_to` | TIMESTAMPTZ NULL | **NULL = 仍然有效**；被取代时置为新事实的 `valid_from` |
| `status` | VARCHAR(16) NOT NULL DEFAULT `'active'` | `active` / `superseded` / `deleted` |
| `deleted_at` | TIMESTAMPTZ NULL | 软删（D3） |
| `created_at` / `updated_at` | TIMESTAMPTZ NOT NULL | |

**索引**

- `UNIQUE (source_kind, source_id, subject, predicate, object)` —— **抽取幂等**（同一条来源重复消费不重复入库）。
- `INDEX (user_id, status, valid_to)` —— 摘要重建与注入查询的主路径。
- `INDEX (user_id, subject, predicate, valid_to)` —— 矛盾判定（找同 subject+predicate 的 active 行）。
- `PARTIAL UNIQUE (user_id, subject, predicate) WHERE status='active'` —— 防同一键两条 active。

### 3.2 `memory_note`（自述条）

`id` BIGSERIAL PK · `user_id` · `content` TEXT · `extract_status`（`pending`/`done`/`failed`）· `created_at` · `deleted_at`。

> 「我的记忆」页写入的自述原文。**不进 `conversations` / `messages`**（D6）
> ⇒ 天然不触发 AI 回复 / 情绪分析 / 数字人。

### 3.3 `memory_extract_run`（抽取审计/幂等）

`source_kind` + `source_id`（**复合主键**）· `user_id` · `conversation_id`（自述条为 0）· `status`（`pending`/`done`/`failed`）· `facts_count` · `error` · `created_at` / `updated_at`。

> 作用：① 同一条来源**只抽一次**（`ON CONFLICT DO NOTHING`；failed 允许重试）；
> ② 失败可观测（喂 T-18 指标）；③ 为 c 阶段的图重建提供"哪些来源已抽取过"的水位线。

---

## §4 抽取 worker（ai-svc · 独立 consumer group）

**新文件**：`internal/memory/extractor.go`（接口 + 假实现可注入）、`internal/logic/memory_extract_handler.go`、`internal/repository/memory_fact.go`。

**拓扑（两个触发源，同一套抽取器）**

```
chat-events ──┬─ group "ai-svc"        → 既有情绪分析（不动）
              └─ group "ai-svc-memory" → 新增：消息事实抽取

「我的记忆」页 ─ POST /api/v1/memory/notes → BFF → gRPC MemoryService.CreateNote
                                              → ai-svc 写 memory_note + 异步抽取 → 回执
```

**混合粒度（D2）**

1. `message.created` 且 `role=user` ⇒ 抽取（每条一次 LLM）。
2. `conversation.closed` ⇒ 对**本会话已抽事实**做一次确定性合并/失效判定（不调 LLM）。
3. 「我的记忆」自述条 ⇒ 写入即异步抽取（`source_kind='note'`）。

**抽取契约（LLM 输出）**

```json
{"facts":[{"subject":"用户","predicate":"职业","object":"后端工程师",
           "text":"用户是一名后端工程师","valid_from":"2026-10-19T10:00:00Z",
           "confidence":0.9}]}
```

- **只抽"关于用户且长期有效"的事实**（偏好 / 身份 / 关系 / 长期状态）；一次性事件、寒暄、闲聊**不抽**（精确率优先，T-19）。
- `valid_from` 缺失时回落 `MessageCreatedData.CreatedAt`。
- 解析失败 ⇒ `return error`（交给既有重试/DLQ），**不得写半成品行**。

**双时间轴失效（T-3 / T-15）**

同 `(user_id, subject, predicate)` 已有 `valid_to IS NULL` 且 `object` 不同 ⇒ 同一事务内：

```sql
UPDATE memory_fact SET valid_to = $new_valid_from, status='superseded'
 WHERE user_id=$u AND subject=$s AND predicate=$p AND valid_to IS NULL AND object <> $o;
INSERT ... (新行) ON CONFLICT (source_kind, source_id, subject, predicate, object) DO NOTHING;
```

**幂等**：先把 `(source_kind, source_id)` 插入 `memory_extract_run`，冲突即跳过（避免 LLM 重复调用）；`failed` 行允许重试。

**失败边界**：抽取是**异步增强**——LLM/DB 挂只影响本 consumer / 本自述条，
**发消息链路零感知**（T-5 A/B 在收口验）；自述条失败只影响回执文案（"已记录，稍后处理"）。

---

## §5 摘要与注入

### 5.1 摘要写侧（ai-svc）

- 键：`memory:summary:user:{user_id}` · TTL **24h**（`CL-D-07` 同构）。
- 内容：按 `valid_from DESC` 取 active 事实，逐行 `- [时间] fact_text`，**≤600 token**（超限截断最旧的）。
- 写入时机：每次抽取事务提交后 **best-effort 重建**；Redis 挂 ⇒ 记 metric + warn，**不回滚 PG**。
- 需要给 ai-svc **新增 Redis 客户端**（env：`REDIS_ADDR` / `REDIS_PASSWORD` / `REDIS_DB`）。
  **不纳入 `/health/ready`**：摘要是 best-effort（Redis 挂只降级为空），把它计入 readiness 会把
  "记忆不可用"误判成"服务不可用"（与 BFF 对 nil 探针的处理同口径）。

### 5.2 注入侧（BFF）

- `segmentKindMemory = "memory"`；段序 `base → agent → personality → **memory** → emotion → guard`。
- `assembleInput.memory string`；`memorySegment()` 空串 ⇒ 不产生段（**无记忆时逐字等于现状 golden**）。
- 取数：`MemorySummaryProvider`（接口）→ Redis `GET`；**err / 空 / 超时 ⇒ ""**（增强不是依赖）。
- **两处 `assembleMessages` 调用点都要传**（`ai_stream_handler.go:425-436` / `:497-507`），沿用"字面量计数钉"。
- 预算：记忆段 ≤600 token；截断策略 = 保留最近的条目（c 阶段再引入"最相关"）。
- 运维开关（两个服务各读各的，避免一个变量两种语义）：
  ai-svc 抽取用 `MEMORY_ENABLED`；BFF 注入用 `MEMORY_INJECTION_ENABLED`（**均默认 true**）。
  ——只做运维级 kill switch，**用户级开关归 d**。

---

## §6 风险（本切片特有）

| # | 风险 | 对策 |
|---|---|---|
| R-A1 | **中文抽取质量**（指代、口语、方言） | 评测集子集（T-19 的"事实类"部分）+ 人工抽检；**不许用"演示两次通过"代替评测** |
| R-A2 | **LLM 成本/延迟** | 全异步 + 独立 group + token/耗时指标；`MEMORY_ENABLED=false` 可整体关断 |
| R-A3 | **用户隔离**（同一浏览器换账号） | 所有查询强制 `user_id`；T-17 用两账号交叉验证；摘要键带 `user_id` |
| R-A4 | **PG 与 Redis 不一致** | PG 为准；摘要**可重建**（读 miss 不阻塞，A5 提供重建脚本/命令） |
| R-A5 | **BFF Redis client 只随登录锁开关构建** | 记忆注入不得复用该开关：独立构建/注入 client，且 `LOGIN_LOCK_BACKEND` 非 redis 时也要有连接 |
| R-A6 | **ai-svc 新增 Redis 依赖** | compose 加 env + readiness 探测；Redis 不可用时**降级**（抽取照做，摘要跳过） |
| R-A7 | **迁移落点** | 只写 `emotion_echo_ai`；`scripts/test_migrations_no_service_order.sh` 会拦跨 schema |
| R-A8 | **组装层零回归** | 记忆段为空时必须**逐字**等于既有 golden（T-7）；改任何段序都先跑该钉 |

---

## §7 执行步骤（A0~A7）

| Phase | 内容 | TDD 循环（RED → GREEN） | 本 Phase 内最小验证 |
|---|---|---|---|
| **A0** ✅ | **工程准备**：登记小 PR（#260）+ 阶段分支 + draft PR（#261） | — | 分支与 PR 存在 |
| **A1** ✅ | **迁移 + 仓储**：`i011`（fact / note / extract_run）+ 模型 + planner + 仓储 | ① 迁移契约 ② 写读一致 ③ 双时间轴失效 ④ 幂等 | `go test ./migrations/... ./internal/repository/...` |
| **A2** | **抽取 worker**：事件常量 + 配置 + 独立 group + `Extractor`（LLM/假） + handler（message / note 两源） | ⑤ 同源重复 ⇒ 事实数 1 ⑥ 矛盾 ⇒ 旧行 superseded ⑦ 失败 ⇒ 返回 error（failed 可重试）⑧ LLM 输出解析容忍度 | `go test ./internal/memory/... ./internal/logic/...` |
| **A3** | **摘要写侧**：summary builder（纯函数）+ Redis 写 | ⑨ builder 排序/截断/转义 ⑩ Redis 挂 ⇒ 不 panic、PG 已提交 | `go test ./internal/memory/...`（miniredis） |
| **A4** | **注入侧（BFF）**：`segmentKindMemory` + provider + 两处调用点 | ⑪ 无记忆 ⇒ **逐字等于 golden** ⑫ 有记忆 ⇒ guard 仍末位 ⑬ provider 报错 ⇒ 段为空 ⑭ 超预算 ⇒ 截断 | `go test ./internal/handler/...`（golden + 段序 + 字面量计数钉） |
| **A5** | **契约与入口**：`proto/memory.proto`（**CreateNote / GetGraph**）+ 生成物 + `rpc_status.tsv` + ai-svc gRPC 实现 + BFF downstream/handlers（`ListNotes` **未做**：用户裁定页面"只放关系图"，自述列表无消费方） | ⑮ create 返回回执且不触发 AI ⑯ list 按用户隔离 ⑰ graph 只含该用户 ⑱ 未鉴权/越权被拒 | `go test ./...`（ai-svc / BFF）+ 契约钉 |
| **A6** | **前端「我的记忆」tab**：侧边栏入口 + 自述 composer + 回执 + 列表 + PG 关系图 | ⑲ 组件渲染与提交回执 ⑳ 关系图数据映射 ㉑ 空态/报错态 | `pnpm test`（vitest）+ 生产构建 |
| **A7** | **端到端 + 收口**（**唯一跑完整验收的地方**） | ㉒ 跨会话人可见钉 ㉓ 自述条端到端（无 AI 回复） | 起栈（本轨 tag）→ 迁移 → 两条链路实拍 → 指标上板 → `go test ./...` + `pnpm test` → 报告 + 第二方核对 |

### 7.1 测试点对照（全量 21 点中，本切片覆盖哪些）

| 覆盖 | 点 |
|---|---|
| **本阶段 PASS 目标** | T-1 T-2 T-3 T-4 T-5 T-7 T-8 T-9 T-10 T-11 T-12 T-15 T-17 T-20 |
| **本阶段部分覆盖** | T-18（抽取/摘要指标 + 一个面板 expr；完整面板随 d）· T-19（**事实类子集**）· T-21（**自述页 + PG 关系图已落地**；用户级开关/删除确认文案仍待 d） |
| **后移** | T-6（实体消解）→ c · T-13 / T-14（图后端与三层召回）→ c · T-16（删除闭环）→ d |

> 后移项**必须在 `report.md` 逐条写明"未做、归哪个阶段"**，不得用 `N/A` 掩盖（anti-patterns AP-03）。

### 7.2 提交计划（每个循环 = 一个 commit，先测试后实现）

```
test(cl-08a): 迁移契约 + memory_fact 写读契约
feat(cl-08a): i011 记忆迁移（fact / note / extract_run）+ 仓储
test(cl-08a): 抽取幂等 / 矛盾失效 / 失败可重试 / LLM 解析
feat(cl-08a): ai-svc 抽取 worker（message + note 两源）
test(cl-08a): 摘要构建排序与截断
feat(cl-08a): Redis 用户级摘要写侧
test(cl-08a): 记忆段零回归 / 段序 / 降级 / 预算
feat(cl-08a): BFF memory 段注入
test(cl-08a): MemoryService 契约（Create/List/Graph + 鉴权）
feat(cl-08a): proto/memory.proto + ai-svc gRPC + BFF 端点
test(cl-08a): 我的记忆页组件（提交/回执/列表/图映射）
feat(cl-08a): 侧边栏 tab + 自述页 + PG 关系图
chore(cl-08a): 收口——报告、指标板、账本回填
```

---

## §8 验收口径（回应 D5）

**A1~A4 之间**：只跑**本 Phase 的单测 + 编译 + lint**；**不跑** IAB / Playwright / smoke / §2.4 六契约 / 第二方核对。

**A5（收口，只跑一次）**：完整验收一次 —— 起栈 → 迁移 → 端到端 → `go test ./...` → 前端 `pnpm test` → `scripts/smoke_data_layer.py`（走网关 :19080）→ 报告四值 + 证据 → 第二方核对。

**收口判据（切片级）**

- 会话 A 埋事实 → **新会话 B** 问起时答得出（T-20，人可见）；
- 事实更新后**只用新值**、旧值可追溯（T-15）；
- 无记忆时输出**逐字等于**基线（T-7）；
- 同消息重复消费**不重复入库**（T-4）；LLM 挂**不阻塞发送**（T-5 运行时 A/B）；
- 用户隔离（T-17 两账号）。

---

## §9 交付物

1. 阶段分支 `feat/cl-08a-memory-facts` + **draft 阶段 PR**（指向 `main`）。
2. 登记小 PR（**先落 main**）：`roadmap.md` CL-08 行 → `in-progress`（注明"CL-08a 切片"）· `parallel-contract.md §三` 占位行。
3. 代码：迁移 ×2、ai-svc 抽取 worker + Redis 摘要、BFF memory 段、单测与回归钉。
4. `stages/cl-08-memory/report.md`（收口时写，含四值判定与后移项清单）。
5. 大范围测试报告与证据（A5）。

---

## §10 未决 / 需用户后续拍板

| # | 事项 | 现状 | 何时需要 |
|---|---|---|---|
| N5 | **用户级"关闭记忆"开关**（默认开；关闭 = 停抽取 + 停注入 + 保留已有） | 本阶段只做**运维级** `MEMORY_ENABLED`；用户级开关归 d | d 开工前 |
| N6 | **管理页删除确认文案** | D3 已定语义，文案待定 | d 开工前 |
| — | **评测集题目**（跨会话召回 / 精度 / A-B） | A5 前需定稿，否则 T-19 无据 | A5 前 |

---

## §11 一句话

> **本阶段只证明一件事**：用户说过的事实，跨会话能被 AI 记住并按新旧生效 —— 不碰图、不碰 embedding、不碰管理页。
