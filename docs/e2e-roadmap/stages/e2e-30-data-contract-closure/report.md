---
stage: e2e-30
title: 数据契约收口（§2.4 六项数据契约 smoke 全绿 + helm template/lint 渲染回归 + 转挂账本收口）
status: done
created: 2026-10-08
last-updated: 2026-10-13（**收口轮 → ✅ done**：E2E-30 数据契约收口**收口** —— 名下 **11 条** A5 阻塞账本按用户 2026-10-13 裁定「转专项」（owner 去阶段编号 + 写明去向与触发条件）⇒ `e2e_stage_audit.py --all` **0 FAIL**；T-6 销账 + §2.5 收口自检三连通过。**E2E 路线图 30 阶段自此全部收口**）。**上一轮**：2026-10-12（**执行轮续**：起栈阻断 E2E-F-216 修复（PR #194）+ #14/#15/#18/#6/#7/§8 落地（PR #195/#197/#198）+ **裁定经用户逐条追认**（plan §4.1）+ #19/#20 结论（PR #199）；**§2.4 smoke 实测 24/24 PASS exit 0**（**实测时点 = 05:2x、镜像刚重建后**；此后服务源码提交使 §8 再次 STALE ⇒ 现跑 17/24，§契约 1~4/§7 仍全绿）；仍 partial —— **仅剩 T-6 收口**）
---

# E2E-30 数据契约收口 — 执行记录（report）

> 判定依据：[RUNBOOK.md](../../RUNBOOK.md) §3 六步循环 / §4 判定分级 / §4.1 证据有效性 / §7 收口契约；反例：[anti-patterns.md](../../anti-patterns.md)。
> 任务书：[plan.md](plan.md)（20 测试点 / 6 TDD 循环 L1~L6 / 6 个执行期 [M]，M3 已转出至 [E2E-31](../e2e-31-internal-rpc-convergence/plan.md)）。
> **本文件为「进行中记录」**：阶段已开工但**未收口**，`status: partial`；§0 未完成清单是唯一真相源。

---

## 0. 未完成清单（唯一真相源 · 收口时必须逐条销账）

> **✅ 2026-10-13 已收口 → 阶段判 `done`**。收口轮处置：**T-6 销账**（状态三处一致 `partial → done` + §2.5 收口自检三连）——其先前阻塞的「A5 阻塞」由用户 2026-10-13 裁定 **「转专项」** 解除；名下 **11 条** A5 阻塞账本 owner 去阶段编号 + 写明去向与触发条件（见下表下「A5 阻塞」）。`e2e_stage_audit.py --all` **0 FAIL**。
>
> **📌 关键更正（收口轮实测发现）**：原文写「名下 7 条未解决」**低估** —— 审计器 A5 口径实际为 **11 条**（多出 `F-183`/`F-186`/`F-192`/`F-196`；`F-183` 已是「转专项」但 owner 仍写字面量 `E2E-30`，`F-186`/`F-192`/`F-196` 状态用「已关闭/已文档化·转运维」，不在审计器 `RESOLVED_MARKERS` 词表内 ⇒ 均仍被 A5 计入）。11 条已一并处置。
>
> 本阶段**已收口**。**2026-10-12 执行轮续**推进：T-1/T-2/T-3/T-4/T-5 销账 + #14/#15/#18/#6/#7 落地（详见 [§9](#9-2026-10-12-执行轮续)）。

| 编号 | 未完成事项 | 责任人 / 去向 | 可核验判据 | 现状 |
|------|-----------|--------------|-----------|------|
| ~~T-1~~ | ~~**§8 镜像新鲜度口径**（E2E-F-215）~~ | 执行者按**推荐②**落地；**用户 2026-10-12 追认「保持致命」** | 判据改为「镜像构建时间 ≥ 该服务源码最近 commit」，并读**镜像**而非容器 | ✅ **已做（2026-10-12）**：两处真实缺陷已修（取错对象 + 范围错，**非降级断言**）；`check_image_freshness.sh` 同源修正。重建后的 §8 结果见 §9 |
| ~~T-2~~ | ~~**组 D 剩余 4 项**：#14 / #15 / #16 / #18~~ | — | 每条 TDD 循环有 RED→GREEN 记录；[M] 两条有裁定 | ✅ **#14 / #15 / #18 已做**（§9）；**#16 按 M4 裁定 = 转专项**（本阶段只登记不修，Kafka trace 连接属架构级） |
| ~~T-3~~ | ~~**组 E 2 项**（#19 / #20）~~ | E2E-30 执行者；M5 = 转运维 runbook（已追认） | #19 有复跑判据；#20 有逐条分类 | ✅ **已做（2026-10-12，§2 #19、#20；§9.7、§9.8）** |
| ~~**T-4**~~ | ~~**测点 #6/#7 证据形态**~~ | E2E-30 执行者 | #6 有 integration test 断言写入值 ∈ enum；#7 在 `KAFKA_ENABLED=false` 形态下跑通 §契约 1+2 | ✅ **#6 / #7 均已做（2026-10-12，§2 #6、#7；§9.5、§9.6）** |
| ~~T-5~~ | ~~**[M] M1 / M6 裁定**~~ | M1 = 按证据关闭；M6 = **推荐③**（不接 required check） | 两条各有裁定记录 | ✅ **已追认（2026-10-12 用户确认）**：**M1** = §契约 4 走「严格非空」判据（实测已 PASS：`summary` 非空 + `emotionDistribution.len=1`）；**M6** = ③「暂不接 CI」——真跑 smoke 需完整 19 容器 dev 栈，GitHub-hosted runner 无栈，设为 required check 会恒失败（既有静态守卫 `check_smoke_gateway.sh` 已接 CI） |
| **T-6** | **收口**：report 补全（§3~§9）/ plan+roadmap 状态三处一致 / 账本逐条对账 / §13.3 第二方核对 / §2.5 收口自检 | E2E-30 执行者 | `e2e_stage_audit.py --all` 0 FAIL + 第二方核对通过 + 账本对账干净 | ✅ **已做（2026-10-13 收口轮）**：report §3~§9 补全 ✅ / 账本逐条对账 ✅（11 条转专项）/ **§13.3 第二方核对 ✅（三轮：FAIL → 修 7 项 → 复验 2 项未闭环 + 2 处新矛盾 → 再修 4 项 → 末轮 PASS）** / **状态三处一致 `partial → done` ✅** / **§2.5 收口自检三连 ✅** |

