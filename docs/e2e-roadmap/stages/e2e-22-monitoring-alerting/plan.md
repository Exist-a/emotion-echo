---
stage: e2e-22
title: 监控告警（Prometheus 抓取闭环 + Grafana 面板实数据 + Alertmanager 通知渠道）
type: transformation
status: partial
created: 2026-09-29
depends-on: []
blocks: [E2E-28]
gate: []
related-findings: [E2E-F-11]
---

# E2E-22 监控告警 — 详档

> **类型**：transformation —— 采集与告警**配置齐备但关键环节空转**：Prometheus 有 5 个 job 却不抓 `llm-service`/`alertmanager` 自身、Alertmanager 只有一个**无任何集成段**的空 receiver、Grafana 面板只有 JSON 存在性断言（**从未验证过能出数据**）、k8s 侧 `rule_files` 指向空 glob ⇒ **告警规则 0 条且零报错**。本阶段把"配了"变成"实测 firing + 通知真的送达"。
> **依据**：roadmap §第六批 E2E-22 行（"Prometheus 抓取 / Grafana 面板 / Alertmanager 通知渠道"）+ 账本 **E2E-F-11**（唯一归属本阶段的留账）+ [stage-86-outbox-dead-alert](../../../stages/stage-86-outbox-dead-alert-2026-09-13.md)（告警链路唯一的完整落地档，其 §六 明确把"Alertmanager 外部通知渠道"列为 open 交给本阶段）。
> **方法论**：[RUNBOOK.md](../../RUNBOOK.md)（§4 判定分级 / §4.1 证据有效性 / §6 TDD 纪律 / §7 收口 11 项 / §13 审计）+ [anti-patterns.md](../../anti-patterns.md)。
> **前置阶段**：E2E-21 ✅ done（2026-09-29，PR #124~#126）。本阶段与其共享 `obs` profile 栈，但**不共享其改动面**（E2E-21 动 `loki/`、`promtail`，本阶段动 `prometheus/`、`alertmanager/`、`grafana/`）。

---

## 1. 阶段目标

把四件"看起来做了"的事变成**实测可查**，并补上两个当前**静默失效**的缺口：

| # | 目标 | 现状（有证据） | 目标态 |
|---|------|--------------|--------|
| 1 | **抓取面完整** | 5 个 job / 10 个 target（6 Go svc + APISIX + OAP + Prometheus 自监控 + kafka-exporter）；但 `emotion-llm-service` **暴露了 `/metrics` 却无 target**（`emotion-llm-service/main.py:215-217` vs `deploy/prometheus/prometheus.yml:26-32`），k8s 侧却有 annotation（`charts/emotion-echo/charts/llm-service/templates/deployment.yaml:18-19`）⇒ **双栈不一致，dev 采不到** | 7 业务目标全 UP + `EXPECTED_TARGETS` 与 `prometheus.yml` **自动比对**（防再漂移） |
| 2 | **告警真的响** | 4 条规则已写（`deploy/prometheus/rules/{kafka-lag,outbox-dead,kafka-dlq}.yml`），smoke 只断言其中 **2 条"已加载"**（`scripts/smoke_observability.py:337-348`）——**从未断言任何告警真的 firing 或送达** | 快路径（`up == 0`）+ 慢路径（业务计数器）双路验证 pending→firing→送达→resolved |
| 3 | **通知渠道** | dev `dev-ui` receiver **无任何集成段**（`deploy/alertmanager/alertmanager.yml:20-22`）⇒ 只能人眼看 UI；k8s 侧指向不可达占位域名（`charts/emotion-echo/charts/alertmanager/values.yaml:12`） | dev 接入**可机器断言的 webhook receiver**（闭环 E2E-F-11） |
| 4 | **面板出数据** | smoke 只断言 dashboard JSON 存在 + 面板标题（`:155-182`），**从不断言面板查询有数据**（对照 memory `frontend-visual-evidence-failure-modes`：断言全绿但界面是坏的） | 每个面板的 `expr` 经 Prometheus 查询**返回非空 series** + `[V]` 截图被查看 |

**并显式记录一个本阶段无法闭合的缺口**：Prometheus **自身死亡无法自告警**（计划期实测 §8.3.1 已发生：三个观测容器同刻 `Exited(255)`，Prometheus 死了 ⇒ 无人能发告警）。彻底解需 deadman's switch / blackbox exporter 这类**进程外**方案，单机 dev 无法自举 ⇒ 记账本 + 显式边界，不假装闭合。

---

## 2. 范围与边界

### 做

#### A. 观测栈自身可观测（黑瞎子缺口）

