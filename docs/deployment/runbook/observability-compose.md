# Observability dev compose 运维手册（Ops Runbook）

> **目标读者**：dev compose 维护者 / QA / 后续开发者
> **目的**：记录 PR-OBS-1~7 落地的 dev compose 可观测性三层（metrics + logs + traces）
> + Kafka consumer lag 监控，让任何接手者能在 5 分钟内上手
> **配合文档**：[observability-sprint-b.md](/docs/plans/observability-sprint-b.md)（plan）/
> [stage-43-kafka-reliability-sprint-a.md](/docs/stages/stage-43-kafka-reliability-sprint-a.md)（Kafka Sprint A 收口）

---

## 一、启动后第一件事

### 1.1 启动命令

```bash
# 单条命令起 dev compose + obs profile（含 prometheus/grafana/loki/promtail/kafka-exporter）
cd deploy
COMPOSE_PROFILES=obs docker compose \
  -f docker-compose.infra.yml \
  -f docker-compose.apps.yml \
  up -d

# 等 30-60s 让 scrape + provisioning 完成
sleep 60
```

### 1.2 验收清单（5 分钟内可走完）

| # | 检查 | 命令 | 期望 |
|---|------|------|------|
| 1 | 容器健康 | `docker ps --format "{{.Names}}\t{{.Status}}" \| grep -E 'obs\|bff\|chat\|user'"` | 6 个 obs 容器 + 6 个业务 svc healthy |
| 2 | Prometheus 就绪 | `curl :9090/-/ready` | "Prometheus Server is Ready." |
| 3 | Grafana 就绪 | `curl :3000/api/health` | `{"database":"ok"}` |
| 4 | Loki 就绪 | `curl :3100/ready` | "ready"（注意：loki /ready 在 compactor 启动后 ~15s 才返回 200）|
| 5 | Kafka exporter 就绪 | `curl :9308/metrics` | body 含 `kafka_consumergroup_lag` |
| 6 | 跑全 smoke | `python scripts/smoke_observability.py` | 11 项 PASS + 1 项 FAIL（prometheus scrape targets，需干净环境） |

### 1.3 UI 入口速查

| 组件 | URL | 默认凭证 |
|------|-----|----------|
| Grafana | http://localhost:3000 | admin / admin（dev 默认，**prod 必须改**）|
| Prometheus | http://localhost:9090 | 无（dev 无鉴权）|
| Alertmanager | （PR-OBS-8 范围外，dev 未启用）| — |
| SkyWalking UI | http://localhost:8080 | 无 |
| APISIX Dashboard | http://localhost:9180/ui/ | admin key 在 `deploy/apisix/config.yaml` |

---

## 二、看 targets（Prometheus 视角）

### 2.1 列出所有 scrape target 状态

```bash
curl -s http://localhost:9090/api/v1/targets?state=active | python -m json.tool | grep -E '"instance"|"health"' | head -20
```

### 2.2 期望的 scrape job（PR-OBS-4 + PR-OBS-7）

| Job | Targets | 用途 |
|-----|---------|------|
| `emotion-echo-services` | 6 业务 svc | HTTP /metrics |
| `apisix` | emotion-echo-apisix:9091 | APISIX 自 metrics |
| `skywalking-oap` | emotion-echo-sw-oap:1234 | OAP 自 metrics（**注意**：sw-oap env 未设 SW_TELEMETRY=prometheus，**DOWN**，留作 PR-OBS-? 启用）|
| `prometheus` | localhost:9090 | 自监控 |
| `kafka-exporter` | emotion-echo-kafka-exporter:9308 | Kafka consumer lag（PR-OBS-7）|

### 2.3 排查 scrape target DOWN

| 现象 | 根因 | 处置 |
|------|------|------|
| 业务 svc target DOWN | 容器 Restarting 或 Nacos 注册失败 | `docker logs <svc>` 查 Nacos 500（Stage 39 §二同源问题）|
| skywalking-oap DOWN | OAP 未启 SW_TELEMETRY | **接受**（dev 不需要）|
| kafka-exporter DOWN | 容器未起或 KAFKA_BROKERS 错 | `docker logs emotion-echo-kafka-exporter` |
| apisix DOWN | apisix-seed 失败 / Nacos 上游 503 | `docker logs emotion-echo-apisix` |

