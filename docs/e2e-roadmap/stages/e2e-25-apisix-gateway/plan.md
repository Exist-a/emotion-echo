---
stage: e2e-25
title: 网关 APISIX（路由/JWT 插件/限流/CORS + 上游健康检查与重连 + seed↔admin 持久化治理）
type: transformation
status: in-progress
created: 2026-10-02
last-updated: 2026-10-02（开工：§0.2 复核 6/6 完成 + 新发现 N1/N2 回填；F-137/F-154/F-139/F-145 四条名下账本 + 3 项计划期新事实）
depends-on: []
blocks: []
gate: []            # 无开工前阻塞决策门；3 个执行期 [M] 决策点见 §4
related-findings: [E2E-F-137, E2E-F-139, E2E-F-145, E2E-F-154]
---

# E2E-25 网关 APISIX — 详档（任务书）

> **类型**：transformation —— 网关的**路由/插件/限流/CORS 配置全部存在且日常在跑**（`deploy/apisix/seed.sh` 723 行 + `config.yaml` 433 行 + 15 路由 + 6 nacos upstream + jwt-auth consumer），但它**当前不能证明自己是对的**：
> ① **上游健康完全外包给 Nacos 心跳**——账本 E2E-F-154：6 个 upstream JSON 有 `discovery_type: nacos` 但**没有 `checks` 段**，进程拒连但心跳还在的节点不会被 APISIX 剔除（D-30 实测摘除滞后 ≥75s）；账本 E2E-F-137：BFF 一重启就经历全站 503（E2E-17 只做了临时缓解，根因归本阶段）；
> ② **seed 重跑会静默覆盖 admin 手工改动**——账本 E2E-F-139：任何 APISIX admin 改动在下次 `compose up` 触发 seed 时被冲掉，无 diff 检测、无 CI 兜底（Stage 112 route 116 漂移事故即先例）；
> ③ **限流跨节点防放大从未实测**——账本 E2E-F-145：`limit-count policy=redis` 已持久化到路由链，但 dev 单节点下"多节点合计配额 = 单节点配额"零验证。
> **本阶段把"网关有配置"变成"上游故障自愈有实测窗口、seed↔admin 关系有机器守卫、JWT/限流/CORS 逐点可断言"。**

> **依据**：roadmap §第七批 E2E-25 行（"路由注册/JWT 插件/限流/CORS + 上游 nacos-discovery 健康检查/重连 + seed↔admin 持久化关系"）+ 本阶段名下 4 条留账（F-137 / F-139 / F-145 / F-154）。
>
> **方法论**：[RUNBOOK.md](../../RUNBOOK.md)（§4 判定分级 / §4.1 证据有效性 / §6 TDD 纪律 / §7 收口 11 项 / §13 审计）+ [anti-patterns.md](../../anti-patterns.md)。
> **前置阶段**：无依赖前置（roadmap 排期 ✅）。上一阶段 E2E-24 ✅ done（2026-10-02，20/20 PASS + 第二方核对两轮通过）。
> **开工前置**（roadmap 声明）：先治理 F-137/F-139 两个迁入根因——**二者即本阶段主体测试内容**（§2 组 A / 组 B），非独立前置任务。

---

## 0. 计划期调研（**先摆事实，再排计划**）

