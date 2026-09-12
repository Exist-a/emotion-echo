# Stage 86 — Outbox dead 行告警全链（指标 → Prometheus 规则 → Alertmanager）

> 日期：2026-09-13
> 类型：feat + fix（TDD：RED 失败测试 → GREEN 最小实现 → 真实容器 e2e）
> 关联：roadmap open 清单第 4 项（Kafka P3，§3.6）、Stage 43 PR-A6.1（dead 状态机本体）、
> Stage 73（Protobuf 契约）、PR-OBS-7（lag 告警先例）

---

## 一、背景

kafka-reliability-gaps.md §3.6（P3）分两半：
- **dead 状态机**：MarkFailed 超 MaxAttempts → status=dead，不再被 ListPending 扫描
  —— **Stage 43 已落地**（62a293b），但 relay 层没有测试（只有 repo 级 outbox_test.go）。
- **dead 告警**：dead = 事件永久丢失（区别于 lag 的可追积压），需 log + metrics + 告警
  —— log 已有，**metrics 与告警链路完全缺失**，本期收口。

设计决策：
- 指标 `emotion_echo_outbox_events_dead_total`（无 label Counter）——dead 事件预期极低，
  不需要维度拆分；severity 用 **critical**（数据丢失），与 kafka-lag 的 warning 区分。
- dev 的 Alertmanager 只做**聚合去重 + :9093 Web UI**，不接外部通知渠道
  （单人 dev 无值班群；prod 演进时只加 receivers，不动 Prometheus 侧）。
- 顺手把 MaxAttempts 配置化（`Outbox.MaxAttempts` + `OUTBOX_MAX_ATTEMPTS` env），
  默认 100 与 Stage 43 硬编码一致，0 = 关闭 dead 状态机（向后兼容语义保留）。

## 二、落地内容（按层）

| 层 | 内容 |
|---|---|
| chat-svc 指标 | `internal/outbox/metrics.go`：`OutboxEventsDeadTotal`（promauto 注册 default registry，main.go :8890 /metrics 已有暴露通路）；relay.go MarkDead 成功后 `IncDead()` |
| chat-svc 配置 | `internal/config/config.go` 新增 `Outbox.MaxAttempts`（SetDefaults 默认 100）；main.go `applyEnvOverrides` 读 `OUTBOX_MAX_ATTEMPTS`（非法值忽略走默认）+ `relay.MaxAttempts = c.Outbox.MaxAttempts` |
| 告警规则 | `deploy/prometheus/rules/outbox-dead.yml`：`OutboxEventsDead`（expr: counter > 0，for 1m，critical/chat-svc） |
| Alertmanager | `deploy/alertmanager/alertmanager.yml`（route: dev-ui receiver，无集成纯 UI 展示）+ infra compose `alertmanager` 服务（prom/alertmanager:v0.27.0，obs profile，:9093）+ prometheus.yml `alerting:` 段接线 |
| smoke | `smoke_observability.py` 新增 4 断言：OutboxEventsDead 规则加载 / alertmanager healthy / prometheus 发现 alertmanager（v2.51 API 返回 `url` 而非 `labels`）/ dead 计数器 series 端到端可查 |
| **迁移修复** | analytics 001 移除 `msg_summary_v` 旧 8 列重复定义（详见 §四，e2e 揪出的 Stage 82 遗留幂等性 bug） |

## 三、RED→GREEN 过程

RED（3 文件）：
1. `relay_dead_test.go` — `TestRelay_MaxAttemptsReached_MarksDead`：补 Stage 43 缺失的
   relay 级 dead 状态机测试（回归锁，直接绿）+ 断言 dead 计数器递增（**红**：指标不存在）
2. `relay_dead_test.go` — `TestRelay_MaxAttemptsZero_DisablesDead`：MaxAttempts=0 无限重试
   回归锁 + 计数器不动（红）
3. `config_test.go` / `main_internal_test.go` — Outbox.SetDefaults 默认 100 / 显式值保留 /
   env 覆盖 4 场景表驱动（红：Outbox 类型未定义，编译红）

GREEN：metrics.go + relay.go IncDead + config Outbox + main.go env 接线，全绿。
教训一记：两个断言全局 counter 的测试不能 `t.Parallel`（首跑互踩 delta）。

## 四、e2e 揪出的问题：analytics 001 幂等性 bug（Stage 82 遗留）

