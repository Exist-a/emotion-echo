# 12 · Stage 28+ 学习路线与推荐阅读

> 系列：[11 docker-compose → K8s 映射](./11-compose-to-k8s.md) · **12 学习路线** · 结束

**适合谁**：完成了 Stage 27（本地 K8s 全量闭环）后想知道"下一步该学什么、什么时候学、坑是什么"的读者。
**读完你能**：画出 Stage 28-35 的学习地图，知道每个阶段的投入产出比，能挑下一步要深入的方向。

---

## 一句话总结

**Stage 27 = "本地能跑"。Stage 28-35 = "生产稳定 / 可观测 / 安全合规 / 多 region / GitOps"**。

按"先解决痛点"原则排序，建议路线：**可观测 → 安全 → GitOps → 生产迁移 → 高可用 → 多 region**。

---

## 一、Stage 27 完成后我们有什么

✅ 本地 kind 集群
✅ Helm umbrella chart（17 子 chart + 79 资源）
✅ APISIX Ingress + 16 条 ApisixRoute
✅ 4 个 namespace 分层
✅ ConfigMap + Secret + Probes + SecurityContext
✅ render-assert 测试套件
✅ docker-compose 作为 fallback 保留

❌ 没有 HPA（手动扩缩）
❌ 没有 PDB（自愿中断不保险）
❌ 没有 cert-manager（HTTP only）
❌ 没有 Prometheus / Grafana（metrics）
❌ 没有 Loki（日志聚合）
❌ 没有 GitOps（手动 helm install）
❌ 没有生产 ACK 部署

---

## 二、Stage 28-35 学习路线图

### Stage 28 · 可观测性三件套（推荐第一个）

**为什么第一个**：Stage 27 跑通后第一个问题是"现在系统怎么样了"。

| 组件 | 用途 | 投入 |
|------|------|------|
| **Prometheus** | metrics 采集（CPU、内存、QPS、延迟） | 1-2 天 |
| **Grafana** | metrics 可视化（dashboard） | 0.5 天 |
| **Loki** | 日志聚合（pod logs 集中存） | 1-2 天 |
| **Alertmanager** | 告警（CPU > 80% 触发告警） | 0.5 天 |
| **SkyWalking 接入** | 链路追踪（已经在用，要导出） | 0.5 天 |

