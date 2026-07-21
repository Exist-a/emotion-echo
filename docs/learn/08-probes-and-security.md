# 08 · 探针、SecurityContext、资源限额实战意义

> 系列：[07 网络与 DNS](./07-networking-and-dns.md) · **08 探针/安全/资源** · [09 踩坑全记录](./09-stage-27-pitfalls.md) ...

**适合谁**：写 yaml 时直接抄模板，不知道为什么模板要带 `securityContext.runAsNonRoot: true` / `readinessProbe` 的读者。
**读完你能**：解释 startup/readiness/liveness 三种探针的差异，说出"我们的应用为什么需要 runAsNonRoot"，能挑出错误的资源 limits 配置。

---

## 一句话总结

**探针 = K8s 怎么知道你的应用"活的"。SecurityContext = 你的容器能用哪些权限。资源 limits = 你的容器最多能吃多少 CPU/内存。**

三个字段决定了一个 Pod **是否接收流量 / 是否被重启 / 是否被 OOM Kill**。

---

## 一、3 种探针详解

### 1.1 全景对比

| 探针 | 何时跑 | 失败后果 | 典型配置 | 我们的应用 |
|------|--------|---------|---------|-----------|
| **startupProbe** | 启动阶段 | 重启容器 | 失败 N 次才重启 | 4 Go svc / ai-svc / llm |
| **readinessProbe** | 启动后持续 | 从 Service 摘掉 Pod | 失败 3 次就摘 | 所有 svc |
| **livenessProbe** | 启动后持续 | 重启容器 | 失败 3 次重启 | 所有 svc |

### 1.2 时序图

```
Pod 启动
   ↓
startupProbe 开始执行
   ↓
   失败 → 继续跑（达到 failureThreshold 才重启）
   ↓
启动成功 → 标记 startup succeeded
   ↓
readinessProbe 开始执行
   ↓
   成功 → Pod 加入 Service endpoint（接收流量）
   失败 → 从 endpoint 移除（不接收流量）
   ↓
livenessProbe 同时持续执行
   ↓
   失败 → 重启 Pod
```

### 1.3 详细解释

#### startupProbe

**作用**：给"启动慢"的应用一个宽限期。**只跑一次**（成功之后就不跑了）。

```yaml
startupProbe:
  httpGet:
    path: /health
    port: http
  failureThreshold: 30        # 最多失败 30 次
  periodSeconds: 10          # 每 10 秒一次
  # 最长宽限：30 * 10 = 300 秒 = 5 分钟
```

**什么时候用**：XTTS 模型加载要 2-3 分钟；ai-svc 第一次连 kafka + llm-service 要 30 秒。不配 startupProbe，livenessProbe 在启动期间就触发"假失败"，Pod 不断重启。

**我们项目**：
| svc | startupProbe failureThreshold | 原因 |
|-----|-----------------------------|------|
| user-svc / chat-svc / analytics-svc / assessment-svc | 6 × 5s = 30s | go-zero 启动快 |
| ai-svc | 12 × 5s = 60s | 要连 kafka + llm-service |
| llm-service | 12 × 5s = 60s | torch 模型初始化 |
| fer | 12 × 5s = 60s | OpenCV 加载 |
| sensevoice | 30 × 10s = 300s | 模型下载 |
| xtts | 36 × 10s = 360s | Coqui TTS 加载 + warmup |

#### readinessProbe

**作用**：告诉 K8s "我能不能接流量"。**持续跑**。

```yaml
readinessProbe:
  httpGet:
    path: /health
    port: http
  initialDelaySeconds: 5      # 启动后 5 秒开始
  periodSeconds: 10           # 每 10 秒一次
  failureThreshold: 3         # 失败 3 次标记不健康
```

**失败后果**：Pod **不重启**，只是从 Service endpoint 列表移除（不接收流量）。

**什么时候会失败**：
- 应用还在启动（DB 还没连上）
- 应用过载（goroutine 死锁）
- 下游依赖挂了（chat-svc → user-svc 时 user-svc 不健康）

#### livenessProbe

**作用**：判断"应用是不是死了，要重启"。**持续跑**。