起栈时 `emotion-echo-db-migrate` 失败：analytics 001 `CREATE OR REPLACE VIEW
emotion_echo_chat.msg_summary_v` 报 **cannot drop columns from view**。

- 根因：`migrate.sh` 每次 `up` **全量重跑**且要求幂等；Stage 82 PR-3b 把视图升级为带
  `intent` 的 9 列版时，只改了 chat-svc 005（DROP+CREATE+GRANT）与集中式
  deploy/db/04-create-views.sql，**analytics 001 的旧 8 列重复定义漏同步**——重跑时
  CREATE OR REPLACE 会挤掉 intent 列被 Postgres 拒绝。
- 之前未暴露的原因：Stage 83/85 的 e2e 都是在已迁移完成的栈上滚动重启 svc，
  未触发 db-migrate 全量重跑。
- 修复：analytics 001 移除 msg_summary_v 定义（所有权收敛到 chat-svc 005 +
  04-create-views.sql，消除双 owner 漂移点），留注释说明。
- 回归锁：`deploy/db/test_migrations_contract.sh` §契约 4（全量重复执行）本就该抓此类
  bug——本次属于"契约在、执行缺席"（栈未重跑过），修复后 PASS。

## 五、验收

- `go test ./...` 7 模块全绿 + chat-svc `go vet` 干净
- smoke_observability **17/17 PASS**（含新增 4 断言）
- smoke_data_layer **11/11 PASS**（§契约 3 含 analytics_reader 读 msg_summary_v——
  迁移修复未破坏视图与授权）
- 迁移契约测试全 PASS（§契约 4 幂等重跑 / §契约 5 initdb 链）
- **真实容器 e2e（全链）**：
  1. chat-svc /metrics 暴露 `emotion_echo_outbox_events_dead_total 0`
  2. 插入毒消息（`data.messageId="not-an-int"`，结构合法但 typed 反序列化失败）→
     relay 每秒重试，attempts 29→58→87→**100 次 fail 后 status=dead**
  3. Prometheus 计数器=1 → `OutboxEventsDead` pending（for 1m）→ **firing(critical)**
  4. **Alertmanager 收到告警**（api/v2/alerts：OutboxEventsDead / active / critical / chat-svc）
  5. 清理毒行 + 重启 chat-svc → 计数器归零、告警解除
- 运维注：本机镜像源（aliyun/ustc/163）均拉不到 prom/alertmanager，经
  `docker.m.daocloud.io` 前缀拉取后 retag；其他机器复现部署时同样处理。

## 六、本批未做（open）

| 项 | 说明 | 去向 |
|---|---|---|
| consumer 进程级指标（消费速率/处理耗时） | §1.4 残余，lag 告警已覆盖主场景，可选 | kafka-reliability-gaps（已迁 landed）residuals |
| Alertmanager 外部通知渠道 | dev 仅 UI 展示；prod 按 severity 分流 webhook | prod 部署时处理（alertmanager.yml 已留接入点） |
| MaxAttempts 的 Grafana 面板 | 当前只有告警无面板；dead 计数器 99% 时间恒 0，面板价值低 | 暂不做 |

## 七、调研依据

- 代码：`emotion-echo-chat-svc/internal/outbox/{relay.go,relay_test.go}`、
  `internal/repository/outbox.go`（MarkDead/OutboxStatusDead 已存在=Stage 43 语义基线）、
  `internal/config/config.go`、`main.go`（applyEnvOverrides + relay 构造 + :8890 /metrics）、
  `emotion-echo-shared/pkg/metrics/{metrics.go,fusion_metrics.go}`（promauto 范式 +
  RegistryGatherCounter 测试辅助）、`emotion-echo-analytics-svc/migrations/001_create_views.sql`、
  `emotion-echo-chat-svc/migrations/005_refresh_msg_summary_view.sql`、
  `deploy/db/{migrate.sh,test_migrations_contract.sh,04-create-views.sql}`、
  `scripts/smoke_observability.py`
- 文档：kafka-reliability-gaps.md §1.6/§3.6、stage-43-kafka-reliability-sprint-a.md §3.5、
  stage-73（UnmarshalChatEventJSON 契约，e2e 毒消息设计依据）、roadmap open 清单第 4 项
- 现状 smoke：本 stage 收口时 smoke_data_layer 11/11（迁移修复前后各跑一轮）