> 本节全部为本轮**亲自回读的文件 / 亲自执行的探针**，非引用历史结论（AGENTS §〇 文档功课）。执行期若发现与本节不符，**以实测为准并回填本节**（AP-02）。
>
> **已读实现文件**：`deploy/apisix/seed.sh`（全文 723 行）、`deploy/apisix/config.yaml`（全文 433 行）、`emotion-echo-web-bff/nacos_boot.go`（全文）、`emotion-echo-web-bff/main.go`（Nacos 启动重试段 :135-168）、`deploy/apisix/seed_test.js`（结构断言）、`scripts/check_routes_alignment.sh`（路由对齐守卫）、`deploy/docker-compose.apps.yml`（apisix-seed :732-757 / web :806-826 依赖接线）。
> **已查 ADR / 决策**：`docs/architecture/decisions.md` 决策 2（APISIX+etcd，2026-08-31 退役）/ 决策 7（jwt-auth，退役注记）/ 决策 8（限流熔断，退役注记）/ 决策 9（BFF 定位，DOC 更正段）/ 决策 11（**APISIX 复职**，2026-09-03 生效，3.18.0-debian + etcd v3.5）/ 决策 12（BFF 纯聚合层）；`docs/stages/stage-32-apisix-reintroduction.md`、`stage-109a-apisix-jwt-401-fix-2026-09-16.md`、`stage-112`（route 116 漂移）；`docs/e2e-roadmap/decisions.md` D-27/D-28（policy=redis 保留）/ D-29（liveness-readiness 分离）/ D-30（**seed 只写 upstream 定义、节点由 discovery 按 fetch_interval:30 拉取**；主动健康检查归 F-154/E2E-25）；账本 F-137/F-139/F-145/F-154/F-130。
> **smoke（本轮已跑）**：`node deploy/apisix/seed_test.js` → **61/61 PASS**；`bash scripts/check_routes_alignment.sh` → **PASS=2 FAIL=0**（含契约 3：BFF 8 个 auth action ⊆ APISIX 白名单）；运行时只读探针见 §0.2。
> **外部依赖官方文档**（功课④）：
> - APISIX Health Check tutorial：<https://apisix.apache.org/docs/apisix/tutorials/health-check/>——`checks.active`（http_path 默认 `/`、healthy.successes 默认 2、unhealthy.http_failures 默认 5、interval 默认 1s）；**"只在 upstream 被请求后才启动健康检查"**；**"passive 单独无法把节点标回健康，必须与 active 组合"**；**"无健康节点可选时会继续访问该 upstream"**。
> - APISIX nacos discovery：<https://apisix.apache.org/docs/apisix/discovery/nacos/>——连接信息在顶层 `discovery.nacos`，per-service 参数（namespace_id/group_name）在 `discovery_args`（与 seed.sh:221-251 现状一致）。
> - ⚠️ **官方文档没有明文承诺 active checks 对"discovery 动态节点"生效**（health-check 页与 FAQ 均无此说明）——这是 M1 决策点（§4）的存在原因，执行期第一轮运行时验证必须先落这一条，再决定 checks 方案是否成立。

### 0.1 计划期实测事实表

