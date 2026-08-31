# Emotion-Echo · 微服务架构文档（当前总览）

> ⚠️ **架构最终决策请看 [architecture-decisions.md](./architecture-decisions.md)（ADR）**。
> 本文档是 ADR 之下的"实施总览"，描述当前运行状态与目标架构。
> 最后更新：2026-09-03（同步 Stage 31/32/33 演进路线）
>
> **Stage 31 实施状态**（2026-09）：🚧 进行中
> - ✅ PR-01：文档收口（本版本）
> - ☐ PR-02..06：shared 库（discovery / configcenter）
> - ☐ PR-07..10：svc 接入（user/chat/assessment/analytics/ai/web-bff/llm-service）
> - ☐ PR-11..12：compose + Helm subchart

## 🌐 系统全景

### 目标架构（Stage 31/32/33 落地后）

```
                          浏览器 / 客户端
                                │
                                ▼ HTTPS (prod: nginx:alpine 前置终结 TLS)
                   ┌──────────────────────────────┐
                   │  APISIX 3.18.0 网关层 :9080    │  ← 决策 11
                   │  - jwt-auth (真正验签)         │
                   │  - limit-count / limit-req     │
                   │  - api-breaker                │
                   │  - cors / prometheus          │
                   │  - 路由配置存 etcd v3.5       │
                   └──────────┬───────────────────┘
                              │ X-User-Id 注入
                              ▼
                   ┌──────────────────────────────┐
                   │  web-bff 纯聚合层 :8894       │  ← 决策 12
                   │  - 多服务聚合 / 字段裁剪        │
                   │  - SSE 流式编排 / 多端适配      │
                   │  ❌ 不再做鉴权/CORS/限流/熔断   │
                   └──────────┬───────────────────┘
                              │
       ┌──────────┬───────────┼───────────┬──────────┬──────────┐
       ▼          ▼           ▼           ▼          ▼          ▼
   user-svc  assessment    chat-svc    ai-svc  analytics  emotion-llm
   :8888     :8889         :8890       :8891   :8893      :8000+gRPC
   (Gin)     (Gin)         (Gin)      (Gin)   (Gin)     :50051
       │          │             │   │       │              │
       │          │             │   ▼       │              │
       │          │             │  Kafka    │              │
       │          │             │   │       │              │
       │          │             │   └───────┤              │
       │          │             │           ▼              │
       │          │             │      llm-service ◄───────┘
       │          │             │
       └──────────┴─────────────┴────────────────────────────
                              │
                   ┌──────────▼───────────────────┐
                   │  Nacos 2.4.x 注册+配置       │  ← 决策 10
                   │  - 6 Go svc + 1 Python svc  │
                   │    主动注册 + 5s 心跳        │
                   │  - 配置热更新（30s 拉取）      │
                   │  - namespace: emotion-echo- │
                   │    {dev, prod}              │
                   └──────────────────────────────┘
                              │
   ┌──────────────────────────▼───────────────────────────┐
   │  基础设施                                              │
   │  Postgres (5 schema) │ Kafka │ SkyWalking │ Redis (闲置) │
   │  ❌ Nacos 删过（stage-5）→ ✅ 演进回归（Stage 31）      │
   │  ❌ APISIX 删过（stage-30）→ ✅ 演进回归（Stage 32）     │
   └────────────────────────────────────────────────────────┘
```

**架构关键决策**（详见 ADR）：
- **HTTP 框架**：Gin（非 go-zero）
- **服务发现 / 配置中心**：**Nacos 2.4.x**（Stage 31，演进回归）
- **API 网关**：**APISIX 3.18.0** + etcd v3.5（Stage 32，演进回归）
- **BFF 定位**：纯聚合层（决策 12，不再兼任网关）
- **跨服务调用**：HTTP（dev） + gRPC（ai-svc ↔ llm-service）
- **异步事件**：Kafka `chat-events`（Stage 33 恢复写库路径）
- **鉴权 / 限流 / 熔断**：全部在 APISIX 网关层（BFF 不再承担）

### 当前过渡形态（2026-09 现状 → Stage 31/32/33 演进中）

| 层级 | 当前 | 目标 |
|------|------|------|
| API 网关 | ❌ BFF 兼任（产生 P0） | ✅ APISIX 独立层 |
| 服务发现 | ⚠️ 静态 DNS（compose 容器名） | ✅ Nacos 注册中心 |
| 配置中心 | ⚠️ yaml + env | ✅ Nacos 配置中心 |
| BFF | ⚠️ 网关 + 聚合（混合职责） | ✅ 纯聚合层 |
| 鉴权 | ❌ JWT 不验签 | ✅ APISIX jwt-auth |
| 限流/熔断 | ❌ 未实现 | ✅ APISIX 插件链 |

---

## 🧩 服务清单

