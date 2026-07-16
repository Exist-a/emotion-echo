# Stage 21 · Kubernetes 化策略（针对阿里云 ACK + ACR）

**日期**：2026-07-15
**目标**：把 Stage 20 容器化的两个核心服务（emotion-llm-service + emotion-echo-ai-svc）+ 基础设施（postgres/redis/kafka/skywalking）部署到阿里云 ACK。
**目标读者**：项目维护者（学习 K8s 中）+ 后续接手部署的人。
**前置依赖**：Stage 20（容器化）+ Stage 11-19（gRPC 双协议 + mTLS + 健康检查）。

---

## 一、为什么需要 K8s

当前（Stage 20）的痛点（用本地 docker-compose 时遇到的真实问题）：

| 痛点 | 真实场景 | docker-compose 的局限 | K8s 的解决方案 |
|------|----------|---------------------|----------------|
| **单点故障** | 13 小时 postgres 容器挂了，整个系统瘫了 | 没有自动恢复（除非 `restart: always` 凑合） | ReplicaSet + Deployment 自动重启 |
| **滚动更新** | 改一行 yaml 需要手动 stop / start 容器 | `docker compose up` 是"全停再起"，有 downtime | Deployment 滚动更新，零 downtime |
| **扩缩容** | 用户高峰期 ai-svc 处理不过来 | 改 compose 副本数 + 重启 | HPA（基于 CPU/内存/自定义指标自动扩缩） |
| **多环境** | dev / staging / prod 配置差异 | 维护多个 compose 文件，容易漂移 | Kustomize overlay 或 Helm values |
| **跨主机** | 想把 postgres 放到另一台机器 | 跨主机网络难配置 | K8s CNI 网络（Pod 可跨节点通信） |
| **服务发现** | 容器名只在同一 compose 网络内有效 | 改 host 就得改所有 yaml | K8s Service + DNS 自动发现 |
| **配置管理** | 不同环境用不同 yaml 文件 | 配置改动需要重新 build image | ConfigMap + Secret + envFrom |
| **Secret 泄露** | `deploy/tls/*.key` 提交进 git | 私密性差 | SealedSecret / KMS 加密 |

**结论**：Stage 20 是"本地能跑"，Stage 21 是"生产稳定"。

---

## 二、部署目标（先确定目标，再选工具）

**目标环境**：阿里云华东1（杭州）/ 华北2（北京）任一 region。

**约束**：
- 一个项目 = 一套 K8s 集群（不要 dev/prod 共用）
- 至少 3 个 master + 3 个 worker 节点（生产高可用）
- worker 节点按业务负载扩缩（HPA + Cluster Autoscaler）
- 所有容器镜像存到 ACR（阿里云容器镜像服务）
- 日志存到 SLS（日志服务）
- 监控指标用 ARMS（应用实时监控服务，对接 Prometheus）
- 域名 + HTTPS：阿里云 DNS + 免费证书（Let's Encrypt via cert-manager）

---

## 三、集群选型（ACK vs 自建 vs EKS vs GKE）

### 3.1 候选方案

| 方案 | 形态 | 计费 | 适用 |
|------|------|------|------|
| **ACK 托管版**（推荐） | 阿里云托管 master + 自管 worker | master 免费，worker 按规格 | 99% 的项目 |
| **ACK 专有版** | 阿里云托管 master + 专有宿主机 worker | 更贵 | 安全合规要求高的金融 |
| **ACK Serverless**（ASK） | 无节点，Pod 直接跑在阿里云上 | 按 Pod 资源使用计费 | 流量波动大、运维成本敏感 |
| **自建 K8s**（kubeadm / k3s / rke2） | 完全自己管 | 服务器 + 人力 | 不推荐，托管更划算 |
| **EKS / GKE / AKS** | 海外托管 | 海外节点 | 项目出海才考虑 |

### 3.2 为什么选 ACK 托管版

**正面理由**：

