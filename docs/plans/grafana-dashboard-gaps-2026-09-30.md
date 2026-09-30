---
status: planned
priority: medium
owner: TBD
created: 2026-09-30
type: gap-analysis
source: 2026-09-30 用户会话（"我没用过 grafana，你可以帮我分析一下对这个项目来说面板配置是否有不足吗"）——只读调查，未改任何代码
depends-on: []
related-plans:
  - observability-sprint-b.md（PR-OBS-1~8 建了本套面板，16 PR 全落地 —— 本文是其覆盖度盘点，非替代）
  - conversation-memory-pending-decision-2026-09-24.md（同为"外部发现"型计划）
related-stages:
  - e2e-22-monitoring-alerting（✅ done，面板 expr 语义修正在该阶段落地）
  - e2e-21-logging-observability（✅ done，Loki + trace_id 链路在其落地）
related-ledger:
  - E2E-F-150（DLQ 24 条死信，E2E-22 已修面板可见性，处置归 E2E-24）
  - E2E-F-152（Prometheus 自身死亡时告警失明，归属运维轮）
  - E2E-F-153（dev/k8s 双栈漂移 —— **本计划 §A 假设 3 已下调其严重性**）
related-adrs:
  - adr-2026-09-loki-aggregator-dev.md（§2.1/§2.3 docker_sd 采集决策已实现）
---

# Grafana 面板覆盖度缺口（待排期）

> **本文只列"需要改的地方"，不重复现状描述。** 现状与实测数据见 §A。
> 全部条目为**新增**面板或**文档修正**，不改动现有 7 个面板，风险低。

---

## §A 上下文与假设（含本轮实测基线）

### A.1 项目前提（用户 2026-09-30 明确）

**本项目不使用 Kubernetes**，部署形态是 `deploy/docker-compose.*.yml`（infra + apps 双文件）。`docs/deployment/README.md:21` 的 k8s 部署文档至今标"待建"，`charts/emotion-echo/Chart.yaml:14` 自述"决策 23 冻结期（学习资产不追加投入）"，E2E-22 plan §决策 2 明确"本阶段不对齐版本"。

⇒ **`charts/**` 下的 Grafana 占位看板不是活缺陷，是冻结资产。** 任何涉及它的评估都要先扣掉这一项。

### A.2 现有面板清单（2026-09-30 运行时实测，全部返回真实数据）

Grafana 在 `http://localhost:13000`（admin/admin，folder `Emotion-Echo`），2 看板 7 面板：

| # | 看板 | 面板 | expr | 实测 |
|---|---|---|---|---|
| 1 | Overview | HTTP Request Rate | `sum by (service)(rate(emotion_echo_http_requests_total[1m]))` | series=6，0.045~0.089 req/s |
| 2 | Overview | HTTP Error Rate 5xx% | 5xx rate `or ...*0` ÷ 总 rate | series=6 |
| 3 | Overview | HTTP p95 Latency | `histogram_quantile(0.95, ...)` | series=6 |
| 4 | Overview | Goroutines | `sum by (job)(go_goroutines)` | services 377 / prometheus 69 / alertmanager 32 / kafka-exporter 11 |
| 5 | Kafka | ai-svc Consumer Lag | `kafka_consumergroup_lag{consumergroup="ai-svc"}` | series=1 |
| 6 | Kafka | analytics-svc Consumer Lag | `kafka_consumergroup_lag{consumergroup="analytics-svc"}` | series=1 |
| 7 | Kafka | DLQ Depth | `sum(kafka_topic_partition_current_offset{topic="chat-events-dlq"}) or vector(0)` | **24** |

6 个 scrape job 全部 `health=up`（alertmanager / apisix / emotion-echo-services×7 / kafka-exporter / prometheus / skywalking-oap）。

### A.3 核心判断

**现有面板对"基础设施是否活着"够用，对"业务是否正常"不足。** 7 个面板全是技术指标，没有一个能回答"用户能否正常跟 AI 聊天""报表数据新不新鲜"——而这是本项目最贵的资产。

生产端已有 **19 个 `emotion_*` 指标**，面板只用了 3 个（`emotion_echo_http_requests_total` + duration bucket），其余 **16 个零可视化**。

