# 05 · APISIX vs Ingress-NGINX 网关选型实战

> 系列：[04 Umbrella chart](./04-umbrella-chart.md) · **05 网关选型** · [06 K8s 资源解构](./06-k8s-resources.md) ...

**适合谁**：被 K8s 网关概念搞晕（Ingress / Ingress Controller / API Gateway / Service Mesh 边界在哪）的读者，以及想知道"为什么我们 Emotion-Echo 选 APISIX 而不是社区默认 Ingress-NGINX"的人。
**读完你能**：自己画出一张"网关技术地图"，知道 APISIX/Envoy/Nginx 各擅长什么，能向同事解释"APISIX 解决了 nginx 301 about:blank bug"是什么含义。

---

## 一句话总结

**Ingress-NGINX = "够用的默认"。APISIX = "可扩展的现代网关"。**

我们 Emotion-Echo 因为 16 条精细路由 + 多插件需求 + 已经被 nginx 301 bug 坑过（Stage 26-Q），所以选了 APISIX 3.10+。

---

## 一、K8s 网关的 4 层概念（先别混）

| 层 | 是什么 | 谁实现 | 你写什么 |
|----|--------|--------|----------|
| **Ingress** | K8s 的资源对象（spec） | K8s API server | `Ingress` yaml |
| **Ingress Controller** | 把 Ingress 翻译成网关配置 | NGINX / APISIX / Traefik | controller pod 跑什么 |
| **API Gateway** | 完整网关产品（含路由 + 限流 + 鉴权 + 监控） | APISIX / Kong / Envoy | 通常跟 Ingress Controller 配合 |
| **Service Mesh** | 服务间 mTLS + 可观测 | Istio / Linkerd | sidecar 模式 |

**关系**：Ingress Controller 通常**实现**一个 API Gateway 的功能。APISIX 既能当 Ingress Controller 用，也能脱离 K8s 独立运行。

---

## 二、四种主流 K8s Ingress Controller 对比

| 维度 | Ingress-NGINX | APISIX Ingress | Traefik | Contour (Envoy) |
|------|---------------|---------------|---------|----------------|
| **底层** | NGINX + Lua | APISIX (etcd + Lua) | 自研 Go | Envoy |
| **配置存储** | ConfigMap | etcd / K8s CRD | K8s CRD | K8s CRD |
| **路由配置** | Ingress yaml | Ingress + ApisixRoute | IngressRoute | IngressRoute |
| **插件生态** | 弱（需手写 Lua） | **强**（jwt-auth / limit-count / prometheus 等 80+ 插件） | 中 | 中（靠 Envoy filter） |
| **性能** | 高（C10K 优化） | **很高**（基于 etcd + LuaJIT） | 高 | 很高（Envoy） |
| **社区** | K8s 官方维护 | Apache 顶级项目 | Containous | VMware |
| **学习曲线** | 低（最常见） | 中（概念多） | 低 | 高（Envoy 复杂） |
| **CRD 支持** | 弱 | **强**（v2 CRD） | 中 | 中 |
| **多协议** | HTTP/HTTPS | HTTP/gRPC/WebSocket/MQTT | HTTP/gRPC | 全 |
| **中文文档** | 多 | 多（Apache 国内贡献多） | 中 | 少 |

### 2.1 Ingress-NGINX 是"事实标准"，但有两个坑

1. **nginx 301 about:blank bug**（Stage 26-Q 真实踩过）：apache/apisix:3.9.0-debian 镜像触发 nginx openresty 内部 `return 301 about:blank`，导致前端访问 API 直接 redirect 到空白页。
2. **插件开发门槛高**：要写 Lua + 重新编译 nginx（一般人写不动）。

### 2.2 APISIX 的三大优势

1. **插件丰富**：80+ 官方插件，jwt-auth / limit-count / prometheus / cors / key-auth / proxy-rewrite 等都是**配置式**启用
2. **动态路由**：改 ApisixRoute CRD 后秒级生效（不需要 reload nginx）
3. **国产 + 中文文档好**：Apache 国内大厂（API7 / 支流科技）维护，问题排查容易

### 2.3 我们项目的具体需求匹配

| 我们的需求 | Ingress-NGINX | APISIX 3.10+ |
|-----------|---------------|---------------|
| 16 条路由精确匹配 | ✅ | ✅ |
| jwt-auth 鉴权 | 需手写 Lua | ✅ 开箱即用 |
| limit-count 限流 | 需手写 | ✅ 一行配置 |
| prometheus 指标 | 需手写 exporter | ✅ 自带 `/apisix/prometheus/metrics` |
| 多协议（gRPC + HTTP） | ⚠️ 配起来繁琐 | ✅ `protocol: grpc` 一行 |
| **修 nginx 301 about:blank** | ❌ 还是会被坑 | ✅ 3.10+ 修了 |