```yaml
livenessProbe:
  httpGet:
    path: /health
    port: http
  initialDelaySeconds: 60     # 启动后 60 秒开始
  periodSeconds: 30           # 每 30 秒一次
  failureThreshold: 3         # 失败 3 次重启 Pod
```

**失败后果**：**重启 Pod**（不是 Service 摘流量）。

**注意点**：
- 不能太敏感：liveness 太严格 → Pod 频繁重启（雪崩）
- 不能太宽松：liveness 太松 → 真死了 K8s 不知道
- **不要** 把 readinessProbe 直接复制成 livenessProbe（语义不同）

### 1.4 `/health` 端点的设计

我们 Stage 27 的 `/health` 端点实现：

```go
// Stage 11-12: gin /health handler
func HealthHandler(c *gin.Context) {
    // 浅探活：只确认 HTTP server 活着
    c.JSON(200, gin.H{"status": "ok"})
}
```

**为什么是浅探活**：
- liveness 太深（探活时连 DB）→ DB 抖动 → Pod 被重启 → 雪崩
- readiness 可以深（探活时连 DB）→ DB 不通时摘流量

**最佳实践**：
- `/health` → liveness 用（浅）
- `/ready` → readiness 用（深，可连下游）

我们项目把所有探针都指向 `/health`（浅）以简化；**生产建议分两个端点**。

---

## 二、SecurityContext 详解

### 2.1 是什么

**SecurityContext = 容器/Pod 的安全配置**（用户、权限、文件系统）。

### 2.2 Pod 级 vs Container 级

```yaml
spec:
  securityContext:           # Pod 级（影响所有容器）
    runAsNonRoot: true
    runAsUser: 65532
    fsGroup: 65532
  containers:
    - name: app
      image: my-app
      securityContext:      # Container 级（只影响这个容器）
        allowPrivilegeEscalation: false
        readOnlyRootFilesystem: true
        capabilities:
          drop:
            - ALL
```

### 2.3 关键字段实战意义

| 字段 | 作用 | 为什么要 |
|------|------|---------|
| `runAsNonRoot: true` | 容器必须以非 root 用户运行 | 防止容器逃逸 |
| `runAsUser: 65532` | 指定用户 UID | nobody 用户（K8s 约定） |
| `fsGroup: 65532` | 卷挂载的 GID | 让 nobody 能读写挂载的卷 |
| `allowPrivilegeEscalation: false` | 禁止 setuid 二进制 | 防止权限升级攻击 |
| `readOnlyRootFilesystem: true` | 根文件系统只读 | 防止运行时写 /usr；逼着把可变目录挂 tmpfs |
| `capabilities.drop: [ALL]` | 丢掉所有 Linux capabilities | 默认 14 个 cap 全不要 |

### 2.4 我们的应用为什么能 runAsNonRoot

go-zero / Gin / FastAPI 都不需要 root 权限。
镜像里如果是用 `USER nobody` 构建的，直接能跑。
**没 USER 指令的镜像** 需要 Dockerfile 改：

```dockerfile
# 加这一行（在 ENTRYPOINT 之前）
RUN useradd -r -u 65532 -g nogroup nobody || true
USER 65532
```

### 2.5 readOnlyRootFilesystem + tmpfs

如果设 `readOnlyRootFilesystem: true`，**任何写 /tmp /var/cache /var/log 都会失败**。解法：

```yaml
volumeMounts:
  - name: tmp
    mountPath: /tmp
  - name: cache
    mountPath: /app/cache
volumes:
  - name: tmp
    emptyDir:
      medium: Memory           # 内存盘（性能好）
      sizeLimit: 100Mi
  - name: cache
    emptyDir:
      medium: Memory
      sizeLimit: 500Mi
```

**我们项目的妥协**：Stage 27 没全开 readOnlyRootFilesystem（go-zero 一些库要写 /tmp）。**生产建议**逐步开。

### 2.6 我们项目的 SecurityContext

```yaml
# 通用 svc 的 SecurityContext
securityContext:
  runAsNonRoot: true
  runAsUser: 65532
  runAsGroup: 65532
  fsGroup: 65532
  seccompProfile:
    type: RuntimeDefault
```

