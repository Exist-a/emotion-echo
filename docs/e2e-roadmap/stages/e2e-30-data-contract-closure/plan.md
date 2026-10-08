---
stage: e2e-30
title: 数据契约收口（§2.4 六项数据契约 smoke 全绿 + helm template/lint 渲染回归 + 转挂账本收口）
type: transformation
status: pending
created: 2026-10-08
last-updated: 2026-10-08（建档；**本轮未使用 docker** ⇒ 运行时/smoke 验证留待开工；建档期只读事实表见 §0.1）
depends-on: []
blocks: []
gate: []            # 无开工前阻塞决策门；执行期 [M] 决策点见 §4
related-findings: [E2E-F-180, E2E-F-183, E2E-F-184, E2E-F-186, E2E-F-187, E2E-F-188, E2E-F-189, E2E-F-191, E2E-F-192, E2E-F-195, E2E-F-196, E2E-F-197, E2E-F-204, E2E-F-208, E2E-F-209]
---

# E2E-30 数据契约收口 — 详档（任务书）

> **类型**：transformation —— 本项目长期存在「**smoke 全绿 ≠ 数据对**」的失真（AGENTS.md §2.4 就是为它立的）：
> ① §2.4 六项数据契约的 smoke 脚本**已存在**，但**在 dev 已不可跑**（基址指向 D-47 已移除的 `:8894`，见 §0.1 F2）；
> ② helm 侧只有 **loki / prometheus 两个子 chart** 有渲染断言，**主 chart（22 个子 chart）零渲染回归**，历史上"空 glob 静默 0 条规则"就是这类静默失效（E2E-22）；
> ③ 本阶段是 roadmap 的**末阶段 + 收口兜底**：13 条账本以 `E2E-30/专项` 形态转挂在此（多为"范围外只记账"的跨阶段债）。
> **本阶段把"契约配了 / chart 能装"变成"契约逐项可断言、chart 渲染可机器回归、转挂债逐条有裁定"。**

> **依据**：roadmap §第八批 E2E-30 行（"§2.4 六项数据契约 smoke 全绿 + helm template/lint 渲染回归"）+ 本阶段名下 13 条转挂账本（F-180/183/184/186/187/188/189/191/192/195/196/197/204）+ 建档期新登 2 条（**E2E-F-208** 内部 gRPC 死代码 / **E2E-F-209** smoke 脚本 `:8894` 漂移）。
>
> **方法论**：[RUNBOOK.md](../../RUNBOOK.md)（§2.1 环境铁律 / §4 判定分级 / §4.1 证据有效性 / §6 TDD 纪律 / §7 收口 11 项 / §13 审计）+ [anti-patterns.md](../../anti-patterns.md)。
> **前置阶段**：E2E-29（横切：异常与安全）✅ done（2026-10-08，含遗留项跟进轮 PR #179）；无硬依赖。
> **阶段边界声明**：本阶段**不做**新业务功能；**不做**架构重写（死 gRPC 若选"补全走 gRPC"须单独立项，见 §4 M3）。

---

## 0. 计划期调研（**先摆事实，再排计划**）

> 本节全部为本轮**亲自回读的文件 / 亲自执行的只读命令**，非引用历史结论（AGENTS.md §〇 文档功课）。
> **执行环境约束（本轮）**：用户明示"目前 docker 占用，请不要使用 docker" ⇒ 本轮**未启动/未读取任何容器**，**未跑 smoke**（§0.1 F1 的"smoke 现状"以脚本源码回读 + 端口映射回读为据，非运行实测）。执行期若与本节不符，**以实测为准并回填本节**（AP-02）。

