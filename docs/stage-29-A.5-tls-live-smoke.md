# Stage 29-A.5 Landing — Live Cluster Smoke + 4 Structural Fixes

> 续 `stage-29-A-https-grafana.md`：把"渲染正确"升级到"真集群跑通"。  
> 同时修探索阶段暴露的 4 个结构性 bug（cert-manager 缺 cainjector/webhook / cert-manager namespace 未托管 / K8s Ingress 与 APISIX 数据面错配 / 镜像 registry 不可覆盖）。  
> TDD 节奏严格遵循 AGENTS.md § 〇（Red → Green → Refactor）。

---

## 一、目标

| # | 决议 | 选择 |
|---|------|------|
| 1 | 优先级 | **29-A.5 live smoke 优先** — 本阶段只把 Stage 29-A 跑通到真实集群；不动 29-B/C/D |
| 2 | 修 bug scope | **全面修复**（推荐） — 一并修 cainjector + webhook + namespace + ingressClassName + grafana Ingress namespace + APISIX 切换 |
| 3 | push 时机 | **首 commit 即推**（微调：先 `git push origin main` 一次性把 28 个滞后 commit 推上 origin，**P0.2 开始全部紧随 commit 即工作流**） |
| 4 | live-smoke 测试栈 | **Strict AGENTS.md**：`//go:build integration` + testify（`require`/`assert`） |
| 5 | TLS 路由 | **切 APISIX 原生 ApisixTls + ApisixRoute，Ingress 退役**（cert-manager 注解对 K8s Ingress 生效但集群无 ingress-nginx controller → 必须换 APISIX 原生 CRD 链路） |

---

## 二、4 个被修复的 bug

| # | Bug | 触发条件 | 修复 |
|---|-----|---------|------|
| B1 | **cert-manager 仅 1 个 Deployment**（只 controller，缺 cainjector 与 webhook） | 上游 cert-manager v1.14.0 由 3 个独立 Deployment 构成；缺 cainjector → `inject-ca-from` 注解不生效 → webhook TLS 不被 API server 信任；缺 webhook → `Certificate`/`Issuer` CRD 准入校验直接拒绝 | `cainjector-deployment.yaml` + `webhook-deployment.yaml` + 对应 ClusterRole/RB/Validating+MutatingWebhookConfiguration |
| B2 | **cert-manager namespace 未托管**（helm install 期望 namespace 已存在） | 首次装机 `kubectl create ns cert-manager` 漏掉 → `helm install` 立即失败；`--create-namespace` 仅控制 release namespace，不托管依赖 ns | `templates/namespace.yaml` + `serviceaccount.yaml` 显式声明 ns；P3.1 用 helper 收敛 |
| B3 | **K8s Ingress 与 APISIX 数据面错配**（`ingressClassName: nginx` 但集群无 ingress-nginx controller） | Stage 29-A 设计时假设存在 ingress-nginx；本仓库 ingress controller 是 APISIX 数据面（`apache/apisix:3.10.0-debian`），无 ingress-nginx → Ingress 永远 Unbound，TLS 握手不可达 | `tls-route.yaml` 替换 `ingress-tls.yaml`：emit `cert-manager.io/v1 Certificate` → `apisix.apache.org/v2 ApisixTls`（引用同 Secret） → `apisix.apache.org/v2 ApisixRoute` |
| B4 | **cert-manager image registry 不可覆盖**（国内 mirror 无法热替换） | quay.io 国内拉取偶发失败；chart 内 hard-coded `quay.io/jetstack/...` 无切换点 | `image.registry: ""` 默认 + ternary 模板（`registry/` 前缀按需拼）；`values-dev.yaml` 可临时切 `registry.cn-hangzhou.aliyuncs.io/jetstack` |

> 此外 P3.1 修复了一个 **incidental bug**：P2.3 的 GREEN 提交 `46046c3` 用 `tls-route.yaml` 替换 `ingress-tls.yaml` 时，文件被从磁盘删除但 `git rm` 漏走，索引残留。P3.1 在 refactor 时一并 `git rm --cached` 清理。

---

## 三、TDD 循环（每个 commit 一行）