---

## §B 待排期条目

### B1 · 业务链路看板（**最高投入产出比**）

| | |
|---|---|
| **缺口** | 业务链路"发消息 → chat-svc → LLM → AI 回复 → analytics 消费 → 报表"在 Grafana 上完全不可见 |
| **证据** | 指标现成、无需新增埋点：<br>· `emotion_echo_fusion_llm_call_total` — LLM 调用量<br>· `emotion_echo_fusion_fallback_total` — **降级到 mock 的次数**<br>· `emotion_echo_fusion_llm_latency_seconds_bucket` — LLM 延迟 |
| **为什么重要** | `fusion_fallback_total` 持续上涨 = 用户正在跟不会思考的 mock AI 聊天。同类问题历史上多次出现（Stage 103 "聊天无 AI 回复"、Stage 107 会话超时），现在要发现它得手动 `curl :9090/api/v1/query` |
| **修法** | 新增 dashboard `emotion-echo-business`（3~4 panel），复用现成 metric，无需改 Go 代码 |
| **工作量** | ≈ 0.5 人天（纯 JSON + smoke 断言） |
| **回归钉** | `scripts/smoke_observability.py` 增 3 条"panel expr 返回非空 series"断言（该文件 2026-09-29 已建立此形态，见其 §面板断言段） |

### B2 · 有告警无面板的不对称

| | |
|---|---|
| **缺口** | 4 个 critical 告警指标，Grafana 上零图形。告警把你叫醒后没有现场图 |
| **证据** | `deploy/prometheus/rules/` 4 文件 7 规则，其中 4 条对应指标无面板：<br>· `emotion_echo_outbox_events_dead_total`（OutboxEventsDead，事件重试 100 次仍失败 = **永久丢失**）<br>· `emotion_echo_dlq_publish_total{result="failure"}`（AIDLQPublishFailure）<br>· `emotion_echo_analytics_dlq_publish_total{result="failure"}`（AnalyticsDLQPublishFailure）<br>· `up == 0`（PrometheusTargetDown） |
| **修法** | 并入 B1 看板（前三项）+ B3（第四项） |
| **工作量** | ≈ 0.3 人天 |
| **注意** | DLQ 面板在 E2E-22 已显式标注 `non-alerting`（死信进 DLQ 是预期行为），B1 加 DLQ 面板时**保留该标注**，不要顺手加告警——死信堆积的处置机制归 E2E-24（E2E-F-150） |

### B3 · 观测面自身健康看板

| | |
|---|---|
| **缺口** | Grafana 无 target 健康总览。E2E-22 记录 2026-09-29 六个容器同刻 `Exited(255)`，事后判断"当时哪个 target 掉了"只能靠人回忆 |
| **证据** | `PrometheusTargetDown` 规则已存在但只在 Alertmanager UI 可见；Grafana 无对应 panel |
| **修法** | 新增 `emotion-echo-observability` 看板：① target 健康矩阵 `up` by job/instance；② 当前 firing 告警列表 `ALERTS{alertstate="firing"}` |
| **工作量** | ≈ 0.3 人天 |
| **回归钉** | 负向对照：故意停一个 target ⇒ 该 panel 立刻反映（对齐 E2E-22 §负向对照 ① 的做法） |

### B4 · 日志侧看板（数据齐了，一行没画）

| | |
|---|---|
| **缺口** | Loki 已接通、7 个服务结构化日志在采、trace_id 跨服务链路已打通（E2E-21 核心成果）、前端错误上报已接（E2E-F-148 闭环）——**零个面板**，全靠 Explore 手敲 LogQL |
| **证据** | datasource `loki` 已注册（`deploy/grafana/provisioning/datasources/datasource.yaml:29`）；`grep -c loki deploy/grafana/dashboards/*.json` → 两个看板均 **0** |
| **修法** | 新增 `emotion-echo-logs` 看板：① 按 svc 的 error 级日志速率 `sum by (svc)(rate({job="services",level="error"}[5m]))`；② **trace_id 查询面板**（`{job="services"} |= "<trace_id>"` 表格）——手敲 LogQL 的门槛足以让人放弃排查，这是本条最有价值的部分 |
| **工作量** | ≈ 0.5 人天 |
| **前置校验** | LogQL 语法需**实机跑通**再写死进 JSON（Loki 3.2.0 迁移期 `/query` 已不支持日志查询，须用 `/query_range` —— 见 E2E-F-147 修复说明） |

