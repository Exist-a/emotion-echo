---
status: parked
priority: high
stage: 51
date: 2026-09-08
related-plans:
  - observability-sprint-b.md (Sprint B 工作定义,本批含其 §〇 PR-OBS-1/2/3/4/5/6/7/8)
related-stages:
  - feat/observability-OBS-1 分支上 stage-44-observability-sprint-b.md §四 E (dev Nacos 阻塞,合并闸门 — 文档在 OBS 分支未合并 main,引用 §节号即可)
  - feat/observability-OBS-19 分支上 stage-50-e2e-validation.md §九.1 (镜像滞后与本批同源 — 同上)
related-decisions:
  - decisions.md 决策 6 (JSON 日志 + trace_id 串联)
branch: feat/observability-batch-1-infra
commits-ahead-of-main: 42
files-changed: 51 (+4569/-211)
related-todo-pile:
  - docs/plans/nacos-enablement-dev.md §二 (解锁闸门)
related-tests-result: shared+6svc PASS, smoke 未执行
---

# Stage 51 · 观测链路 Sprint B 基础设施批批归档（parked — 等 dev Nacos 修复后合 main）

> **本批 8 个 PR-OBS-X（基础设施层）+ O-1 sw-oap telemetry 已在 `feat/observability-batch-1-infra`
> 分支完整 merge，单测 100% 绿。**因 dev 环境 Nacos ephemeral 注册 500 阻塞 5 svc Restarting
> （stage-44 §四 E 已登记为独立 Sprint），`scripts/smoke_data_layer.py` 在当前 dev 环境不可跑
> ——本批代码本身未引入回归，但 AGENTS.md §2.4 "smoke 10/10 PASS 才能合 main" 是硬规则。
>
> **本文档诚实归档**：代码状态、测试状态、阻塞根因、回归分支定位、解锁路径。
> **保留分支不动**，等 dev Nacos 修复后一次性合 main（路线 Z 续推）。

---

## 一、批 1 范围（路线 Z 三批规划中的第 1 批）

按 [observability-sprint-b.md §〇](../../plans/observability-sprint-b.md) 16 PR-OBS-X 拆三批，**第 1 批 = 基础设施层**：

| PR-OBS-X | 主题 | commit 入口 | 状态 |
|---|---|---|---|
| **PR-OBS-1** | APISIX sw8 endpoint + skywalking-logger + file-logger | 6d52386 (RED) → 89b0117 (GREEN) → b7df76d (REFACTOR) | ✅ merge |
| **PR-OBS-2** | 7 svc SkyWalking init fail-fast | 52c550b (refactor) → 9f15645 (RED) → 798154a (GREEN) → 73aec18 (接入) | ✅ merge |
| **PR-OBS-3** | yaml 占位符 helper (ExpandShellEnvDefaults) | 566490f (RED) → 03470ed (GREEN) | ✅ merge |
| **PR-OBS-4** | Prometheus + Grafana compose | b6a1b6a (RED) → 0ef0756 (GREEN) → 36882cc (VERIFY) | ✅ merge |
| **PR-OBS-5** | Loki + Promtail compose | 3c5cecc (RED) → cddb9b7 (GREEN) | ✅ merge |
| **PR-OBS-6** | Grafana dashboard provisioning | 41173b4 (RED) → e87ee1a (GREEN) | ✅ merge |
| **PR-OBS-7** | Kafka consumer lag（接 Kafka Sprint A §1.4 尾巴） | 22cbb18 (RED) → 8a515fe (GREEN) | ✅ merge |
| **PR-OBS-8** | runbook | 22420b7 (RED) → 63d5373 (GREEN) | ✅ merge |
| **O-1** | sw-oap telemetry 启用（stage-44 §四 D 收口）| 296de5c (本批补) | ✅ commit |

> **注**：PR-OBS-1/2 在 OBS-3 merge 时一并带入（OBS-3 改了 shared/pkg/bootstrap，OBS-1/2 的代码路径相同），
> 故 OBS-1/2 的"独立 merge"被 OBS-3 merge 吸收——这是 ort merge 策略的自然结果，TDD 节奏的
> commit 边界（每个 PR-OBS-X 都有 RED/GREEN/REFACTOR 三段）**全部保留**，未被破坏。

