---
status: partial
stage: CL-08a
slug: cl-08-memory
created: 2026-10-19
plan: cl-08a-execution-plan.md
---

# CL-08a · 记忆（用户事实 + 「我的记忆」自述入口）— 阶段报告

> **结论：`partial`（接近收口）** —— 两条主链路（正常会话事实 / 自述条 → PG → Redis 摘要 → prompt 注入）
> 已在真栈上端到端跑通；**T-18 / T-19 / T-20 / T-21 与第二方核对均已完成**（核对抓出的 2 个真缺陷已修）。
> 仍未完成的只有：**T-5 / T-17 的运行时 A/B**、以及第二方核对列出的 3 项非阻塞遗留（见 §2.2）。
> 按 RUNBOOK 纪律，执行者不自行宣布 `done`。

---

## §0 摘要

| 项 | 状态 |
|---|---|
| 阶段 PR | [#261](https://github.com/Exist-a/emotion-echo/pull/261)（draft） |
| 登记 PR | [#260](https://github.com/Exist-a/emotion-echo/pull/260)（已合并 `9920eb13`） |
| 范围修正 PR | [#262](https://github.com/Exist-a/emotion-echo/pull/262)（已合并 `6a7ddaf4`） |
| 运行镜像 | `emotion-echo/{ai-svc,web-bff,web}:cl08a`（本轨后缀 tag，未覆盖公共 tag） |
| 迁移 | `i011_create_memory_facts.sql` 已应用（db-migrate `OK`，1000ms） |
| 运行时证据 | [`evidence/runtime-e2e-2026-10-19.txt`](evidence/runtime-e2e-2026-10-19.txt) |

**交付形态**（用户 2026-10-19 两次裁定的合成）：

1. **正常会话**：`message.created` → ai-svc 独立消费组 `ai-svc-memory` → LLM 抽事实 → PG（双时间轴）→ Redis 摘要 → BFF `memory` 段注入。
2. **「我的记忆」页**：侧边栏新 tab → `POST /memory/notes` → 独立 `memory_note`（**不进 conversations/messages**）→ 异步抽取 → 固定回执「记忆已更新」→ 中心辐射关系图（PG 事实渲染）。

---

## §1 测试点判定

> 四值：`PASS` / `FAIL` / `BLOCKED` / `N/A`。判定依据 = 证据文件或命令输出，不是"已实现"。

| # | 测试点 | 判定 | 依据 |
|---|--------|------|------|
| T-1 | 迁移幂等 | **PASS** | db-migrate 日志 `OK emotion-echo-ai-svc/i011_create_memory_facts.sql（1000ms）`；文件全部 `IF NOT EXISTS`，契约测试钉住 |
| T-2 | 事实行写读一致 | **PASS** | 仓储契约测试（内存实现）+ PG 实查（字段齐全；证据 §3 列出居住地 2 行）。⚠️ PG 实现本身**无 integration test**（见 §2.2 建议项） |
| T-3 | 双时间轴 | **PASS** | 运行时：上海行 `status=superseded`、`valid_to=2026-10-18 23:38:34+00`；北京行 active（见证据 §3） |
| T-4 | 抽取幂等 | **PASS** | 单测：同源重复消费 ⇒ LLM 只调 1 次、事实数 1；`(source_kind, source_id, subject, predicate, object)` 唯一索引 |
| T-5 | 抽取失败不阻塞主链路 | **部分 PASS** | 单测证明异步 + 失败可重试；**未做**"停 LLM → 发消息仍成功"的运行时 A/B |
| T-7 | 无记忆时逐字零回归 | **PASS** | `TestPromptAssembly_NoMemory_ByteIdenticalToGolden`（等于 `goldenBase`） |
| T-8 | 段序不变量 | **PASS** | `TestPromptAssembly_MemorySegmentOrder`：base→agent→memory→emotion→guard；两处 `assembleMessages` 都传 memory |
| T-9 | 记忆段内容正确 | **PASS** | 运行时：新会话问"还记得我是做什么的、住在哪" ⇒ 回复「记得的，阿哲。你在北京，做后端工程师。」 |
| T-10 | 关键路径不查图 | **PASS**（架构级） | 注入路径只读 Redis（`downstream.MemorySummarySource`），没有任何图/DB 调用 |
| T-11 | 降级 | **PASS** | 单测：未装配 / 来源报错 / 无身份 / redis.Nil 均返回空串，对话不失败 |
| T-12 | 预算 | **PASS** | 写侧 `BuildSummary` 600 token 预算（超限丢最旧）+ 读侧 `clampMemorySegment` 1200 rune 兜底 |
| T-15 | 事实更新端到端 | **PASS** | 运行时：上海→北京；摘要同步为北京（证据 §3/§4） |
| T-17 | 用户隔离 | **部分 PASS** | 单测：`ListActiveByUser` / 图 / 摘要均按 user_id；**未做**双账号运行时交叉验证 |
| T-18 | 指标被消费 | **PASS** | 4 个 collector（extract/facts_written/summary_build/duration）已在 ai-svc `/metrics` 暴露、被 Prometheus 抓取；business dashboard 新增 5 个面板；`obs_monitoring.test.js` 组 ⑨ 双向钉住。证据 [`evidence/t18-metrics-2026-10-19.txt`](evidence/t18-metrics-2026-10-19.txt) |
| T-19 | 评测集（价值证明） | **PASS**（精确率子项 N/A → c） | `scripts/eval_memory_recall.py` 实跑 **PASS=10 / FAIL=0 / N/A=1**：4/4 跨会话召回、事实更新（图上成都上线且杭州下线 + 回复只用新值）、开/关记忆 A/B 均成立。证据 [`evidence/t19-eval-2026-10-19.txt`](evidence/t19-eval-2026-10-19.txt)。⚠️ **"不相关不注入"记为 N/A**：本阶段设计是**全量摘要常带注入**（≤600 token），问天气时模型会顺带提到已存事实；**按需相关性检索归 CL-08c**（图/向量召回），不以假绿掩盖 |
| T-20 | 跨会话人可见钉 | **PASS** | `e2e/cl-08a-memory.spec.ts` 第 2 例：UI 里在「我的记忆」写「我叫云舟，是一名插画师」→ 新会话问「你还记得我叫什么名字、做什么工作吗」→ 最新回复复述 **云舟 / 插画**（Playwright 2/2 通过） |
| T-21 | 记忆可见性口径 | **PASS** | 用户 2026-10-19 裁定：只放关系图（中心辐射）；页面 + 截图见 [`evidence/t20-memory-page.png`](evidence/t20-memory-page.png)。用户级开关与删除仍归 d |
| T-6 | 实体消解 | **N/A → c** | 需要图/实体层；本阶段不做 |
| T-13 / T-14 | 图后端 / 三层召回 | **N/A → c** | Graphiti+Kuzu 顺延 |
| T-16 | 删除闭环 | **N/A → d** | 用户裁定：删除全归 CL-08d |

---

## §2 已实现清单

**代码（按 TDD 循环提交）**

- `emotion-echo-ai-svc/internal/model/memory_fact.go`：`MemoryFact` / `MemoryNote` / `MemoryExtractRun`
- `.../repository/memory_fact_planner.go`：双时间轴计划（纯函数）+ 会话收尾确定性合并
- `.../repository/memory_fact_repository.go`：接口 + 内存替身 + Postgres 实现（先失效后插入）
- `.../memory/`：LLM 抽取器（OpenAI 兼容）、摘要构建、Redis 摘要写侧
- `.../logic/memory_extract_handler.go`：message / note 两源共用 `extractSource`
- `.../grpcserver/memory_{server,graph}.go`：`MemoryService`（CreateNote / GetGraph）
- `proto/memory.proto` + 生成物 `shared/pkg/emotionmemory`
- `emotion-echo-web-bff/internal/{downstream,handler}/memory_*`：客户端 + 端点
- `emotion-echo-web/app/pages/chat/memory.vue` + `utils/memoryGraph.ts` + `types/memory.ts`
- `emotion-echo-web/app/{layouts/nav.vue, router.options.ts, lib/apiRoutes.ts}`

**测试**

| 套件 | 结果 |
|---|---|
| `go test ./...`（ai-svc） | 全绿（20 包） |
| `go test ./...`（web-bff） | 全绿（含路由契约 50 条） |
| `pnpm test`（web） | **879 passed / 103 files** |
| `pnpm exec nuxt typecheck` | 无 TS 错误 |
| `scripts/check_rpc_status.sh` | PASS（proto 45 条全登记；新增 2 条 live） |
| `scripts/check_proto_layout.py` | PASS |

---

## §2.1 运行态抓出的真缺陷（已修）

| 缺陷 | 现象 | 根因 | 修复 |
|---|---|---|---|
| **GraphChart 未注册** | 生产构建下「我的记忆」图表区**完全空白**：容器在、`_echarts_instance_` 已挂、**canvas 数为 0**、控制台**无任何报错** | `nuxt.config.ts` 的 `nuxt-echarts` 树摇只注册了 `BarChart/LineChart/PieChart/RadarChart`，`graph` 系列没有注册 ⇒ 静默不画 | `charts` 增加 `GraphChart`；新增 vitest 契约钉（config 必须含 `GraphChart`），并重建 `web:cl08a` 后 Playwright 复测通过 |

> 这条只有**真浏览器**能抓到：类型检查、vitest、`pnpm build` 全绿。

## §2.2 第二方核对抓出的问题（已修）

| # | 问题 | 影响 | 修复 |
|---|---|---|---|
| 1 | **MemoryService 信任请求体 `user_id`**（`memory_server.go`） | 本机进程可用 `x-user-id:1` + body `user_id:2` 越权读写他人记忆（8892 发布到宿主、gRPC 无 TLS） | 新增 `memoryCallerID`：身份只取 metadata；无身份 ⇒ `Unauthenticated`；body 与 metadata 不一致 ⇒ `PermissionDenied`；补 2 个负向测试 |
| 2 | **`conversation.closed` protobuf 解码断路**（`consumer/proto_decode.go`） | `Data` 恒 nil ⇒ "会话收尾合并"在生产**永远不执行**；单测因手工构造 Data 而假绿 | 补 `ConversationClosed` 解码；把只断言 `Type` 的旧测试改为断言 `Data` 三个字段 |
| 3 | **双时间轴乱序反转**（`PlanFactUpsert`） | 迟到的旧事实会让较新的 active 下线（旧值复活 + 零长度区间）；旧测试只断言"不出现负区间"，把反转固化成了正确 | 新增 `newerActiveFor`：迟到旧事实落为**已关区间的历史行**，现有 active 不动；测试改为强断言 |
| 4 | **BFF 读摘要无独立超时** | Redis 不可达时每次对话可能先阻塞 ~5s（go-redis DialTimeout）才调 LLM | 读取加 300ms 硬超时，超时按"无记忆"降级 |
| 5 | 计划文档与代码不一致（health/ready、开关名、ListNotes、SQL 伪代码） | 误导后续实现 | 已逐条更正（见 `cl-08a-execution-plan.md`） |

**仍未处理（已记录）**：Postgres 仓储无 integration test（并发冲突时 PG 报错 vs 内存静默跳过的语义差异）；`FinishExtractRun` 失败会让运行记录停在 pending；E2E 用固定 20s 等待（canvas 无法用 DOM 断言）。这三项归 **CL-08c/d 或 CL-09**。

## §3 未完成 / 后移（如实记录）

| # | 项 | 归属 | 说明 |
|---|---|---|---|
| 1 | ~~T-18 指标 + 面板~~ | ✅ **已完成（2026-10-19）** | 4 个 collector + 5 个面板 + 契约组 ⑨；Prometheus 实测 p95≈0.975s、rate>0 |
| 2 | ~~T-19 评测集~~ | ✅ **已完成（2026-10-19）** | `scripts/eval_memory_recall.py` 可复跑；PASS=10 / FAIL=0 / N/A=1；"不相关不注入"按设计差异记为 N/A 并移交 c |
| 3 | ~~T-20 Playwright 钉~~ | ✅ **已完成（2026-10-19）** | 2/2 通过；跨会话 UI 复述 + 关系图渲染截图 |
| 4 | ~~浏览器视觉验收~~ | ✅ **已完成（2026-10-19）** | 生产构建 IAB/Playwright 截图；**并抓出真缺陷**（见下） |
| 5 | ~~第二方核对~~ | ✅ **已完成（2026-10-19）** | 独立代理只读审查 7 项；抓出 **2 个真缺陷 + 1 个语义缺陷**，均已修（见 §2.2） |
| 6 | **T-5 / T-17 运行时 A/B** | 未做 | 单测已覆盖；"停 LLM 发消息仍成功"与"双账号交叉"未在真栈跑 |
| 6 | **用户级「关闭记忆」开关** | CL-08d | 当前只有运维级 `MEMORY_ENABLED` / `MEMORY_INJECTION_ENABLED` |
| 7 | **删除闭环** | CL-08d | 用户裁定 |
| 8 | **图后端 / 三层召回 / 实体消解** | CL-08c | 当前图由 PG 事实渲染，切数据源即可 |

**已知边界（不夸大）**

- `conversation.closed` 的合并是**确定性去重**（同键保留最新），**不是语义合并**（那需要二次 LLM，成本更高）。
- 自述条的异步抽取是**进程内 goroutine**：进程在抽取完成前退出会留下 `pending` 条（状态可观测，但**没有启动补抽 sweeper**）。
- 摘要 token 是**粗估**（中文 1 rune ≈ 1 token），不是精确 tokenizer。

---

## §4 环境与镜像（R4 交代）

- 栈：19 容器 healthy；`ai-svc` / `web-bff` / `web` 跑 **`:cl08a`**，其余为公共 tag。
- `deploy/.devmode-session`：owner `lane-cl-08`，until 2026-10-19 23:59（接替 CL-06 的过期锁）。
- ⚠️ **环境漂移**：`user-svc` 公共 tag `v0.1.5` **不含 CL-06 的 `ListAgentProfiles`** ⇒ BFF 日志报
  `Unimplemented: unknown method ListAgentProfiles`，角色人设段被跳过（**不影响记忆链路**）。
  已登账 `CL-F-59`（公共镜像 tag 未随 CL-06 合并更新）。
- 首次 `CreateConversation` 仍会撞 5s 截止（重试即成功，E2E-F-115 既有账，非本阶段引入）。
