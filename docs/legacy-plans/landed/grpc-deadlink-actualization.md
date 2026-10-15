---
status: landed
priority: high
owner: lane-cl-c（阶段 CL-02）
created: 2026-10-13
landed: 2026-10-15（阶段 CL-02 · PR #214；逐条终态与证据见 `docs/closure-roadmap/stages/cl-02-grpc-contract/{plan,report}.md`）
last-updated: 2026-10-15（**落地归档**：CL-02 收口，本文件自 `docs/plans/` 迁入 `docs/legacy-plans/landed/`；引用旧路径的文档请改指本文件）
type: implementation-plan
source: 2026-10-13 会话（用户："grpc 这个说很久了，改很多次都没坐实"）
depends-on: []
stage: CL-02（docs/closure-roadmap/stages/cl-02-grpc-contract/plan.md）
related-plans:
  - prompt-pipeline-redesign.md（同批）
---

# Plan — gRPC 死链路「坐实」与清理

> **只描述要做的事，不含代码改动。**
> 现状：43 个 RPC 里有 **7 个是"死"的**（server 实现存在、无生产调用方），当前处置是"**保留 + 标注 + 守卫**"（E2E-F-208 已标"已解决"）。但这种处置在面试/审阅视角**不算"坐实"**——它只钉了"清单真实"，没让任何一条真正可用或真正消失。本计划把每条死链**二选一裁决为"接真"或"清除/归档"**，并升级守卫从"标记计数"到"每条 RPC 有明确状态"。

> **⚠︎ 与 E2E 路线图的关系**：本计划是 **E2E-F-208 的后续**（F-208 完成了"清点 + 保留标注 + 守卫"，但用户认为"没坐实"）。**不新登记 E2E-F**；若需回填账本，仅更新 F-208 的状态说明。

> **执行状态（2026-10-14）**：本计划**已排入阶段 CL-02**（roadmap §2 挂靠），由 **C 轨**（`feat/cl-02-grpc-contract`，工作副本 `../Emotion-Echo-cl02`，draft PR #214）执行。
> **执行计划 = 阶段详档** [docs/closure-roadmap/stages/cl-02-grpc-contract/plan.md](../../closure-roadmap/stages/cl-02-grpc-contract/plan.md)——**它把本计划 §4 的 7 个 Phase 细化为「🟢 不需栈 / 🔴 需栈」并补齐测试点与守卫设计**；本文档继续作为**设计与裁决依据**，两者冲突时以阶段详档为准。

---

## §0 上下文（★ 写文档前的功课，按 AGENTS.md §〇）

### 0.1 死 RPC 全清单（7 处，file:line）

| RPC | proto | server 实现 | 为何死 |
|---|---|---|---|
| `ChatService.StreamMessages` | `chat.proto:65` | `chat_server.go:298`（**恒返 `Unimplemented`** `:302`） | BFF 方法 `chat_grpc.go:174` **不在 `ChatClient` 接口内**（`chat.go:88-105`）→ 零调用；聊天流走 BFF SSE |
| `UserService.Logout` | `user.proto:74` | `user_server.go:220`（只返 `success=true`，无副作用） | BFF 方法 `user_grpc.go:111` **不在 `UserClient` 接口内**；登出只清 cookie |
| `UserService.VerifySecurityAnswer` | `user.proto:79` | `user_server.go:239` | 在接口内（`user.go:91`）但**零 handler 调用**——在用者是兄弟 `VerifySecurityAnswerByUsername`（`auth_handler.go:485`） |
| `EmotionLLMService.AnalyzeBatch` | `emotion_llm.proto:18` | `grpc_server.py:345`（真实现） | 无批量业务；全仓唯一引用是测试 fake（`grpc_analyzer_test.go:67`） |
| `AnalyticsService.MentalHealthHistory` | `metric.proto:59` | `metric_server.go:336`（**恒返空 `Records:nil`** `:341`） | BFF `AnalyticsClient` 无此方法（`analytics.go:105-118`） |
| `AnalyticsService.MentalHealthTrigger` | `metric.proto:62` | `metric_server.go:349`（真实现） | 无消费方（HTTP 路由在 `main.go:335`，也无消费） |
| `AnalyticsService.MentalHealthTrend` | `metric.proto:65` | `metric_server.go:373`（真实现） | 无消费方（HTTP 路由 `main.go:336`） |

