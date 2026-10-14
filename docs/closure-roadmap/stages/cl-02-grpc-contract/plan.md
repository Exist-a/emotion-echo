---
stage: cl-02
title: gRPC 契约闭环（死链逐条裁决 + 守卫升级 + "严格走 gRPC"坐实）
type: transformation
status: in-progress
created: 2026-10-14
last-updated: 2026-10-14（建档 C 轨并行开工 → **Phase 1 守卫升级已落地**（清单 + 5 断言 + 负向对照，🟢 不需栈）；**未起 dev 栈**，运行时证据待 Phase 2/4/6，见 §A.4）
depends-on: []
blocks: []
gate: []            # 无开工前阻塞决策门；7 条死链裁决 + 1 条模型服务边界共 8 个 [M] 见 §7
source:
  - docs/plans/grpc-deadlink-actualization.md（主，本阶段完整承载）
related-plans:
  - docs/plans/observability-instrumentation-and-dashboards.md（CL-01，共享 `downstream/**` 目录但不同文件）
  - docs/plans/prompt-pipeline-redesign.md（CL-04，共享 `downstream/llm*.go` 同目录不同文件）
---

# CL-02 · gRPC 契约闭环 — 执行计划

> **核心口径（roadmap §4 CL-02）**：后端**各服务间调用严格走 gRPC**；**HTTP 仅作显式降级**
> （`*_TRANSPORT=http` 回滚开关）——降级**可接受**，不算违规。用户原话："gRPC 搞了很久，本阶段要把它**坐实**。"
>
> **现状一句话**：**主干已 gRPC 化（默认值即 grpc），但有 7 条 RPC"挂着没站着"** ——
> server 实现存在、全仓无生产调用方，唯一处置是"注释 + 计数守卫"（E2E-31 T-1，用户 2026-10-09 裁定"保留 + 标注"）。
> **标注 ≠ 可用 ≠ 消失**：审阅者问"这接口谁在用"，答案仍是"没人调"。
> 本阶段把每条**二选一裁决为"接真"或"归档/清除"**，并把守卫从"标记数=10"升级为"**每条 RPC 有明确状态 + live 必有调用方**"。
>
> **执行协议**：[RUNBOOK.md](../../RUNBOOK.md)（六步循环 / 证据有效性 / 收口契约）
> + [parallel-contract.md](../../parallel-contract.md)（**C 轨**占位与文件面隔离）
> + [anti-patterns.md](../../../e2e-roadmap/anti-patterns.md)（14 类失真反例）。
>
> **并行**：本阶段为 **C 轨**，与 **A 轨（CL-04）**、**B 轨（CL-01）** 并行。
> 文件面：与 A/B 在 `emotion-echo-web-bff/internal/downstream/` **同目录但不同文件**（契约 §1.2 允许并行的情形）；
> 唯一**独占**的脊柱是 `proto/**`（当前无其他轨占用）。**dev 栈单锁**：A 轨持 `deploy/.devmode-session` 至 2026-10-14 12:00，
> B 轨排队在后 ⇒ 本阶段"需栈"的 Phase 排在更后（§3 Phase 2/4/6 的运行时部分）。

---

## §A 上下文（★ 写前功课，按 AGENTS.md §〇）

### A.1 已读代码（真实文件，本次实读亲核）

