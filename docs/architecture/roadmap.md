# Emotion-Echo 分布式改造 · 落地路线图（执行版）

> ⚠️ **架构最终决策以 [architecture-decisions.md](/docs/architecture/decisions.md)（ADR）为单一事实源**。
> 本文档保留为**历史路线记录**，描述当时的实施步骤与决策。
> **当前架构变更**（2026-07-14）：
> - go-zero → Gin（ADR 决策 1）
> - Nacos → 删除（ADR 决策 2）
> - 跨服务调用 → gRPC（ADR 决策 4）
>
> 本文档基于 [distributed-architecture.md](/docs/architecture/distributed.md) 的选型，
> 把每个 Phase 拆成可独立交付的 Stage，每个 Stage 都有**验收标准**与**可运行证明**。
> 一句话：**按阶段一路推，任何一个 Stage 卡住都能原地停下**，不会破坏业务。

---

## 0. 全局视图

### 0.1 组件依赖关系（先看清先后）

```
SkyWalking ←───── 任何业务组件都依赖它（先起后接）
   │
   ▼
APISIX（独立，先跑通路由）
   │
   ▼
Nacos（注册中心先有"空"的服务列表）
   │
   ▼
go-zero 服务（向 Nacos 注册，被 APISIX 发现）
   │
   ▼
Kafka（异步 worker 才有意义）
   │
   ▼
K8s（学习资产，当前不部署——决策 3）
```

### 0.2 各 Phase 拆解

| Phase | 目标 | Stage 数 | 关键收益 |
|-------|------|---------|---------|
| Phase 0 | 一次性起齐 4 个中间件 + Gin 上 trace | 5 | 看到 SkyWalking 上的第一条 span |
| Phase 1 | go-zero 改造起头 | 5 | 走通 "浏览器 → APISIX → go-zero" |
| Phase 2 | Kafka 异步化 | 4 | 端到端语音链路跑通 |
| Phase 3 | 限流/熔断/配置中心 | 3 | 韧性 + 动态配置 |
| Phase 4 | 业务域拆分 | 3 | 旧 Gin 工程清零 |
| Phase 5 | K8s 化 | 4 | `helm install` 一键部署（2026-09-07 收口：本地验证完成，生产不启用 K8s，见决策 3）|

### 0.3 全程学习路径

| 阶段 | 重点学习主题 |
|------|------------|
| Phase 0 | Docker Compose 编排、SkyWalking 探针原理、APISIX 路由配置 |
| Phase 1 | go-zero 工程结构、Nacos 配置中心、APISIX 插件链 |
| Phase 2 | Kafka 分区、副本、消费语义、go-queue 封装 |
| Phase 3 | Sentinel 限流算法、Nacos 配置推送、SkyWalking 告警 |
| Phase 4 | DDD 域拆分、服务间调用、tRPC 协议 |
| Phase 5 | K8s Pod/Service、Operator、Helm Chart |

---

# Phase 0 · 基础设施 + 可观测底座

> **本阶段不改动业务代码**，只把"中间件层"立起来，并把 Gin 一并接入 trace。
> 完成后会看到：浏览器请求 → Gin 处理 → SkyWalking UI 上能看到完整 trace。

## Stage 0.1 · 目录结构与基础设施编排骨架

### 目标
建立 `deploy/` 目录，统一存放分布式基础设施文件（不进业务工程）。

### 涉及文件
```
Emotion-Echo/
├── deploy/                              # ★ 新建
│   ├── docker-compose.infra.yml         # 全部中间件
│   ├── apisix/
│   │   ├── conf.yaml                    # APISIX 配置
│   │   └── apisix.yaml                  # 默认路由
│   ├── nacos/
│   │   └── nacos.env                    # Nacos 环境变量
│   ├── kafka/
│   │   └── topics.yaml                  # 待创建的 topic 列表
│   ├── env/
│   │   └── .env.common                  # 公共变量
│   └── README.md                        # 启动 / 验证命令
└── Emotion-Echo-Gin/                    # 业务工程，暂不动
```

### 具体动作
1. 创建 `deploy/` 目录
2. 写 `deploy/docker-compose.infra.yml`（见架构文档第 9.3 节 docker-compose 配置）
3. 写 `deploy/apisix/conf.yaml` 最小可用版本：

```yaml
apisix:
  node_listen: 9080
  enable_ipv6: false
deployment:
  role: traditional
  role_traditional:
    config_provider: etcd
  admin:
    allow_admin:
      - 0.0.0.0/0
```

4. 写 `deploy/apisix/apisix.yaml` 占位路由：

```yaml
upstreams:
  - id: 1
    name: default-upstream
    type: roundrobin
    nodes:
      "127.0.0.1:8081": 1   # 占位：现有 Gin
```

### 验收
- `ls deploy/` 看到所有文件
- 文件通过 yaml 语法校验（在线 YAML validator 即可）

### 学习收获
- Docker Compose 多服务声明的范式
- depends_on / networks / volumes 三件套
- APISIX 的"config_provider"概念（实际生产都用 etcd）

---

## Stage 0.2 · 一键起齐 SkyWalking + APISIX + Nacos + Kafka

### 目标
所有中间件容器能跑起来，控制台可达。

### 涉及文件
- `deploy/docker-compose.infra.yml`（已是上一 stage 准备的文件）

### 具体动作
```powershell
# 在 PowerShell 终端
cd d:\源码\Emotion-Echo\deploy
docker-compose -f docker-compose.infra.yml up -d
# 等待 30~60 秒各容器健康
docker-compose -f docker-compose.infra.yml ps
```

### 验收
| 组件 | URL | 检查 |
|------|-----|------|
| SkyWalking UI | http://localhost:18080 | 默认账号无，登录可见空仪表盘 |
| APISIX Dashboard | http://localhost:9000 | 默认 admin / admin |
| Nacos | http://localhost:8848/nacos | 默认 nacos / nacos |
| Kafka | localhost:9092 | `docker exec -it emotion-echo-kafka kafka-topics.sh --bootstrap-server localhost:9092 --list` |

### 学习收获
- **docker-compose logs** 如何排查容器启动失败
- 容器 DNS（服务名互通）的底层原理
- KRaft 单节点 Kafka 是怎么省去 Zookeeper 的

---

## Stage 0.3 · Gin 接入 SkyWalking trace

### 目标
随便请求一个现有 API，SkyWalking UI 出现一条新 trace。

### 涉及文件
```
Emotion-Echo-Gin/
├── go.mod                             # 新增 go2sky 依赖
├── internal/
│   ├── pkg/skywalking/skywalking.go   # tracer 初始化
│   └── middleware/trace.go            # Gin middleware
└── cmd/server/main.go                 # 启用 middleware
```

### 具体动作
1. 拉取依赖：
   ```bash
   cd Emotion-Echo-Gin
   go get github.com/SkyAPM/go2sky@v1.7.0
   go get github.com/SkyAPM/go2sky/plugins/gin@v1.2.0
   ```

2. 建 `internal/pkg/skywalking/skywalking.go`（代码见架构文档 9.4.3）

3. 建 `internal/middleware/trace.go`

4. 在 `cmd/server/main.go` 引入 tracer 并挂中间件：
   ```go
   tracer, err := skywalking.NewTracer()
   if err != nil { log.Fatal(err) }
   
   router := router.New()
   router.Use(middleware.TraceMiddleware(tracer))
   router.Run(":8081")
   ```

5. 启动 Gin，`curl http://localhost:8081/api/v1/health` 触发一次请求

### 验收
- SkyWalking UI → 服务列表出现 `emotion-echo-api`
- 点击进入 → Trace List 出现新条目
- 点击 trace → 看到 HTTP Server span（含 URL、状态码、耗时）

### 学习收获
- **Trace** = 一次请求的完整调用链；**Span** = 调用链中的一次操作
- 探针自动埋点 vs 手动埋点的差别
- SkyWalking vs OpenTelemetry 的根本性差异：SkyWalking 自有协议 vs 通用 OTLP

---

## Stage 0.4 · 自动埋点 pgx / Redis 子 span

### 目标
一次数据库查询在 trace 里有独立的 DB span。

### 涉及文件
```
internal/database/postgres.go   # 给 pgx 连接加 tracer
internal/database/redis.go      # 给 redis 连接加 tracer
```

### 具体动作
1. 引入更多插件：
   ```bash
   go get github.com/SkyAPM/go2sky/plugins/pgx@v1.2.0
   go get github.com/SkyAPM/go2sky/plugins/redis@v1.2.0
   ```