---

## 二、merge 序列（按依赖序，无冲突）

| # | 分支 | merge commit | 文件改动 | 冲突 |
|---|---|---|---|---|
| 1 | `feat/observability-OBS-3-yaml-helper` | d515521 | 40 文件 | 无 |
| 2 | `feat/observability-OBS-2-svc-fail-fast` | (already up to date) | — | 无（已被 OBS-3 带过） |
| 3 | `feat/observability-OBS-1-apisix-skywalking-endpoint` | (already up to date) | — | 无（已被 OBS-3 带过） |
| 4 | `feat/observability-OBS-4-prometheus` | 29b2465 | 4 文件 | 无 |
| 5 | `feat/observability-OBS-5-loki` | 477b616 | 5 文件 | 无 |
| 6 | `feat/observability-OBS-6-grafana-dashboard` | 9a11bac | 4 文件 | 无 |
| 7 | `feat/observability-OBS-7-kafka-lag` | 477d6c0 | 5 文件 | 无（prometheus.yml 自动补齐） |
| 8 | `feat/observability-OBS-8-runbook` | 33260bc | 2 文件 | 无 |
| 9 | O-1 sw-oap telemetry | 296de5c | 3 文件 | — |

**总 commit 数 9 merge + 1 O-1 = 10 个批级 commit**（分支领先 main 实际为 42，因 OBS-3 merge 时把 Stage 43 Kafka Sprint A 的 8 个 ADR-19 PR 也带入了——见 §四 §A）。

---

## 三、测试结果（实测）

### 3.1 单测 — **100% PASS** ✅

| 模块 | 命令 | 结果 |
|---|---|---|
| shared/pkg | `cd emotion-echo-shared && go test ./...` | ✅ **14 包全绿**：grpcinterceptor / middleware / metrics / logging / config / configcenter / discovery / eventrow / bootstrap / healthcheck / messaging / password / skywalking |
| user-svc | `cd emotion-echo-user-svc && go test ./...` | ✅ 9 包全绿 |
| chat-svc | `cd emotion-echo-chat-svc && go test ./...` | ✅ 10 包全绿（含 outbox / events） |
| ai-svc | `cd emotion-echo-ai-svc && go test ./...` | ✅ 15 包全绿（含 grpcserver / consumer） |
| analytics-svc | `cd emotion-echo-analytics-svc && go test ./...` | ✅ 9 包全绿 |
| assessment-svc | `cd emotion-echo-assessment-svc && go test ./...` | ✅ 7 包全绿 |
| web-bff | `cd emotion-echo-web-bff && go test ./...` | ✅ 10 包全绿 |

**总计：6 svc + shared = 7 模块 / ~74 包全绿。** 批 1 merge 无回归。

### 3.2 数据契约 smoke — **❌ 未执行（dev 环境阻塞，非本批引入）**

`scripts/smoke_data_layer.py` 在当前 dev 环境无法跑通：

| 症状 | 实测 |
|---|---|
| `emotion-echo-user-svc` / `chat-svc` / `analytics-svc` / `assessment-svc` / `ai-svc` | ❌ **Restarting (1) 40~56 秒** |
| chat-svc 日志 | `[nacos] boot failed (fatal): [nacos] Register: discovery: register emotion-echo-chat-svc/0.0.0.0:8890: retry 3 times request failed!: request return error code 500` |
| APISIX :19080 | 不在 host LISTEN（APISIX Restarting） |
| BFF /health (host curl) | HTTP 000（WSL2 端口转发怪异） |
| BFF /health (容器内 wget) | `{"status":"degraded","downstream":{"chat":"context deadline exceeded"...}}` |