1. **master 节点免运维**：apiserver / etcd / scheduler / controller-manager 阿里云帮管，省 1 个 SRE
2. **网络深度集成**：ACK 用阿里云 VPC + 阿里云 SLB（云负载均衡），Pod IP 在 VPC 内可直接访问
3. **存储一站式**：阿里云云盘 / NAS / OSS 直接挂到 Pod，不用自己部署 Ceph / MinIO
4. **国内访问速度快**：相比自建需要单独买服务器 + 备案 + 拉镜像慢（Docker Hub 被墙）
5. **成本可预期**：master 免费，worker 按规格按小时计费，不用预留

**反面理由（也需要知道）**：
- master 不暴露 SSH，无法 debug master（但一般不需要）
- 部分高级网络策略需企业版
- 跨 region 集群迁移较麻烦（先不考虑）

### 3.3 为什么不用 ASK（Serverless）

Serverless 看起来最省事，但我们不用：
- ✅ 优点：免运维、秒级扩容、按用量付费
- ❌ 缺点：
  - 不能跑 DaemonSet（SkyWalking agent 没法跑每个节点一份）
  - 不能用 hostPort（port 已固定）
  - cold start 问题（第一次拉镜像慢）
  - SkyWalking OAP / Kafka / Postgres 这些**有状态服务**上 Serverless 反而更难管

**结论**：ACK 托管版（标准）+ 自己管 worker。

### 3.4 集群规格建议（按业务量预测）

| 节点角色 | 数量 | 规格 | 用途 |
|----------|------|------|------|
| Master（托管） | 3 | 阿里云托管 | 不占费用 |
| Worker - 系统 | 3 | 4C8G | SkyWalking OAP, Kafka |
| Worker - 业务 | 2-10（自动扩） | 8C16G | ai-svc, llm-service, 其他 |
| Worker - 数据库 | 2 | 8C16G + SSD | postgres, redis |

学习用最低配即可（约 1500-3000 元/月）。

---

## 四、镜像仓库：ACR vs Docker Hub vs Harbor

### 4.1 候选对比

| 仓库 | 国内访问 | 私有仓库 | 集成 ACK | 与阿里云其他服务联动 |
|------|----------|----------|----------|----------------------|
| **ACR（推荐）** | ✅ 走阿里云内网 | ✅ 命名空间 + 访问凭证 | ✅ 一键拉取，VPC 内网走 | ✅ 安全扫描、镜像签名 |
| Docker Hub | ❌ 国内慢，被墙 | ✅ 私有仓库限 1 个 | ⚠️ 需配 imagePullSecret | ⚠️ 弱 |
| Harbor（自建） | ✅ 自托管可控 | ✅ 完全控制 | ⚠️ 要单独部署 + 维护证书 | ❌ 弱 |
| 腾讯云 TCR | ✅ 国内快 | ✅ | ⚠️ 跨云不能用 | ⚠️ 仅腾讯云 |

### 4.2 为什么选 ACR

1. **国内速度**：阿里云内网拉镜像 1-2 秒（Docker Hub 经常 timeout）
2. **VPC 内网访问**：Pod 不走公网拉镜像，节省流量 + 提高安全
3. **自动同步**：可从 Docker Hub 同步公开镜像（如 SkyWalking）
4. **企业版安全扫描**：自动扫描 CVE，免费版也有基础扫描
5. **与 ACK 深度集成**：在 ACK 创建节点时自动配置 imagePullSecret

### 4.3 ACR 命名空间设计

```
emotion-echo/                       # 个人版 namespace
├── emotion-echo/llm-service       # 业务镜像
├── emotion-echo/ai-svc
└── emotion-echo/web               # 前端（Nuxt）
```

每个环境（dev / staging / prod）用不同 tag：
```
emotion-echo/llm-service:v0.1.0
emotion-echo/llm-service:v0.1.0-dev
emotion-echo/llm-service:v0.1.0-prod
```

学习用简单的 tag 就行，正式生产要做 tag 规范。

---

## 五、网络方案：ACK 网络插件 + Ingress

### 5.1 网络插件（CN）