### B5 · APISIX 指标已采但零使用

| | |
|---|---|
| **缺口** | `prometheus.yml` 抓了 APISIX `/apisix/prometheus/metrics`，两个看板对 `apisix` 引用数 = **0** |
| **为什么值得做** | APISIX 指标是**用户真实流量**的 QPS/延迟/状态码；现有面板的 HTTP QPS 是服务**内部自测**的。用户实际体验到什么恰恰没有面板 |
| **修法** | 并入 B3 看板或新建；先 `curl :9090/api/v1/label/__name__/values \| grep apisix` 确认实际可用指标名再写 expr（**不要凭记忆写指标名**） |
| **工作量** | ≈ 0.3 人天 |

### B6 · 基础设施盲区：Postgres / Redis 无任何 exporter

| | |
|---|---|
| **缺口** | compose 无 postgres-exporter / redis-exporter / node-exporter（`grep -n "exporter" deploy/docker-compose.infra.yml` → 仅 kafka-exporter + APISIX 自带 exporter） |
| **代价** | Postgres 是本项目无可争议的单点（业务数据 + 报表 + outbox 全在里面），但连接数、慢查询、锁等待、WAL、磁盘全是黑的。**根因在 DB 层的问题用现有面板查不出来**——账本未解决的 E2E-F-149（分区表化后 `ON CONFLICT (event_id)` 失效，analytics 每条消息消费失败 3 次、报表持续缺数据）即属此类 |
| **修法** | 加 postgres-exporter 服务 + prometheus.yml scrape job + 4~5 个核心指标面板（连接数/慢查询率/死元组/表膨胀/WAL 堆积） |
| **工作量** | ≈ 1 人天（含镜像选型与内存限额，本项目 `.wslconfig` 内存上限是已知约束，见 E2E-16 记录） |
| **排期建议** | 中期。B1~B3 优先级高于此项 |

### B7 · 无 recording rules

| | |
|---|---|
| **缺口** | `grep -rn recording_rule deploy/ charts/` → **0 命中**。所有面板每次刷新重算 `sum by (service)(rate(...))` |
| **影响** | Kafka 看板默认时间范围 `now-6h`，长期使用会明显变慢 |
| **修法** | 把 `emotion_echo:http_requests_per_second:rate5m` 等常用聚合写成 recording rule，面板直查 |
| **工作量** | ≈ 0.3 人天（需同步改 `rule_files` 目录约定） |

### B8 · 两个新手必踩的配置坑（**建议随 B1 一并处理**）

| | |
|---|---|
| **坑 1** | provider 配了 `allowUiUpdates: true` + `updateIntervalSeconds: 30` + 挂 `grafana_data` 持久卷。**UI 里改面板看似成功，30 秒后被磁盘 JSON 覆盖回去**。改动必须落 `deploy/grafana/dashboards/*.json` |
| **坑 2** | 无 dashboard 间跳转链接、无 templating 变量（`by svc` 写死在 expr）、无 Grafana 内建告警（全部走 Prometheus rules，故 Grafana 里**不会亮红点**，须自行访问 `:9093`） |
| **修法** | 坑 1 在 `dashboards.yaml` 注释里写明；坑 2 随 B1/B3 加 `templating` 变量与看板 `links` |
| **工作量** | ≈ 0.2 人天（注释 + JSON 字段） |

### B9 · 文档缺口（针对"没用过 Grafana"的用户）