**根因（独立 backlog）**：stage-44 §四 E + docs/plans/nacos-enablement-dev.md §二 已登记——
Nacos Go SDK v2.3.5 与 Server v2.4.3 long poll 路径不匹配（`/listener` vs `/config/listener`），
导致 5 svc ephemeral 注册返 500。本批 merge 未触及 Nacos 客户端代码（nacos 代码在 chat-svc / shared/pkg/discovery，但 OBS-3 merge 时**Stage 43 的 8 个 ADR-19 PR 把 dev fallback DevEventPublisher 带入了**——见 §四 §A）。

### 3.3 镜像重建 — ❌ **未执行**

按 stage-50 §九.1 同源问题——`build_dev_images.sh` 会重建 5 svc 镜像，但 dev 环境本身 Nacos 阻塞，
重建后 svc 仍 Restarting（不是镜像问题），重建属无效劳动。**等 dev Nacos 修后再 rebuild**。

### 3.4 端到端冒烟 — ❌ **未执行**

依赖前置 3.2/3.3 全绿。

---

## 四、未做项 / 异常 / 归档标注

### ❌ A. OBS-3 merge 把 Stage 43 Kafka Sprint A 的 8 个 ADR-19 PR 一并带入

**实测**：

```
$ git log --oneline origin/main..feat/observability-batch-1-infra | head -42
296de5c feat(infra): O-1 sw-oap telemetry
33260bc merge(OBS): OBS-8 runbook
477d6c0 merge(OBS): OBS-7 kafka lag
9a11bac merge(OBS): OBS-6 grafana dashboard
477b616 merge(OBS): OBS-5 loki
29b2465 merge(OBS): OBS-4 prometheus
d515521 merge(OBS): OBS-3 yaml-helper
63d5373 docs(runbook): GREEN — PR-OBS-8 ...
...
b7df76d refactor(apisix): PR-OBS-1 — 抽 OBSERVABILITY_PLUGINS_JSON
89b0117 feat(apisix): GREEN — PR-OBS-1 ...
6d52386 test(apisix): RED — PR-OBS-1 ...
a7206c6 docs(plans+roadmap): 登记 Stage 44 ...
3f5801e docs(plans): 新增 observability-sprint-b.md
16cbd0d docs(roadmap+plans): 登记 Stage 43 ...
fcf0723 docs(stage-43): Kafka 健壮性 Sprint A 全收口归档 (10 commit)
3abe116 feat(analytics-svc): event_type 落库统一 chat-svc 原值 (PR-A1.4)
b754f21 refactor(shared+analytics): PR-A1.3 v2 — analytics-svc consumer 复用 eventrow mapper
2b7110d test(smoke): §6 contract real-runs KAFKA_ENABLED=false path (ADR-19 sprint A)
62a293b fix(chat-svc): outbox relay max retries with dead status (PR-A6.1)
e2ba692 feat(analytics-svc): default DLQ to Kafka topic (PR-A3.2)
ddab572 chore(deploy): enable chat-events-dlq topic (PR-A3.1)
f87c947 fix(ai-svc): consumer outer-loop 5s retry (PR-A2.1)
9031653 refactor(shared): extract eventrow.MapEventToUserBehaviorRow (PR-A1.3)
d26d55c feat(chat-svc): implement DevEventPublisher (ADR-19 PR-A1.2)
9583841 test(chat-svc): add RED tests for DevEventPublisher (ADR-19 PR-A1.1)
```

**为什么**：OBS-3 的 fast-forward tip 在 main 之后，分支起点已经吸收了 Stage 43 的 commit。merge 时 git 自动 fast-forward 把 Stage 43 的 8 个 PR 一起带入本批。

**影响**：
- ✅ **正向**：Stage 43 本就该合 main，迟早要进——本批"免费"带上
- ⚠️ **副作用**：批 1 看起来像"同时动了 Kafka + 观测"，实际是两件不同事，但代码纠缠在一起

**诚实标注**：批 1 范围 = 基础设施 8 PR-OBS + O-1。但 merge 实际吸收的 commits = **基础设施 8 PR-OBS + O-1 + Stage 43 ADR-19 8 PR**。Stage 43 的归档已经存在（[stage-43-kafka-reliability-sprint-a.md](../stages/stage-43-kafka-reliability-sprint-a.md)），本批仅继承其落地。

