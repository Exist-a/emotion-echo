# 02 · 本地集群原理解析：kind / minikube / k3d 怎么选？

> 系列：[01 K8s 认知地图](./01-why-kubernetes.md) · **02 本地集群** · [03 Helm 入门](./03-helm-fundamentals.md) ...

**适合谁**：要在自己笔记本上跑 K8s 但被一堆工具（kind/minikube/k3d/Docker Desktop）搞晕的读者。
**读完你能**：说出 kind 和 minikube 的根本区别，知道我们项目为什么选 kind，能解释 kind 把"集群跑在容器里"这件事的实现原理。

---

## 一句话总结

**kind = "K8s-in-Docker"**。它把 K8s 的 master/worker 节点都跑成 Docker 容器。**minikube = "K8s-in-VM"**。它启动一个虚拟机，在 VM 里跑 K8s。

选择哪一边，取决于你的**操作系统 + 性能需求 + 多节点需求**。

---

## 一、四个常见本地集群工具速览

| 工具 | 形态 | 启动时间 | 多节点 | 资源占用 | 主流 OS |
|------|------|----------|--------|----------|---------|
| **kind** | K8s 节点跑成 Docker 容器 | 30 秒 | ✅ 原生支持 | 低（复用宿主 Docker） | macOS / Linux / **Windows** |
| **minikube** | K8s 节点跑在 VM / 容器 / bare-metal | 1-3 分钟 | ⚠️ 需额外配置 | 中（VM 占 2GB 内存） | 全平台 |
| **k3d** | K8s 节点跑成 Docker 容器（用 k3s） | 15 秒 | ✅ 原生支持 | 极低（k3s 比 k8s 轻 40%） | macOS / Linux / **Windows** |
| **Docker Desktop K8s** | K8s 集成进 Docker Desktop | 5-10 分钟 | ❌ 单节点 | 高 | macOS / Windows |

我们 Emotion-Echo Stage 27 选 **kind**，下面解释为什么。

---

## 二、kind 是什么？怎么工作的？

### 2.1 一句话原理

**kind = 把 K8s 节点（control-plane / worker）**用 Docker 容器**模拟出来**。这些容器里跑的是真实的 kubelet / kube-proxy / kube-apiserver，所以**和你在阿里云 ACK / AWS EKS 上看到的 K8s API 完全一致**。

```
┌─────────────────────── 宿主机 (你的笔记本) ───────────────────────┐
│                                                                  │
│  ┌───────────────────── kind 集群 ─────────────────────────────┐ │
│  │                                                              │ │
│  │  ┌──────────────────┐  ┌──────────────────┐  ┌────────────┐ │ │
│  │  │ control-plane    │  │ worker           │  │ worker     │ │ │
│  │  │ (Docker 容器)    │  │ (Docker 容器)    │  │ (Docker)   │ │ │
│  │  │                  │  │                  │  │            │ │ │
│  │  │  - kube-apiserver│  │ - kubelet        │  │ - kubelet  │ │ │
│  │  │  - etcd          │  │ - kube-proxy     │  │ - kubelet  │ │ │
│  │  │  - scheduler     │  │ - 容器运行时     │  │            │ │ │
│  │  │  - controller    │  │                  │  │            │ │ │
│  │  └──────────────────┘  └──────────────────┘  └────────────┘ │ │
│  │                                                              │ │
│  └──────────────────────────────────────────────────────────────┘ │
│                                                                  │
│  你的 shell ──kubectl──→ kubeconfig 指向 control-plane:6443     │
│  你的浏览器 ──localhost:9080──→ kind 节点 9080 → APISIX Pod    │
│                                                                  │
└──────────────────────────────────────────────────────────────────┘
```

### 2.2 关键点：节点就是容器

- **control-plane** 是一个特殊容器（名字固定为 `<cluster-name>-control-plane`）
- **worker** 是另一个容器（名字固定为 `<cluster-name>-worker` / `worker2`）
- 这些容器共享宿主机的 Docker daemon，所以**容器之间能通信**（用 Docker 网络 `kind`）
- kubelet 在容器**里面**跑，所以它能拉起新的 Pod（Pod 也在 Docker 容器里）

> **很多人误以为**：kind 把 K8s 装进一个容器里。其实是：每一个 K8s 节点 = 一个容器。

### 2.3 kind 的两大杀手锏

#### 杀手锏 1：`kind load docker-image`

普通 K8s 集群：你 push 镜像到 ACR/Docker Hub → kubelet 从仓库 pull。
kind：你直接 `kind load docker-image my-img:v1`，**镜像直接被导入节点容器里**，不用任何镜像仓库。

