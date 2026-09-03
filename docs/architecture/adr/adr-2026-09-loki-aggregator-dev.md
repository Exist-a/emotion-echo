# ADR · 2026-09 · dev 模式日志聚合后端 = Loki（filesystem 单节点）

> **本文档仅约束 dev compose 路径的日志聚合后端选型**。prod 长期存储（Loki chunk store 切 S3 / minio / 替换为 EFK）留待独立 ADR，本文不展开。
>
> **决策状态**：✅ **Accepted**（2026-09-04，本 ADR 落地）
> **实施状态**：🚧 待 plans/observability-compose-gap.md 执行
> **关联**：[plans/observability-compose-gap.md](../../plans/observability-compose-gap.md) §3.2

---

## 一、上下文

Stage 28 在 k8s 路径已完整交付可观测性四件套（[STAGE-28-LANDING.md](../../stages/STAGE-28-LANDING.md)），其中 `charts/loki` 子 chart 使用 **Loki 3.2.0 + Promtail DaemonSet**（[charts/emotion-echo/charts/loki/values.yaml](../../../charts/emotion-echo/charts/loki/values.yaml)）。dev compose 路径 [deploy/docker-compose.infra.yml](../../../deploy/docker-compose.infra.yml) 当前**未装** Loki / Promtail——APISIX access_log 写到容器内 `logs/access.log`（[deploy/apisix/config.yaml:269](../../../deploy/apisix/config.yaml#L269)），无 volume mount、无外部聚合。

### 1.1 决策驱动

- [architecture/decisions.md:101](../decisions.md) 写 "Loki + Grafana 或 ELK" 是模糊意向，不是决策
- [docs/plans/observability-compose-gap.md](../../plans/observability-compose-gap.md) §3.2 需要给 dev 日志聚合定后端
- 不约束 prod 长期存储，避免本次 plan scope 蔓延

### 1.2 候选

| 维度 | Loki + Promtail（filesystem 单节点）| EFK（Elasticsearch + Fluentd + Kibana）| 直接 docker logs |
|------|--------------------------------------|------------------------------------------|------------------|
| 与 k8s 路径一致性 | ✅ 完全一致（镜像版本、retention、config 模式） | ❌ 全新引入另一套栈 | ✅ 零依赖 |
| dev compose 启动成本 | 🟡 中（多 1 个服务 + Promtail 需 docker.sock） | 🔴 高（3 个服务 + JVM 内存） | ✅ 零成本 |
| 查询能力 | 🟡 LogQL（标签过滤，弱全文检索）| 🟢 强全文检索 | ❌ 无 |
| 资源占用 | 🟢 低（Loki 单节点 ~128Mi 内存）| 🔴 高（ES JVM 至少 1GB）| ✅ 零 |
| 与 Grafana 集成 | ✅ 已有 [Stage 28-B grafana sidecar](../../stages/STAGE-28-LANDING.md) | ⚠️ 需另接 | ❌ 无 |
| 标签 / 多租户 | ✅ 第一公民 | ⚠️ 弱 | ❌ 无 |

---

## 二、决策

**dev compose 路径日志聚合后端 = Loki 3.2.0 + Promtail DaemonSet-style（filesystem 单节点模式）**。

镜像与版本与 [charts/emotion-echo/charts/loki/values.yaml](../../../charts/emotion-echo/charts/loki/values.yaml) 严格对齐，便于 dev/k8s 路径行为一致。

### 2.1 范围

| 包含 | 不包含 |
|------|--------|
| ✅ Loki 单节点 filesystem 模式（与 [loki.yml 模板](../../../charts/emotion-echo/charts/loki/templates/configmap.yaml) 一致）| ❌ prod 长期存储（S3 / minio / 其它 chunk store） |
| ✅ Promtail 采集 docker container stdout（用 `/var/lib/docker/containers` 卷或 docker.sock）| ❌ Loki 多副本 / 集群模式 |
| ✅ retention 1d（与 k8s 路径 values-dev.yaml 一致）| ❌ ELK / 其它日志栈对比 |
| ✅ 与 Grafana datasource sidecar 集成 | ❌ 跨 region 聚合 |
| ✅ APISIX access_log 通过 file-logger 插件落盘 + Promtail 推 Loki | |

### 2.2 命名 / 镜像 / 端口

| 项 | 值 | 来源 |
|----|----|------|
| 镜像 | `grafana/loki:3.2.0` | [charts/loki/values.yaml:3-5](../../../charts/emotion-echo/charts/loki/values.yaml#L3-L5) |
| Promtail 镜像 | `grafana/promtail:3.2.0` | 同上 |
| Loki 端口 | `3100` | [charts/loki/values.yaml:14-16](../../../charts/emotion-echo/charts/loki/values.yaml#L14-L16) |
| retention | 1d | [values-dev.yaml:43](../../../charts/emotion-echo/values-dev.yaml#L43) |
| 数据卷 | `loki_data`（compose volume）| 类比 [loki/pvc.yaml](../../../charts/emotion-echo/charts/loki/templates/pvc.yaml) |

### 2.3 Promtail 部署形态

- k8s 路径用 **DaemonSet**（[charts/loki/templates/daemonset-promtail.yaml](../../../charts/emotion-echo/charts/loki/templates/daemonset-promtail.yaml)）每节点采集
- compose 路径：单节点机器上跑 **1 个 promtail service** 即可；用 `docker.sock:ro` 列出所有 container log 文件（路径 `/var/lib/docker/containers/<id>/*.log`）

---

## 三、与既有决策的关系

| 既有决策 | 影响 |
|----------|------|
| [docs/architecture/decisions.md:101](../decisions.md) "Loki + Grafana 或 ELK" | **部分承接**——dev 单节点定 Loki；prod 长期存储留作 ADR-2 |
| Stage 28 k8s 路径 Loki 选型 | **完全对齐**——镜像版本、retention、Promtail 模式全部复用 |
| Stage 28 业务 svc `prometheus.io/scrape: "true"` annotation | 不影响（日志独立维度）|

---

## 四、风险与缓解

| 风险 | 缓解 |
|------|------|
| Promtail 需要 docker.sock 权限 | dev compose `volumes: - /var/run/docker.sock:/var/run/docker.sock:ro`；明示 host 风险（容器逃逸），仅 dev 用 |
| Loki 单节点脑裂（dev） | dev 单机足够，无 HA；prod 走 3 节点集群（未来 ADR-2）|
| log 查询能力弱（LogQL 不如 ES 全文检索）| dev 调试够用；prod 全文检索需求另开 ADR 评估 EFK |
| retention 1d 太短 | dev 默认即可；prod retention 配置走 values-prod 模板（与 [values-prod.yaml:27](../../../charts/emotion-echo/values-prod.yaml#L27) 一致）|

---

## 五、未来 ADR 候选

- **ADR-2**：prod Loki 长期存储（S3 / minio）chunk_store_config 选型
- **ADR-3**：日志聚合迁移到 EFK 的触发条件与判定标准（如需全文检索 / 跨 region 聚合 / 合规审计）
- **ADR-4**：跨服务 trace sampling 策略（SkyWalking 默认 100% 太重）

---

> 撰写时间：2026-09-04
> 关联 plan：[plans/observability-compose-gap.md](../../plans/observability-compose-gap.md)
> 关联 stage：[STAGE-28-LANDING.md](../../stages/STAGE-28-LANDING.md) §三