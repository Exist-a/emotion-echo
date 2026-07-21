# 06 · K8s 资源对象逐个解构

> 系列：[05 网关选型](./05-apisix-vs-nginx.md) · **06 资源解构** · [07 网络模型](./07-networking-and-dns.md) ...

**适合谁**：知道 K8s 大概念但分不清"Deployment 和 StatefulSet 差在哪""ConfigMap 和 Secret 区别是什么"的读者。
**读完你能**：看 chart template 一眼看出每个资源是什么角色，能自己挑"postgres 该用 Deployment 还是 StatefulSet"。

---

## 一句话总结

**K8s 资源 = 你能 yaml 描述的每一种"想要的状态"**。常见 7 种（Pod / Deployment / StatefulSet / Service / ConfigMap / Secret / PVC），每个管一类事。这一篇把 7 个全部拆开，看 Emotion-Echo 怎么用。

---

## 一、决策树：我该用哪个资源？

```
┌─ 是"一组相同 Pod 的控制器"吗？
│
├─ 是 → Pod 里的数据重要吗？
│       │
│       ├─ 不重要（无状态） → Deployment  + ClusterIP Service
│       │   例：user-svc / chat-svc / web
│       │
│       └─ 重要（数据不能丢） → StatefulSet + headless Service + PVC
│           例：postgres / kafka / etcd
│
├─ 不是 → 是"外部 IP"吗？
│        │
│        ├─ 内部访问 → Service (ClusterIP)
│        ├─ 浏览器访问 → Service (NodePort/LoadBalancer) + Ingress
│        └─ 外部直接连 Pod → 不推荐（用 Service）
│
├─ 是"配置"吗？
│   │
│   ├─ 明文配置 → ConfigMap
│   └─ 敏感配置 → Secret
│
└─ 是"存储"吗？
    │
    ├─ 临时 → emptyDir
    └─ 持久 → PersistentVolumeClaim (PVC)
```

---

## 二、Pod（最小调度单元）

### 2.1 是什么

**Pod = 一组共享网络和存储的容器集合**。K8s 调度、扩缩、IP 分配的最小单位。

### 2.2 关键事实

- Pod 内所有容器共享**同一个 Pod IP**
- Pod 内所有容器共享**网络命名空间**（localhost 互通）
- Pod 有自己的**存储卷**（同一 Pod 内所有容器可挂）
- Pod 有生命周期（创建 → 运行 → 销毁），是**临时**的

### 2.3 你**几乎从不直接写 Pod**

直接 `kind: Pod` 写出来，K8s 不会自动重启它。**99% 场景下你写 Deployment/StatefulSet**，让它们去创建 Pod。

### 2.4 Pod 里的容器共享网络（sidecar 模式）

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: app-with-sidecar
spec:
  containers:
    - name: app
      image: my-app
    - name: log-collector       # sidecar：收集 app 日志
      image: fluentd
      volumeMounts:
        - name: logs
          mountPath: /var/log/app
  volumes:
    - name: logs
      emptyDir: {}
```

---

## 三、Deployment（无状态应用管理者）

### 3.1 是什么

**Deployment = "我要 N 个相同 Pod" + "滚动更新策略" + "回滚记录"**。

### 3.2 核心字段

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: user-svc
spec:
  replicas: 1                       # 副本数
  selector:
    matchLabels:
      app: user-svc                 # 匹配 Pod 标签
  strategy:
    type: RollingUpdate             # 默认滚动更新
    rollingUpdate:
      maxSurge: 1                   # 最多多 1 个
      maxUnavailable: 0             # 最多少 0 个（保证零 downtime）
  template:                          # Pod 模板（创建 Pod 用）
    metadata:
      labels:
        app: user-svc
    spec:
      containers:
        - name: user-svc
          image: emotion-echo/user-svc:v0.1.0
          ports:
            - containerPort: 8888
```

### 3.3 Deployment 干的事

| 行为 | 触发时机 | 例子 |
|------|---------|------|
| **创建 Pod** | apply 后 / 现有 Pod < replicas | replicas=3 但只有 2 → 启动第 3 个 |
| **删除多余 Pod** | 现有 Pod > replicas | 你 `kubectl scale --replicas=1` → 删 2 个 |
| **滚动更新** | image / env / template 改了 | `kubectl set image deployment/user-svc ...` |
| **回滚** | `kubectl rollout undo` | image 错了，一键回到上一版本 |