分类：**全部为 (c) 类**（proto 预留 / 功能未接出 / 族内冗余）——**没有** (a) 回滚开关式或 (b) 恒真分支式（assessment 的 (b) 类已由 E2E-31 删除）。

### 0.2 现有守卫与账本

| 项 | 事实 |
|---|---|
| 守卫脚本 | `scripts/check_dead_grpc_inventory.sh`：`MARKER="E2E-31 已知未接线"`（`:23`）、`EXPECTED=10`（`:25`）；`:12-18` 自述"**不守零调用，只守清单真实性**" |
| 标记点 10 处 | server 7（metric_server.go:330/346/371、chat_server.go:294、user_server.go:217/235、grpc_server.py:350）+ BFF 3（chat_grpc.go:172、user_grpc.go:109、user_grpc.go:171） |
| CI 接入 | `.github/workflows/e2e-guards.yml:189` |
| 账本 | `docs/e2e-roadmap/discovered-unresolved.md` **E2E-F-208 状态="✅ 已解决（2026-10-09）"**（保留+标注+守卫） |
| 清点主文档 | `docs/e2e-roadmap/findings/2026-10-08-dead-grpc-inventory.md`（§0 结论 15 处；`:19` 明确"**技术债，非文档漂移**"） |

### 0.3 相关 ADR / 决策

| 文档 | 结论 |
|---|---|
| `adr-2026-09-incremental-rpc-adoption.md`（ADR-18） | §A 新增一律 gRPC；§B 已有 HTTP"找时间迁移（非阻塞 backlog）"；§C proto-first；§F HTTP 不主动废弃 → **"proto 预留不算 must-fix"的官方依据** |
| `adr-2026-09-decision-4-closure.md` | 决策 4 收口：宣称 gRPC **覆盖 21/21=100%**（§四）；§八 backlog 列 StreamMessages 低优先 |
| `adr-2026-09-survey-http-bypass.md` | survey 5 端点曾走 HTTP 绕过 gRPC；**§后果已如实记录"gRPC 路径变死代码"**，并含 2026-10-08 更正记录 |
| `adr-2026-09-mental-health-trigger-save.md` | mental_health_assessments 写入链 = trigger runner 末尾 Save（解释了 Trigger 的 server 为何真实） |

### 0.4 关键发现（口径问题，未登记）

- **决策 4 的"100% 覆盖"口径偏乐观**：它按"server 实现 + client 方法存在"计数，把 `Logout`（client 不在接口、零调用）与当时被 HTTP 旁路的 assessment 5 RPC 都算"已接线"。assessment 部分 E2E-F-208 已单列，**但 `Logout` 的"有 client 无 handler 调用"仅在 findings/report 里以"保留+标注"处理——决策 4 收口 ADR 本体未更正**。
- **术语陷阱**：`*_TRANSPORT=http` 的 HTTP fallback（chat/user/analytics/assessment/ai）**不是死代码**，是 (a) 类**回滚开关**，**不在**本计划"死链"范围内。

### 0.5 架构假设清单

| 假设 | 现状核对 |
|---|---|
| "死 RPC" = server 在、无生产调用 | ✅ 7 处均符合 |
| 保留标注 = 已坐实 | ❌ 用户不认——**标注 ≠ 可用 ≠ 消失** |
| 存在范式可循（既有 ADR 已定 proto-first、HTTP 不主动废弃） | ✅ 本计划在既有 ADR 框架内做**逐条裁决**，不推翻 ADR-18 |

---

## §1 问题陈述

一句话：**7 条 gRPC 链"挂着"而非"站着"**——它们既没被接到业务里（不可用），也没被删掉（还占着 proto/接口/守卫的心智负担），唯一处置是注释 + 计数守卫。审阅者会问"这接口谁在用？调完干什么？"，答案往往是"没人调"。