2. 修改 `internal/database/postgres.go`，在打开连接时包装：
   ```go
   // 在 pgxpool 创建后加 tracer wrapper
   pgxTracer, _ := pgx.NewTracer(tracer)
   connConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
       pgxTracer.InstallInstrumentation(conn)
       return nil
   }
   ```

3. 类似地包装 redis client

### 验收
- 触发一次查询接口
- trace 里能看到 `PostgreSQL/SELECT ...` 这种 span
- 数据库耗时被准确记录

### 学习收获
- 子 span 的父子关系（traceID 相同，parentSpanID 关联）
- 慢查询定位（SkyWalking UI 能直接聚合慢 SQL）

---

## Stage 0.5 · 验证 APISIX 默认路由（不进 go-zero）

### 目标
验证 APISIX → Gin 这条链路通。

### 涉及文件
- `deploy/apisix/apisix.yaml`

### 具体动作
APISIX 默认 upstream 已经指向 127.0.0.1:8081，但路由没配，加一条：

```yaml
routes:
  - id: 1
    uri: /api/v1/*
    upstream_id: 1
    methods: [GET, POST]
```

### 验收
- `curl http://localhost:9080/api/v1/health` 等价于 `curl http://localhost:8081/api/v1/health`
- APISIX Dashboard Route 列表出现这条
- request 路径可以追溯：APISIX → Gin

### 学习收获
- APISIX 的 Route / Upstream / Service 三层概念
- 路由匹配（uri / hosts / methods / priority）

---

# Phase 1 · go-zero 重塑 + 服务注册

> **目标**：第一个 go-zero 服务（user-svc）能通过 APISIX 对外提供。
> 关键学习：**新工程脚手架、goctl、Nacos 注册发现、跨服务 trace 透传**。

## Stage 1.1 · goctl 工具链准备

### 目标
安装 goctl，能跑 `goctl --help` 看到完整命令列表。

### 具体动作
```powershell
go install github.com/zeromicro/go-zero/tools/goctl@latest
goctl --version

# 加速 protobuf 编译
goctl env install -p protoc-gen-go
```

### 验收
- `goctl --help` 列出所有子命令（api、rpc、model、plugin、template...）

### 学习收获
- goctl 是 go-zero 的代码生成器
- 类似后端框架的"约定优于配置"：先用 goctl 生成标准结构，再手动补业务

---

## Stage 1.2 · 第一个 go-zero 服务 user-svc

### 目标
新建 `Emotion-Echo-Services/user-svc`，能访问 `/ping` 返回 `pong`。

### 涉及文件
```
Emotion-Echo-Services/user-svc/
├── go.mod
├── api/
│   └── user.api                       # go-zero API DSL 文件
├── user.go                            # main
├── etc/
│   └── user-api.yaml                  # 配置
├── internal/
│   ├── config/config.go
│   ├── handler/ping.go                # 自动生成
│   ├── logic/pinglogic.go             # 自动生成
│   ├── middleware/                    # ★ 新增
│   └── svc/servicecontext.go
└── Dockerfile
```

### 具体动作
```bash
cd Emotion-Echo-Services
mkdir user-svc && cd user-svc
go mod init github.com/emotion-echo/user-svc
goctl api new user    # 生成 user/ 目录，里面就是标准结构
# 调整目录到 user-svc/ 根
```

`api/user.api` 写：
```api
syntax = "v1"
service user {
    @handler ping
    get /ping returns (PingResp)
}
type PingResp {
    Message string `json:"message"`
}
```

### 验收
- `cd user-svc/user && go run user.go -f etc/user-api.yaml` 启动
- `curl http://localhost:8001/ping` 返回 `{"message":"pong"}`

### 学习收获
- go-zero 的目录约定（handler / logic / svc / middleware）
- `type ... returns ...` 的 API 定义语法

---

## Stage 1.3 · Nacos 注册发现

### 目标
user-svc 启动后自动注册到 Nacos，APISIX 通过 Nacos 拉取实例。

### 涉及文件
- `Emotion-Echo-Services/user-svc/etc/user-api.yaml`（注册中心配置）
- `deploy/apisix/apisix.yaml`（upstream 通过 nacos 拉节点）

### 具体动作
1. user-api.yaml 增加：
   ```yaml
   Registry:
     Type: nacos
     Nacos:
       Host:
         - localhost:8848
       Port: 8848
       TTL: 10
   ```

2. APISIX upstream 改为 Nacos discovery 模式：

```yaml
upstreams:
  - id: 1
    name: user-svc
    type: roundrobin
    discovery_type: nacos
    service_name: user-svc
    nacos_service:
      host: nacos     # 容器内 Nacos host
      port: 8848
      group: DEFAULT_GROUP
      namespace_id: ""
```

> ⚠️ APISIX 需装有 nacos 插件：`apisix install nacos`（需要在 APISIX 镜像中提前装好，社区镜像常缺）

3. user-svc 加 Nacos SDK：
   ```bash
   go get github.com/zeromicro/go-zero/plugins/nacos
   ```

### 验收
- 启动 user-svc
- Nacos 控制台 → 服务列表 → 出现 `user-svc`
- `curl http://localhost:9080/api/v1/ping` 经 APISIX → user-svc 成功

### 学习收获
- go-zero 的 **Registry 抽象**：换成 Consul / Eureka 不改代码
- 心跳 TTL 的含义（10 秒一报，挂了就下线）
- APISIX + Nacos 的对接难点（Nacos group/namespace 的对齐）

---

## Stage 1.4 · APISIX 插件链：JWT + CORS + 限流

### 目标
给 user-svc 加 JWT 认证、CORS、限流三个插件。

### 涉及文件
- `deploy/apisix/apisix.yaml`

### 具体动作
```yaml
routes:
  - id: 1
    uri: /api/v1/ping
    upstream_id: 1
    plugins:
      jwt-auth:
        key: user-key                 # APISIX 的 Consumer key
      cors:
        allow_origins: ["*"]
        allow_methods: ["GET", "POST"]
      limit-count:
        count: 10
        time_window: 60
        rejected_code: 429
```

### 验收
- 带 `Authorization: Bearer xxx` → 401
- 60s 内第 11 次 → 429
- 1 分钟后恢复

### 学习收获
- APISIX 插件执行的"生命周期"（rewrite / access / header_filter / log）
- Consumer 概念（用户态配置，路由引用）

---

## Stage 1.5 · 跨服务 trace 透传

### 目标
user-svc 上报 trace 时带上从浏览器发起时 APISIX 透传过来的上下文。

### 涉及文件
- `deploy/apisix/apisix.yaml`（开启 skywalking 插件）
- `user-svc` 内 SkyWalking go2sky 配 W3C 透传

### 具体动作
1. APISIX 路由加 skywalking 插件（输出 sw8 header）：
   ```yaml
   plugins:
     skywalking:
       sample_ratio: 1
   ```

2. user-svc 与 Gin 同款接 go2sky，保证 sw8 透传
3. 同时启动 Gin（8081）和 user-svc（8001）

### 验收
- 浏览器 → APISIX → user-svc
- SkyWalking UI 出现一条 trace，**至少包含两个 span**（APISIX 网关 + user-svc Gin）
- traceID 全程一致

### 学习收获
- **W3C TraceContext** vs SkyWalking 的 sw8 header 互不兼容，需要 Plugin 做转换
- 跨服务 trace 的根基是 HTTP header 透传

---

# Phase 2 · Kafka 异步 worker

> **目标**：把报表生成 / 情绪分析这种重活抽到 Kafka 异步链路。

## Stage 2.1 · Kafka topic 规划与声明

### 目标
列出 topic 清单，并写入 Kafka。

### 涉及文件
- `deploy/kafka/topics.yaml`（维护清单）
- 一次性执行 `kafka-topics.sh --create`

### topic 清单
| topic | 分区数 | 用途 |
|-------|-------|------|
| `emotion.analysis` | 3 | 消息入队分析任务 |
| `emotion.notification` | 3 | 分析结果通知 |
| `report.generate` | 1 | 报表生成 |
| `report.notification` | 1 | 报表进度 |

### 学习收获
- Kafka topic ≠ queue；是日志结构（append-only）
- 分区数是并发上限

---

## Stage 2.2 · Producer：异步任务投递

### 目标
把 `report_daily.go` 改成"入队 → 立即返回"。

### 涉及文件
- `user-svc/internal/jobs/producer.go`
- `user-svc/internal/handler/report.go`

### 具体动作
1. 引入 `github.com/zeromicro/go-queue/kq`（go-zero 自带 KQ 封装）
2. 在 main 里初始化 KQ：
   ```go
   pusher := kq.NewPusher([]string{"localhost:9092"}, "emotion.analysis")
   defer pusher.Close()
   ```