| 插件 | 性能 | 复杂度 | 适用 |
|------|------|--------|------|
| **Terway（ACK 默认，推荐）** | 高 | 中 | 阿里云 VPC 原生集成，Pod IP 走 VPC |
| Flannel | 中 | 低 | 简单场景 |
| Calico | 高 | 高 | 复杂网络策略 |

ACK 默认装 **Terway**，学习不用纠结就用默认的。

### 5.2 Service 三种类型

| 类型 | 用途 | Emotion-Echo 用法 |
|------|------|-------------------|
| **ClusterIP**（默认） | 集群内部通信 | ai-svc → postgres、ai-svc → emotion-llm-service |
| **NodePort** | 简单外网访问 | 不推荐生产用 |
| **LoadBalancer** | 云厂商负载均衡 | APISIX / API Gateway 用 |

### 5.3 Ingress 选型

| 方案 | 优点 | 缺点 | 学习路径 |
|------|------|------|----------|
| **Ingress-NGINX** | 社区主流，文档多 | 配置稍复杂 | ⭐ 推荐学习首选 |
| **APISIX Ingress**（推荐） | 已有 APISIX 经验，功能丰富（限流/认证） | 学习曲线略高 | ⭐⭐ 我们项目已有 APISIX 经验 |
| Traefik | 配置简单 | 国内资料少 | ⭐⭐ |
| 阿里云 SLB Ingress | 阿里云原生 | 高级功能收费 | ⭐⭐⭐ |

**为什么选 APISIX Ingress**：
1. 我们 Stage 6 + Stage 7 已经在用 APISIX，熟悉度高
2. APISIX Ingress 是 APISIX 的 K8s controller，复用现有路由配置经验
3. 支持我们已经在用的 jwt-auth、限流、prometheus 插件
4. eBPF 性能比 NGINX Ingress 高 30%

但学习阶段建议先用 **Ingress-NGINX**（资料最多），生产再切 APISIX Ingress。

### 5.4 mTLS / gRPC 内部通信

| 方案 | 用法 |
|------|------|
| **APISIX Ingress + sidecar** | 复杂，已写好 mTLS 证书就保留 Stage 18 |
| **Istio service mesh**（P2 阶段再说） | 自动 mTLS，但学习曲线陡 |

**学习阶段保留 Stage 18 的 mTLS 证书挂载**就够了。

---

## 六、部署工具：Helm vs Kustomize vs ArgoCD

### 6.1 三者对比

| 工具 | 本质 | 学习曲线 | 适用场景 |
|------|------|----------|----------|
| **Helm**（推荐） | K8s 模板 + values | 中 | 通用部署，几乎所有项目都用 |
| Kustomize | YAML overlay | 低 | 简单多环境配置 |
| ArgoCD | GitOps 控制器 | 高 | 长期维护 + 团队协作 |

### 6.2 Helm vs Kustomize 深度对比

| 维度 | Helm | Kustomize |
|------|------|----------|
| 本质 | 模板（Go template）| YAML patch |
| 复用 | chart 复用 | base + overlay |
| 参数化 | values.yaml（强大）| 简单的替换 |
| 多人协作 | 容易（chart 库） | 容易（git diff 友好） |
| 调试 | helm template 渲染 | kubectl kustomize 渲染 |
| 包管理 | helm repo（依赖管理） | 无 |
| 学习 | 推荐 | 入门简单 |

**结论**：**Helm 是行业标准**，学习用 Helm 受益最大。

### 6.3 Helm chart 结构（推荐写法）

```
emotion-echo/
├── charts/                       # 自己写的 charts
│   ├── emotion-llm-service/
│   │   ├── Chart.yaml
│   │   ├── values.yaml           # 默认值
│   │   ├── values-dev.yaml       # dev 覆盖
│   │   ├── values-prod.yaml      # prod 覆盖
│   │   └── templates/
│   │       ├── deployment.yaml
│   │       ├── service.yaml
│   │       ├── configmap.yaml
│   │       ├── secret.yaml
│   │       ├── ingress.yaml
│   │       ├── hpa.yaml
│   │       ├── serviceaccount.yaml
│   │       ├── poddisruptionbudget.yaml
│   │       └── _helpers.tpl
│   ├── ai-svc/
│   │   └── ...（同结构）
│   └── postgres/
│       └── ...（同结构）
```