### 3.4 我们项目哪些用 Deployment

| svc | 为什么 Deployment |
|-----|------------------|
| user-svc | 无状态，Pod 挂了重启就行 |
| chat-svc | 无状态（数据在 postgres） |
| analytics-svc | 无状态 |
| assessment-svc | 无状态 |
| ai-svc | 无状态（数据走 postgres + kafka） |
| llm-service | 无状态 |
| fer / sensevoice / xtts | 加载模型，无状态（模型缓存用 PVC 但不绑 Pod） |
| web | Nuxt SSR，但 K8s 部署时是无状态 SPA |
| apisix | 网关，多副本 + readiness 切换 |
| skywalking-ui / skywalking-oap | skywalking-oap 有状态 → 我们 Stage 27 用 StatefulSet |

---

## 四、StatefulSet（有状态应用管理者）

### 4.1 跟 Deployment 差在哪

| 特性 | Deployment | StatefulSet |
|------|-----------|-------------|
| **Pod 名** | 随机（`user-svc-7d5b8c9f-x2k`） | 有序（`postgres-0`, `postgres-1`） |
| **Pod 标识稳定** | ❌ 重启后名字变 | ✅ 重启后名字不变 |
| **PVC** | 多 Pod 共享 1 个 PVC（容易冲突） | 每个 Pod 独立 PVC（`data-postgres-0`） |
| **网络标识** | 通过 Service 负载均衡 | 每个 Pod 独立 DNS（`postgres-0.postgres-headless`） |
| **滚动更新** | 默认任意顺序 | **默认逆序**（先更新最后一个） |

### 4.2 完整 StatefulSet 示例（postgres）

```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: postgres
  namespace: ee-data
spec:
  serviceName: postgres-headless      # 关联 headless Service
  replicas: 1
  selector:
    matchLabels: { app: postgres }
  template:
    metadata:
      labels: { app: postgres }
    spec:
      containers:
        - name: postgres
          image: postgres:15-alpine
          ports:
            - { name: postgres, containerPort: 5432 }
          env:
            - name: POSTGRES_PASSWORD
              valueFrom:
                secretKeyRef:
                  name: postgres-secret
                  key: password
          volumeMounts:
            - name: data
              mountPath: /var/lib/postgresql/data
  # PVC 模板：每个 Pod 一个独立 PVC
  volumeClaimTemplates:
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        resources:
          requests:
            storage: 10Gi
```

### 4.3 为什么 postgres 必须 StatefulSet

postgres 的数据目录 `/var/lib/postgresql/data` 必须**绑定到具体哪个 Pod**：

| 用 Deployment 的后果 | 用 StatefulSet 的好处 |
|---------------------|---------------------|
| Pod 重建 → PVC 可能被挂到另一个 Pod → 数据错乱 | Pod-0 重启后还是 pod-0，PVC 还是 data-postgres-0 |
| 多副本时，多 Pod 共享一个 PVC → **数据竞争** | 每个 Pod 独立 PVC，互不干扰 |
| 不能做主从（Pod 名字随机） | 可以有 `postgres-0`（主）+ `postgres-1`（从） |

### 4.4 我们项目哪些用 StatefulSet

| 组件 | 为什么 |
|------|--------|
| postgres | 数据库 |
| kafka | 日志持久 + 副本顺序 |
| etcd | APISIX 配置存储 |
| skywalking-oap | H2 文件模式数据 |

### 4.5 重要：单实例 StatefulSet 不算"高可用"

`replicas: 1` 的 StatefulSet 跟 `Deployment` 在可用性上**没本质区别**。Stage 27 学习阶段 1 副本足够，生产要 3 副本 + Patroni/Repmgr 才能 HA。

---

## 五、Service（虚拟 IP + DNS）

### 5.1 是什么

**Service = 一组 Pod 的稳定访问入口**（虚拟 IP + DNS 名字）。

### 5.2 三种类型

| 类型 | 用途 | 我们项目用 |
|------|------|-----------|
| **ClusterIP**（默认） | 集群内部访问 | 所有内部 svc（user-svc, chat-svc, ...） |
| **NodePort** | 节点端口暴露（学习用） | apisix-gateway → 宿主机 9080 |
| **LoadBalancer** | 云厂商负载均衡（生产用） | 生产 ACK 用 SLB |