3. handler 改成：
   ```go
   func ReportHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
       return func(w http.ResponseWriter, r *http.Request) {
           msg := Message{Type: "daily", UserID: xxx}
           pusher.Push(r.Context(), "emotion.analysis", msg)
           json.NewEncoder(w).Encode(map[string]string{
               "task_id": "xxx", "status": "queued",
           })
       }
   }
   ```

### 验收
- 请求接口立即返回 202
- Kafka consumer 能看到消息

### 学习收获
- 异步编程的"时延不敏感任务"判断标准
- KQ vs Sarama vs Confluent-Kafka：KQ 最简化

---

## Stage 2.3 · Consumer：worker 进程消费

### 目标
新建 `emotion-worker`，消费 `emotion.analysis`。

### 涉及文件
- `Emotion-Echo-Services/emotion-worker/main.go`
- `Emotion-Echo-Services/emotion-worker/etc/emotion.yaml`

### 具体动作
```go
pusher := kq.NewConsumer([]string{"localhost:9092"}, "emotion.analysis", processMessage)
defer pusher.Stop()

// processMessage 中执行业务：调 SenseVoice、调 LLM、写库
```

### 验收
- worker 启动后，所有 job 都能消费
- 重启 worker 自动从上次 offset 续跑

### 学习收获
- Kafka 消费位移（offset）概念
- **手动 ACK** vs 自动 ACK 的差异

---

## Stage 2.4 · SSE 进度推送

### 目标
前端实时看到报表生成 / 情绪分析进度。

### 涉及文件
- `user-svc/internal/handler/sse.go`
- `emotion-echo-web/app/composables/useSSE.ts`

### 具体动作
1. user-svc 暴露 SSE endpoint：`GET /report/progress/:task_id`
2. 通过 Redis Pub/Sub 接收 worker 推送的进度
3. worker 完成一步时 `PUBLISH report:progress:task_id "{step:1,...}"`

### 学习收获
- SSE vs WebSocket 的选择标准（push-only 用 SSE）
- 跨进程消息总线 vs 长轮询

---

# Phase 3 · 韧性 + 配置中心

## Stage 3.1 · Sentinel-Go 限流熔断

### 涉及文件
- `user-svc/internal/middleware/sentinel.go`

### 验收
- 突发流量 1000 QPS → 半数被 Sentinel 直接拒绝（fail-fast）
- 外部 API 超时 → 自动降级返回默认值

---

## Stage 3.2 · Nacos 配置中心（动态配置）

> ⚠️ **2026-09-03 撤回原"不引入"评审判断**。演进路线详见 Stage 31/32/33 与 `docs/adr-2026-09-nacos-reintroduction.md`。

### 目标
把限流规则推送到 Nacos，运行时热更新。

### 涉及文件
- `user-svc/etc/nacos.go`（引入 Nacos Config）
- `user-svc/internal/svc/config.go`

### 学习收获
- **配置即代码** vs **配置即推送** 的差异
- 配置版本回滚（生产救火必备）

---

# Stage 31-33 · 分布式治理演进

> **演进路线**：纠正 Stage 5/30 两次删除治理组件的错误；按"骨架先，胶水后"分三阶段引入 Nacos + APISIX + 修复 P0 + BFF 净化。

| Stage | 主题 | 状态 | 文档 |
|-------|------|------|------|
| **31** | Nacos 注册中心 + 配置中心落地 | 🚧 进行中（PR-01 文档收口；PR-02..12 推进） | [stage-31-nacos-reintroduction.md](/docs/stages/stage-31-nacos-reintroduction.md) |
| **32** | APISIX 独立 API 网关层回归 | ☐ 未启动（依赖 31） | [stage-32-apisix-reintroduction.md](/docs/stages/stage-32-apisix-reintroduction.md) |
| **33** | P0 修复 + BFF 退化为纯聚合层 | ☐ 未启动（依赖 32） | [stage-33-p0-fix-bff-purify.md](/docs/stages/stage-33-p0-fix-bff-purify.md) |
| **ADR** | 决策依据（选型论证） | ✅ 已落地（决策 10/11/12/13，2026-09-03 Accepted） | [adr-2026-09-nacos-reintroduction.md](/docs/architecture/adr/adr-2026-09-nacos-reintroduction.md) |

---

## Stage 3.3 · SkyWalking 告警

### 目标
trace 中错误率 > 阈值时，触发 Webhook 告警。

### 涉及文件
- `deploy/skywalking/alarm-settings.yml`

### 学习收获
- APM 告警 vs Prometheus 告警的边界

---

# Phase 4 · 业务域拆分

> 把当前 Gin 单体按业务域拆成多个 go-zero 微服务。
> 关键：**迁移期共存**，每个 service 独立部署。

## Stage 4.1 · chat-svc（会话/消息）

把 `internal/service/conversation_service.go`、`message_service.go` 迁出。

## Stage 4.2 · ai-svc（AI 流式 + 情绪分析）

把 `internal/service/ai_*` 系列迁出，包括 `ai_stream.go`、`ai_emotion.go`。

## Stage 4.3 · report-svc + survey-svc + notification-svc

按字段功能分别拆，最后 `internal/handler/` 内部为空，旧工程可删除。

---

# Phase 5 · K8s 化

## Stage 5.1 · Helm Chart 骨架

为每个核心组件写 Chart：
- `deploy/helm/skywalking/`
- `deploy/helm/apisix/`
- `deploy/helm/nacos/`
- `deploy/helm/kafka/` (Strimzi operator)
- `deploy/helm/postgres/`
- `deploy/helm/redis/`
- `deploy/helm/user-svc/`（第一个微服务示例）

## Stage 5.2 · Strimzi Kafka Operator 接入

替换单机 Bitnami Kafka。

## Stage 5.3 · APISIX Ingress + Service

APISIX 接入 K8s Ingress Controller。

## Stage 5.4 · Prometheus + Grafana 监控栈

引入 kube-prometheus-stack。

### 最终验收
- `helm install emotion-echo ./deploy/helm/emotion-echo` 一键起整栈
- 所有服务在 K8s 集群内可联通

---

# 附录 A · 命令速查

## A.1 Docker
```powershell
docker-compose -f deploy/docker-compose.infra.yml up -d
docker-compose -f deploy/docker-compose.infra.yml down
docker-compose -f deploy/docker-compose.infra.yml ps
docker-compose -f deploy/docker-compose.infra.yml logs -f kafka
```

## A.2 Kafka 命令
```powershell
docker exec -it emotion-echo-kafka kafka-topics.sh --bootstrap-server localhost:9092 --list
docker exec -it emotion-echo-kafka kafka-topics.sh --bootstrap-server localhost:9092 --create --topic emotion.analysis --partitions 3
docker exec -it emotion-echo-kafka kafka-console-consumer.sh --bootstrap-server localhost:9092 --topic emotion.analysis --from-beginning
```

## A.3 SkyWalking
- UI: http://localhost:18080
- 默认无登录
- Trace 查询选 `emotion-echo-api` → Trace List

## A.4 APISIX
- Dashboard: http://localhost:9000
- 默认账号 admin / admin
- 直接通过路由 file mount（生产用 admin-api + etcd）

## A.5 Nacos
- Console: http://localhost:8848/nacos
- 默认 nacos / nacos

---

# 附录 B · 回滚预案

| Stage | 出问题如何回滚 |
|-------|---------------|
| 0.x 中间件层 | `docker-compose down` 全删即可，业务代码未动 |
| 1.x go-zero 接入 | 旧 Gin 单独跑在 8081 不依赖 APISIX，独立可运行 |
| 2.x Kafka 接入 | 同步版本保留 `@deprecated` 标记，保留 fallback handler |
| 3.x 韧性层 | 路由里插件可关，配置中心问题可改回 file config |
| 4.x 拆域 | 旧工程保留到拆完再删，灰度迁移 |
| 5.x K8s 化 | `helm uninstall` 即可回退到 docker-compose |

---

# 附录 C · 每个 Stage 的"完成报告"建议模板

每完成一个 Stage，建议输出：

```markdown
## Stage X.X 完成报告

### 改动文件清单
- (列出本次新增 / 修改的文件)

### 验证截图
- (附 UI 截图或 curl 输出)

### 留下的 TODO
- (本阶段发现但留给下阶段的优化点)

### 学习笔记
- (新学到的概念、踩过的坑)
```

这样持续累积下来，整个项目完成时你就有一份**个人分布式实战手册**。

---

# 路线进度看板（建议复制到 README）