---

## 三、看 logs（Loki 视角）

### 3.1 直接查 Loki API

```bash
# 所有 apisix access.log (需触发 APISIX 请求后才有数据)
curl -sG http://localhost:3100/loki/api/v1/query \
  --data-urlencode 'query={job="apisix"}' | python -m json.tool

# 限制最近 5 分钟
curl -sG http://localhost:3100/loki/api/v1/query \
  --data-urlencode 'query={job="apisix"}' \
  --data-urlencode 'limit=100' | python -m json.tool
```

### 3.2 通过 Grafana Explore

1. 打开 http://localhost:3000/explore
2. datasource 选 **Loki**
3. query: `{job="apisix"}`
4. 时间范围：Last 15 minutes

### 3.3 Promtail 采集源（PR-OBS-5）

| 来源 | 路径 | 说明 |
|------|------|------|
| APISIX access.log | `/var/log/apisix-access.log` (容器内) | PR-OBS-1 file-logger 落盘 + PR-OBS-5 volume mount 到 host `./tmp/apisix-access.log` |
| 业务 svc stdout | （未采集，PR-OBS-? 范围）| docker sd 模式未来扩展 |

### 3.4 常见 Loki 问题

| 现象 | 根因 | 处置 |
|------|------|------|
| query 返空 `{job="apisix"}` | 无 APISIX 触发或 apisix 未起 | 访问任意 `/api/v1/*` 触发；查 apisix 容器状态 |
| `file-logger` 报错 | APISIX plugin 配置错（PR-OBS-1 89b0117）| `curl :9180/apisix/admin/routes/100 -H "X-API-KEY: <key>"` 查 plugins.file-logger |
| promtail 一直空跑 | mount 路径错 | `docker exec emotion-echo-promtail ls -la /var/log/` |

---

## 四、看 trace（SkyWalking 视角）

### 4.1 通过 SkyWalking UI

1. 打开 http://localhost:8080
2. 左侧菜单 → **Trace** 或 **Topology**
3. **Topology** 应显示 `emotion-echo-web-bff → emotion-echo-chat-svc` 边
   - **注意**：dev 环境 5 业务 svc 因 Nacos Restarting 不一定能完整看到 trace
4. **Trace** 列表按时间倒序，点开看完整 span 链

### 4.2 通过 trace_id 跨服务查询

```bash
# 从 BFF access.log 或日志里提取 trace_id（SW 格式：{segment_id}-{span_id}-{service}）
# 然后到 SkyWalking UI 的 Trace 页面粘贴 trace_id 查询
```

### 4.3 PR-OBS-2 改进点（已落地）

- 7 svc 统一用 `shared/bootstrap.BootstrapSkyWalkingTracer`（不再静默吞错）
- `STARTUP_STRICT=true` env 启用 fail-fast（默认 dev 关闭）
- `STARTUP_STRICT_DEPS=postgres,kafka,skywalking,llm` 控制哪些依赖触发 fail-fast
- counter `emotion_echo_skywalking_init_failed_total{service}` 暴露给 Prometheus

### 4.4 常见 trace 问题

| 现象 | 根因 | 处置 |
|------|------|------|
| 看不到 trace | OAP 不可达 + 5 svc tracer init 静默失败（PR-OBS-2 修复前）| `curl :9090/metrics \| grep skywalking_init_failed_total` 验证 |
| 跨服务 trace 不连续 | 中间 svc 未挂 GinSkywalkingMiddleware | 查 main.go: `grep GinSkywalkingMiddleware` |
| go2sky dial fail | sw-oap 端口错（PR-OBS-1 已修 host + port）| `curl emotion-echo-sw-oap:11800` 验证 gRPC |

---

## 五、故障排查

### 5.1 dev compose 启动后整体不健康

按"先看 metrics、再看 logs、最后看 trace"顺序排查：

