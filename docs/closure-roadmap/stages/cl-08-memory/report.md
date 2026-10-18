---
status: partial
stage: CL-08a
slug: cl-08-memory
created: 2026-10-19
plan: cl-08a-execution-plan.md
---

# CL-08a · 记忆（用户事实 + 「我的记忆」自述入口）— 阶段报告

> **结论：`partial`** —— 两条主链路（正常会话事实 / 自述条 → PG → Redis 摘要 → prompt 注入）
> 已在真栈上端到端跑通；但 **T-18（指标与面板）未落地**、**T-19（评测集）未做**、
> **浏览器视觉验收未做**、**第二方核对未做**。按 RUNBOOK 纪律，执行者不自行宣布 `done`。

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
| T-2 | 事实行写读一致 | **PASS** | 仓储契约测试（内存实现在 `repository` 包）+ PG 实查（4 行，字段齐全） |
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
| T-18 | 指标被消费 | **BLOCKED** | **未实现**：抽取/摘要无指标，也无面板。见 §3 未完成项 |
| T-19 | 评测集（价值证明） | **BLOCKED** | **未做**：只有 2 条手工样例，不构成召回率/精确率数字 |
| T-20 | 跨会话人可见钉 | **部分 PASS** | API 级已实拍（SSE 回复复述事实）；**未做** Playwright 钉子 |
| T-21 | 记忆可见性口径 | **部分 PASS** | 用户已裁：只放关系图（中心辐射）；页面/路由已落地；用户级开关归 d |
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

## §3 未完成 / 后移（如实记录）

| # | 项 | 归属 | 说明 |
|---|---|---|---|
| 1 | **T-18 指标 + 面板** | 本阶段未做 | 抽取次数/失败数/token、摘要构建结果均无指标；Grafana 无对应面板。**这违反 CL-01 的「埋了必须有人看」纪律** ⇒ 落地时指标与面板必须同批 |
| 2 | **T-19 评测集** | 本阶段未做 | 需要一组跨会话题目（召回/精确/开-关 A/B）。当前只有 2 条手工样例 |
| 3 | **T-20 Playwright 钉** | 本阶段未做 | API 级已实拍；浏览器回归钉待补 |
| 4 | **浏览器视觉验收** | 本阶段未做 | `/chat/memory` 未登录返回 302→/login（路由与鉴权正常），但**未做登录态截图**；关系图渲染未人眼确认 |
| 5 | **第二方核对** | 未做 | 按 RUNBOOK 收口契约需独立核对 |
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
