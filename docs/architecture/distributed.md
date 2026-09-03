# Emotion-Echo 分布式架构 · 选型说明

> ⚠️ **架构最终决策以 [architecture-decisions.md](./architecture-decisions.md)（ADR）为单一事实源**。
> 本文档是 ADR 的补充说明，解释**为什么**做这些选型。
> 最后更新：2026-07-14

---

## 1. 顶层架构（dev 阶段）

```
浏览器 (Nuxt 前端)
    │
    ▼ HTTP
┌─────────────────────────┐
│  APISIX 网关 :9080       │  ← 鉴权 / 限流 / 熔断 / CORS / 审计日志
└────────────┬────────────┘
             │ 路由（etcd 存配置）
   ┌─────────┼─────────┬─────────┬─────────┬─────────┐
   ▼         ▼         ▼         ▼         ▼         ▼
user-svc assessment chat-svc ai-svc analytics llm-svc
:8888    :8889       :8890    :8891    :8892     :8000
(Gin)    (Gin)       (Gin)    (Gin)    (Gin)     (Python)

┌────────────────────────────────────────────────────────┐
│  基础设施层                                             │
│  Postgres (5 schema) + Kafka + SkyWalking + etcd       │
│  ❌ 不使用 Nacos（ADR 决策 2）                          │
└────────────────────────────────────────────────────────┘
```

---

## 2. 技术栈选型总览

| 类别 | 选型 | 替代方案 | 决策原因 |
|------|------|---------|---------|
| **HTTP 框架** | **Gin** | go-zero, chi, net/http | 复用 legacy 业务代码 + 团队熟悉 |
| **服务发现** | **APISIX + etcd** | Nacos, Consul, Eureka | APISIX 原生后端，少组件 |
| **API 网关** | **Apache APISIX** | Kong, Nginx | 国产最热，插件丰富，K8s 友好 |
| **消息队列** | **Apache Kafka** | RabbitMQ, RocketMQ | 行业标配，Kafka Streams 生态 |
| **链路追踪** | **Apache SkyWalking** | Jaeger, Zipkin | APM 一体化，国产生态最好 |
| **数据库** | **PostgreSQL** | MySQL | 已有，schema 隔离天然适配多租户 |
| **缓存** | **Redis** | — | 已有，未来分布式锁/限流 |
| **配置中心** | **本地 yaml** | Nacos Config, Consul KV | 简单直接，演进到 K8s ConfigMap |
| **内部 RPC**（未来）| **gRPC + proto** | HTTP/REST | 跨语言契约标准 |
| **Python AI** | **FastAPI**（HTTP）/ gRPC（未来）| Flask, Django | 轻量 + async |
| **监控** | **SkyWalking**（dev） | Prometheus+Grafana | dev 够用，K8s 时再升级 |

---

## 3. 选型深度说明

### 3.1 HTTP 框架 = Gin（不是 go-zero）

**最初选择 go-zero 的理由**：
- 自带 RPC/熔断/限流/缓存/分布式任务
- 一站式，少胶水代码
- 中国大厂（字节/滴滴/得物）大规模生产

**改用 Gin 的理由**（基于实际使用反馈）：
- 实际只用到 go-zero 的 30%（HTTP server + goctl 模板）
- go-zero 的核心特性（zrpc/breaker/limit）我们都没用
- legacy `emotion-echo-gin` 有完整业务代码（14 个 handler）
- 团队已经熟悉 Gin，零学习成本

**结论**：用 Gin 不是因为"更好"，而是因为"在我们这套场景下更实际"。

### 3.2 服务发现 = APISIX + etcd（不是 Nacos）

**最初选择 Nacos 的理由**：
- 阿里出品，国产最热
- 同时承担注册中心 + 配置中心
- 与 APISIX/K8s 集成好

**改用 APISIX+etcd 的理由**（基于实际使用反馈）：
- Nacos 注册了但没人读（APISIX 不读 Nacos）
- Nacos 挂了导致所有 svc 起不来（强耦合故障源）
- APISIX + etcd 已经把"路由 + 配置存储"包圆了
- 简化架构：少一个组件 = 少一个故障点

**结论**：**当一个组件只写不读时，它就是负担**。删。

### 3.3 跨服务调用 = gRPC（未来）

**当前**：HTTP（ai-svc → emotion-llm-service）

**未来升级 gRPC 的理由**：
- **跨语言契约**：proto 文件是单一事实源（Go client + Python server 自动生成）
- **强类型**：编译期检查 API 契约
- **HTTP/2**：多路复用、二进制编码、性能高
- **流式支持**：未来 SSE 推送、实时情绪分析可用 stream RPC

**外部 API 仍然走 HTTP REST**（浏览器不能直接调 gRPC）。

### 3.4 部署 = 本地 Docker → K8s manifests

**dev 阶段**：docker-compose（多容器在一台机）

**未来 K8s 阶段**：
- manifests 准备好但不部署
- 等代码稳定、APISIX 鉴权插件完成后再上 K8s
- K8s 阶段配置用 ConfigMap / Secret（不是 Nacos）

### 3.5 鉴权 / 限流 / 熔断 = APISIX 网关层

**不在 svc 内部做的原因**（白盒审计原则）：
- 鉴权：每个 svc 都做 = 重复 5 次 + 容易漏改 = 不安全。网关做一次 = 集中可控
- 限流：网关知道全局流量，svc 只知道自己
- 熔断：网关是 svc 的统一入口，能保护所有 svc

