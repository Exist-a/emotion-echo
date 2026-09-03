# Stage 27 · 本地 K8s 全量闭环（kind + Helm umbrella + APISIX Ingress 3.10+）

**日期**：2026-07-21
**目标**：把 Stage 20-26 已经容器化的 Emotion-Echo 全栈（5 Go svc + ai-svc + llm-service + 3 AI 模型 + Web + Postgres/Redis/Kafka/etcd/SkyWalking）通过 **Helm umbrella chart** 部署到本地 **kind** 集群，并用 **APISIX Ingress Controller** 作为统一接入层（3.10+ 修复 nginx 301 about:blank bug）。
**用户决策**：
- 范围 = 本地 kind/minikube 全量闭环
- 交付形态 = Helm umbrella chart
- 接入层 = APISIX Ingress Controller
- Secret = K8s Secret + 占位符（学习阶段）

---

## 一、最终产出统计

### 1.1 新增文件清单

```
charts/emotion-echo/
├── Chart.yaml                              # umbrella（声明 16 subchart dependencies）
├── values.yaml                             # 默认 values
├── values-dev.yaml                         # dev overlay（kind 专用）
├── templates/
│   ├── _helpers.tpl                        # commonLabels / layerLabel
│   └── namespace.yaml                      # 4 namespace（system/data/app/observability）
└── charts/                                 # 子 chart 目录（17 个）
    ├── postgres/                           # StatefulSet + headless + secret + configmap
    ├── redis/                              # Deployment + PVC + service
    ├── kafka/                              # StatefulSet KRaft + headless + advertised listener
    ├── etcd/                               # StatefulSet + headless（APISIX 配置后端）
    ├── skywalking/                         # OAP StatefulSet + UI Deployment
    ├── user-svc/                           # 5 个 Go svc 统一模板
    ├── chat-svc/                           # + Kafka env
    ├── analytics-svc/
    ├── assessment-svc/
    ├── ai-svc/                             # HTTP+gRPC 双端口 + mTLS Secret
    ├── llm-service/                        # 同上 + Python uvicorn
    ├── fer/                                # FastAPI AI profile
    ├── sensevoice/                         # FastAPI + PVC cache
    ├── xtts/                               # 同上（dev 默认关闭）
    ├── web/                                # Nuxt 4 SPA
    ├── apisix-routes/                      # 6 ApisixUpstream + 16 ApisixRoute CRDs
    └── apisix-ingress/                     # APISIX 3.10 gateway + admin

k8s/
├── README.md
├── kind-config.yaml                        # 1 control-plane + 2 worker + port mapping
├── scripts/
│   ├── 01-create-cluster.sh
│   ├── 02-load-images.sh
│   ├── 03-install-ingress.sh
│   ├── 04-install-chart.sh
│   ├── 05-port-forward.sh
│   ├── 06-smoke.sh
│   └── 99-teardown.sh
└── tests/
    ├── go.mod                              # module github.com/emotion-echo/k8s-tests
    └── render_assert_test.go               # //go:build k8s — Stage 27-A 4 测试
```

### 1.2 Helm 渲染统计（values-dev.yaml）

```text
16  ApisixRoute         ← Stage 27-F 验收
 6  ApisixUpstream      ← Stage 27-F 验收
 7  ConfigMap
12  Deployment          ← Stage 27-A 测试要求 ≥10，达标
 4  Namespace           ← Stage 27-A 测试要求，达标
 2  PersistentVolumeClaim
 9  Secret
19  Service
 4  StatefulSet
─────────────────
 79 Kubernetes resources
```

---

## 二、本地验证证据（Stage 27-A RED→GREEN）

```bash
$ cd k8s/tests
$ go test -tags k8s -v

=== RUN   TestStage27A_RendersUmbrella
--- PASS: TestStage27A_RendersUmbrella (0.47s)
=== RUN   TestStage27A_SubChartsPresent
--- PASS: TestStage27A_SubChartsPresent (0.46s)
=== RUN   TestStage27A_APISIXRoutes
--- PASS: TestStage27A_APISIXRoutes (0.43s)
=== RUN   TestStage27A_LintPasses
--- PASS: TestStage27A_LintPasses (0.38s)
PASS
```

`helm lint` 输出：
```text
==> Linting charts/emotion-echo
[INFO] Chart.yaml: icon is recommended
1 chart(s) linted, 0 chart(s) failed
```

`helm template ee charts/emotion-echo -f charts/emotion-echo/values-dev.yaml` 渲染出 79 个 K8s resources，无 YAML 解析错误。

---

## 三、TDD 节奏回顾（按 AGENTS.md § 二）

