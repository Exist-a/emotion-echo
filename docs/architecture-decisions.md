# Emotion-Echo · 架构决策记录（ADR）

> **本文档是 Emotion-Echo 微服务架构的"单一事实源"（Single Source of Truth）。**
> 所有 stage 文档、路线图、代码组织、配置都应与本文档一致。
> 决策变更时，**先改本文档，再改代码**，保持文档先行。
> 最后更新：2026-07-14

---

## 🎯 项目定位

**目标**：构建一个完整的、生产级 Go 微服务架构（含跨语言 Python LLM），
服务于 Emotion-Echo 情绪分析产品。

**当前阶段**：dev / 本地 docker-compose，未上生产。

**演进路线**：本地 Docker → K8s manifests 准备 → 未来上生产。

---

## ✅ 已敲定的决策（不可变更）

### 决策 1：HTTP 框架 = **Gin**（不再用 go-zero）

| 维度 | 选择 |
|------|------|
| Go svc HTTP 框架 | **Gin** (`github.com/gin-gonic/gin`) |
| 原因 | 复用 legacy `emotion-echo-gin` 14 个 handler，团队零学习成本 |

**❌ 废弃**：go-zero 框架、goctl 代码生成器、tRPC 协议。

**理由**（决策记录）：
- 实际只用到 go-zero 的 30%（HTTP server + goctl 模板）
- go-zero 的核心特性（zrpc/breaker/limit/registry）我们都没用
- legacy 已经有完整的 Gin 业务代码，搬迁成本远低于重写

### 决策 2：服务发现 = **APISIX + etcd**（删除 Nacos）

| 维度 | 选择 |
|------|------|
| 服务注册 | **APISIX** 直管 etcd 配置 |
| 配置存储 | etcd（APISIX 原生后端） |
| svc 主动注册 | **不需要**（svc 直接监听端口，APISIX 用固定 upstream） |

**❌ 废弃**：Nacos 作为注册中心。

**理由**：
- Nacos 注册了但没人读（APISIX 不读 Nacos）
- Nacos 挂了导致所有 svc 起不来（强耦合故障源）
- APISIX + etcd 已经把"路由 + 配置存储"包圆了
- 简化架构：少一个组件 = 少一个故障点

### 决策 3：分布式部署 = **本地 Docker + K8s manifests 准备**

| 维度 | 选择 |
|------|------|
| 当前部署 | docker-compose（dev） |
| 未来部署 | K8s manifests（写好但不部署，等代码稳定后再上） |
| etcd 形态 | dev: 单节点；K8s: 集群（Raft 3 节点）|

**❌ 禁止**：在代码未完成时上 K8s。

### 决策 4：跨服务调用 = **gRPC + .proto**（未来）

| 维度 | 选择 |
|------|------|
| 外部 API（浏览器→svc） | HTTP REST + JSON + APISIX |
| 内部 svc-to-svc | **gRPC + .proto**（待实施） |
| 异步事件 | Kafka + JSON（已有 chat-events） |

**当前阶段**：内部调用只有 ai-svc → emotion-llm-service，使用 HTTP（待升级 gRPC）。

### 决策 5：Python LLM 服务 = **独立微服务 + gRPC server**

| 维度 | 选择 |
|------|------|
| 部署形态 | 独立进程/容器 `emotion-llm-service` |
| 框架 | FastAPI（HTTP 阶段）/ gRPC server（未来升级）|
| 协议 | 当前 HTTP → 升级为 gRPC（.proto 单一事实源）|

**理由**：
- 与 Go svc 解耦，可独立扩缩容
- proto 文件作为跨语言 API 契约
- Python LLM 可换实现（FastAPI → 直接 model serving）

### 决策 6：审计 = **白盒化 + JSON 日志 + trace_id 串联**

| 维度 | 选择 |
|------|------|
| 日志格式 | JSON（结构化） |
| 必含字段 | `ts`, `level`, `svc`, `trace_id`, `user_id`, `action` |
| API 访问日志 | APISIX access log（边缘） |
| 用户操作审计 | 各 svc 业务 logger |
| 集中存储（dev） | 各 svc `out.log` 文件 |
| 集中存储（prod） | Loki + Grafana 或 ELK |

### 决策 7：鉴权 = **APISIX jwt-auth**（替换 svc mock 鉴权）

| 维度 | 选择 |
|------|------|
| 鉴权位置 | **APISIX 网关** |
| svc 端 | **信任 APISIX 注入的 X-User-Id header** |
| 鉴权算法 | JWT（APISIX jwt-auth 插件） |

**❌ 废弃**：svc 内部 mock `X-User-Id` 鉴权（不安全）。

### 决策 8：限流 + 熔断 = **APISIX 插件**

| 维度 | 选择 |
|------|------|
| 限流 | APISIX `limit-count`（按 user_id） |
| 熔断 | APISIX `api-breaker`（保护下游 svc） |
| CORS | APISIX `cors`（统一一次配） |

