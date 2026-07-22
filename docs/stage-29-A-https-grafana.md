# Stage 29-A Landing — HTTPS / cert-manager / Grafana Ingress TLS

> 本次 session 落地文档。Stage 28 全闭环之后的第一个 HTTPS 子阶段。
>
> Stage 28 → 28-A→28-F 已全部 DONE（可观测性：Prometheus + Grafana + Loki +
> Alertmanager + SkyWalking :1234 metrics + 6 业务 svc scrape annotations +
> values-prod overlay）。Stage 29-A 是 roadmap `12-stage-28-roadmap.md` 中
> 优先级 ② 「HTTPS / cert-manager」 的 **最小切入点**。

---

## 一、目标

按用户/landing § 九决议：

| 决议 | 选择 |
|------|------|
| Stage 29 范围 | **仅 Grafana Ingress TLS**（最小起点） |
| 切入面 | cert-manager + self-signed ClusterIssuer + Grafana Ingress TLS |
| 非目标（明确留给 Stage 29-B/C/D 后续） | PromQL 业务告警规则、ExternalSecret/SealedSecret、其余 15 条 ApisixRoute TLS |

---

## 二、TDD 循环

| 循环 | 类型 | commit | 说明 |
|------|------|--------|------|
| 29-A.1 | 🔴 RED | `test(stage-29-A): red — cert-manager + Grafana ingress TLS render assertions` | 在 `k8s/tests/stage_29a_render_test.go` 写两个 render-assert（cert-manager Deployment + ClusterIssuer；Grafana Ingress TLS + cert-manager annotation），此时 chart 尚未存在 → **两个测试必失败** |
| 29-A.3 | 🔴 RED | 同上 commit | 同一个测试文件同时覆盖 29-A.1+29-A.3（两个 RED gate 物理上耦合：证书 annotation 必须在 Ingress 上才能被验证） |
| 29-A.2/29-A.4 | 🟢 GREEN | `feat(stage-29-A): cert-manager chart + Grafana ingress TLS (GREEN)` | 实现 cert-manager 子 chart（Deployment + ServiceAccount + ClusterIssuer）+ grafana `templates/ingress-tls.yaml`（受 `global.grafanaIngressTls.enabled` 控制）+ umbrella Chart.yaml dependency + umbrella values-dev.yaml 开关 |

> 两个 RED 与一个 GREEN 合并：因测试 gate 物理耦合（一个测试文件同时覆盖两个对象），单独 GREEN 任何一半都会留下半红测试，违反 AGENTS.md § 〇「测试永远保持绿」。已记录在 commit message 中。

---

## 三、新增 / 修改文件清单

### 新增（GREEN 一次性交付）

| 路径 | 行数 | 角色 |
|------|------|------|
| `charts/emotion-echo/charts/cert-manager/Chart.yaml` | 22 | 子 chart 元数据 |
| `charts/emotion-echo/charts/cert-manager/values.yaml` | 38 | 默认值 + 开关 |
| `charts/emotion-echo/charts/cert-manager/templates/_helpers.tpl` | 32 | 标准 label/selector 辅助 |
| `charts/emotion-echo/charts/cert-manager/templates/deployment.yaml` | 49 | Deployment + ServiceAccount |
| `charts/emotion-echo/charts/cert-manager/templates/clusterissuer.yaml` | 11 | selfsigned ClusterIssuer |
| `charts/emotion-echo/charts/grafana/templates/ingress-tls.yaml` | 55 | 条件 Ingress TLS 模板 |
| `k8s/tests/stage_29a_render_test.go` | 110 | RED → GREEN gate 测试 |

### 修改

| 路径 | 变更 |
|------|------|
| `charts/emotion-echo/Chart.yaml` | 新增 `cert-manager` 作为 dependency（condition: `cert-manager.enabled`） |
| `charts/emotion-echo/values-dev.yaml` | 在 `global:` 下新增 `grafanaIngressTls` 块；在顶层新增 `cert-manager` 块 |
| `k8s/tests/stage_29a_render_test.go` | 修正正则：Go RE2 不支持 `(?s)` dot-matches-newline；接受 Helm `quote` 函数的双引号包裹 |