| # | 事实 | 证据（`文件:行号` / 探针输出） |
|---|------|--------------------------|
| **F-a** | **F-154 成立：6 个 nacos upstream 均无 `checks` 段** | `seed.sh:226-251` `put_nacos_upstream` 生成的 JSON 只有 name/type/discovery_type/service_name/discovery_args/timeout；`grep -c checks seed.sh` 命中全为注释。节点健康 100% 外包给 Nacos 心跳 + `config.yaml:202-212` `fetch_interval: 30`；D-30 实测停 user-svc 后 **75s** 路由仍 401 而非 503（摘除滞后基线） |
| **F-b** | **F-137 治理现状：BFF 侧已"部分实现"，APISIX 侧零治理** | dev 启动已有 backoff retry（`main.go:135-168`：prod fail-fast / dev 10 次、2s 起 cap 30s）+ `/health` 带 version + `dev-up.sh` 注册校验（E2E-17 PR#77 临时缓解）；**剩余根因 = APISIX 上游无主动健康检查、故障窗口不可观测**（与 F-a 一并治理，D-30 备注"不要拆成两次改动"） |
| **F-c** | **F-139 成立：seed 全量 PUT 覆盖 admin 手工改动，无 diff/无 CI 兜底** | `seed.sh` Step 2/2.5/4 每次**全量 PUT** 6 upstream + 1 consumer + 15 路由（无 GET-before-PUT、无漂移检测）；Stage 112 的 route 116 漂移事故是先例，现有清理只对**已知 id 116** 点删（`seed.sh:701-715`）；`check_routes_alignment.sh` 只对齐"BFF auth action ⊆ seed 白名单"（静态），**不对比运行时 admin 实际配置** |
| **F-d** | **计划期新发现：`PLUGINS_JSON` 是死变量且限流配置分叉** | `seed.sh:410-455` 定义 `PLUGINS_JSON`（内含 `"policy": "local"` 硬编码，:426），但全部路由 PUT 只引用 `CATCHALL_PLUGINS_JSON`（:546）与 `AUTH_WHITELIST_PLUGINS`（:640）——`PLUGINS_JSON` **零引用**；而实际生效链用的是 `$LIMIT_POLICY`（默认 redis，:116）。同一文件两处 limit-count 配置互斥，属"改错地方不生效"型漂移陷阱 |
| **F-e** | **计划期新发现：seed_test.js 弱断言实例** | 断言 `['set -euo pipefail', src.includes('set -euo pipefail')]` 靠 `seed.sh:24` **注释**里的字面量满足；实际脚本 `:29` 是 `set -eu`（ash 兼容，注释已说明理由）。**断言绿但断的与事实相反**——组 B 建 CI 兜底前必须先修此类断言，否则新守卫从第一天起就在说谎 |
| **F-f** | **计划期新发现：网关自身"假绿"（etcd 依赖不可观测）** | compose 中 apisix healthcheck = `timeout 3 bash -c '</dev/tcp/127.0.0.1/9080'`（`docker inspect` 实证，纯 TCP 连通）。本轮 etcd `Exited(255)` 状态下启动的 APISIX 实测：数据面 `/apisix-health` 与 `/api/v1/auth/login` 全 **404**（路由表空）、admin API 报 `has no healthy etcd endpoint available`，容器 healthcheck 仍 **healthy**。与 E2E-23 修掉的"降级启动报健康"同型，但发生在网关/编排层（候选账本 E2E-F-176，组 A 测试点 #5 治理） |
| **F-g** | **infra 栈同刻 Exited(255) 是复发模式** | 2026-10-02 本轮实测：prometheus/postgres/redis/nacos/etcd/minio 同刻 `Exited(255)`（与 E2E-22 计划期 2026-09-29 观测同类外部终止签名）。**开工复核（§0.2）必须先恢复环境**，且 seed 的 `condition: service_healthy` 链（`apps.yml:736-748`）意味着任一下游不健康 seed 就不跑——环境半死状态会连锁放大 |
| **F-h** | **jwt-auth 验签链路现状（复职后已落地）** | consumer 验签（`seed.sh:285-326`，key=`user`/secret=`BFF_JWT_SECRET`，占位符时显式 WARN）+ catch-all `store_in_ctx` + serverless-post-function 注入 `X-User-Id`（`:484-531`，无条件覆盖防伪造）+ `data_encryption.enable_encrypt_fields: false`（`config.yaml:393-401`，stage-109a 规避 APISIX 3.18 fetch_secrets 上游 bug）。白名单 8 条（route 110-115/117-119；116 已删不复用） |
| **F-i** | **F-145：policy=redis 已持久化但跨节点零验证** | `LIMIT_POLICY` 默认 redis（`seed.sh:116`）已写入实际路由链（CATCHALL/AUTH_WHITELIST）；dev 仅单 APISIX 节点；admin API 绑 `127.0.0.1:9180` + compose 端口层封堵（`config.yaml:328-349`）。**"多节点合计配额 = 单节点配额"从未实测** |
| **F-j** | **CORS 三处定义点，无单源守卫** | `seed.sh:104`（2 origin 兜底默认）/ `apps.yml:757-`（4 origin 注入，F-139 ①已修）/ `services.env.example`——F-139 症状已修，但"三处同源"无机器断言，回归靠人眼 |
| **F-k** | **文档漂移：决策 7 退役注记与现状不符** | `decisions.md:168` 仍写"当前全链路 JWT **不验签**……为审计 P0 S-1"，而 jwt-auth consumer 验签已随 Stage 109a 落地（F-h）。执行期顺手更正该注记（文档小修，随本阶段 PR） |

### 0.2 开工复核清单（第一天执行，防止任务书事实表过期）