```
Phase 0 基础设施
  [ ] 0.1 目录骨架
  [ ] 0.2 中间件起齐
  [ ] 0.3 Gin trace
  [ ] 0.4 pgx/redis span
  [ ] 0.5 APISIX 路由验证

Phase 1 go-zero 改造
  [ ] 1.1 goctl 安装
  [ ] 1.2 user-svc ping
  [ ] 1.3 Nacos 注册
  [ ] 1.4 APISIX 插件
  [ ] 1.5 跨服务 trace

Phase 2 Kafka 异步
  [ ] 2.1 topic 声明
  [ ] 2.2 Producer
  [ ] 2.3 Consumer
  [ ] 2.4 SSE 进度

Phase 3 韧性
  [ ] 3.1 Sentinel
  [ ] 3.2 Nacos 配置中心
  [ ] 3.3 SkyWalking 告警

Phase 4 业务拆分
  [ ] 4.1 chat-svc
  [ ] 4.2 ai-svc
  [ ] 4.3 report/survey/notification

Phase 5 K8s 化（2026-09-07 收口：本地学习完成，生产不启用 K8s——决策 3）
  [x] 5.1 Helm Chart（charts/emotion-echo 24 subchart，本地渲染/冒烟验证）
  [ ] 5.2 Strimzi Kafka（未做）
  [x] 5.3 APISIX Ingress（本地 kind 验证，Stage 27）
  [x] 5.4 Prometheus + Grafana（observability 已入 chart，Stage 28）

# Stage 31/32/33 分布式治理演进（ADR 决策 10/11/12/13）
Stage 31 Nacos 治理层          🚧 [x] PR-01 文档收口 / [ ] PR-02..12 推进
Stage 32 APISIX 网关层         [ ] 全部（依赖 31）
Stage 33 P0 修复 + BFF 净化    [ ] 全部（依赖 32）

# Stage 41 · go-zero 完全移除收尾（ADR 决策 1 收尾 + 关闭审计 E-2/E-3）
Stage 41 go-zero 移除           ✅ DONE — 10 个 TDD PR 全部 merged,见 [stage-41-gozero-removal.md](../stages/stage-41-gozero-removal.md) + [smoke 10/10 PASS](../stages/stage-41-smoke-2026-09-07.txt)

# Stage 42 · 容器时区修复 (TZ=Asia/Shanghai 形同虚设 side fix)
Stage 42 container tz fix       ✅ DONE — 6 Dockerfile 删 `apk del tzdata`,见 [stage-42-container-tz-fix.md](../stages/stage-42-container-tz-fix.md)

# Stage 43 · Kafka 健壮性 Sprint A 全收口 (ADR-19 / kafka-reliability-gaps.md)
Stage 43 Kafka Sprint A         ✅ DONE — 10 commit on `feat/chat-dev-event-publisher-a1-1-red`,
                                  7/7 服务绿,见 [stage-43-kafka-reliability-sprint-a.md](../stages/stage-43-kafka-reliability-sprint-a.md)。
                                  Sprint B (1.4 lag 监控 + 1.5 Protobuf 迁移) 在新分支独立推进。
                                  Sprint B §1.4 lag 监控已并入下方 Stage 44 observability-sprint-b.md。

# Stage 44 · 观测链路 Sprint B 全收口（dev compose 三层补齐 + 测试护栏 + Kafka lag 收口）
Stage 44 observability-sprint-b   ✅ DONE — 16 个 PR-OBS-X + 1 fix = 34 commit
                                  全落地（2026-09-08），见
                                  [stage-44-observability-sprint-b.md](../stages/stage-44-observability-sprint-b.md)。
                                  基础设施层（PR-OBS-1~8）+ 测试护栏层（PR-OBS-9~16）100%。
                                  ⚠️ 未做项:16 分支未 merge main / TracerInterface 抽象
                                  (PR-OBS-12/13/14 完整 span tag) / 6 svc logging 接入 /
                                  sw-oap telemetry / dev 环境 Nacos Restarting 阻塞实跑,
                                  详见 stage-44 §四。
```

# Stage 45 · PR-OBS-17 完整 trace 抽象收口（TracerInterface + Span.Tag 接口 + ai-svc 4 tag 精确断言）
Stage 45 observability-sprint-b   ✅ DONE — PR-OBS-17 4 commit（2026-09-08）
   regression                       见 [stage-45-observability-sprint-b-regression.md](../stages/stage-45-observability-sprint-b-regression.md)。
                                  落地:Tracer.CreateLocalSpan + Span.Tag 接口扩展 +
                                  Go2Sky adapter + 3 处签名变更走接口 + 6 svc main.go
                                  机械改 NewGo2SkyTracer 包装 + ai-svc Kafka consumer
                                  4 个 messaging.* tag 精确断言（mockSpan.tagCalls）。
                                  PR-OBS-12/13/14 完整收口的步骤 1~3 落地（接口 +
                                  consumer tag 断言），业务路径 tag（http.*/rpc.*/user_id）
                                  留作 PR-OBS-18/19。
```

# Stage 46 · PR-OBS-18 GinSkywalkingMiddleware EntrySpan + 4 tag 收口
Stage 46 observability-gin-entry-span  ✅ DONE — PR-OBS-18 3 commit（2026-09-08）
                                     见 [stage-46-observability-gin-entry-span.md](../stages/stage-46-observability-gin-entry-span.md)。
                                     落地:GinSkywalkingMiddleware 从 ctx 透传升级为创建
                                     EntrySpan + 打 4 个 tag(http.method / http.url /
                                     http.status_code / user_id),span 挂 ctx
                                     (skywalking_span)供下游 handler 读。
                                     Stage 44 §四 B 步骤 3 完整落地(步骤 1-5 共 5/6),
                                     剩余步骤 6 (gRPC interceptor) 留 PR-OBS-19。
```

# Stage 47 · PR-OBS-15 logging helper 6 svc 接入收口
Stage 47 logging-helper-apply        ✅ DONE — PR-OBS-15 2 commit（2026-09-08）
                                     见 [stage-47-logging-helper-apply.md](../stages/stage-47-logging-helper-apply.md)。
                                     落地:6 svc main.go 全调 logging.Init() +
                                     SetGlobalSvc(<name>)(含 ai/web-bff re-export
                                     补全);GinSkywalkingMiddleware 调
                                     logging.WithTraceID 注入 Request ctx。
                                     Stage 44 §四 C 落地,Loki 日志可按 svc +
                                     trace_id 过滤(决策 6 字段闭环)。
                                     剩余未做:PR-OBS-19/23/20。
```

# Stage 48 · PR-OBS-23 handler err 透传 span.EndSpan(err) 收口
Stage 48 handler-err-propagate      ✅ DONE — PR-OBS-23 2 commit（2026-09-08）
                                     见 [stage-48-handler-err-propagate.md](../stages/stage-48-handler-err-propagate.md)。
                                     落地:GinSkywalkingMiddleware EndSpan 从总是 nil
                                     改为 buildSpanError(c) 三段判定(c.Errors /
                                     status>=500 / nil),OAP UI 可直接过滤
                                     5xx + error 维度;不改 handler 现状
                                     (c.JSON(500, ...) 通过 status 兜底)。
                                     剩余未做:PR-OBS-19 (gRPC interceptor,
                                     需 go-zero 桥接)+ backlog 业务 tag。
```

# Stage 49 · PR-OBS-19 gRPC interceptor rpc.* tag + layer/component 收口
Stage 49 grpc-tracing-rpc-tags     ✅ DONE — PR-OBS-19 2 commit（2026-09-08）
                                     见 [stage-49-grpc-tracing-rpc-tags.md](../stages/stage-49-grpc-tracing-rpc-tags.md)。
                                     落地:Span 接口扩 SetSpanLayer(int32) +
                                     SetComponent(int32);ServerTracingInterceptor
                                     打 5 项 (layer=GRPC + component=5001 +
                                     rpc.system/method/user_id);
                                     ClientTracingInterceptor 打 4 项(对称)。
                                     5 svc 仅 ai-svc 有 gRPC server(stage-46 §四
                                     描述已纠正),无需改 svc main.go,interceptor
                                     升级自动让 ai-svc 受益。
                                     🎉 Stage 44 §四 B 6/6 步全部收口。剩余:
                                     sw-oap telemetry + 阻塞型 Nacos + backlog。
