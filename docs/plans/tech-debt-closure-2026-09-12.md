---
status: planned
priority: high
created: 2026-09-12
owner: User
related-decisions:
  - decisions.md 决策 3（K8s 备好不部署）
  - decisions.md 决策 11（APISIX 网关唯一业务入口）
related-plans:
  - docs/plans/kafka-reliability-gaps.md §1.4
  - docs/plans/todo-pile-2026-09-04.md §D6
related-stages:
  - stage-73-kafka-protobuf-migration-2026-09-12.md §五（open 清单来源）
---

# Plan — 技术债收尾批次（2026-09-12，Stage 74 执行源）

> 来源：Stage 73 收口报告 §五 open 清单 + 决策 4 ADR §八。
> 本计划把 3 组技术债收拢执行：Grafana lag 面板确认、dev 栈 web 前端容器、Helm 残余处置。

## §A 假设清单（与现状核实对比）

| 假设 | 核实结果 |
|------|---------|
| Grafana lag 面板"provisioning 待查" | 面板 JSON / provider / 双 volume 挂载 / smoke 断言 8-10 均已落地（Stage 44 PR-OBS-7）；唯一缺口 = datasource 未写显式 `uid: prometheus` 而面板 JSON 硬编码该 uid |
| web 容器未起 = node:20-alpine 拉取受限 | 本地无 `emotion-echo/web:v0.1.0` 镜像，build 第一步 `FROM node:20-alpine` 被 Docker Hub 匿名限流卡住（同 stage-30-B / stage-59 先例） |
| Helm 残余 5 项是 open backlog | **决策 3（2026-09-07 收口）已定 K8s「备好不部署」**，chart 仅为学习资产 → 残余冻结不处置（用户 2026-09-12 拍板：冻结 + 纠偏 + 停更声明） |
| apiBaseUrl 指向需修 | 无需修：compose web 指向 APISIX 宿主映射 `http://localhost:19080/api/v1`，与决策 11 一致；仅 Helm chart 侧 web 指 BFF（随冻结不处置） |

## 执行批次

### A. Grafana lag 面板确认（~2h）

- RED：`scripts/smoke_observability.py` 新增断言——Prometheus datasource uid 必须为 `prometheus`（当前必失败）
- GREEN：`deploy/grafana/provisioning/datasources/datasource.yaml` 加 `uid: prometheus`（Loki 同理核查 dashboard JSON 引用）
- 起 obs 栈触发业务事件，浏览器 GUI 确认 3 panel（ai-svc lag / analytics-svc lag / DLQ stat）渲染正常
- `python scripts/smoke_observability.py` 全绿；回写 kafka-reliability-gaps.md §1.4

### B. dev 栈 web 前端容器（~2h）

- 运维动作：国内镜像源 pull `node:20-alpine` 后 retag（不改 Dockerfile）
- RED：`scripts/test_build_dev_images.sh` 断言 `ALL_SVCS` 含 `emotion-echo-web`、默认 `BASE_IMAGES` 含 `node:20-alpine`
- GREEN：`scripts/build_dev_images.sh:24-26,36` 补默认值
- build + 起 web 容器，验证 :3000 可访问、经 APISIX 19080 API 联通、healthcheck 通过

### C. Helm 冻结处置（~1.5h，纯文档 + 注释）

- decisions.md 登记冻结决策：5 项残余（NACOS_ADDR 硬编码 / web 绕过网关 / xtts 镜像源 / 缺 MinIO·db-migrate·apisix-seed chart / values-prod 停更）在 K8s 不启用期内冻结，重启条件 = 多机迁移启动
- 纠偏 `charts/emotion-echo/Chart.yaml:13`、`values.yaml:30,62` 过期"APISIX 已退役"注释
- `values-prod.yaml` 头部加停更声明（停更于 Stage 28-F，指向决策 3）
- 回写 backlog-order 项 4 / todo-pile §D6 指向冻结决策
- 验证 `helm lint` 仍 0 failed

### 收尾

- dev 栈全量拉起跑 `python scripts/smoke_data_layer.py`（§契约 1-6 回归，预期 11/11）
- 写 `docs/stages/stage-74-tech-debt-closure-2026-09-12.md` 收口报告（Stage 73 §五 open 逐项销账）
- 本计划落地后迁 `docs/legacy-plans/landed/`

## 调研依据

- 已读：deploy/grafana/provisioning/datasources/datasource.yaml、deploy/grafana/dashboards/kafka-consumer-lag.json、scripts/smoke_observability.py、scripts/build_dev_images.sh、emotion-echo-web/{Dockerfile,package.json,nuxt.config.ts}、deploy/{docker-compose.apps.yml,docker-compose.infra.yml,compose.dev.yml}、charts/emotion-echo/{Chart.yaml,values.yaml,values-prod.yaml}、docs/architecture/decisions.md
- 已查：kafka-reliability-gaps.md §1.4、observability-sprint-b.md PR-OBS-7、stage-73 §五、stage-72 项 4、stage-59（镜像拉取受限先例）、stage-36（Bug 9 npm registry）
- 用户决策（2026-09-12）：Helm 残余冻结 + 纠偏 + 停更声明；A/B 照做；node:20-alpine 走镜像源 retag