| svc | 端口 | 框架 | DB schema | 业务职责 | 状态 |
|-----|------|------|-----------|---------|------|
| **web-bff** | 8894 | Gin | — | 纯聚合层（Stage 33 净化） | ⚠️ 网关职责迁出中 |
| **user-svc** | 8888 | Gin | emotion_echo_user | 用户/Auth/上传 | ✅ |
| **assessment-svc** | 8889 | Gin | emotion_echo_assessment | 量表/评估/报告 | ✅ |
| **chat-svc** | 8890 | Gin | emotion_echo_chat | 会话/消息 + outbox | ⚠️ 写库路径待 R-2 修复 |
| **ai-svc** | 8891 / gRPC 8892 | Gin | emotion_echo_ai | 情绪分析编排 | ✅ |
| **analytics-svc** | 8893 | Gin | emotion_echo_analytics | 行为事件/报表 | ⚠️ DLQ 待接通 |
| **emotion-llm-service** | 8000 / gRPC 50051 | FastAPI | — | 文本情绪分析（关键词器） | ✅ |
| **Emotion-Echo-Web** | 3000 | Nuxt 3 | — | 前端 SPA | ⚠️ SSE 解析待 R-1 修复 |
| **APISIX** | 9080 / 9180 | OpenResty | etcd | API 网关层 | ☐ Stage 32 启动 |
| **Nacos** | 8848 / 9848/9849 | Java | derby | 注册+配置 | 🚧 Stage 31 启动（PR-01 文档收口；PR-02..12 推进） |

---

## 📁 项目结构

```
Emotion-Echo/
├── AGENTS.md                                ← TDD 强约束
├── docs/
│   ├── architecture-decisions.md            ← 🆕 单一事实源（ADR）
│   ├── microservices-architecture.md        ← 本文档（实施总览）
│   ├── distributed-roadmap.md               ← 5-Phase 路线图
│   ├── distributed-architecture.md          ← 选型说明
│   ├── microservice-decomposition-plan.md   ← 拆分规划
│   ├── stage-0-learnings.md                 ← 阶段报告
│   ├── stage-1-completion.md
│   ├── stage-2-async-pipeline.md
│   ├── stage-3-llm-integration.md
│   └── stage-4-emotion-query.md
├── deploy/
│   ├── docker-compose.infra.yml             ← 容器编排
│   ├── apisix/                              ← 网关配置
│   └── db/                                  ← schema 脚本
│       ├── README.md
│       ├── 01-create-schemas.sql
│       └── 02-create-tables-in-schemas.sql
├── emotion-echo-shared/                     ← 共享代码
│   ├── pkg/skywalking/                      ← tracer + gorm + redis hooks
│   ├── pkg/messaging/                       ← Kafka Producer/Consumer（TDD: 5+2）
│   └── pkg/middleware/                      ← Gin middleware（auth/cors/recover）
├── emotion-echo-user-svc/                   ← 5 Go svc 各自独立
├── emotion-echo-assessment-svc/
├── emotion-echo-chat-svc/
├── emotion-echo-ai-svc/
├── emotion-echo-analytics-svc/
├── emotion-llm-service/                     ← Python FastAPI
├── legacy/emotion-echo-gin/                 ← 旧单体（业务参考 + handler 来源）
└── emotion-echo-front/                      ← Nuxt 前端
```

---

## 🔧 各 svc 的标准目录

```
emotion-echo-{domain}-svc/
├── cmd/main.go                              ← main 入口（Gin）
├── go.mod                                   ← replace → shared
├── etc/{domain}-api.yaml                    ← 配置（无 Nacos 段）
├── {domain}-svc.exe                         ← 编译产物
└── internal/
    ├── config/                              ← yaml struct
    ├── handler/                             ← Gin HandlerFunc
    ├── logic/                               ← 业务实现（手写 TDD）
    ├── model/                               ← GORM 模型
    ├── repository/                          ← Repo interface + InMemory + Postgres
    ├── svc/servicecontext.go                ← 依赖注入容器
    └── middleware/                          ← svc 专属中间件（如有）
```

---

## 🔑 各 svc 的关键设计

1. **HTTP 框架**：Gin（`github.com/gin-gonic/gin`）
2. **服务发现**：**Nacos 主动注册**（Stage 31 PR-02..09），5s 心跳，namespace=`emotion-echo-dev/prod`
3. **SkyWalking tracer**：每个 svc 用 go2sky + gRPC Reporter → 自动上报
4. **GORM + schema-qualified 名**：`TableName()` 返回 `emotion_echo_xxx.tbl_name`
5. **Repository 模式**：interface + InMemory（测试替身）+ Postgres（生产）
6. **鉴权**：**信任 APISIX 注入的 X-User-Id**（Stage 32 落地后；当前 JWT 不验签是审计 P0 S-1）
7. **配置**：yaml 文件 + **Nacos 配置中心运营参数**（Stage 31 PR-04/05），仅放 feature flag / 限流阈值 / 模型路由表
8. **健康检查**：每个 svc 暴露 `/health`，返回 dbOk / kafkaOk

