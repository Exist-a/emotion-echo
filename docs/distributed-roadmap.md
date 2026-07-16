# Emotion-Echo 分布式改造 · 落地路线图（执行版）

> ⚠️ **架构最终决策以 [architecture-decisions.md](./architecture-decisions.md)（ADR）为单一事实源**。
> 本文档保留为**历史路线记录**，描述当时的实施步骤与决策。
> **当前架构变更**（2026-07-14）：
> - go-zero → Gin（ADR 决策 1）
> - Nacos → 删除（ADR 决策 2）
> - 跨服务调用 → gRPC（ADR 决策 4）
>
> 本文档基于 [distributed-architecture.md](./distributed-architecture.md) 的选型，
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
K8s（最终部署形态）
```

### 0.2 各 Phase 拆解

| Phase | 目标 | Stage 数 | 关键收益 |
|-------|------|---------|---------|
| Phase 0 | 一次性起齐 4 个中间件 + Gin 上 trace | 5 | 看到 SkyWalking 上的第一条 span |
| Phase 1 | go-zero 改造起头 | 5 | 走通 "浏览器 → APISIX → go-zero" |
| Phase 2 | Kafka 异步化 | 4 | 端到端语音链路跑通 |
| Phase 3 | 限流/熔断/配置中心 | 3 | 韧性 + 动态配置 |
| Phase 4 | 业务域拆分 | 3 | 旧 Gin 工程清零 |
| Phase 5 | K8s 化 | 4 | `helm install` 一键部署 |

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
- `Emotion-Echo-Web/app/composables/useSSE.ts`

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

### 目标
把限流规则推送到 Nacos，运行时热更新。

### 涉及文件
- `user-svc/etc/nacos.go`（引入 Nacos Config）
- `user-svc/internal/svc/config.go`

### 学习收获
- **配置即代码** vs **配置即推送** 的差异
- 配置版本回滚（生产救火必备）

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

Phase 5 K8s 化
  [ ] 5.1 Helm Chart
  [ ] 5.2 Strimzi Kafka
  [ ] 5.3 APISIX Ingress
  [ ] 5.4 Prometheus + Grafana
```

---

# 下一步行动

按 `Phase 0 / Stage 0.1` 开始：建 `deploy/` 目录、写最简 docker-compose，跑通 Hello World 即成功。

随时告诉我哪个 Stage 已经完成，或卡在哪个地方 —— 我会带你逐个 Stage 推进。