```bash
# 1. 看容器整体健康
docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

# 2. 看 5 业务 svc 是否 Restarting（最常见 — Nacos 500）
for svc in user chat assessment analytics ai-svc; do
  echo "=== emotion-echo-$svc ==="
  docker logs --tail 5 emotion-echo-$svc 2>&1 | grep -E "nacos|Register"
done

# 3. 看 obs 容器健康
docker ps --format "{{.Names}}\t{{.Status}}" | grep -E 'prometheus|grafana|loki|promtail|kafka-exporter'
```

### 5.2 Prometheus target 全部 DOWN

```bash
# 1. prometheus 容器内能否访问 target?
docker exec emotion-echo-prometheus wget -qO- http://emotion-echo-web-bff:8894/metrics | head -5

# 2. 网络层检查（容器 DNS）
docker exec emotion-echo-prometheus nslookup emotion-echo-web-bff
```

### 5.3 Loki 查不到 access.log

```bash
# 1. promtail 是否启动
docker logs emotion-echo-promtail --tail 20

# 2. host 上 ./tmp/apisix-access.log 是否有数据
ls -la tmp/apisix-access.log
tail -1 tmp/apisix-access.log

# 3. APISIX 是否真在写（触发请求）
curl http://localhost:19080/api/v1/health  # 经 APISIX 触发 access.log
ls -la tmp/apisix-access.log
```

### 5.4 Kafka consumer lag 告警

```bash
# 1. 看 kafka-exporter :9308/metrics 是否含 lag
curl -s :9308/metrics | grep kafka_consumergroup_lag

# 2. 看 prometheus alert 状态
curl -s :9090/api/v1/alerts | python -m json.tool

# 3. 看具体 consumer lag
curl -sG :9090/api/v1/query \
  --data-urlencode 'query=kafka_consumergroup_lag{consumergroup=~"ai-svc|analytics-svc"}'
```

### 5.5 dev compose down + 重启丢失 volume

**不要 `down -v`**（带 `-v` 会删 postgres_data / kafka_data / prometheus_data / grafana_data / loki_data）：

```bash
# 安全 down（保留 volume）
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml --profile obs down

# 完全清理（数据丢失，慎用）
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml --profile obs down -v
```

### 5.6 决策树（症状 → 根因 → 处置）

```
症状: smoke_observability.py 多个 FAIL
  ├─ prometheus scrape targets FAIL
  │   ├─ 全部 DOWN → prometheus 容器问题 → docker logs emotion-echo-prometheus
  │   ├─ 部分业务 svc DOWN → Nacos Restarting → Stage 39 §二根因,接受(环境问题)
  │   └─ skywalking-oap DOWN → OAP 未启 SW_TELEMETRY,接受
  ├─ grafana health FAIL → grafana 未启 → docker logs emotion-echo-grafana
  ├─ loki ready FAIL → loki compactor 未就绪 → 等 30s 重试
  ├─ kafka-exporter unreachable → 容器未起 → docker ps 含 emotion-echo-kafka-exporter
  └─ apisix access.log FAIL → apisix container restarting → docker inspect emotion-echo-apisix
```

---

## 六、参考资料

- [observability-sprint-b.md](/docs/plans/observability-sprint-b.md)（Sprint B 完整 plan）
- [stage-43-kafka-reliability-sprint-a.md](/docs/stages/stage-43-kafka-reliability-sprint-a.md)（Kafka Sprint A 收口 + §1.4 lag 监控尾巴）
- [stage-35-ops-runbook.md](/docs/deployment/runbook/stage-35-ops-runbook.md)（参考 runbook 风格）
- [stage-34-ops-runbook.md](/docs/deployment/runbook/stage-34-ops-runbook.md)
- [adr-2026-09-loki-aggregator-dev.md](/docs/architecture/adr/adr-2026-09-loki-aggregator-dev.md)（dev Loki 选型）
- [scripts/smoke_observability.py](/scripts/smoke_observability.py)（12 项断言完整定义）
- [AGENTS.md §〇 §二 §2.4](/AGENTS.md)（TDD + 数据契约验收）

---

## 七、版本

- 创建：2026-09-08（Sprint B PR-OBS-8）
- 配合 commit：feat/observability-OBS-8-runbook
- 下一版：PR-OBS-9~16 测试护栏落地后补 metrics 契约测试章节