### 6.4 GitOps / ArgoCD（可选进阶）

学完 Helm 后学 GitOps：

- 代码 push 到 Git → ArgoCD 自动同步到 K8s
- 优势：审计、回滚、PR-review
- 学习曲线：⭐⭐⭐（但你 K8s 熟了后这个最有价值）

**学习路径：先 Helm → 再 ArgoCD**。

---

## 七、Secret 管理：为什么要单独搞

### 7.1 痛点

- 当前 `deploy/tls/*.key` 直接 commit 进 git
- 任何人 clone 都能拿到私钥 → **生产事故级风险**
- K8s Secret 默认是 base64 编码（**不是加密**），etcd 里明文存

### 7.2 候选方案（按推荐度）

| 方案 | 实现 | 学习曲线 |
|------|------|----------|
| **阿里云 KMS 加密 + External Secrets**（推荐） | ACK 自带 KMS 插件 + External Secrets Operator | ⭐⭐ |
| **Sealed Secrets** | 用 kubeseal 加密，commit 密文即可 | ⭐ |
| HashiCorp Vault | 业界标准 Vault，功能最强 | ⭐⭐⭐ |
| 直接用 K8s Secret（**不推荐生产**） | 简单但明文 | — |

### 7.3 推荐：ExternalSecrets + 阿里云 KMS

```yaml
# ExternalSecret CRD，把"阿里云 KMS 中的密钥"自动同步到 K8s Secret
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: llm-api-key
spec:
  secretStoreRef:
    kind: ClusterSecretStore
    name: aliyun-kms
  target:
    name: llm-secret       # 同步到 K8s Secret
  data:
    - secretKey: api-key
      remoteRef:
        key: emotion-echo/llm-api-key
        property: value
```

**好处**：
- 密钥只在阿里云 KMS 内存活
- 集群里只有同步的 K8s Secret（仍然 base64，但有 RBAC 控制访问）
- 阿里云控制台有完整审计日志

**学习用 Sealed Secret（更简单）**：
```bash
# 加密
echo -n "my-secret-key" | kubeseal --cert pub-cert.pem > sealed-secret.json

# 部署
kubectl apply -f sealed-secret.json
```

---

## 八、可观测性（日志 + 监控 + 告警）

### 8.1 日志方案

| 方案 | 优势 | 学习曲线 |
|------|------|----------|
| **阿里云 SLS**（推荐） | 阿里云原生、K8s 集成、便宜 | ⭐ |
| 自建 EFK（Elasticsearch + Fluentd + Kibana） | 灵活 | ⭐⭐⭐ |
| Loki + Grafana | 轻量 | ⭐⭐ |

**推荐原因**：
- ACK 控制台一键开启日志组件
- SLS 价格低（0.3 元/GB/月）
- 自动按 namespace / pod 索引
- 支持 K8s 上下文查看（哪个 pod 在跑）

**集成方式**：DaemonSet 跑 logtail（阿里云官方） → 容器 stdout 直接收集。

**Stage 20-2 已经输出 JSON 日志** → SLS 可以按字段索引、过滤、告警。

### 8.2 监控方案

| 方案 | 优势 | 学习曲线 |
|------|------|----------|
| **ARMS Prometheus 监控**（推荐） | 阿里云托管、ARMS Grafana 集成 | ⭐ |
| 自建 Prometheus + Grafana | 完全控制 | ⭐⭐⭐ |

**集成步骤**：
1. ACK 控制台开启 ARMS Prometheus
2. Stage 20-5 + Stage 20-P0-2 暴露的 `/metrics` 自动被 Prometheus 抓取
3. ARMS 自动生成 ServiceMonitor CRD（K8s Prometheus 声明式抓取）
4. ARMS Grafana 看 panel

**关键指标**（已实现）：
- `ai_svc_http_requests_total{method, path, status}` — 请求量
- `ai_svc_http_request_duration_seconds` — P50/P99 延迟
- `llm_analyze_total{emotion, status}` — 业务量
- `llm_grpc_requests_total{method, status}` — gRPC 量