> 环境基线（本轮计划期实测）：**infra 栈全停**（F-g）——16 个业务容器在跑但 etcd/nacos/redis/postgres 不在，apisix `healthy`（假绿，F-f）。开工第一步不是写代码，是把环境拉回 RUNBOOK 基线再复核。

| # | 复核项 | 通过标准 |
|---|--------|----------|
| 1 | 按 RUNBOOK §八 登记 `.devmode-session` → 错峰拉起全栈（含 infra）→ 19 容器 healthy | `docker ps` 全 healthy；apisix-seed Exited(0) |
| 2 | F-a 复核：`curl admin GET /upstreams/1..6` | 响应 JSON 无 `checks` 键（若已有则本阶段组 A 降级为回归验证并回填 §0.1） |
| 3 | F-c 复现：手工 PUT 一条测试路由 → 重跑 seed → 确认被静默覆盖/删除 | drift 行为与账本描述一致 |
| 4 | F-i 基线：单节点压 60 req/min → 429 | 记录单节点配额基线，供 #18 双节点对照 |
| 5 | F-f 复现（负向）：`docker stop etcd` → 数据面/admin/healthcheck 三方状态 | 404/报错/healthy 三态并存可复现（测完立即恢复） |
| 6 | 第二 APISIX 节点资源可行性（M3 前置）：docker stats 余量评估 | 内存余量 ≥ 512M（.wslconfig 8GB 上限，19 容器稳态 ≈6G 贴顶的前科） |

#### 0.2.1 开工复核实测回填（2026-10-02 执行，全部完成）

| # | 结果 | 证据 |
|---|------|------|
| 1 | ✅ 全栈恢复：30 容器，应用 6 svc + infra 全 healthy，`db-migrate`/`apisix-seed`/`minio-init`/`kafka-init` 均 Exited(0)，Nacos `count:6` | `docker ps` 输出 + `curl nacos service/list` |
| 2 | ✅ **F-a 成立**：6 个 upstream 运行时 JSON 均无 `checks` 键（逐一 grep 计数 = 0） | admin GET /upstreams/1..6 |
| 3 | ✅ **F-c 成立且比账本更细——漂移有两形态**：① 篡改已 seed 对象（route 100 裸 PUT 成仅 prometheus）→ seed 重跑**静默覆盖回**（jwt-auth 恢复）；② **额外对象**（手工建 route 299 `/api/v1/__drift_probe__`）→ seed 重跑**不清理**（seed 只点删已知 id 116），且数据面可命中（401=路由活着，jwt-auth 拦截） | 两形态均 admin GET + 数据面 curl 双证据；探针路由已删 |
| 4 | ✅ **F-i 基线 + 新发现 N2**：70 次连发 login → 60×200 + 10×**503**（非 429）。根因：`AUTH_WHITELIST_PLUGINS`（seed.sh:625）的 limit-count **漏配 `rejected_code`** ⇒ 走 APISIX 默认 503，与 catch-all（route 100，`rejected_code: 429`）不一致；redis 键 `plugin-limit-count:v1:/apisix/routes/110:...` 正常计数 ⇒ policy=redis 本身工作正常。**修复列入 #17 同轮**（白名单补 `rejected_code: 429`） | 状态码分布 + route 110 GET 配置（limit-count 无 rejected_code 字段）+ redis 键存在 |
| 5 | ✅ **F-f 两形态齐**：① 启动期无 etcd（建档时实测）= 数据面 404 + admin 报错 + healthcheck healthy；② 运行中停 etcd（本轮）= admin `has no healthy etcd endpoint available` + **数据面靠内存缓存继续 200** + healthcheck healthy。附加新发现 **N1**：`/apisix-health`（route 205）**无 upstream**，命中即 openresty 503——网关自健康路由本身是坏的，`#5` 修 healthcheck 时同轮修 | stop/start etcd 前后三探针输出；route 205 GET（仅 prometheus 插件，无 upstream_id） |
| 6 | ✅ 第二节点可行：全栈实际内存 ≈2.9GiB（27 容器），APISIX 单节点 388MiB ⇒ 临时第二节点 +400MiB 无压力 | `docker stats --no-stream` |

