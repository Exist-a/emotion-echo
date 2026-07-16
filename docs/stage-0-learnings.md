# Stage 0 落地总结 + 个人学习路径指南

> ⚠️ **架构决策请看 [architecture-decisions.md](./architecture-decisions.md)（ADR）**。
> 本文档保留为历史过程记录（2026-07-13 当时状态）。
> **当前已变更**：go-zero → Gin；Nacos → 删除。

> 写日期：2026-07-13  
> 目标：把今天 / 昨天落地的所有内容，按"做了什么、为什么这么做、背后学到了什么、下面该学什么"四个角度串起来。  
> 阅读建议：**先粗看一遍第四节（学习路径）**，根据你的兴趣回头看具体章节。

---

## 一、今天我们到底做了什么（一条命令清单）

### 1.1 基础设施层（deploy/ 目录）

```bash
docker-compose -f deploy/docker-compose.infra.yml up -d
```

这一条命令拉起了 **8 个容器**，构成一个最小可用的分布式环境：

| # | 容器 | 端口（宿主机→容器） | 在系统里的角色 | 我们怎么用它的 |
|---|------|------|------|------|
| 1 | postgres | 5432 | 关系型数据库 | 业务持久化 |
| 2 | redis | 6379 | 缓存 / 分布式锁 / 队列 | JWT 黑名单 / 会话缓存 |
| 3 | **nacos** | 8848+9848(gRPC) | 服务注册中心 + 配置中心 | Phase 1 给微服务发现用 |
| 4 | **etcd** | 2379+2380 | APISIX 配置存储 | APISIX 把路由写在 etcd 里 |
| 5 | **APISIX** | 9080 / 9180 / 9091 | API 网关（边缘入口） | 统一入口 / 鉴权 / 限流 |
| 6 | **apache/kafka** | 9092 | 分布式消息队列 | Phase 2 给异步任务用 |
| 7 | **SkyWalking OAP** | 11800(gRPC)/12800(HTTP) | 链路追踪后端 | Gin 通过 go2sky 上报 |
| 8 | **SkyWalking UI** | 18080 | 链路追踪前端 | 浏览器看 trace |

### 1.2 业务代码层（emotion-echo-gin/）

3 个文件改动，把 Gin 接进了 SkyWalking：

| 文件 | 改动类型 | 干了什么 |
|------|------|------|
| `internal/pkg/skywalking/skywalking.go` | 新建 | 包装了 `go2sky.NewTracer` + `reporter.NewGRPCReporter` |
| `internal/middleware/trace.go` | 新建 | 返回 `func(http.Handler) http.Handler` 包装器 |
| `cmd/server/main.go` | 改 3 行 | 启动时 `skywalking.Init()` + `middleware.APM()(handler)` 套最外层 |

跑起来后，访问 `http://localhost:18100/health` 等任意接口，会自动在 SkyWalking UI 上看到一条 trace。

### 1.3 中途发现的"踩坑清单"

| # | 坑 | 解决 |
|---|----|------|
| 1 | `bitnami/kafka:3.6` 镜像 tag 已废弃 | 换成 `apache/kafka:3.7.0` 官方镜像 |
| 2 | postgres 拒空密码 | 加 `POSTGRES_HOST_AUTH_METHOD=trust` + 显式密码 |
| 3 | APISIX 初始设成 standalone，加 env var 没用 | 加 etcd 容器，切 traditional 模式 |
| 4 | APISIX 配置文件名不是 `conf.yaml` 而是 `config.yaml` | 改挂载路径 |
| 5 | APISIX 的 `etcd:` 配置要放在 `deployment:` **下面**，不是顶层 | 修正 yaml 结构 |
| 6 | SkyWalking 的 `SW_HEALTH_CHECKER=true` 找不到 module | 去掉该 env var |
| 7 | SkyWalking UI 启动时 `SW_OAP_ADDRESS` 必须带 `http://` scheme | 加全路径 |
| 8 | `localhost:8080` 被系统 ApplicationWebServer 占用 | 改用 `EE_SERVER_PORT=18100` |
| 9 | PowerShell 把 `curl` 解析成 `Invoke-WebRequest` | 改用 `curl.exe` |
| 10 | admin container exec 跨命令会被打断 | 改用日志文件 + 长度短命令 |

---

## 二、每个组件在系统中的角色（这是练手最该搞清的事）

整个系统按职责可以分成 **5 层**，从下到上：

```
┌───────────────────────────────────────────┐
│  ⑤  业务应用  emotion-echo-gin  (Go 单体)  │  ← 现在的你
├───────────────────────────────────────────┤
│  ④  API 网关  APISIX                       │  ← 上线统一入口 / 鉴权
├───────────────────────────────────────────┤
│  ③  可观测   SkyWalking OAP + UI          │  ← 全链路可视化
├───────────────────────────────────────────┤
│  ②  通信/协调  Nacos  +  Kafka  +  etcd   │  ← 注册中心 / 队列 / 配置
├───────────────────────────────────────────┤
│  ①  数据层    Postgres  +  Redis          │  ← 旧组件，先不动
└───────────────────────────────────────────┘
```