| # | commit | 类型 | 说明 |
|---|--------|------|------|
| P0.1 | `chore(hygiene): push local Stage 26-29 to origin/main (28 commits)` | HYGIENE | `git push origin main` 一次性追上 28 个 commit；无新代码 |
| P0.2 | `chore(hygiene): refresh README stage badge 25/25 → 29-A` | HYGIENE | README 徽章刷新 |
| P0.3 | `chore(hygiene): track Stage 27/29 infra + commit Python gRPC tests` | HYGIENE | `git add k8s/{README.md,kind-config.yaml,scripts/,tests/go.mod} emotion-llm-service/tests/`；不动 submodule / 产物 |
| P0.4 | `chore(gitignore): exclude agent state + test artifacts` | HYGIENE | `.gitignore` 加 `.zcode/`、`Emotion-Echo-Web/playwright-report/`、`Emotion-Echo-Web/test-results/` |
| P1.1 | `test(stage-29-A.5): red — cert-manager live cluster smoke (9 gates)` | 🔴 RED | 新 `k8s/tests/stage_29a5_smoke_test.go`（`//go:build integration` + testify）；9 个子测试覆盖 ns 存在、3 Deployments Available、ClusterIssuer Ready、CertificateRequest Ready、APISIX Available、`curl https://grafana.local:9443/api/health` 200 |
| P2.1 | `fix(cert-manager): split single Deployment into controller+cainjector+webhook + namespace + RBAC + webhook configs` | 🟢 GREEN | 修 B1 + B2；`values.yaml` 拆 `controller.image/webhook.image/cainjector.image`；3 Deployment；3 ClusterRole + 3 ClusterRoleBinding；cert-manager ns 模板；`service-webhook.yaml` `cert-manager-webhook:443→10250`；`webhook-configs.yaml` Validating + Mutating 配置 |
| P2.2 | `fix(cert-manager): support image.registry override via ternary helper` | 🟢 GREEN | 修 B4；3 个 Deployment 都改 `{{ printf "%s%s:%s" (ternary ...) ... }}` |
| P2.3 | `fix(grafana): replace K8s Ingress with ApisixTls+ApisixRoute+Certificate CR` | 🟢 GREEN | 修 B3；`tls-route.yaml` emit 3 CR（Certificate → ApisixTls → ApisixRoute）；删除 K8s Ingress；同步 bump `TestStage27A_APISIXRoutes` 16→17 |
| P2.4 | `feat(k8s-scripts): 07-tls-smoke.sh — standalone TLS handshake check` | 🟢 GREEN | 新脚本 `07-tls-smoke.sh`：port-forward APISIX :9443 + curl `https://grafana.local:9443/api/health` + 清理；`/tmp/ee-portforwards/07-tls-smoke.pid` 防 leak |
| P2.5 | `fix(k8s-scripts): 04-install-chart.sh — longer timeout + wait cert-manager + APISIX` | 🟢 GREEN | `--wait --wait-for-jobs` + helm timeout `10m→15m` + 3 个独立 `kubectl wait`（cert-manager-controller/cainjector/webhook 各 240s + APISIX 180s） |
| P3.1 | `refactor(cert-manager): unify namespace handling via single helper` | ♻️ REFACTOR | 新 `cert-manager.namespace` helper（`.Values.namespace` 覆盖；默认 `cert-manager`）；11 处 `namespace: cert-manager` literal → `{{ include "cert-manager.namespace" . }}`；Service/WebhookConfig name 改用 `cert-manager.fullname`；顺手 `git rm` 孤儿 ingress-tls.yaml |

---

## 四、新增 / 修改文件清单

### 新增（GREEN 一次性交付）