- **A1** `deploy/prometheus/prometheus.yml` 补 `alertmanager` job（当前 5 个 job 中**没有 alertmanager 自身**——收告警的东西自己无指标）。复用 `component` label 范式（`:42` / `:52` / `:59`）。
- **A2** 同文件补 `emotion-llm-service:8000` 到 `emotion-echo-services` job 的 targets。实测该容器 `Up (healthy)`、`8000/tcp` 已 expose（未映射宿主，**容器网络内可达**正是 Prometheus 所在网络所需）。
- **A3** 新增 `deploy/prometheus/rules/observability-self.yml`：
  - `PrometheusTargetDown`：`up == 0`，`for: 2m`，severity `critical` —— 覆盖 `alertmanager` / `kafka-exporter` / `skywalking-oap` 等观测面自身 target。
  - `AlertmanagerNotificationFailing`：`rate(alertmanager_notifications_failed_total[5m]) > 0`，`for: 5m`，severity `critical` —— **专治"通知发不出去且无人知道"**（E2E-F-11 的反向自检）。
  - 沿用现有规则文件的注释密度与 `groups/name/interval` 范式（照 `outbox-dead.yml`）。
- **A4** 在 plan/report 显式声明 **Prometheus 自身死亡不在本阶段能力内**（见 §2.3）。

#### B. 通知渠道打通（闭环 E2E-F-11）

- **B1** 新增 `deploy/obs-mock-receiver/`：`python:3-alpine` 跑 stdlib `http.server` 的 30 行接收器 —— `POST /alert` 落盘 + `GET /received` 返回最近 N 条告警 JSON。**只作 dev 断言靶子**，不引入第三方镜像（§6 风险 R2）。
- **B2** `deploy/alertmanager/alertmanager.yml` 增 `mock-webhook` receiver（`webhook_configs` + `send_resolved: true`），`route` 按 severity 分流：`critical → mock-webhook`、`warning → dev-ui`（**保留 `dev-ui`**，不破坏既有"UI 可见全部告警"语义，只把 critical 多送一份到可断言通道）。
- **B3** `deploy/docker-compose.infra.yml` 挂 `obs-mock-receiver`（`profiles: ["obs"]`，与同 profile 其它服务一致），receiver URL 用 compose 服务名 `http://obs-mock-receiver:8080/alert`。
- **B4** **`dev-ui` receiver 保持无集成段**（`alertmanager.yml:20-22` 的既有决策不动）—— Stage 86 §一 已定调"dev 只做聚合去重 + Web UI"，本阶段是**加一条并行的可断言通道**，不是推翻该决策。

#### C. 告警全链 firing 实证

- **C1** **快路径**（秒级~分钟级，无需造数）：停掉一个 scrape target ⇒ `PrometheusTargetDown` pending → firing（`scrape_interval: 15s` + `for: 2m`）。
- **C2** **慢路径**（业务计数器，沿用 Stage 86 §五 的毒消息法或 `OUTBOX_MAX_ATTEMPTS` 调小）⇒ `OutboxEventsDead` firing。**成本较高**（Stage 86 记录 100 次重试），测试点 #9 允许 `BLOCKED`+理由（RUNBOOK §4.2：环境允许时不得标 BLOCKED，故须先尝试 `OUTBOX_MAX_ATTEMPTS=1` 加速路径）。
- **C3** **resolved 链路**：target 恢复 ⇒ `send_resolved: true` ⇒ mock receiver 收到 `status: resolved`。
- **C4** **分组去重**：`group_by: ["alertname","component"]`（`alertmanager.yml:15`）生效验证 —— 同一 alertname 下多个 target 同时 DOWN 应只产生**一个告警组**而非 N 个。

#### D. Grafana 面板"有 JSON ≠ 有数据"

- **D1** `scripts/smoke_observability.py` 对 `emotion-echo-overview` 的 **4 个面板**（HTTP Request Rate / Error Rate 5xx% / p95 Latency / Goroutines）逐个取 `expr` → 打 Prometheus `/api/v1/query` → 断言 **series 非空**。当前只断言 JSON 存在（`:155-182`）。
- **D2** 同理覆盖 `kafka-consumer-lag.json` 的 3 个面板。
- **D3** `[V]` 截图：面板**渲染出真实曲线/数字**（RUNBOOK §4.1：截图必须被查看；`[V]` 点缺截图 ⇒ 审计 A7 报 FAIL）。

#### E. k8s 侧告警规则悬空（真缺陷，dev 无法验运行时）