这在**离线 / 弱网**环境是救命稻草。我们 Emotion-Echo 项目 13 个本地镜像，要是没有 `kind load`，每个都要 push + pull 一遍，慢 + 浪费带宽。

#### 杀手锏 2：节点端口映射

kind 用 `extraPortMappings` 把**容器节点**的端口映射到**宿主机**：

```yaml
# kind-config.yaml
nodes:
  - role: control-plane
    extraPortMappings:
      - containerPort: 9080    # 容器节点上的 9080
        hostPort: 9080          # 宿主机的 9080
      - containerPort: 18080
        hostPort: 18080
```

所以你浏览器输 `localhost:9080` 就能访问到 K8s 集群**里面**的 APISIX Pod。

### 2.4 kind 的劣势（要诚实知道）

| 劣势 | 影响 |
|------|------|
| **镜像加载慢**（Windows） | 13 个镜像每个几百 MB，`kind load` 走 stdin 流，Windows 上偶尔超时 |
| **无真正的多节点隔离** | 节点还是容器，不能测真实多机网络 |
| **CPU/内存超额会 OOM** | 节点容器只有宿主的资源，多个 Pod 互相挤 |
| **不支持的部分** | hostNetwork / GPU 透传 / 真实硬件特性 |

---

## 三、minikube vs kind 实战对比

| 维度 | kind | minikube |
|------|------|----------|
| **底层** | Docker 容器 | VM (VirtualBox/HyperKit/Hyper-V) 或 Docker |
| **启动** | `kind create cluster`（30 秒） | `minikube start`（1-3 分钟） |
| **多节点** | `--config` 一行 worker | `minikube node add` 较繁琐 |
| **镜像加载** | `kind load docker-image` | `minikube image load`（类似） |
| **端口转发** | `extraPortMappings`（静态） | `minikube service --url`（动态） |
| **Dashboard** | 需另装 | `minikube dashboard`（自带） |
| **Windows 兼容性** | ✅ Docker for Windows 必备 | ✅ Hyper-V/WSL2 都行 |
| **官方推荐** | K8s 项目测试用 | 学习用 |

**我们选 kind 的理由**：
1. **启动快**（30 秒）—— 文档要截图、要回归，频繁重启
2. **多节点简单**（一个 yaml 文件就 OK）—— Stage 27 验证 multi-pod 调度
3. **Windows 友好**（无需 Hyper-V/WSL2 单独配置）—— 团队成员用 Win 多
4. **和 K8s 1.30+ 同步更新**（kind 是 K8s SIG 出品）—— 测试新特性

---

## 四、Stage 27 kind 配置详解

### 4.1 完整 kind-config.yaml

```yaml
# k8s/kind-config.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4

# 节点定义：1 control-plane + 2 worker
nodes:
  - role: control-plane
    kubeadmConfigPatches:
      - |
        kind: InitConfiguration
        nodeRegistration:
          kubeletExtraArgs:
            node-labels: "ingress-ready=true"   # 给 ingress controller 用
    extraPortMappings:
      # 端口映射：宿主机 → K8s 节点容器
      - containerPort: 9080      # APISIX gateway
        hostPort: 9080
        protocol: TCP
      - containerPort: 18080     # SkyWalking UI
        hostPort: 18080
        protocol: TCP
      - containerPort: 3000      # Web SPA（备用）
        hostPort: 3000
        protocol: TCP

  - role: worker
  - role: worker
```

### 4.2 关键字段解释

| 字段 | 含义 | 我们为什么要 |
|------|------|--------------|
| `nodes[].role` | 节点角色 | 一个 control-plane + 两个 worker，模拟生产 |
| `extraPortMappings` | 端口映射 | 让浏览器能访问集群内 Pod |
| `node-labels: ingress-ready=true` | 节点标签 | 让 ingress controller **只**跑在 control-plane |
| `kubeadmConfigPatches` | 启动参数 | 给 kubelet 加标签，避免每次手动 `kubectl label` |

### 4.3 创建 + 加载 + 列出

```bash
# 1. 创建集群（30 秒）
kind create cluster --config k8s/kind-config.yaml --name ee-cluster

# 2. 加载本地镜像（13 个镜像一次性灌入）
kind load docker-image emotion-echo/user-svc:v0.1.0 \
                       emotion-echo/chat-svc:v0.1.0 \
                       emotion-echo/ai-svc:v0.1.0 \
                       --name ee-cluster

# 3. 查看节点
kubectl get nodes
# NAME                       STATUS   ROLES           AGE   VERSION
# ee-cluster-control-plane   Ready    control-plane   1m    v1.30.0
# ee-cluster-worker          Ready    <none>          1m    v1.30.0
# ee-cluster-worker2         Ready    <none>          1m    v1.30.0

# 4. 删除集群
kind delete cluster --name ee-cluster
```