| 文件 | 关键事实（亲核结论） |
|---|---|
| `proto/chat.proto:65` | `rpc StreamMessages (…) returns (stream ChatEvent)` —— 注释 `:58` 自述"替代 SSE 聊天流"，实际从未启用 |
| `proto/user.proto:74 / :79 / :91` | `Logout` / `VerifySecurityAnswer` / `VerifySecurityAnswerByUsername` 三者并存（**族内冗余**已确认） |
| `proto/metric.proto:56-65` | 心理健康四兄弟：`Assessment`（在用）/ `History` / `Trigger` / `Trend` |
| `proto/emotion_llm.proto:18` | `rpc AnalyzeBatch (…) returns (stream AnalyzeResponse)`，注释标"Stage 16" |
| `proto/*.proto` 全量 | **43 条 RPC**（chat 8 / user 10 / metric 9 / agent 5 / emotion_query 7 / emotion_llm 4） |
| `emotion-echo-web-bff/internal/downstream/chat.go:88-105` | `ChatClient` 接口 **7 个方法，无 `StreamMessages`** ⇒ `chat_grpc.go:172` 的死方法不在接口内，零调用 |
| `emotion-echo-web-bff/internal/downstream/chat_grpc.go:172-176` | `StreamMessages` 方法体存在且带 `E2E-31 已知未接线` 标记 |
| `emotion-echo-chat-svc/internal/grpcserver/chat_server.go:298-302` | **恒返 `codes.Unimplemented`**（明确"无流式订阅业务"）—— 热链是 BFF SSE |
| `emotion-echo-web-bff/internal/downstream/analytics.go:105-118` | `AnalyticsClient` 接口 **6 个方法，无任何 `MentalHealth*`** |
| `emotion-echo-analytics-svc/internal/grpcserver/metric_server.go:336-341` | `MentalHealthHistory` **恒返 `Records: nil`**（占位实现） |
| `emotion-echo-analytics-svc/internal/grpcserver/metric_server.go:349-368` | `MentalHealthTrigger` **真实现**（`TriggerAssessment` + job store，E2E-15 已验） |
| `emotion-echo-analytics-svc/internal/grpcserver/metric_server.go:373-` | `MentalHealthTrend` **真实现**（`GetTrend`，`ErrNotFound` 归零） |
| `emotion-echo-web-bff/internal/downstream/user_grpc.go:109 / :171` | 两处 `E2E-31` 标记：`Logout`（不在 `UserClient` 接口）+ `VerifySecurityAnswer`（在接口内但零 handler 调用） |
| `scripts/check_dead_grpc_inventory.sh` | `MARKER="E2E-31 已知未接线"`（`:23`）、`EXPECTED=10`（`:25`）；`:12-18` 自述"**不守零调用，只守清单真实性**"⇒ 正是本阶段要升级的对象 |
| `.github/workflows/e2e-guards.yml:183-184` | 守卫 23/23 已接 CI（`bash scripts/check_dead_grpc_inventory.sh`） |
| `emotion-echo-web-bff/internal/config/config.go:360-373` | 5 个 `*_TRANSPORT` 开关（USER/CHAT/ASSESSMENT/ANALYTICS/AI），默认值 = **grpc** |
| `emotion-echo-web-bff/main.go:504` + `handler/survey_handler.go:16-20` | assessment 的 `assessmentBase != ""` 恒真旁路 **E2E-31 已删**，统一走 `AssessmentClient`（gRPC） |

**亲核的标记点计数**：`grep -rn "E2E-31 已知未接线"` 命中 **10 处**，与守卫脚本 `EXPECTED=10` 一致
（server 7：`metric_server.go`×3 / `chat_server.go`×1 / `user_server.go`×2 / `grpc_server.py`×1；BFF 3：`chat_grpc.go`×1 / `user_grpc.go`×2）。

**Phase 1 补充亲核（全量 43 条，证据见 [`evidence/phase1-scan.md`](evidence/phase1-scan.md)）**：
- **43 条 RPC 全部定位到调用点** ⇒ 结论：**7 条死链是完整清单**（未发现第 8 条）；一度怀疑的 `GetEmotionByMessage/…Conversation/…Fused` **经查证为活链**（BFF `emotion_query_handler.go:46/66/102`）。
- 🔴 **RPC 名 ≠ 客户端方法名（14 条）**：如 `ReportsTrend→TrendReport`、`ListMyResults→ListResults`、`UpdateProfile→UpdateMe`、`GetUserById→GetByID`。⇒ **"live 有调用方"不能靠 RPC 名 grep**（会把 6 条 metric RPC 误判成死链），证据必须登记 `(文件, 符号)`。
- **同型陷阱**：`SendMessage` 撞 Kafka producer（`ai-svc/consumer/dlq.go:169`）、`Register` 撞 Nacos（`ai-svc/nacos_boot.go:86`）与 BFF 路由注册（`main.go:501`）。

**亲核的"是否还有非开关式 HTTP 直连"**：`grep -rn "http://emotion-\|http://localhost:8"` 在非测试 Go 代码中
**零命中** ⇒ 服务间"严格走 gRPC"的**主干已成立**，本阶段的活是"**减面 + 坐实留下的**"，不是"再迁一批"。

### A.2 相关 ADR / 决策（已查）

