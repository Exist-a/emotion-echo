---
status: landed
priority: medium
owner: lane-cl-b（阶段 CL-01）
created: 2026-10-11
landed: 2026-10-14（阶段 CL-01 · PR #211；四块看板 A/B/C/D + 6 条告警规则全部落地，推进方式与逐点证据见 `docs/closure-roadmap/stages/cl-01-observability/{plan,report}.md`）
last-updated: 2026-10-14（**落地归档**：CL-01 收口，本文件自 `docs/plans/` 迁入 `docs/legacy-plans/landed/`；引用旧路径的文档请改指本文件）
type: implementation-plan
source: 2026-10-11 会话（用户："把没接入的业务指标接入一下，整理文档就行"）；承接 2026-10-11 健康检查/面板核查 + 2026-09-30 缺口分析
depends-on: []
related-plans:
  - grafana-dashboard-gaps-2026-09-30.md（缺口分析：B1 业务看板 / B2 有告警无面板 / B3 观测面自身健康 / B4 日志看板 —— 本计划是它的**实施版**并已把 B4 纳入范围）
  - observability-compose-gap.md（PR-OBS-1~8 建了现有 7 面板，本计划在其上新增）
---

# Plan — 未接入的业务指标接入监控栈（Grafana 面板 + Prometheus 告警）

> **只描述要做的事，不含代码改动。**
> 现状：生产端已埋 **19 个自定义 `emotion*` 指标**，但 Grafana 只画了 **2 个**（HTTP 请求量/延迟），Prometheus 只为 **3 个**配了告警。**约 15 个业务/可靠性指标"埋了点，没人看"**。本计划把这些指标 + 已采到却零面板的日志（Loki）接进看板与告警。

> **⚠︎ 与 E2E 路线图的关系（2026-10-11 用户明确）**：本计划**独立于 E2E 路线图**——30 阶段已定稿，本计划**不登记 E2E-F 账本编号、不归属任何阶段**。它是"运维/可观测性增强"，与 E2E 阶段体系脱钩。（既有 `grafana-dashboard-gaps-2026-09-30.md` 中 E2E-22 的引用仅作历史来源，不代表归属。）

---

## §0 上下文（★ 写文档前的功课，按 AGENTS.md §〇）

### 0.1 已读代码 / 配置（真实文件）

| 文件 | 关键事实 |
|---|---|
| `emotion-echo-shared/pkg/metrics/metrics.go` | HTTP 指标 + `skywalking_init_failed_total` / `model_client_init_failed_total` / `mv_refresh_{success,fail,duration}` / `panic_total` 定义（含 label） |
| `emotion-echo-shared/pkg/metrics/fusion_metrics.go` | `fusion_llm_call_total{outcome}` / `fusion_llm_latency_seconds{outcome}` / `fusion_fallback_total{stage}` / `fusion_worker_tick_total{outcome}` / `fusion_lru_stat{kind}` / `emotion_fusion_calls_total{modality,result}` / `emotion_fusion_duration_seconds{modality}` |
| `emotion-echo-shared/pkg/metrics/tts_metrics.go` | `tts_fallback_total{from}`（cloud→local 降级） |
| `emotion-echo-chat-svc/internal/outbox/metrics.go` | `outbox_events_dead_total` / `outbox_sent_via_fallback_total` / `outbox_cleaned_total` |
| `emotion-echo-ai-svc/internal/consumer/dlq_metrics.go`、`emotion-echo-analytics-svc/internal/kafka/dlq_metrics.go` | `dlq_publish_total{result}`（两服务各一份命名空间） |
| `deploy/grafana/dashboards/{emotion-echo-overview,kafka-consumer-lag}.json` | 现有 **2 看板 7 面板**；expr 无一个引用上述业务指标 |
| `deploy/grafana/provisioning/dashboards/dashboards.yaml` | file provider 自动加载 `/etc/grafana/dashboards/*.json`；`allowUiUpdates:true` + `updateIntervalSeconds:30`（**UI 改会被磁盘 JSON 覆盖**） |
| `deploy/grafana/provisioning/datasources/datasource.yaml` | datasource = **Prometheus + Loki**（Loki 已注册，可直接建日志看板） |
| `deploy/prometheus/prometheus.yml` | `metrics_path: /metrics`（**不采 `/health`**）；`rule_files: /etc/prometheus/rules/*.yml`（新增规则文件自动加载） |
| `deploy/prometheus/rules/{kafka-lag,kafka-dlq,outbox-dead,observability-self}.yml` | 现有 **4 文件 7 规则**；kafka-lag.yml 作新增规则的格式模板 |
| `scripts/smoke_observability.py` | 已有"**把面板 expr 真打到 Prometheus、要求返回非空 series**"的断言机制（`_panel_exprs`）——本计划**回归钉落点** |
| `deploy/loki/promtail-config.yaml` | `docker_sd_configs` + 容器名白名单（7 个 svc 已采；`{job="services"}` 可查） |