### 5.3 ClusterIP 示例（最常见）

```yaml
apiVersion: v1
kind: Service
metadata:
  name: user-svc
  namespace: ee-app
spec:
  selector: { app: user-svc }      # 选 app=user-svc 的 Pod
  ports:
    - name: http
      port: 8888                    # Service 自己的端口
      targetPort: 8888              # Pod 容器端口
```

**自动效果**：
- K8s 分配 ClusterIP（如 10.96.45.78）
- K8s DNS 注册 `user-svc.ee-app.svc.cluster.local` → 10.96.45.78
- kube-proxy 配 iptables / ipvs，把 ClusterIP 流量转到任意一个健康 Pod

### 5.4 headless Service（StatefulSet 用）

```yaml
apiVersion: v1
kind: Service
metadata:
  name: postgres-headless
spec:
  clusterIP: None                   # 关键：None = headless
  selector: { app: postgres }
  ports:
    - { name: pg, port: 5432 }
```

**差异**：headless Service **不分配 ClusterIP**，DNS 直接返回所有 Pod 的 IP：

| 普通 Service DNS | headless Service DNS |
|-----------------|---------------------|
| `user-svc.ee-app` → 10.96.45.78（单一 ClusterIP） | `postgres-headless` → [10.244.1.5, 10.244.1.6]（所有 Pod IP） |
| kube-proxy 负载均衡 | 客户端直接连某个 Pod IP |

**为什么 StatefulSet 要 headless**：postgres-0 / postgres-1 需要**独立访问**（如同步、复制），不能用负载均衡糊弄。

### 5.5 NodePort 示例

```yaml
apiVersion: v1
kind: Service
metadata:
  name: apisix-gateway
spec:
  type: NodePort
  selector: { app: apisix }
  ports:
    - name: http
      port: 9080
      targetPort: 9080
      nodePort: 30080                # 节点端口（30000-32767）
```

浏览器 `localhost:30080` → 节点 30080 → Service 9080 → apisix Pod 9080。

**在 kind 里**：extraPortMappings 把容器节点的 30080 映射到宿主机的 9080（用户视角是 9080，实际节点上 30080）。

---

## 六、ConfigMap（明文配置）

### 6.1 是什么

**ConfigMap = 存明文配置**（yaml 文件 / env 变量 / 命令行参数），让 deployment 不用关心配置内容。

### 6.2 两种用法

#### 用法 1：env 注入

```yaml
# ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: user-svc-config
data:
  LOG_LEVEL: "info"
  GIN_MODE: "release"

---
# Deployment 引用
spec:
  containers:
    - name: user-svc
      envFrom:
        - configMapRef:
            name: user-svc-config
```

#### 用法 2：volume 挂载（完整文件）

```yaml
# ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: user-svc-config
data:
  user-api.yaml: |
    Name: user-api
    Port: 8888
    Log:
      Mode: json
      Level: info

---
# Deployment 引用
spec:
  containers:
    - name: user-svc
      volumeMounts:
        - name: config
          mountPath: /app/etc
  volumes:
    - name: config
      configMap:
        name: user-svc-config
```

### 6.3 我们项目的关键决策：把 etc/<svc>-api.yaml 搬进 ConfigMap

go-zero 的配置文件原本是宿主目录 bind mount：

```yaml
# docker-compose
volumes:
  - ../emotion-echo-user-svc/etc/user-api.yaml:/app/etc/user-api.yaml:ro
```

K8s 里我们改用 ConfigMap：

```yaml
# Helm chart
data:
  user-api.yaml: |
    {{- .Files.Get "files/user-api.yaml" | nindent 4 }}
```

**好处**：
- **ConfigMap 是单一可信源**（不再散落在多个 bind mount）
- **改配置不用重新 build image**（kubectl edit cm 即可）
- **可以走 GitOps**（ConfigMap 在 git 里就有版本控制）

### 6.4 `${VAR:-default}` 占位符问题（Stage 27 重点解决）

go-zero 配置文件里有这种占位符：
```yaml
Log:
  Mode: ${LOG_FORMAT:-json}
```