---

## 1. 范围

**做**：
- 组 A（上游健康与重连）：upstream `checks` 落地 + 动态节点兼容性实测 + 故障摘除/恢复时窗 + BFF 重启自愈窗口 + 网关自身 healthcheck 语义（F-137 / F-154 / F-f）
- 组 B（seed↔admin 持久化治理）：幂等性契约 + 运行时漂移检测工具 + CI 兜底 + 弱断言修复（F-139 / F-e / F-d）
- 组 C（jwt-auth 插件链路）：验签/白名单/X-User-Id 注入运行时逐点断言（F-h 基础上的验收，非新功能）
- 组 D（限流与 CORS）：单节点 429、双节点 policy=redis 不放大（F-145）、CORS 单源回归钉
- 文档收口：决策 7 退役注记更正（F-k）+ 新决策登记（D-35 起）

**不做（明确划出边界）**：
- **JWT 密钥轮换 / 过期刷新 / IDOR / 越权**——归 E2E-29（横切安全）；本阶段只验"插件按当前配置正确工作"
- **SSL/TLS 终结**——`config.yaml:408` `ssl.enable: false` 有 Stage 32 记录在案的 3.9 bug；prod TLS 属部署专项，本阶段不碰
- **etcd 集群化 / APISIX 高可用常驻部署**——本阶段只做 F-145 要求的最小双节点临时验证，不做常驻架构改动（无 ADR 不动架构，anti-pattern AP-06）
- **服务发现机制更换**（决策 2 的退役史不重开；Nacos 路线由决策 11 背书）
- **前端 429/503 的 UX**——范围外，发现只记账

---

## 2. 测试点总表（20 个）

> 判定分级：`[A]` 自动/脚本断言 · `[V]` 视觉/IAB 实测 · `[M]` 需裁定（执行期决策点，见 §4）。运行时类测试点全部要求**正负两向对照**（§4.1 证据有效性）：故障注入必须配上恢复后对照，避免"单次绿 = 巧合"。

### 组 A：上游健康检查与重连（F-137 / F-154 / F-f —— 一并治理，不拆两次改动）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|----------|
| 1 | [A] | **RED→GREEN**：seed_test.js 新增断言 `put_nacos_upstream` 生成的 upstream JSON 含 `checks.active`（探测路径、阈值、interval）；先红后绿落地 seed.sh | 断言先 FAIL；seed.sh 落地后 61→N PASS 全绿 |
| 2 | [M] | **nacos discovery 动态节点 × active checks 兼容性实测**：官方无明文承诺（§0 外部文档），先落运行时验证再定方案 | 实测有效 → checks 方案成立；无效 → 走 M1 fallback 决策并记录 |
| 3 | [A] | 故障摘除时窗：停 user-svc → APISIX 标记节点不健康的时间 ≤ active 探测收敛上限（对照 D-30 的 75s 滞后基线）；重启后节点自动回归 | 摘除/恢复两向都有时间戳证据 |
| 4 | [A] | BFF 重启自愈：重启 web-bff 全程不碰 apisix/seed → 网关 503 窗口有实测上限（discovery fetch_interval + 健康收敛） | 前后对照：503 窗口 ≤ 断言值；恢复后 login 200 |
| 5 | [A] | **网关自身 healthcheck 语义**（F-f）：etcd 停时容器转 unhealthy 或 `/apisix-health` 如实报错（与数据面 404 一致）；负向对照落行为层 | etcd 停 → 容器 unhealthy（或等效信号）；恢复 → healthy；补 compose/守卫 |
| 6 | [V] | IAB 故障注入：上游故障期间用户可见行为（503 或熔断 503）+ 恢复后全站可用 | 截图两向；无"白屏无提示"类静默失败 |