- **E1** 现状：`charts/.../prometheus/templates/configmap.yaml:35-36` 声明 `rule_files: /etc/prometheus/rules/*.yml`，但该 configmap 的 `data` **只有 `prometheus.yml` 一个 key**（`:16-17`），且 `deployment.yaml:56-58` 只挂了 `config`（整个 `/etc/prometheus`）与 `data`（`/prometheus`）⇒ **k8s 侧规则文件不存在，且 Prometheus 对空 glob 不报错 ⇒ 0 条规则零告警静默失效**（与 E2E-F-147 promtail 权限静默失效同型）。
- **E2** 修法：新增 `templates/configmap-rules.yaml`（把 4 个规则文件内容内联）+ `deployment.yaml` 加 `rules` volume 挂 `/etc/prometheus/rules`。
- **E3** **验证边界**：本机无 k8s 集群 ⇒ 只做 `helm template` 渲染契约钉（新增 `scripts/test_helm_prometheus_render.sh`，照 `scripts/test_helm_loki_render.sh` 先例，含负向对照）。**运行时验证列为待办**（写进 report，不假装做过）。

#### F. 文档漂移修正（4 处，均为验收误导源）

| # | 位置 | 现状错误 | 修正 |
|---|------|---------|------|
| F1 | `docs/deployment/runbook/observability-compose.md:36` | "跑全 smoke ⇒ 11 项 PASS" | 实际脚本已 20 项（E2E-21 收口后） |
| F2 | 同文件 `:44` | "Alertmanager \|（PR-OBS-8 范围外，dev 未启用）" | compose 已有该服务且 prometheus.yml `:76-79` 已接线 |
| F3 | 同文件 `:64` | "sw-oap env 未设 `SW_TELEMETRY=prometheus`，**DOWN**" | `docker-compose.infra.yml:161-163` **已设** |
| F4 | `deploy/configuration.md:196` | `bash scripts/smoke_observability.sh` | 实际文件是 `.py` |

### 不做（边界）

| 边界项 | 理由 | 去向 |
|--------|------|------|
| **prod 真实通知渠道**（钉钉 / 企微 / 邮件 / Slack） | 需真实凭据 + 外部服务；Stage 86 §一 已定调"dev 单人开发场景无意义" | **待决策**（§6 决策 1）+ 账本 |
| **Prometheus 自身死亡的外部探测**（deadman's switch / blackbox exporter） | 需 Prometheus **进程之外**的发送方；单机 dev 无法自举，且 cloud 服务需账号 | 账本（§2.3） |
| **大批量业务告警规则**（错误率 / 延迟 / 队列深度全面化） | 阈值需**真实流量基线**，本轮拍脑袋定阈值必误报 | 归 **E2E-28**（性能与延迟基线，本阶段的 `blocks` 下游） |
| recording rules / 指标基数治理 | 无数据量依据，凭空优化 | 账本 |
| k8s 侧**运行时**告警验证 | 本机无集群 | 只做渲染契约（§2.3） |
| dev↔k8s 版本对齐（prometheus `2.51.2`↔`2.54.0`、grafana `10.4.2`↔`11.2.0`） | 升级需重验全部采集面，且与告警链路无因果关系 | **待决策**（§6 决策 2）+ 账本 |
| `inhibit_rules` 补齐到 dev | k8s 已有（`configmap.yaml:26-31`），dev 单人无值班价值，补了也无人消费 | 账本 |
| `OutboxEventsDead` 的 Grafana 面板 | Stage 86 §六 已裁定"暂不做"（dead 计数器 99% 时间恒 0，面板价值低） | 保持裁定，不重开 |
| 端侧化 / 浏览器端上报 | AGENTS §八 Lane O 独占域 | 归 Lane O |
| 改 `deploy/.env.local` / 任何密钥 | AGENTS §四 红线 | 禁止 |

### 2.3 三个"本阶段无法闭合"的缺口（显式记录，防止日后被当成遗漏）

1. **Prometheus 自身死亡 ⇒ 零告警**。本阶段 A3 只能让 Prometheus 死**之后仍活着的其他组件**被发现；Prometheus 进程死 ⇒ 规则不评估 ⇒ 无人发告警。真实解需进程外发送方。**本阶段不假装解决**。
2. **k8s 侧运行时未验**。E2E-21 对 promtail 权限已留同款声明（`report.md` 中"只做 helm template 渲染回归，dev 无法真实验证"）⇒ 本阶段沿用，**不得**在 report 里写"k8s 告警已验证"。
3. **告警阈值的合理性未验**。本阶段只验"规则能加载、能 firing、能送达"，**不验"阈值设得对不对"** —— 后者需要 E2E-28 的基线数据。