**已读实现文件**（≥3，AGENTS §〇 ①）：
`scripts/smoke_data_layer.py`（全文结构 + `:32` 基址 + §契约 1~8 分节）、`emotion-echo-web-bff/internal/handler/survey_handler.go`（`assessmentBase` 恒真分支 `:72/121/188/246/295`）、`emotion-echo-web-bff/internal/downstream/assessment_grpc.go`（5 方法 + `:94/:99` stub）、`emotion-echo-assessment-svc/internal/grpcserver/agent_server.go`（251 行 + `Unimplemented` 嵌入）、`emotion-echo-chat-svc/internal/grpcserver/chat_server.go`（`StreamMessages` → Unimplemented `:289-293`）、`emotion-echo-web-bff/main.go`（`:376-380` 5 条 dial + `:483` `WithAssessmentBase` + `:528` personality HTTP）、`emotion-echo-web-bff/internal/config/config.go`（`:190` assessment BaseURL 默认）、`deploy/docker-compose.apps.yml`（`:699` D-47 移除 8894 注释 / `:613` BFF tag）、`deploy/compose.dev.yml`（`:36-40` 8894 不再发布）。

**已读测试文件 / 守卫**（AGENTS §〇 ①）：
`emotion-echo-web-bff/internal/handler/survey_handler_test.go`、`emotion-echo-assessment-svc/internal/grpcserver/agent_server_test.go`（+`_nildeps`/`server_race`）、`scripts/test_migrations_contract.sh`、`scripts/test_helm_loki_render.sh` / `test_helm_prometheus_render.sh`（子 chart 渲染范式）、`deploy/db/test_migrations_contract.sh`。

**已查 ADR / 决策 / stage**（AGENTS §〇 ②）：
`docs/architecture/adr/adr-2026-09-survey-http-bypass.md`（**survey 走 HTTP 致 assessment gRPC 成死代码——ADR §后果已如实写明**）、`adr-2026-09-incremental-rpc-adoption.md`（ADR-18 §B 把 BFF→assessment 迁移列为 **P3 非阻塞 backlog**）、`adr-2026-09-decision-4-closure.md`（svc-to-svc = gRPC）、`docs/e2e-roadmap/decisions.md`（D-07 digest 已知值族 / D-30/D-35 发现机制）、`docs/e2e-roadmap/debt-paydown-plan.md`、`docs/e2e-roadmap/remediation.md:345`（"E2E-08 → E2E-30 按 roadmap 推进；R-02/R-03 余项可并入"）。

**外部依赖官方文档**（AGENTS §〇 ④，本轮按需）：
Helm 官方 `helm template` / `helm lint`（本地实测 `helm v3.18.4+gd80839c`，见 §0.1 F4）；PG `GRANT` / 视图权限语义（`deploy/db/04-create-views.sql` 回读）。

**smoke / 运行时探针（AGENTS §〇 ③）**：**本轮 N/A —— 用户禁用 docker**。替代证据 = 脚本源码回读（§0.1 F2）+ 端口映射回读（F3）。**执行期开工第一件事必须补跑**（§0.2 #2）。

### 0.0 假设清单（本文假设，与现状对比见 §0.1）

| # | 本文假设 | 依据 / 现状 |
|---|---------|------------|
| 1 | "§2.4 的 smoke 脚本还不存在（AGENTS 写'待建'）" | **不成立**：脚本 `scripts/smoke_data_layer.py` **2026-09-24 已存在**（21929 字节，含 §契约 1~8）⇒ AGENTS §2.4 文案过期（F1/F2） |
| 2 | "smoke 脚本现在能跑" | **不成立**：基址 `:8894` 已被 E2E-29 D-47 收掉 ⇒ dev 跑必连接失败（F2/F3） |
| 3 | "helm 主 chart 有渲染回归" | **不成立**：仅 loki/prometheus **子 chart** 有；主 chart（22 子 chart）零渲染断言（F4/F5） |
| 4 | "转挂账本多为已修、只剩形式" | **不成立**：13 条转挂中 **12 条仍 🔴 未解决**（唯一 ✅ 的是 F-179，已在本轮前一 PR 修） |
| 5 | "内部 gRPC 都接线了" | **不成立**：15 处"构建了没人用"（E2E-F-208，详见 findings） |

### 0.1 计划期实测事实表（**只读；未用 docker**）