### 组 B：seed ↔ admin 持久化治理（F-139 / F-e / F-d）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|----------|
| 7 | [A] | **幂等性契约**：连续两次 seed → admin 全量配置（routes/upstreams/consumers）GET 快照 diff 为空 | diff 工具输出空集 |
| 8 | [A] | **漂移检测**：手工 PUT 测试路由 → 工具报告 drift 非空且定位到该路由；重跑 seed 后 drift 清零 | 两向对照齐全（对应 §0.2 #3） |
| 9 | [M] | **漂移处置策略**：seed 检测到漂移时 fail-closed（要求人工确认）vs 覆盖后出报告——用户拍板后落 decisions.md（D-36 起） | 决策记录 + 对应行为落地 |
| 10 | [A] | **集合回归钉**：seed 后运行时 `/api/v1/*` 路由集合 = seed 白名单集合（防 route 116 类漂移复发），接进 CI | CI 有该检查且红绿可演示 |
| 11 | [A] | **弱断言修复**（F-e）：`set -euo pipefail` 断言改为与 `set -eu` 事实一致；全量复核 seed_test.js 61 断言无"被注释满足"型 | 修复后仍 61/61（语义已修正） |
| 12 | [A] | `check_routes_alignment.sh` 保持全绿（含契约 3 白名单对齐） | 收口时 PASS 全绿 |

### 组 C：jwt-auth 插件链路（插件行为；密钥轮换/越权归 E2E-29）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|----------|
| 13 | [A] | 无效签名 token → 401；有效 token → 200 且 `X-User-Id` 注入值 = JWT sub（secret 从 `.env.local`，非占位符） | 两向 4 断言 |
| 14 | [A] | 客户端伪造 `X-User-Id` header 被无条件覆盖（负向对照：带伪造 header 的有效请求返回的是真实身份） | 伪造值不透传 |
| 15 | [A] | 白名单 8 条运行时免 jwt-auth 抽测（重点 117 verify-security-answer / 118 security-questions / 119 client-error 历史翻车点） | 8/8 无 token 可达业务端点 |
| 16 | [V] | 浏览器端到端：登录 → dashboard → 聊天 → 报表全 200（防 Stage 112 cookie/header 漂移复发） | IAB 截图 + 无 401 |

### 组 D：限流与 CORS（F-145 / F-j）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|----------|
| 17 | [A] | 单节点 limit-count：>60 req/min → 429；**顺带清除 `PLUGINS_JSON` 死变量**（F-d）并加断言锁死"不得存在未引用的插件配置块" | 429 实测 + 死变量清除有断言 |
| 18 | [M] | **F-145 双节点验证**：临时第二 APISIX 节点（同 etcd/redis）双节点合计配额 = 单节点配额（不放大）；资源不可行 → M3 降级决策 | 实测数据或决策记录 |
| 19 | [A] | CORS：preflight 六头齐全 + origin 白名单与 env 单源一致（seed.sh/compose/services.env.example 三处同源断言）；重跑 seed 后仍成立（F-139 症状回归钉） | 断言 + 重跑后复测 |
| 20 | [M] | api-breaker 熔断：mock upstream 连发 5xx → open → `break_response_code` 503 → `open_time` 30s 后半开恢复 | 触发/恢复两向时间戳证据 |

---

## 3. TDD 循环划分（RED→GREEN→REFACTOR）

> 运行时验收类测试点（#2/#3/#4/#6/#13-#20）不是 TDD 对象（它们是验收断言），但每个**守卫/工具/配置改动**必须先红后绿：

| 循环 | RED（先写失败的测试） | GREEN（最小实现） |
|------|----------------------|-------------------|
| C1 | seed_test.js 断言 upstream JSON 含 `checks.active` → FAIL | `seed.sh put_nacos_upstream` 补 checks 段（含 #11 弱断言修复同轮） |
| C2 | drift 检测工具的契约测试（样例快照 + 预期 diff 报告）→ FAIL | `scripts/check_apisix_drift.sh`（GET 快照对比，bash + curl，不引新依赖） |
| C3 | compose healthcheck 语义守卫（断言非 TCP-only / 含 etcd 依赖信号）→ FAIL | apisix healthcheck 改造 +（如需）config.yaml 探活端点 |
| C4 | seed_test.js 断言"无未引用插件配置块"（杀 PLUGINS_JSON）→ FAIL | 删除死变量 + 引用清理 |