### 0.2 相关 ADR / 决策

- 无 ADR 专门覆盖"指标→面板映射"；E2E-22 / `observability-sprint-b.md` 已确立"指标埋点 + 告警"范式，本计划只补消费端。
- 已知约束：本项目**不用 k8s**，`charts/**` 的 Grafana 占位看板是**冻结资产**，不改。

### 0.3 缺口定性（三类）

1. **指标无面板**：19 个自定义指标里只有 2 个（HTTP）有面板；3 个有告警无面板；**其余 14 个既无面板也无告警**。
2. **关键行为无告警**：连 **HTTP 5xx 错误率**都只有面板、没有告警。
3. **日志已采零面板**：Loki 已接、7 个服务结构化日志在采、trace_id 跨服务链路已通——**Grafana 上一条日志面板都没有**，排障全靠手敲 LogQL。

> **另注（范围外）**："健康检查没进监控栈"（`/health` 不被 Prometheus 采集、无 blackbox）是**另一个独立缺口**，**不在本计划范围**。

---

## §1 问题陈述

一句话：**指标埋了但只消费了 2/19、日志采了但零面板**——业务是否正常（用户能不能正常跟 AI 对话、报表数据新不新鲜）在监控上**不可见**。

逐项（指标 → 现状）：

| 指标 | 含义 | 面板 | 告警 |
|---|---|---|---|
| `emotion_echo_http_requests_total` / `_duration_seconds` | HTTP 量/延迟 | ✅ | ❌（**5xx 无告警**） |
| `emotion_echo_fusion_llm_call_total` / `_llm_latency_seconds` | LLM 调用量/延迟 | ❌ | ❌ |
| `emotion_echo_fusion_fallback_total` | **降级到 mock 的次数** | ❌ | ❌ |
| `emotion_fusion_calls_total` / `_duration_seconds` | fusion 整体调用/延迟 | ❌ | ❌ |
| `emotion_echo_fusion_worker_tick_total` / `_lru_stat` | fusion worker / LRU | ❌ | ❌ |
| `emotion_echo_tts_fallback_total` | TTS 云→本地降级 | ❌ | ❌ |
| `emotion_echo_outbox_events_dead_total` | outbox 事件永久失败 | ❌ | ✅ |
| `emotion_echo_outbox_sent_via_fallback_total` | **内存兜底"假装成功"计数（黑洞）** | ❌ | ❌ |
| `emotion_echo_outbox_cleaned_total` | outbox 清理量 | ❌ | ❌ |
| `emotion_echo_dlq_publish_total{result}` / `emotion_echo_analytics_dlq_publish_total{result}` | DLQ 投递失败 | ❌ | ✅ |
| `emotion_echo_panic_total` | handler panic | ❌ | ❌ |
| `emotion_echo_skywalking_init_failed_total` | tracer 初始化失败 | ❌ | ❌ |
| `emotion_echo_model_client_init_failed_total` | 模型客户端 init 失败 | ❌ | ❌ |
| `emotion_echo_mv_refresh_{success,fail,duration_seconds}` | 物化视图刷新 | ❌ | ❌ |

---

## §2 目标与非目标

### 目标
- **G1** 为**无面板**的指标建看板（复用现成指标，**不改 Go 埋点代码**）。
- **G2** 为**关键行为**补告警规则（降级、panic、init 失败、黑洞计数、5xx）。
- **G3** 建 **Loki 日志看板**（错误速率 + **trace_id 查询面板**）。
- **G4** 回归钉：扩展 `scripts/smoke_observability.py`，对**每个新面板的 expr** 断言"返回非空 series"，对**每条新规则**做可加载性/触发对照。
- **G5** 修正文档：面板清单、告警清单与现状一致。

### 非目标
- 不改任何 Go 埋点（指标已齐全）。
- 不动 `charts/**`（k8s 冻结资产）。
- 不引入 postgres/redis exporter（既有计划 B6，中期项）。
- 不做 recording rules（既有计划 B7）。
- **不登记 E2E-F 账本、不归属任何 E2E 阶段**（§顶部说明）。