| # | 事实 | 证据（`文件:行号` / 命令输出） |
|---|------|--------------------------|
| **F1** | ✅ **`scripts/smoke_data_layer.py` 已存在**（非"待建"），含 §契约 1~8（1~6 = AGENTS §2.4；7 = Nacos 实例列表；8 = 镜像新鲜度） | `ls -la` → 21929 字节，mtime 2026-09-24；`grep -n "契约" ` → `:171/193/212/230/277/290/351/401` 八节 |
| **F2** | 🔴 **脚本基址指向 D-47 已移除的端口 ⇒ dev 不可跑** | `smoke_data_layer.py:32` `BFF = "http://localhost:8894"`；`:372` Nacos `localhost:8848`（该端口**仍在** profiles=dev，`docker-compose.infra.yml:240`） |
| **F3** | ✅ **8894 宿主映射确已移除**（D-47 ③） | `deploy/docker-compose.apps.yml:699` 注释 + `compose.dev.yml:36-40`"dev 也不再发布 8894"；E2E-29 实测宿主直连 `code=000` |
| **F4** | ✅ **helm 本地可用；主 chart 渲染/lint 当前通过** | `helm version --short` → `v3.18.4+gd80839c`；`helm template ee charts/emotion-echo` → **rc=0，3039 行**；`helm lint charts/emotion-echo` → `1 chart(s) linted, 0 failed` |
| **F5** | 🟡 **主 chart 无渲染回归守卫**（仅子 chart 有） | `ls scripts/ \| grep helm` → 只有 `test_helm_loki_render.sh` / `test_helm_prometheus_render.sh`；`charts/emotion-echo/charts/` 有 **22 个子 chart**；CI 只有 `helm-loki-render` / `helm-prometheus-render` 两个 job（`doc-drift-check.yml:209/241`） |
| **F6** | 🔴 **§2.4 六契约当前无一被机器门禁消费** | 无任何 CI job 跑 `smoke_data_layer.py`（`grep -rn smoke_data_layer .github/` 零命中）；AGENTS §2.4 是"人工清单" |
| **F7** | 🟡 **本阶段名下 13 条转挂账本，12 条 🔴 未解决** | 逐行解析账本 owner 列含 `E2E-30` 的行：F-179(✅)/F-180/F-183/F-184/F-186/F-187/F-188/F-189/F-191/F-192/F-195/F-196/F-197/F-204 |
| **F8** | 🔴 **内部 gRPC「构建了没人用」15 处** | 见 [findings/2026-10-08-dead-grpc-inventory.md](../../findings/2026-10-08-dead-grpc-inventory.md)；最大一块 = BFF→assessment 整条链（C/D 类） |
| **F9** | ✅ **账本编号连续**（1..209 无缺号） | `python scripts/e2e_stage_audit.py --all` → **30 阶段 0 FAIL**（含 A8 连续性）；本轮新登 F-208/F-209 |
| **F10** | ✅ **迁移/视图守卫已存在**（可复用，不必新建） | `deploy/db/test_migrations_contract.sh` / `scripts/test_migrations_no_service_order.sh` / CI `view-consistency` job（`doc-drift-check.yml`） |
| **F11** | 🟡 **AGENTS §2.4 文案与现状漂移**（"待建"） | `AGENTS.md` §2.4 末行"smoke 脚本：`scripts/smoke_data_layer.py`（**待建**）" —— 实际已存在（F1） |
| **F12** | ✅ **环境基线未复核**（本轮禁用 docker） | 未执行 `docker ps`/`docker inspect`；被验镜像 tag 为 `web-bff:v0.1.37`（`apps.yml:613`，E2E-29 遗留项跟进轮产物） |

### 0.2 开工复核清单（第一天执行，防止任务书事实表过期）