**A5 阻塞 —— ✅ 已解除（2026-10-13）**：审计器 A5 规定「**`done` 阶段不得持有归属本阶段的未解决账本条目**」。**用户 2026-10-13 裁定 = 「转专项」**（原两选一：① 转「专项」／② 保持 `partial`）。**收口轮实测 A5 冲突 = 11 条**（非原文所写 7 条）：`F-183` / `F-184` / `F-186` / `F-191` / `F-192` / `F-195` / `F-196` / `F-197` / `F-212` / `F-217` / `F-219` —— 多出的 4 条中，`F-183` 已是「转专项」但 owner 仍写字面量 `E2E-30`；`F-186`（已关闭）/ `F-192` / `F-196`（已文档化·转运维）状态用词不在审计器 `RESOLVED_MARKERS`（已解决/已修复/全部修复/降级并记录/已知限制/不归属阶段）内 ⇒ 均仍被 A5 计入。**处置**：11 条 owner 统一去掉阶段编号（改为「专项 · &lt;主题&gt;」）+ 逐条写明**去向与触发条件**（审计器 A5 只读 owner 列的 `E2E-&lt;数字&gt;` 字面量）。**结果**：A5 冲突 **11 → 0**，`e2e_stage_audit.py --all` **0 FAIL**。

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
| 6 | §契约 5 schema↔写入端一致性 | [A] | PASS | ✅ **2026-10-12**：按**权威 DDL** 清点「`VARCHAR(32) NOT NULL` 枚举列」= **2 列**（`messages.intent` / `fused_emotions.primary_emotion`；其余同型列 `purpose`/`category`/`risk_level`/`voice|face.primary_emotion` 等**均 nullable**，不属本判据）。各加 **integration test**（testcontainers 真库往返）：`chat-svc/integration_test/intent_enum_integration_test.go`（列宽 32 + NOT NULL + 白名单 6 值逐个原样读回）、`ai-svc/integration_test/fused_emotion_enum_integration_test.go`（列宽 32 + NOT NULL + 9 情绪逐个原样读回）。**顺带修一处真实漂移**：`model.Message.Intent` GORM tag 仍是 `size:16` 而 DDL 已由 c007 扩到 `VARCHAR(32)` ⇒ 改 `size:32` + 新增 `TestMessage_IntentGormTagSize_MatchesDDLWiden` 钉住。**新登 E2E-F-217**（`user_oauth` 残留表）。见 §9.5 |
| 7 | §契约 6 `KAFKA_ENABLED=false` 路径不空跑 | [A] | PASS | ✅ **2026-10-12**：以 `KAFKA_ENABLED=false` 重建 chat-svc（走 `DevEventPublisher` 直写）→ 触发事件 → smoke §6 **`OK: 近 10 分钟内 15 行由 DevEventPublisher 直写`**；同轮 §契约 1~5 / §7 仍全绿。**顺带修两处**：① compose 把 chat-svc `KAFKA_ENABLED` 由**硬编码 `"true"`** 改为 `${KAFKA_ENABLED:-true}`（此前 §6 分支**不可触发**，smoke 恒 SKIP）；② smoke §6 判据 `event_id LIKE 'smoke-%'` 是**恒 FAIL 的假判据**（event_id 全为 UUID）→ 改为**时间窗**（**新登 E2E-F-218**）。见 §9.6 |
| 8 | 主 chart 渲染 | [A] | PASS | 默认渲染 **19 origin**（主 chart + 18 默认开子 chart）；守卫断言"每个子 chart 都出现" |
| 9 | 主 chart lint | [A] | PASS | rc=0，`1 chart(s) linted, 0 chart(s) failed`，零 ERROR |
| 10 | 渲染回归守卫 + CI（L6） | [A] | PASS | 新增 `scripts/test_helm_main_render.sh`（三不变量 + 负向对照）→ 接 `doc-drift-check.yml` **新 job `helm-main-render`**，并同步进总闸 `needs`（被 `check_doc_drift_gate_needs.sh` 抓到漏项后修） |
| 11 | 视图定义一致性 | [A] | PASS | `python scripts/check_view_consistency.py` → `4 个 view 全部一致（1 多点 + 3 单点）` |
| 12 | 迁移 checksum 三层核对 | [A] | PASS | `deploy/db/test_migrations_contract.sh` 全 PASS；`scripts/test_migrations_no_service_order.sh` → 80 pass / 0 fail |
| 13 | **E2E-F-188** digest 守卫扩扫 compose（L2） | [A] | PASS | RED 抓到 4 个浮动 `:latest`（+1 误报）→ 按本机 RepoDigest 钉死 + `scratch` 豁免 → GREEN（`total image 31 / unpinned 0`）；**负向对照**：写回 `:latest` ⇒ RED |
| 14 | **E2E-F-187** voice `HEAD` + `PutObject` 超时（L3） | [A] | PASS | ✅ **2026-10-12**：`Register` 补 `r.HEAD("/api/v1/voice/audio/:filekey")`（gin 不自动转发 HEAD）；`upload` 的 `PutObject` 加 **3s deadline** + `isStorageUnavailableErr→503`。TDD 3 用例 RED→GREEN。**运行验证**（重建 BFF 后经网关）：`HEAD http://localhost:19080/api/v1/voice/audio/<k>` → `Content-Type: application/json; charset=utf-8` 404（**我们 handler 的 JSON 404**，而非 gin 的 `text/plain "404 page not found"`）⇒ HEAD 已注册。**IAB 实测（2026-10-12）**：真实浏览器登录 → 会话详情页存在语音气泡，其 `<audio>` 从**网关反代 URL**（`http://localhost:19080/api/v1/voice/audio/…webm`）加载完成并播完（`readyState=4` / `duration=1.74s` / `error=null`）；在**浏览器 origin** 下 `fetch` 该 URL 的 **HEAD 与 GET 均 200**（`audio/webm`, 29276B，无 CORS 报错）——curl 覆盖不到的这一层已验 |
| 15 | **E2E-F-204** svc tracer 初始化有限重试（L4） | [A] | PASS | ✅ **2026-10-12**：新增 `sharedbootstrap.BootstrapSkyWalkingTracerRetry`（有限 attempts + 退避，ctx 取消立即退出，封顶不无限阻塞）；6 调用点（chat/assessment/analytics/user/ai/web-bff）切 `(…, 2s, 5, 2s)`。TDD 3 用例 RED→GREEN；`emotion-echo-shared` 模块 `go test ./...` 全绿 |
| 16 | **E2E-F-183** Kafka 发布 span 断链（L5） | [A]+[M] | BLOCKED | **M4 裁定 = 转专项（2026-10-12，用户追认）**：跨进程 trace 连接需把 trace ctx 持久化进 **outbox 行**再在发布时恢复，属**架构级 + 须 ADR**；E2E-30 为收口末阶段不承载。本阶段只登记不修（F-183 保留 🔴，owner = 专项） |
| 17 | **E2E-F-180** 审计器 A3 静默失效 | [A] | PASS | RED 3 项 → GREEN 6 PASS；A3 现对全部 **31 阶段**生效且 `audit --all` 0 FAIL。**根因更正**：账本写"子表编号"不准，真因是**章节标题**（e2e-25 用「测试点总表」） |
| 18 | **E2E-F-186** readiness 是否含 storage | [A]+[M] | PASS | **M2 裁定①（2026-10-12，用户追认）**：① xtts 移出 readiness 硬依赖（F-210 ✅，**运行验证**：`/health` deps 含 xtts、`/health/ready` 下游不含 xtts）；② storage **不翻转 readiness**（已验证 `/health/ready` 的 deps 仅 nacos/redis）。**未实现（如实记录）**：storage「只观测」需新增"仅观测、不判就绪"维度 ⇒ F-186 保留 🟡 |
| 19 | **E2E-F-195/F-197** 时序敏感 spec 稳定性 | [A] | PASS | ✅ **2026-10-12（判据，非"重跑变绿"）**：① 定向跑 `performance-baseline` + `digital-human-tts`（chromium+mobile）→ **F-195 的 #12 与 F-197 相关用例均未复现**；② 对 #12 做 **`--repeat-each=4` × 2 project = 8/8 PASS**，gap = `431/377/456/365/634/532/682/656 ms`（稳定远离 1000ms 阈值）。**判据**：现有 #12 spec 覆盖的是"两段音频衔接"，而 **F-195 的失败形态（段在 pumpQueue 被静默丢弃）需 `new.vue`→`[id].vue` 跨路由实例的 unmount-stop 与段入队交错** —— spec **未覆盖该路径**，其绿不能证伪 F-195（与账本原判"修前 spec #12 在新时序下不可作门禁"一致）。F-197 同理。⇒ 需**专门覆盖该时序的新 spec** 才能复现/作门禁；F-195/F-197 保留 🔴（下一轮）。见 §9.7 |
| 20 | **E2E-F-189/F-192/F-196** 基线与环境面收口 | [A]+[M] | PASS | ✅ **2026-10-12**：**全量 Playwright 基线实跑**（chromium+mobile）→ **66 failed / 252 passed / 2 skipped（37.6m）**，逐条按根因分类（§9.8）：**本阶段域 0 条**；**26 条同源于"陈旧探针直连 `:8894`"（新登 E2E-F-219）**、2 条环境缺失（Grafana/obs 未起）、4 条夹具缺失（`TEST_USERNAME`）、~26 条负载/时序、4 条 E2E-15 数据断言。**M5**：F-192/F-196 已转运维（RUNBOOK §2.4）。见 §9.8 |