面试视角：宣称"微服务 gRPC 化"却存在 7 条死链，且"改了很多次都没坐实"——**修复方向应是"减少面（删/归档）+ 坐实留下的（接真）"，而不是再加标注**。

---

## §2 目标与非目标

### 目标

- **G1 逐条裁决**：对 7 条死链每条给出并落地 **三选一**：
  - **接真（wire-in）**：接到 BFF → 前端，真正被业务使用；
  - **归档（archive）**：proto 标注 `deprecated/reserved`，文档记录"明确不做"，从"待办"心智里移出；
  - **清除（delete）**：proto + server + client + 守卫标记一并删除。
- **G2 修正口径**：更正决策 4 收口的"100%"表述（或加更正记录），使文档与代码一致。
- **G3 升级守卫**：从"标记数=10"升级为**每条 RPC 有明确状态**（live / archived / deleted）且可机械校验；防止"再挂一条没人发现"。
- **G4 明确 HTTP fallback 边界**：在守卫/文档里声明"`*_TRANSPORT=http` 是回滚开关，不算死链"，避免误伤。

### 非目标

- **不推翻 ADR-18**（proto-first、不主动废弃 HTTP 的既定框架不动）。
- **不重构活链路**（21/43 活 RPC 不动）。
- 不新登记 E2E-F、不挂阶段（仅回填 F-208 说明）。

---

## §3 设计（逐条裁决建议）

| RPC | 业务价值 | 建议裁决 | 理由 |
|---|---|---|---|
| `MentalHealthHistory` | 中（心理评估历史） | **接真** | 若"我的空间"要有"评估历史"，这是现成后端；否则归档 |
| `MentalHealthTrigger` | 中（手动触发评估） | **接真 or 归档** | server 真实现；取决于前端是否需要"立即评估"入口 |
| `MentalHealthTrend` | 中（趋势） | **接真 or 归档** | 同 History；可与报表合并消费 |
| `AnalyzeBatch` | 低（无批量场景） | **清除** | 无批量业务；测试 fake 可删 |
| `StreamMessages` | 低（聊天流走 SSE） | **归档** | ADR 已定"故意不做"（决策 4 §四 #3）；proto 标 reserved + 删 BFF 死方法 |
| `Logout` | 低 | **归档 or 清除** | 现状只清 cookie；要么接真（服务端登出副作用）要么删 proto 该方法 |
| `VerifySecurityAnswer`（族内冗余） | — | **清除** | 与 `...ByUsername` 重复；删冗余 + 保留在用的那个 |

> **裁决原则**：**能接真的（有前端需求）优先接真**（真闭环）；无需求的**清除**（减面）；已被 ADR 明确"不做"的**归档**（proto 标 reserved，避免反复被当待办）。

### 3.1 守卫升级设计

- 新增一份"RPC 状态清单"（脚本内数据或 JSON）：每个 RPC → `live|archived|deleted`。
- 守卫断言：**proto 里出现的 RPC ∈ 清单**；`deleted` 的 RPC **不得出现在 proto/server/client**；`archived` 的必须带 `reserved/deprecated` 注释；`live` 的必须**有生产调用方**（grep 断言非仅测试）。
- 替换现有 `EXPECTED=10` 计数（或叠加）。

---

## §4 分阶段实施（TDD）

| Phase | 内容 | RED 断言 |
|---|---|---|
| 0 | 裁决拍板（§8 D1~D3）：7 条各定 live/archive/delete | — |
| 1 | 守卫升级：RPC 状态清单 + "live 必有调用方"断言（先 RED） | 守卫测试：故意造一条"live 但无调用"→ FAIL |
| 2 | **清除**类：删 proto/server/client/标记 | 编译过 + 守卫绿 + 无残留引用 |
| 3 | **归档**类：proto 标 `reserved/deprecated` + 文档记录 | 守卫断言 archived 必带注释 |
| 4 | **接真**类：BFF client + handler + 前端入口 | 端到端契约测试：前端能取到数据 |
| 5 | 更正决策 4 ADR 口径 | — （文档一致性核对） |
| 6 | 回归钉：`check_dead_grpc_inventory.sh` 新版全绿 + CI | CI job 全绿 |