---

## 五、kind 集群内"网络是怎么连的"（最容易踩坑的地方）

### 5.1 Pod 之间怎么通信？

```
Pod A (10.244.1.5)  ─┐
                     ├── 在同一个 kind 集群里,通过 Docker 网络 `kind` 互通
Pod B (10.244.1.6)  ─┘

这些 IP 是 kind 用 CNI (默认 kindnet) 在 Docker 网络里分配的
```

### 5.2 Pod 怎么访问外部（如 Docker Hub）？

```
Pod → 节点容器 → 宿主机 Docker daemon → 走宿主机的网络出去
```

所以 Pod 能拉镜像（如果用公网镜像），但拉镜像的"客户端"实际上是宿主机。

### 5.3 宿主机怎么访问 Pod？

```
浏览器 localhost:9080
       │
       ↓ (extraPortMappings)
K8s 节点容器 :9080 (kind 把它暴露到宿主机网络)
       │
       ↓ (kube-proxy iptables/ipvs 转发)
APISIX Pod :9080
```

### 5.4 节点之间怎么通信？

control-plane 和 worker 在 **同一个 Docker 网络 `kind`** 里，所以互相能 ping。

---

## 六、Stage 27 踩过的坑（真实记录）

### 坑 1：Windows 文件锁

**症状**：`mv kind.exe /usr/local/bin/` 时报 "Device or resource busy"
**原因**：kind.exe 在被某个进程使用，Windows 不允许移动。
**解决**：先 TaskStop 所有 kind 相关进程；如果还不行，重启 shell（这一步用户在我们项目里就遇到了一次）。

### 坑 2：`kind load docker-image` 超时

**症状**：大镜像（XTTS ~3GB）`kind load` 卡死
**原因**：kind 走 stdin 流传镜像，Windows shell 对大流处理弱。
**解决**：分批 load，或者 `docker save` + `kind load image-archive`。

### 坑 3：端口已被占用

**症状**：`kind create cluster` 报 "port 9080 already in use"
**原因**：宿主机 9080 已经被 docker-compose APISIX 占用。
**解决**：先 `docker-compose down`，或者改 extraPortMappings 到 8088。

### 坑 4：内存不够 OOM

**症状**：`kind create` 之后 control-plane 容器一直在 Restarting。
**原因**：宿主内存 < 4GB，kind 启动已经吃满。
**解决**：关掉其他应用，或改用 `k3d`（更轻）。

---

## 七、本节自检

1. **kind 把 K8s 节点跑在什么里？**
2. **`extraPortMappings` 解决什么问题？**
3. **为什么我们项目选 kind 不选 minikube？（至少说 2 个理由）**
4. **kind 在 Windows 上有什么已知坑？**
5. **如果镜像太大加载慢，kind 的解决思路是什么？**

<details>
<summary>📋 参考答案</summary>

1. Docker 容器里。每一个 K8s 节点 = 一个 Docker 容器，容器里跑真实的 kubelet / kube-apiserver。
2. 解决"宿主机浏览器如何访问到 K8s 集群内部的 Pod"问题。把节点容器的端口映射到宿主机，浏览器输 localhost:9080 就能访问 APISIX Pod。
3. 启动快（30 秒 vs 1-3 分钟）；多节点配置简单；Windows 友好；和 K8s 主版本同步更新。
4. 大镜像 `kind load` 走 stdin 容易超时；文件锁（mv 失败）；端口被 docker-compose 占用冲突。
5. 用 `docker save` 导出 tar 包，再 `kind load image-archive`。

</details>

---

## 八、推荐阅读

| 资源 | 链接 | 价值 |
|------|------|------|
| kind 官方文档 | https://kind.sigs.k8s.io/ | 权威但略简 |
| kind 源码 | https://github.com/kubernetes-sigs/kind | 看 `pkg/cluster/` 理解实现 |
| minikube vs kind | https://www.infracloud.io/blogs/kind-vs-minikube/ | 深度对比 |
| k3d 文档 | https://k3d.io/ | 如果想换更轻的工具 |
| KIND 内部架构 | https://github.com/kubernetes-sigs/kind/blob/main/docs/design.md | 想深挖就看 |

---

> **下一步**：[03 Helm 模板语言与 chart 架构深入](./03-helm-fundamentals.md) —— 我们的 Stage 27 全部交付物都是 Helm chart。