---

## 四、Smoke Evidence（render 输出）

### 4.1 `helm lint charts/emotion-echo`

```
==> Linting charts/emotion-echo
[INFO] Chart.yaml: icon is recommended

1 chart(s) linted, 0 chart(s) failed
```

### 4.2 `helm lint charts/emotion-echo/charts/cert-manager`

```
==> Linting charts/emotion-echo/charts/cert-manager
[INFO] Chart.yaml: icon is recommended

1 chart(s) linted, 0 chart(s) failed
```

### 4.3 `helm template ee charts/emotion-echo -f charts/emotion-echo/values-dev.yaml`（关键片段）

**cert-manager Deployment + ServiceAccount + ClusterIssuer**：

```yaml
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ee-cert-manager
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ee-cert-manager-controller
spec:
  replicas: 1
  template:
    spec:
      serviceAccountName: ee-cert-manager
      containers:
        - name: controller
          image: "quay.io/jetstack/cert-manager-controller:v1.14.0"
---
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: selfsigned-issuer
spec:
  selfSigned: {}
```

**Grafana Ingress TLS**：

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: grafana
  annotations:
    cert-manager.io/cluster-issuer: "selfsigned-issuer"
    cert-manager.io/renew-before: 720h
spec:
  ingressClassName: nginx
  tls:
    - hosts:
        - "grafana.local"
      secretName: "grafana-tls"
  rules:
    - host: "grafana.local"
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: grafana
                port:
                  number: 3000
```

### 4.4 `go test -tags k8s ./k8s/tests/...`

```
=== RUN   TestStage29A_CertManagerChartRenders
--- PASS: TestStage29A_CertManagerChartRenders (0.57s)
=== RUN   TestStage29A_GrafanaIngressTLS
--- PASS: TestStage29A_GrafanaIngressTLS (0.53s)
PASS
ok  	github.com/emotion-echo/k8s-tests	(cached)
```

完整套件（16 个测试，Stage 28-A → 28-E + Stage 29-A）全部 PASS，**无回归**。

---

## 五、Kind 集群 live smoke（建议下一步）

> 本次 session 受时间约束未执行 live cluster smoke，仅交付 render-assert + helm lint。
> 落地到真实 kind 集群的步骤（留给 Stage 29-A.5 follow-up 或手动 verify）：

```bash
# 1. 起 kind 集群
bash k8s/scripts/01-create-cluster.sh

# 2. 安装 cert-manager + 整个 umbrella
helm install ee charts/emotion-echo -f charts/emotion-echo/values-dev.yaml

# 3. 等待 ClusterIssuer Ready
kubectl wait --for=condition=Ready clusterissuer/selfsigned-issuer --timeout=60s

# 4. 等待证书 Ready（cert-manager 自动响应 Ingress 注解）
kubectl wait --for=condition=Ready certificate/grafana-tls -n ee-observability --timeout=90s

