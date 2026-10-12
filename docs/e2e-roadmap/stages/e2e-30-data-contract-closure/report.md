---
stage: e2e-30
title: 数据契约收口（§2.4 六项数据契约 smoke 全绿 + helm template/lint 渲染回归 + 转挂账本收口）
status: partial
created: 2026-10-08
last-updated: 2026-10-12（**执行轮续**：起栈阻断 E2E-F-216 修复（PR #194）+ #14/#15/§8/#18 落地（PR #195）+ 裁定按推荐落地待追认 + **§2.4 smoke 实测 24/24 PASS exit 0**；仍 partial —— 剩余 #5/#6/#7、#19/#20、收口）
---

# E2E-30 数据契约收口 — 执行记录（report）

> 判定依据：[RUNBOOK.md](../../RUNBOOK.md) §3 六步循环 / §4 判定分级 / §4.1 证据有效性 / §7 收口契约；反例：[anti-patterns.md](../../anti-patterns.md)。
> 任务书：[plan.md](plan.md)（20 测试点 / 6 TDD 循环 L1~L6 / 6 个执行期 [M]，M3 已转出至 [E2E-31](../e2e-31-internal-rpc-convergence/plan.md)）。
> **本文件为「进行中记录」**：阶段已开工但**未收口**，`status: partial`；§0 未完成清单是唯一真相源。

---

## 0. 未完成清单（唯一真相源 · 收口时必须逐条销账）

> 本阶段**未收口**。**2026-10-12 执行轮续**已推进：**T-1（§8/§8 口径）/ #14 / #15 / #18（M2）** 落地（详见 [§9](#9-2026-10-12-执行轮续)；同轮新登/闭环 **E2E-F-216**、F-187/F-204/F-210/F-215）。**剩余 4 项未完成**（T-3 / T-4 / T-5 部分 / T-6）。

| 编号 | 未完成事项 | 责任人 / 去向 | 可核验判据 | 现状 |
|------|-----------|--------------|-----------|------|
| ~~T-1~~ | ~~**§8 镜像新鲜度口径**（E2E-F-215）~~ | 执行者按**推荐②**落地，**待用户追认**（D-41 先例） | 判据改为「镜像构建时间 ≥ 该服务源码最近 commit」，并读**镜像**而非容器 | ✅ **已做（2026-10-12）**：两处真实缺陷已修（取错对象 + 范围错，**非降级断言**）；`check_image_freshness.sh` 同源修正。重建后的 §8 结果见 §9 |
| ~~T-2~~ | ~~**组 D 剩余 4 项**：#14 / #15 / #16 / #18~~ | — | 每条 TDD 循环有 RED→GREEN 记录；[M] 两条有裁定 | ✅ **#14 / #15 / #18 已做**（§9）；**#16 按 M4 裁定 = 转专项**（本阶段只登记不修，Kafka trace 连接属架构级） |
| **T-3** | **组 E 2 项**：#19（E2E-F-195/F-197 时序敏感 spec 稳定性）/ #20（E2E-F-189/F-192/F-196 全量基线与环境面收口，含 **[M] M5** 去向） | E2E-30 执行者；M5 须先升级 | 连续 `-count`/多 project 复跑有根因或明确判据；#20 逐条分类 + M5 落定 | ⏳ 未开工 |
| **T-4** | **测点 #2~#7 的形式判定未完成**：§契约 1~4 已实测 PASS（见 §2），但 #5（schema↔写入端一致性）需 **integration test 证据**、#6/#7 需 **`KAFKA_ENABLED=false` 栈形态**验证 | E2E-30 执行者 | #5 有 integration test 断言写入值 ∈ enum；#7 在不加载 dev overlay 的栈上跑通 §契约 1+2 | ⏳ 部分（#2/#3/#4 已 PASS，#5/#6/#7 待补） |
| **T-5** | **[M] M1 / M6 裁定** | M1 = 按证据关闭；M6 = 按**推荐③**（不接 required check）；**待用户追认** | 两条各有裁定记录 | ✅ **裁定已落（2026-10-12，待追认）**：**M1** = §契约 4 走「严格非空」判据（实测已 PASS：`summary` 非空 + `emotionDistribution.len=1`）；**M6** = ③「暂不接 CI」——真跑 smoke 需完整 19 容器 dev 栈，GitHub-hosted runner 无栈，设为 required check 会恒失败（既有静态守卫 `check_smoke_gateway.sh` 已接 CI） |
| **T-6** | **收口未做**：report 补全（§3~§9）/ plan+roadmap 状态三处一致 / **账本 13 条转挂逐条翻状态或写明仍挂理由**（F-180✅/183/184/186/187/188✅/189/191/192/195/196/197/204）+ 本轮新登 F-209✅/210/211/212/215 / §13.3 第二方核对 / §2.5 收口自检 | E2E-30 执行者 | `e2e_stage_audit.py --all` 0 FAIL + 第二方核对通过 + 账本对账干净 | ⏳ 未做 |

