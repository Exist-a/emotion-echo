# 07 · 网络模型与 DNS 解析

> 系列：[06 K8s 资源解构](./06-k8s-resources.md) · **07 网络与 DNS** · [08 探针/安全](./08-probes-and-security.md) ...

**适合谁**：被 `user-svc.ee-app.svc.cluster.local` 这种 FQDN 吓到、被"Pod 内 Pod 之间能 ping 但 host 名不同"搞晕的读者。
**读完你能**：自己画出 K8s 网络栈，能解释"Kafka advertised listener 必须用 StatefulSet pod DNS"的原因，能配置 K8s 内部任意两个 Pod 之间通信。

---

## 一句话总结

**K8s 网络的核心规则是 "3 个 every"**：

1. Every Pod 都有自己的 IP
2. Every Pod 都能跟所有其他 Pod 直连（不需要 NAT）
3. Every agent（节点、Pod、容器）都能跟所有 Pod 直连

我们 Emotion-Echo 项目里那些"必须用 StatefulSet pod DNS" / "Pod 之间用 service FQDN" 的怪规矩，背后都是这 3 条规则在支撑。

---

## 一、K8s 网络的 4 层抽象

```
┌─────────────────────────────────────────────────────────┐
│  L4 外部网络                                              │
│     ↓                                                    │
│  Ingress Controller (APISIX) :9080                       │
│     ↓                                                    │
│  L3 Service 虚拟 IP (ClusterIP 10.96.x.x)                │
│     ↓                                                    │
│  L2 kube-proxy (iptables/ipvs 转发)                      │
│     ↓                                                    │
│  L1 Pod IP (10.244.x.x) + Container Port                 │
│     ↓                                                    │
│  Container 内 (localhost)                                │
└─────────────────────────────────────────────────────────┘
```

每一层都有自己的 IP 和名字，**层之间用 DNS / iptables 翻译**。

---

## 二、3 个"every"原则详解

### 2.1 Every Pod 都有自己的 IP

- Pod 启动时 kubelet 调 CNI 插件（如 kindnet / flannel / calico）申请 IP
- Pod IP 在**整个集群内唯一**
- Pod 重启 IP 可能变（除非是 StatefulSet + headless）

### 2.2 Every Pod 都能跟所有其他 Pod 直连

**不论 Pod 在不在同一节点**，都能直连：

```
Pod A (worker1) ─┐
                 ├── 直接互通（不要 NAT，不要端口映射）
Pod B (worker2) ─┘
```

**怎么做到**：CNI 插件在每个节点配路由，让容器网络的 IP 段在节点之间可达（vxlan / bgp / host-gw）。

### 2.3 Every agent 都能跟所有 Pod 直连

```
节点 (192.168.1.x)  ── 能 ping Pod IP (10.244.x.x)
Pod A               ── 能 ping 节点 IP
```

---

## 三、DNS 解析（最核心）

### 3.1 K8s 内置 DNS：CoreDNS

K8s 在每个节点跑一个 **CoreDNS Pod**，监听 service / endpoint 变化，自动注册 DNS 记录。

### 3.2 Service 的 DNS 记录

| 形式 | 示例 | 解析为 |
|------|------|--------|
| **短名** | `user-svc` | Service `user-svc` 在**当前 ns** |
| **带 ns** | `user-svc.ee-app` | Service `user-svc` 在 `ee-app` ns |
| **完整 FQDN** | `user-svc.ee-app.svc.cluster.local` | 完整路径 |
| **Pod 名**（headless Service） | `postgres-0.postgres-headless.ee-data.svc.cluster.local` | Pod-0 的 IP |

### 3.3 search 域（重要）

`/etc/resolv.conf` 里 Pod 默认有 search 域：

```
search ee-app.svc.cluster.local svc.cluster.local cluster.local
```

所以在 `ee-app` ns 里的 Pod 写 `user-svc` 就能解析为 `user-svc.ee-app.svc.cluster.local`。

**坑**：跨 ns 必须带 ns 名。在 `ee-app` ns 的 Pod 写 `postgres` 解析不到，必须写 `postgres.ee-data` 或 `postgres.ee-data.svc.cluster.local`。

### 3.4 我们项目的 DNS 实际例子

```yaml
# ai-svc deployment env
env:
  - name: POSTGRES_DSN
    value: "host=postgres.ee-data.svc.cluster.local user=postgres ..."
  - name: KAFKA_BROKERS
    value: "kafka-0.kafka-headless.ee-data.svc.cluster.local:9092"
  - name: LLM_GRPC_ADDR
    value: "llm-service.ee-app.svc.cluster.local:50051"
```