### ⑤ 业务应用（你写的）
- **现在状态**：单体 Gin 服务，已经接入 SkyWalking
- **未来去向**：按业务领域拆微服务（Phase 1 起）

### ④ API 网关（APISIX）

**两件事**：
1. **统一入口**：所有外部请求（来自浏览器 / APP / 第三方）都进 APISIX，再由 APISIX 转发到具体的微服务。
2. **统一策略**：插件式的能力——你写一段 Lua 或 YAML 就能加这些：
   - `jwt-auth` 鉴权
   - `limit-count` 限流
   - `cors` 跨域
   - `prometheus` 指标
   - `skywalking` 链路透传
   
**核心概念 —"数据面 vs 控制面"**：
- **数据面**（9080）= 真正处理 HTTP 请求的 OpenResty，**不能重启**
- **控制面**（9180 Admin API / 9181 Dashboard）= 配置变更入口
- 配置存在 etcd，所以 etcd 挂了不影响流量，重启 APISIX 配置还在

**企业实战**：APISIX 95% 和 Kong 同款，但用 Apache 协议 + 用 etcd（不是 PostgreSQL）+ 用 Lua/Admin API，不像 Kong 那样需要 restart reload。

### ③ 可观测（SkyWalking）

**三件事**：
1. **Trace（链路）**：一次请求从浏览器到 DB 的完整调用链
2. **Metrics（指标）**：每个服务的 QPS / 延迟 / 错误率
3. **Logs（日志）**：可以基于 traceId 把日志串起来

**SkyWalking 的两条数据通道**：
- **gRPC 端口 11800**：Agent（go2sky / javaagent）上行数据
- **HTTP 端口 12800**：UI / API Client 下行查询

**OAP 是啥？** = "Observability Analysis Platform"，SkyWalking 的后端，做两件事：
- 接收 agent 来的 raw 段数据
- 把它聚合 → 存到存储（H2 / ES / MySQL）→ 暴露 API 给 UI

**核心概念 —"Span / Trace / Segment"**：
- **Span**：一个操作（比如一次 DB 查询）
- **Trace**：一次请求跨多个服务的整个调用链
- **Segment**：一个 JVM / Go 进程内的 span 集合

我们 go2sky 上报的是 Segment，OAP 自动把它和上下游连成 Trace。

### ② 通信 / 协调层

| 组件 | 用于 | 为什么选这个 |
|------|------|------|
| **Nacos** | 服务发现 + 配置中心 | 阿里出品，社区活跃，一个组件顶俩 |
| **Kafka** | 异步消息队列 | 单机延迟低、生态强、社区最大 |
| **etcd** | APISIX 配置存储 | APISIX 官方推荐 |

**重点名词 —"控制面 / 数据面"再次出现**：
- Nacos 既是注册中心（gRPC 9848）又是配置中心
- Kafka 既有 Leader 也有 Follower，有自己的选举机制（KRaft）
- 它们都自己维护集群一致性，对业务透明

### ① 数据层
老朋友 PostgreSQL + Redis，没动。

---

## 三、今天的关键领悟（这些是你三天前可能还不知道的）

| 领悟 | 解释 |
|------|------|
| **分布式不是"装组件"** | 装组件 5 分钟，真正难的是 **组件选型**（今天我们踩了多少镜像坑）、**组件间版本匹配**（APISIX 跟 etcd、go2sky 跟 OAP 要版本对齐）、**故障兜底**（OAP 挂了 agent 不能阻塞主链路——这就是为啥我们让 tracer 失败时 skip 而不是 panic） |
| **APM 不是可选项** | 现代分布式系统的"调试工具"。没有 SkyWalking，一个慢请求从浏览器传到 DB，**你完全不知道卡哪了**。我们今天亲手写了一个 noop fallback 中间件，意义就是：**业务必须能跑、没有 APM 也要能查问题** |
| **跨服务需要 trace 上下文传递** | 我们只接了 Gin→OAP 的单条链。**Phase 1 改造后**，user-svc→chat-svc→DB 这种链路就要靠 **header 传递**（W3C traceparent / sw8）把 traceId 串起来 |
| **治理 / 业务分离** | 上面 ④③②① 都是"治理系统"，**不是业务**。一家公司早期都在业务代码里搞这些，后期全栈抽出来做平台。这就是为什么 go-zero 这种框架会自带限流熔断负载均衡，因为它把治理抽象成 service base 库 |
| **etcd vs Nacos 职能重叠** | 看起来功能重复，但设计哲学不同：<br>**etcd**：强一致性 KV，专为频繁重写（APISIX 配置秒级变更）优化；无服务发现抽象<br>**Nacos**：服务发现 + 配置，弱一致性，对人友好（带控制台）；有 namespace/group 概念 |