汇总：PASS 19 / FAIL 0 / BLOCKED 1 / N/A 0

> **（2026-10-12 更新）修正**：原汇总曾写 "PASS 13 / BLOCKED 7"，与本表逐行不符（当时实为 12/8）——本轮据逐行重数并更正（**当前值见上**）。

> **关于 BLOCKED 1/20（5%）**：远低于收口门槛 1/3。**唯一剩余 = #16**（Kafka trace 断链，**已由用户裁定 M4 转专项**、附独立计划文档，本阶段只登记不修）。
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

## 4. 发现与分类（账本对账）

> RUNBOOK §10 模板章节。本阶段名下账本逐条对账（**收口契约要求：逐条翻状态或写明仍挂理由**）。

| 账本 | 现象（摘要） | 归属 / 去向 | 状态 |
|---|---|---|---|
| **F-180** | 审计器 A3 静默失效（**真因=章节标题**，非账本原写的"子表编号"） | E2E-30 | ✅ 已修（PR #190） |
| **F-183** | Kafka 发布 span 断链（跨进程 trace 连接） | **转专项** → [docs/plans/outbox-trace-continuity.md](../../../plans/outbox-trace-continuity.md)（用户裁定 M4） | 🟡 转专项 |
| **F-184** | IAB 渲染帧与 DOM 不同步 ⇒ 截图证据失真 | 工具 / IAB 环境（或上游报） | 🔴 保留（去向已定：[V] 证据改由 Playwright 独立渲染栈产出，IAB 仅作 DOM 级验证） |
| **F-186** | readiness 不含 storage（MinIO 停不翻转） | E2E-30 | ✅ **关闭**（用户裁定「不翻转」即关闭，M2） |
| **F-187** | voice 同型两缺口（HEAD 404 / PutObject 无 deadline） | E2E-30 | ✅ 已修（PR #195） |
| **F-188** | digest 守卫不覆盖 compose `image:` | E2E-30 | ✅ 已修（PR #191） |
| **F-189** | 全量 Playwright 基线红（失败跨 11 个他阶段 spec） | E2E-30 **#20** | 🔴 → 本轮分类（见 §2 #20） |
| **F-191** | Grafana TTFT 面板/告警缺失 | E2E-30（记账不修） | 🔴 保留（触发条件：需要该面板/告警时再补，先红后绿） |
| **F-192** | 宿主→网关间歇 ConnectionReset 簇 | **转运维** | ✅ 已文档化（RUNBOOK §2.4；用户裁定 M5） |
| **F-195** | TTS 段播放层时序敏感（段被静默丢弃） | E2E-30 **#19** | 🔴 → 本轮复跑定位（见 §2 #19） |
| **F-196** | dev `:3000` 被 web 容器抢占 | **转运维** | ✅ 已文档化（RUNBOOK §2.4；用户裁定 M5） |
| **F-197** | mobile 视口 fill 后发送按钮不启用 | E2E-30 **#19** | 🔴 → 本轮复跑定位（见 §2 #19） |
| **F-204** | svc tracer 初始化无重试（OAP 后起永不上报） | E2E-30 | ✅ 已修（PR #195） |
| **F-209** | §2.4 smoke 在 dev 不可跑（基址 `:8894` 已被 D-47 收掉） | E2E-30 | ✅ 已修（PR #190） |
| **F-210** | BFF readiness 硬依赖 profile 门控的 xtts | E2E-30 | ✅ 已修（PR #195，运行验证） |
| **F-211** | 网关缺 BFF health 路由 ⇒ smoke 健康前置不可复核 | E2E-30 | ✅ **已解决**（L1 采用方案②：登录成功即证 BFF 可用，零新增路由；`check_smoke_gateway.sh` 钉住） |
| **F-212** | xtts「暂时停用」遗留面（readiness / TTS 回退链 / 编排镜像） | E2E-30 + D-50 | 🟡 **部分**：readiness 摘除已做（= F-210 ✅）；**TTS 回退链的用户可见行为 + 镜像清理未做**（随 D-50 落地） |
| **F-215** | §8 镜像新鲜度判据粗（非产物提交恒红） | E2E-30 | ✅ 已修（PR #195；用户裁定保持「致命」） |
| **F-216** | analytics-svc 冷启动 SIGSEGV ⇒ `compose up` EXIT=1 | E2E-30 | ✅ 已修（PR #194） |
| **F-217** | `user_oauth` 残留表（drop 脚本 initdb-only） | E2E-30 | 🔴 只登记（本轮不改） |
| **F-218** | smoke §6 判据恒 FAIL（假 FAIL） | E2E-30 | ✅ 已修（PR #198） |