| 名字 | 解析为 | 用途 |
|------|--------|------|
| `postgres.ee-data.svc.cluster.local` | Service `postgres` 的 ClusterIP | 普通 svc 调用 |
| `kafka-0.kafka-headless.ee-data.svc.cluster.local` | Pod kafka-0 的 IP | Kafka client 需要直连具体 broker |
| `llm-service.ee-app.svc.cluster.local` | Service `llm-service` 的 ClusterIP | ai-svc → llm-service |

---

## 四、为什么 Kafka 必须用 StatefulSet pod DNS

### 4.1 docker-compose 时代的写法

```yaml
# docker-compose.apps.yml
KAFKA_BROKERS: '["emotion-echo-kafka:9092"]'
```

这能跑通是因为：
- `emotion-echo-kafka` 是 docker-compose 自动给容器分配的 DNS 名
- 同一 docker 网络内有效

### 4.2 K8s 里同样的写法会失败

```yaml
# 错误写法
KAFKA_BROKERS: '["kafka:9092"]'
```

**Kafka 客户端要解析这个地址**：当 Kafka client 连上 broker，broker 返回 advertised listener（也就是 broker 自己的"对外地址"）。Kafka client 再去连这个 advertised 地址。

**问题**：如果 broker advertised 是 `kafka:9092`（不带 ns），ai-svc Pod 里 CoreDNS **在 ee-app ns 搜索**，搜不到 `kafka`（除非 ee-app ns 有名为 kafka 的 svc）。结果：Kafka client 拿到 advertised 地址后无法连接。

### 4.3 Stage 27 的解决：StatefulSet pod DNS

```yaml
# ConfigMap
data:
  KAFKA_ADVERTISED_LISTENERS: "PLAINTEXT://kafka-0.kafka-headless.ee-data.svc.cluster.local:9092"
```

**为什么这个能行**：
- `kafka-0.kafka-headless.ee-data.svc.cluster.local` 是 **完整 FQDN**
- ai-svc Pod 不依赖 search 域，直接绝对解析
- Kafka client 拿到这个 advertised 地址后能连上

### 4.4 教训

**跨 ns + 跨 Pod 直连的通信**，**必须用完整 FQDN**，不能写短名。

---

## 五、Service vs Pod 直连的取舍

### 5.1 用 Service 的场景（默认）

```bash
# chat-svc → user-svc（普通 HTTP 调用）
curl http://user-svc.ee-app.svc.cluster.local:8888/health
```

**走 Service 的好处**：
- kube-proxy 负载均衡
- Pod 重启 IP 变了不影响调用方
- 支持 readinessProbe 自动剔除不健康 Pod

### 5.2 用 Pod IP 直连的场景（少数）

| 场景 | 原因 |
|------|------|
| Kafka / etcd 集群内部通信 | 必须知道对方是第几个副本 |
| 数据库主从同步 | 主从关系明确，不能负载均衡 |
| gRPC 长连接负载不均 | 需要客户端挑特定 Pod（用 Service 的会话亲和） |

### 5.3 我们项目哪些场景用 Pod 直连

```yaml
# chat-svc → postgres
POSTGRES_DSN: "host=postgres.ee-data.svc.cluster.local ..."   # Service（够用）

# ai-svc → kafka
KAFKA_BROKERS: "kafka-0.kafka-headless.ee-data.svc.cluster.local:9092"   # Pod 直连

# ai-svc → llm-service
LLM_GRPC_ADDR: "llm-service.ee-app.svc.cluster.local:50051"   # Service（gRPC）
```

---

## 六、ingress + Service + Pod 的端到端路径

当用户访问 `http://localhost:9080/api/v1/users/me` 时：

```
1. 浏览器 localhost:9080
       ↓ (kind extraPortMappings)
2. 节点容器 :9080 (kind 把宿主端口映射到节点容器)
       ↓
3. APISIX Pod (ee-system ns) 监听 :9080
       ↓
4. APISIX 看 /api/v1/users/me 匹配 r-user-me 这条 ApisixRoute
       ↓
5. ApisixRoute 后端 ApisixUpstream user-svc
       ↓
6. APISIX 去解析 user-svc.ee-app.svc.cluster.local
       ↓ (CoreDNS)
7. 返回 10.96.45.78 (user-svc Service ClusterIP)
       ↓ (kube-proxy iptables 转发)
8. 选一个健康的 user-svc Pod (10.244.x.x:8888)
       ↓
9. user-svc Pod 处理请求
       ↓
10. 返回 JSON 给浏览器
```