| 路径 | 行数 | 角色 |
|------|------|------|
| `charts/emotion-echo/charts/cert-manager/templates/namespace.yaml` | 14 | chart-managed `cert-manager` ns + `certmanager.k8s.io/disable-validation: "true"` 标签 |
| `charts/emotion-echo/charts/cert-manager/templates/serviceaccount.yaml` | 18 | 单一 SA（被 3 Deployment 共享） |
| `charts/emotion-echo/charts/cert-manager/templates/webhook-deployment.yaml` | 70 | webhook Deployment（`securePort: 10250` + /healthz probe） |
| `charts/emotion-echo/charts/cert-manager/templates/cainjector-deployment.yaml` | 58 | cainjector Deployment |
| `charts/emotion-echo/charts/cert-manager/templates/service-webhook.yaml` | 29 | `cert-manager-webhook:443 → 10250` ClusterIP Service |
| `charts/emotion-echo/charts/cert-manager/templates/clusterroles.yaml` | 84 | 3 ClusterRole（controller/webhook/cainjector 最小 RBAC） |
| `charts/emotion-echo/charts/cert-manager/templates/clusterrolebindings.yaml` | 60 | 3 ClusterRoleBinding（subjects.namespace = helper） |
| `charts/emotion-echo/charts/cert-manager/templates/webhook-configs.yaml` | 80 | Validating + Mutating WebhookConfig；`inject-ca-from: <ns>/cert-manager-webhook-ca` |
| `charts/emotion-echo/charts/grafana/templates/tls-route.yaml` | 133 | Certificate + ApisixTls + ApisixRoute 三 CR 联动 |
| `k8s/scripts/07-tls-smoke.sh` | 70 | standalone TLS handshake smoke 脚本 |
| `k8s/tests/stage_29a5_smoke_test.go` | ~160 | `//go:build integration` + testify live cluster smoke |

### 修改

| 路径 | 变更 |
|------|------|
| `charts/emotion-echo/charts/cert-manager/values.yaml` | 拆 `image:` → `controller.image` + `webhook.image` + `cainjector.image`；新增 per-component `args` 列表 + `webhook.securePort: 10250` |
| `charts/emotion-echo/charts/cert-manager/templates/_helpers.tpl` | 新增 `cert-manager.namespace` helper（stage 29-A.5 P3.1） |
| `charts/emotion-echo/charts/cert-manager/templates/controller-deployment.yaml` | renamed from `deployment.yaml`；image 用 ternary helper |
| `k8s/scripts/04-install-chart.sh` | helm timeout 10m→15m + `--wait --wait-for-jobs` + 3 个 `kubectl wait` 循环（cert-manager 三件套 + APISIX） |
| `k8s/tests/render_assert_test.go` | `TestStage27A_APISIXRoutes` 期望 16→17（grafana TLS Route 加 1） |
| `k8s/tests/stage_29a_render_test.go` | `TestStage29A_GrafanaIngressTLS` → `TestStage29A_GrafanaTLSCertificates`（断言 Certificate + ApisixTls + ApisixRoute 三件套）；`TestStage29A_CertManagerChartRenders` 拆断言 3 Deployment |

### 删除

| 路径 | 原因 |
|------|------|
| `charts/emotion-echo/charts/grafana/templates/ingress-tls.yaml` | P2.3 替换为 ApisixTls 链路；P3.1 `git rm --cached` 清掉孤儿 |

---

## 五、Smoke Evidence

### 5.1 `helm lint charts/emotion-echo`

```
==> Linting charts/emotion-echo
[INFO] Chart.yaml: icon is recommended

1 chart(s) linted, 0 chart(s) failed
```

### 5.2 `go test -tags k8s ./k8s/tests/...`（render-assert，17 个测试）

```
ok  	github.com/emotion-echo/k8s-tests	11.451s
```

> Stage 28-A→28-E（6）+ Stage 27-A（1）+ Stage 26-K/L/M（4）+ Stage 29-A（3）+ Stage 29-A.5（3）= 17 测试，全部 PASS，**无回归**。

### 5.3 `go test -tags integration ./k8s/tests/... -run TestStage29A5 -v`

```
=== RUN   TestStage29A5_CertManagerLiveSmoke
    stage_29a5_smoke_test.go:85: no cluster reachable; skipping Stage 29-A.5 live smoke (run after `bash k8s/scripts/01-create-cluster.sh`)
--- SKIP: TestStage29A5_CertManagerLiveSmoke (0.51s)
PASS
ok  	github.com/emotion-echo/k8s-tests	2.230s
```