---

## 📡 协议分层

| 流量 | 协议 | 序列化 | 入口 | 状态 |
|------|------|--------|------|------|
| **外部 API**（浏览器→svc）| HTTP REST | JSON | APISIX | ✅ 已通 |
| **内部 RPC**（svc↔svc）| HTTP（当前）/ gRPC（未来）| JSON / Protobuf | 直接连 | 🔄 过渡期 |
| **异步事件**（svc→svc）| Kafka | JSON | Kafka broker | ✅ chat-events 已通 |

---

## 🎯 TDD 全景

| 包 | 测试数 | 类型 | 状态 |
|----|------|------|------|
| `emotion-echo-shared/pkg/messaging` | 5 + 2 集成 | Unit + Integration | ✅ |
| `emotion-echo-shared/pkg/skywalking` | 手动验证 | Integration | ✅ |
| `emotion-echo-user-svc/internal/repository` | 5 | Unit | ✅ |
| `emotion-echo-user-svc/internal/logic` | 3 | Unit | ✅ |
| `emotion-echo-assessment-svc/repository` | 5 | Unit | ✅ |
| `emotion-echo-assessment-svc/logic` | 1 | Unit | ✅ |
| `emotion-echo-chat-svc/repository` | 5 | Unit | ✅ |
| `emotion-echo-chat-svc/logic` | 8 | Unit | ✅ |
| `emotion-echo-ai-svc/repository` | 3 | Unit | ✅ |
| `emotion-echo-ai-svc/analyzer` | 4 + 5 | Unit | ✅ |
| `emotion-echo-ai-svc/logic` | 5 + 4 | Unit | ✅ |
| `emotion-echo-analytics-svc/repository` | 3 | Unit | ✅ |
| `emotion-echo-analytics-svc/logic` | 1 | Unit | ✅ |
| **总计** | **70+ 测试 + 2 集成** | | ✅ |

---

## 🚦 启动 / 验证命令

```bash
# 1. 启动基础设施（含 Nacos 2.4.x，Stage 31 PR-11 落地）
cd deploy && docker compose -f docker-compose.infra.yml up -d

# 2. 启动 6 个 Go svc（各自目录；启动后自动注册到 Nacos）
cd emotion-echo-user-svc && ./user-svc.exe &
cd emotion-echo-assessment-svc && ./assessment-svc.exe &
cd emotion-echo-chat-svc && ./chat-svc.exe &
cd emotion-echo-ai-svc && ./ai-svc.exe &
cd emotion-echo-analytics-svc && ./analytics-svc.exe &
cd emotion-echo-web-bff && ./web-bff.exe &

# 3. 启动 Python LLM（启动后自动注册到 Nacos）
cd emotion-llm-service && python main.py &

# 4. 验证（通过 web-bff；APISIX Stage 32 落地后改 :9080）
curl http://localhost:8894/health          # BFF 聚合下游健康探测
curl http://localhost:8888/health          # user-svc
curl http://localhost:8889/health          # assessment-svc
curl http://localhost:8890/health          # chat-svc
curl http://localhost:8891/health          # ai-svc
curl http://localhost:8893/health          # analytics-svc

# 5. 验证 Nacos 注册中心（Stage 31 验收）
open http://localhost:8848/nacos           # 默认 nacos/nacos
# 服务管理 → 服务列表 → 7 个 service-name（user/chat/assessment/analytics/ai/web-bff/emotion-llm-service）且 health=true
./scripts/list_nacos_instances.sh

# 6. 看 trace
open http://localhost:18080
```

---

## 📊 当前进度

```
Phase 0 基础设施       ████████████████████ 100% ✅
Phase 1 微服务拆分      ████████████████████ 100% ✅
Phase 2 Kafka          ████████████████████ 100% ✅
Phase 3 LLM 接入       ████████████████████ 100% ✅
Phase 4 业务深化        █████████████░░░░░  75%
Phase 5 韧性+网关鉴权   ██████░░░░░░░░░░░░  30%  ← 待加 jwt-auth/limit/breaker
Phase 6 K8s manifests  ░░░░░░░░░░░░░░░░░░░░   0%
Phase 7 gRPC 升级      ░░░░░░░░░░░░░░░░░░░░   0%
```

---

## 🔮 下一步（按优先级）

1. **删除 Nacos**（代码 + docker-compose）
2. **svc 框架迁移**（go-zero → Gin，按 ADR）
3. **APISIX P0/P1 插件**：jwt-auth + limit-count + api-breaker
4. **从 legacy 搬业务 handler**（14 个 handler 按域分配）
5. **proto 文件起草**（emotion-llm Analyze）
6. **gRPC 升级**（ai-svc → emotion-llm-service）
7. **K8s manifests**（每个 svc 一个 deployment + service）

详见 [architecture-decisions.md](./architecture-decisions.md)。