```yaml
# Container 级
securityContext:
  allowPrivilegeEscalation: false
  capabilities:
    drop:
      - ALL
```

**注意**：postgres / kafka / redis 这些**官方镜像**通常默认以 root 跑。**我们 Stage 27 没改这些**（学习阶段省事）。生产部署应该改镜像或加 initContainer 切用户。

---

## 三、资源 requests 与 limits

### 3.1 是什么

**requests = 调度时承诺的最小资源**（K8s 用这个决定 Pod 放哪个节点）
**limits = 运行时允许的最大资源**（超过会触发 OOM Kill / CPU throttling）

### 3.2 关键概念

| 概念 | 含义 |
|------|------|
| **CPU requests** | 调度用；1 CPU = 1000m（millicores）；100m = 0.1 核 |
| **CPU limits** | 超过会被 throttling（限流，不 Kill） |
| **memory requests** | 调度用 |
| **memory limits** | 超过会被 OOM Kill（直接重启） |

### 3.3 为什么必须设 resources

**不设 requests**：
- K8s 默认当 0 调度（Pod 可能挤到任何节点，包括资源已满的）
- 结果：Pod 启动后抢不到 CPU，性能不可预期

**不设 limits**：
- 内存泄漏的 Pod 会吃光节点内存 → 节点上其他 Pod 也被 OOM Kill
- 一个 Pod 影响整个节点（**吵闹邻居**问题）

### 3.4 怎么设（实战经验值）

我们项目 Stage 27 的 resources（来自 docker-compose 的 limits 换算）：

```yaml
# user-svc / chat-svc / analytics-svc / assessment-svc（轻量 Go svc）
resources:
  requests: { cpu: 100m, memory: 64Mi }
  limits:   { cpu: 500m, memory: 256Mi }

# ai-svc / llm-service（中等 Python）
resources:
  requests: { cpu: 200m, memory: 128Mi }
  limits:   { cpu: 1000m, memory: 512Mi }

# sensevoice（funasr + torch）
resources:
  requests: { cpu: 500m, memory: 512Mi }
  limits:   { cpu: 1500m, memory: 1536Mi }

# xtts（Coqui TTS + torch）
resources:
  requests: { cpu: 500m, memory: 1024Mi }
  limits:   { cpu: 2000m, memory: 3072Mi }
```

**黄金法则**：
- **requests** < **limits**（必须）
- **limits** ≈ docker-compose 的 `deploy.resources.limits`（一致性）
- **memory limits** 至少给 1.5x requests（避免 OOM）
- **CPU limits** 可不设（throttling 比 Kill 安全）

### 3.5 QoS Class（K8s 怎么决定先杀谁）

K8s 根据 resources 把 Pod 分三类：

| QoS | 规则 | OOM 优先级 |
|-----|------|-----------|
| **Guaranteed** | requests == limits（每个容器） | 最后杀 |
| **Burstable** | requests < limits | 中间 |
| **BestEffort** | requests 都没设 | **最先杀** |

我们 Stage 27 大部分 Pod 是 **Burstable**（requests < limits）。**生产关键服务**（postgres / apisix）应该做成 **Guaranteed**（requests == limits）。

### 3.6 Namespace 资源配额

```yaml
apiVersion: v1
kind: ResourceQuota
metadata:
  name: ee-app-quota
  namespace: ee-app
spec:
  hard:
    requests.cpu: "10"          # ns 总 CPU requests ≤ 10 核
    requests.memory: 20Gi      # ns 总内存 requests ≤ 20Gi
    limits.cpu: "20"
    limits.memory: 40Gi
    pods: "50"                  # 最多 50 个 Pod
```

**作用**：一个 ns 的 Pod 不能吃超过 quota 的资源（防止单个 ns 抢光集群）。

---

## 四、PodDisruptionBudget（PDB，可用性保险）

### 4.1 是什么

**PDB = "自愿中断时（如节点维护），至少保持 N 个 Pod 可用"**。

### 4.2 例子

```yaml
apiVersion: policy/v1
kind: PodDisruptionBudget
metadata:
  name: user-svc-pdb
spec:
  minAvailable: 1             # 至少 1 个 Pod 可用
  selector:
    matchLabels: { app: user-svc }
```