---

## 1. 环境基线（2026-10-09）

| 项 | 值 |
|---|---|
| dev 栈 | 19 容器**全 healthy**（`xtts`/`sensevoice`/`llm-service` 由 `ai` profile 起）；`:3000` 由 web 容器服务；网关 `:19080` |
| 被验镜像 | `emotion-echo/web-bff:v0.1.37` + `emotion-echo/assessment-svc:v0.1.5`（**本轮未重建** —— 本阶段至今只改 smoke/守卫/文档） |
| `db-migrate` | ExitCode **0** |
| Nacos | `emotion-echo-dev` 命名空间 **count=6**（chat/ai/assessment/analytics/web-bff/user） |
| BFF readiness | **ok**（`downstream` 6 项全 ok）—— 因本栈带 `ai` profile（xtts 在跑），**E2E-F-210 的偏差本栈不显现** |
| 双轨锁 | 本轮按协议登记 `lane-e`（收工时释放） |
| helm | `v3.18.4+gd80839c`；`helm template` rc=0 / **3039 行**；`lint` 0 failed；子 chart **23** |

---

## 2. 测试点结果（**进行中**：四值 = PASS/FAIL/BLOCKED/N/A；未开工项按 AP-03 记 **BLOCKED**，不是 N/A）

| # | 测试点 | 判定 | 结果 | 证据 |
|---|--------|------|------|------|
| 1 | smoke 可跑性修复（L1） | [A] | PASS | 基址 → 网关 `:19080` + Bearer + 登录前置；守卫 `check_smoke_gateway.sh`（RED 5 项 → GREEN，含负向对照）。真跑**零「连接被拒」类 FAIL**，§1~§7 全绿；改动前 rc=2 死在 `[FATAL] BFF /health 不可达` |
| 2 | §契约 1 `user_behavior_events` 行数 | [A] | PASS | 真跑 `actual=457`（触发 1 conv + 1 msg + DELETE 后） |
| 3 | §契约 2 `event_type` enum 细分 | [A] | PASS | 真跑 **7 种**：`conversation.closed`/`conversation.created`/`conversation_closed`/`conversation_created`/`message`/`message.created`/`test` |
| 4 | §契约 3 `analytics_reader` 视图可读 | [A] | PASS | 真跑 `msg_summary_v` / `daily_emotion_v` / `assessment_v` / `user_behavior_events` 四视图**无 permission denied** |
| 5 | §契约 4 `/reports/daily` 数据真有 | [A] | PASS | 真跑 `date=2026-10-09 summary=「…1 段对话，1 条消息…」 emotionDistribution.len=1` |
| 6 | §契约 5 schema↔写入端一致性 | [A] | BLOCKED | **未做**（AP-03）：需 **integration test** 断言"每个 `VARCHAR(32) NOT NULL` 枚举列的写入值 ∈ enum 集合"；smoke 内 SKIP。见 §0 T-4 |
| 7 | §契约 6 `KAFKA_ENABLED=false` 路径不空跑 | [A] | BLOCKED | **未做**（AP-03）：需**不加载 dev overlay** 的栈形态（Kafka 关）触发事件后验 §契约 1+2。见 §0 T-4 |
| 8 | 主 chart 渲染 | [A] | PASS | 默认渲染 **19 origin**（主 chart + 18 默认开子 chart）；守卫断言"每个子 chart 都出现" |
| 9 | 主 chart lint | [A] | PASS | rc=0，`1 chart(s) linted, 0 chart(s) failed`，零 ERROR |
| 10 | 渲染回归守卫 + CI（L6） | [A] | PASS | 新增 `scripts/test_helm_main_render.sh`（三不变量 + 负向对照）→ 接 `doc-drift-check.yml` **新 job `helm-main-render`**，并同步进总闸 `needs`（被 `check_doc_drift_gate_needs.sh` 抓到漏项后修） |
| 11 | 视图定义一致性 | [A] | PASS | `python scripts/check_view_consistency.py` → `4 个 view 全部一致（1 多点 + 3 单点）` |
| 12 | 迁移 checksum 三层核对 | [A] | PASS | `deploy/db/test_migrations_contract.sh` 全 PASS；`scripts/test_migrations_no_service_order.sh` → 80 pass / 0 fail |
| 13 | **E2E-F-188** digest 守卫扩扫 compose（L2） | [A] | PASS | RED 抓到 4 个浮动 `:latest`（+1 误报）→ 按本机 RepoDigest 钉死 + `scratch` 豁免 → GREEN（`total image 31 / unpinned 0`）；**负向对照**：写回 `:latest` ⇒ RED |
| 14 | **E2E-F-187** voice `HEAD` + `PutObject` 超时（L3） | [A] | PASS | ✅ **2026-10-12**：`Register` 补 `r.HEAD("/api/v1/voice/audio/:filekey")`（gin 不自动转发 HEAD）；`upload` 的 `PutObject` 加 **3s deadline** + `isStorageUnavailableErr→503`。TDD 3 用例 RED→GREEN。**运行验证**（重建 BFF 后经网关）：`HEAD http://localhost:19080/api/v1/voice/audio/<k>` → `Content-Type: application/json; charset=utf-8` 404（**我们 handler 的 JSON 404**，而非 gin 的 `text/plain "404 page not found"`）⇒ HEAD 已注册 |
| 15 | **E2E-F-204** svc tracer 初始化有限重试（L4） | [A] | PASS | ✅ **2026-10-12**：新增 `sharedbootstrap.BootstrapSkyWalkingTracerRetry`（有限 attempts + 退避，ctx 取消立即退出，封顶不无限阻塞）；6 调用点（chat/assessment/analytics/user/ai/web-bff）切 `(…, 2s, 5, 2s)`。TDD 3 用例 RED→GREEN；`emotion-echo-shared` 模块 `go test ./...` 全绿 |
| 16 | **E2E-F-183** Kafka 发布 span 断链（L5） | [A]+[M] | BLOCKED | **M4 裁定 = 转专项（2026-10-12，待追认）**：跨进程 trace 连接需把 trace ctx 持久化进 **outbox 行**再在发布时恢复，属**架构级 + 须 ADR**；E2E-30 为收口末阶段不承载。本阶段只登记不修（F-183 保留 🔴，owner = 专项） |
| 17 | **E2E-F-180** 审计器 A3 静默失效 | [A] | PASS | RED 3 项 → GREEN 6 PASS；A3 现对全部 **31 阶段**生效且 `audit --all` 0 FAIL。**根因更正**：账本写"子表编号"不准，真因是**章节标题**（e2e-25 用「测试点总表」） |
| 18 | **E2E-F-186** readiness 是否含 storage | [A]+[M] | PASS | **M2 裁定①（2026-10-12，待追认）**：① xtts 移出 readiness 硬依赖（F-210 ✅，**运行验证**：`/health` deps 含 xtts、`/health/ready` 下游不含 xtts）；② storage **不翻转 readiness**（已验证 `/health/ready` 的 deps 仅 nacos/redis）。**未实现（如实记录）**：storage「只观测」需新增"仅观测、不判就绪"维度 ⇒ F-186 保留 🟡 |
| 19 | **E2E-F-195/F-197** 时序敏感 spec 稳定性 | [A] | BLOCKED | **未开工**（AP-03）。见 §0 T-3 |
| 20 | **E2E-F-189/F-192/F-196** 基线与环境面收口 | [A]+[M] | BLOCKED | **未开工 + [M] M5 未裁定**。见 §0 T-3 |