## 5. 修复清单（TDD 记录）

| 循环 / 项 | RED | GREEN | PR |
|---|---|---|---|
| **L1** smoke 可跑化 | 静态守卫断言基址 ≠ `:8894`（当前 `:8894` ⇒ 必红） | 改走网关 `:19080` + Bearer + 登录前置 | #190 |
| **L2** digest 扩扫 compose | 写回未 pin 的 `:latest` ⇒ 守卫 RED | 扩扫 compose `image:` + 钉 4 个 + `scratch` 豁免 | #191 |
| **L3** voice HEAD / PutObject 超时 | HEAD 404（gin 不转发）/ 存储不可用返 500 且 ctx 无 deadline | 补 `r.HEAD` + 3s deadline + `isStorageUnavailableErr→503` | #195 |
| **L4** svc tracer 重试 | `undefined: BootstrapSkyWalkingTracerRetry` | 新增 `…TracerRetry`（有限 attempts+退避+ctx 取消即退），6 调用点 | #195 |
| **L5** Kafka trace 连接 | — | **未做**（M4 裁定转专项，附 ADR 的要求随专项） | — |
| **L6** helm 主 chart 渲染回归 | `test_helm_main_render.sh` 不存在 | 新增守卫（三不变量 + 负向对照）→ 接 CI job `helm-main-render` | #190 |
| **#6** 枚举列（非 L 循环） | `model.Message.Intent` tag 含 `size:16` | 改 `size:32` + 2 个 integration test（真库往返） | #197 |
| **#7** §契约 6（非 L 循环） | §6 判据 `event_id LIKE 'smoke-%'` 恒不匹配 | 改时间窗 + compose `KAFKA_ENABLED` 可覆盖 | #198 |
| **F-216** nil-guard | `panic … consumer.go:124 addr=0x78`（与生产同址） | `WithMaxRetries` nil-guard + compose `depends_on: kafka: service_healthy` | #194 |
| **F-180** 审计器 A3 | 解析器不认章节标题 ⇒ 3 项 RED | 标题容错（+ 守卫 26/26） | #190 |

## 6. 回归钉