**触发场景**：
- 节点 drain（kubectl drain）
- 集群升级
- 自动伸缩缩容

**不设 PDB 后果**：节点 drain 时所有 Pod 同时被驱逐 → 服务**短时不可用**。

### 4.3 我们 Stage 27 没启用 PDB

学习阶段 replicas=1，PDB 没意义（minAvailable=1 等于 0 副本）。**生产**应该开。

---

## 五、HPA（HorizontalPodAutoscaler，水平自动扩缩）

### 5.1 是什么

**HPA = "CPU 超过 70%，自动加 Pod；降下来自动减 Pod"**。

### 5.2 例子

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
```

**前提**：要装 metrics-server（kubectl top 才能跑）。

我们 Stage 27 没启用 HPA（学习阶段），生产关键 svc（ai-svc / chat-svc）应该开。

---

## 六、我们项目的"实战组合"长这样

```yaml
# user-svc deployment.yaml 完整 SecurityContext + 探针 + 资源
apiVersion: apps/v1
kind: Deployment
metadata:
  name: user-svc
spec:
  replicas: 1
  selector:
    matchLabels: { app: user-svc }
  template:
    spec:
      securityContext:        # Pod 级
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
        fsGroup: 65532
      containers:
        - name: user-svc
          image: emotion-echo/user-svc:v0.1.0
          securityContext:    # Container 级
            allowPrivilegeEscalation: false
            capabilities:
              drop: [ALL]
          ports:
            - { name: http, containerPort: 8888 }
          startupProbe:
            httpGet: { path: /health, port: http }
            periodSeconds: 5
            failureThreshold: 6       # 30s 宽限
          readinessProbe:
            httpGet: { path: /health, port: http }
            periodSeconds: 10
            failureThreshold: 3
          livenessProbe:
            httpGet: { path: /health, port: http }
            initialDelaySeconds: 60
            periodSeconds: 30
            failureThreshold: 3
          resources:
            requests: { cpu: 100m, memory: 64Mi }
            limits:   { cpu: 500m, memory: 256Mi }
```

**这一段字段**等于："这个 Pod 用 nobody 跑；不需要 root；启动给 30s 宽限；启动后 5s 进流量；挂了 3 次重启；吃不超过 0.5 核 256MB"。

---

## 七、本节自检

1. **startupProbe / readinessProbe / livenessProbe 的区别？失败后果分别是什么？**
2. **为什么 XTTS 需要 360s 启动宽限？**
3. **`runAsNonRoot: true` 真的有必要吗？**
4. **`readOnlyRootFilesystem: true` 开了之后 `/tmp` 写不了怎么办？**
5. **requests 和 limits 的关系？**

<details>
<summary>📋 参考答案</summary>

1. startupProbe 只在启动期跑，失败重启；readinessProbe 持续跑，失败从 Service 摘流量但不重启；livenessProbe 持续跑，失败重启。
2. XTTS 第一次启动要加载 Coqui TTS 模型 ~1-2GB，从磁盘读到内存要 1-3 分钟；没宽限的话 livenessProbe 假失败导致 Pod 不断重启。
3. 容器以 root 跑意味着容器逃逸后能拿到节点 root 权限；runAsNonRoot 是基本安全屏障。
4. 把 /tmp /var/cache 等可变目录用 emptyDir（medium: Memory）挂载成内存盘。
5. requests < limits 是常规；requests 决定调度，limits 决定运行时上限；memory 超 limits 直接 OOM Kill。

</details>

---

## 八、推荐阅读

| 资源 | 链接 |
|------|------|
| Pod Lifecycle / Probes | https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/ |
| SecurityContext | https://kubernetes.io/docs/tasks/configure-pod-container/security-context/ |
| Resource Management | https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/ |
| Pod Quality of Service | https://kubernetes.io/docs/concepts/workloads/pods/pod-qos/ |
| HPA | https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/ |

---

> **下一步**：[09 Stage 27 踩坑全记录](./09-stage-27-pitfalls.md) —— 我们写 chart 时真实遇到的 10+ 个坑（helm template 语法、Windows 文件锁、YAML 解析、CRD 版本不匹配）。