# Stage 28 — K8s 可观测四件套（Prometheus + Grafana + Loki + Alertmanager）

> 本阶段目标：在 Stage 27 已跑通的本地 kind 集群之上，把"系统现在怎么样 / 哪里出错 / 哪个 Pod 卡了"全部可视化 + 可告警。
> 沿用 Stage 27 的 17 子 chart 惯例，本次新增 4 个独立子 chart（**prometheus / grafana / loki / alertmanager**），全部使用 Helm 3 umbrella pattern 编排。

## 一、目标与范围（与 Stage 27 一致的不变量）

- ✅ **不修改任何业务 Go 代码**
- ✅ **不动 docker-compose**（`deploy/` 保留作为 fallback）
- ✅ **TDD 强制**：每个 PR 一个 Red→Green 循环（先写失败 render-assert 测试 → 写最小 chart → 测试绿）
- ✅ **本地 kind 优先**，不依赖任何云服务
- ✅ **学习阶段 Secret 占位符**（Slack / 钉钉 webhook 留 `dev-webhook-placeholder.invalid`）

## 二、新增 4 个子 chart（Stage 28-A → 28-D）

| 子 chart | ns | 端口 | 用途 | 关键文件数 |
|--------|----|------|------|----------|
| `prometheus` | ee-observability | 9090 | metrics 采集 + 存储（15d retention） | 8 |
| `grafana` | ee-observability | 3000 | 可视化 + dashboard（sidecar 自动加载） | 9 |
| `loki` | ee-observability | 3100 | 日志聚合（单节点 filesystem） | 9 |
| `alertmanager` | ee-observability | 9093 | 告警 webhook（dev placeholder） | 7 |
| `promtail` (随 loki) | ee-observability | — | DaemonSet 日志采集 | (在 loki chart 内) |

每个子 chart 都遵循 Stage 27 已确立的模板结构：
```
charts/<name>/
├── Chart.yaml          # apiVersion v2, version 0.1.0
├── values.yaml         # 镜像 / 端口 / 资源
└── templates/
    ├── _helpers.tpl    # namespaceObservability / labels / selectorLabels
    ├── deployment.yaml (or statefulset.yaml / daemonset.yaml)
    ├── service.yaml    # ClusterIP:<port>
    ├── configmap.yaml  # *.yml 配置
    ├── secret.yaml     # webhook / admin 密码（占位符）
    ├── pvc.yaml        # 仅 prometheus / loki
    └── serviceaccount.yaml (仅 prometheus)
```

### 关键 helper pattern（Stage 27 已沉淀 → Stage 28 复用）

```yaml
{{- define "prometheus.namespace" -}}
{{- $key := .key -}}
{{- $fallback := .default -}}
{{- if and .Values .Values.namespaces (index .Values.namespaces $key) -}}
{{- index .Values.namespaces $key -}}
{{- else -}}
{{- $fallback -}}
{{- end -}}
{{- end -}}

{{- define "prometheus.namespaceObservability" -}}
{{- include "prometheus.namespace" (dict "key" "observability" "default" "ee-observability" "Values" .Values) -}}
{{- end -}}
```

**为什么不能用 `default`**：Go template 的 `default` 不能拦截 nested dict access panic——`.Values.namespaces.observability` 会在 `default` 介入前就先 panic。所以必须显式写 `if and .Values .Values.namespaces (index ...)` 模式。

## 三、辅助改动（Stage 28-E）

| 改动点 | 文件 | 原因 |
|--------|------|------|
| 6 个业务 svc 加 `prometheus.io/scrape: "true"` + `prometheus.io/port: "<port>"` 注解 | `charts/.../{user,chat,analytics,assessment}-svc/templates/deployment.yaml` + ai-svc / llm-service | 让 Prometheus 通过 kubernetes_sd_config role: pod 主动发现 metrics 端点（无需 ServiceMonitor CRD） |
| SkyWalking OAP 启用自监控 `:1234` | `charts/.../skywalking/templates/oap.yaml` | 让 Prometheus 能 scrape OAP 内部 telemetry 指标 |
| umbrella Chart.yaml 加 4 个 dependency | `charts/emotion-echo/Chart.yaml` | 让 `helm install` 自动带上 4 个子 chart |
| umbrella values.yaml 加 4 个 `enabled: false` 默认值 | `charts/emotion-echo/values.yaml` | 关闭默认（节省资源），按需在 overlay 中开启 |
| 新增 values-dev.yaml 覆盖（4 件套全开） | `charts/emotion-echo/values-dev.yaml` | dev 环境一键启用 |
| 新增 values-prod.yaml 模板（retention / 资源） | `charts/emotion-echo/values-prod.yaml` | prod 环境样板（webhook / password 走 ExternalSecret） |