| 文档 | 结论（对本阶段的约束） |
|---|---|
| `adr-2026-09-incremental-rpc-adoption.md`（ADR-18） | §A 新增一律 gRPC；§B 已有 HTTP"**非阻塞** backlog"；§C proto-first；§F HTTP 不主动废弃 → **"proto 预留不算 must-fix"的官方依据**，本阶段**不推翻** |
| `adr-2026-09-decision-4-closure.md` | 决策 4 宣称 gRPC **覆盖 21/21=100%**（§四）；§八 backlog 列 `StreamMessages` 为低优先 → **本阶段须加更正记录**（该口径把"client 方法存在"当"已接线"，漏了 `Logout` 这类"有 client 无 handler 调用"） |
| `adr-2026-09-survey-http-bypass.md` | survey 5 端点曾 HTTP 绕过 gRPC，**已修**（main.go:504 统一走 gRPC）；该 ADR 含"更正记录"先例 → 本阶段更正决策 4 时**沿用此先例形式** |
| `adr-2026-09-mental-health-trigger-save.md` | `MentalHealthTrigger` 的 server 为何"真实现"（写入链 = trigger runner 末尾 Save，E2E-15 已验） |
| `docs/closure-roadmap/decisions.md` | **无 CL-D-xx** 与本阶段冲突；本阶段新决策沿用 `CL-D-xx` 续号 |
| `docs/_meta/parallel-tracks.md` §二 资源 2 | **main PR 合并窗口串行** + 每次合并后跑 `e2e_stage_audit.py --all` 确认 0 FAIL —— `proto/**` 变更影响面大于普通文件，本阶段合并更须遵守 |

### A.3 架构假设清单（本文假设 vs 现状）

| 假设 | 现状核对 |
|---|---|
| "死 RPC" = server 在、无生产调用方 | ✅ 亲核 7 处均符合 |
| "保留 + 标注 + 计数守卫" 已算坐实 | ❌ 用户不认（"改很多次都没坐实"）——**标注 ≠ 可用 ≠ 消失** |
| `*_TRANSPORT=http` 是死代码 | ❌ **是 (a) 类回滚开关，不是死链**；守卫升级必须显式豁免，否则误伤（G4） |
| 服务间仍存在 HTTP 直连待迁 | ❌ 亲核**零命中**；assessment 旁路已删 ⇒ 假设**不成立**，本阶段不重做这项 |
| 删除 proto 方法 = 只删一行 | ❌ 波及生成桩 / 测试 fake / 其他 proto import / 生成工具链（memory：改 proto 必须复现原生成方式 `protoc 32.1` + `protoc-gen-go v1.36.11` + `protoc-gen-go-grpc v1.6.2`，**以文件名调用**） |
| 三条 mental-health RPC 有前端需求 | ❓ **未知**（`apiRoutes.ts` 只留注释、BFF 未暴露三条端点）⇒ 属 §7 D1/D5，**须用户裁定** |

### A.4 未跑 smoke / 未取运行时证据声明（诚实性，按 RUNBOOK §4）

本详档为**建档期产物**，**未起 dev 栈**（A 轨持锁至 12:00，且本阶段前段为 🟢 不需栈）。
因此：**"清除后全栈起得来""接真后端到端有数据""CI 通知"三类结论目前无运行时证据**，
均为待取项（§3 Phase 2/4/6 的 🔴 部分）。§3 每个 Phase 均标 🟢/🔴，**🔴 未取证前不得判 PASS**。

---

## §1 目标与非目标

### 目标（对齐 roadmap §4 与来源计划 §2）

- **G1 逐条裁决**：7 条死链每条落地 **三选一** —— **接真（wire-in）** / **归档（archive，proto 标 `reserved/deprecated`）** / **清除（delete，proto+server+client+标记一并删）**。
- **G2 修正口径**：更正决策 4 收口 ADR 的"100% 覆盖"表述（**以"更正记录"形式，不重写历史**），使文档与代码一致。
- **G3 升级守卫**：从"标记数 = 10"升级为 **"43 条 RPC 每条有明确状态（live/archived/deleted）+ `live` 必须有调用方证据"**，可机械校验。
- **G4 声明 HTTP fallback 边界**：守卫/文档显式声明 "`*_TRANSPORT=http` 是回滚开关，**不算死链**"，避免误伤（防"守卫过严"反模式）。

