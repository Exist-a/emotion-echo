# 01 · 为什么我们需要 Kubernetes？（认知地图）

> 系列：[00 学习路径](./00-index.md) · **01 认知地图** · [02 本地集群](./02-local-cluster.md) · [03 Helm 入门](./03-helm-fundamentals.md) ...

**适合谁**：第一次接触容器编排 / 只用过 docker-compose / 想搞清楚"K8s 比 compose 多做了哪些事"的读者。
**读完你能**：用自己的话向同事解释"K8s 不是 Docker 的替代品，而是 Docker 之上的'自动化运维层'"。

---

## 一句话总结

**Kubernetes（K8s）= 容器编排平台 = "如果你有 1000 个 Docker 容器，怎么让它们正确启动、排错、扩容、安全运行？"的答案。**

Docker 解决了"一个应用怎么打包"，K8s 解决"几千个打包好的应用怎么在几台机器上协调运行"。

---

## 一、痛点驱动学习（先看问题，再看解法）

我们 Emotion-Echo 项目用 docker-compose 已经能跑了，那为什么还要学 K8s？看 Stage 26-P 文档记录的**真实踩坑**：

| 真实场景 | docker-compose 的局限 | K8s 的解法 |
|----------|---------------------|------------|
| Postgres 容器挂掉，整个系统瘫了 13 小时 | `restart: always` 凑合用，但**没有自愈意识** | Deployment 自带 `restartPolicy: Always` + **健康检查不通过就重建 Pod** |
| 改一个 yaml，`docker compose up` 必须先 stop 再 start | 有 **downtime** | **滚动更新**：旧 Pod 跑着的同时启新 Pod，流量无缝切换 |
| ai-svc 在高峰期处理不过来 | 改 `replicas: 5` + 重启整个 stack | **HPA**：CPU 超过 70% 自动扩容到 10 个副本，CPU 降下来自动缩容 |
| 想把 postgres 放到另一台机器 | 跨主机的容器网络要自己搞（macvlan / overlay） | **CNI 网络**：Pod 跨节点通信，Pod IP 在所有节点都能路由 |
| 容器名 `emotion-echo-user-svc` 只有在同一 compose 网络里能用 | 改 host 名就得改所有 yaml | **Service + DNS**：K8s 自动给 svc 分配 DNS 记录，Pod 重启 IP 变了也不影响调用方 |
| 不同环境用不同 yaml（dev/staging/prod） | 维护 N 份 compose 文件，容易漂移 | **Helm values overlay** 或 **Kustomize overlay**：一份 chart + 多份 values |
| `deploy/tls/*.key` 直接 commit 进 Git | 私密性差，权限控制弱 | **Secret 对象**：base64 编码 + RBAC 权限 + 生产可换 KMS 加密 |

**结论**：docker-compose 是"本地能跑"，K8s 是"生产稳定"。

---

## 二、K8s 的核心抽象（先记 7 个名词）

K8s 的一切都是"声明式 API 对象"。你写一个 yaml 文件告诉 K8s"我想要什么"，K8s 自己想办法实现。这 7 个名词你必须先记牢：

### 1. Pod（最小调度单元）

```
┌─────────────────── Pod ───────────────────┐
│ ┌─────────┐  ┌─────────┐  ┌─────────┐   │
│ │ Container│  │ Container│  │ Container│   │  ← 共享网络 + 存储
│ │  (app)   │  │ (sidecar)│  │ (logs)   │   │
│ └─────────┘  └─────────┘  └─────────┘   │
│ 共享 IP: 10.244.1.5                       │
│ 共享 /tmp、/var/log                       │
└──────────────────────────────────────────┘
```

- **关键概念**：Pod 不是容器，Pod 是"一组共享网络与存储的容器集合"
- **实战**：99% 的场景一个 Pod 一个容器，sidecar 模式才会多容器
- **不要直接创建 Pod** —— 用 Deployment/StatefulSet 间接创建

### 2. Deployment（无状态应用管理者）

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: user-svc
spec:
  replicas: 3              # 期望 3 个副本
  selector: { ... }
  template:                # Pod 模板
    spec:
      containers:
        - name: user-svc
          image: emotion-echo/user-svc:v0.1.0