## 四、TDD 循环拆分（每 PR 一个 cycle，符合 AGENTS.md § 二）

| PR | 内容 | 测试数 | commit 数 |
|----|------|--------|----------|
| Stage 28-A | prometheus 子 chart + scrape config | 4 (2 render + 2 lint) | 3 (test+feat+refactor) |
| Stage 28-B | grafana 子 chart + dashboard sidecar | 2 | 2 |
| Stage 28-C | loki + promtail DaemonSet | 2 | 2 |
| Stage 28-D | alertmanager 子 chart + webhook Secret | 2 | 2 |
| Stage 28-E | 6 业务 svc annotations + skywalking :1234 | 2 | 2 |
| Stage 28-F | umbrella 集成 + values-prod + 本文档 | 0 新增（沿用已有） | 1 |

**全部 16 个 render-assert 测试 PASS**（耗时 ~10s，远低于 5s/测试的硬约束）。

## 五、scrape 配置（prometheus.yml 核心）

```yaml
scrape_configs:
  - job_name: kubernetes-pods
    kubernetes_sd_configs: [{ role: pod }]
    relabel_configs:
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_scrape]
        action: keep
        regex: "true"
      - source_labels: [__meta_kubernetes_pod_annotation_prometheus_io_port]
        action: replace
        target_label: __address__
        regex: ([^:]+)(?::\d+)?;(\d+)
        replacement: $1:$2
      - source_labels: [__meta_kubernetes_namespace]
        action: replace
        target_label: namespace
      - source_labels: [__meta_kubernetes_pod_name]
        action: replace
        target_label: pod
  - job_name: skywalking-oap
    static_configs:
      - targets: ['skywalking-oap.ee-data.svc.cluster.local:1234']
  - job_name: apisix
    static_configs:
      - targets: ['apisix-gateway.ee-system.svc.cluster.local:9091']
    metrics_path: /apisix/prometheus/metrics
  - job_name: prometheus-self
    static_configs: [{ targets: ['localhost:9090'] }]
```

## 六、本地验证（kind 集群）

```bash
# 1. kind 灌镜像（一次性）
kind load docker-image prom/prometheus:v2.54.0 grafana/grafana:11.2.0 \
                    grafana/loki:3.2.0 grafana/promtail:3.2.0 \
                    prom/alertmanager:v0.27.0 --name ee-dev

# 2. helm install（dry-run 先验）
helm install ee ./charts/emotion-echo -f ./charts/emotion-echo/values-dev.yaml --dry-run

# 3. 真正部署
helm install ee ./charts/emotion-echo -f ./charts/emotion-echo/values-dev.yaml

# 4. port-forward 三个端口
kubectl port-forward -n ee-observability svc/prometheus 9090:9090
kubectl port-forward -n ee-observability svc/grafana 3000:3000
kubectl port-forward -n ee-observability svc/alertmanager 9093:9093
```

### 截图位置（待 smoke 后补）
- `docs/stage-28-observability/grafana-home.png` — Grafana 启动后默认 dashboard
- `docs/stage-28-observability/prometheus-targets.png` — 16 个 scrape target 全绿
- `docs/stage-28-observability/loki-explore.png` — Loki 日志查询
- `docs/stage-28-observability/alertmanager-status.png` — Alertmanager 状态

## 七、DoD（完成定义）

| 项 | 状态 | 证据 |
|----|------|------|
| `helm lint ./charts/emotion-echo` 全绿 | ✅ | `1 chart(s) linted, 0 chart(s) failed` |
| `helm template ee ./charts/emotion-echo` 输出 ≥ 4 个新 Deployment（prometheus/grafana/loki/alertmanager） | ✅ | 实测 16 Deployments + 4 StatefulSets + 1 DaemonSet |
| `go test -tags k8s ./k8s/tests/...` 全绿 | ✅ | 16/16 PASS in ~10s |
| 6 个业务 svc deployment 都有 `prometheus.io/scrape: "true"` annotation | ✅ | render-assert `TestStage28E_BusinessMetricsAnnotations` |
| SkyWalking OAP 暴露 `:1234` metrics 端点 | ✅ | render-assert `TestStage28E_SkyWalkingOAPExposesMetrics` |
| Grafana 启动后能打开 | ⏳ smoke | dev cluster 上 port-forward 待补 |
| docs/stage-28-observability.md 交付 | ✅ | 本文档 |