---

## §3 设计：看板 / 告警 映射

### 3.1 看板 A：`emotion-echo-business`（AI 业务链路，**最高投入产出比**）

| 面板 | 类型 | expr（示例，落地前须实机确认指标名/标签） |
|---|---|---|
| LLM 调用量 by outcome | timeseries | `sum by (outcome)(rate(emotion_echo_fusion_llm_call_total[5m]))` |
| LLM 延迟 P95 by outcome | timeseries | `histogram_quantile(0.95, sum by (le,outcome)(rate(emotion_echo_fusion_llm_latency_seconds_bucket[5m])))` |
| **降级到 mock by stage** | timeseries | `sum by (stage)(rate(emotion_echo_fusion_fallback_total[5m]))` |
| fusion 调用 by modality/result | timeseries | `sum by (modality,result)(rate(emotion_fusion_calls_total[5m]))` |
| fusion 延迟 P95 by modality | timeseries | `histogram_quantile(0.95, sum by (le,modality)(rate(emotion_fusion_duration_seconds_bucket[5m])))` |
| TTS 云→本地降级 | timeseries | `sum(rate(emotion_echo_tts_fallback_total[5m]))` |
| fusion worker tick by outcome | timeseries | `sum by (outcome)(rate(emotion_echo_fusion_worker_tick_total[5m]))` |
| fusion LRU 命中/未命中 | timeseries | `emotion_echo_fusion_lru_stat` |

> **为什么这个看板最值**：`fusion_fallback_total` 持续上涨 = **用户正在跟不会思考的 mock AI 聊天**——历史上多次发生，但只能靠手敲 `curl :9090` 才发现。看板让它在图形上直接可见。

### 3.2 看板 B：`emotion-echo-reliability`（可靠性）

| 面板 | 类型 | expr |
|---|---|---|
| outbox 死事件 | stat + timeseries | `emotion_echo_outbox_events_dead_total` |
| **outbox 内存兜底（黑洞）** | stat | `emotion_echo_outbox_sent_via_fallback_total` |
| outbox 清理量 | timeseries | `rate(emotion_echo_outbox_cleaned_total[5m])` |
| DLQ 投递失败（ai / analytics） | timeseries | `rate(emotion_echo_dlq_publish_total{result="failure"}[5m])` + `rate(emotion_echo_analytics_dlq_publish_total{result="failure"}[5m])` |
| panic by service | timeseries | `sum by (service)(rate(emotion_echo_panic_total[5m]))` |
| tracer 初始化失败 | stat | `emotion_echo_skywalking_init_failed_total` |
| 模型客户端 init 失败 by model | timeseries | `sum by (model)(rate(emotion_echo_model_client_init_failed_total[5m]))` |
| MV 刷新失败率 / 耗时 | timeseries | `rate(emotion_echo_mv_refresh_fail_total[5m])` + `histogram_quantile(0.95, ...mv_refresh_duration_seconds_bucket...)` |

### 3.3 看板 C：`emotion-echo-observability`（观测面自身）

| 面板 | expr |
|---|---|
| Target 健康矩阵 | `up` by `job`/`instance` |
| 当前 firing 告警 | `ALERTS{alertstate="firing"}` |

### 3.4 看板 D：`emotion-echo-logs`（Loki，本次一并做 ← 用户 2026-10-11 拍板）

| 面板 | 类型 | LogQL（示例，落地前须实机跑通） |
|---|---|---|
| 各 svc error 日志速率 | timeseries | `sum by (svc)(rate({job="services",level="error"}[5m]))` |
| **trace_id 查询面板** | logs/table | `{job="services"} \|= "<trace_id>"` |

> **为什么值**：trace_id 跨服务链路已打通，但手敲 LogQL 的门槛足以让人放弃排查——这是本条最有价值的部分（"一键粘 trace_id 看全链路日志"）。
> **前置校验**：LogQL 语法须**实机跑通**再写死进 JSON。**Loki 3.2.0 迁移期 `/query` 已不支持日志查询，须用 `/query_range`**（见 E2E-F-147 修复说明）。label 名（`level` / `svc`）以实机 `label/__name__/values` 为准。

### 3.5 新增告警规则（`deploy/prometheus/rules/`，新文件自动加载）