**白盒审计视角**：
- 网关配置（APISIX yaml）= 审计可查
- svc 业务代码 = 不掺杂鉴权/限流，纯粹
- 职责清晰 = 审计容易

---

## 4. 与"原栈"的差异（心里有数）

| 原方案（legacy Gin 单体）| 升级后（微服务架构）|
|--------------------------|---------------------|
| Gin 单进程 | 5 个 Gin svc + 1 Python svc |
| Gin 自带路由 | Gin + APISIX（双层路由）|
| 直接 DB 连接 | svc → 各自 schema → Postgres |
| 无消息队列 | Kafka（异步事件）|
| 无服务发现 | APISIX + etcd（静态 upstream）|
| 自写 JWT 中间件 | APISIX jwt-auth（统一）|
| 无熔断 | APISIX api-breaker |
| 无审计日志 | APISIX access log + 各 svc JSON 日志 |
| 无链路追踪 | SkyWalking（go2sky）|
| 无统一日志 | dev: 各 svc out.log；prod: Loki/Grafana |

---

## 5. 风险与权衡

| 风险 | 应对 |
|------|------|
| 删除 Nacos 后失去服务列表视图 | Docker ps + APISIX route 列表 + SkyWalking 拓扑已够用 |
| go-zero → Gin 迁移需重写 5 个 svc | 1 天工作量，业务代码从 legacy 搬过来 |
| APISIX 静态 upstream 扩缩容麻烦 | 现阶段 svc 数量固定（5+1），未来可升级 etcd lease |
| gRPC 浏览器不能直接调 | 外部 API 仍走 HTTP REST + APISIX |
| K8s 部署未实践 | manifests 先写好，验证后再上 |

---

## 6. 已敲定的决策汇总

详见 [architecture-decisions.md](./architecture-decisions.md)，包括：

| # | 决策项 | 选择 | 替代方案 |
|---|--------|------|---------|
| 1 | HTTP 框架 | **Gin** | go-zero, chi |
| 2 | 服务发现 | **APISIX + etcd** | Nacos, Consul |
| 3 | 部署形态 | Docker (dev) + K8s manifests | 纯 K8s, 纯 Docker |
| 4 | 跨服务协议 | HTTP（dev）→ gRPC（未来）| 全 HTTP, 全 gRPC |
| 5 | Python AI 形态 | 独立服务 + gRPC server | 内嵌进 ai-svc |
| 6 | 审计方式 | JSON 日志 + trace_id | ELK（重量）|
| 7 | 鉴权位置 | APISIX jwt-auth | svc 内部 |
| 8 | 限流熔断 | APISIX 插件 | svc 内部 |

---

## 7. 后续演进路径

```
Phase 0 基础设施       ✅ Docker Compose 起齐所有中间件
Phase 1 微服务拆分      ✅ 5 个 Gin svc + 5 schema 拆分
Phase 2 Kafka          ✅ 异步管道 chat-events 跑通
Phase 3 LLM 接入       ✅ 跨语言情绪分析（HTTP → 升级 gRPC）
Phase 4 业务深化        🔄 emotion 查询闭环
Phase 5 韧性+网关鉴权   ⏳ jwt-auth + limit-count + api-breaker
Phase 6 K8s manifests  ⏳ 每个 svc deployment + service yaml
Phase 7 gRPC 升级      ⏳ proto 定义 + ai-svc 升级
Phase 8 业务完整化      ⏳ 14 个 handler 全部迁移
```

---

## 8. 不再使用的内容（清退清单）

| 旧组件 | 替代 | 状态 |
|--------|------|------|
| go-zero 框架 | Gin | 待迁移 |
| goctl 代码生成器 | 手写 handler | 待废弃 |
| go-zero zrpc | gRPC + proto | 待升级 |
| go-zero breaker/limit | APISIX 插件 | 待配置 |
| go-zero conf.MustLoad | yaml.Unmarshal | 待迁移 |
| go-zero logx | log/slog | 待迁移 |
| Nacos 注册中心 | APISIX + etcd | 待删除 |
| Nacos 配置中心 | yaml + K8s ConfigMap | 不引入 |
| 自写 discovery 包 | 无（svc 不主动注册）| 待删除 |
| svc mock X-User-Id | APISIX jwt-auth | 待替换 |
| 发现式服务路由（client-side LB）| 直连 + APISIX upstream | 不引入 |

---

## 9. 命名约定

| 类型 | 命名 | 示例 |
|------|------|------|
| Go svc 二进制 | `emotion-echo-{domain}-svc.exe` | `chat-svc.exe` |
| Go svc 目录 | `emotion-echo-{domain}-svc/` | `emotion-echo-chat-svc/` |
| Python svc | `emotion-{purpose}-service/` | `emotion-llm-service/` |
| DB schema | `emotion_echo_{domain}` | `emotion_echo_user` |
| 表名 | `{schema}.{domain}_entity` | `emotion_echo_chat.conversations` |
| Go module | `github.com/emotion-echo/{svc}` | `github.com/emotion-echo/chat-svc` |
| Kafka topic | `{domain}-events` | `chat-events` |
| Port | 8888 + domain index | user:8888, chat:8890 |
| Trace svc name | `emotion-echo-{domain}-svc` | `emotion-echo-chat-svc` |