### 8.3 告警方案

**阿里云 ARMS 告警**（推荐）：
- 触发条件：`llm_analyze_total{status="err"}` 增长 > 0.1/s
- 触发条件：`ai_svc_http_request_duration_seconds` P99 > 1s
- 触发条件：K8s Pod restart > 3/分钟
- 通知方式：钉钉/企业微信/邮件/PagerDuty

**学习用免费的 webhook + 钉钉机器人**就行。

---

## 九、CI/CD（自动化）

### 9.1 推荐流程

```
代码 push → GitHub Webhook → 阿里云云效 / GitHub Actions →
  docker buildx → ACR（push 镜像，tag=sha+版本）→
  helm upgrade --install → ACK（更新）→
  ArgoCD 自动同步（可选 GitOps）
```

### 9.2 学习路径

| 阶段 | 工具 | 复杂度 |
|------|------|--------|
| Step 1 | 在本机 `docker buildx build --push` → `kubectl apply -f` | 0 |
| Step 2 | GitHub Actions + ACR | ⭐ |
| Step 3 | 阿里云云效 Flow（更适合国内）| ⭐⭐ |
| Step 4 | ArgoCD GitOps（最终形态）| ⭐⭐⭐ |

### 9.3 GitHub Actions 示例骨架

```yaml
# .github/workflows/deploy.yml
name: build & deploy
on:
  push:
    branches: [main]

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Login to ACR
        uses: aliyun/acr-login@v1
        with:
          region: cn-hangzhou
          registry: emotion-echo-registry.cn-hangzhou.aliyuncs.com
      - name: Build & push ai-svc
        run: |
          docker buildx build \
            -f emotion-echo-ai-svc/Dockerfile \
            -t emotion-echo-registry.cn-hangzhou.aliyuncs.com/emotion-echo/ai-svc:${{ github.sha }} \
            --push .
```

---

## 十、资源 + HPA（自动扩缩）

### 10.1 资源配额（必须设，否则 OOM 被 K8s kill）

```yaml
resources:
  requests:        # 调度用
    cpu: "200m"    # 0.2 核
    memory: "256Mi"
  limits:          # 硬上限
    cpu: "1000m"   # 1 核
    memory: "512Mi"
```

### 10.2 HPA（Horizontal Pod Autoscaler）

```yaml
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: ai-svc-hpa
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: ai-svc
  minReplicas: 2
  maxReplicas: 10
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
    - type: Pods
      pods:
        metric:
          name: ai_svc_http_requests_per_second
        target:
          type: AverageValue
          averageValue: "100"
```

**三种触发扩缩的信号**：
1. CPU 使用率（最常用）
2. 内存使用率（小心，Java 应用内存波动大）
3. 自定义业务指标（QPS、消息堆积量）

### 10.3 PodDisruptionBudget（PDB，保护可用性）

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: ai-svc-pdb
spec:
  minAvailable: 1       # 至少保留 1 个 pod 可用
  selector:
    matchLabels:
      app: ai-svc
