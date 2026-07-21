# 04 · Umbrella chart 设计哲学与多环境策略

> 系列：[03 Helm 入门](./03-helm-fundamentals.md) · **04 Umbrella chart** · [05 APISIX 选型](./05-apisix-vs-nginx.md) ...

**适合谁**：管理多个微服务 + 多个环境（dev/staging/prod），想知道"怎样用一个 helm install 把整个系统装好"的读者。
**读完你能**：解释 umbrella chart 为什么是 K8s 多 svc 项目的标准做法，能说出我们 Stage 27 怎么用 17 个子 chart 合成一个发布。

---

## 一句话总结

**Umbrella chart = 一个总 chart，里面套多个子 chart，一次 `helm install` 把整个分布式系统全部装上。**

类比：Linux 发行版（Ubuntu = umbrella，nginx/apache/mysql = 子包）。你 `apt install ubuntu-desktop` 装的是"一组互相依赖的应用"，不是一个单一 app。

---

## 一、为什么需要 Umbrella chart

### 1.1 我们 Emotion-Echo 的现实

我们 Stage 27 要装的不是 1 个应用，是 17 个：

| 类别 | 数量 | 内容 |
|------|------|------|
| 数据层 | 5 | postgres / redis / kafka / etcd / skywalking |
| Go 微服务 | 4 | user-svc / chat-svc / analytics-svc / assessment-svc |
| Python AI 网关 | 2 | ai-svc / llm-service |
| AI 模型 | 3 | fer / sensevoice / xtts |
| 接入层 | 2 | web / apisix-routes |
| 基础设施 | 1 | apisix-ingress |
| **合计** | **17** | |

如果不 umbrella：
- 你要 `helm install postgres ./chart`，`helm install redis ./chart`，`helm install user-svc ./chart` ... **17 次**
- 每个 release 单独管理版本、回滚、状态
- 多个 release 之间的依赖关系靠人记（chat-svc 必须等 postgres 起来）
- 多环境复制粘贴 17 份 yaml

umbrella chart = **一个 chart 装一切**：

```bash
helm install ee ./charts/emotion-echo -f values-dev.yaml
# 一次性装好 17 个 svc + 4 个 ns + 所有 ConfigMap + 所有 Secret
```

### 1.2 三个对比方案

| 方案 | 一次 install 装... | 适用 |
|------|-------------------|------|
| **单 chart（17 个 template）** | 一个 svc，但模板里写 17 份 | ❌ 不可维护 |
| **17 个独立 chart** | 一个 svc | ❌ 部署体验差 |
| **Umbrella chart** | 所有 svc | ✅ 复杂分布式系统标准做法 |

---

## 二、Umbrella chart 结构

```
charts/emotion-echo/                # umbrella
├── Chart.yaml                       # 声明依赖 17 个子 chart
├── values.yaml                      # 默认全局配置
├── values-dev.yaml                  # dev 覆盖
├── values-prod.yaml                 # prod 覆盖
└── charts/                          # 内联子 chart（不用 helm dependency update）
    ├── postgres/                    # 子 chart 1
    │   ├── Chart.yaml
    │   ├── values.yaml
    │   └── templates/
    │       └── statefulset.yaml
    ├── user-svc/                    # 子 chart 2
    ├── chat-svc/                    # 子 chart 3
    ├── ...                          # 共 17 个
    └── apisix-routes/               # 子 chart 17
```

### 2.1 关键点：内联子 chart（不走 helm dependency update）

传统做法（Helm 2 时代）：
```bash
# 子 chart 在另一个仓库
helm dependency update    # 拉子 chart 下来（需要外网）
helm install ...
```

我们的做法（Helm 3 推荐）：
```bash
# 子 chart 直接在 charts/<name>/ 目录里
helm install ee ./charts/emotion-echo    # 直接装，不走 dependency update
```

**好处**：
- **离线可用**：没有外网也能装（学习环境友好）
- **git clone 即用**：不依赖外部仓库
- **改子 chart 即生效**：不用 bump 子 chart 版本
- **可整体 lint**：一个 `helm lint` 把所有子 chart 一起检

**代价**：
- 子 chart 的版本变化要自己 bump
- 不复用社区的 stable chart（如 bitnami/postgresql）

我们项目的子 chart 都是**自写**的（场景太定制），所以内联是正确选择。