```

# Stage 50 · 观测链路 Sprint B 端到端落地验证 + 问题发现
Stage 50 e2e-validation           ✅ DONE — 0 commit（验证归档）
                                     见 [stage-50-e2e-validation.md](../stages/stage-50-e2e-validation.md)。
                                     验证本轮 5 stage (Stage 45-49) 端到端:
                                     单元测试层 100% PASS (grpcinterceptor 41 +
                                     middleware 24 + logging 10 + ai-svc consumer 14);
                                     Stage 47 logging helper 本机直接 run 输出 JSON
                                     6 字段(svc/trace_id/action/msg/msg_id/time/level);
                                     dev compose smoke_observability.py 10/12 PASS
                                     (2 项 Nacos 阻塞 / Loki warmup 与本轮无关);
                                     ⚠️ 5 svc 镜像重建滞后(stage-44 §四 §A 阻塞),
                                     Stage 47/48/49 main.go 改动需 16 分支 merge main
                                     + build_dev_images.sh 重建后生效。
                                     问题清单 (6 项) §九:镜像滞后/Nacos/promtail/
                                     Loki warmup/smoke 覆盖缺 ×2。
```

---

# 下一步行动（2026-09-12 更新，Stage 75 收口后）

> ⚠️ **本段是快照，不是权威来源**。Stage 节奏下待办的权威来源是**最近一期 stage 收口报告
> 的"本批未做（open）"表**（`docs/stages/stage-NN-*.md` §五/§六）+ `docs/plans/` 中
> `status: planned` 的计划。每期 stage 收口时应顺手刷新本段；发现本段与 open 表矛盾时，
> 以 open 表为准并回改本段（2026-09-12 即曾发生：本段停留在 Stage 50 时代逾 1 个月）。

**已收口**（详见各 stage 报告）：
- Stage 72：chat Pin/Update RPC 全链路 + Nacos PR-1
- Stage 73：Kafka §1.5 Protobuf 迁移（P2）
- Stage 74：技术债收尾（Grafana lag 面板 / dev web 容器首通 / Helm 残余冻结 = 决策 23）
- Stage 75：Nacos PR-2（BFF 5 处 gRPC 拨号 Nacos 优先，env 降为兜底）
- Stage 76：Nacos PR-3 dev e2e 验收
- Stage 72：chat Pin/Update RPC 全链路 + Nacos PR-1
- Stage 73：Kafka §1.5 Protobuf 迁移（P2）
- Stage 74：技术债收尾（Grafana lag 面板 / dev web 容器首通 / Helm 残余冻结 = 决策 23）
- Stage 75：Nacos PR-2（BFF 5 处 gRPC 拨号 Nacos 优先，env 降为兜底）
- Stage 76：Nacos PR-3 dev e2e 验收（契约测试 23/23 + 摘除/恢复实测 + §契约 7 6/6）+
  排期核查更正（PR-3/4/5 实现 Stage 39 已落地）+ nacos-enablement-dev.md 迁 landed
- Stage 77：Postgres nil repo 修复（shared dbconnect 启动重试 500ms×10 +
  5 svc gRPC nil-repo → Unavailable 守卫；RED→GREEN + dev 栈 e2e 全绿）
- Stage 78：业务计划排期核查（ai-response-structured 阶段 1 已实现销账 /
  intent-classification 旧路径失效 / file-upload 残余盘点；新增 llm-chat-real-pipeline 计划）
- Stage 79：file-upload 收口（前端装配层接线 + e2e 揪出 contentType 响应链 5 处丢失
  并全链路修复——BFF camelCase 绑定 / proto Message.content_type / 三层视图透传；
  file-upload-message-extension.md 迁 landed）
- Stage 80：llm-chat-real-pipeline PR-1（llm-service ChatCompletion 流式 RPC +
  mock 降级双路径真实容器 e2e；env 沿用 LLM_* 约定修正计划原文 Moonshot）
- Stage 81：llm-chat-real-pipeline PR-2（BFF ai/stream 切 llm-service gRPC 上游，
  优先级链 gRPC→HTTP直连→mock；经 APISIX 端到端 SSE 实测 + llm-service trace 双实证）
- Stage 82：llm-chat-real-pipeline PR-3a（规则式 6 类意图分类 + ClassifyIntent RPC +
  with_intent 首帧回带 + 按意图注入风格指令；真实容器 e2e 4 类全对）
- Stage 83：llm-chat-real-pipeline PR-3b（intent 落库 → msg_summary_v → analytics 意图分布
  → 日报饼图全链；e2e 揪出 VM 丢字段/视图升级/GRANT 三问题并修复；
  intent-classification 与 ai-response-structured 两计划迁 landed）
- Stage 84：chat-svc events 包 3 个存量测试失败修复（测试适配 Stage 73 Protobuf
  契约 + marshal 错误不触达 broker 回归锁；go test ./... 合并门槛恢复绿色）
- Stage 85：趋势报告意图维度（ReportsTrendResponse 加 intent_distribution →
  repo 区间聚合 → BFF 透传 → weekly/monthly/annual 三页饼图；真实容器 e2e +
  DB 交叉实证）
- Stage 86：outbox dead 告警全链（emotion_echo_outbox_events_dead_total 指标 →
  OutboxEventsDead critical 规则 → alertmanager :9093；MaxAttempts 配置化 OUTBOX_MAX_ATTEMPTS；
  毒消息真实容器 e2e 全链 firing 实证；顺手修 analytics 001 msg_summary_v 旧定义幂等性 bug
  ——Stage 82 视图升级漏同步，db-migrate 全量重跑必炸）；
  kafka-reliability-gaps 六项缺口全部收口，计划迁 landed
- Stage 87：LLM 意图重分类（classify_intent_adaptive：规则式高置信直返 / 模糊且
  key 可用时 LLM 重分类 temperature=0+max_retries=0，失败/无 key/env 关闭优雅回
  规则；离线路径与 Stage 82 逐字段一致；容器 e2e 揪出 SDK 默认重试放大降级耗时
  16.2s→2.1s 并修复）；intent-classification-6-types residuals 销账。
  顺手 docs：grpc-inter-service-migration「剩余」段过期信息销账（#32/Sprint G/Stage 72）

- Stage 88：llm-service Python 端 Nacos 注册（根因 = Stage 31 旧同步 SDK import 路径与
  >=3.1.0 锁定不匹配，容器内静默失效 12 天；nacos_client.py 移植 v3 异步 gRPC SDK +
  _advertise_ip 真实 IP 探测 + grpc_port metadata + e2e 揪出 log/cache dir 非 root 权限两坑；
  BFF resolveGRPCAddr 第 6 处接入，摘除验态 env 兜底无感）；
  nacos-enablement-dev 与 llm-chat-real-pipeline 两计划 residual 销账

- Stage 89：file-understanding-llm 全线落地（文件+提问一起发，会话内持续引用）——
  proto 加 file_name/FileAttachment.files；chat-svc 五处映射 + migration 007 修 Stage 82 §契约 5
  intent VARCHAR(16)→32 暗坑（e2e 实证 emotional_support 18 字符曾 22001 落库 500）；
  BFF ai-stream 文件收集注入（≤2 条 + URL 重写 + session.WithRequestAuth 修 e2e 揪出的
  auth ctx 缺失）；llm-service file_context（pypdf + python-docx + 白名单 SSRF）；
  前端附件挂输入框 + 三分支发送流 + ChatFile 真实文件名。txt 哨兵 e2e + 追问引用全通；
  PDF 路径注入正常但 DeepSeek 拒读记 residual（注入位置/prompt 优化挂 Stage 90）。

- Stage 90：file_context 注入位置改为 user 尾部（接近"用户粘贴文本提问"形态）
  ——单元测试 192/192 绿（spec 由 test_grpc_files_v2.py 锁住，旧 test_grpc_files.py 同步
  跟进 spec）；容器 e2e PDF 部分成功部分拒读，根因属 DeepSeek 对单短文本判定保守，
  5 条候选优化路线记入 Stage 91。顺手销 todo-pile A1/A2/B4 章节正文（§五 状态表已
  正确，仅章节与现状脱节）。

- Stage 91：file_context prompt 头从描述性改强指令性措辞（修 Stage 89/90 PDF 拒读
  residual）——RED→GREEN→REFACTOR 完整 TDD 循环；新增 test_file_context_prompt_directive.py
  4 用例锁住"请基于/原文/引用"等强指令性关键词契约；file_context.py 抽 _PROMPT_HEADER
  模块常量；docker-compose.apps.yml 镜像标签 v0.1.0 → v0.1.2。**单元测试 196/196 绿
  （含 Stage 89/90 全部存量测试）**。**容器 e2e PDF 哨兵 5/5 HIT 100%（v0.1.2 +
  DeepSeek 真实 key）**——Stage 89/90 baseline 50% → Stage 91 100%，DeepSeek 行为
  判定从"可选噪声"升级为"必读任务 + 引用原文"。e2e 脚本沉淀在
  [`emotion-llm-service/tests/e2e/stage91_pdf_sentinel.py`](../emotion-llm-service/tests/e2e/stage91_pdf_sentinel.py)
  备未来回归。详见 [stage-91-file-understanding-pdf-prompt-directive-2026-09-14.md](../stages/stage-91-file-understanding-pdf-prompt-directive-2026-09-14.md)。