---

## 4. 执行期 [M] 决策点

| # | 决策 | 背景 | 备选 |
|---|------|------|------|
| M1 | checks × nacos discovery 不生效时的 fallback | 官方文档无明文承诺（§0） | ① passive checks + api-breaker 收紧；② 调低 fetch_interval + 缩短 Nacos 心跳超时；③ 上游改静态 nodes + 独立探针（动架构，需 ADR） |
| M2 | seed 漂移处置策略（#9） | F-139 核心 | ① fail-closed（seed 检测漂移即退出，人工确认）；② 覆盖 + 自动 drift 报告进 CI artifact |
| M3 | F-145 双节点验证资源方案（#18） | .wslconfig 8GB 上限、19 容器 ≈6G 稳态（F-g 前科） | ① 临时第二节点压完即撤；② docker 单机双 compose project 隔离；③ 降级为"配置静态断言 + 账本留痕待多节点环境" |

---

## 5. 收口门槛

1. 20/20 测试点四值判定（PASS/FAIL/BLOCKED/N/A），**BLOCKED ≤ 1/3 不得判 done**；`N/A` 必须附"为什么不适用"证据（禁用 N/A 掩盖未做）
2. 名下 4 条账本（F-137/F-139/F-145/F-154）全部闭环或显式转挂；F-137/F-154 必须**同轮闭环**（D-30 备注）
3. `e2e_stage_audit.py --all` 0 FAIL
4. Playwright 回归钉落地（路由白名单集合 + 网关健康语义各至少 1 条）
5. decisions.md 登记 M1/M2/M3 裁决（D-35 起）+ 决策 7 注记更正（F-k）
6. RUNBOOK 涉及处更新（apisix-seed 语义 / healthcheck 排查段 / 命令速查 drift 工具）
7. STATUS.md 按模板撰写（已做/未做分列，禁美化）；收口后迁 `docs/stages/`

---

## 6. 风险与缓解

| 风险 | 缓解 |
|------|------|
| infra 同刻 Exited(255) 复发打断执行（F-g） | 开工复核 #1 先恢复基线；所有"停容器"类故障注入测完立即恢复；每轮收工三查（AGENTS §八.6） |
| checks 对动态节点不生效导致组 A 返工 | M1 前置：#2 排在组 A 第一顺位，先验证再定实现，不做无依据的配置堆砌 |
| 双节点验证击穿 .wslconfig 8GB | M3 备选降级路径已列；压测前 docker stats 基线留证 |
| seed 停摆连锁（condition: service_healthy 链） | 故障注入一律单变量；恢复顺序 = 依赖方向（infra → svc → seed） |
| 与 E2E-29 的边界模糊（jwt/限流天然相邻） | §1 边界表为准：本阶段验"当前配置正确性"，E2E-29 验"攻击面与轮换"；发现越权/IDOR 类问题只记账 |

---

## 7. 引用

- 账本：[discovered-unresolved.md](../../discovered-unresolved.md) F-137 / F-139 / F-145 / F-154（2026-10-02 计划期逐条复核成立）
- 决策：[decisions.md](../../decisions.md) D-27 / D-28 / D-29 / D-30 / D-34（编号续 D-35 起）
- 架构决策：[docs/architecture/decisions.md](../../../architecture/decisions.md) 决策 2 / 7 / 8 / 9 / 11 / 12
- 历史阶段：[stage-32-apisix-reintroduction.md](../../../stages/stage-32-apisix-reintroduction.md)、[stage-109a-apisix-jwt-401-fix](../../../stages/stage-109a-apisix-jwt-401-fix-2026-09-16.md)、Stage 112（route 116 漂移事故）
- 官方文档：[APISIX Health Check](https://apisix.apache.org/docs/apisix/tutorials/health-check/)、[APISIX nacos discovery](https://apisix.apache.org/docs/apisix/discovery/nacos/)、[Nacos×APISIX 博客](https://nacos.io/en/blog/apisix)