---

## §5 验收与回归钉

- **机械守卫（核心）**：升级后的守卫脚本对 43 条 RPC 断言状态合法 + `live` 有调用方 + `deleted` 无残留。
- **清除项**：编译 + 全仓 grep 无残留引用。
- **接真项**：端到端有数据（BFF handler 测试 + 前端可调用）。
- **文档一致**：决策 4 ADR 口径与代码一致（含 `Logout`）。
- **收口**：落地后迁 `docs/legacy-plans/landed/`；回填 E2E-F-208 说明。

---

## §6 风险与坑

| 风险 | 说明 | 规避 |
|---|---|---|
| **删除误伤** | proto 删除会波及生成桩、测试 fake、其他 proto import | 先 grep 全仓 + 生成流程核对（memory：改 proto 必须复现原生成方式） |
| **"接真"变半环** | 只接 BFF 没接前端 = 又一个半环 | 接真必须**到前端**，否则选择归档/删除 |
| **守卫过严误报** | HTTP fallback 被误判死链 | G4 显式声明 `*_TRANSPORT=http` 豁免 |
| **ADR 翻案成本** | decision-4 是已收口决策 | 只加**更正记录**（如 survey-bypass ADR 先例），不重写历史 |
| **反复"没坐实"** | 只加注释又会重演 | 本计划的核心动作是**减面 + 接真**，不是加标记 |

---

## §7 与既有计划的关系

- **E2E-F-208 / `findings/2026-10-08-dead-grpc-inventory.md`**：本计划是其**后续收口**（从"保留+标注+守卫"推进到"接真/归档/清除"）。**不重新翻案**，只推进。
- **`user-custom-agent-persona.md` / `prompt-pipeline-redesign.md`（同批）**：无语义依赖；同属"闭环收口"主题。

---

## §8 待拍板

| # | 事项 | 说明与建议 |
|---|---|---|
| D1 | 三条 mental-health RPC：接真还是归档 | 取决于"我的空间"是否要评估历史/趋势入口。**建议先确认前端需求再定** |
| D2 | `StreamMessages` 归档还是清除 | ADR 已"故意不做"→ **建议归档（保留 reserved）** |
| D3 | `Logout`：接真（服务端登出）还是清除 proto | 现状只清 cookie，服务端登出无副作用 → **建议清除 proto 该方法或实现真登出** |
| D4 | 守卫升级是否替换旧 `EXPECTED=10` | **建议叠加**（旧守卫保清单真实，新守卫保状态合法） |

---

## §9 调研依据（本次实读，不凭记忆）

- proto：`proto/{chat,user,agent,metric,emotion_query,emotion_llm,chat_events}.proto`（43 RPC 全量核对）
- server：`chat-svc/internal/grpcserver/chat_server.go`、`user-svc/internal/grpcserver/user_server.go`、`analytics-svc/internal/grpcserver/metric_server.go`、`assessment-svc/internal/grpcserver/agent_server.go`、`ai-svc/internal/grpcserver/server.go`、`emotion-llm-service/grpc_server.py`
- BFF：`internal/downstream/{chat.go,chat_grpc.go,user.go,user_grpc.go,analytics.go,assessment.go,ai.go,emotion_query.go,llm_grpc.go}`
- 守卫：`scripts/check_dead_grpc_inventory.sh`、`.github/workflows/e2e-guards.yml`
- 文档：`docs/e2e-roadmap/findings/2026-10-08-dead-grpc-inventory.md`、`discovered-unresolved.md`（E2E-F-208）、`stages/e2e-31-internal-rpc-convergence/{plan,report}.md`
- ADR：`adr-2026-09-{decision-4-closure,incremental-rpc-adoption,survey-http-bypass,mental-health-trigger-save}.md`

**未能确认**（需决策/实机）：三条 mental-health RPC 的前端业务需求（D1）；删除 `Logout` proto 方法对前端登出流程的影响；RPC 生成流程（改 proto 后如何重生成桩，memory 记录过此坑）。