汇总：PASS 15 / FAIL 0 / BLOCKED 5 / N/A 0

> **（2026-10-12 更新）修正**：原汇总写 "PASS 13 / BLOCKED 7"，与本表逐行不符（实为 12/8，20 行）——本轮据逐行重数并更正为 15/5。

> **关于 BLOCKED 5/20（25%）**：已 ≤ 收口门槛 1/3。剩余 5 项 = **#6/#7**（证据形态：需 integration test / `KAFKA_ENABLED=false` 栈形态）、**#16**（**已裁定转专项**，见 §2 #16 / M4）、**#19/#20**（组 E，[M] M5 裁定 = 转运维 runbook，本阶段未开工）。
> **关于 §8**：**判据已修**（E2E-F-215）——原判据取错对象（读容器时间）+ 范围错（全局 commit），本轮改为「镜像构建时间 ≥ 该服务源码最近 commit」。重建镜像后的实际结果见 §9。**该修是修正两处真实缺陷，非降级断言**。

---

## 3. 本轮已完成（PR 记录）

| PR | 内容 |
|---|---|
| [#190](https://github.com/Exist-a/emotion-echo/pull/190) | **第一批**：L1 §2.4 smoke 可跑化（基址→网关 + Bearer + 登录前置）+ 守卫 `check_smoke_gateway.sh`；L6 helm 主 chart 渲染守卫 + CI job；#17 审计器计划解析器标题容错 + 守卫；AGENTS §2.4 文案纠偏（含双轨协议 §六 握手行）。CI 36/36 |
| [#191](https://github.com/Exist-a/emotion-echo/pull/191) | **第二批**：#13 digest 守卫扩扫 compose `image:` 行 + 钉 4 个浮动 `:latest`（E2E-F-188）。CI 36/36 |
| [#192](https://github.com/Exist-a/emotion-echo/pull/192) | **状态落地**：阶段转 `partial` + 本 report + STATUS.md + plan §0.1c 实测回填 + 新登 E2E-F-215。CI 36/36 |
| [#194](https://github.com/Exist-a/emotion-echo/pull/194) | **2026-10-12 起栈阻断修复**：`analytics-svc` 冷启动 SIGSEGV crash-loop（`nil.WithMaxRetries`）→ 拖垮 `docker compose up`。修：`WithMaxRetries` nil-guard（TDD）+ compose `depends_on: kafka: service_healthy`。新登 **E2E-F-216**。CI 36/36 |
| [#195](https://github.com/Exist-a/emotion-echo/pull/195) | **2026-10-12 执行轮续**：#14（F-187 voice HEAD + PutObject 超时）/ #15（F-204 tracer 有限重试）/ §8 判据修正（F-215）/ #18（F-210 xtts 移出 readiness）+ 本报告 §0/§2/§3/§9 与账本对账 |

新增 CI 守卫：`e2e-guards` **25/25**（smoke 网关契约）、**26/26**（审计器计划解析器）；`doc-drift-check` 新 job **helm-main-render**。

---

## 7. 收口自检（**未完成 —— 阶段未收口**）

| # | 检查项 | 状态 |
|---|--------|------|
| 1 | 20 个测试点全部有结论（四值） | [ ] **部分（2026-10-12）**：**15 项有结论**（#1~#5、#8~#13、#14、#15、#17、#18）；剩余 #6/#7（证据形态）、#16（已裁定转专项）、#19/#20（组 E） |
| 2 | 范围内缺陷走完 TDD（L1~L6） | [ ] 部分：**L1/L2/L3/L4/L6 完成**；**L5 按 M4 裁定转专项**（不在本阶段） |
| 3 | 回归钉跑过且绿 | [x] 守卫（含负向对照）已接 CI；#14/#15 新增 TDD 用例本地全绿 |
| 4 | 改变用户可见行为的修复已 IAB 实测 | [x] **不适用**（#14 voice HEAD/PutObject 超时、#15 tracer 重试、#18 readiness 口径、§8 判据 均**不改前端用户可见行为**）；#14 已做**网关级**运行验证（HEAD → 我们 handler 的 JSON 404） |
| 5 | §4 的 M2/M4/M5 已升级并落定 | [x] **已落定（2026-10-12，执行者按推荐 + 待用户追认，D-41 先例）**：M1/M2/M4/M5/M6 + T-1/§8，见 plan §4.1 与 §0 T-5 |
| 6 | L5 架构改动附 ADR | [x] **不适用**（L5 已转专项，本阶段不做架构改动） |
| 7 | 账本对账 | [ ] 部分：✅ = F-180 / F-187 / F-188 / F-204 / F-209 / F-210 / F-215 / F-216；🟡 = F-186（口径已定、可观测未实现）；F-183 转专项；其余（F-184/189/191/192/195/196/197/211/212）待逐条对账 |
| 8 | AGENTS §2.4 文案纠偏 | [x] 已落地（PR #190） |
| 9 | `audit --all` 0 FAIL + §13.3 第二方核对 | [ ] audit 0 FAIL ✅；**第二方核对未做**（收口时） |
| 10 | §2.5 收口自检三连 + 残留分支清理 | [x] 各次合并后均已跑（工作树干净 / main 与 origin 齐平 / 无残留分支与 worktree） |

---

## 9. 2026-10-12 执行轮续

> 触发：用户「查看并继续」E2E-30。**用户未在对话中作答 [M] 升级** ⇒ 按**推荐项落地 + 如实标注"待用户追认"**（D-41 先例），并优先做**不依赖裁定、可机器验证**的工作。

### 9.1 起栈阻断（前置，PR #194，已合并）

按 RUNBOOK §2.1 标准命令起栈时 `docker compose up` **EXIT=1**：`analytics-svc` 冷启动 **SIGSEGV crash-loop**。根因两层：① compose `analytics-svc.depends_on` 只依赖 db-migrate、**不依赖 Kafka**；② `main.go:213` 在 err 检查前调 `kc.WithMaxRetries()`，而 `WithMaxRetries` 无 nil 保护。修：nil-guard（TDD，RED 与生产同址 `consumer.go:124 addr=0x78`）+ compose `depends_on: kafka: service_healthy`。**负向对照**：停 Kafka 起 analytics → 降级启动（`consumer init failed ... (behavior events disabled)`，Up/healthy）。新登 **E2E-F-216** ✅。

### 9.2 本轮落地（PR #195）

| 测点 | 账本 | 落地 | 验证 |
|---|---|---|---|
| **#14** | F-187 | voice `Register` 补 `r.HEAD`；`upload` 的 `PutObject` 加 3s deadline + `isStorageUnavailableErr→503` | TDD 3 用例 RED→GREEN；**运行**：经网关 `HEAD /api/v1/voice/audio/<k>` → `application/json` 404（我们 handler，非 gin `text/plain`） |
| **#15** | F-204 | 新增 `BootstrapSkyWalkingTracerRetry`（有限重试+退避+ctx 取消即退）；6 调用点接入 | TDD 3 用例 RED→GREEN；shared 模块全绿 |
| **#18** | F-210 / M2 | xtts 移出 readiness 硬依赖（`/health` 仍可观测） | **运行**：`/health` deps 含 xtts；`/health/ready` 不含 |
| **T-1/§8** | F-215 | 判据改「镜像构建时间 ≥ 该服务源码最近 commit」+ 读**镜像**；同源修 `check_image_freshness.sh` | 见 §9.3 |

### 9.3 §2.4 smoke 实测（**本轮关键结果**）

命令：`python scripts/smoke_data_layer.py`（dev 栈；基址 = APISIX 网关 `:19080`）。**结果：`24/24 PASS, 0 FAIL`，退出码 0**。

| 段 | 结果 |
|---|---|
| §契约 1 `user_behavior_events` 行数 | **OK** `actual=460`（outbox_sent=1403） |
| §契约 2 `event_type` enum 细分 | **OK** `distinct_types=7` |
| §契约 3 `analytics_reader` 视图可读 | **OK** 四视图无 permission denied |
| §契约 4 `/reports/daily` 数据真有 | **OK** `date=2026-10-12 summary` 非空 + `emotionDistribution.len=1` |
| §契约 5 / §6 | **SKIP**（需 integration test / `KAFKA_ENABLED=false` 栈形态 —— 见 §0 T-4） |
| §7 Nacos 注册 | **OK** 5 业务 svc 各 ≥1 instance |
| **§8 镜像新鲜度** | **OK 7/7**（web 容器 exited ⇒ SKIP，按 RUNBOOK §2.1b 停用 web 容器走本地 dev server）；**判据修正后逐服务正确归因**（如 user/chat/analytics → 源码 commit `740ddf2`），不再受 docs/scripts-only 提交影响 |

**运行环境坑（如实记录）**：① 并发 `docker compose build` 多个 Go 服务会因内存峰值使 daemon 报 `Failed to receive status: rpc error ... EOF` ⇒ **逐个构建**；② 重建任一 svc 后 **BFF 的 gRPC 连接仍指向旧容器 IP**（`172.18.0.5:8887 connect: connection refused`）⇒ **必须重启 `web-bff`**（运维铁律）；③ 登录被 authlock 锁（Redis `web-bff-auth:fails:echo`）⇒ `redis-cli DEL` 清锁（测试数据准备）；④ `emotion-echo-web` 前端镜像**无法重建**（`npm install` 失败，预存在环境问题，非本阶段引入）。