---

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| 依赖阶段完成 | **无**（E2E-21 已 done，是并列的观测面阶段，非前置） |
| 决策门（RUNBOOK §9） | ✅ 无阻塞项（D-04 i18n 不阻塞任何阶段）。§6 的两个待决策项**不阻塞**——均可按 plan 给出的推荐方案继续 |
| 环境 | dev 全栈 + `obs` profile（`docker-compose.infra.yml:369-488`，六个观测服务**全部** `profiles: ["obs"]`） |
| **必须先恢复的容器** | ⚠️ 计划期实测：`emotion-echo-prometheus` / `alertmanager` / `kafka-exporter` / `sw-oap` / `sw-ui` / `minio` / `kafka` 均 **`Exited (255)` @ 2026-09-29T03:12:40Z**（同一时刻、`OOMKilled=false`、无 error ⇒ 一次外部终止事件，非容器自身崩溃）。**开工第一步必须恢复并确认全 UP**，否则测试点 #1~#4 全部无对象（详见 §8.3.1） |
| 数据准备 | 无需造数；C1 快路径只需停/启一个 target |

环境启动命令（**必须带 `--env-file .env.local`**（AGENTS §四）与 `--profile dev`、`--profile obs`）：

```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local --profile dev --profile obs up -d
```

> 本阶段需要 `--profile obs`（RUNBOOK §12 速查表未含此 profile，执行时以本节为准；E2E-21 同样踩过这条）。
> 启动后按 RUNBOOK §2.1 核对 Nacos 注册（`count:6`），否则会把 503 误判成被测功能缺陷。

### 3.1 开工前必须先跑的可行性探针（RED 前置）

```bash
# 探针 1：观测栈全 UP（期望 prometheus/alertmanager/kafka-exporter/sw-oap 全部 Up）
docker ps --filter "name=emotion-echo" --format "{{.Names}}\t{{.Status}}" | grep -E "prometheus|alertmanager|kafka-exporter|sw-oap"
# 探针 2：prometheus 端点（期望 9090/-/healthy 200）
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:9090/-/healthy
# 探针 3：mock receiver 镜像可拉（依赖 §6 风险 R2）
docker pull python:3-alpine
```

**判据**：探针 1 全 Up + 探针 2 = 200 + 探针 3 成功 ⇒ 可开工。任一失败 ⇒ 按 RUNBOOK §8 升级，**不得**带着半死栈写测试点结论。

---

## 4. 测试点清单

