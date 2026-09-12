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

**当前 open 清单**（2026-09-12 Stage 81 收口后刷新）：

1. **llm-chat-real-pipeline PR-3**（意图分类重写版 + 按意图结构化回复——挂载点已就绪：
   llm-service ChatCompletion 前后插分类/风格指令；解锁 ai-response-structured 阶段 2
   与 intent-classification-6-types）
2. llm-service Python 端 Nacos 注册（BFF 目前 env 直连；注册后切 Nacos 优先模式）
3. Kafka P3：outbox relay dead 告警接 alertmanager（kafka-reliability-gaps.md §3.6，小）/
   §1.4 可选：consumer 进程级指标
4. Nacos 深水区（可选，非近期）：SDK 升级 v2.4.x / Subscribe 动态感知 /
   llm-service Python 端注册；DB 纳入 fail-fast required 依赖（可选强化，stage-77 §四）
5. web 历史 typecheck 错误 96 处（charts/DigitalHuman 等遗留，非新引入，低优先）（nacos-enablement-dev.md §一.1）

**已冻结**（勿捡）：Helm 残余 5 项（决策 23：K8s 备好不部署，重启条件 = 多机迁移启动）。
