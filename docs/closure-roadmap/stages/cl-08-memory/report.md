---
status: done
stage: CL-08a
slug: cl-08-memory
created: 2026-10-19
updated: 2026-10-21
plan: cl-08a-execution-plan.md
closed_by: 用户 2026-10-21 指示收口（"先收口 a，三方核对用子代理"）；第二方核对由独立子代理完成，见 §6
---

# CL-08a · 记忆（用户事实 + 「我的记忆」自述入口）— 阶段报告

> **结论：`done`（2026-10-21 收口）**
> 两条主链路（正常会话事实 / 自述条 → PG → Redis 摘要 → prompt 注入）在真栈端到端跑通；
> **21 个测试点全部有结论**（PASS 17 · N/A 4，其中 3 项明确移交 c、1 项移交 d）；
> 收口前补完了 2026-10-19 遗留的运行态 A/B，并在补测过程中**又抓到 5 条问题**（§2.3：产品缺陷 3 + 工具/测试 2）。
> 第二方核对 = 独立子代理只读审查，**首轮判 FAIL（4 处收口声明不成立）**，整改记录见 §6。

---

## §0 摘要

| 项 | 状态 |
|---|---|
| 阶段 PR | [#261](https://github.com/Exist-a/emotion-echo/pull/261)（squash → `505e9134`，2026-10-19） |
| 残留清销 PR | [#267](https://github.com/Exist-a/emotion-echo/pull/267)（squash → `749be05e`）：抽取卡 pending 修复 + PG 集成测试 + E2E 去固定等待 |
| 幻觉日期护栏 PR | [#268](https://github.com/Exist-a/emotion-echo/pull/268)（squash → `5731e857`）：`valid_from` 校验 + 运行态证据 + 账本 |
| 收口文档 PR | 本 PR（report 翻终态 + roadmap/契约回填） |
| 运行镜像 | `emotion-echo/{ai-svc,web-bff,web}:cl08ares`（轨道后缀 tag，未覆盖公共 tag），经 `deploy/compose.track-override.yml` 挂载 |
| 迁移 | `i011_create_memory_facts.sql`（幂等，db-migrate `OK`） |

**交付形态**（用户 2026-10-19 两次裁定的合成）：

1. **正常会话**：`message.created` → ai-svc 独立消费组 `ai-svc-memory` → LLM 抽事实 → PG（双时间轴）→ Redis 用户级摘要 → BFF `memory` 段注入。
2. **「我的记忆」页**：侧边栏 tab → `POST /memory/notes` → 独立 `memory_note`（**不进 conversations/messages**）→ 异步抽取 → 固定回执「记忆已更新」→ 中心辐射关系图（由 PG 事实渲染）。

---

## §1 测试点判定

> 四值：`PASS` / `FAIL` / `BLOCKED` / `N/A`。判定依据 = 证据文件或命令输出，不是"已实现"。

| # | 测试点 | 判定 | 依据 |
|---|--------|------|------|
| T-1 | 迁移幂等 | **PASS（证据强度不足）** | 文件全 `IF NOT EXISTS` + 契约测试钉住；但**没有真的跑两遍**，引用过的 db-migrate 日志也未留档 ⇒ 已登账 `CL-F-70` |
| T-2 | 事实行写读一致 | **PASS** | 内存契约测试 + **PG integration test**（`TestPostgresMemoryFactRepo_ApplyFacts_SupersedeKeepsHistory`，2026-10-20 新增） |
| T-3 | 双时间轴 | **PASS** | 运行时：上海行 `status=superseded`、`valid_to` 置位；北京行 active（`runtime-e2e-2026-10-19.txt` §3）+ PG 集成测试强断言 `valid_to == 新行 valid_from`。⚠️ 收口轮发现「消息来源事实的 `valid_from` 单位错」（§2.3-5）已修；修后摘要行显示 `2026-10`（见 `t5-t17-runtime-2026-10-21.txt`） |
| T-4 | 抽取幂等 | **PASS** | 单测：同源重复消费 ⇒ LLM 只调 1 次；唯一索引 `(source_kind,source_id,subject,predicate,object)` |
| T-5 | 抽取失败不阻塞主链路 | **PASS**（2026-10-21 补运行时） | 把 ai-svc 的 `LLM_BASE_URL` 指到不可达：消息照发、落库 1 行；run 落 `failed`（错误原文 `memory: call llm: Post …`）；恢复后新消息 `done\|2` 且事实进库（`t5-t17-runtime-2026-10-21.txt`；**2026-10-22 复跑 11/11 PASS**，见 `t5-t17-runtime-2026-10-22.txt`） |
| T-6 | 实体消解（中文指代） | **N/A → c** | 需图/实体层，本阶段不做 |
| T-7 | 无记忆时逐字零回归 | **PASS** | `TestPromptAssembly_NoMemory_ByteIdenticalToGolden`（等于 `goldenBase`） |
| T-8 | 段序不变量 | **PASS** | `TestPromptAssembly_MemorySegmentOrder`：base→agent→memory→emotion→guard；两处 `assembleMessages` 都传 memory |
| T-9 | 记忆段内容正确 | **PASS** | 运行时：新会话问"还记得我做什么的、住哪" ⇒ 复述职业+城市 |
| T-10 | 关键路径不查图 | **PASS**（架构级） | 注入路径只读 Redis（`downstream.MemorySummarySource`），无图/DB 调用 |
| T-11 | 降级 | **PASS（证据强度不足）** | handler 层四种降级有单测；但 **BFF 真实 Redis 读取路径**（`redisMemorySummary` 的 `redis.Nil` / 300ms 超时分支）**无测试**，只有 fake source ⇒ 已登账 `CL-F-70` |
| T-12 | 预算 | **PASS** | 写侧 600 token（超限丢最旧）+ 读侧 1200 rune 兜底 |
| T-13 / T-14 | 图后端 / 三层召回 | **N/A → c** | Graphiti+Kuzu 顺延 |
| T-15 | 事实更新端到端 | **PASS** | 上海→北京；摘要同步为北京（`runtime-e2e-2026-10-19.txt` §3/§4） |
| T-16 | 删除闭环 | **N/A → d** | 用户裁定：删除全归 CL-08d |
| T-17 | 用户隔离 | **PASS**（2026-10-21 补运行时） | 双账号（echo uid=1 / smoke_user uid=2）：PG 事实双向不交叉；Redis `memory:summary:user:<id>` 双向不交叉（`t5-t17-runtime-2026-10-21.txt`；**2026-10-22 复跑 11/11 PASS**，见 `t5-t17-runtime-2026-10-22.txt`） |
| T-18 | 指标被消费 | **PASS** | 4 个 collector 在 `/metrics` 暴露、被 Prometheus 抓取；business dashboard 5 个面板；`obs_monitoring.test.js` 组 ⑨ 双向钉住（`t18-metrics-2026-10-19.txt`） |
| T-19 | 评测集（价值证明） | **PASS**（精确率子项 N/A → c） | `scripts/eval_memory_recall.py`：PASS=10 / FAIL=0 / N/A=1；4/4 跨会话召回 + 事实更新 + 开/关 A/B（`t19-eval-2026-10-19.txt`） |
| T-20 | 跨会话人可见钉 | **PASS** | `e2e/cl-08a-memory.spec.ts` **2 passed**：2026-10-21 `43.9s`、**2026-10-22 复跑 `44.3s`**（`evidence/t20-playwright-2026-10-2{1,2}.txt`）；UI 写自述 → 新会话复述「云舟 / 插画师」 |
| T-21 | 记忆可见性口径 | **PASS**（用户裁定） | 只放关系图（中心辐射）；页面见 `evidence/iab-*.png` |

**数量**：21 点 = PASS 17 · N/A 4（T-6/13/14 → c，T-16 → d）· FAIL 0 · BLOCKED 0。
> 注：2026-10-21 第二方核对**抓出这里原写「PASS 15」是错的**（逐行数应为 17），已改正；同错还出现在 roadmap / parallel-contract 两处，一并修正。

---

## §2 已实现清单

**代码（按 TDD 循环提交）**

- `emotion-echo-ai-svc/internal/model/memory_fact.go`：`MemoryFact` / `MemoryNote` / `MemoryExtractRun`
- `.../repository/memory_fact_planner.go`：双时间轴计划（纯函数）+ 会话收尾确定性合并
- `.../repository/memory_fact_repository.go`：接口 + 内存替身 + Postgres 实现；**2026-10-20 新增 `ExtractRunLease`**
- `.../memory/llm_extractor.go`：LLM 抽取器；**2026-10-20 新增 `sanitizeValidFrom` 护栏**
- `.../memory/summary_store.go`：Redis 用户级摘要（`memory:summary:user:<id>`，TTL 24h）
- `.../logic/memory_extract_handler.go`：message / note 两源共用 `extractSource`
- `.../grpcserver/memory_{server,graph}.go`：`MemoryService`（CreateNote / GetGraph）
- `proto/memory.proto` + 生成物 `shared/pkg/emotionmemory`
- `emotion-echo-web-bff/internal/{downstream,handler}/memory_*`
- `emotion-echo-web/app/pages/chat/memory.vue` + `utils/memoryGraph.ts` + `types/memory.ts`
- `emotion-echo-web/app/{layouts/nav.vue, router.options.ts, lib/apiRoutes.ts}`

**测试**

| 套件 | 结果 |
|---|---|
| `go test ./...`（ai-svc，**清空 HTTP_PROXY**） | **18/18 包全绿**（含 `internal/grpcserver` 111s ok）—— 见 §7 |
| `go test -tags integration ./integration_test/ -run TestPostgresMemoryFactRepo` | **4/4 PASS**（2026-10-20 新增） |
| `go test ./...`（web-bff） | 全绿（含路由契约 50 条） |
| `pnpm test`（web） | 879 passed / 103 files |
| `pnpm exec playwright test e2e/cl-08a-memory.spec.ts` | **2 passed**（2026-10-21 复跑） |
| `scripts/check_rpc_status.sh` / `check_proto_layout.py` | PASS |

### §2.1 运行态抓到的真缺陷（2026-10-19，已修）

| 缺陷 | 现象 | 根因 | 修复 |
|---|---|---|---|
| **GraphChart 未注册** | 生产构建下图表区完全空白：容器在、`_echarts_instance_` 已挂、**canvas 数为 0**、控制台无报错 | `nuxt.config.ts` 的 `nuxt-echarts` 树摇只注册 Bar/Line/Pie/Radar，`graph` 未注册 ⇒ 静默不画 | 注册 `GraphChart` + vitest 契约钉；重建后才通过 |

> 只有真浏览器能抓到：typecheck / vitest / `pnpm build` 全绿。

### §2.2 第二方核对抓出的问题（2026-10-19，**部分已修**：第 1 项的越权面仍未关闭，见 `CL-F-66`）

| # | 问题 | 影响 | 修复 |
|---|---|---|---|
| 1 | `MemoryService` 信任请求体 `user_id` | 本机进程可用 `x-user-id:1` + body `user_id:2` 越权读写他人记忆 | 身份只取 metadata；不一致 ⇒ `PermissionDenied`；补 2 个负向测试。⚠️ **2026-10-21 核对指出：只修了「两值不一致」这一形态** —— ai-svc gRPC 未挂 internal auth 拦截器且 `8892` 发布到宿主，本机进程把 metadata/body 都写成同一 victim id 仍可越权 ⇒ 已登账 **`CL-F-66`（高）**，本阶段**不宣称越权面已关闭** |
| 2 | `conversation.closed` protobuf 解码断路 | `Data` 恒 nil ⇒ 会话收尾合并在生产**从不执行**；单测因手工构造 Data 而假绿 | 补解码 + 改测试断言 `Data` 三字段 |
| 3 | 双时间轴乱序反转 | 迟到的旧事实让较新 active 下线（旧值复活 + 零长度区间） | `newerActiveFor`：迟到旧事实落为已关区间历史行；测试改强断言 |
| 4 | BFF 读摘要无独立超时 | Redis 不可达时每次对话先阻塞 ~5s | 300ms 硬超时，超时按"无记忆"降级 |
| 5 | 计划文档与代码不一致 | 误导后续实现 | 逐条更正 |

### §2.3 残留清销轮补测抓出的真缺陷（2026-10-20/21，已修）

| # | 缺陷 | 机制（读码确证） | 修复 | 证据 |
|---|---|---|---|---|
| 1 | **抽取运行卡 `pending` ⇒ 该来源永久不再抽取** | `FinishExtractRun(done)` 失败只 `return error`，行仍 pending → 重投时 `TryMarkExtractRun` **只对 `failed` 放行** → `first=false` → 跳过；仓内无 sweeper | ① handler 回填失败时 best-effort 落 `failed` 再返回错误；② `ExtractRunLease=10m`：pending 超租约视为孤儿，允许重抽并续租（内存/PG 同判据） | PR #267；`TestMemoryExtractHandler_FinishDoneFailure_LeavesRetryableState`、`TestMemoryFactRepo_InMemory_TryMarkExtractRun_StalePendingIsRetryable` |
| 2 | **LLM 幻觉 `valid_from` 锁死事实** | 抽取器把 `58771-09-12T00:00:00Z` 直接落库 ⇒ 该 active 的 `valid_from` 在遥远未来 ⇒ planner 乱序保护把之后**所有**真实日期的新说法降级为 `superseded` ⇒ 用户改口后图谱/注入永远停在旧值，且不自愈 | `sanitizeValidFrom`：只接受 `[1970-01-01, now+24h]`，越界回落来源时间 | PR #268；2 条新单测先红后绿；dev 库污染行就地修正 |
| 3 | E2E 固定 `waitForTimeout(20_000)` | 慢则假红、快则白等 | 读 `access_token` cookie 直连网关轮询 `/api/v1/memory/graph`（`page.reload` 与客户端导航两条路都实测失败，原因写入注释） | E2E `2 passed`（43.9s；原光盲等就 20s） |
| 4 | TDD 门禁遇二进制改动崩溃 | `git numstat` 对 PNG 返回 `-`，脚本拿去做整数比较 ⇒ `integer expected` ⇒ 把"脚本坏了"伪装成"违反 TDD" | 非纯数字跳过 | PR #268 |
| 5 | **事件时间戳单位不一致 ⇒ 所有消息来源事实的 `valid_from` 落在 58773 年** | chat-svc 发布 `createdAt` 用 **UnixMilli**（`sendmessagelogic.go:116` / `proto_marshal.go:31`），ai-svc `occurredAt()` 按**秒**解析 ⇒ 事实时间 ≈ 5.8 万年后；双时间轴排序全错，**摘要直接写出「58773-03 用户是一名木工」注入 prompt**。上一个护栏（`sanitizeValidFrom`）只挡 LLM 给的日期，挡不住这条回落路径 —— 说明"打补丁"必须先找对根因 | `occurredAt()` 加单位判据：`>= 1e11` 按毫秒解析，否则按秒（1e11 秒 ≈ 5138 年，两种单位都安全） | 收口轮（本 PR）；`TestOccurredAt_AcceptsMillisecondsFromEvent` 先红（实得 `58774-02-05`）后绿；dev 库 **11 行 `valid_from` + 17 行 `valid_to`** 均已就地修正（首轮只修了 `valid_from`，被第二方核对抓出，见 §6） |

> **教训（写进流程候选）**：本阶段 3 个真缺陷（GraphChart / 解码断路 / valid_from 幻觉）**全部只在真栈上暴露**，
> 单测、类型检查、生产构建全绿。⇒ "能编译 + 单测绿"不能当收口依据。

---

## §3 未完成 / 后移（如实记录）

| # | 项 | 归属 | 说明 |
|---|---|---|---|
| 1 | **抽取失败的补偿（sweeper）** | **CL-09 / `CL-F-62`** | 实测：LLM 不可达窗口内失败的那条**永远不会被重抽**（Kafka offset 已提交，无 sweeper）⇒ 事实永久缺失。本阶段的租约只解决"重投发生时"的放行 |
| 2 | 用户级「关闭记忆」开关 | CL-08d | 当前只有运维级 `MEMORY_ENABLED` / `MEMORY_INJECTION_ENABLED` |
| 3 | 删除闭环 | CL-08d | 用户裁定 |
| 4 | 图后端 / 三层召回 / 实体消解 | CL-08c | 当前图由 PG 事实渲染，切数据源即可（另见 `CL-F-56/57/58`） |
| 5 | 内存替身的「陈旧计划」语义 | CL-09 | PG 由 `uq_memory_fact_active_key` 拦下并报错（可重投收敛）；内存替身静默放过（会短暂两条 active）。只把 PG 行为钉死，未改替身 |
| 6 | 关系图标签重叠 | **CL-11 / `CL-F-64`** | 节点多时标签互相压字（见 `evidence/iab-2-memory-after-note-2026-10-21.png`），可读性差 |
| 6b | **其它消费者是否有同款时间单位问题** | CL-09 | 本轮只修了记忆链路的 `occurredAt`；`emotion` 消费链路（`consumehandler.go`）未逐一核对是否也存在秒/毫秒混用（`CL-F-65` 的遗留半场） |
| 7 | 「不相关不注入」（精确率） | CL-08c | T-19 记 N/A：本阶段是**全量摘要常带注入**（≤600 token），按需相关性检索归 c |
| 8 | **`conversation.closed` 的触发语义与计划不符** | CL-09 / `CL-F-67` 同批 | 全仓唯一生产者是 `DeleteConversation`（`deleteconversationlogic.go:81-90`）⇒ **正常结束会话不会触发**；且它要合并的「同键两条 active」在 PG 里被 `uq_memory_fact_active_key` 禁止（集成测试反向证明）⇒ 该触发在生产形态下近乎死代码。"解码通了"≠"功能可用" |
| 9 | **`ListActiveByConversation` 不带 `user_id`** | CL-09 / `CL-F-67` | 仓储文件头声称「所有查询强制带 user_id」，实际该方法与 `handleConversationClosed` 都只用 `conversation_id`、忽略事件里的 `data.UserID` |
| 10 | **租约(10m) 远长于消费重试窗口(~14s)** | CL-09 / `CL-F-68` | DB 在 `TryMark` 之后失联时，3 次原地重试都会因「租约内 pending」被判 `first=false` 并当成功返回 ⇒ offset 前进、事实丢失 |
| 11 | 等时时间戳仍产生零长度区间 | CL-09 / `CL-F-69` | `planner` 只夹住「负区间」，`valid_from == valid_to` 的情况仍在 |
| 12 | 测试强度不足项 | CL-09 / `CL-F-70` | ① T-11 的真实 Redis 路径无测试；② 迁移幂等只有字符串断言、没真跑两遍；③ 指标测试读全局计数器且并行；④ 会话收尾合并测试手工造 PG 不允许的状态 |

---

## §4 环境与镜像（R4 交代）

- **2026-10-22 复测轮实测快照**：18 容器 healthy（另一条轨道未起 `ai`/`obs` profile）；`ai-svc` / `web-bff` / `web` 跑 **`:cl08ares`**（经 `deploy/compose.track-override.yml` 挂载），其余公共 tag。
- `deploy/.devmode-session`：owner `lane-cl-08a-closeout`，until 2026-10-22 23:59（用户 09:38 明确授权，接管自 `lane-dachuang-support` 的结题截图会话）。
- 复测轮的 BFF↔ai-svc 现象：**重建 ai-svc 后 BFF 仍连旧 IP**（`no route to host` 或 5s 超时），需 `docker restart emotion-echo-web-bff` + 重跑 `apisix-seed` 才恢复 ⇒ 已并入 `CL-F-47` 同族证据。
- ⚠️ **公共 tag 全面滞后**（`CL-F-60` 补记）：`user-svc:v0.1.5`（无 `ListAgentProfiles`）、`web-bff:v0.1.37`（`/api/v1/memory/*` 404）、`web:v0.1.9`（`.output` 里没有「我的记忆」）⇒ **按文档起栈得到的界面里没有本功能**，必须用轨道 tag 重建。
- 已知环境噪声：首次 `CreateConversation` 仍可能撞 5s 截止（`E2E-F-115` 既有账，非本阶段引入）。

---

## §5 证据清单

| 文件 | 内容 |
|---|---|
| `evidence/runtime-e2e-2026-10-19.txt` | 2026-10-19 运行时端到端（DB 快照 / 摘要 / 回复原文） |
| `evidence/t18-metrics-2026-10-19.txt` | T-18 指标被采集（Prometheus 实查） |
| `evidence/t19-eval-2026-10-19.txt` | T-19 评测集逐题结论 |
| `evidence/t20-playwright-2026-10-19.txt` | T-20 Playwright 首次通过记录 |
| `evidence/t20-memory-page.png` | 关系图渲染截图（E2E 产出） |
| `evidence/t20-playwright-2026-10-21.txt` | **收口轮 E2E runner 原始日志**（`2 passed (43.9s)`，逐条耗时） |
| `evidence/iab-1-memory-page-2026-10-21.png` | **收口轮浏览器实测**：「我的记忆」页 + 侧边栏入口 + 关系图 |
| `evidence/iab-2-memory-after-note-2026-10-21.png` | **收口轮浏览器实测**：写自述 → 回执「记忆已更新」→ 图新增节点 |
| `evidence/t5-t17-runtime-2026-10-21.txt` | T-5 / T-17 运行时 A/B 输出（2026-10-21 重跑） |
| `evidence/t5-t17-runtime-2026-10-22.txt` | **复测轮 T-5 三段 + T-17，11/11 PASS**（基线 / LLM 不可达 / 恢复） |
| `evidence/t20-playwright-2026-10-22.txt` | **复测轮 E2E runner 日志**（`2 passed (44.3s)`） |
| `evidence/go-test-all-2026-10-22.txt` | **ai-svc 全量单测 18/18 包全绿**（清空 `HTTP_PROXY` 后，含 grpcserver 111s ok） |
| `evidence/iab-1-memory-page-2026-10-22.png` · `iab-2-memory-after-note-2026-10-22.png` | 复测轮页面实测（`console errors = []`，写入「汤圆/柴犬」后图更新） |

---

## §6 第二方核对（独立子代理，2026-10-21）

> 用户 2026-10-21 指定「三方核对用子代理」。本代理为**独立只读审查**（不共享本轮执行者的上下文），
> 任务被明确要求为「证伪」而非背书，覆盖 A~H 八项：测试点可核查性 / 五条修复真实性 / TDD 真伪 / 安全 /
> 假绿测试 / 文档一致性 / 自主发现的缺陷 / 实跑单测。

### 6.1 首轮结论：**FAIL** ——「功能没做」不成立，但「收口声明」有 4 处不成立

| # | 抓出的问题 | 严重度 | 处置 |
|---|---|---|---|
| 1 | **PASS 数写错**：报告/roadmap/契约三处都写「PASS 15」，逐行实数应为 **17** | 文档准确性 | ✅ 三处已改 |
| 2 | **契约行自相矛盾**：先写「done」，同一单元格末尾仍留着旧的「状态是 partial 不是 done，残留 T-5/T-17…」 | 文档准确性 | ✅ 已删除过期尾巴 |
| 3 | **§6 是空的却被三处引用为「已完成」** | 流程诚实性 | ✅ 本节即补 |
| 4 | **数据修复只做一半**：`valid_from` 修了，**17 行 `valid_to` 仍是 58771/58773** | 数据正确性 | ✅ 已修（`valid_to = valid_from + 1s`），复验 0 行 |
| 5 | **grpcserver 失败数低报**：报告写「4 个子用例」，实测 **13 个顶层用例** | 数字准确性 | ✅ 已改 |
| 6 | **「再修 2 个真缺陷」与 §2.3 的 5 行对不上** | 表述 | ✅ 改为「5 条（产品 3 + 工具/测试 2）」 |
| 7 | **安全：越权面只关了半扇门**（见下） | **高** | ⚠️ **未修**，登账 `CL-F-66` |
| 8 | **会话维度查询不带 user_id**、`conversation.closed` 语义与计划不符 | 中 | ⚠️ 未修，登账 `CL-F-67` + §3-8/9 |
| 9 | **租约 10m 远长于重试窗口 ~14s** ⇒ pending 被当成功吞掉 | 中 | ⚠️ 未修，登账 `CL-F-68` |
| 10 | **等时时间戳仍写出零长度区间**（§2.2-3 只修了负区间） | 低 | ⚠️ 未修，登账 `CL-F-69` |
| 11 | **四处测试强度不足**（T-11 真实 Redis 路径无测试、迁移幂等只断言字符串、指标读全局计数、会话收尾测试造 PG 不允许的状态） | 中 | ⚠️ 未修，登账 `CL-F-70`；T-11 行已改为「PASS（证据强度不足）」 |

### 6.2 首轮指出的「不能算 done」的两条硬伤（已按此调整收口口径）

**① 安全（`CL-F-66`，高）**：`ai-svc` gRPC 只挂了 userid 提取拦截器、**没有 internal auth**，而 `8892` 发布到宿主。
`memoryCallerID` 只拦「body 与 metadata 不一致」，攻击者把两者写成同一个 victim id 即可。
⇒ 本报告**不再声称越权面已关闭**，§2.2-1 已加限定。

**② `conversation.closed` 近乎死代码（`CL-F-67`）**：唯一生产者在 `DeleteConversation`，
正常结束会话不会触发；且它要合并的「同键两条 active」被 PG 唯一索引禁止。
⇒ 已在 §3 登记为已知边界，**不把「解码通了」当成「会话收尾功能可用」**。

### 6.3 本轮未按子代理建议做的事（及理由）

| 建议 | 为什么不现在做 |
|---|---|
| 立刻给 MemoryService 加 internal auth | 跨 BFF/ai-svc/shared 三处、牵动 5 个服务的拦截器链，属 CL-12 一轮独立的活；本阶段强行塞进来会破坏"单 PR 一个 TDD 循环"的约定 |
| 给 `ListActiveByConversation` 加 user_id 并改接口 | 接口变更会波及 handler + 内存替身 + PG 实现 + 4 个测试文件，且与 CL-08c 的图重建/删除闭环同一文件面，合并做更划算（已写进 `CL-F-67` 的修法） |
| 把 `valid_from` 下界从 1970 放宽 | 属产品口径（多早算"合法早年事实"），执行侧不自拟；已记录在 §7 |

### 6.4 复核轮（第二轮，独立子代理，2026-10-22）：**FAIL，抓出 9 项**

> 第二轮复核的子代理（同一只读代理，任务=复核首轮 11 项整改）给出 **FAIL**。
> 它抓到的最重要一条是：**我上一版的「自检表」本身就在造假**（grep 计数写错、commit hash 引用了不在 main 上的分支提交）。

| # | 复核轮抓出的问题 | 严重度 | 处置 |
|---|---|---|---|
| 1 | `report.md` T-20 与 §2.3 写 E2E「30.3s」，但 10-21 证据实为 **43.9s**（30.4s 是 **10-19** 的旧跑），且 §5 自己写 43.9s —— 前后矛盾 | 中 | ✅ 已改（43.9s / 44.3s，两处并附证据文件） |
| 2 | §6.4 自检表 4 个 grep 计数全错（`PASS 17` 实 4 处而非 2；`PASS 15` 实 3 处而非 0；`13 个顶层用例` 实 3 处；`CL-F-66` 实 5 处） | 中 | ✅ 该表已整体重写（本节） |
| 3 | §6.4 #9 引用 `93f48d4b`，但该 commit **不在 main 上**（squash 后是 `a5ffc9a0`） | 中 | ✅ 已改为引用 squash 后的 commit |
| 4 | **grpcserver 失败数仍错**（写 13，实测 **14 个顶层失败**）且**归因错误** | **高** | ✅ 见下条：根因查清后整段撤销 |
| 5 | **grpcserver 根因误判**：不是 `startTestServer` 守卫（`server_test.go:78-91` 真有 TCP 探活），而是本机 `HTTP_PROXY=http://127.0.0.1:61967` 让 gRPC 走代理 502；清空代理后整包 `ok` | **高** | ✅ §7 已重写；并补跑全量：**18/18 包全绿**（`evidence/go-test-all-2026-10-22.txt`） |
| 6 | `roadmap.md` 的「遗留（均另挂账）」只列 3 条，漏 `CL-F-66（高）/67/68/69/70` | 中 | ✅ 已补全 8 条 |
| 7 | T-1 是无保留 PASS，但依据的 db-migrate 日志未留档、也没真跑两遍（账本 `CL-F-70` 自述） | 低 | ✅ 已降级为「PASS（证据强度不足）」 |
| 8 | §2.2 标题写「已修」，与表内「只修一半、`CL-F-66` 高危未闭」不一致 | 低 | ✅ 标题已加限定 |
| 9 | §4 环境快照用现在时，但锁 owner / 容器数已漂移 | 低 | ✅ 已改为复测轮实测快照 |

### 6.5 两条「把环境问题当成代码问题」的更正（本阶段教训）

| 我最初的判断 | 真相 | 代价 |
|---|---|---|
| T-5 用「停 `llm-service`」制造 LLM 失败 —— 跑出 `done\|0` 后以为抽取没失败 | 记忆抽取走**云端 `LLM_BASE_URL`**，与本地 `llm-service` 无关 | 白跑一轮，且差点把「0 条事实」当成 bug |
| `internal/grpcserver` 13~14 个用例 `Unavailable`，判为「既有测试脆弱性 + 本机环境，干净 main 同样红」 | 是本机 **`HTTP_PROXY`** 让 gRPC 走代理返回 502 | 连续两版报告写错归因，还差点给仓库立一个不存在的「已知边界」 |

**结论**：报「环境问题」之前，必须把代理、容器指向、依赖地址三类外部变量逐个排除并留下命令输出；
否则「已知边界」会变成掩盖自身误判的黑洞。