| 钉 | 类型 | 接入 |
|---|---|---|
| `scripts/check_smoke_gateway.sh` | 静态（smoke 基址 ≠ `:8894`、含负向对照） | CI `e2e-guards`（25/25） |
| `scripts/test_helm_main_render.sh` | 静态（主 chart 三不变量 + 负向对照） | CI `doc-drift-check` job `helm-main-render` |
| `scripts/test_audit_plan_parser.sh` | 静态（A3 标题容错 + 负向对照） | CI `e2e-guards`（26/26） |
| `emotion-echo-chat-svc/internal/model/intent_contract5_test.go` | 契约（tag 宽度 == DDL） | `go test` |
| `emotion-echo-chat-svc/integration_test/intent_enum_integration_test.go` | 集成（真库往返 6 值） | `-tags integration` |
| `emotion-echo-ai-svc/integration_test/fused_emotion_enum_integration_test.go` | 集成（真库往返 9 情绪） | `-tags integration` |
| `emotion-echo-analytics-svc/internal/kafka/ops_retry_test.go`（nil receiver 用例） | 契约（nil-safe） | `go test` |
| `emotion-echo-shared/pkg/bootstrap/tracer_test.go`（retry 3 用例） | 契约（重试/退避/ctx 取消） | `go test` |
| `emotion-echo-web-bff/internal/handler/voice_handler_test.go`（HEAD / 超时 / 路由契约） | 契约 + 真 server HEAD | `go test` |
| `emotion-echo-web-bff/main_test.go`（readiness 探针 / 路由契约） | 契约 | `go test` |