```

- **K8s 保证**：始终有 3 个 Pod 在跑（挂了自动重建）
- **滚动更新**：`kubectl set image` 后，旧 Pod 一个一个被新 Pod 替换
- **回滚**：`kubectl rollout undo deployment/user-svc` 一键回到上一版本

### 3. StatefulSet（有状态应用管理者）

跟 Deployment 几乎一样，但多两个特性：
- **稳定的网络标识**：`pod-0`、`pod-1`、`pod-2`（Pod 重建后名字不变）
- **稳定的存储**：`pvc-pod-0`、`pvc-pod-1`（每个 Pod 一个独立 PVC）

**什么用 StatefulSet**：postgres、redis、kafka、etcd、zookeeper —— 这些需要"我是第几个"的数据服务。

### 4. Service（虚拟 IP + DNS）

Pod IP 是会变的（重启就变）。Service 给你一个**固定的虚拟 IP + DNS 名字**：

```yaml
apiVersion: v1
kind: Service
metadata:
  name: user-svc
spec:
  selector: { app: user-svc }    # 选哪些 Pod
  ports:
    - port: 8888                # Service IP 的端口
      targetPort: 8888          # Pod 容器端口
```

K8s 自动给这个 Service 分配：
- 一个虚拟 IP（ClusterIP）
- 一个 DNS 名字：`user-svc.ee-app.svc.cluster.local`

**为什么需要**：`user-svc` Pod IP 是 `10.244.1.5`，重启可能变成 `10.244.1.9`。但所有客户端只要连 `user-svc:8888`，K8s 自动把流量转到新 Pod 上。

### 5. ConfigMap（明文配置）

把 yaml 配置、env 变量从代码里剥离：

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: user-svc-config
data:
  user-api.yaml: |
    Name: user-api
    Port: 8888
```

Pod 通过 volume 挂载或 env 引用它。

### 6. Secret（敏感配置）

跟 ConfigMap 一样，但 base64 编码，且可单独配 RBAC 权限（谁能读）：

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: user-svc-secret
type: Opaque
stringData:                # stringData 会自动 base64
  postgresDsn: "host=postgres user=postgres password=postgres dbname=emotion_echo"
```

> **警告**：K8s Secret 默认只是 base64，**不是加密**！生产要用 External Secrets + KMS。

### 7. Namespace（虚拟集群）

把一个 K8s 集群拆成多个"虚拟集群"。我们 Stage 27 用 4 个：

| Namespace | 内容 |
|-----------|------|
| `ee-system` | APISIX Controller、etcd（APISIX 后端） |
| `ee-data` | postgres、redis、kafka、skywalking |
| `ee-app` | 5 Go svc + ai-svc + llm-service + 3 AI + web |
| `ee-observability` | （预留 prometheus/grafana） |

好处：
- **RBAC 隔离**：开发不能删 data 命名空间的 PVC
- **资源配额**：给每个 ns 不同的 CPU/内存上限
- **逻辑清晰**：一眼看出每个组件属于哪一层

---

## 三、K8s 的"控制循环"思想（最重要的一条原理）

K8s 一切都是**声明式 + 终态调和（reconciliation loop）**：

```
你：  kubectl apply -f deployment.yaml   # "我要 3 个 user-svc Pod"
K8s： 当前只有 2 个 → 启动第 3 个
      现在有 4 个（多了一个） → 删掉多余的
      image 版本变了 → 滚动更新（旧 Pod 一个一个被替换）