| | |
|---|---|
| **缺口 1** | `docs/deployment/runbook/observability-compose.md`（258 行）**全篇是 curl 命令**。§1.3「UI 入口速查」只给 URL 与账号，**从头到尾没有一句"打开 Grafana 后看哪几个面板、各面板什么含义、什么值算异常、怀疑哪个服务"**。curl 是给排障精确取证用的，不是日常看健康度用的 |
| **缺口 2（文档漂移）** | 同文件 `:109` 写「业务 svc stdout \|（未采集，PR-OBS-? 范围）\| docker sd 模式未来扩展」——**已过时**。`deploy/loki/promtail-config.yaml:42` 已是 `docker_sd_configs` + 容器名白名单，E2E-F-07 已于 2026-09-28 闭环（实测 `added Docker target` × 7，`{job="services"}` 命中 7 个 svc）。该行会让人误以为 Go 服务日志根本没进 Loki |
| **修法** | runbook 增「面板导览」一节（7 现有 + N 新面板逐个说明：含义 / 正常区间 / 异常时先查什么）；修正 `:109` |
| **工作量** | ≈ 0.3 人天 |
| **归属说明** | 缺口 2 是**纯文档修正**、无歧义，可随时单独做，不必等排期；缺口 1 随 B1~B3 落地后写才准确（否则会立刻再次过时） |

---

## §C 待用户拍板

| # | 事项 | 说明 |
|---|---|---|
| C1 | **本计划是否登记 E2E-F 账本编号** | 本计划由外部发现（非阶段演进内部识别），按项目体例可登记为新发现项。但 8 条里 B1~B5、B7 属"增强"、B6/B8/B9 属"补齐"，性质不同，是否都值得占编号需拍板 |
| C2 | **归属哪个 E2E 阶段** | E2E-22 已 done，本计划不属其范围。候选：并入 **E2E-24**（消息链路 outbox→Kafka→DLQ，覆盖 B1/B2 部分）/ **E2E-30**（数据契约收口）/ 新开监控收口阶段。roadmap 30 阶段无一以"Grafana 面板覆盖度"为目标 |
| C3 | **CI 里两个 helm job 是否清理** | `.github/workflows/doc-drift-check.yml` 的 `helm-loki-render`(:202) + `helm-prometheus-render`(:234) 为**不部署的 k8s 学习资产**持续花 CI 时间。因属"决策 23 冻结期"产物，删除属范围外决策，需拍板 |
| C4 | **B6 postgres-exporter 的内存成本** | 本项目 `.wslconfig` 内存上限是已知硬约束（19 容器栈稳态已贴顶，见 E2E-16 记录）。加 exporter 需评估是否挤占既有容器预算 |

---

## §D 调研依据

实际读过的文件（不凭记忆，全部 Read/Grep 过）：

- `deploy/grafana/dashboards/emotion-echo-overview.json`、`kafka-consumer-lag.json`（面板 expr 原文逐条摘录）
- `deploy/grafana/provisioning/dashboards/dashboards.yaml`、`datasources/datasource.yaml`
- `deploy/prometheus/prometheus.yml`、`deploy/prometheus/rules/{kafka-dlq,kafka-lag,observability-self,outbox-dead}.yml`
- `deploy/docker-compose.infra.yml`（grafana/prometheus/loki/promtail/kafka-exporter/alertmanager 服务段 + exporter grep）
- `deploy/loki/promtail-config.yaml`（确认为 `docker_sd_configs`，据此判定 runbook `:109` 过时）
- `charts/emotion-echo/charts/grafana/templates/configmap-dashboards.yaml`（占位看板，据此下调 k8s 相关严重性）
- `scripts/smoke_observability.py`（现有面板断言形态，B1~B3 回归钉的落点）
- `docs/e2e-roadmap/stages/e2e-22-monitoring-alerting/{plan,report}.md`、`docs/e2e-roadmap/discovered-unresolved.md`（E2E-F-07/150/152/153 原文）
- `docs/deployment/README.md`、`docs/deployment/runbook/observability-compose.md`

运行时实测（只读查询，未重启未修改）：`docker ps`、`GET :9090/api/v1/targets`、`GET :9090/api/v1/query`（7 条 panel expr 逐条 + `label/__name__/values`）、`GET :13000/api/search`、`GET :13000/api/datasources`。

**未能确认**（需实机验证，本文未据此下结论）：

- k8s sidecar 渲染后是否真在跑（无集群环境，且该项已按 §A.1 降级）
- `grafana_data` 卷内是否有 UI 手工改过并落盘的偏差（provider 允许 UI 编辑）
- B4 的 LogQL 语法未经实机跑通
- B5 的 APISIX 实际指标名未列举