### 非目标

- **不推翻 ADR-18**（proto-first、不主动废弃 HTTP 的既定框架不动）。
- **不重构活链路**（43 - 7 = 36 条活 RPC 不动）。
- **不新增 HTTP→gRPC 迁移**（亲核已零残留，见 §A.1 末）。
- **不做 CL-03 的异步 trace 连续性与 DLQ**（归 CL-03；B 轨 CL-01 已含 DLQ span 埋点）。
- **不新登记 E2E-F**（来源计划 §开头明确：仅回填 E2E-F-208 的状态说明）。

---

## §2 测试点清单（每点标 `[A]`/`[V]`/`[M]`）

> 判定与证据口径见 [RUNBOOK §4](../../RUNBOOK.md)。`[M]` = 需用户裁定，**不阻塞"代码 + 守卫"部分**。

### A 组 · 死链逐条裁决落地（G1）

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| T-01 | `StreamMessages` 裁决落地（建议**归档**：ADR 已"故意不做"） | [M]→[A] | proto 加 `reserved`/注释 + 删 BFF 死方法 `chat_grpc.go:172` + 守卫状态=archived | commit + 守卫输出 |
| T-02 | `Logout` 裁决落地（建议**清除 proto 该方法**或实现真登出） | [M]→[A] | 按裁定：删 proto+server+client 三处，或接真后 grep 到调用方 | commit + 守卫输出 |
| T-03 | `VerifySecurityAnswer`（族内冗余）**清除** | [A] | 删 proto/server/client + 保留在用的 `…ByUsername`（`auth_handler.go:485`） | 编译过 + 全仓 grep 无残留 |
| T-04 | `AnalyzeBatch` **清除**（无批量业务） | [A] | 删 proto/server + 测试 fake（`grpc_analyzer_test.go:67`） | 编译过 + grep 无残留 |
| T-05 | `MentalHealthHistory` 裁决落地（接真 or 归档） | [M]→[A] | 按裁定 | commit + 证据 |
| T-06 | `MentalHealthTrigger` 裁决落地（接真 or 归档；server 真实现现成） | [M]→[A] | 按裁定 | commit + 证据 |
| T-07 | `MentalHealthTrend` 裁决落地（同上） | [M]→[A] | 按裁定 | commit + 证据 |
| T-08 | 清除类**无残留引用**（proto/server/client/测试 fake/文档） | [A] | 全仓 grep（含 `--include=*.py/*.ts`） | grep 输出为空 |

### B 组 · 守卫升级（G3/G4）

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| T-09 | 状态清单**覆盖全部 43 条 RPC**（不重不漏） | [A] | 守卫断言：proto 实际 `rpc` 集合 ⊆/= 清单键集合 | 守卫 PASS 输出 |
| T-10 | `deleted` 的 RPC **不得出现在 proto/server/client** | [A] | 守卫对每条 deleted 做三处 grep 断言 | 守卫 PASS 输出 |
| T-11 | `archived` 的 RPC **必带** `reserved`/`deprecated` 标注 | [A] | 守卫断言 proto 对应行邻域含标注 | 守卫 PASS 输出 |
| T-12 | `live` 的 RPC **必须有调用方证据**（清单内登记 file:line，守卫校验该处确含调用） | [A] | 守卫逐条读"调用方证据"列并断言文件存在 + 含客户端方法名 | 守卫 PASS 输出 |
| T-13 | **负向对照**：故意造一条"标 live 但无调用方" → 守卫必须 **FAIL** | [A] | 临时改清单跑守卫，须 RED，再还原 | RED 输出 + 还原后 GREEN 输出 |
| T-14 | HTTP fallback **豁免**：`*_TRANSPORT=http` 不被判死链 | [A] | 守卫内显式豁免清单（5 个开关）+ 负向对照（若误判则 FAIL） | 守卫输出 |

### C 组 · "严格走 gRPC"成色（roadmap 核心口径）

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| T-15 | 5 个 `*_TRANSPORT` 默认值 = **grpc** | [A] | 静态断言 `config.go:360-373` 默认分支 + 运行时 `docker inspect` env（🔴 需栈） | 代码 + env 输出 |
| T-16 | 非开关式 HTTP 直连**保持零残留**（模型服务豁免） | [A] | grep 断言（初查零命中，本点=**守成**）+ 豁免清单声明 | grep 输出 |
| T-17 | ai-svc → FER/SenseVoice/XTTS 是否算"服务间调用" | [M] | 用户裁定（roadmap §4 倾向：**不算**，模型服务走 HTTP 属设计） | 裁定记录 |