判定标记：`[A]` 自动可判 · `[V]` 视觉判定（截图并**被查看**）· `[M]` 需人工/设计裁定（必须升级给用户）。详见 [RUNBOOK.md](../../RUNBOOK.md) §4；证据有效性见 §4.1（**"配置已新增"不算证据**）。

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 1 | 7 业务目标（6 Go svc + **llm-service**）全部 `health==up` | [A] | `GET :9090/api/v1/targets?state=active`，断言 UP 集合 **⊇ 含 llm-service**；给出实际 UP/DOWN 计数 | | ⬜ |
| 2 | **alertmanager 自身**被 prometheus 抓取（新增 job 生效） | [A] | targets 中出现 alertmanager 且 `up==1`；`query{job="alertmanager"}` 返回非空 series | | ⬜ |
| 3 | Grafana 4 个 overview 面板的 `expr` **返回非空 series** | [A] | 逐面板取 expr → Prometheus `/api/v1/query` → 断言 `result` 非空（**当前 smoke 从不做此项**）。⚠️ 面板 expr 可能是从未验证过的想象指标 ⇒ 预期此点会红 | | ⬜ |
| 4 | Grafana 面板**视觉渲染出真实数据**（非 "No data"） | [V] | 浏览器打开 `:13000` dashboard → 截图且**必须被查看** | `screenshots/01-grafana-overview.png` | ⬜ |
| 5 | `kafka-consumer-lag` 3 面板有真实 lag 数据 | [A] | 3 个 expr 查询返回非空 series（lag 恒有数，DLQ 面板例外见下） | | ⬜ |
| 6 | `PrometheusTargetDown` 规则已加载 | [A] | `/api/v1/rules` 中出现该 alertname（对齐现有两条规则的断言写法 `:337-348`） | | ⬜ |
| 7 | **告警真的 firing**（快路径）：停 1 个 target ⇒ pending → firing | [A] | 停 kafka-exporter ⇒ 轮询 `/api/v1/alerts` 至 `state=="firing"`，给出**实际等待秒数** | | ⬜ |
| 8 | firing 告警**送达 mock receiver** | [A] | `GET :<mock>/received` 中出现含该 `alertname` 的记录，断言 **body 含 `startsAt` + `annotations`**（不只是 200） | | ⬜ |
| 9 | 业务指标告警端到端：`OutboxEventsDead` firing | [A] | 优先 `OUTBOX_MAX_ATTEMPTS=1` 加速造死信（重建 chat-svc）；不可行则按 Stage 86 §五 毒消息法。**先尝试再标 BLOCKED**（RUNBOOK §4.2） | | ⬜ |
| 10 | **resolved 链路**：target 恢复 ⇒ 收到 `status: resolved` 通知 | [A] | 恢复 kafka-exporter ⇒ mock receiver 出现同 alertname 的 resolved 记录 | | ⬜ |
| 11 | 告警**分组去重**生效（多 target 同时 DOWN 只 1 个组） | [A] | 同时停 2 个 target ⇒ `/api/v2/alerts` 中该 alertname 的分组数 == 1（`group_by: [alertname, component]`） | | ⬜ |
| 12 | **负向对照**：断链后 smoke **变红** | [A] | 停 alertmanager 或改坏规则 expr ⇒ 重跑 smoke 必须出现 FAIL。**证明收紧后的断言有约束力**（防 AP-11 假绿，E2E-21 同款做法） | | ⬜ |
| 13 | `EXPECTED_TARGETS` 与 `prometheus.yml` **自动一致**（防再漂移） | [A] | smoke 解析 `prometheus.yml` 的 `static_configs` 与常量清单比对，不一致即红（当前两处是**手抄副本**，`prometheus.yml:26-32` 一改而 `smoke_observability.py:38-47` 不会跟着改） | | ⬜ |
| 14 | k8s 侧 rules ConfigMap **渲染**正确 | [A] | `helm template` 断言：rules configmap 存在且 4 条 alert 规则在内 + deployment 有 `/etc/prometheus/rules` volume 挂载 | | ⬜ |
| 15 | k8s 渲染**负向对照** | [A] | 移除挂载 ⇒ 测试脚本必须红（照 `test_helm_loki_render.sh` 的 6/6 + 负向范式） | | ⬜ |
| 16 | 文档漂移 4 处修正 + 静态契约钉 | [A] | 断言 `observability-compose.md` 不含"dev 未启用"/"未设 SW_TELEMETRY"/"11 项"；`configuration.md` 引用 `.py` | | ⬜ |
| 17 | `smoke_observability.py` 收紧后**全绿** | [A] | 实跑输出 PASS 计数 | | ⬜ |
| 18 | **阶段账本对账** | [A] | `discovered-unresolved.md` 中 owner=E2E-22 的条目（E2E-F-11）已翻状态；新发现按 §5 追加 | | ⬜ |

**BLOCKED 预算**：18 点中 `BLOCKED > 6`（1/3）即不得判 done。测试点 #9（慢路径）与 #14/#15（k8s 渲染）是最可能 BLOCKED 的三个，**但 #14/#15 预期可做**（本机有 helm + 已有同类脚本先例）。

---

## 5. 验收标准（DoD）

- [ ] 18 个测试点全部有结论（PASS / FAIL / BLOCKED + **理由**），`FAIL` 已按 §5 分类并修复
- [ ] 修复项走完 TDD（Red → Green → Refactor），且测试点 #12 负向对照通过
- [ ] **告警"配置齐备"升级为"实测 firing + 送达 + resolved"**（#7/#8/#10 三点全过是本阶段区别于既往阶段的核心证据）
- [ ] 回归钉落地：smoke 收紧 + helm 渲染脚本（含负向）+ 文档静态契约钉
- [ ] `python scripts/e2e_stage_audit.py --all` 0 FAIL（计划期实测基线即为 0 FAIL，收口后不得回退）
- [ ] 账本 **E2E-F-11** 状态翻转；本阶段新发现按 RUNBOOK §5 追加（编号连续）
- [ ] `roadmap.md` 状态表 + 顶部「当前激活阶段」同步（激活指针移到 E2E-23 或下一候选）
- [ ] §2.5 收口自检三连通过
- [ ] §7 收口契约 11 项全过（含 §13.3 **第二方核对** —— **执行者不得自行宣布 done**）

### 5.1 回归钉形态说明（为何无 Playwright spec）

RUNBOOK §7 收口契约 #3 的示例路径是 `emotion-echo-web/e2e/<name>.spec.ts`，但该路径**只适用于有用户可见 UI 的阶段**。E2E-21（同样是非前端阶段）已确立先例：回归钉 = **Go 单测 + 静态契约测试 + smoke 断言**，未写 Playwright spec。本阶段沿用该形态（Grafana 面板是 `[V]` 证据而非 E2E 断言面）。**若审查者认为需要 Playwright**，按 §8 升级，不擅自发明第四种形态。

---

## 6. 待决策（不阻塞开工，按推荐方案先走）