| 规则名 | expr | for | severity | 意图 |
|---|---|---|---|---|
| `HTTP5xxRateHigh` | `sum(rate(emotion_echo_http_requests_total{status=~"5.."}[5m])) / sum(rate(emotion_echo_http_requests_total[5m])) > 0.05` | 5m | warning | 服务在报错（现在**无此告警**） |
| `FusionFallbackRateHigh` | `sum(rate(emotion_echo_fusion_fallback_total[5m])) > 0` | 10m | warning | **用户在与 mock AI 对话** |
| `TTSFallbackRateHigh` | `sum(rate(emotion_echo_tts_fallback_total[5m])) > 0` | 10m | warning | cloud TTS 持续不可用 |
| `PanicOccurred` | `increase(emotion_echo_panic_total[5m]) > 0` | 0m | critical | handler panic |
| `SkyWalkingInitFailed` | `increase(emotion_echo_skywalking_init_failed_total[10m]) > 0` | 0m | warning | tracer 没起来 |
| `ModelClientInitFailed` | `increase(emotion_echo_model_client_init_failed_total[10m]) > 0` | 0m | warning | 模型客户端配置缺失 |
| `MVRefreshFailing` | `increase(emotion_echo_mv_refresh_fail_total[15m]) > 0` | 0m | warning | 报表数据不新鲜 |
| `OutboxSentViaFallback` | `increase(emotion_echo_outbox_sent_via_fallback_total[10m]) > 0` | 0m | critical | **黑洞被触发**（事件被"假装成功"） |

> **阈值口径（用户 2026-10-11 拍板）**：降级类告警采用**零容忍 + 时长过滤**——expr 用 `> 0`，但配 `for:`（10m/5m）滤掉瞬时抖动。理由：降级本就是"要立刻知道"的事，不该被速率阈值稀释。
> **格式**：照抄 `deploy/prometheus/rules/kafka-lag.yml`（`labels.severity/component` + `annotations.summary/description`）。
> **DLQ 深度保持 non-alerting**——死信进 DLQ 是预期行为，**不要**给 DLQ 加告警。

---

## §4 分阶段实施（TDD：先断言 RED，再实现 GREEN）

> 这些是"配置"，但项目纪律仍是**先写验收再写实现**：先扩 smoke 断言（RED），再加面板/规则（GREEN），最后负向对照。

### Phase 0 — 指标名/标签 + LogQL 实机核对（**不做这步不许写 expr**）

- 起 obs 栈（`--profile obs`）；`curl :9090/api/v1/label/__name__/values | grep emotion` 逐条确认指标**真实存在**、label 名与代码一致。
- Loki：`{job="services"}` 实机跑通 error 速率与 trace_id 查询（**用 `/query_range`**）。
- **理由**（历史教训）：expr 引用不存在的指标会静默返回空、看着像"没数据"。**不凭记忆写指标名 / LogQL。**

### Phase 1 — 扩展 smoke 断言的 RED

- 在 `scripts/smoke_observability.py` 增加：新看板 UID 可读 + **每个面板 expr 返回非空 series**；新规则文件可加载。
- 此时断言必然 **RED**（面板/规则还没建）——"测试先行"的 Red。

### Phase 2 — 看板 A/B/C/D（GREEN）

- 新增 4 个 dashboard JSON 到 `deploy/grafana/dashboards/`（**改文件，不改 UI**——见 §6 坑 1）。
- A/B/C 复用现成指标、D 用 Loki；**零 Go 代码改动**。

### Phase 3 — 告警规则（GREEN）

- 新增 `deploy/prometheus/rules/*.yml`。
- `promtool check rules` 校验语法（若环境有）；无则靠 Prometheus 启动日志确认加载。

### Phase 4 — 负向对照（关键，防假绿）

- 故意触发一条：如临时压测触发 fusion fallback / 停一个 target ⇒ 断言 **panel 立刻反映 + 告警 firing 送达**（对齐 E2E-22 §负向对照做法）。
- **理由**：正向"面板有数据"不足以证明告警链路通；必须**真触发一次**。

### Phase 5 — 文档同步

- 更新 `docs/deployment/runbook/observability-compose.md`：面板清单（7 → 7+4 看板）、告警清单、每条"什么值算异常、异常先查哪"；修正既有计划点名的过时行。

---

## §5 验收与回归钉