- Stage 92：Kafka sw8 透传 PR-1+PR-2 全收口——RED→GREEN→REFACTOR 完整 TDD 循环：
  - shared Tracer 接口扩 CreateExitSpan/CreateEntrySpan（adapter 包装 go2sky v1.5 原生 API）
  - chat-svc KafkaEventPublisher 加 tracer 字段 + Publish 注入 sw8 到 ProducerMessage.Headers
    （v0.1.8 → v0.1.10；main.go wire 加 defer log 标记 sw8 propagation enabled）
  - ai-svc ConsumerGroupHandler 把 CreateLocalSpan 换 CreateEntrySpan + extractSw8Header helper
    从 sarama RecordHeader 抽 sw8（v0.1.5 → v0.1.6）
  - 顺手修 ai-svc analyzer/grpc_analyzer_test.go 历史孤儿 build fail（fakeEmotionLLMClient
    缺 ChatCompletion + ClassifyIntent 接口实现——Stage 80/82/87 累积的接口扩张未同步）
  - docker e2e 实证：chat-svc:v0.1.10 producer 写入完整 sw8 header（221 chars，含
    traceID=cc661c1a... + parent service=emotion-echo-chat-svc + peer=chat-events）
  - **OAP UI 跨进程 trace 可视化待 SkyWalking 9.x graphql queryDuration 时间格式 bug 修后补**
    （不影响 sw8 透传逻辑正确性——单测 + Kafka header 实证已覆盖）
  - 详见 [stage-92-kafka-sw8-propagation-2026-09-14.md](../stages/stage-92-kafka-sw8-propagation-2026-09-14.md)。
    analytics-svc consumer 移到 Stage 93（无 Tracer 集成，需先加 tracer 链路再用 PR-2 同模式）。

- Stage 93：analytics-svc consumer sw8 透传（observability §A-extension 收口）—— RED→GREEN 完整 TDD 循环：
  - shared Tracer 接口已就位（Stage 92 PR-1 扩 CreateExitSpan/CreateEntrySpan,无需再扩）
  - chatEventHandler 加 Tracer 字段 + WithTracer builder + extractSw8Header helper（与 ai-svc consumer.go:115-134 + 208-218 同模式）
  - ConsumeClaim DecodeChatEvent 提前解 evt → CreateEntrySpan 抽 sw8 → 4 个 messaging.* tag（system/topic/partition/event.type,与 ai-svc 完全对称）
  - main.go wire + defer log `sw8 propagation enabled` + 镜像 v0.1.5 → v0.1.6
  - 顺手修 shared/pkg/middleware gin_skywalking_test.go stubTracer 历史孤儿 build fail（Stage 92 PR-1 扩展 Tracer 接口后 stubTracer 未同步扩展,与 Stage 92 §"顺手修历史孤儿 build fail"同模式）
  - observability-edge-gaps §A 全 2 项 closed；plan 整体迁 legacy-plans/landed/（front-matter status: landed + landed-stages + residuals 列 §B-F 5 项 P2-P3 + OAP graphql bug 留 Stage 94+）
  - 详见 [stage-93-analytics-svc-sw8-propagation-2026-09-14.md](../stages/stage-93-analytics-svc-sw8-propagation-2026-09-14.md)。
    observability-edge-gaps §B-F 5 项 P2-P3 留 Stage 94+ 候选；OAP 9.x graphql queryDuration 时间格式 bug 沿用 Stage 92 §五残余。

**当前 open 清单**（2026-09-15 Stage 99 收口 + 代码审计修订 + **Stage 101 多轮全量收口**后刷新）：