| # | 决策 | 推荐 | 备选 | 现状影响 |
|---|------|------|------|--------|
| 1 | **通知渠道最终形态** | dev 走 **mock webhook**（本方案）：可机器断言、成本为零、不需凭据 | 真实钉钉/企微/邮件：需用户提供 webhook 地址或 SMTP 凭据 | 若选真实渠道，B1~B3 作废，改为"仅验证配置渲染正确 + 送达由用户侧确认"（测试点 #8 降级为 `[M]`） |
| 2 | **dev↔k8s 版本对齐** | **本阶段不对齐**，记账本 | 升 dev 到 prometheus 2.54.0 / grafana 11.2.0 | 若要对齐，本阶段所有断言需在升级后重跑一遍，成本显著上升 |

> 按 RUNBOOK §8"升级时不要问要不要继续，而是给出结论 + 建议 + 需要用户做的具体决定"：两项**均不阻塞**，按推荐方案执行，用户若否决再调整。

---

## 7. 已知风险

| 风险 | 应对 |
|------|------|
| 观测栈恢复后仍有 target DOWN（`sw-oap` 镜像拉取问题，见 stage-86 §五 运维注：本机镜像源需 `docker.m.daocloud.io` 前缀） | §3.1 探针 1 先行；DOWN 则记录并判断是否属本阶段范围（`skywalking-oap` job 的 DOWN 属观测面，**范围内**） |
| mock receiver 镜像 `python:3-alpine` 拉不到（国内源） | 备选：改用已在栈内的镜像（如 `emotion-echo/llm-service` 的 python 基础镜像复用）；再不行则**该点 BLOCKED+理由**，不留空 receiver（`inbound` 空 receiver 会被审计判为假接线） |
| 停 kafka-exporter 影响后续测试（chat/analytics 的 lag 面板） | 测试点 #7/#11 的停启动作集中在收尾前，且**必须恢复并复测 #5** |
| 重建 chat-svc 造死信（#9）耗时且可能带出新问题（如 E2E-F-107 Nacos 不重试） | #9 允许 BLOCKED+理由；造数优先用 API/SQL 而非 UI |
| `OUTBOX_MAX_ATTEMPTS=1` 改变全局行为可能污染其它测试 | 造完死信后**必须**恢复默认值并重启 chat-svc，且在 report 记明 |
| Grafana 面板 expr 引用**从未存在过的指标名**（测试点 #3 预期红） | 这正是本阶段要抓的缺陷；修复方向是"改面板 expr 对齐实际暴露的指标"，**不是**"删断言" |
| helm 渲染脚本在 CI 缺 helm 二进制 | 照 `scripts/test_helm_loki_render.sh` 的接入方式（该脚本已接 `helm-loki-render` job），新增脚本同样接 job |
| 六个观测服务**无 healthcheck**（`docker-compose.infra.yml:369-488` 全无 `healthcheck:` 段） | 属编排健壮性议题，本阶段用探针 1 人工确认即可，**不扩范围**；记账本 |
| 恢复 7 个 Exited 容器时误触发 dev mode 双轨冲突 | `.devmode-session` 计划期实测不存在（§8.3.4）；恢复后**立即**写锁并在收口删除（AGENTS §八） |

---

## 8. 产出物

- 执行记录：`docs/e2e-roadmap/stages/e2e-22-monitoring-alerting/report.md`（按 RUNBOOK §10 模板）
- 截图：`screenshots/01-grafana-overview.png`（测试点 #4）、`02-grafana-consumer-lag.png`（#5）、`03-alertmanager-firing.png`（#7 firing 态）
- 回归钉：`scripts/smoke_observability.py`（新增/收紧断言 + 解析 `prometheus.yml` 防漂移）、`scripts/test_helm_prometheus_render.sh`（新，含负向）、`scripts/check_observability_docs.sh` 或等价静态契约（文档漂移 4 处）
- 新增文件：`deploy/prometheus/rules/observability-self.yml`、`deploy/obs-mock-receiver/`（`Dockerfile` + 接收器脚本）、`charts/emotion-echo/charts/prometheus/templates/configmap-rules.yaml`
- 可能的 ADR：若"dev 通知渠道形态"（mock vs 真实）或"告警分层策略"偏离既有约定 ⇒ 需新立 ADR（RUNBOOK §13.3 断言 15 会查；Stage 86 的 dev 决策若被改动**必须**同步该 stage 文档）

---

## 9. 调研依据（AGENTS §〇.6 — 计划期 2026-09-29）

### 9.1 亲自回读的文件（Read 工具，非转述）