### D 组 · 口径与文档一致（G2）

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| T-18 | 决策 4 ADR 的"100% 覆盖"已加**更正记录**（含 `Logout`） | [A] | 读 ADR diff，须含更正段 + 日期 + 触发原因 | commit diff |
| T-19 | E2E-F-208 说明回填（"保留+标注"→"已逐条裁决"） | [A] | `discovered-unresolved.md` diff（**保留原行**，只改状态说明） | diff |
| T-20 | 来源计划迁 `docs/legacy-plans/landed/` 且入站链接同步修 | [A] | 迁移后 grep 断链检查（RUNBOOK 收口契约 5） | grep 输出 |

### E 组 · 端到端与 CI（🔴 需栈 / 需 CI 运行）

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| T-21 | 清除后**全栈起得来**且服务注册齐全（Nacos `count:6`） | [A] | `docker compose … --profile dev up -d` + Nacos 服务列表 | 命令 + 输出 |
| T-22 | 接真类 RPC（若 D1 裁"接真"）**端到端有数据** | [A]+[V] | BFF handler 测试 + 前端真实可调用（curl/截图） | 输出 + 截图 |
| T-23 | 相关功能**无回退**（登出/找回密码/聊天/情绪分析 smoke） | [A] | §2.4 数据契约 smoke（走网关 :19080 + Bearer） | smoke 退出码 |
| T-24 | 新版守卫接入 CI 且**真能红** | [A] | 本地 RED 复现 + CI job 输出 | 两次输出对照 |

---

## §3 分阶段实施（TDD：先 RED 回归钉，再 GREEN）

> **栈依赖标注**：🟢 = 不需栈（worktree 内可做） / 🔴 = 需 dev 栈（受 `.devmode-session` 约束，排在 A/B 之后）。

| Phase | 内容 | 栈 | RED 断言 |
|---|---|---|---|
| **0** | 7 条裁决拍板（§7 D1~D6）+ 模型服务边界（D7） | 🟢 | —（[M]，用户裁定后回填本表） |
| **1** | **守卫升级（核心）✅ 已落地**：`scripts/rpc_status.tsv`（43 行 × 7 列：`rpc / proto:line / status / evidence_path / evidence_symbol / marker_sites / note`）+ `scripts/check_rpc_status.sh` 五条断言（A1 覆盖 / A2 live 必有证据 / A3 标记数与清单一致 / A4 终态规则 / A5 HTTP 豁免）；**标记预期数改为由清单推导**（取代硬编码 `EXPECTED=10`） | 🟢 | **已证**：A1~A5 现行全绿；负向对照 2 组（改错证据符号 → A2 FAIL；给 live 行挂标记 → A3 双 FAIL）见 [`evidence/phase1-scan.md`](evidence/phase1-scan.md) §4。**A4 当前为空断言**（无 archived/deleted 行），Phase 2/3 后才有实义 |
| **2** | **清除类**：删 proto/server/client/测试 fake/标记（T-03/T-04 + 裁定为 delete 的项） | 🟢改码 / 🔴验证 | RED：残留 grep 命中 ⇒ 删净后 GREEN；`protoc` 重生成后编译过 |
| **3** | **归档类**：proto 标 `reserved`/`deprecated` + 注释记录"明确不做"（T-01/T-02 若裁归档） | 🟢 | RED：守卫断 archived 必带标注 ⇒ 未标时 FAIL → 标注后 GREEN |
| **4** | **接真类**：BFF client + handler + **前端入口**（T-05~T-07 若裁接真） | 🔴 | RED：前端拿不到数据的契约测试 ⇒ 接完 GREEN（**必须到前端，否则改判归档/清除**） |
| **5** | **口径更正 + 账本回填**（T-18/T-19） | 🟢 | —（文档一致性核对；决策 4 ADR 用"更正记录"先例形式） |
| **6** | **收口**：迁移来源计划（T-20）+ 全栈冒烟（T-21/T-23）+ 负向对照（T-13 ✅ 已在 Phase 1 证过）+ CI 接线（T-24） | 🔴 | CI job 首次跑须能红（Phase 1 已在本地证过 RED 能力，此处验"接进 CI 后仍生效"） |

