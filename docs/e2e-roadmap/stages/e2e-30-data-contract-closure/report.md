---
stage: e2e-30
title: 数据契约收口（§2.4 六项数据契约 smoke 全绿 + helm template/lint 渲染回归 + 转挂账本收口）
status: partial
created: 2026-10-08
last-updated: 2026-10-09（**执行轮（进行中）**：L1/L6 + 测点 #11/#12/#13/#17 已落地并经 CI 35→36 项全绿合并；组 D 剩余 / 组 E / §8 口径 / [M] 待裁定）
---

# E2E-30 数据契约收口 — 执行记录（report）

> 判定依据：[RUNBOOK.md](../../RUNBOOK.md) §3 六步循环 / §4 判定分级 / §4.1 证据有效性 / §7 收口契约；反例：[anti-patterns.md](../../anti-patterns.md)。
> 任务书：[plan.md](plan.md)（20 测试点 / 6 TDD 循环 L1~L6 / 6 个执行期 [M]，M3 已转出至 [E2E-31](../e2e-31-internal-rpc-convergence/plan.md)）。
> **本文件为「进行中记录」**：阶段已开工但**未收口**，`status: partial`；§0 未完成清单是唯一真相源。

---

## 0. 未完成清单（唯一真相源 · 收口时必须逐条销账）

> 本阶段**未收口**：共 **6 项未完成**。

| 编号 | 未完成事项 | 责任人 / 去向 | 可核验判据 | 现状 |
|------|-----------|--------------|-----------|------|
| **T-1** | **§8 镜像新鲜度口径未定**（判据在非产物提交上恒红；详见账本 **E2E-F-215**） | **需用户裁定**：① 重建镜像+recreate ② §8 改口径（严格版留 `check_image_freshness.sh`）③ 只在 report 分类 | 三选一落定后，smoke 在**无服务代码改动**的提交上退出码为 0 | ⚠️ 未定；**本轮未擅自改断言** |
| **T-2** | **组 D 剩余 4 项**：#14（E2E-F-187 voice `HEAD` + `PutObject` 超时）/ #15（E2E-F-204 tracer 初始化有限重试）/ #16（E2E-F-183 Kafka 发布 span 断链，**需 ADR**）/ #18（E2E-F-186 readiness 是否含 storage） | E2E-30 执行者；**#16 与 #18 是 [M]（M4 / M2）⇒ 须先升级给用户** | 每条 TDD 循环有 RED→GREEN 记录；[M] 两条有用户裁定 | ⏳ 未开工（#14/#15 需重建 BFF/ai-svc 镜像验证，与 T-1 的裁定耦合） |
| **T-3** | **组 E 2 项**：#19（E2E-F-195/F-197 时序敏感 spec 稳定性）/ #20（E2E-F-189/F-192/F-196 全量基线与环境面收口，含 **[M] M5** 去向） | E2E-30 执行者；M5 须先升级 | 连续 `-count`/多 project 复跑有根因或明确判据；#20 逐条分类 + M5 落定 | ⏳ 未开工 |
| **T-4** | **测点 #2~#7 的形式判定未完成**：§契约 1~4 已实测 PASS（见 §2），但 #5（schema↔写入端一致性）需 **integration test 证据**、#6/#7 需 **`KAFKA_ENABLED=false` 栈形态**验证 | E2E-30 执行者 | #5 有 integration test 断言写入值 ∈ enum；#7 在不加载 dev overlay 的栈上跑通 §契约 1+2 | ⏳ 部分（#2/#3/#4 已 PASS，#5/#6/#7 待补） |
| **T-5** | **[M] M1 / M6 未裁定**：M1（§契约 4 在 dev 无 AI 触发时的判据）/ M6（§2.4 smoke 是否接 CI 及阻塞/告警形态） | **需用户裁定** | 两条各有裁定记录 + 对应落地 | ⚠️ 未定（M6 与 T-1 强相关：§8 口径不定则 smoke 退出码不可用） |
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
| 14 | **E2E-F-187** voice `HEAD` + `PutObject` 超时（L3） | [A] | BLOCKED | **未开工**（AP-03）；需重建 BFF 镜像验证，与 §0 T-1 的口径裁定耦合。见 §0 T-2 |
| 15 | **E2E-F-204** svc tracer 初始化有限重试（L4） | [A] | BLOCKED | **未开工**（AP-03）；需重建/编排验证。见 §0 T-2 |
| 16 | **E2E-F-183** Kafka 发布 span 断链（L5） | [A]+[M] | BLOCKED | **未开工 + [M] M4 未裁定**（跨进程 trace 连接属架构级，须 ADR）。见 §0 T-2 |
| 17 | **E2E-F-180** 审计器 A3 静默失效 | [A] | PASS | RED 3 项 → GREEN 6 PASS；A3 现对全部 **31 阶段**生效且 `audit --all` 0 FAIL。**根因更正**：账本写"子表编号"不准，真因是**章节标题**（e2e-25 用「测试点总表」） |
| 18 | **E2E-F-186** readiness 是否含 storage | [A]+[M] | BLOCKED | **未开工 + [M] M2 未裁定**。见 §0 T-2 |
| 19 | **E2E-F-195/F-197** 时序敏感 spec 稳定性 | [A] | BLOCKED | **未开工**（AP-03）。见 §0 T-3 |
| 20 | **E2E-F-189/F-192/F-196** 基线与环境面收口 | [A]+[M] | BLOCKED | **未开工 + [M] M5 未裁定**。见 §0 T-3 |