| # | 复核项 | 通过标准 |
|---|--------|---------|
| 1 | **docker 可用性**（本轮被禁，开工需确认已释放） | `docker ps` 可返回；`deploy/.devmode-session` 无占用（双轨锁） |
| 2 | **补跑 smoke 看真现状**（本轮 N/A） | `python scripts/smoke_data_layer.py` 原样跑 → 记录真实 FAIL 清单（**禁止默认其"仍坏"**）；随后做 L1 修复 |
| 3 | **环境基线**（RUNBOOK §2.1） | 6 应用服务 + infra healthy；`db-migrate` ExitCode 0；Nacos `count` 复核；`--env-file .env.local --profile dev` |
| 4 | **helm 版本与渲染基线** | `helm version` ≥3.18；`helm template` 行数基线复核（本轮 3039 行，若因改动变化须说明） |
| 5 | **重跑 F4/F5** | 主 chart `template`/`lint` 仍 rc=0；子 chart 渲染守卫仍绿 |
| 6 | **重跑 F2/F3**（8894 现状） | 宿主 `curl localhost:8894` → `code=000`；确认脚本基址仍是 `:8894`（未被人先修） |
| 7 | **账本编号连续性** | 新登编号从 **E2E-F-210** 起（当前最大 F-209） |

---

## 1. 范围

### 做

- **§2.4 六项数据契约 smoke 可跑化 + 全绿**：修 `smoke_data_layer.py` 基址（`:8894` → 网关 `:19080`，含登录取 Bearer），并**把 smoke 接进 CI**（F6：当前零门禁消费）
- **helm 渲染回归**：主 chart `helm template` + `helm lint` 断言脚本（新增 `scripts/test_helm_main_render.sh`）+ 接 CI（覆盖 22 子 chart；含"空 glob 静默失效"负向对照）
- **schema / 视图 / 迁移治理复核**：视图定义一致性、迁移 checksum 三层核对在新变更上仍成立（复用既有守卫）
- **转挂账本收口**：13 条 `E2E-30/专项` 逐条**修 或 裁定**（不得静默留挂）；其中代码级可修者走 TDD
- **AGENTS §2.4 文案纠偏**（"待建" → 实际状态 + 基址口径）

### 不做（边界）

- **新业务功能 / 新服务**；**架构重写**（死 gRPC 若选"补全走 gRPC"= 单独立项，见 §4 M3）
- **prod 真实部署**；helm 只做 `template`/`lint` 渲染断言（**不 apply 到任何集群**）
- **渗透/性能专项**（属 E2E-29 / E2E-28 已收口域）
- **纯环境面账本**（F-192 宿主端口 RST / F-196 :3000 抢占）若裁定为"运维/环境"⇒ 本阶段只登记去向，不修（§4 M5）
- 读/打印 `deploy/.env.local` 内容、密钥进仓（AGENTS §四红线）

---

## 2. 测试点清单（20 个）

判定标记：`[A]` 自动可判 · `[V]` 视觉判定（需截图并被查看）· `[M]` 需人工/设计裁定（必须升级给用户）。详见 [RUNBOOK.md](../../RUNBOOK.md) §4。

### 组 A：数据契约 smoke 可跑 + AGENTS §2.4 六契约（7）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|---------|
| 1 | [A] | **smoke 可跑性修复（F-209 主测点，TDD 循环 L1）** | `smoke_data_layer.py` 基址改走网关 `:19080`（登录取 Bearer 后调用）；**断言脚本能跑完且失败项均为真实契约 FAIL**（不得再出现"连接被拒"类 FAIL）；同时补一条静态守卫钉住"基址非 `:8894`" |
| 2 | [A] | §契约 1 `user_behavior_events` 行数 = 业务事件数 | 触发 1 message + 1 conversation → 行数增量 ≥ 对应事件数（抓"dev 模式 outbox 永远 pending"类 bug） |
| 3 | [A] | §契约 2 `event_type` enum 细分 | `GROUP BY event_type` 出现 **≥2 种**（不能全 `'conversation'`） |
| 4 | [A] | §契约 3 `analytics_reader` 视图可读 | 以 `analytics_reader` 查**所有** `*_v` 视图 → 无 `permission denied` |
| 5 | [A] | §契约 4 `/api/v1/reports/daily` 数据真有 | `data.summary != ""` 且 `emotionDistribution.length > 0`（抓"Kafka off 时情绪分析无数据"） |
| 6 | [A] | §契约 5 schema 与写入端一致性 | 每个 `VARCHAR(32) NOT NULL` 枚举列，至少 1 个 integration test 断言写入值 ∈ enum 集合 |
| 7 | [A] | §契约 6 `KAFKA_ENABLED=false` 路径不空跑 | 按 infra+apps 起栈（不加载 dev overlay 的 Kafka 分支）→ 触发事件 → 30s 后 §契约 1+2 仍通过 |