---

## 三、Chart.yaml 依赖声明

```yaml
# charts/emotion-echo/Chart.yaml
apiVersion: v2
name: emotion-echo
description: Five schemas - one per service.
type: application
version: 0.1.0
appVersion: "v0.1.0"

dependencies:
  - name: postgres
    version: 0.1.0
    condition: postgres.enabled
  - name: redis
    version: 0.1.0
    condition: redis.enabled
  - name: kafka
    version: 0.1.0
    condition: kafka.enabled
  - name: etcd
    version: 0.1.0
    condition: etcd.enabled
  - name: skywalking
    version: 0.1.0
    condition: skywalking.enabled
  - name: user-svc
    version: 0.1.0
    condition: user-svc.enabled
  - name: chat-svc
    version: 0.1.0
    condition: chat-svc.enabled
  # ... 共 17 个
```

**关键字段**：
- `condition: postgres.enabled` —— 允许在 values 里 `postgres.enabled: false` 关闭
- 不写 `repository:` —— 因为是内联（Helm 自动从 `charts/postgres/` 找）

---

## 四、Values 全局配置

### 4.1 全局约定

```yaml
# charts/emotion-echo/values.yaml
global:
  secrets:
    postgresPassword: "postgres"   # dev 默认，生产覆盖
    postgresUser: "postgres"
    postgresDB: "emotion_echo"
    internalApiKey: "dev-key-change-me-please-32chars-min"
    apisixAdminKey: "dev-apisix-admin-key"
    jwtSecret: "dev-jwt-secret-please-rotate"
  images:
    registry: ""                   # 生产填 ACR 地址
    tag: "v0.1.0"
    pullPolicy: IfNotPresent

# 各子 chart 默认值（可被 -f 覆盖）
postgres:
  enabled: true
  storage: 10Gi
  port: 5432

user-svc:
  enabled: true
  replicaCount: 1
  image:
    repository: emotion-echo/user-svc
    tag: v0.1.0
```

### 4.2 子 chart 怎么读全局 secrets？

子 chart 模板里：

```yaml
# charts/emotion-echo/charts/user-svc/templates/secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: {{ include "user-svc.fullname" . }}
stringData:
  {{- if and .Values.global .Values.global.secrets }}
  POSTGRES_DSN: "host=postgres.ee-data.svc.cluster.local user={{ .Values.global.secrets.postgresUser }} password={{ .Values.global.secrets.postgresPassword }} dbname={{ .Values.global.secrets.postgresDB }} sslmode=disable search_path=emotion_echo_user"
  {{- end }}
```

**这是我们踩过的坑**：直接 `{{ .Values.global.secrets.postgresPassword }}` 会 nil 指针 panic（如果 global.secrets 没设）。必须用 `if and ...` 链式判断。

---

## 五、多环境策略：values-overlay 模式

### 5.1 三个环境三份 overlay

```yaml
# values.yaml       # 默认（kind / dev）
global:
  images:
    registry: ""
    tag: "v0.1.0"
    pullPolicy: IfNotPresent
postgres:
  enabled: true
  storage: 1Gi
xtts:
  enabled: false            # dev 默认关掉，太重

---
# values-staging.yaml
global:
  images:
    registry: emotion-echo-registry.cn-hangzhou.aliyuncs.com
    tag: "v0.2.0-rc1"
    pullPolicy: Always
postgres:
  enabled: true
  storage: 50Gi             # staging 用更大盘
xtts:
  enabled: true

---
# values-prod.yaml
global:
  images:
    registry: emotion-echo-registry.cn-hangzhou.aliyuncs.com
    tag: "v0.2.0"
    pullPolicy: Always
postgres:
  enabled: true
  storage: 200Gi            # 生产大盘 + HA
  replicaCount: 3           # StatefulSet 也可副本（要小心）
xtts:
  enabled: true
```

### 5.2 怎么用

```bash
# dev
helm install ee ./charts/emotion-echo -f values.yaml

# staging
helm install ee-staging ./charts/emotion-echo \
  -f values.yaml \
  -f values-staging.yaml      # 后面的覆盖前面的

# prod
helm install ee-prod ./charts/emotion-echo \
  -f values.yaml \
  -f values-prod.yaml
```

**3 个 release 在同一个集群里**（不同 ns），互不干扰。

### 5.3 Secret 怎么处理