## 八、关键决策记录

| 决策 | 理由 |
|------|------|
| 4 个独立子 chart（不是 1 个 observability 子 chart） | 符合 Stage 27 既定的 17 子 chart 惯例，可独立开关 + 升级 |
| 启用 SkyWalking OAP `:1234` 自监控 | 让 Prometheus 能 scrape OAP 内部 telemetry（CPU / 内存 / JVM） |
| 用 pod annotations 发现 metrics（不用 ServiceMonitor CRD） | 减少外部依赖（不装 prometheus-operator CRD）；annotations 简单可读；社区标准做法 |
| Grafana dashboard 用 ConfigMap + label 自动加载 | 零运维（sidecar 1.0+ 标准做法）；dashboard 跟 chart 一起走 git |
| Loki 用 Promtail DaemonSet 采集 | 标准做法；同 chart 出，自包含；不依赖额外插件 |
| Alertmanager webhook 用 Secret + values 占位符 | 学习阶段策略（与 Stage 27 一致）；生产换 ExternalSecret |
| Namespace 用 `ee-observability`（Stage 27 已建） | 不新建 ns；4 个 observability 组件物理隔离 |
| values.yaml 默认 `enabled: false` | 默认不开（节省 ~700MB RAM）；overlay 按需开启 |

## 九、不在 Stage 28 范围（留给 Stage 29+）

- ❌ cert-manager / HTTPS（Grafana 8080 用 NodePort 暴露即可）
- ❌ Production Slack / 钉钉 webhook 真配置（用占位符）
- ❌ 长期存储（minio / s3）—— Stage 28 用 local fs
- ❌ 多 region Prometheus federation
- ❌ PagerDuty / OpsGenie 集成
- ❌ PromQL alert rules 业务侧定义（HighErrorRate / PodOOMKilled 等 plan 已列出，留 Stage 29）

## 十、风险与缓解

| 风险 | 等级 | 缓解 |
|------|------|------|
| Prometheus 1GB+ 内存占用 | 中 | dev 默认 `--storage.tsdb.retention.time=1d`；prod 15d + PVC 50Gi |
| Grafana / Loki PV 在 kind 重启丢数据 | 低 | dev 用 local fs（学习阶段），prod 切 PVC + StatefulSet |
| Promtail DaemonSet 在 kind 单节点 OK | 低 | 单节点 DaemonSet = 1 Pod，无问题 |
| APISIX 9091 admin port 暴露给 Prometheus scrape | 低 | `9091` 本来就在 Service 里，Prometheus 通过 svc FQDN 访问 |
| SkyWalking OAP 启用自监控会多消耗 ~50MB 内存 | 低 | resources.limits.memory 已留余量 |

## 十一、commit 清单（按时间倒序）

```
ce932df feat(stage-28-E): add prometheus.io scrape annotations to 6 business svcs + expose skywalking-oap :1234 metrics
25b89ad test(stage-28-E): add render-assert RED gate for business svc annotations + skywalking :1234
26f9ef5 feat(stage-28-D): alertmanager 子 chart + umbrella dependency (Stage 28-D GREEN)
5e5ccf0 test(stage-28-D): add render-assert RED gate for alertmanager subchart
9c5986a feat(stage-28-C): loki + promtail subchart GREEN
... (Stage 28-A / 28-B earlier commits)
```

## 十二、学习产物（已落地）

- `docs/learn/12-stage-28-roadmap.md` — Stage 28 路线图（已落地时勾选）
- `docs/learn/10-tdd-for-k8s.md` — render-assert 测试模式（Stage 28-A 验证过）
- `docs/learn/11-compose-to-k8s.md` — 12 个字段 docker-compose → K8s 映射（含 prometheus.io 注解）

---

**关键不变量重申**：
1. 整个过程**不修改任何业务代码**
2. **docker-compose 路径不删**
3. **TDD 净增**：每个 PR 先写失败测试
4. 所有 webhook / 密码走 Secret + values 占位符
5. 沿用 Stage 27 的 17 子 chart 惯例（4 个独立子 chart）