### 组 B：helm 渲染回归（3）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|---------|
| 8 | [A] | **主 chart 渲染**（`helm template charts/emotion-echo`） | rc=0；**22 个子 chart 全部渲染**（断言渲染清单含每个子 chart 的关键资源名，非仅"总行数>0"）；基线行数变化须说明 |
| 9 | [A] | **主 chart lint**（`helm lint charts/emotion-echo`） | rc=0；`ERROR` 零；`INFO`（如 icon 建议）可接受但须记录 |
| 10 | [A] | **渲染回归守卫脚本 + CI**（新增 `scripts/test_helm_main_render.sh`，TDD 循环 L6） | 断言关键资源存在 + **负向对照**（把某处渲染条件改坏 ⇒ 脚本必须 RED）；接 CI job（与 `helm-loki-render` 同范式） |

### 组 C：schema / 视图 / 迁移治理（2）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|---------|
| 11 | [A] | 视图定义一致性守卫覆盖本轮变更 | `view-consistency` job 对 `deploy/db/04-create-views.sql` 与运行时视图定义比对仍绿（若有新增/改动视图则同步） |
| 12 | [A] | 迁移 checksum 三层核对仍成立 | `deploy/db/test_migrations_contract.sh` + `test_migrations_no_service_order.sh` 全绿；新增迁移的 checksum 与 HEAD 文件、`schema_migrations` 一致 |

### 组 D：转挂账本收口（6）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|---------|
| 13 | [A] | **E2E-F-188** digest 守卫不覆盖 compose（TDD 循环 L2） | `check_docker_digests.sh` 扩扫 `docker-compose.*.yml` 的 `image:` 行（含 registry 不可达时的 D-07 已知值豁免）；负向对照（写回未 pin 的 tag ⇒ RED） |
| 14 | [A] | **E2E-F-187** voice 链路同型两缺口（TDD 循环 L3） | ① `GET /api/v1/voice/audio/:filekey` 补 `HEAD`（网关 HEAD 现 404）；② `PutObject` 加 ctx 超时；断言含"停机上传 → 503 而非挂起" |
| 15 | [A] | **E2E-F-204** svc tracer 初始化无重试（TDD 循环 L4） | tracer init 加**有限重试/退避**（或编排层 depends_on OAP healthy）；断言"OAP 后起 ⇒ 服务仍能上报"（负向：OAP 永不可达 ⇒ 不无限阻塞启动） |
| 16 | [A]+[M] | **E2E-F-183** Kafka 发布 span 断链（TDD 循环 L5） | producer 侧 span 与 consumer 侧同 `traceId` + ref 互指；**需 ADR**（跨进程 trace 连接属架构级）；方案取舍 = §4 M4 |
| 17 | [A] | **E2E-F-180** 审计器 A3 不认子表编号 | `e2e_stage_audit.py` A3 能解析子表编号（或 plan 用解析器兼容格式）；断言 e2e-25 类 plan 不再报 WARN「未解析到测试点编号」 |
| 18 | [A]+[M] | **E2E-F-186** readiness 是否含 storage | 二选一裁定后落地：① storage 作 readiness **硬依赖**（MinIO 停 ⇒ `/health/ready` 翻转）② 只加**可观测**不翻转；方案取舍 = §4 M2 |

### 组 E：环境面 / 基线账本（2）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|---------|
| 19 | [A] | **E2E-F-195 / F-197** 时序敏感 spec 稳定性 | 播放层 `pumpQueue`/`play()` 与 mobile 输入时序两类 flaky 的**根因定位或明确判据**（连续 `-count`/多 project 复跑；不得只"重跑变绿"就判 PASS） |
| 20 | [A]+[M] | **E2E-F-189 / F-192 / F-196** 基线与环境面收口 | ① 全量 Playwright 基线失败清单**逐条分类**（跨阶段 → 各自 owner；本阶段 → 修）；② 宿主端口 RST 簇 / `:3000` 抢占是否转运维 —— 去向裁定 = §4 M5 |