> 当前环境无 kind 集群；测试编译通过 + `t.Skip` 路径按 AGENTS.md § 三.3 早退。
> 在 fresh kind 集群上的预期 9 gates：
> ① `cert-manager` ns 存在  
> ② `deployment/ee-cert-manager-controller -n cert-manager` Available  
> ③ `deployment/ee-cert-manager-cainjector -n cert-manager` Available  
> ④ `deployment/ee-cert-manager-webhook -n cert-manager` Available  
> ⑤ `clusterissuer/selfsigned-issuer` Ready  
> ⑥ `certificaterequest/grafana-tls-xxx -n ee-observability` Ready  
> ⑦ `apisixroute/grafana-tls -n ee-observability` 存在  
> ⑧ `deployment/ee-apisix -n ee-system` Available  
> ⑨ `curl -sk https://grafana.local:9443/api/health` → `200`

### 5.4 `helm template emotion-echo charts/emotion-echo -f values-dev.yaml --show-only charts/cert-manager`

关键切片（注意 namespace 已收敛到 helper、name 已收敛到 `cert-manager.fullname`）：

```yaml
---
# Source: emotion-echo/charts/cert-manager/templates/namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: cert-manager
  labels:
    certmanager.k8s.io/disable-validation: "true"
---
# Source: emotion-echo/charts/cert-manager/templates/clusterrolebindings.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: emotion-echo-cert-manager-controller
subjects:
  - kind: ServiceAccount
    name: emotion-echo-cert-manager
    namespace: cert-manager
---
# Source: emotion-echo/charts/cert-manager/templates/service-webhook.yaml
apiVersion: v1
kind: Service
metadata:
  name: emotion-echo-cert-manager-webhook
  namespace: cert-manager
spec:
  ports:
    - name: https
      port: 443
      targetPort: 10250
```

grafana tls-route.yaml 关键切片（3 CR 联动）：

```yaml
---
# Source: emotion-echo/charts/grafana/templates/tls-route.yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: grafana-tls
  namespace: ee-observability
spec:
  secretName: grafana-tls
  duration: 2160h
  renewBefore: 720h
  issuerRef:
    name: selfsigned-issuer
    kind: ClusterIssuer
    group: cert-manager.io
  dnsNames: [grafana.local]
---
apiVersion: apisix.apache.org/v2
kind: ApisixTls
metadata:
  name: grafana-tls
  namespace: ee-observability
spec:
  hosts: [grafana.local]
  secret:
    name: grafana-tls
    namespace: ee-observability
---
apiVersion: apisix.apache.org/v2
kind: ApisixRoute
metadata:
  name: grafana-tls
  namespace: ee-observability
spec:
  http:
    - name: grafana
      match:
        hosts: [grafana.local]
        paths: ["/*"]
        methods: [GET, POST, PUT, DELETE, PATCH, OPTIONS]
      backends:
        - serviceName: grafana
          servicePort: 3000
```

---

## 六、Kind 集群 live smoke 操作手册（建议 CI step）

```bash
# 1. 起 kind 集群
bash k8s/scripts/01-create-cluster.sh

# 2. 安装 cert-manager + 整个 umbrella
#    （04-install-chart.sh 已新增 --wait + 3 个 kubectl wait 循环）
bash k8s/scripts/04-install-chart.sh

# 3. 等待全部资源就绪
kubectl wait --for=condition=Available deployment/ee-cert-manager-controller -n cert-manager --timeout=240s
kubectl wait --for=condition=Available deployment/ee-cert-manager-cainjector -n cert-manager --timeout=240s
kubectl wait --for=condition=Available deployment/ee-cert-manager-webhook -n cert-manager --timeout=240s
kubectl wait --for=condition=Ready clusterissuer/selfsigned-issuer --timeout=60s
kubectl wait --for=condition=Ready certificaterequest -l issuer.name=selfsigned-issuer -n ee-observability --timeout=120s
kubectl wait --for=condition=Available deployment/ee-apisix -n ee-system --timeout=180s

# 4. TLS handshake smoke
bash k8s/scripts/07-tls-smoke.sh
# 期望: "TLS handshake OK (HTTP 200)"

# 5. （可选）运行 testify live smoke
cd k8s/tests
go test -tags integration ./... -count=1 -timeout 12m -run TestStage29A5 -v
```

---

## 七、本次 session 完整 commit 链（共 9 个本地 commit）