**问题**：go-zero 1.x **不解析** `${}` 占位符，需要应用层自己 expand。
**K8s 解法**：用 Helm `tpl` 函数预渲染，或直接**移除占位符**（我们 Stage 27 选后者）。

```yaml
# 原来
Mode: ${LOG_FORMAT:-json}

# K8s 里
Mode: json    # 直接写死；或者 {{ .Values.logFormat | default "json" }}
```

---

## 七、Secret（敏感配置）

### 7.1 跟 ConfigMap 的差异

| 维度 | ConfigMap | Secret |
|------|-----------|--------|
| **编码** | 明文 | base64（**注意：不是加密**） |
| **类型** | 通用 | `Opaque` / `kubernetes.io/tls` / `dockerconfigjson` |
| **RBAC** | 默认同 ns 全员可读 | 可单独配（学习阶段常同 ConfigMap） |
| **用途** | 普通配置 | 密码 / API Key / TLS 证书 |

### 7.2 完整示例（mTLS 证书）

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: ai-svc-tls
  namespace: ee-app
type: kubernetes.io/tls
stringData:
  tls.crt: |
    -----BEGIN CERTIFICATE-----
    MIIDxTCCAq2gAwIBAgIRAL...
    -----END CERTIFICATE-----
  tls.key: |
    -----BEGIN RSA PRIVATE KEY-----
    MIIEowIBAAKCAQEAuZxL...
    -----END RSA PRIVATE KEY-----
```

Pod 挂载：
```yaml
volumes:
  - name: tls
    secret:
      secretName: ai-svc-tls
containers:
  - volumeMounts:
      - name: tls
        mountPath: /app/etc/tls
```

### 7.3 我们的 Secret 学习阶段策略

```yaml
# values-dev.yaml（commit 进 git）
global:
  secrets:
    postgresPassword: "postgres"           # dev 默认
    internalApiKey: "dev-key-change-me-please-32chars-min"   # 占位符
    apisixAdminKey: "dev-apisix-admin-key"
```

**生产切换**：用 External Secrets Operator + 阿里云 KMS，或 `values-prod-secret.yaml`（gitignored）。

### 7.4 警告：base64 ≠ 加密

K8s Secret 默认只是 base64，**任何人能 kubectl get secret -o yaml 看到明文**。生产必须：
1. 开启 etcd 加密（`--encryption-provider-config`）
2. RBAC 限制（不能给所有 SA 配 get secrets 权限）
3. 用 External Secrets Operator 集成云 KMS

---

## 八、PVC（PersistentVolumeClaim，持久存储）

### 8.1 是什么

**PVC = "我要 N GB 持久磁盘"**。K8s 自动从 StorageClass 配 PV 给你。

### 8.2 三层关系

```
StorageClass（磁盘类型，如 SSD）── 由云厂商/CSI driver 提供
     │
     ▼ 按需创建
PersistentVolume（PV，真实磁盘）── 集群管理员预创建 或 dynamic provision
     │
     ▼ 被绑定
PersistentVolumeClaim（PVC）── 用户声明 "我要 10Gi ReadWriteOnce"
     │
     ▼ 被 Pod 挂载
容器内 /var/lib/postgresql/data
```

### 8.3 PVC 示例

```yaml
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: postgres-data
  namespace: ee-data
spec:
  accessModes:
    - ReadWriteOnce           # 单节点读写
  resources:
    requests:
      storage: 10Gi
  storageClassName: standard   # 用 default storage class
```

### 8.4 StatefulSet 里的 PVC 模板

```yaml
spec:
  volumeClaimTemplates:        # 关键：模板
    - metadata:
        name: data
      spec:
        accessModes: ["ReadWriteOnce"]
        resources:
          requests:
            storage: 10Gi