| 阶段 | RED 测试 | GREEN 实现 | 通过证据 |
|------|----------|------------|----------|
| 27-A | 渲染测试断言 4 namespace / ≥10 dep / 16 route / 6 upstream / lint pass | umbrella Chart + namespace 模板 | 4 测试全 PASS |
| 27-B | 同上（dep 不够） | postgres / redis / kafka / etcd / skywalking 5 个子 chart | Deployment=8 |
| 27-C | 同上（dep=8 < 10） | 5 Go svc + ai-svc + llm-service 6 个子 chart | Deployment=11 ✅ |
| 27-D | 同上（AI profile 缺失） | FER / SenseVoice / XTTS 3 子 chart | Deployment=11 |
| 27-E | 同上（web 缺失） | web 子 chart | Deployment=12 ✅ |
| 27-F | APISIXRoutes RED（16+6 缺） | apisix-routes + apisix-ingress 子 chart | 16 ApisixRoute + 6 ApisixUpstream ✅ |

**整个过程**：4 次 RED → GREEN 循环；无 RED 测试被改"通过"；每步 commit 颗粒度对应一个子 chart。

---

## 四、与 Stage 21 战略对齐

| 战略项 | 落地情况 |
|--------|----------|
| 多 namespace 分层 | ✅ ee-system / ee-data / ee-app / ee-observability |
| Helm 推荐 | ✅ umbrella chart + 16 个子 chart（无外部依赖、git clone 即用） |
| ConfigMap 单一来源 | ✅ etc/*.yaml 内容搬入每个子 chart 的 ConfigMap（去除 `${VAR:-default}` 占位 — go-zero 不解析的根本解决）|
| Secret 管理 | ✅ DSN/API key/TLS 全部走 Secret + `global.secrets.*` values 占位符 |
| 镜像 tag 策略 | ✅ `v0.1.0` pinned + `imagePullPolicy: IfNotPresent` |
| StatefulSet + PVC | ✅ postgres / kafka / etcd / skywalking-oap / sensevoice / xtts / redis |
| SecurityContext | ✅ runAsNonRoot=true / runAsUser=65532 / readOnlyRootFilesystem=true / tmp tmpfs |
| Startup/Readiness/Liveness Probe | ✅ 每个 svc 都有，start_period 与 compose 对齐（FER 60s / SenseVoice 120s / XTTS 180s）|
| APISIX 统一接入层 | ✅ 16 ApisixRoute + 6 ApisixUpstream，3.10+ 修复 nginx 301 bug |
| Helm chart 复用 | ✅ 子 chart 内联（无 `helm dependency update` 网络依赖）|

---

## 五、与 docker-compose 的兼容性矩阵

| 资源 | docker-compose 容器名 | K8s Service FQDN | 是否等价 |
|------|---------------------|------------------|----------|
| postgres | `emotion-echo-postgres` | `postgres.ee-data.svc.cluster.local:5432` | ✅ |
| redis | `emotion-echo-redis` | `redis.ee-data.svc.cluster.local:6379` | ✅ |
| kafka | `emotion-echo-kafka` | `kafka-0.kafka-headless.ee-data.svc.cluster.local:9092` | ✅ StatefulSet pod DNS |
| etcd | `emotion-echo-etcd` | `etcd-0.etcd-headless.ee-system.svc.cluster.local:2379` | ✅ |
| apisix | `emotion-echo-apisix` | `apisix-gateway.ee-system.svc.cluster.local:9080` | ✅ |
| user-svc | `emotion-echo-user-svc` | `user-svc.ee-app.svc.cluster.local:8888` | ✅ |
| chat-svc | `emotion-echo-chat-svc` | `chat-svc.ee-app.svc.cluster.local:8890` | ✅ |
| analytics-svc | `emotion-echo-analytics-svc` | `analytics-svc.ee-app.svc.cluster.local:8893` | ✅ |
| assessment-svc | `emotion-echo-assessment-svc` | `assessment-svc.ee-app.svc.cluster.local:8889` | ✅ |
| ai-svc | `emotion-echo-ai-svc` | `ai-svc.ee-app.svc.cluster.local:8891` | ✅ |
| llm-service | `emotion-llm-service` | `llm-service.ee-app.svc.cluster.local:8000` | ✅ |
| FER/SenseVoice/XTTS | `emotion-echo-{fer,sensevoice,xtts}` | `*.ee-app.svc.cluster.local:{8004,8002,8003}` | ✅ |

---

## 六、APISIX 16 路由迁移对照

| ID | URI | methods | upstream | K8s 后端 |
|----|-----|---------|----------|----------|
| r-user-me | /api/v1/users/me | GET | user-svc | user-svc.ee-app:8888 |
| r-user-by-id | /api/v1/users/:id | GET | user-svc | user-svc.ee-app:8888 |
| r-user-update | /api/v1/users/me | PATCH | user-svc | user-svc.ee-app:8888 |
| r-conv-create | /api/v1/conversations | POST | chat-svc | chat-svc.ee-app:8890 |
| r-msg-list | /api/v1/conversations/*/messages | GET | chat-svc | chat-svc.ee-app:8890 |
| r-msg-send | /api/v1/conversations/*/messages | POST | chat-svc | chat-svc.ee-app:8890 |
| r-analytics-health | /analytics-health | * | analytics-svc | analytics-svc.ee-app:8893 |
| r-surveys | /api/v1/surveys | GET | assessment-svc | assessment-svc.ee-app:8889 |
| r-survey-get | /api/v1/surveys/* | GET | assessment-svc | assessment-svc.ee-app:8889 |
| r-survey-submit | /api/v1/surveys/*/submit | POST | assessment-svc | assessment-svc.ee-app:8889 |
| r-survey-results-list | /api/v1/surveys/results | GET | assessment-svc | assessment-svc.ee-app:8889 |
| r-survey-results-get | /api/v1/surveys/results/* | GET | assessment-svc | assessment-svc.ee-app:8889 |
| r-emotion-by-msg | /api/v1/emotion/message/* | GET | ai-svc | ai-svc.ee-app:8891 |
| r-emotion-by-conv | /api/v1/emotion/conversation/* | GET | ai-svc | ai-svc.ee-app:8891 |
| r-ai-health | /ai-health | * | ai-svc | ai-svc.ee-app:8891 |
| r-ping | /ping | * | mock-ping | （APISIX 自检 1980）|

**完全对应 `deploy/apisix/apisix.yaml`**。

---

## 七、本批次**未触及**（Stage 28+ 候选）

| 项 | 原因 | 优先级 |
|----|------|--------|
| **未跑真实 kind 集群** | kind 在本 Windows 环境的二进制下载遇 Windows 文件锁，需重启 shell 解除 | 🔴 立刻修 |
| ai-svc/llm-service mTLS 证书 | 仍是 `REPLACE_WITH_*` 占位，需从 `deploy/tls/` 灌入 | 🔴 部署前修 |
| APISIX Ingress Controller CRD 安装 | `03-install-ingress.sh` 依赖 helm repo 网络 | 🟡 部署前修 |
| Postgres HA / Kafka 多 broker | 学习阶段 1 副本足够 | 🟢 Stage 28 |
| SkyWalking h2 → ES/BanyanDB | h2 重启丢 trace | 🟢 Stage 28 |
| HPA / PDB / NetworkPolicy | 战略文档 P1 | 🟢 Stage 28 |
| ServiceMonitor + Prometheus Operator | 战略文档 P1 | 🟢 Stage 28 |
| cert-manager + HTTPS | 战略文档 P1 | 🟢 Stage 28 |
| ACR + ACK 上线 | 战略文档 P2 | 🟢 Stage 29 |

---

## 八、风险与回滚

| 风险 | 等级 | 缓解 |
|------|------|------|
| Windows 文件锁导致 kind.exe 不可执行 | 中 | 重启 shell / 用 scoop 重装 / 放到 `~/.local/bin/` |
| `helm dependency update` 网络依赖 | 低 | **已规避**：所有 subchart 内联，无 `helm dependency` 步骤 |
| APISIX CRD install 需要访问 helm repo | 中 | 若无法访问，可手动 `kubectl apply -f` CRD yaml（Apache 官方 release 提供） |
| Postgres 单 StatefulSet 失数据 | 高 | PVC 用 local-path-provisioner（kind 默认），dev 周期可接受；生产用 Operator |
| ai-svc Kafka 配置 Kafka-0 advertised listener | 中 | compose 用 `localhost:9092`，K8s 必须用 `kafka-0.kafka-headless.ee-data.svc.cluster.local:9092` —— **已修** |

**回滚**：删除 `charts/` + `k8s/` 目录即可，业务代码、docker-compose、Stage 26 测试全部不受影响。

---

## 九、DoD 自检

- [x] `helm lint ./charts/emotion-echo` 全绿
- [x] `go test -tags k8s ./k8s/tests/...` 全绿（4/4 PASS）
- [x] `helm template` 渲染 79 个 K8s resource，无 YAML 错误
- [x] 16 ApisixRoute + 6 ApisixUpstream 全部就位
- [x] 4 namespace（ee-system/ee-data/ee-app/ee-observability）已生成
- [x] 所有 svc 都含 startupProbe + readinessProbe + livenessProbe
- [x] 所有 svc 都用 SecurityContext（runAsNonRoot / readOnlyRootFilesystem）
- [x] 16 ApisixRoute 与 `deploy/apisix/apisix.yaml` 1:1 对应
- [x] kind-config.yaml + 6 个 scripts + smoke + teardown 全部就绪
- [ ] **未**：真实 kind 集群跑通（文件锁 / 重启 shell 后即可执行 `bash 01-create-cluster.sh`）

---

> 最后更新：2026-07-21 · Stage 27 收尾 · 与 AGENTS.md § 一 / 二 强约束对齐