**学习阶段**（我们项目）：把 Secret 默认值直接放 `values.yaml`，commit 进 git。
**生产阶段**：用 External Secrets Operator + 阿里云 KMS：

```yaml
# values-prod-secret.yaml（gitignored）
global:
  secrets:
    postgresPassword: "real-prod-password"
    internalApiKey: "real-32-char-key"
    apisixAdminKey: "real-apisix-key"
```

```bash
helm install ee-prod ./charts/emotion-echo \
  -f values.yaml \
  -f values-prod.yaml \
  -f values-prod-secret.yaml    # gitignored
```

---

## 六、Umbrella chart 的依赖顺序

Helm 装 umbrella 时，**子 chart 之间是按字母顺序装的**（kafka → postgres → redis → ...）。但**实际依赖**不是字母序：

| 实际依赖 | 字母序冲突 |
|---------|----------|
| chat-svc → postgres, kafka | chat < postgres |
| ai-svc → postgres, kafka, llm-service | ai < postgres |
| user-svc → postgres | user < postgres |

**为什么不会出问题**：K8s 的 readinessProbe + startupProbe 会保证 Pod 在依赖没就绪时**不接收流量**。`initContainer` 可以显式等待依赖。

**最佳实践**：用 `initContainer` 显式 wait：

```yaml
initContainers:
  - name: wait-for-postgres
    image: busybox:1.36
    command: ['sh', '-c', 'until nc -z postgres.ee-data 5432; do echo waiting for postgres; sleep 2; done']
```

我们 Stage 27 选择**依赖 K8s 的 readinessProbe**，不写 initContainer（简洁优先）。

---

## 七、Umbrella chart 与 ArgoCD / GitOps

当进入 Stage 28（GitOps 阶段），umbrella chart 的优势会放大：

```yaml
# argocd-app.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: emotion-echo
spec:
  source:
    repoURL: https://github.com/your-org/Emotion-Echo
    path: charts/emotion-echo
    helm:
      valueFiles:
        - values.yaml
        - values-prod.yaml
  destination:
    server: https://kubernetes.default.svc
    namespace: ee-app
  syncPolicy:
    automated:
      prune: true
      selfHeal: true
```

ArgoCD 监控 git 仓库，`git push` 后自动 `helm install --atomic`。
**一个 Application = 一个 umbrella = 整个系统**。

---

## 八、本节自检

1. **umbrella chart 解决什么痛点？**
2. **内联子 chart（charts/ 目录）和 `helm dependency update` 的核心区别是什么？**
3. **`values.yaml` 和 `values-prod.yaml` 同时 -f 时谁覆盖谁？**
4. **为什么子 chart 里读 global.secrets 要用 `if and` 链式判断？**
5. **Helm 装子 chart 是按字母序，但实际依赖不是字母序，怎么不出问题？**

<details>
<summary>📋 参考答案</summary>

1. 多 svc 系统的"一个 install 装一切"诉求；统一管理依赖；多环境 overlay。
2. 内联：子 chart 在 charts/ 目录，直接装；dependency update：从外部仓库（git/charts museum）拉。 内联适合定制多 + 离线场景；dependency update 适合复用社区 chart。
3. 后面的 -f 覆盖前面的。`values.yaml` 是默认，`values-prod.yaml` 是 prod 特定覆盖。
4. 因为 global.secrets 可能没设（比如某环境用了 external-secrets），直接 `.x` 会 nil panic。`if and a b c d` 链式保证所有前置都不为 nil 才渲染。
5. K8s 的 readinessProbe/startupProbe 机制保证 Pod 在依赖没就绪时不接收流量；必要时可用 initContainer 显式 wait。

</details>

---

## 九、推荐阅读

| 资源 | 链接 |
|------|------|
| Helm Umbrella Charts | https://helm.sh/docs/chart_best_practices/umbrella_charts/ |
| Kustomize vs Helm | https://kubectl.docs.kubernetes.io/guides/introduction/kustomize/ |
| External Secrets | https://external-secrets.io/ |
| ArgoCD | https://argo-cd.readthedocs.io/ |

---

> **下一步**：[05 APISIX vs Ingress-NGINX 网关选型实战](./05-apisix-vs-nginx.md) —— 我们为什么选了 APISIX 3.10+ 而不是社区默认的 Ingress-NGINX。