- **回归钉（机器可跑）**：`scripts/smoke_observability.py` 新增断言全绿；`promtool check rules`（或 Prometheus 加载日志）无错。
- **负向对照**：Phase 4 的触发-观测记录（截图/查询结果）作证据。
- **人工验收**：打开 Grafana，`emotion-echo-business` 能回答"现在有没有在降级"；`emotion-echo-logs` 能粘 trace_id 看全链路。
- **收口**：本计划落地后迁 `docs/legacy-plans/landed/`；同步 `grafana-dashboard-gaps-2026-09-30.md` 的 B1/B2/B3/B4 状态。

---

## §6 风险与坑（多数来自既有计划实测）

| 坑 | 说明 | 规避 |
|---|---|---|
| **UI 改动被覆盖** | provider `allowUiUpdates:true` + `updateIntervalSeconds:30` + 持久卷 ⇒ UI 改面板 30 秒后被磁盘 JSON 覆盖 | **一切改动落 `deploy/grafana/dashboards/*.json`**，不在 UI 改 |
| **凭记忆写指标名** | expr 引用不存在的指标 → 空 series，看着像"没数据" | Phase 0 先 `label/__name__/values` 核对 |
| **LogQL 写错** | 面板永远空；Loki 3.2.0 `/query` 不支持日志查询 | Phase 0 用 `/query_range` 实机跑通再写死 |
| **告警阈值拍脑袋** | 太低 → 误报刷屏；太高 → 形同虚设 | 零容忍 + `for:` 时长过滤；上线后按真实数据校准 |
| **DLQ 误加告警** | 死信进 DLQ 是**预期行为** | DLQ 面板保持 `non-alerting` |
| **一次性计数器** | `outbox_sent_via_fallback_total` 是**启动时递增**的一次性计数器 | 告警用 `increase(...[10m])` 而非绝对值 |
| **内存约束** | 本项目内存上限是硬约束（19 容器栈已贴顶） | 本计划**只加配置不加容器**，无内存增量 |
| **k8s 冻结资产** | `charts/**` 占位看板无意义 | 不碰 |

---

## §7 与既有计划的关系

- **`grafana-dashboard-gaps-2026-09-30.md`（缺口分析）**：本计划是它的**实施版**——B1/B2/B3/B4 → 本计划 §3.1/§3.2/§3.3/§3.4；B5（APISIX 指标）/B6（pg/redis exporter）/B7（recording rules）**不在本计划范围**，仍待排期。
- **E2E 路线图**：**不挂钩**（§顶部）。
- **健康检查没进监控栈**：独立缺口，**不在本计划范围**。

---

## §8 决策记录（2026-10-11 用户拍板）

| # | 事项 | 结论 |
|---|---|---|
| D1 | 看板粒度 | **3 个独立看板**（business / reliability / observability）+ 本次**追加 `emotion-echo-logs`**（见 D3） |
| D2 | 降级类告警阈值 | **零容忍 + 时长过滤**：expr `> 0` 配 `for:` 10m |
| D3 | 是否顺带做 Loki 日志看板（B4） | **一起做** —— 纳入本计划 §3.4 |
| D4 | 账本归属 | **不登记 E2E-F 编号、不归属任何 E2E 阶段**（30 阶段已定稿；本计划及此前发现的问题均与 E2E 阶段体系脱钩） |

---

## §9 调研依据（本次实读，不凭记忆）

- `emotion-echo-shared/pkg/metrics/{metrics.go, fusion_metrics.go, tts_metrics.go}`
- `emotion-echo-chat-svc/internal/outbox/metrics.go`、`emotion-echo-ai-svc/internal/consumer/dlq_metrics.go`、`emotion-echo-analytics-svc/internal/kafka/dlq_metrics.go`
- `deploy/grafana/dashboards/*.json`（面板/expr 逐条）、`deploy/grafana/provisioning/dashboards/dashboards.yaml`、`deploy/grafana/provisioning/datasources/datasource.yaml`
- `deploy/prometheus/prometheus.yml`、`deploy/prometheus/rules/*.yml`
- `deploy/loki/promtail-config.yaml`
- `scripts/smoke_observability.py`、`docs/plans/grafana-dashboard-gaps-2026-09-30.md`
- 全仓指标名清点：`grep -rhoE "emotion_echo_[a-z_]+|emotion_fusion_[a-z_]+" --include="*.go"`（19 个自定义指标）

**未能确认**（需实机，本文未据此下结论）：各指标的**实际 label 取值**（如 fusion `stage` 合法值、Loki `level`/`svc` label）、哪些指标在当前运行态**已有数据**（部分 counter 可能未被触发过）——Phase 0 核对。