| 文件 | 读到的关键事实 |
|------|--------------|
| `deploy/alertmanager/alertmanager.yml`（全文 22 行） | `:13-18` route（`group_by: [alertname, component]` / `group_wait: 10s` / `repeat_interval: 4h`）；`:20-22` **唯一 receiver `dev-ui` 无任何集成段**；`:1-8` 注释自述"dev 只做聚合去重 + Web UI"并预留 prod 接入点 |
| `deploy/prometheus/prometheus.yml`（全文 79 行） | 5 个 job（`emotion-echo-services` 6 target / `apisix` / `skywalking-oap` / `prometheus` 自监控 / `kafka-exporter`）；**无 alertmanager job、无 llm-service target**；`:70-71` `rule_files`；`:76-79` alertmanager 接线 |
| `charts/emotion-echo/charts/alertmanager/templates/configmap.yaml`（全文 31 行） | `:13-18` route `group_by: [alertname, cluster, service]`（**与 dev 不同**）；`:20-24` receiver 指向 `.Values.webhook` 默认 `.invalid` 占位域名；`:26-31` **有 `inhibit_rules`（dev 侧没有）** |
| `charts/emotion-echo/charts/prometheus/templates/configmap.yaml:16-45` | `data:` 下**只有 `prometheus.yml` 一个 key**（`:16-17`）；`:35-36` `rule_files: /etc/prometheus/rules/*.yml` 注释自称"Stage 28-D adds 4 alert rules; mount same ConfigMap" ⇒ **意图挂载但实际无 rules key** |
| `charts/emotion-echo/charts/prometheus/templates/deployment.yaml:50-70` | `volumeMounts` 只有 `config`（`/etc/prometheus`）与 `data`（`/prometheus`）⇒ **无 `/etc/prometheus/rules` 挂载**（E1 的一手证据） |
| `scripts/smoke_observability.py:30-47, 320-405` | `EXPECTED_TARGETS` **8 项手抄清单**（无 llm-service / kafka-exporter / alertmanager）；规则断言只查 `KafkaConsumerGroupLagHigh` + `OutboxEventsDead` **"已加载"**；断言 15 用 counter 初始值 0 也算过 |
| `docs/stages/stage-86-outbox-dead-alert-2026-09-13.md`（全文） | §一 dev 决策（"只做聚合去重 + Web UI，不接外部渠道"）；§二 落地 4 层；§五 真实容器 e2e 全链五步（毒消息造死信法）；**§六 三项 open**：外部通知渠道（交本阶段）、consumer 进程级指标、MaxAttempts 面板（裁定暂不做） |
| `docs/deployment/runbook/observability-compose.md:30-68` | F1/F2/F3 三处过期（11 项 smoke / "Alertmanager dev 未启用" / "sw-oap 未设 SW_TELEMETRY → DOWN"） |
| `deploy/docker-compose.apps.yml:283-344` | `emotion-llm-service` 容器名 / `restart: on-failure` / healthcheck 打 `:8000/health` |

### 9.2 已查 ADR / 既有决策

| 文档 | 结论 |
|------|------|
| `docs/architecture/adr/`（41 个） | **无监控告警专属 ADR**。沾边的仅 `adr-2026-09-loki-aggregator-dev.md`（只覆盖 logs 层，不含 Prometheus/Alertmanager/Grafana 决策）⇒ 本阶段若定下"通知渠道形态"等决策，**需新立 ADR**（§8 产出物） |
| `stage-86` §一 + §六 | dev 通知渠道是**显式决策**（非遗漏），且明确"prod 演进时只加 receivers 不动 prometheus 侧" ⇒ B2 的分流设计不违背该决策 |
| `decisions.md` / RUNBOOK §9 | 无阻塞 E2E-22 的决策门；D-04 i18n 不阻塞任何阶段 |
| `charts/emotion-echo/Chart.yaml:14` | "本 chart 整体处于 决策 23 冻结期（学习资产不追加投入）" ⇒ **E2 只做修缺陷不追加 k8s 能力**；umbrella 四个观测 subchart 全 `enabled: false` 是设计而非缺陷，不动 |

### 9.3 计划期实测探针（本轮实跑，非引用）

**9.3.1 观测栈当前是死的 —— 且零告警**

```
$ docker ps -a --filter "name=emotion-echo" --format "{{.Names}}\t{{.Status}}"
emotion-echo-prometheus       Exited (255) 2 hours ago
emotion-echo-alertmanager     Exited (255) 2 hours ago
emotion-echo-kafka-exporter   Exited (255) 2 hours ago
emotion-echo-sw-oap           Exited (255) 2 hours ago
emotion-echo-minio / kafka / sw-ui  Exited (255) 2 hours ago
（grafana / loki / promtail / 6 业务 svc / web 仍 Up）

$ docker inspect emotion-echo-prometheus --format '{{.State.ExitCode}} | OOMKilled={{.State.OOMKilled}} | Error={{.State.Error}}'
255 | OOMKilled=false | Error=          # FinishedAt 2026-09-29T03:12:40Z，7 个容器同刻
$ docker inspect emotion-echo-alertmanager --format ...   → 255 | OOMKilled=false（03:12:40Z）
```