```

K8s 自动给每个 Pod 创建一个 PVC：`data-postgres-0`、`data-postgres-1`。

### 8.5 accessModes 选哪个

| Mode | 含义 | 适用 |
|------|------|------|
| **ReadWriteOnce (RWO)** | 单节点读写 | 数据库（postgres/redis） |
| **ReadOnlyMany (ROX)** | 多节点只读 | 静态资源 |
| **ReadWriteMany (RWX)** | 多节点读写 | 共享文件系统（NFS / cephfs） |
| **ReadWriteOncePod (RWOP)** | 单 Pod 读写 | K8s 1.22+，更严格 |

### 8.6 我们项目的 PVC 设计

| PVC | size | 用途 | accessMode |
|-----|------|------|-----------|
| postgres-data | 10Gi | postgres 数据 | RWO |
| redis-data | 1Gi | redis dump | RWO |
| kafka-data | 10Gi | kafka 日志 | RWO |
| etcd-data | 1Gi | etcd 数据 | RWO |
| skywalking-data | 5Gi | OAP h2 数据 | RWO |
| sensevoice-cache | 1Gi | 模型缓存 | RWO |
| xtts-cache | 5Gi | 模型缓存 | RWO |

**学习阶段**：kind 默认 storage class 是 `standard`，本地路径 `/var/lib/...`。

---

## 九、Namespace（虚拟集群）

### 9.1 是什么

**Namespace = 一个 K8s 集群内的"虚拟集群"**，把资源分组。

### 9.2 我们项目的 4 个 ns

```yaml
# charts/emotion-echo/templates/namespace.yaml
apiVersion: v1
kind: Namespace
metadata:
  name: ee-system
  labels: { layer: ingress }
---
apiVersion: v1
kind: Namespace
metadata:
  name: ee-data
  labels: { layer: data }
---
apiVersion: v1
kind: Namespace
metadata:
  name: ee-app
  labels: { layer: app }
---
apiVersion: v1
kind: Namespace
metadata:
  name: ee-observability
  labels: { layer: observability }
```

| ns | 装的什么 | label |
|----|---------|-------|
| ee-system | apisix-ingress + apisix + etcd | `layer=ingress` |
| ee-data | postgres / redis / kafka / skywalking | `layer=data` |
| ee-app | 4 Go svc + ai-svc + llm + 3 AI + web | `layer=app` |
| ee-observability | （预留 prometheus/grafana） | `layer=observability` |

### 9.3 ns 解决了什么

- **RBAC 隔离**：dev SA 只能 get ee-app 的资源，不能动 ee-data
- **资源配额**：给每个 ns 不同的 CPU/内存 quota
- **逻辑清晰**：一眼看出组件属于哪一层

---

## 十、本节自检

1. **Pod 和 Deployment 是什么关系？**
2. **为什么 postgres 用 StatefulSet 而不用 Deployment？**
3. **headless Service 和普通 Service 的核心区别？**
4. **ConfigMap 和 Secret 的真正区别（除了 base64）？**
5. **PVC 和 PV 是什么关系？StatefulSet 怎么用 PVC 模板？**

<details>
<summary>📋 参考答案</summary>

1. Pod 是 K8s 调度的最小单元；Deployment 是"管理多个相同 Pod"的控制器。直接写 Pod 不会自动重启，写 Deployment 才会。
2. postgres 数据必须绑定到具体 Pod；StatefulSet 提供稳定 Pod 名（postgres-0）+ 独立 PVC（data-postgres-0），Pod 重启后名字和 PVC 都不变。
3. 普通 Service 分配 ClusterIP + kube-proxy 负载均衡；headless Service 不分配 ClusterIP，DNS 直接返回所有 Pod IP，客户端自己挑 Pod。
4. 主要是使用场景不同（明文配置 vs 敏感配置）；K8s 内部实现也不同（Secret 可以配 type=tls 之类的专用类型）；可以单独配 RBAC。
5. PV 是真实磁盘（管理员预创建或动态 provision）；PVC 是用户声明（"我要 10Gi"）；K8s 把 PVC 绑给一个 PV。StatefulSet 用 volumeClaimTemplates 自动给每个 Pod 创建一个独立 PVC。

</details>

---

## 十一、推荐阅读

| 资源 | 链接 |
|------|------|
| K8s Concepts | https://kubernetes.io/docs/concepts/ |
| Pod Lifecycle | https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/ |
| StatefulSet Basics | https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/ |
| Service 类型 | https://kubernetes.io/docs/concepts/services-networking/service/ |
| PVC 详解 | https://kubernetes.io/docs/concepts/storage/persistent-volumes/ |

---

> **下一步**：[07 网络模型与 DNS](./07-networking-and-dns.md) —— 那个 `<svc>.<ns>.svc.cluster.local` 字符串到底怎么解析的？Kafka advertised listener 为什么必须改？