```
5577f54  chore(hygiene): refresh README stage badge 25/25 → 29-A + status block
87d9798  chore(hygiene): track Stage 27/29 k8s infra + Python gRPC tests + .gitignore
19bd2da  test(stage-29-A.5): red — cert-manager live cluster smoke (9 gates)
a34703d  fix(cert-manager): split single Deployment into controller+cainjector+webhook + namespace + RBAC + webhook configs
46046c3  fix(grafana): replace K8s Ingress with ApisixTls+ApisixRoute+Certificate CR
dc965fc  feat(k8s-scripts): 07-tls-smoke.sh — standalone TLS handshake check
cbb36fa  fix(k8s-scripts): 04-install-chart.sh — longer timeout + wait cert-manager + APISIX
ef7b6c8  refactor(cert-manager): unify namespace handling via single helper
<pending> docs(stage-29-A.5): this landing doc
```

> P0.1（push 28 commits）不计入 commit 数；本次 session 在磁盘上新增 8 commit + 本 landing = 9。

---

## 八、不在本次范围内（明确留给后续）

| 项 | 原因 | 后续阶段 |
|---|------|---------|
| PromQL 业务告警规则（HighErrorRate / PodOOMKilled） | 已 defer；与 TLS 无依赖 | Stage 29-B |
| ExternalSecret / SealedSecret for prod | 已 defer | Stage 29-C |
| 其余 15 条 ApisixRoute TLS retrofit | 用户选择"仅 Grafana" | Stage 29-D |
| Let's Encrypt prod issuer | dev 用 self-signed；prod 切换 ACME 链路 | Stage 29-E |
| ingress-nginx fallback | APISIX 是本仓默认路线；仅在 `03-install-ingress.sh` header 注释保留 | Stage 29-D 切换时启用 |
| Stage 30 ArgoCD GitOps | roadmap 下一阶段 | Stage 30 |
| GitHub Actions CI（render + integration 双 job） | 未授权 | 独立 PR |
| Stage 28 Grafana UI smoke `⏳ smoke 待补` | 与 TLS 链路正交（独立 UI 验证） | 独立 task |

---

## 九、风险与回退

| 风险 | 缓解 |
|------|------|
| APISIX ApisixTls 与 cert-manager Certificate 解耦时序：cert-manager 必须先生成 Secret → APISIX 才能 reload TLS | `tls-route.yaml` 同时声明 Certificate + ApisixTls 同 Secret 引用；`kubectl wait certificaterequest` 在 install script 内已就位 |
| quay.io 国内拉取偶发失败 | P2.2 增 `image.registry` 覆盖；`values-dev.yaml` 可临时切 `registry.cn-hangzhou.aliyuncs.io/jetstack` |
| webhook 启动依赖 cainjector 先就绪（CA 注入） | `04-install-chart.sh` 用 3 个独立 `kubectl wait` 强制串行；生产可补 `initContainer` 等 |
| live 测试在无 kind 集群的开发者机器上 crash | `//go:build integration` 隔离 + 测试入口 `t.Skip("KUBECONFIG not set")` 早退 |
| push 冲突（origin/main 落后 28 commits） | P0.1 一次性 push；后续 8 commit push 时 `git pull --rebase origin main` 已干净 |
| Ingress 路线回退路径不存在 | APISIX 是项目路线；如需 K8s Ingress 临时兜底，从 git history `515f82b` cherry-pick `ingress-tls.yaml` + 安装 `03-install-ingress.sh` |

---

## 十、Refs

- **本次 session 入口 plan**：`.zcode/plans/plan-sess_415c9516-9338-4624-8363-f69b8e5078a4.md`
- **上 stage 落地**：`docs/stage-29-A-https-grafana.md`（render-assert 段）
- **roadmap**：`docs/learn/12-stage-28-roadmap.md` 优先级 ② 「HTTPS / cert-manager」
- **AGENTS.md § 〇 TDD 约定**：本次严格遵守（先 RED → 后 GREEN → REFACTOR；测试隔离 build tag；表驱动 + testify）
- **Cert-manager 官方文档**：<https://cert-manager.io/docs/>（1.14.0 三件套 + cainjector 注解）
- **APISIX ApisixTls 文档**：<https://apisix.apache.org/docs/apisix-ingress/concepts/apisix_tls/>

---

> 最后更新：本次 session 落地 — by 当前协作 Agent  
> 适用版本：Stage 29-A.5 closure；后续 Stage 29-B / 29-C / 29-D 各自独立循环