汇总行（执行期填写）：PASS x / FAIL y / BLOCKED z / N/A w

---

## 3. TDD 循环划分（RED→GREEN→REFACTOR）

| 循环 | RED（先行失败测试） | GREEN（最小实现） | 影响面 |
|------|-------------------|------------------|--------|
| **L1** | 静态守卫断言 `smoke_data_layer.py` 基址 == `:19080`（当前 `:8894` ⇒ 必红） | 改基址 + 登录取 Bearer + 经网关调用 | 仅脚本；**注意 D-47 后宿主无法直连 BFF** |
| **L2** | `check_docker_digests.sh` 新增 compose 用例（写回未 pin tag ⇒ 必红） | 扩扫 compose `image:` 行 + D-07 豁免 | 守卫脚本 + CI |
| **L3** | voice HEAD 反代契约测试（当前网关 HEAD 404 ⇒ 必红）+ PutObject 超时用例 | Register 补 `r.HEAD` + PutObject ctx 超时 + `isStorageUnavailable` 映射 | BFF voice_handler / storage |
| **L4** | tracer init 重试测试（OAP 未起 ⇒ 当前永久不上报，必红） | 有限重试/退避（封顶，不无限阻塞启动） | shared/pkg/skywalking 或编排 depends_on |
| **L5** | Kafka 发布 span 连接断言（当前 producer span 独立根，必红） | 按 §4 M4 裁定落地 + **附 ADR** | chat-svc relay + 消费侧 ctx；**架构级须 ADR** |
| **L6** | `test_helm_main_render.sh` 断言关键资源（当前脚本不存在 ⇒ 必红） | 新增渲染断言 + 负向对照 + 接 CI | `scripts/` + CI |

> **边界提醒**：L5 触及"跨进程 trace 连接/架构"属 RUNBOOK §13.3 架构关键词 ⇒ **必须同批附 ADR + `docs/architecture/decisions.md` 变更**（防 AP-08）。L3 若新增路由须同步 `check_routes_alignment.sh` 契约（前端 ⊆ BFF ⊆ APISIX 白名单）。

---

## 4. 执行期 [M] 决策点

| # | 决策 | 背景 | 备选 |
|---|------|------|------|
| **M1** | §契约 4 的判据在"dev 无 AI 触发"时如何定 | `emotionDistribution` 依赖 AI 情绪分析落库；dev 冷启动可能合法为空 | ① 严格非空（先触发一次 AI 分析再断言）② 允许空但断言"AI 分析表有行" ③ 分环境判据 |
| **M2** | E2E-F-186 readiness 是否含 storage | MinIO 停时 `/health/ready` 不翻转 | ① 硬依赖（翻转）② 只可观测 ③ 折中（超时+告警） |
| **M3** | **E2E-F-208 死 gRPC 治理形态** | 15 处"构建了没人用"；ADR 已记录未清理 | ① 保留 + 注释 + "不得新增调用方"守卫 ② **删客户端死代码**（保留 HTTP 实现）③ 补全改走 gRPC（扩 proto JSONB，ADR-18 §B P3）④ assessment-svc 停起 :8886 ⑤ **单独立项 E2E-31**（若 ③） |
| **M4** | E2E-F-183 Kafka trace 断链是否本阶段修 | 修法涉及 relay 后台 ctx + 消费侧提取（架构级） | ① 本阶段修（附 ADR）② 记账转专项（与 M3 合并考虑） |
| **M5** | 环境面账本（F-192 / F-196）去向 | 属"宿主端口/长连接层"，非业务代码 | ① 本阶段修 ② 转运维 runbook ③ 封存（记录判据） |
| **M6** | §2.4 smoke 是否接 CI（阻塞 vs 告警） | 当前零门禁消费（F6） | ① required check（阻塞）② 仅告警 ③ 只在含 schema/chat/analytics/BFF 改动的 PR 触发 |

---

## 5. 收口门槛