> **Phase 1 落地方式（修正建档时的预想）**：建档时设想"守卫对现状必红"，实测**不成立**——
> 因为清单是按现状机械生成后逐条亲核的（43 条全部定位到调用点），A1~A5 是对**现状的完整描述**，
> 故现行即绿。"钉子能红"的证据改用**负向对照**给出（改错证据符号 → A2 FAIL；给 live 行挂标记 → A3 双 FAIL，
> 见 [evidence/phase1-scan.md](evidence/phase1-scan.md) §4）——**这比"必红"更强**：它证明断言不是永真式。
> **红线不变**：Phase 2/3/4 不得先删代码再补守卫（TDD 倒置反模式）；改一条 RPC 的终态**必须同步改清单**，
> 否则 A1/A3/A4 立刻红（这正是"接线/清除必须是一次显式决策"的机械保证）。

---

## §4 收口判据（"做到什么程度"，对齐 roadmap §4 DoD）

**功能判据**
- 7 条死链**每条都有终态**：要么真被业务调用（接真到前端），要么在 proto 里明确 `reserved`（归档），要么全仓消失（清除）。**不存在"仍在 proto 里挂着但没人用"的第八条**。
- 守卫能回答"任意一条 RPC 现在是什么状态、谁在调它"。

**机器判据**
- `scripts/check_rpc_status.sh` 覆盖 43/43 条 RPC；4 条断言全绿；**负向对照（T-13）已证能红**。
- 全仓 grep：清除类零残留；archived 类必带标注。
- 决策 4 ADR 有更正记录；E2E-F-208 状态说明回填。
- 全栈起得来（Nacos `count:6`）+ §2.4 数据契约 smoke 退出码 0。

---

## §5 回归钉

- **主钉（bash 契约测试）**：`scripts/check_rpc_status.sh`（**已落地**）+ `scripts/rpc_status.tsv`
  —— 覆盖 5 组断言：A1 覆盖 43/43 / A2 live 有调用方证据 / A3 标记数与清单一致（推导式）/ A4 终态规则 / A5 HTTP 豁免；
  **负向对照已证能红 2 组**（见 [`evidence/phase1-scan.md`](evidence/phase1-scan.md) §4）。
  （沿用 `e2e-guards.yml` 现有"脚本 + 守卫 job"形态，不引新框架。旧 `check_dead_grpc_inventory.sh` **保留叠加**，见 §7 D6。）
- **辅助钉**：清除项的**编译回归**（`go build ./...` + `python -m compileall`）+ `protoc` 重生成幂等性核对。
- **CI**：接 `e2e-guards.yml` 作为**新守卫 job**（保留 23/23 旧守卫做"清单真实性"叠加，见 §7 D6）。

---

## §6 风险与坑

| 风险 | 说明 | 规避 |
|---|---|---|
| **删除误伤** | proto 删除波及生成桩 / 测试 fake / 其他 proto import / 生成工具链 | 先全仓 grep（含 `.py`/`.ts`）+ **复现原生成方式**（`protoc 32.1` + 两个插件，**以文件名调用**）；改完必重建镜像再验收（RUNBOOK §2） |
| **"接真"变半环** | 只接 BFF 没接前端 = 又一个半环（本阶段要消灭的正是这个） | 接真判据**必须到前端可见**；否则退回归档/清除 |
| **守卫过严误报** | `*_TRANSPORT=http` 回滚开关被误判死链 | Phase 1 就内置豁免清单（T-14）+ 负向对照 |
| **守卫"只报不拦"** | 脚本写了但没接 CI / 接了不真跑 | T-24 要求"接进 CI 后仍能红"，不接受"已创建脚本"当证据（RUNBOOK §4） |
| **worktree 缺 `.env.local`** | 本 worktree 无 gitignored 的 `deploy/.env.local` ⇒ 起栈会中断；且 worktree 源码**不能直接 build 成镜像** | 已知（CL-F-07，A 轨登账）；本阶段 🔴 Phase 走 A 轨验证过的绕过路径「worktree build + 共享目录 up」，或与 A/B 协商窗口 |
| **worktree 下守卫假红** | `check_git_layout.py:55` 用 `is_dir()` 判 `.git` ⇒ worktree 恒红（CL-F-06） | 已知，不在本阶段修（范围外只记账） |
| **镜像新鲜度守卫假红** | `check_image_freshness.sh` 纯时间比较（CL-F-08） | 🔴 Phase 判"改完是否生效"须**看行为证据**，不只看时间戳 |
| **dev 栈排队** | A 轨持锁至 12:00，B 轨排队在后 ⇒ 本阶段 🔴 Phase 排更后 | 🟢 Phase（1/3/5/6 的文档与守卫）先推；🔴 部分在占位行登记排队 |
| **ADR 翻案成本** | 决策 4 已收口 | 只加**更正记录**（沿用 survey-bypass ADR 先例），不重写历史 |