**结论**：选 APISIX。

---

## 三、APISIX 的架构原理（一张图看懂）

```
                          K8s 集群
┌───────────────────────────────────────────────────────────────┐
│                                                               │
│   ┌─────────────────┐       ┌──────────────────┐             │
│   │ apisix-ingress-  │       │ apisix (gateway) │             │
│   │ controller       │       │ (Pod, 多副本)     │             │
│   │                  │       │                  │             │
│   │  监听 K8s CRD   │──────→│  反向代理 Pod     │             │
│   │  ApisixRoute    │ etcd  │  + 跑插件         │             │
│   │  ApisixUpstream │       │                  │             │
│   │                  │       │  端口 9080       │             │
│   └─────────────────┘       └──────────────────┘             │
│            │                          │                       │
│            │                          │                       │
│            └────────────┬─────────────┘                       │
│                         ▼                                     │
│                ┌──────────────────┐                           │
│                │ etcd (StatefulSet)│                          │
│                │ 存路由/上游/插件  │                           │
│                └──────────────────┘                           │
│                                                               │
│   ┌───────────────────────────────────────────────┐          │
│   │ 业务 Pod (user-svc / chat-svc / ...)            │          │
│   │  ◄── apisix gateway 反向代理流量 ──             │          │
│   └───────────────────────────────────────────────┘          │
│                                                               │
└───────────────────────────────────────────────────────────────┘
```

**关键流程**：

1. 你 apply `ApisixRoute` CRD 到 K8s
2. `apisix-ingress-controller` Pod watch 到变化
3. controller 把 ApisixRoute 转成 etcd 的 JSON 配置
4. `apisix` gateway Pod 监听 etcd 变化，**动态**更新内存路由表（不需要 reload）
5. 用户请求到 apisix:9080 → 匹配路由 → 反向代理到 `user-svc.ee-app:8888`

---

## 四、APISIX CRD 详解

### 4.1 ApisixUpstream（后端服务抽象）

```yaml
apiVersion: apisix.apache.org/v2
kind: ApisixUpstream
metadata:
  name: user-svc
  namespace: ee-app
spec:
  upstream:
    type: RoundRobin
    nodes:
      - host: user-svc.ee-app.svc.cluster.local
        port: 8888
        weight: 100
```

**类比 NGINX upstream block**：
```nginx
upstream user-svc {
  server user-svc.ee-app.svc.cluster.local:8888 weight=100;
}
```

### 4.2 ApisixRoute（路由规则）

```yaml
apiVersion: apisix.apache.org/v2
kind: ApisixRoute
metadata:
  name: r-user-me
  namespace: ee-app
spec:
  http:
    - name: user-me
      match:
        paths:
          - /api/v1/users/me
        methods:
          - GET
      backends:
        - serviceName: user-svc
          servicePort: 8888
      plugins:
        - name: jwt-auth
          enable: true
        - name: prometheus
          enable: true
```

**类比 NGINX location block**：
```nginx
location /api/v1/users/me {
  proxy_pass http://user-svc;
}
```

### 4.3 完整 ApisixRoute 示例（带插件）

```yaml
apiVersion: apisix.apache.org/v2
kind: ApisixRoute
metadata:
  name: r-conv-create
spec:
  http:
    - name: conv-create
      match:
        paths:
          - /api/v1/conversations
        methods:
          - POST
      backends:
        - serviceName: chat-svc
          servicePort: 8890
      plugins:
        - name: jwt-auth
          enable: true
          config:                  # 插件配置
            key: user-id
        - name: limit-count
          enable: true
          config:
            count: 100
            time_window: 60
            key: remote_addr
            rejected_code: 429
        - name: prometheus
          enable: true
```

**插件叠加 = APISIX 的核心价值**。一行配置启用 jwt-auth + 限流 + 监控。

---

## 五、Stage 27 APISIX 部署实践

### 5.1 APISIX Ingress Controller 安装

```bash
# k8s/scripts/03-install-ingress.sh
helm repo add apisix https://charts.apisix.apache.org
helm repo update

helm install apisix apisix/apisix-ingress-controller \
  --namespace ee-system \
  --create-namespace \
  --set config.apisix.serviceNamespace=ee-system \
  --set ingressClass=apisix
```

### 5.2 APISIX Gateway + etcd 安装