> **前端 spec**：本阶段改动**均非用户可见行为**（voice HEAD/超时、tracer、readiness 口径、§8 判据），故未新增 Playwright spec；**IAB 实测**见 [§9.4](#94-iab-实测真实浏览器2026-10-12)。

## 7. 待决策 / 升级项

**已全部落定**：M1 / M2 / M4 / M5 / M6 + T-1/§8 经用户 2026-10-12 逐条追认（见 [plan.md](plan.md) §4.1）。

**收口轮（2026-10-13）**：A5 阻塞经用户裁定 **「转专项」** 解除 —— 名下 **11 条**未解决账本 owner 已去阶段编号 + 写明去向与触发条件（详见 §0）。**不再有归属本阶段的待决项**。

**转出「专项」的后续去向（不再是本阶段的责任）**：

| 项 | 内容 | 去向 / 触发条件（见账本 owner 列） |
|---|---|---|
| **F-217** | `user_oauth` 残留表修法（DROP 进 migrate.sh 扫描目录，或给 02 补幂等 `DROP TABLE IF EXISTS`） | 专项 · schema/迁移治理（下轮迁移治理时） |
| **F-219** | 3 个回归钉直连已收掉的 `:8894` | 专项 · 回归钉治理（下轮 spec 维护时） |
| **F-184** | IAB 渲染帧失真（截图证据不可信） | 专项 · IAB/截图证据工具治理（需 IAB 截图作证据时） |
| **F-191** | Grafana TTFT 面板/告警 | 专项 · 服务端 TTFT 埋点（需要面板/告警时） |
| **F-212** | xtts 停用的 TTS 回退链用户可见行为 + 镜像清理（随 D-50） | 专项 · TTS 回退策略（D-50 落地时） |
| **F-183** | Kafka trace 跨进程连接（架构级，须 ADR） | 专项 · 消息链路 trace 连接（计划落地时） |
| **F-186 / F-192 / F-195 / F-196 / F-197** | 健康观测延伸 / 宿主端口层 / TTS 播放层时序 / RUNBOOK 治理 / mobile 输入时序 | 专项（见账本 owner 列） |

## 8. 收口自检（**✅ 已完成 —— 2026-10-13 收口轮**）

| # | 检查项 | 状态 |
|---|--------|------|
| 1 | 20 个测试点全部有结论（四值） | [x] **已全部有结论（2026-10-12）**：**19 PASS** + **1 BLOCKED（#16，已裁定转专项）**；无 FAIL / N/A |
| 2 | 范围内缺陷走完 TDD（L1~L6） | [x] **L1/L2/L3/L4/L6 完成**；**L5 按 M4 裁定转专项**（不在本阶段） |
| 3 | 回归钉跑过且绿 | [x] 守卫（含负向对照）已接 CI；#14/#15 新增 TDD 用例本地全绿 |
| 4 | 改变用户可见行为的修复已 IAB 实测 | [x] **已做 IAB 实测（2026-10-12）**：#14 触及语音链路（用户可见）⇒ 真实浏览器实测 —— 登录 → 会话详情 → 语音气泡 `<audio>` 经网关反代**加载并播完**（`readyState=4`/`duration=1.74s`/`error=null`）；浏览器 origin 下 `HEAD`/`GET /api/v1/voice/audio/<k>` **均 200**（CORS 正常）；我的空间页正常渲染。**未覆盖（如实记录）**：真实麦克风**录音→上传**链路（IAB 无法授予麦克风权限）⇒ 上传侧 503 映射未在浏览器内验证（仅单测 + 网关 HEAD 代理） |
| 5 | §4 的 M2/M4/M5 已升级并落定 | [x] **已落定（2026-10-12，用户经 AskUserQuestion 逐条追认）**：M1/M2/M4/M5/M6 + T-1/§8，见 plan §4.1 与 §0 T-5 |
| 6 | L5 架构改动附 ADR | [x] **不适用**（L5 已转专项，本阶段不做架构改动） |
| 7 | 账本对账 | [x] **已完成（2026-10-13 收口轮）**：✅ = F-180 / F-187 / F-188 / F-189 / F-204 / F-209 / F-210 / F-211 / F-215 / F-216 / F-218；**11 条未解决按用户裁定转「专项」**（F-183/184/186/191/192/195/196/197/212/217/219，owner 去阶段编号 + 写明去向与触发条件）⇒ A5 冲突 **11 → 0** |
| 8 | AGENTS §2.4 文案纠偏 | [x] 已落地（PR #190） |
| 9 | `audit --all` 0 FAIL + §13.3 第二方核对 | [x] `audit --all` **0 FAIL** ✅；**§13.3 第二方核对已完成**（独立子代理三轮：首轮 **FAIL**（7 项）→ 修 7 → 复验（2 项未闭环 + 2 处新矛盾）→ 再修 4 → **末轮 PASS**） |
| 10 | §2.5 收口自检三连 + 残留分支清理 | [x] **2026-10-13 收口轮跑通**：工作树干净（Stage 113 已随 PR #202 入库；仅 `.zcodeignore` 工具文件未跟踪，非产物）/ main 与 origin 齐平 / 无残留已合并分支与 worktree |
| 11 | **状态三处一致**（plan / report / roadmap） | [x] 三处 `status: done`（2026-10-13）；roadmap 表格行 + 「当前激活」已同步（改为「无 —— 30 阶段全部收口 ✅」） |

### 8.1 第二方核对（2026-10-13 收口轮，独立子代理）

依 [RUNBOOK.md](../../RUNBOOK.md) §7 #10「**执行者不得自行宣布 done**」——由**独立于执行者的子代理**按 §13.3 十七条 + §7 十一项逐条核对本收口批次（只读，未改任何文件）。**结论：数据契约本体全部独立复算通过（非仅引用报告）**；抓出 **3 项**须处理，均已处置：

| 核对者发现 | 处置 |
|---|---|
| §7#7 / §7#8：核对该批时「未提交 / 未推送」；`git branch --merged main` 非空 | ✅ 核对时点早于本批 commit；提交推送后复验：`git branch --merged main` **仅 `main`**、`git status -sb` 无 ahead/behind、工作树仅 `.zcodeignore` 未跟踪 |
| §13.3#12 坏链：`docs/stages/stage-114-…md` 5 处 `../../e2e-roadmap/` 解析失败；`plan.md` §5 一处畸形链接（把 `[M4]` 指向了 `§4.1`，非路径） | ✅ 已修（改为 `../e2e-roadmap/`；`M4（§4.1）`）。**另发现 `docs/e2e-roadmap/` 下 26 处既有坏链**（系统性、非本批引入）—— 范围外，**登记账本 [E2E-F-220](../../discovered-unresolved.md)**，不在本阶段修 |
| §13.3#3：report §8#10 的 `[x]` 在提交前属 AP-12 型「假完成」 | ✅ 提交后为真；本行措辞据实（见 §8#10） |

**核对者独立复算项（自算，非引用报告）**：A5 冲突 `[]`（11→0；逐条抽查 11 条 owner 确无 `E2E-<数字>` 字面量且均含「去向 + 触发条件」）；汇总行 `19/0/1/0` 之和 = 表数据行数 20；plan 编号 ⊆ report 编号；账本编号 1..219 无缺号/重复/畸形；三处 `status` 均 `done`；`e2e_stage_audit.py --all` **31 阶段 0 FAIL**。

---

## 9. 2026-10-12 执行轮续

> 触发：用户「查看并继续」E2E-30。**首轮 [M] 升级未获回答** ⇒ 执行者按**推荐项落地**并如实标注来源；**随后用户 2026-10-12 经 AskUserQuestion 逐条追认**（见 plan §4.1）。全程优先做**不依赖裁定、可机器验证**的工作。

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

命令：`python scripts/smoke_data_layer.py`（dev 栈；基址 = APISIX 网关 `:19080`）。**结果：`24/24 PASS, 0 FAIL`，退出码 0**（**实测时点 2026-10-12 05:2x**，8 个服务镜像刚重建后）。**⚠️ 该「全绿」不是 standing result**：§8 判据为「镜像构建时间 ≥ 该服务源码最近 commit」，而此后落地的服务源码提交（#195 `abc3369` / #197 `3aeb0c7`）晚于镜像构建时间 ⇒ **现跑 §8 为 7 STALE（smoke 17/24、exit 1）**，§契约 1~4 与 §7 仍全绿。此为用户裁定「§8 保持致命」的**预期表现**（要全绿需重建镜像）。

| 段 | 结果 |
|---|---|
| §契约 1 `user_behavior_events` 行数 | **OK** `actual=460`（outbox_sent=1403） |
| §契约 2 `event_type` enum 细分 | **OK** `distinct_types=7` |
| §契约 3 `analytics_reader` 视图可读 | **OK** 四视图无 permission denied |
| §契约 4 `/reports/daily` 数据真有 | **OK** `date=2026-10-12 summary` 非空 + `emotionDistribution.len=1` |
| §契约 5 / §6 | **SKIP**（需 integration test / `KAFKA_ENABLED=false` 栈形态 —— 见 §0 T-4） |
| §7 Nacos 注册 | **OK** 5 业务 svc 各 ≥1 instance |
| **§8 镜像新鲜度** | **当时 OK 7/7**（web 容器 exited ⇒ SKIP，按 RUNBOOK §2.1b 停用 web 容器走本地 dev server）；**判据修正后逐服务正确归因**（如 user/chat/analytics → 源码 commit `740ddf2`），不再受 docs/scripts-only 提交影响。**⚠️ 该行是当时结果；之后的服务源码提交（#195/#197）使 §8 现为 7 STALE（见上段）** |

**运行环境坑（如实记录）**：① 并发 `docker compose build` 多个 Go 服务会因内存峰值使 daemon 报 `Failed to receive status: rpc error ... EOF` ⇒ **逐个构建**；② 重建任一 svc 后 **BFF 的 gRPC 连接仍指向旧容器 IP**（`172.18.0.5:8887 connect: connection refused`）⇒ **必须重启 `web-bff`**（运维铁律）；③ 登录被 authlock 锁（Redis `web-bff-auth:fails:echo`）⇒ `redis-cli DEL` 清锁（测试数据准备）；④ `emotion-echo-web` 前端镜像**无法重建**（`npm install` 失败，预存在环境问题，非本阶段引入）。

### 9.4 IAB 实测（真实浏览器，2026-10-12）

用户质询"iab 测了吗"⇒ 本轮补做。**环境**：`docker stop emotion-echo-web`（§2.1b）+ 本地 `pnpm dev --port 3000`（chunk URL 为源码路径形态，确认是真 dev server）；后端栈 + 网关 `:19080`。

| 步骤 | 结果 |
|---|---|
| 打开 `http://localhost:3000` | 301 → `/login`，登录页正常渲染（无报错） |
| 「用演示账号快速体验」登录 | 成功 → `/chat/conversation/new`，侧栏/会话列表/输入区渲染正常 |
| 打开会话 `#550` | 消息列表完整；输入区含 `添加附件`/`开启摄像头`/`开始录音`/`发送消息`；**存在语音气泡（`▶` + `0:01`）** |
| 点语音气泡 ▶ | `<audio>` 从 `http://localhost:19080/api/v1/voice/audio/60b58648-….webm` 加载完成并**播完**：`readyState=4` / `duration=1.739994` / `currentTime=1.739994` / `error=null` |
| 浏览器 origin 下 `fetch` 该 URL | **HEAD 200** + **GET 200**（`content-type: audio/webm`，`content-length: 29276`），无 CORS/网络报错 |
| 我的空间 `/chat/user` | 正常渲染（用户信息 / 相处记录 / 人格画像 分区；"暂无数据"为预期空态） |

**结论**：#14 的语音反代回放路径在**真实浏览器**下可用，且新增 **HEAD 路由经网关对浏览器 origin 可用**（CORS 正常）——这是 curl/Playwright APIRequest 覆盖不到的一层。
**未覆盖（如实记录）**：真实**麦克风录音 → 上传**（IAB 无法授予麦克风权限）⇒ #14 的**上传侧** `PutObject` 3s 超时 → 503 映射未在浏览器内验证（证据 = 单测 `TestVoiceHandler_Upload_StorageUnavailable_Returns503` + 网关 HEAD 代理行为）。

### 9.5 测点 #6（§契约 5）——枚举列写入值 ∈ enum 的 integration test（2026-10-12）

按**权威 DDL**（`deploy/db/02-create-tables-in-schemas.sql` + 各 svc `migrations/`）清点「`VARCHAR(32) NOT NULL` 枚举列」，结果 = **2 列**（其余同型列 `purpose`/`category`/`risk_level`/`voice|face.primary_emotion` 等**均 nullable**，不属本判据）：

| 列 | 写入端白名单 | integration test |
|---|---|---|
| `emotion_echo_chat.messages.intent` | `internal/logic.allowedIntents`（6 类） | `emotion-echo-chat-svc/integration_test/intent_enum_integration_test.go` — 列宽 = VARCHAR(32) NOT NULL；6 个白名单值经真实 repo `AppendMessage` → `ListMessages` **原样读回**；库内值 ∈ 白名单 ∪ {''} |
| `emotion_echo_ai.fused_emotions.primary_emotion` | `internal/fusion.allowedEmotions`（9 标签） | `emotion-echo-ai-svc/integration_test/fused_emotion_enum_integration_test.go` — 列宽 = VARCHAR(32) NOT NULL；9 情绪经 `PostgresFusedEmotionRepo.Upsert` → `GetByMessageID` **原样读回** |

**顺带修掉一处真实漂移（DDL↔model）**：`model.Message.Intent` 的 GORM tag 仍是 `size:16`，而 `migrations/c007_widen_intent_to_32.sql` 早已把 DDL 扩到 `VARCHAR(32)`。改 tag → `size:32`，新增 `internal/model/intent_contract5_test.go`（RED：tag 含 `size:16` 不含 `size:32` → GREEN）钉住。

**顺带清点出 E2E-F-217**：运行库 `emotion_echo_user.user_oauth` 仍在，而权威 DDL 已删该表 —— `05-drop-user-oauth.sql` 只挂在 postgres 的 `/docker-entrypoint-initdb.d/`（仅空库首次初始化执行）⇒ 老库上从不运行。代码侧 0 引用、功能无影响；已登记，本轮不改。

**跑法**：`TESTCONTAINERS_RYUK_DISABLED=true go test -tags integration ./integration_test/ -run <Test>`（本机 ryuk 清理容器起不来，需禁用以跑通 —— 非测试自身问题）。
**顺带加固**：chat-svc 的 `pgContainerDesc` 建表此前缺 `context/closed_at/deleted_at/metadata/intent/file_name`，使任何 conversation/message 写入类集成测试**根本跑不起来** —— 已补齐到与真实 DDL 列集合一致（§契约 5 的 schema 保真）。

### 9.6 测点 #7（§契约 6）—— `KAFKA_ENABLED=false` 路径不空跑（2026-10-12）

**做法**：以 `KAFKA_ENABLED=false` 重建 chat-svc（shell 导出该 env 覆盖）→ 日志 `[events] using DevEventPublisher (KAFKA_ENABLED=false, dev-only path)` → 触发业务事件 → 跑 smoke。**结果**：§6 **PASS**（`OK: 近 10 分钟内 15 行由 DevEventPublisher 直写`），§契约 1~5 / §7 同轮全绿。

**为何不是「不加载 dev overlay」**（对 plan 原措辞的更正）：`compose.dev.yml` **并不配置 Kafka**；决定路径的是 `KAFKA_ENABLED`（apps.yml 的 `${KAFKA_ENABLED:-true}`）。故"不加载 dev overlay"并非 §6 的判别条件，**`KAFKA_ENABLED=false` 才是**。本轮以 env 覆盖达成，未拆 dev overlay（避免破坏 §2.1 网关链路）。

**同轮修掉两个真问题**：

1. **compose：chat-svc `KAFKA_ENABLED` 硬编码 `"true"`** ⇒ §6 分支在标准栈上**根本不可触发**（smoke 恒走 SKIP）。改为 `${KAFKA_ENABLED:-true}`（**默认不变**，与 ai-svc 对齐）。
2. **smoke §6 判据恒 FAIL（假 FAIL，新登 E2E-F-218）**：原判据 `WHERE event_id LIKE 'smoke-%'`，而 `user_behavior_events.event_id` **全仓都是 UUID**（实测 `smoke-` 前缀 **0 行**）⇒ 一旦真跑必红，把"路径其实是好的"掩盖成 FAIL。改为**时间窗** `WHERE occurred_at >= NOW() - INTERVAL '10 minutes'`（`KAFKA_ENABLED=false` 时 chat-svc 绕开 Kafka、analytics consumer 收不到 ⇒ 近窗内唯一写入者就是 DevEventPublisher）。

> **方法论**：这是"**门禁自身缺陷**"的又一例（同 E2E-F-180 审计器 A3 / E2E-22 空 glob）——**从未真跑过的分支里的假判据**会长期潜伏。修法（先让分支可触发，再真跑）比"读代码相信它没问题"可靠。

**收尾**：验证后 chat-svc 已恢复 `KAFKA_ENABLED=true` + 重启 web-bff（容器 IP 变更铁律）。

### 9.7 测点 #19（F-195 / F-197 时序敏感 spec）—— 复跑与判据（2026-10-12）

**跑法**：`BASE_URL=http://localhost:3000 pnpm exec playwright test <spec> --repeat-each=4`（chromium + mobile）。

| 对象 | 结果 |
|---|---|
| `performance-baseline.spec.ts` + `digital-human-tts.spec.ts`（两 project） | F-195 的 `#12 段间 gap`、F-197 相关用例 **均未复现** |
| `#12 段间 gap` `--repeat-each=4` × 2 project | **8/8 PASS**；gap = `431 / 377 / 456 / 365 / 634 / 532 / 682 / 656 ms`（阈值 1000ms，余量充足） |

**判据（本轮结论，不是「重跑变绿就判 PASS」）**：
- 现有 `#12` spec 断言的是「**两段音频衔接**的 gap」，而 **F-195 的失败形态是「段在 `pumpQueue` 被静默丢弃（`play()` 零执行）」**，触发路径是 **`new.vue` → `[id].vue` 跨路由实例时，unmount 的 `stopTTS` 与接管实例的段入队交错**（账本 F-195 已记录该根因方向）。
- **该路径不在 `#12` 的覆盖范围内** ⇒ `#12` 绿**不能证伪** F-195。同理 **F-197**（mobile 视口 `fill` 后事件分发时序）也不在现有 mobile 用例的覆盖路径上。
- ⇒ **明确判据**：要复现/作门禁，需**新写覆盖该时序的 spec**（导航中途触发切段 / mobile `fill` 后事件分发断言），而非复用现有 `#12`。F-195 / F-197 **保留 🔴**（owner = 下一轮）。

### 9.8 测点 #20（F-189 全量基线）—— 逐条分类（2026-10-12）

**命令**：`BASE_URL=http://localhost:3000 pnpm exec playwright test`（28 spec × 2 project，`workers:1` 串行）。
**结果**：**66 failed / 252 passed / 2 skipped（37.6m）**。**本阶段（E2E-30）域内 0 条失败。**

| 桶 | 条数 | 根因 | 归属 |
|---|---|---|---|
| **陈旧探针直连 `:8894`** | **26** | `settings.spec.ts`（20）+ `health-discovery.spec.ts`（4）+ `e2e-16-mobile.spec.ts`（2）—— 该宿主端口 **E2E-29 D-47 ③ 已收掉**（防绕过网关伪造身份）⇒ `connect ECONNREFUSED ::1:8894`。**新登 E2E-F-219** | 他阶段回归钉（修法落**回归钉治理**；需换判据，非只改端口） |
| **环境缺失（Grafana / obs 栈）** | 2 | `performance-baseline #17/#18` 需 `:13000` Grafana，obs 栈未运行 | 环境（需 `--profile obs` 起栈） |
| **spec 夹具缺失** | 4 | `password-recovery`：`ReferenceError: TEST_USERNAME is not defined` | E2E-07 spec / 回归钉治理 |
| **负载 / 时序**（`toBeVisible` / `locator.waitFor` / `Test timeout`） | ~26 | 全量串跑下页面/接口变慢（F-189 已知签名） | 跨阶段（需复跑甄别；**非确定性**） |
| **跨阶段数据断言** | 4 | `dashboard-reports`：`overallScore 应 > N` / `UI 会话数应 = API conversationCount` | E2E-15 |
| 其它 | ~4 | `settings` 2 条 `toContain`、`password` 2 条 `toHaveURL` | 跨阶段（待细查） |

**方法学（如实记录）**：全量串跑受 **dev 限流（APISIX route100/110 各 60/60s）** 与负载影响 ⇒「负载/时序」桶**不可直接判为回归**（与 F-189 两轮 31/255、58/238 同型）。**本轮价值 = 把 66 条按根因归并**，其中 **39% 是单一陈旧探针（E2E-F-219）** —— 比逐条看 UI 超时有价值得多。

**副作用处理**：全量跑会**重写他阶段截图/基线 JSON**（`stages/e2e-28-*/baseline/*.json`、`screenshots/*.png`）⇒ 已 `git checkout` 还原，未入库。