**学习资料**：
- [Prometheus 官方文档](https://prometheus.io/docs/)
- [Grafana 官方教程](https://grafana.com/tutorials/)
- [Loki 入门](https://grafana.com/docs/loki/latest/getting-started/)

**预计产出**：
- `charts/observability/` 子 chart（Prometheus + Grafana + Loki）
- Grafana dashboard（业务 + 基础设施）
- 告警规则（CPU / 内存 / 错误率）

### Stage 29 · HTTPS / cert-manager

**为什么这个**：生产必须 HTTPS（明文 HTTP 容易被嗅探）。

| 任务 | 投入 |
|------|------|
| cert-manager 装到 ee-system | 0.5 天 |
| 自签证书（dev） | 0.5 天 |
| Let's Encrypt（prod） | 1 天 |
| APISIX Ingress 配 TLS | 0.5 天 |
| 改造 16 条 ApisixRoute 加 tls | 0.5 天 |

**学习资料**：
- [cert-manager 官方文档](https://cert-manager.io/docs/)
- [APISIX TLS 教程](https://apisix.apache.org/docs/apisix/tutorials/ssl/)

### Stage 30 · GitOps（ArgoCD）

**为什么这个**：手动 `helm install` 容易出错、难追溯。

| 任务 | 投入 |
|------|------|
| ArgoCD 装到 ee-system | 0.5 天 |
| Application yaml（指向 umbrella chart） | 0.5 天 |
| 多环境（dev/staging/prod） | 1 天 |
| Slack / 钉钉通知 | 0.5 天 |
| 自动 sync / 自动 prune | 0.5 天 |

**学习资料**：
- [ArgoCD 官方文档](https://argo-cd.readthedocs.io/)
- [ArgoCD 入门教程](https://argo-cd.readthedocs.io/en/stable/getting_started/)

### Stage 31 · 生产 ACK / ACR 迁移

**为什么这个**：本地 kind 不是生产，最终要上云。

| 任务 | 投入 |
|------|------|
| 创建 ACK 集群（terraform） | 1 天 |
| 创建 ACR 仓库（13 个镜像 push） | 0.5 天 |
| values-prod.yaml（镜像仓库 + 资源 + replicas） | 1 天 |
| 域名 + DNS 解析 | 0.5 天 |
| 第一次部署 + smoke | 1 天 |
| 监控 + 告警对接（短信/钉钉） | 1 天 |

**学习资料**：
- [阿里云 ACK 文档](https://help.aliyun.com/product/85222.html)
- [阿里云 ACR 文档](https://help.aliyun.com/product/60716.html)

### Stage 32 · 安全合规

**为什么这个**：生产面临安全审查。

| 任务 | 投入 |
|------|------|
| External Secrets + 阿里云 KMS | 1-2 天 |
| NetworkPolicy（限制 ns 间通信） | 1 天 |
| PodSecurityPolicy / PSA | 1 天 |
| OPA Gatekeeper（策略校验） | 1-2 天 |
| image 漏洞扫描（trivy） | 0.5 天 |

**学习资料**：
- [External Secrets Operator](https://external-secrets.io/)
- [NetworkPolicy 文档](https://kubernetes.io/docs/concepts/services-networking/network-policies/)
- [OPA Gatekeeper](https://open-policy-agent.github.io/gatekeeper/)

### Stage 33 · 高可用（HA）

**为什么这个**：单点故障是 Stage 27 的最大遗留问题。

| 任务 | 投入 |
|------|------|
| Postgres HA（Patroni + 3 副本） | 3-5 天 |
| Kafka 多 broker（3 副本） | 2-3 天 |
| etcd 3 副本 | 1 天 |
| Redis Sentinel / Cluster | 1-2 天 |
| APISIX 多副本 + Keepalived | 1 天 |
| HPA（基于 CPU / 自定义指标） | 1-2 天 |
| PDB（PodDisruptionBudget） | 0.5 天 |
| Cluster Autoscaler（节点自动扩缩） | 1-2 天 |

**学习资料**：
- [Patroni 文档](https://patroni.readthedocs.io/)
- [HPA 深入](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/)

### Stage 34 · 多 region / 灾备

**为什么这个**：生产最后一步——容灾。

| 任务 | 投入 |
|------|------|
| 多 region ACK 集群 | 5 天 |
| 跨 region 数据库同步（DR） | 5-10 天 |
| 全局流量管理（GTM） | 2 天 |
| 异地多活 / 单元化 | 10+ 天 |

**学习资料**：
- [多集群 K8s 模式](https://kubernetes.io/docs/concepts/cluster-administration/federation/)

---

## 三、按"性价比"排序的推荐路径

```
最优先（投入小，价值大）：
  ① Stage 28 可观测（2-3 天让你看见系统）
  ② Stage 29 HTTPS（半天让你能上域名）
  ③ Stage 30 GitOps（1 天让你不再手动 apply）

次优先（投入中，价值中）：
  ④ Stage 31 上 ACK（必须做，但 ops 偏多）
  ⑤ Stage 33 HPA / HA（流量起来后必备）

后置（投入大，价值特定场景）：
  ⑥ Stage 32 安全合规（看甲方要求）
  ⑦ Stage 34 多 region（看业务规模）
```

---

## 四、每阶段的最小学习投入

| 目标 | 最小学习 |
|------|---------|
| 跑通可观测 | 看 30 分钟 Prometheus 视频 + 抄 1 份 Helm chart |
| 配 HTTPS | 看 cert-manager 文档 1 小时 + 抄 1 份 ClusterIssuer yaml |
| 上 GitOps | 看 ArgoCD getting started 1 小时 + apply 1 个 Application |
| HPA | 看 HPA 文档 30 分钟 + apply 1 个 HPA |
| 上 ACK | 看阿里云 ACK 快速入门 2 小时 + 改 values-prod |

---

## 五、推荐阅读分级

### 入门（先读）

| 资源 | 价值 |
|------|------|
| 《Kubernetes in Action》Marko Luksa | K8s 圣经 |
| 《Kubernetes 权威指南》龚正 等 | 中文圣经 |
| [K8s 官方 Interactive Tutorial](https://kubernetes.io/docs/tutorials/kubernetes-basics/) | 30 分钟 |
| [Helm 官方文档](https://helm.sh/docs/) | chart 必读 |
| [APISIX 官方文档](https://apisix.apache.org/docs/) | 网关必读 |

### 进阶（深入时读）

| 资源 | 价值 |
|------|------|
| 《Kubernetes 进阶实战》马哥教育 | 进阶 |
| 《Production Kubernetes》 | 生产落地 |
| [K8s The Hard Way](https://github.com/kelseyhightower/kubernetes-the-hard-way) | 理解 K8s 内部 |
| [Helm Best Practices](https://helm.sh/docs/chart_best_practices/) | chart 设计 |
| [Prometheus 实战](https://prometheus.io/docs/prometheus/latest/) | metrics |

### 专家（特定方向）

| 方向 | 资源 |
|------|------|
| **Service Mesh** | 《Istio 实战》/ [Istio 文档](https://istio.io/latest/docs/) |
| **GitOps** | 《GitOps: Road to Continuous Delivery》/ [ArgoCD 文档](https://argo-cd.readthedocs.io/) |
| **多集群** | [Karmada](https://karmada.io/) / [Cluster API](https://cluster-api.sigs.k8s.io/) |
| **Serverless K8s** | [Knative](https://knative.dev/) / [阿里云 ASK](https://www.alibabacloud.com/product/kubernetes) |
| **eBPF** | [Cilium](https://cilium.io/) / [eBPF 文档](https://ebpf.io/) |

---

## 六、避坑建议（来自 Stage 27）

### 不要一次学完所有 K8s 概念

**错**：先看完整本书再动手（看 3 个月还没跑通）。
**对**：边做项目边查文档（Stage 27 做了 1 周就掌握 80% 知识）。

### 不要追求"完美架构"

**错**：从一开始就要 GitOps + HA + 多 region。
**对**：本地 kind 跑通 → ACK 跑通 → 再考虑 HA。

### 不要忽略 docker-compose

**错**：K8s 上线后删 docker-compose。
**对**：保留 compose 作为**快速调试**工具（CI、单机开发都比 K8s 快）。

### 不要在生产前用 `:latest` tag

**错**：CI build 出来 `:latest`，所有环境都拉同一个。
**对**：`:v0.1.0` + git SHA（如 `:v0.1.0-abc1234`）。

### 不要把 Secret 提交进 git

**错**：`values-prod.yaml` 含真实密码 commit 进去。
**对**：用 `values-prod-secret.yaml`（gitignored）+ External Secrets + KMS。

---

## 七、Stage 27 之后的 K8s 实战 checklist

```
□ 加 Prometheus + Grafana（看见指标）
□ 加 Loki（聚合日志）
□ 加 cert-manager + HTTPS
□ 加 ArgoCD（GitOps）
□ 加 HPA（自动扩缩）
□ 加 PDB（自愿中断保护）
□ 加 NetworkPolicy（ns 间隔离）
□ 加 External Secrets（生产 KMS）
□ 加 OPA Gatekeeper（策略校验）
□ 加 image 扫描（trivy）
□ 申请 ACK + ACR 账号
□ 写 values-prod.yaml
□ 跑 ack smoke
□ 备份 etcd + postgres
□ 写 Runbook（事故响应手册）
```

---

## 八、本节自检

1. **Stage 27 完成后第一个该学的 stage 是？**
2. **HPA 和 PDB 解决什么问题？**
3. **GitOps 工具最常用的是什么？**
4. **本地 kind 集群和 ACK 的核心差异是什么？**
5. **生产前的 Secret 管理用什么？**

<details>
<summary>📋 参考答案</summary>

1. Stage 28 可观测（Prometheus + Grafana + Loki），因为"看不见的系统无法改进"。
2. HPA = 自动扩缩 Pod（流量大自动加副本）；PDB = 自愿中断时保证 N 个 Pod 可用（节点维护时不全部驱逐）。
3. ArgoCD（GitOps 工具，监听 git 仓库变化自动 apply）。
4. ACK 是阿里云托管 K8s（master 阿里云管、worker 你管）+ 阿里云 VPC / SLB / 云盘深度集成；本地 kind 是 Docker 容器节点，无云集成。
5. External Secrets Operator + 阿里云 KMS（生产不能直接用 K8s Secret）。

</details>

---

## 九、推荐阅读

| 资源 | 链接 |
|------|------|
| 《Kubernetes in Action》 | https://www.manning.com/books/kubernetes-in-action |
| 《Production Kubernetes》 | https://www.oreilly.com/library/view/production-kubernetes/9781492090026/ |
| CNCF Landscape | https://landscape.cncf.io/ |
| K8s GitHub | https://github.com/kubernetes/kubernetes |
| 阿里云 K8s 最佳实践 | https://help.aliyun.com/document_detail/160337.html |

---

> **🎉 Stage 27 学习文档系列完结**

回到首页：[00 学习路径](./00-index.md)