我们的 `apisix-ingress` 子 chart（自写，不依赖官方 chart）：

```yaml
# charts/emotion-echo/charts/apisix-ingress/templates/deployment.yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: apisix
  namespace: ee-system
spec:
  replicas: 1
  selector:
    matchLabels: { app: apisix }
  template:
    spec:
      containers:
        - name: apisix
          image: apache/apisix:3.10.0-debian
          ports:
            - { name: http, containerPort: 9080 }
            - { name: admin, containerPort: 9091 }
          env:
            - name: APISIX_ETCD_ENDPOINTS
              value: "http://etcd-0.etcd-headless.ee-system.svc.cluster.local:2379"
```

### 5.3 NodePort 暴露

```yaml
# charts/emotion-echo/charts/apisix-ingress/templates/service.yaml
apiVersion: v1
kind: Service
metadata:
  name: apisix-gateway
  namespace: ee-system
spec:
  type: NodePort
  selector: { app: apisix }
  ports:
    - name: http
      port: 9080
      targetPort: 9080
      nodePort: 30080     # 配合 kind extraPortMappings 9080
```

---

## 六、APISIX vs Envoy vs Kong（扩展阅读）

| 维度 | APISIX | Kong | Envoy + Istio |
|------|--------|------|---------------|
| **语言** | Lua + etcd | Lua + Postgres | C++ |
| **配置存储** | etcd | Postgres | xDS API |
| **最擅长** | 边缘网关 + 插件丰富 | API 商业版（付费插件） | Service Mesh 边车 |
| **性能** | 极高（LuaJIT） | 高 | 极高 |
| **上手难度** | 中 | 中 | 高 |
| **学习建议** | API 网关 / K8s ingress | API 网关 + 商业插件 | 想做 service mesh 时 |

**我们项目选 APISIX 而非 Envoy/Istio 的原因**：
- 我们要的是**边缘网关**（流量入口），不是**服务间 mesh**（Pod-to-Pod）
- Istio 的 sidecar 模式会**给每个业务 Pod 注入 envoy**，复杂且占资源
- APISIX 专注"流量入口"场景，更纯粹

---

## 七、APISIX 学习路径

```
入门        装一个 APISIX + 一个 upstream + 一个 route
   ↓
CRD 模式    ApisixRoute + ApisixUpstream 配 K8s svc
   ↓
插件        jwt-auth / limit-count / prometheus 各开一个
   ↓
调优        etcd 集群模式 / apisix 多副本 / 配置缓存
   ↓
源码        apisix/init.lua / resty/router 等核心模块
```

---

## 八、本节自检

1. **Ingress 和 Ingress Controller 的关系？**
2. **APISIX 比 Ingress-NGINX 强在哪？（至少 3 点）**
3. **apisix-ingress-controller 和 apisix gateway 是两个东西吗？**
4. **`upstream` 在 APISIX 里对应 K8s 的什么？**
5. **APISIX 3.10+ 修的 nginx 301 about:blank bug 是什么症状？**

<details>
<summary>📋 参考答案</summary>

1. Ingress 是 K8s API 资源对象（声明式 spec）；Ingress Controller 是 watch 这些对象并实现路由的进程（NGINX/APISIX/Traefik 等）。
2. 插件生态丰富（80+ 开箱即用）；动态路由（不需要 reload nginx）；多协议支持好；中文社区 + 国产维护。
3. 是。apisix-ingress-controller 监听 K8s CRD 翻译成 etcd 配置；apisix gateway 实际处理流量。两个 pod，独立部署。
4. 对应 K8s 的 Service（虚拟 IP + DNS）；在 ApisixUpstream 里通过 serviceName + servicePort 引用。
5. 用户访问 api path 时被 nginx 内部 301 重定向到 about:blank 空白页，路由完全失效。Stage 26-Q 实测 apache/apisix:3.9.0-debian 镜像有这问题。

</details>

---

## 九、推荐阅读

| 资源 | 链接 |
|------|------|
| APISIX 官方文档 | https://apisix.apache.org/docs/ |
| APISIX Ingress Controller | https://github.com/apache/apisix-ingress-controller |
| APISIX vs Envoy vs Kong | https://www.cncf.io/blog/2022/08/15/ |
| Ingress-NGINX 文档 | https://kubernetes.github.io/ingress-nginx/ |
| Envoy 入门 | https://www.envoyproxy.io/docs/envoy/latest/ |

---

> **下一步**：[06 K8s 资源对象逐个解构](./06-k8s-resources.md) —— Deployment/StatefulSet/Service/PVC/ConfigMap/Secret 一个个拆开看我们项目怎么用。