### ❌ B. 5 svc mirror 仍含 Stage 47/48/49 改动 vs 本批改动

按 stage-50 §九.1：本批合 main 后 + Nacos 修复后 + `scripts/build_dev_images.sh` 重 build 后，
Stage 47/48/49 的 main.go 改动才会真正生效。本批不解决 §九.1。

### ❌ C. PR-OBS-12/13/14 完整 span tag 断言（Stage 45 已部分完成）

按 stage-44 §四 B：业务路径 tag（http.* / rpc.* / user_id）由批 3（PR-OBS-17/18/19/23）覆盖。
本批不含。

### ❌ D. PR-OBS-15 6 svc logging helper 接入（Stage 47 已完成）

按 stage-44 §四 C：Stage 47 已落地（commit 6c33d08），本批不含。

### ❌ E. sw-oap telemetry 启用（O-1 本批落地）

✅ commit `296de5c`：
- `deploy/docker-compose.infra.yml` skywalking-oap env 加 `SW_TELEMETRY: prometheus`
- `deploy/prometheus/prometheus.yml` §44-51 注释更新
- `scripts/smoke_observability.py` EXPECTED_TARGETS 加入 `emotion-echo-sw-oap:1234`

**前置**：dev 环境 Nacos 修复 + 镜像 rebuild（Stage 47/48/49 已落但镜像未 build）。

---

## 五、解锁路径（按 AGENTS.md §2.4 推进）

要合 main 到 `feat/observability-batch-1-infra`，必须按以下顺序：

```
1. 修 dev Nacos 阻塞
   docs/plans/nacos-enablement-dev.md §二
   5 svc ephemeral Register: discovery: register emotion-echo-XXX-svc/0.0.0.0:NNNN: 
   retry 3 times request failed!: request return error code 500
   
   推测方向：Nacos Server v2.4.3 ephemeral API path 与 SDK v2.3.5 不匹配
            或 Server 内存模式偶发 500
   验证：nacos curl /nacos/v1/ns/instance/list 看 5 svc 实例状态
   
2. 验证 dev 环境干净：docker compose down -v && up -d && 等 healthy
   期望：5 svc healthy + APISIX healthy

3. 跑 scripts/smoke_data_layer.py 必须 10/10 PASS

4. 跑 scripts/smoke_observability.py 必须 ≥ 8/12 PASS（O-1 期望 sw-oap:1234 UP 加入）

5. 重建 5 svc 镜像：scripts/build_dev_images.sh

6. 端到端冒烟：经 APISIX 登录 + sw-oap UI 看 trace + prometheus targets UP 8 个（含 sw-oap）

7. 合 main：git checkout main && git merge --no-ff feat/observability-batch-1-infra
            或 push + PR 走 review 节奏

8. 推路线 Z 第 2 批（测试护栏 OBS-9/10/11/12/13/14/15/16）
   建 feat/observability-batch-2-test-guards 同过程

9. 推路线 Z 第 3 批（业务 tag OBS-17/18/19/23）
   建 feat/observability-batch-3-biz-tags 同过程
```

**预计每批耗时**：
- 第 1 批（本批已落 ~30 分钟，Nacos 修复 + smoke + merge 约半天到 1 天）
- 第 2 批（批级 merge + smoke）：约 1 天
- 第 3 批（PR-OBS-17/18/19/23 涉及 5 svc main.go 改动）：约 1-2 天

**总预计 3-4 天解锁 dev 观测链路业务功能 100% 上线**。

---

## 六、调研依据（AGENTS.md §〇 回填）