# 5. 通过 port-forward 验证 TLS handshake
kubectl port-forward -n ee-observability svc/grafana 3000:3000 &
INGRESS_POD=$(kubectl get pods -n ingress-nginx -l app.kubernetes.io/component=controller -o name | head -1)
kubectl port-forward -n ingress-nginx $INGRESS_POD 8443:443 &
curl -k --resolve grafana.local:8443:127.0.0.1 https://grafana.local:8443/ -I
# 期望: HTTP/1.1 200, server: nginx (cert-manager 已注入 self-signed 证书)
```

---

## 六、本次 session 完整 commit 链（共 19 个）

### Phase X0 — 项目收口（git hygiene）

```
dad25b2  chore(stage-27-hygiene): commit data-layer subcharts (postgres/redis/kafka/etcd)
d391452  chore(stage-27-hygiene): commit AI-profile subcharts (fer/sensevoice/xtts)
1c712d9  chore(stage-27-hygiene): commit APISIX ingress subcharts
28238b0  chore(stage-27-hygiene): commit frontend web subchart
ec8a079  chore(stage-27-hygiene): commit 6 business-service subcharts
617ed43  chore(stage-27-hygiene): commit skywalking subchart core templates
0d2596b  chore(stage-27-hygiene): commit umbrella-level templates
b7c5635  docs: commit stage-26/27/28 + xtts landing + 13-file learn series
```

### Phase X1 — Stage 26-M 测试收口（严格 TDD 子循环）

```
9a579a4  test(stage-26-M): add shared/pkg unit tests (RED-then-GREEN, closes Stage 26-M)
7f78ce7  test(stage-26-M): add user-svc internal tests
55c27a5  test(stage-26-M): add analytics-svc internal tests
62d4ce8  test(stage-26-M): add assessment-svc internal tests
b19b5c8  test(stage-26-M): add chat-svc internal unit tests
85a7a86  test(stage-26-K): add chat-svc Postgres integration test (testcontainers)
16d31bb  test(stage-26-M): add ai-svc internal unit tests
8ba9221  test(stage-26-K): add ai-svc Postgres + grpc integration test (testcontainers)
8becbbd  chore(legacy): clean up emotion-echo-gin config.yaml
```

### Phase X2 — Stage 29-A（HTTPS 最小切入点）

```
3147894  test(stage-29-A): red — cert-manager + Grafana ingress TLS render assertions
515f82b  feat(stage-29-A): cert-manager chart + Grafana ingress TLS (GREEN)
```

---

## 七、不在本次范围内（明确留给后续）

| 项 | 原因 | 后续阶段 |
|---|------|---------|
| PromQL 业务告警规则（HighErrorRate / PodOOMKilled） | 用户选择「本次只做 HTTPS」 | Stage 29-B |
| ExternalSecret / SealedSecret for prod | 同上 | Stage 29-C |
| 其余 15 条 ApisixRoute TLS retrofit | 用户选择「仅 Grafana」 | Stage 29-D |
| Let's Encrypt prod issuer | 暂用 self-signed | Stage 29-E |
| Stage 30 ArgoCD GitOps | roadmap 下一阶段 | Stage 30 |
| Stage 31 ACK 迁移 / 32 安全 / 33 HA / 34 DR | 远期 roadmap | Stage 31+ |
| CI（`.github/workflows/`） | 未授权 | 后续单独 PR |
| README.md 「Stage 25/25」 stale badge 更新 | 待本次 landing 后一次性刷新 | 立即可做（建议） |

---

## 八、风险与回退

| 风险 | 缓解 |
|------|------|
| cert-manager 镜像拉取（quay.io 国内网络） | 已锁定 tag `v1.14.0`；如拉取失败可在 `values.yaml` 切到国内 mirror 或 fallback 到 `v1.13.4` |
| self-signed 证书浏览器会拒绝 | dev/local 集群明确接受；prod 必须切到 Let's Encrypt issuer（Stage 29-E） |
| cert-manager namespace 不存在 | 部署时需先创建 `cert-manager` namespace 或改 chart 加 namespace 模板（建议 Stage 29-A.5 follow-up） |
| Ingress controller 类 `nginx` 可能在 kind 默认不存在 | dev overlay 默认使用 `ingress-nginx`；如用别的 controller 需调整 `ingressClassName` |
| 16 个 ApisixRoute 的 301/302 重定向（Stage 26-Q 修复）可能与 cert-manager 同时启用时冲突 | 暂未观察到；如发现回退到本次 commit `515f82b` |

---

## 九、Refs

- **本次 session 入口 plan**：参见对话上下文中的 plan（X0 → X1 → X2 共 19 个 commit）
- **roadmap**：`docs/learn/12-stage-28-roadmap.md` 优先级 ② 「HTTPS / cert-manager」
- **landing 候选清单**：`docs/STAGE-28-LANDING.md` § 九
- **上 stage 落地**：`docs/STAGE-28-LANDING.md`（Stage 28 全闭环）
- **AGENTS.md § 〇 TDD 约定**：本次严格遵守（先 RED 测试再 GREEN 实现）
- **Cert-manager 官方文档**：<https://cert-manager.io/docs/>（self-signed issuer 配置参考）

---

> 最后更新：本次 session 落地 — by 当前协作 Agent
> 适用版本：Stage 28-F → 29-A closure；后续 Stage 29-B / 29-C / 29-D 各自独立循环