```

**类比**：你跟 K8s 说"我要 3 个鸡蛋"，K8s 不断检查"现在有几个鸡蛋"，差就补，多就拿。

**为什么这很强大**：
- **自愈**：Pod 挂了 K8s 自动重启
- **弹性**：流量大了 K8s 自动扩
- **可预测**：你写的 yaml 就是最终状态，不需要管"现在 vs 目标"的差异

---

## 四、K8s vs Docker 对比表

| 维度 | Docker | K8s |
|------|--------|-----|
| **抽象单元** | 容器（Container） | Pod（容器组） |
| **配置方式** | 命令式（`docker run`） | 声明式（apply yaml） |
| **网络** | bridge / host / overlay 需手动配 | CNI 自动，Pod IP 集群内可路由 |
| **存储** | volume bind mount | PV/PVC/StorageClass 抽象层 |
| **服务发现** | 容器名（仅 compose 内） | Service + DNS（`*.svc.cluster.local`） |
| **扩缩容** | `docker run` 手动 | HPA 自动基于 CPU/内存/自定义指标 |
| **滚动更新** | 没有 | `kubectl rollout` |
| **配置管理** | env / bind mount | ConfigMap + Secret |
| **多机** | Swarm（基本已死） | 核心场景 |
| **学习曲线** | 1 天上手 | 1-3 个月 |

---

## 五、一个完整的 K8s 请求路径（端到端串起来）

当用户访问 <http://localhost:9080/api/v1/users/me> 时：

```
1. curl 发到 localhost:9080
        ↓
2. host:9080 是 APISIX Pod 的 NodePort（kind 的 extraPortMapping）
        ↓
3. APISIX Pod 看 /api/v1/users/me 命中 r-user-me 这条 ApisixRoute
        ↓
4. ApisixRoute 指向 ApisixUpstream `user-svc`
        ↓
5. ApisixUpstream 后端是 user-svc.ee-app.svc.cluster.local:8888
        ↓
6. K8s DNS 解析 user-svc → ClusterIP 10.96.45.78
        ↓
7. K8s kube-proxy 把 ClusterIP 转到任意一个 user-svc Pod 的 8888 端口
        ↓
8. user-svc Pod 处理请求（DB 查询、返回）
```

**这串链路涉及的对象**：NodePort → Service(APISIX) → ConfigMap(APISIX) → ApisixRoute → ApisixUpstream → Service(user-svc) → Deployment → Pod → Container

---

## 六、本节自检（你能回答吗？）

1. **Pod 和 Container 是什么关系？**
2. **Deployment 跟 StatefulSet 的核心区别是什么？postgres 用哪个？为什么？**
3. **为什么需要 Service？直接用 Pod IP 不行吗？**
4. **K8s 的"声明式 + 控制循环"是什么意思？举个例子。**
5. **Namespace 解决什么问题？**

<details>
<summary>📋 参考答案</summary>

1. Pod 是 K8s 的调度单元，一个 Pod 可以包含 1-N 个共享网络和存储的容器。绝大多数场景 1 Pod = 1 容器。
2. StatefulSet 有稳定的网络标识（pod-0/1/2）和独立的 PVC。postgres 用 StatefulSet 因为数据需要挂特定 Pod，副本号不能乱。
3. Pod IP 会变（重启/调度会改），Service 提供稳定虚拟 IP + DNS。客户端只连 Service，K8s 负责把请求路由到任意一个健康 Pod。
4. 你声明"我想要的状态"（如 replicas=3），K8s 持续比较"当前状态"和"期望状态"，差异就自动修复。Pod 挂了 → 自动重建；replicas 改了 → 自动扩缩容。
5. 多租户隔离 + 资源配额 + RBAC 权限边界。在我们项目里：data 放中间件、app 放业务、system 放基础设施、observability 放监控。

</details>

---

## 七、推荐阅读顺序

| 阶段 | 资源 |
|------|------|
| 入门 | [K8s 官方 Interactive Tutorial](https://kubernetes.io/docs/tutorials/kubernetes-basics/) —— 30 分钟手把手 |
| 视频 | [TechWorld with Nana - Kubernetes Tutorial for Beginners](https://www.youtube.com/watch?v=X48VuDVv0do) —— 4 小时 |
| 深入 | 《Kubernetes in Action》Marko Luksa —— 经典书 |
| 中文 | [Kubernetes 中文文档](https://kubernetes.io/zh-cn/docs/home/) |
| 实战 | [Kind 官方文档](https://kind.sigs.k8s.io/) —— 本地集群 |
| Helm | [Helm 官方文档](https://helm.sh/docs/) —— 我们 Stage 27 主用 |

---

> **下一步**：[02 本地集群：kind vs minikube vs k3d 怎么选](./02-local-cluster.md)