---

## 🏗 当前架构全景

```
                          浏览器 / 客户端
                                │
                                ▼ HTTP
                    ┌─────────────────────────┐
                    │  APISIX 网关 :9080       │  ← 审计/鉴权/限流/熔断/CORS
                    │   9 个路由               │     全在网关层做
                    └────────────┬────────────┘
                                 │ 按 URL 路由（etcd 存配置）
        ┌────────────┬───────────┼───────────┬────────────┬────────────┐
        ▼            ▼           ▼           ▼            ▼            ▼
   user-svc    assessment-svc  chat-svc   ai-svc    analytics-svc   llm-svc
   :8888          :8889         :8890     :8891       :8892        :8000
   (Gin)          (Gin)         (Gin)    (Gin)       (Gin)        (Python FastAPI)
       │             │             │   │       │             │
       │             │             │   │       │             │
       │             │             │   ▼       │             │
       │             │             │  Kafka    │             │
       │             │             │  (chat-   │             │
       │             │             │  events)  │             │
       │             │             │   │       │             │
       │             │             │   └───────┤             │
       │             │             │           ▼             │
       │             │             │    emotion-llm- ◄───────┘
       │             │             │    service (gRPC 未来)
       │             │             │
       ▼             ▼             ▼           ▼             ▼
   emotion_      emotion_      emotion_   emotion_       emotion_
   echo_user     echo_assess   echo_chat  echo_ai        echo_analyt

   ┌────────────────────────────────────────────────────────────┐
   │  基础设施层                                                  │
   │  Postgres (5 schema) + Kafka + SkyWalking + etcd (APISIX)   │
   │  ❌ 删 Nacos（决策 2）                                      │
   └────────────────────────────────────────────────────────────┘
```

---

## 📋 服务清单（权威）

| svc | 端口 | 框架 | DB schema | 业务职责 | 状态 |
|-----|------|------|-----------|---------|------|
| **user-svc** | 8888 | Gin | emotion_echo_user | 用户/Auth/上传 | ✅ Stage 1 完成 |
| **assessment-svc** | 8889 | Gin | emotion_echo_assessment | 量表/评估/报告 | ✅ Stage 1 完成 |
| **chat-svc** | 8890 | Gin | emotion_echo_chat | 会话/消息 | ✅ Stage 1 完成 |
| **ai-svc** | 8891 | Gin | emotion_echo_ai | 情绪分析 | ✅ Stage 1 完成 |
| **analytics-svc** | 8892 | Gin | emotion_echo_analytics | 行为事件 | ✅ Stage 1 完成 |
| **emotion-llm-service** | 8000 | FastAPI | — | Python LLM | ✅ Stage 3 完成 |

---

## 📁 项目结构（权威）

```
Emotion-Echo/
├── AGENTS.md                                ← TDD 强约束
├── docs/
│   ├── architecture-decisions.md            ← 🆕 本文档（单一事实源）
│   ├── microservices-architecture.md        ← 当前架构总览
│   ├── distributed-roadmap.md               ← 5-Phase 路线图
│   ├── distributed-architecture.md          ← 选型说明
│   ├── microservice-decomposition-plan.md   ← 拆分规划
│   ├── stage-0-learnings.md                 ← Stage 0 复盘
│   ├── stage-1-completion.md                ← 阶段报告
│   ├── stage-2-async-pipeline.md
│   ├── stage-3-llm-integration.md
│   └── stage-4-emotion-query.md
├── deploy/
│   ├── docker-compose.infra.yml             ← 容器编排（不含 Nacos）
│   ├── apisix/                              ← 网关配置
│   └── db/                                  ← schema 脚本
├── emotion-echo-shared/                     ← 共享代码（Gin middleware 等）
│   └── pkg/
│       ├── skywalking/                      ← tracer + gorm + redis hooks
│       ├── messaging/                       ← Kafka Producer/Consumer
│       └── middleware/                      ← Gin middleware (auth/cors/recover)
├── emotion-echo-user-svc/                   ← 5 svc 各自独立（Gin）
├── emotion-echo-assessment-svc/
├── emotion-echo-chat-svc/
├── emotion-echo-ai-svc/
├── emotion-echo-analytics-svc/
├── emotion-llm-service/                     ← Python FastAPI
├── legacy/emotion-echo-gin/                 ← 旧单体（Gin，业务参考 + handler 来源）
└── emotion-echo-front/                      ← Nuxt 前端
```

---

## 🔧 各 svc 的标准目录（Gin 风格）