---

## 四、现在该学什么（按优先级排序）

### 🌟 第一优先级：把"今天做的"亲手动一遍

不学新东西之前，先确认你能完全独立复现：

- [ ] **删掉所有容器**，重新 `docker-compose up -d`，能一次成功
- [ ] **杀掉 server-new**，重新 `go build` + 启动
- [ ] **改 `SKY_SERVICE_NAME` 成你的名字**（比如 `my-emotion-echo`），看 UI 上是否变了
- [ ] **故意把 11800 端口改错**，看 server 是否如预期"无声启动继续提供 API"
- [ ] **打开 APISIX 控制台** http://localhost:9180/apisix/admin/routes （即使 403 也试试看 header 怎么传）

> 时间：1~2 小时就能搞定

### 🌟 第二优先级：SkyWalking 的"内核原理"

由于我们已经把它跑通了，下一个最深 ROI 的学习领域：

| 资源 | 形式 | 时长 | 推荐理由 |
|------|------|------|------|
| [Dapper, a Large-Scale Distributed Tracing System (Google 论文)](https://research.google/pubs/dapper-a-large-scale-distributed-tracing-system/) | 论文 | 2 小时 | 所有 APM 系统的祖师爷 |
| [Apache SkyWalking 官方文档 - Architecture](https://skywalking.apache.org/docs/main/latest/en/concepts-and-designs/overview/) | 文档 | 1 小时 | 我们现成的实现，等于看自己用的东西 |
| [OpenTelemetry 中文文档](https://opentelemetry.io/zh/docs/) | 文档 | 3 小时 | OTel 是行业标准，未来跳槽都用得上 |

**学完后能回答的问题**：
- Span 怎么跨进程传 traceId？
- 为什么采样率很重要（为了不让 APM 自己成为瓶颈）？
- SkyWalking 内部存储为什么一般换成 ES？

### 🌟 第三优先级：APISIX 插件体系

我们目前只知道它是个网关，**还不知道它的杀手锏 — 插件**。学习：

- [APISIX 核心概念](https://apisix.apache.org/zh/docs/apisix/terminology-api-gateway/)（30 分钟）
- 跑通这几个插件 demo：`limit-count`、`jwt-auth`、`key-auth`、`cors`、`prometheus`、`skywalking`（每个 30 分钟）

我们文档里提到的 route plugins 写法，就是从这里开始。

### 🌟 第四优先级：Kafka 基础

下一步 Phase 2 我们要让情绪分析走 Kafka，**你至少要知道**：
- Producer / Consumer / Broker / Topic / Partition 是什么
- 至少一次 vs 最多一次 vs 恰好一次语义
- 为什么需要 ack=0/1/all

推荐：[Apache Kafka 中文教程](https://kafka.apache.org/zh/documentation/) 入门章节（约 2 小时）。

### 🚀 第五优先级：准备 Phase 1（go-zero）

最重要！但 **不是今天的事**。这是从"动手做"到"动手做架构"的转折：

1. `go-zero` 框架的核心：**API 定义文件 → RPC 自动生成**，**RPC → gRPC + protobuf**
2. go-zero 的"服务发现 / 负载均衡 / 限流"怎么通过配置就接进 Nacos

> 建议：先看完 SkyWalking 原理 + Kafka 基础，再开始 Phase 1。  
> 不然我们会陷入"配置地狱"——一边学框架一边调 Nacos，跨域。

---

## 五、推荐阅读顺序（已经按 ROI 排好）

```
今天（1h）
  ↓ 玩一下今天的东西，亲手 delete / recreate / 修改

明天（2h）  
  ↓ Dapper 论文 + SkyWalking Architecture

后天（3h）
  ↓ APISIX 官方文档 + 跑 3 个插件

周末（4h）
  ↓ Kafka 基础 + 写一段 producer/consumer demo

下周一（1-2 天）
  ↓ 进入 Phase 1：go-zero 改造用户服务
```

---

## 六、自测清单（确认 Phase 0 真正掌握）

读完这份文档，你能回答以下问题吗？

- [ ] 我能在没有看代码的情况下，完整画出今天 8 个容器的拓扑图
- [ ] 我能用 30 秒讲清楚 APISIX 与 Nginx 区别
- [ ] 我能说清楚 SkyWalking OAP 与 UI 的数据流向（哪边推、哪边拉）
- [ ] 我能解释为什么 etcd 也能做"服务发现"但我们选了 Nacos
- [ ] 我能解释为什么我们让 `skywalking.Init()` 失败时应用依然能跑
- [ ] 我能在 5 分钟内自己写一段 go2sky 的 hello-world 代码

如果答不上来，回去重读对应章节。