---

## §7 [M] 裁定记录（**待用户拍板**，均不阻塞 🟢 部分）

| # | 事项 | 建议 | 状态 |
|---|------|------|------|
| D1 | 三条 mental-health RPC（History/Trigger/Trend）：接真 or 归档 | **先确认前端是否有"评估历史/趋势/立即评估"需求**；有则接真（后端现成），无则归档 | ⏳ 待裁 |
| D2 | `StreamMessages`：归档 or 清除 | **归档**（ADR 已定"故意不做"，proto 标 `reserved` + 删 BFF 死方法） | ⏳ 待裁 |
| D3 | `Logout`：接真（服务端登出副作用）or 清除 proto 方法 | 现状只清 cookie、server 无副作用 ⇒ 建议**清除 proto 该方法**（或实现真登出） | ⏳ 待裁 |
| D4 | `VerifySecurityAnswer` 冗余 & `AnalyzeBatch` 清除 | **清除**（前者与 `…ByUsername` 重复，后者无业务） | ⏳ 待裁 |
| D5 | 三条 mental-health 的 HTTP 路由（analytics-svc `main.go:335-336`）是否一并处置 | 与 D1 同进同出：归档则连 HTTP 路由一并标"未接出" | ⏳ 待裁 |
| D6 | 新守卫是**替换**还是**叠加**旧的 `EXPECTED=10` | **叠加**（旧守卫保"标记清单真实"，新守卫保"状态合法"） | ⏳ 待裁 |
| D7 | ai-svc → FER/SenseVoice/XTTS 是否算"服务间调用"（roadmap 待确认边界） | **不算**（模型服务走 HTTP 属设计；roadmap 倾向） | ⏳ 待裁 |

---

## §8 调研依据（本次实读，不凭记忆）

- **proto**：`proto/{chat,user,metric,emotion_llm}.proto`（RPC 逐条核对；全量 43 条经 `grep -c "^\s*rpc "` 计数）
- **server**：`emotion-echo-chat-svc/internal/grpcserver/chat_server.go`、`emotion-echo-analytics-svc/internal/grpcserver/metric_server.go`、`emotion-echo-user-svc/internal/grpcserver/user_server.go`、`emotion-echo-ai-svc/internal/grpcserver/server.go`、`emotion-echo-assessment-svc/internal/grpcserver/agent_server.go`、`emotion-llm-service/grpc_server.py`
- **BFF**：`internal/downstream/{chat.go,chat_grpc.go,user.go,user_grpc.go,analytics.go,assessment.go,ai.go,llm_grpc.go}`、`internal/config/config.go:360-373`、`main.go:504`、`internal/handler/survey_handler.go`
- **守卫与 CI**：`scripts/check_dead_grpc_inventory.sh`、`.github/workflows/e2e-guards.yml:183-184`（守卫 23/23）
- **ADR**：`adr-2026-09-{incremental-rpc-adoption,decision-4-closure,survey-http-bypass,mental-health-trigger-save}.md`
- **账本与清点**：`docs/e2e-roadmap/discovered-unresolved.md`（E2E-F-208）、`docs/e2e-roadmap/findings/2026-10-08-dead-grpc-inventory.md`
- **来源计划**：`docs/plans/grpc-deadlink-actualization.md`（§0 全清单 + §3 逐条建议 + §8 待拍板）

**未能确认**（需决策或实机）：三条 mental-health RPC 的前端业务需求（D1/D5）；删 `Logout` proto 方法对前端登出流程的影响（D3）；`protoc` 重生成在本机是否一次成功（Phase 2 实测）。
