# 00 · Emotion-Echo K8s 学习路径（Stage 27 学习文档总入口）

> **目标读者**：在 Emotion-Echo Stage 27（本地 kind + Helm umbrella + APISIX 全量闭环）工作的开发者，特别是刚接触 K8s 的学习者。
> **阅读时长**：完整通读约 3-4 小时；按需查阅每篇 10-20 分钟。

---

## 系列导览

### 入门篇（先读这 5 篇）

| # | 标题 | 你会学到 |
|---|------|---------|
| [01](./01-why-kubernetes.md) | **为什么我们需要 K8s？（认知地图）** | K8s 解决 docker-compose 哪些痛点；7 个核心对象一句话总结 |
| [02](./02-local-cluster.md) | **kind / minikube 怎么选** | kind 的 Docker-in-Docker 原理；为什么我们选 kind |
| [03](./03-helm-fundamentals.md) | **Helm 模板语言与 chart 架构** | Chart.yaml / values.yaml / template / helper 的关系 |
| [04](./04-umbrella-chart.md) | **Umbrella chart 设计哲学** | 17 个子 chart 怎么合成一个 release；多环境 overlay |
| [05](./05-apisix-vs-nginx.md) | **APISIX vs Ingress-NGINX 网关选型** | 16 条 ApisixRoute 怎么映射；为什么修 nginx 301 bug |

### 深入篇（按需查阅）

| # | 标题 | 你会学到 |
|---|------|---------|
| [06](./06-k8s-resources.md) | **K8s 资源对象逐个解构** | Deployment vs StatefulSet；headless Service；PVC 模板 |
| [07](./07-networking-and-dns.md) | **网络模型与 DNS** | 3 个 every 原则；FQDN 为什么必须带 ns |
| [08](./08-probes-and-security.md) | **探针、SecurityContext、资源限额** | startup / readiness / liveness 区别；runAsNonRoot 实战 |
| [09](./09-stage-27-pitfalls.md) | **Stage 27 踩坑全记录** | 18 个真实坑 + 修复方法（helm template / Windows / YAML） |

### 实战篇

| # | 标题 | 你会学到 |
|---|------|---------|
| [10](./10-tdd-for-k8s.md) | **TDD 在 K8s 落地中的实操** | render-assert 测试结构；红绿循环节奏 |
| [11](./11-compose-to-k8s.md) | **从 docker-compose 到 K8s 的逐项映射** | 12 类字段对照；postgres / user-svc 真实迁移案例 |

### 路线篇

| # | 标题 | 你会学到 |
|---|------|---------|
| [12](./12-stage-28-roadmap.md) | **Stage 28+ 学习路线与推荐阅读** | 可观测 → HTTPS → GitOps → 生产 ACK → HA 的递进路径 |

---

## 怎么读这份文档

### 场景 A：第一次接触 K8s（按顺序 01 → 05 → 10 → 12）

时间：3 小时
收获：能向同事解释 K8s 是什么、我们项目怎么用、接下来学什么。

### 场景 B：写 Helm chart 时遇到问题（直接看 09 踩坑全记录）

时间：10 分钟
收获：找到你遇到的坑的修复方法。

### 场景 C：从 docker-compose 迁移（看 06、07、11）

时间：1 小时
收获：能照着手册把一份 compose 翻译成 Helm chart。

### 场景 D：要给团队做 K8s 培训（按顺序通读 12 篇）

时间：3-4 小时
收获：能讲 2-4 小时的内部培训课。

---

## 文档约定

### 章节结构

每篇文档遵循统一结构：

1. **一句话总结** —— 30 秒看懂这文档讲什么
2. **前置问题** —— 你为什么会读这文档
3. **核心内容** —— 分小节讲清楚
4. **本节自检** —— 5 个问题 + 答案（折叠）
5. **推荐阅读** —— 1-3 个外部资源

### 配套代码

所有文档都对应 Stage 27 的实际代码：

- **01-05** 对应 `charts/emotion-echo/` 顶层结构
- **06-08** 对应每个子 chart 的 `templates/` 文件
- **09** 对应 Stage 27 真实踩过的 18 个坑
- **10** 对应 `k8s/tests/render-assert_test.go`
- **11** 对应 `deploy/docker-compose.{infra,apps}.yml` → chart 的映射
- **12** 对应 Stage 28+ 的规划

---

## 我们项目的关键不变量（写文档时始终遵守）

1. **不修改业务代码** —— Helm chart 是新增层
2. **保留 docker-compose** —— 作为 dev / CI fallback
3. **APISIX 16 条路由语义不变** —— docker-compose → K8s 路由一致
4. **TDD 净增** —— 每个 chart 有对应的 render-assert 测试
5. **敏感信息用 Secret + values 占位符** —— 学习阶段不直接提交真密码

---

## Stage 27 已完成 vs Stage 28+ 待做

✅ **Stage 27 已完成**（本系列文档讲解范围）：
- 本地 kind 集群
- Helm umbrella chart（17 子 chart + 79 资源）
- APISIX Ingress + 16 条 ApisixRoute
- 4 个 namespace 分层
- ConfigMap + Secret + Probes + SecurityContext
- render-assert 测试套件

❌ **Stage 28+ 待做**（见 [12](./12-stage-28-roadmap.md)）：
- 可观测（Prometheus / Grafana / Loki）
- HTTPS / cert-manager
- GitOps（ArgoCD）
- 生产 ACK / ACR 迁移
- HA（HPA / PDB / 多副本）
- 安全合规（External Secrets / NetworkPolicy / OPA）

---

## 反馈与改进

如果你在阅读过程中发现：
- 哪一段讲得不够清楚
- 哪个坑没被记录
- 哪部分代码对不上
- 哪个图说不清楚

直接在 git 里开 issue 或 PR，我们会更新这系列文档。

---

> **下一步**：[01 为什么我们需要 K8s？](./01-why-kubernetes.md)