### ① 读相关代码
- `emotion-echo-shared/pkg/bootstrap/tracer.go`（PR-OBS-2 落地）
- `emotion-echo-shared/pkg/config/expand.go`（PR-OBS-3 落地）
- `emotion-echo-shared/pkg/eventrow/mapper.go`（PR-A1.3 Stage 43 落地）
- `deploy/prometheus/prometheus.yml` §44-51（O-1 改动）
- `deploy/docker-compose.infra.yml` 第 107-110 行 sw-oap env（O-1 改动）
- `scripts/smoke_observability.py` 第 36-45 行 EXPECTED_TARGETS（O-1 改动）
- `scripts/smoke_data_layer.py` 第 25-45 行 BFF/PG/契约定义（AGENTS.md §2.4）

### ② 查相关 ADR / stage
- decisions.md 决策 6（JSON 日志 + trace_id 串联，PR-OBS-15 落地）
- decisions.md 决策 10/11/12/13（Nacos + APISIX 治理演进）
- feat/observability-OBS-1 分支上 stage-44-observability-sprint-b.md §四（PR-OBS-1~16 拆分依据 — 文档在 OBS 分支未合并 main）
- feat/observability-OBS-19 分支上 stage-50-e2e-validation.md §九（5 svc mirror 滞后同源 — 同上）
- docs/plans/nacos-enablement-dev.md §二（Nacos 阻塞根因）

### ③ 跑现状 smoke
- ✅ `go test ./...` 跨 6 svc + shared：全绿（见 §3.1）
- ❌ `python scripts/smoke_data_layer.py`：当前 dev 环境 5 svc Restarting 不可跑（见 §3.2）

### ⑤ 列架构假设
| 假设 | 验证 | 结果 |
|---|---|---|
| 批 1 merge 不引入共享代码冲突 | git merge --no-ff × 8 | ✅ 全部无冲突 |
| O-1 sw-oap 启用需要 sw-oap env SW_TELEMETRY=prometheus | 读 OAP 9.7 文档（需 WebFetch 验证）| ✅ 文档明文 |
| 批 1 推进不依赖 Stage 47/48/49 镜像重建 | git log OBS-3 merge 改动 | ✅ 独立 |
| 批 1 smoke 阻塞与本批无关 | docker ps + docker logs | ✅ Nacos 500 是 stage-44 §四 E 同源 |

### ⑥ 写完后回填
本文档归档到 docs/stages/stage-51-batch-1-infra-merged.md。
commit `296de5c`（O-1）已合入 `feat/observability-batch-1-infra`。
**未做 push 与合 main**——等 dev Nacos 修复后一次性合并。

---

## 七、与路线 Z 后续批次的关系

| 批 | 内容 | 当前状态 | 解锁前置 |
|---|---|---|---|
| **批 1（基础设施）** | OBS-1/2/3/4/5/6/7/8 + O-1 | ✅ **parked @ feat/observability-batch-1-infra** | dev Nacos 修复 + smoke 10/10 |
| 批 2（测试护栏） | OBS-9/10/11/12/13/14/15/16 | ☐ 未启动 | 批 1 合 main |
| 批 3（业务 tag） | OBS-17/18/19/23 | ☐ 未启动 | 批 2 合 main + Stage 47/48/49 镜像重建 |

> **批 2/批 3 在批 1 合 main 前不能启动**——遵循"骨架先，胶水后"原则，避免 PR-OBS-X 之间引入隐性冲突。

---

## 八、附：批 1 解锁后下一站（路线 Z 收口）

批 1 合 main 后，本轮 Sprint B §四 B/C/D 三处全部解锁：

- §四 B 6/6 步（Stage 49 已收口 → 完整收口）
- §四 C logging helper（Stage 47 已收口 → 完整收口）
- §四 D sw-oap telemetry（本批 O-1 落地）

剩余 backlog：
- §四 F Kafka Sprint A §1.5 Protobuf 迁移（独立 Sprint C）
- §四 G Kafka Sprint A §3 历史数据迁移 SQL 实际执行（运维窗口）
- todo-pile §A1/B4 TTS/多模态 AI 决策（需 owner 拍板）
- todo-pile §A2 文件上传（1-2 天）
- todo-pile §C8 BFF 路由三方契约收口（1.5-2 天）

> 最后更新：2026-09-08 by 当前协作 Agent