---

## 七、我们项目跨 ns 通信全景

```
┌──────────────── ee-system ────────────────┐
│  apisix-ingress-controller                │
│  apisix (gateway :9080)                   │
│  etcd-0 (etcd-headless)                    │
└──────────┬────────────────────────────────┘
           │ (反向代理到 svc)
           ↓
┌──────────────── ee-app ──────────────────┐
│  user-svc (Deployment, ClusterIP)         │
│  chat-svc (Deployment, ClusterIP)         │
│  llm-service (Deployment, ClusterIP)      │  ← gRPC 端口也 ClusterIP
│  ai-svc (Deployment, ClusterIP)            │
│  fer / sensevoice / xtts (Deployment)      │
│  web (Deployment, ClusterIP)              │
└──────┬────────────────────────────────────┘
       │
       ↓ (DSN / Kafka brokers / SkyWalking)
┌──────────────── ee-data ──────────────────┐
│  postgres (StatefulSet + headless)        │
│  redis (Deployment + ClusterIP)           │
│  kafka-0 (StatefulSet + headless)         │
│  skywalking-oap (StatefulSet + headless)  │
└───────────────────────────────────────────┘
```

**典型流量**：
- ai-svc → `postgres.ee-data.svc.cluster.local:5432`（Service）
- ai-svc → `kafka-0.kafka-headless.ee-data.svc.cluster.local:9092`（Pod 直连）
- ai-svc → `llm-service.ee-app.svc.cluster.local:50051`（Service，gRPC）
- ai-svc → `skywalking-oap.ee-data.svc.cluster.local:11800`（Service）
- ai-svc → `xtts.ee-app.svc.cluster.local:8003`（Service）

---

## 八、CNI 插件（K8s 网络的底层实现）

### 8.1 常见 CNI 对比

| CNI | 形态 | 性能 | 配置难度 | 适用 |
|-----|------|------|---------|------|
| **kindnet** | kind 默认 | 中 | 零配置 | kind 本地 |
| **flannel** | VXLAN | 中 | 低 | 简单集群 |
| **calico** | BGP / VXLAN | 高 | 中 | 生产首选 |
| **cilium** | eBPF | 极高 | 高 | 大规模 + 可观测 |

我们 kind 默认用 kindnet，学习阶段够用。

### 8.2 CNI 干了什么

每个 Pod 启动时：
1. CNI 插件给 Pod 分配 IP（在集群 CIDR 内）
2. 在节点上配路由（Pod IP 可达）
3. 配置 veth pair（一头在 Pod 内，一头在节点网络命名空间）

---

## 九、本节自检

1. **K8s 网络的 "3 个 every" 原则是什么？**
2. **`user-svc.ee-app.svc.cluster.local` 每一段是什么意思？**
3. **为什么 Kafka advertised listener 必须用完整 FQDN？**
4. **普通 Service 和 headless Service 在 DNS 上的区别？**
5. **api-svc → llm-service 应该用 Service 还是 Pod 直连？为什么？**

<details>
<summary>📋 参考答案</summary>

1. Every Pod 都有自己 IP；Every Pod 能跟所有 Pod 直连（无 NAT）；Every agent 能跟所有 Pod 直连。
2. user-svc = service 名；ee-app = namespace；svc.cluster.local = 固定后缀（标识这是 K8s service）。
3. 因为 Kafka client 拿到 advertised 后要自己去连。完整 FQDN 不依赖 search 域，能在任意 ns 的 Pod 里解析到正确地址。短名可能因 search 域找不到。
4. 普通 Service DNS 解析为 ClusterIP；headless Service DNS 直接返回所有 Pod IP 列表（无 ClusterIP）。
5. Service（gRPC 走负载均衡；Pod 重启不影响调用方）。ai-svc → llm-service 是普通业务调用，不需要绑特定副本。

</details>

---

## 十、推荐阅读

| 资源 | 链接 |
|------|------|
| K8s Networking | https://kubernetes.io/docs/concepts/cluster-administration/networking/ |
| K8s DNS for Services and Pods | https://kubernetes.io/docs/concepts/services-networking/dns-pod-service/ |
| CoreDNS | https://coredns.io/ |
| CNI 规范 | https://github.com/containernetworking/cni |
| Calico 文档 | https://docs.tigera.io/calico/latest/about |

---

> **下一步**：[08 探针、SecurityContext、资源限额实战意义](./08-probes-and-security.md) —— readinessProbe 跟 startupProbe 差在哪？runAsNonRoot 真的有必要吗？