日志尾部显示两个容器**当时完全正常**（Prometheus "Server is ready" / Alertmanager "gossip settled"），**无崩溃堆栈** ⇒ 一次外部终止事件（非容器自身 OOM/崩溃）。

端点复测印证：

```
prometheus_health=000   grafana_health=200   alertmanager_health=000   loki_health=200   bff=200
```

⇒ **本计划最有说服力的实测素材**：Prometheus 与 Alertmanager 同刻死亡，**没有产生任何告警**（因为发告警的东西自己死了）。这不是假想风险，是本机刚刚发生的事实。测试点 #6/#7 正是为"下次能被发现"服务。

**9.3.2 环境侧另一处异常（范围外，记账本）**

```
$ docker inspect emotion-echo-db-migrate --format '{{.State.ExitCode}}'  → 1
$ docker logs --tail 3 emotion-echo-db-migrate
[migrate] 全部迁移应用完成，共 31 个文件（版本追踪已启用，幂等可重复执行）
[migrate] FATAL: Postgres 30s 内未就绪
```

⇒ 迁移本身全部成功，但**启动顺序竞态**导致容器以 1 退出（RUNBOOK §2.2 期望 `Exited (0)`）。**归属 E2E-23 / 部署编排域，不在本阶段修**，按 RUNBOOK §5 记账本。

**9.3.3 其它**

- `python scripts/e2e_stage_audit.py --all` → **30 阶段 0 FAIL**（本阶段收口后不得回退此基线）。
- `deploy/.devmode-session` **不存在** ⇒ 无双轨锁冲突（AGENTS §八）。
- `emotion-llm-service` 容器实测 `Up (healthy)`、端口 `8000/tcp, 50051/tcp`（**未映射宿主**）⇒ 容器网络内可被抓取，A2 可行。
- `deploy/prometheus/rules/` 实存 3 个文件（`kafka-dlq.yml` / `kafka-lag.yml` / `outbox-dead.yml`）共 4 条 alerting rule，0 条 recording rule。
- `git status -sb` 干净且与 `origin/main` 同步；`git branch --merged main` 仅 main ⇒ 满足 AGENTS §2.5。

### 9.4 引用的既有结论（来自账本 / 历史，非本轮新发现）

- **E2E-F-11**「Alertmanager 无外部通知渠道，仅 Web UI 聚合」→ 本阶段 B 节主目标，**唯一**归属 E2E-22 的留账。
- **E2E-F-07** 已于 E2E-21 闭环（Go 日志进 Loki）—— 与本阶段无因果，但共享 obs 栈，**注意不要重复改 `promtail-config.yaml`**（其负向对照依赖精确的 target 过滤）。
- **E2E-F-149**（分区表 `ON CONFLICT` 失效，致 E2E-15/19 降 partial）—— 属报表数据链，**不阻塞本阶段**；但若测试点 #9 造死信后报表侧无反应，先排除此因素再判 FAIL。

### 9.5 尚未验证、需执行期证实的假设（列入 plan 假设清单）

| 假设 | 若不成立 |
|------|---------|
| A1 恢复后的 `sw-oap:1234` 真能返回 `/metrics`（Stage 86 记录本机镜像源需 daocloud 前缀） | `skywalking-oap` job 判 DOWN；属观测面 ⇒ 范围内修或明确记账 |
| A2 `python:3-alpine` 本机/国内镜像源可拉 | 备选复用栈内 python 镜像；再不行测试点 #8 判 `BLOCKED`+理由，**不留空 receiver** |
| A3 alertmanager 容器能解析 compose 服务名 `obs-mock-receiver` | 改用容器 IP 或 network alias |
| A4 `up == 0` 规则能在 `for: 2m` 内对**已存在的** kafka-exporter DOWN 生效（无需先造故障） | 改为先停后等，测试点 #7 时序相应调整 |
| A5 Grafana 面板 `expr` 引用的指标名与各服务实际暴露的一致 | 预期不成立（测试点 #3 专项）；修复方向为改面板而非改断言 |
| A6 `OUTBOX_MAX_ATTEMPTS=1` 能把造死信成本从"100 次重试"降到分钟级 | 测试点 #9 回退 Stage 86 毒消息法；仍不可行则 `BLOCKED`+理由 |