```
emotion-echo-{domain}-svc/
├── cmd/main.go                              ← main 入口
├── go.mod                                   ← replace → shared
├── etc/{domain}-api.yaml                    ← 配置
├── {domain}-svc.exe                         ← 编译产物
├── internal/
│   ├── config/                              ← yaml struct
│   ├── handler/                             ← Gin HandlerFunc（从 legacy 搬）
│   ├── logic/                               ← 业务实现（手写 TDD）
│   ├── model/                               ← GORM 模型
│   ├── repository/                          ← Repo interface + InMemory + Postgres
│   ├── svc/servicecontext.go                ← 依赖注入容器
│   └── middleware/                          ← svc 专属中间件（如有）
└── tests/                                   ← 集成测试（可选）
```

---

## 📡 协议分层（权威）

| 流量类型 | 协议 | 序列化 | 入口 | 状态 |
|---------|------|--------|------|------|
| **外部 API**（浏览器→svc）| HTTP REST | JSON | APISIX | ✅ 已通 |
| **内部 RPC**（svc↔svc）| gRPC | Protobuf | 直接连 | ⏳ 待实施 |
| **异步事件**（svc→svc）| Kafka | JSON | Kafka broker | ✅ chat-events |

**当前阶段细节**：
- ai-svc → emotion-llm-service：HTTP（待升级 gRPC）
- 其他 svc 之间：无直接调用

---

## 🎯 TDD 原则（不变）

1. **RED 先行**：先写测试，看到编译错误 / 测试失败
2. **GREEN 实现**：最小代码让测试通过
3. **测试是文档**：每个测试描述一个业务规则

测试统计目标：≥ 50 PASS（当前 70+ PASS，已超）。

---

## 🚦 启动 / 验证命令（权威）

```bash
# 1. 启动基础设施（不含 Nacos）
cd deploy && docker compose -f docker-compose.infra.yml up -d

# 2. 启动 5 个 Go svc（各自目录）
cd emotion-echo-user-svc && ./user-svc.exe &
cd emotion-echo-assessment-svc && ./assessment-svc.exe &
cd emotion-echo-chat-svc && ./chat-svc.exe &
cd emotion-echo-ai-svc && ./ai-svc.exe &
cd emotion-echo-analytics-svc && ./analytics-svc.exe &

# 3. 启动 Python LLM
cd emotion-llm-service && python main.py &

# 4. 验证（通过 APISIX 网关）
curl http://localhost:9080/user-health
curl http://localhost:9080/assessment-health
curl http://localhost:9080/chat-health
curl http://localhost:9080/ai-health
curl http://localhost:9080/analytics-health

# 5. 看 trace
open http://localhost:18080
```

---

## 📊 当前进度

```
Phase 0 基础设施    ████████████████████ 100% ✅
Phase 1 微服务拆分   ████████████████████ 100% ✅ (5/5 svc 上线 + 接 DB)
Phase 2 Kafka       ████████████████████ 100% ✅ (异步管道跑通)
Phase 3 LLM 接入    ████████████████████ 100% ✅ (跨语言情绪分析)
Phase 4 业务深化     █████████████░░░░░  75%  (emotion 查询完成)
Phase 5 韧性+网关鉴权 ██████░░░░░░░░░░░░  30%  (jwt-auth/limit/breaker 待加)
Phase 6 K8s         ░░░░░░░░░░░░░░░░░░░░   0%  (manifests 待写)
Phase 7 gRPC 升级    ░░░░░░░░░░░░░░░░░░░░   0%  (proto 待定义)
```

---

## 🔮 下一步（按优先级）

1. **删除 Nacos 代码 + docker-compose**（决策 2 落地）
2. **APISIX P0/P1 插件**：jwt-auth + limit-count + api-breaker
3. **svc 框架迁移**：go-zero → Gin（每个 svc 改 main.go + handler）
4. **从 legacy 搬业务 handler**：14 个 handler 按域分配
5. **proto 文件起草**：emotion-llm Analyze 接口
6. **gRPC 升级**：ai-svc → emotion-llm-service 从 HTTP 升级
7. **K8s manifests**：每个 svc 一个 deployment + service

---

## 📝 决策变更记录

| 日期 | 决策 | 旧→新 | 原因 |
|------|------|-------|------|
| 2026-07-14 | HTTP 框架 | go-zero → **Gin** | 复用 legacy，团队熟悉 |
| 2026-07-14 | 服务发现 | Nacos → **APISIX+etcd** | Nacos 假集成，没人读 |
| 2026-07-14 | 跨服务协议 | （未定）→ **gRPC+proto** | 跨语言契约标准 |
| 2026-07-14 | 鉴权位置 | svc mock → **APISIX jwt-auth** | 安全 |
| 2026-07-14 | 限流熔断位置 | （无）→ **APISIX 插件** | 网关层公共关注点 |
| 2026-07-14 | 部署形态 | docker-compose → **K8s manifests** | 生产演进 |

---

**所有文档（stage-X、roadmap、decomposition-plan）的具体实施细节以本文档为最终裁决。**