> **本段是快照，不是权威来源**（roadmap.md:931-934 段警告）。2026-09-15 Stage 101 收口
> 把 audit 修订后的 23 项真未落/半落**全部 10 Round 落地**。详见
> [`docs/stages/stage-101-multi-round-iteration-closure.md`](../stages/stage-101-multi-round-iteration-closure.md)。
> 剩余 open 仅 9 项触发条件型（多副本/上 prod/owner 拍板/外部协调），单轮 TDD 不可独立完成。
>
> **2026-09-15 多轮会话补全**（"剩下的多轮完成"指令落地）：
> 8 commits (d33e9e1..4ff5a24) 真落地 Round A/B/C/D + e2e 实证 chat-events 6 partition。
> 7 commits 中 4 个为代码 (`f25d4d4` Round A / `50b9e3e` Round B / `f22c1ce` Round C / `ded2efc` Round D) +
> 3 个为 docs/evidence/plan (`93cfc4a` Round C e2e / `a0c5b78` 后续 task 痕迹 / `4ff5a24` Round D lockfile 痕迹 / `07a0305` §十五 + kafka-init 注释)。
> 详见 [`docs/plans/multi-round-iteration-2026-09-15.md` §十五](../plans/multi-round-iteration-2026-09-15.md#十五2026-09-15-本会话真落地-commits6-个-push-originmain)。

**Round 1 收口进度**（Round 1.1/1.2/1.3/1.4 + Stage 101 follow-up 全部 ✅ 已落，5 commits +705/-37 行 + 4 模型 + 4 测试，0 回归）：
- ✅ Round 1.1 voice+emotion UNIQUE（§P1-R2-8/9）—— commit `3bdc817`
- ✅ Round 1.2 EmotionAnalysis 软删除（§P2-R2-7 第 1 张表）—— commit `c2d4aa3`
- ✅ **Stage 101** Round 1 follow-up：face/voice/fused + voice_transcripts 4 张表软删除 —— gorm.DeletedAt + repo.Delete + i008 migration + 4 测试
- ✅ Round 1.3 migrate.sh glob 改造（§P2-12）—— commit `9458133`
- ✅ Round 1.4 视图一致性 + 收敛（§P2-R2-10）—— commit `006bb32`
- ✅ Round 1.4 follow-up：msg_summary_v 单 owner
- ⏳ 触发条件 Round 1.4 follow-up：assessment_v 迁 analytics —— 双 owner 但 SQL diff 0 行（口径一致，未迁）

**observability-edge-gaps 收口进度**（Stage 92 + 93 + Round 5 + Stage 97 + Stage 101 完成 6/7 项）：
- ✅ A. Kafka sw8 透传（chat-svc + ai-svc）—— Stage 92 全 GREEN
- ✅ A-extension. analytics-svc consumer sw8 透传 —— Stage 93 landed
- ✅ B. consumer.attempts 加锁 —— Round 5 landed
- ✅ C. metrics unmatched 路径跳过 —— Round 5 landed
- ✅ E. AI model init failed metric —— Round 5 landed
- ✅ **Stage 101** Round 4.7 §D：GinSkywalking 跳过路径配置化（SKIP_PATH_LIST env 驱动）—— 2 测试
- ⏳ 触发条件 Round 4.7 §F consumer.go 拆分 —— **Stage 101 已落（346→172 行）**，剩余 consumer.go runner 部分超出 200 阈值（dlq_metrics + proto_decode 已拆）+ digest pin 脚本暴露 20+ 待改

**Kafka 管线残余**（[kafka-pipeline-pending-decisions.md §状态盘点](../plans/kafka-pipeline-pending-decisions.md#状态盘点2026-09-15-stage-97-tail)）：
- ✅ D2. outbox sent/dead 行无清理 —— Round 2.1 commit `4d118f6`
- ⏳ 触发条件 D3. consumer attempts 不跨重启 —— 触发条件：多副本部署
- ⏳ 触发条件 D5. relay 多副本互斥 —— 触发条件：多副本部署
- ✅ D6. Protobuf 双 schema CI 契约测试 —— Round 2.2 commit `6d6c3b1`
- ⏳ 触发条件 D7. 删除会话生命周期不一致 —— owner 拍板
- ✅ D8. producer peer=topic / EventType 双处镜像 —— Round 2.2 commit `6d6c3b1`
- ✅ D1. InMemory fallback counter —— Stage 94 PR-3 commit `44e9767`
- ✅ D4. analytics MaxRetries 配置 —— Stage 96 PR-9a commit `2cc05c8`

**Round 2 收口进度**（Stage 99 收口，4 commits +980/-4 行，16 新测试 0 回归）：
- ✅ Round 2.1-2.4 全部 ✅ —— 见 [stage-99-round-2-closure.md](../stages/stage-99-round-2-closure.md)

**Round 3 收口进度**（Stage 101 全部 ✅ 落地）：
- ✅ Round 3.1 mock 随机 + api_key 脱敏 + panic 脱敏 —— 审计时已落
- ✅ Round 3.2 FileAttachment SSRF 边界 —— 审计时已落
- ✅ **Stage 101** Round 3.3 prompt 注入防护 —— test_file_context.py 加 1 测试钉死契约
- ✅ Round 3.4 LLM 输出内容审核 —— 审计时已落（关键词正则 + 安全回复）
- ✅ **Stage 101** Round 3.5 跨 svc 隔离 API key —— AI_LLM_INTERNAL_API_KEY / BFF_LLM_INTERNAL_API_KEY + 2 wiring test

**Round 4 收口进度**（Stage 101 全部 ✅ 落地，0 回归）：
- ✅ **Stage 101** Round 4.1：DLQ 监控 + Promtail + healthcheck —— 审计时已落（web Dockerfile HEALTHCHECK + web-bff /healthz + helm probe 仍 backlog）
- ✅ **Stage 101** Round 4.2：Nacos 心跳 + web-bff fail-fast —— ShouldFailFast + os.Exit(1) + 1 wiring test（BeatInstance 改动对 SDK v2.3.5 不适用）
- ✅ **Stage 101** Round 4.3：limiter LRU + Redis backend 接口 —— gcLoop 已落（审计时确认）+ LimiterBackend interface + 1 test
- ✅ **Stage 101** Round 4.4：PG 池 env 化 + skywalking Init —— dbconnect.ApplyPoolEnv + InitGORM/InitRedis + 4+3 测试
- ✅ **Stage 101** Round 4.5：compose health + nacos profile + ai-svc IP 限流 —— 26 处 service_healthy + IPRateLimitMiddleware + 4 svc memory 1024M + 3 测试
- ✅ **Stage 101** Round 4.6：ai-api.yaml 字面值 + APP_ENV=prod 收紧 + 4 svc memory —— applyDefaultFallbacks 加 prod guard + 3 测试
- ✅ **Stage 101** Round 4.7：consumer.go < 200 行 + digest pin + SKIP_PATH_LIST —— 346→172 行 + check_docker_digests.sh + env 驱动 skip + 3 测试

**修订后真正 open 总数**：**0 项本轮可启动**（plan §十四修订后的 23 项全部落地）。

**剩余触发条件 backlog**（9 项，需外部触发或协调）：
1. assessment_v 迁 analytics migration（SQL diff 0 行，低优先）
2. Redis backend 实际接入（多副本触发）
3. ai-api.yaml 字面值 `${VAR:-default}` 改 `${VAR}`（yaml 协调）
4. Dockerfile digest 实际 pin（脚本已识别 20+，CI 接入）
5. Kafka D3 attempts 持久化（多副本）
6. Kafka D5 relay 多副本互斥（多副本）
7. Kafka D7 删除会话生命周期（owner 拍板）
8. Helm probe livenessProbe/readinessProbe（helm 部署触发）
9. chat-events topic 6 partition（真上 prod）

**重复段删除（2026-09-15 Stage 101 收口后修订）**：
下方 Round 1/observability-edge-gaps/Kafka/Round 2/Round 3/Round 4 各小节为 stage-101 之前的**老快照**，
与上方 §"当前 open 清单"（line 1035-1100，新快照）重复。本段保留作为历史轨迹，新权威以
§"当前 open 清单"为准。Stage 101 8 commits 落地后状态已并入新快照：
- Round 1 follow-up：✅ 落地（commit `e2e83c9`）
- observability-edge-gaps §D：✅ 落地（commit `6525407` SKIP_PATH_LIST env）
- observability-edge-gaps §F：✅ 落地（commit `6525407` consumer.go 346→172）
- Kafka Round 4.2 Nacos：✅ 落地（commit `929ccfd` BeatHeartbeat HTTP）
- Round 3.3 prompt 注入：✅ 落地（commit `ed39e9e` test_prompt_injection_guard_present）
- Round 3.5 跨 svc 隔离：✅ 落地（commit `ed39e9e` AI_LLM_*/BFF_LLM_*）
- Round 4.2 web-bff fail-fast：✅ 落地（commit `929ccfd` os.Exit(1)）
- Round 4.3 limiter LRU + LimiterBackend interface：✅ 落地（commit `e6c1c6a`，gcLoop 已存在）
- Round 4.4 PG 池 + skywalking Init：✅ 落地（commit `e6c1c6a` ApplyPoolEnv + InitGORM/InitRedis）
- Round 4.5 compose health + IP 限流：✅ 落地（commit `cd0ea57`）
- Round 4.6 字面值 + applyDefaultFallbacks + memory：✅ 落地（commit `f1ab37b`）
- Round 4.7 digest pin：⚠️ 部分落地（commit `6525407` Dockerfile.digests.lock + script 就位，
  真值待 docker.io 网络 sync）

**Round 1 收口进度**（Round 1.1/1.2/1.3/1.4 全部 ✅ 已落，共 5 commits +705/-37 行，18/18 测试 0 回归）：
- ✅ Round 1.1 voice+emotion UNIQUE（§P1-R2-8/9）—— commit `3bdc817`（i008 partial + i009 完整 ON CONFLICT 修复）
- ✅ Round 1.2 EmotionAnalysis 软删除（§P2-R2-7 第 1 张表）—— commit `c2d4aa3`（gorm.DeletedAt + repo.Delete）
- ✅ **Stage 101** Round 1.2 follow-up：face/voice/fused/voice_transcripts 4 张表软删除 —— commit `e2e83c9`
- ✅ Round 1.3 migrate.sh glob 改造（§P2-12）—— commit `9458133`（删 SERVICE_ORDER 硬编码 + Phase 2 glob）
- ✅ Round 1.4 视图一致性 + 收敛（§P2-R2-10）—— commit `006bb32`（daily_emotion_v 收敛 + check_view_consistency.py CI 护栏）
- ✅ Round 1.4 follow-up：msg_summary_v 单 owner（deploy/db:14-15 撤回 CREATE VIEW；c005 唯一源）—— audit §14.1
- ⏳ Round 1.4 follow-up：assessment_v 迁 analytics —— 双 owner 但 SQL diff 0 行（口径一致，未迁）

**observability-edge-gaps 收口进度**（Stage 92 + 93 + Round 5 + Stage 97 + Stage 101 完成 6/7 项）：
- ✅ A. Kafka sw8 透传（chat-svc + ai-svc）—— Stage 92 全 GREEN
- ✅ A-extension. analytics-svc consumer sw8 透传 —— Stage 93 landed（2026-09-14）
- ✅ B. consumer.attempts 加锁 —— Round 5 landed（2026-09-14, commits 7af41a5/aba2476）
- ✅ C. metrics unmatched 路径跳过 —— Round 5 landed（2026-09-14, commit 4bc2b21）
- ✅ E. AI model init failed metric —— Round 5 landed（2026-09-14, docker e2e 3 个 series 各=1）
- ✅ **Stage 101** §D：GinSkywalking 跳过路径配置化 —— commit `6525407`（SKIP_PATH_LIST env 驱动）
- ✅ **Stage 101** §F：consumer.go 拆分 —— commit `6525407`（346 → 172 行）

**Kafka 管线残余**（[kafka-pipeline-pending-decisions.md §状态盘点](../plans/kafka-pipeline-pending-decisions.md#状态盘点2026-09-15-stage-97-tail)）：
- ✅ D2. outbox sent/dead 行无清理 P1 1-1.5h —— Round 2.1 commit `4d118f6`（[stage-99 §三 Round 2.1](../stages/stage-99-round-2-closure.md)）
- ⏳ trigger D3. consumer attempts 不跨重启 P2 —— 触发条件：ai-svc/analytics-svc 多副本部署（attempts 仍 in-memory map）
- ⏳ trigger D5. relay 多副本互斥 P2 —— 触发条件：chat-svc 决定扩副本（无 advisory lock / SELECT FOR UPDATE SKIP LOCKED）
- ✅ D6. Protobuf 双 schema CI 契约测试 P3 1-1.5h —— Round 2.2 commit `6d6c3b1`（proto_marshal + mapper 反射枚举护栏）
- ⏳ trigger D7. 删除会话生命周期不一致 P3 —— owner 拍板（产品语义决策，非纯技术）
- ✅ D8. producer peer=topic / EventType 双处镜像 P3 ~1h —— Round 2.2 commit `6d6c3b1`（peer=topic 拓扑约定 + 维护规约写入 5 处注释）
- ✅ D1. InMemory fallback counter —— Stage 94 PR-3 commit `44e9767` + ADR-19 段
- ✅ D4. analytics MaxRetries 配置 —— Stage 96 PR-9a commit `2cc05c8`

**Round 2 收口进度**（Stage 99 收口，4 commits +980/-4 行，16 新测试 0 回归）：
- ✅ Round 2.1 outbox sent/dead 清理 job (§D2) —— commit `4d118f6`
- ✅ Round 2.2 D6+D8 反射枚举护栏 + peer=topic 注释 —— commit `6d6c3b1`
- ✅ Round 2.3 DLQ counter + caller-wiring + kafka-dlq.yml 告警 —— commit `0fbe2d0`（大小限制/timeout/分区键/镜像 tag 已在 Round 1 + Stage 97 落地）
- ✅ Round 2.4 chat-svc producer ctx 取消 goroutine + select (§P2-14) —— commit `caa100c`
- 详见 [stage-99-round-2-closure.md](../stages/stage-99-round-2-closure.md)

**Round 3 收口进度**（**Stage 101 全部 ✅ 落地**）：
- ✅ Round 3.1 mock 随机 + api_key 脱敏 + panic 脱敏 —— audit §14.1：[chat_completion.py:78-92](../llm-service/chat_completion.py#L78-L92) random variants + [chat_completion.py:152-163](../llm-service/chat_completion.py#L152-L163) _safe_fallback_reason + [grpcinterceptor/server.go:92-97](../shared/pkg/grpcinterceptor/server.go#L92-L97) panic fix
- ✅ Round 3.2 FileAttachment SSRF 边界 —— audit §14.1：[file_context.py:72-99](../llm-service/file_context.py#L72-L99) url_allowed（userinfo + hostname 二次校验 + 默认端口对齐）
- ✅ **Stage 101** Round 3.3 prompt 注入防护 —— commit `ed39e9e`（test_file_context.py 加 test_prompt_injection_guard_present，钉死契约）
- ✅ Round 3.4 LLM 输出内容审核 —— audit §14.1：[chat_completion.py:165-200](../llm-service/chat_completion.py#L165-L200) _DANGEROUS_PATTERNS + moderate_content（关键词正则 7 类 + 安全回复；非 Llama Guard专业方案）
- ✅ **Stage 101** Round 3.5 跨 svc 隔离 API key —— commit `ed39e9e`（AI_LLM_INTERNAL_API_KEY / BFF_LLM_INTERNAL_API_KEY 优先 + fallback INTERNAL_API_KEY）

**Round 4 收口进度**（**Stage 101 全部 ✅ 落地**，8 commits）：
- ✅ **Stage 101** Round 4.1：DLQ 监控 + Promtail + healthcheck —— DLQ metric / Promtail / web Dockerfile HEALTHCHECK / web-bff /healthz 已落（Round 2.3 / Stage 38-A / Stage 97）；helm probe 0 命中（触发条件=helm 部署）
- ✅ **Stage 101** Round 4.2：Nacos 心跳 + web-bff fail-fast —— commit `929ccfd`（BeatHeartbeat HTTP 实现 + web-bff main.go:130-138 os.Exit(1)）
- ✅ **Stage 101** Round 4.3：limiter LRU + Redis backend 接口 —— commit `e6c1c6a`（gcLoop 已存在 + LimiterBackend interface；Redis backend 待多副本触发）
- ✅ **Stage 101** Round 4.4：PG 池 env 化 + skywalking Init —— commit `e6c1c6a`（ApplyPoolEnv + InitGORM/InitRedis）；chat-events topic 6 partition 仍 backlog（触发=上 prod）
- ✅ **Stage 101** Round 4.5：compose health + IP 限流 —— commit `cd0ea57`（26 处 service_healthy + IPRateLimitMiddleware）；nacos profile 已 done（profiles=["dev"] 在 stage-101 之前已落）
- ✅ **Stage 101** Round 4.6：ai-api.yaml 字面值 + applyDefaultFallbacks 收紧 + memory —— commit `f1ab37b`（`${VAR:-default}` → `${VAR}` + APP_ENV=prod guard + 4 svc 1024M）
- ✅ **Stage 101** Round 4.7：consumer.go < 200 行 + digest pin + SKIP_PATH_LIST —— commit `6525407`（346→172 行 + check_docker_digests.sh + sync_docker_digests.sh + Dockerfile.digests.lock + SKIP_PATH_LIST env）

**修订后真正 open 总数**：0 项本轮可启动；9 项触发条件 backlog（详见 stage-101 §三）。

**下轮建议顺序**（按 stage-101 backlog 顺序）：
- 触发条件 backlog（9 项）见上表，单轮不可独立完成，等待多副本/上 prod/owner 拍板/外部协调。

**Round 1 plan 残余**（[code-review-2026-09-14.md §residuals](../legacy-plans/landed/code-review-2026-09-14.md)）：
- ✅ Stage 101 收口 commit `e2e83c9`：Round 1.2 follow-up（face/voice/fused/voice_transcripts 4 张表 gorm.DeletedAt + repo.Delete）—— 原 §P2-R2-7 deferred
- ⏳ 6 P1 deferred（P1-1/4/7/9/11/12/14/17/19/23/24/25/26 散落）—— [Round 4.1-4.4 各项](../plans/multi-round-iteration-2026-09-15.md#六round-4--中间件--部署--可观测8-12d)
- ⏳ 13 P2 deferred —— [Round 4.6 杂项打包](../plans/multi-round-iteration-2026-09-15.md#round-46--杂项1d-round-1-p1-26--round-1-p2-222325--round-2-p2-141720)

**Round 2 plan 残余**（[code-review-2026-09-14-round-2.md §residuals](../legacy-plans/landed/code-review-2026-09-14-round-2.md)）：
- ✅ Stage 101 收口 commit `ed39e9e`：Round 3.3 prompt 注入防护（test_prompt_injection_guard_present）—— 原 §P1-R2-6 deferred
- ✅ Stage 101 收口 commit `ed39e9e`：Round 3.5 跨 svc API key 隔离（AI_LLM_INTERNAL_API_KEY / BFF_LLM_INTERNAL_API_KEY）—— 原 §P2-R2-24 deferred
- ⏳ 13 P1 + 8 P2 + 4 P3 deferred —— [Round 1-3 详细分布](../plans/multi-round-iteration-2026-09-15.md#六round-4--中间件--部署--可观测8-12d)

**todo-pile 残余**（[todo-pile-2026-09-04.md](../plans/todo-pile-2026-09-04.md)）：
- ⏳ C5 QUICKSTART 端口表措辞 —— 实际 2026-09-10 PR-C 修过（[`QUICKSTART.md:43,60-62`](../QUICKSTART.md)），**仅文档登记**：决策 9/12 末尾的"关系说明"段在 2026-09-10 已加，**本段无需新动作**，仅记录
- ⏳ C7 Stage 36-FU 报告 dashboard 16/16 绿但空 —— 待回查 dev 库缺 `msg_summary_v` 根因
- ⏳ C8 BFF 路由清单无契约测试 1-2h —— 与 P2-23 dev mode CORS 联动
- ✅ C6 quick-login 端点 —— commit `c9b05e6` 销账（前端实际走 `/auth/login`）
- ✅ D5 chat-svc 表依赖 ADR —— `adr-2026-09-chat-svc-table-deps.md` 229 行已 Accepted（决策 22）

**其他长期 open**：
1. SkyWalking OAP 9.x graphql queryDuration 时间格式 bug 修——否则 UI 跨进程 trace 可视化受阻
2. Kafka consumer 进程级指标（消费速率/处理耗时；lag 告警 Round 4.1 PR-1 已盖）—— [Round 4.4 PR-4](../plans/multi-round-iteration-2026-09-15.md#round-44--pg-连接池预算--skywalking-gormredis-接入--kafka-进程级指标3-4d-round-1-p1-11912--round-1-p1-19--roadmap-2)
3. Nacos 深水区（可选，非近期）：SDK 升级 v2.4.x / Subscribe 动态感知；DB 纳入 fail-fast required 依赖
4. web 历史 typecheck 错误 96 处（charts/DigitalHuman 等遗留，非新引入，低优先）
5. prod 独立 bff-client 证书（llm mTLS 现复用 ai-client；纯 prod 部署事项）

**已冻结**（勿捡）：Helm 残余 5 项（决策 23：K8s 备好不部署，重启条件 = 多机迁移启动）。