```

作用：节点维护时，不会一次性杀掉所有 Pod，保证高可用。

---

## 十一、StatefulSet（有状态服务）

| 服务 | 类型 | 原因 |
|------|------|------|
| postgres | StatefulSet + PVC | 数据持久化 |
| redis | StatefulSet + PVC | AOF 持久化 |
| kafka | StatefulSet + PVC | KRaft log 目录 |
| skywalking-oap | Deployment + PVC | H2/ES 存储 |
| ai-svc | Deployment | 无状态 |
| llm-service | Deployment | 无状态 |

**为什么不直接用云 RDS**（生产推荐）：
- 初期为了快速验证，用 StatefulSet 跑容器版
- 长期建议用 **阿里云 RDS for PostgreSQL**（高可用、备份、监控都现成）
- Redis 用 **阿里云 Tair**（兼容 Redis 协议，更稳定）
- Kafka 用 **阿里云消息队列 Kafka 版**（managed）

学习阶段用容器化 DB **完全 OK**，生产前再迁。

---

## 十二、实施路线图（按学习路径）

### Phase 1：K8s 基础 + 本地（1-2 周）
- [ ] 安装 Docker Desktop（已有）+ minikube / kind / OrbStack（Mac）
- [ ] 跑通 `kubectl apply -f deployment.yaml` 部署 ai-svc
- [ ] 实践：Deployment / Service / ConfigMap / Secret / Ingress
- [ ] 实践：`kubectl logs / exec / describe / port-forward`

### Phase 2：Helm（1 周）
- [ ] 写一个简单 chart（Deployment + Service + Ingress）
- [ ] 用 `helm install` 部署 emotion-llm-service
- [ ] 用 `helm upgrade` 滚动更新
- [ ] 用 `helm rollback` 回滚

### Phase 3：ACK 上线（1-2 周）
- [ ] 创建 ACK 集群（pro 版）
- [ ] 开通 ACR 个人版 / 企业版
- [ ] GitHub Actions 自动构建 → push 到 ACR
- [ ] Helm 部署到 ACK
- [ ] 配置 Ingress + 域名 + HTTPS
- [ ] 开启 SLS 日志 + ARMS 监控

### Phase 4：生产化（2-3 周）
- [ ] External Secrets + KMS
- [ ] HPA + Cluster Autoscaler
- [ ] PodDisruptionBudget
- [ ] NetworkPolicy（Pod 间网络隔离）
- [ ] ResourceQuota + LimitRange

### Phase 5：进阶（持续）
- [ ] ArgoCD GitOps
- [ ] Chaos engineering（chaos-mesh）
- [ ] 灰度发布（Argo Rollouts）
- [ ] 多 region 灾备

---

## 十三、成本估算（学习用最低配）

### 13.1 ACK 标准版最低配

| 资源 | 规格 | 月成本 |
|------|------|--------|
| Master（托管） | — | 免费 |
| Worker × 3 | 2C4G（抢占式） | ~150 元 |
| Worker × 2（业务） | 4C8G | ~400 元 |
| SLB（API 网关） | 性能共享型 | ~30 元 |
| 阿里云 RDS（可选） | 1C2G | ~150 元 |
| ACR 企业版（可选） | 基础版 | ~30 元 |
| SLS 日志 | 5GB/月 | ~1 元 |
| ARMS Prometheus | 基础版 | 免费 |
| **小计** | | **约 700-800 元/月** |

学习阶段 1-2 个月累计 ~1500-2000 元。

### 13.2 成本优化技巧

1. **抢占式实例**：ecs.t6 / ecs.t5 便宜 70%（但可能被回收）
2. **HPA + Cluster Autoscaler**：低峰期缩到 2 个 worker
3. **预留实例券**：长期使用买 1 年可省 30-50%
4. **不用 RDS**：先容器版 postgres，生产再迁
5. **SLS 生命周期**：30 天后自动归档

---

## 十四、踩坑清单（学完这章记得回来勾）

### 14.1 镜像相关

- [ ] build context 必须为仓库根（ai-svc 引用 ../shared）
- [ ] alpine 镜像用 `apk add tini`，**不能用 github 下载的 glibc tini**
- [ ] go-zero yaml 的 `${VAR:-true}` 不能给 bool 字段用（type mismatch）
- [ ] go-zero yaml 不支持 list 字段（`Kafka.Brokers`）env 替换

### 14.2 K8s 常见坑

- [ ] **readiness probe 失败** → pod 永远 not ready → Service 不发流量
- [ ] **liveness probe 太严格** → pod restart loop（建议 start_period 留够）
- [ ] **resources 没设** → pod 被 OOMKilled 但日志看不出来
- [ ] **PVC 没设 storage class** → 一直 Pending
- [ ] **Service selector 不匹配** → pod 一直 ClusterIP 没流量
- [ ] **IngressClass 不匹配** → 域名能解析但 404
- [ ] **imagePullSecrets 忘了** → ImagePullBackOff
- [ ] **affinity 太严格** → pod 永远 Pending

### 14.3 阿里云特有

- [ ] **备案**：用阿里云必须做 ICP 备案，否则 80 端口被封
- [ ] **VPC 内网**：Pod 在 VPC 内访问公网需 NAT 网关
- [ ] **SLS 地域**：必须和 ACK 同一个 region
- [ ] **ACR 跨账号**：个人 ACR → 企业 ACR 需要迁移

---

## 十五、学习资源

### 15.1 书籍（系统学习）

| 书 | 推荐度 | 时长 |
|----|--------|------|
| 《Kubernetes 权威指南》（第 5 版）| ⭐⭐⭐ | 1 个月通读 |
| 《深入剖析 Kubernetes》| ⭐⭐⭐ | 2 周 |
| 《Kubernetes in Action》| ⭐⭐⭐ | 1 个月 |

### 15.2 在线课程

- **阿里云官方 ACK 文档**（必看）：https://help.aliyun.com/ack
- **华为云 / 腾讯云 K8s 课程**（B 站免费）
- **Kubernetes The Hard Way**（GitHub 经典教程）

### 15.3 实战项目

- 跟着本项目走（最实际）
- [killerngshen/app-k8s](https://github.com/killerngshen/app-k8s) - 类似栈实战
- 部署一个简单的 nginx + redis web app

### 15.4 常用命令速查

```bash
# 集群信息
kubectl cluster-info
kubectl get nodes -o wide