汇总：PASS 13 / FAIL 0 / BLOCKED 7 / N/A 0

> **关于 BLOCKED 7/20（35%）**：收口门槛要求「BLOCKED ≤ 1/3」，本阶段**未收口**，该比例是"开工 65% 时的中间态"，不是收口结论；收口前必须把组 D/组 E 与 #6/#7 逐条落定，使 BLOCKED 降到位（或在 report 写明仍挂理由）。
> **关于 §8**：8 项 **STALE** —— 既非连接类也非契约类，属**判据粗**（详见 **E2E-F-215** 与 §0 T-1）。**本轮未擅自改断言**。

**§8（镜像新鲜度）**：8 项 **STALE** —— 既非连接类也非契约类，属**判据粗**（详见 **E2E-F-215** 与 §0 T-1）。**本轮未擅自改断言**。

---

## 3. 本轮已完成（PR 记录）

| PR | 内容 |
|---|---|
| [#190](https://github.com/Exist-a/emotion-echo/pull/190) | **第一批**：L1 §2.4 smoke 可跑化（基址→网关 + Bearer + 登录前置）+ 守卫 `check_smoke_gateway.sh`；L6 helm 主 chart 渲染守卫 + CI job；#17 审计器计划解析器标题容错 + 守卫；AGENTS §2.4 文案纠偏（含双轨协议 §六 握手行）。CI 36/36 |
| [#191](https://github.com/Exist-a/emotion-echo/pull/191) | **第二批**：#13 digest 守卫扩扫 compose `image:` 行 + 钉 4 个浮动 `:latest`（E2E-F-188）。CI 36/36 |

新增 CI 守卫：`e2e-guards` **25/25**（smoke 网关契约）、**26/26**（审计器计划解析器）；`doc-drift-check` 新 job **helm-main-render**。

---

## 7. 收口自检（**未完成 —— 阶段未收口**）

| # | 检查项 | 状态 |
|---|--------|------|
| 1 | 20 个测试点全部有结论（四值） | [ ] **未完成**：13 项有结论（#1~#5、#8~#13、#17），#6/#7/#14~#16/#18~#20 待做 |
| 2 | 范围内缺陷走完 TDD（L1~L6） | [ ] 部分：L1 / L2 / L6 完成；**L3/L4/L5 未开工** |
| 3 | 回归钉跑过且绿 | [x] 本轮三条守卫（含负向对照）均已接 CI 并通过 |
| 4 | 改变用户可见行为的修复已 IAB 实测 | [ ] 本阶段至今无用户可见行为改动（故不适用）；#14/#15 落地后需评估 |
| 5 | §4 的 M2/M4/M5 已升级并落定 | [ ] **未落定**：M1/M2/M4/M5/M6 全部待裁定（见 §0 T-1/T-2/T-3/T-5） |
| 6 | L5 架构改动附 ADR | [ ] 未开工 |
| 7 | 账本对账 | [ ] 部分：F-180/F-188/F-209 已 ✅；其余 13 条待对账（见 §0 T-6） |
| 8 | AGENTS §2.4 文案纠偏 | [x] 已落地（PR #190） |
| 9 | `audit --all` 0 FAIL + §13.3 第二方核对 | [ ] audit 0 FAIL ✅；**第二方核对未做**（收口时） |
| 10 | §2.5 收口自检三连 + 残留分支清理 | [x] 本轮两次合并后均已跑（工作树干净 / main 与 origin 齐平 / 无残留分支与 worktree） |