- [ ] 20 个测试点全部有结论（PASS/FAIL/BLOCKED/N/A 四值，BLOCKED ≤ 1/3）
- [ ] 范围内缺陷走完 TDD（L1~L6 各自 RED→GREEN 记录）
- [ ] **回归钉**：新增/扩展 spec（数据契约 smoke 契约化 + helm 渲染断言），**跑过且绿**
- [ ] **凡"改变前端用户可见行为"的修复，必须 IAB 实测真实场景**（E2E-29 教训）
- [ ] §4 的 M2/M3/M4/M5 **已升级给用户并落定**（未落定不得判 done）
- [ ] L5 架构改动附 ADR + `docs/architecture/decisions.md`
- [ ] **账本对账**：13 条转挂（F-180/183/184/186/187/188/189/191/192/195/196/197/204）+ 本轮新登 2 条（F-208/209）**逐条翻状态或写明仍挂理由**；`E2E-F-184`（IAB 渲染帧失真）须给出"工具治理/上游报"的明确去向
- [ ] **AGENTS §2.4 文案纠偏**（"待建" → 实际；基址口径）已提交
- [ ] `python scripts/e2e_stage_audit.py --all` → 0 FAIL；§13.3 第二方核对通过
- [ ] §2.5 收口自检三连 + 残留分支/worktree 清理

---

## 6. 风险与缓解

| 风险 | 应对 |
|------|------|
| **smoke 脚本修基址后仍 FAIL（真实契约问题）** | 区分"连接类 FAIL"与"契约类 FAIL"（L1 断言前者为零）；契约类 FAIL 按 §契约 1~6 逐个 TDD 修，不掩盖 |
| **helm 渲染断言写成"行数 > 0"弱断言** | 断言**具体资源名存在**（22 子 chart 逐个），并写负向对照（改坏 ⇒ RED）——防 E2E-22「空 glob 静默 0 条」同型 |
| **转挂账本范围膨胀（12 条 🔴）** | 用 §4 决策点把"代码级可修"与"环境/架构级"分开；后者裁定去向后**只登记不修**（RUNBOOK §5） |
| **M3（死 gRPC）选"补全"会引发大改** | 默认建议 **②删客户端死代码 + ①守卫**（低风险）；③ 单独立项，不在本阶段做 |
| **dev 无 AI 触发导致 §契约 4 假 FAIL** | §4 M1 先定判据；断言前先触发一次 AI 分析 |
| 本阶段是末阶段，易被当"什么都塞进来" | §1 不做清单写死边界；范围外一律只记账 |
| 本轮未用 docker 导致事实表可能过期 | §0.2 开工复核 7 项**逐项跑**（尤其 #2 补跑 smoke、#6 重跑 F2/F3） |

---

## 7. 引用

- 执行协议：[RUNBOOK.md](../../RUNBOOK.md) · 反例：[anti-patterns.md](../../anti-patterns.md)
- 排期与状态：[roadmap.md](../../roadmap.md) · 账本：[discovered-unresolved.md](../../discovered-unresolved.md)（转挂 13 条 + 新登 **E2E-F-208 / E2E-F-209**）
- 调研产物：[findings/2026-10-08-dead-grpc-inventory.md](../../findings/2026-10-08-dead-grpc-inventory.md)（内部 gRPC 死代码清点）
- 决策：[decisions.md](../../decisions.md)（D-07 族 / D-30 / D-35）· [architecture/decisions.md](../../../architecture/decisions.md)
- ADR：[adr-2026-09-survey-http-bypass.md](../../../architecture/adr/adr-2026-09-survey-http-bypass.md) · [adr-2026-09-incremental-rpc-adoption.md](../../../architecture/adr/adr-2026-09-incremental-rpc-adoption.md)（ADR-18）
- 契约来源：[AGENTS.md](../../../../AGENTS.md) §2.4（六项数据契约清单）
- 执行记录（收口时写）：`stages/e2e-30-data-contract-closure/report.md`
- 回归钉：`emotion-echo-web/e2e/data-contract.spec.ts`（拟）+ `scripts/test_helm_main_render.sh`（拟）