# 资源操作
kubectl get pods -A                      # 所有命名空间的 pod
kubectl get svc,ing                      # 所有服务和 ingress
kubectl describe pod <name>              # 排错必看
kubectl logs -f <pod> --tail 100        # 实时日志
kubectl exec -it <pod> -- sh             # 进容器
kubectl port-forward svc/<name> 8080:80  # 本地访问

# Helm
helm list -A                             # 所有 release
helm status <release>                    # 详情
helm upgrade --install <release> <chart> # 升级或安装
helm uninstall <release>                 # 卸载
helm template <chart>                    # 渲染模板（dry-run）
```

---

## 十六、决策总结（一句话回顾）

| 决策点 | 选择 | 一句话理由 |
|--------|------|-----------|
| 集群 | **ACK 托管版** | 国内快、master 免费、深度集成阿里云 |
| 镜像仓库 | **ACR** | 国内访问快、VPC 内网免费流量、与 ACK 集成 |
| 网络插件 | **Terway**（默认）| 阿里云原生，不用纠结 |
| Ingress | **Ingress-NGINX（学习）/ APISIX Ingress（生产）**| 学习找资料用 NGINX，生产复用 APISIX 经验 |
| 部署工具 | **Helm** | 行业标准、所有 K8s 项目都用 |
| Secret | **External Secrets + KMS（生产）/ Sealed Secret（学习）**| 学习曲线友好 |
| 日志 | **阿里云 SLS** | 一键集成、按 GB 便宜 |
| 监控 | **ARMS Prometheus** | 阿里云托管、Stage 20-5 已经埋好 |
| CI/CD | **GitHub Actions → ACR → Helm → ACK**（学习）| 渐进式，熟练后换 ArgoCD |
| 数据库 | **容器版本（学习）/ 阿里云 RDS（生产）**| 学习阶段快，生产阶段稳 |

---

## 十七、Stage 22+ 候选

落地 K8s 后继续推进：
- **Stage 22**：CI/CD（GitHub Actions → ACR → 自动部署）
- **Stage 23**：可观测性完善（ARMS 告警规则 + Grafana dashboard）
- **Stage 24**：Service Mesh 探索（Istio / OpenTelemetry）
- **Stage 25**：多 region 容灾
- **Stage 26**：混沌工程（chaos-mesh）
- **Stage 27**：性能压测（k6 + ghz）

---

**下一步建议**：先在本地跑通 **Phase 1（minikube + kubectl apply）**，再用 Helm 把 emotion-llm-service 部署上去，把所有命令跑一遍，再考虑上 ACK。**不要跳过 Phase 1 直接上 ACK**，云上调试更慢。