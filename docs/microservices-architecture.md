# Emotion-Echo · 微服务架构文档（当前总览）

> ⚠️ **架构最终决策请看 [architecture-decisions.md](./architecture-decisions.md)（ADR）**。
> 本文档是 ADR 之下的"实施总览"，描述当前运行状态。
> 最后更新：2026-07-14

## 🌐 系统全景

```
                       浏览器 / 客户端
                             │
                             ▼ HTTP
                 ┌─────────────────────────┐
                 │  APISIX 网关 :9080        │  ← 鉴权/限流/熔断/CORS
                 │   9 个路由                │     全在网关层做
                 └─────────────┬────────────┘
                              │ 路由（etcd 存储）
       ┌──────────┬───────────┼───────────┬──────────┬──────────┐
       ▼          ▼           ▼           ▼          ▼          ▼
   user-svc  assessment    chat-svc    ai-svc   analytics   llm-svc
   :8888     :8889         :8890       :8891    :8892       :8000
   (Gin)     (Gin)         (Gin)      (Gin)    (Gin)       (Python)
       │          │             │   │       │              │
       │          │             │   ▼       │              │
       │          │             │  Kafka    │              │
       │          │             │   │       │              │
       │          │             │   └───────┤              │
       │          │             │           ▼              │
       │          │             │    emotion-llm-service  │
       │          │             │    (Python HTTP 调用)   │
       │          │             │           ▲              │
       │          │             │           └──────────────┘
       ▼          ▼             ▼           ▼              ▼
   emotion_    emotion_      emotion_   emotion_        —
   echo_user   echo_assess   echo_chat  echo_ai
                                   ┌── emotion_echo_analytics
```

**架构关键决策**（详见 ADR）：
- **HTTP 框架**：Gin（非 go-zero）
- **服务发现**：APISIX 直管 etcd（**不**用 Nacos）
- **跨服务调用**：HTTP（dev）→ gRPC（未来）
- **异步事件**：Kafka `chat-events`
- **鉴权 / 限流 / 熔断**：全部在 APISIX 网关层

---

## 🧩 服务清单

| svc | 端口 | 框架 | DB schema | 业务职责 | TDD 测试 | 状态 |
|-----|------|------|-----------|---------|---------|------|
| **user-svc** | 8888 | Gin | emotion_echo_user | 用户/Auth/上传 | 8 PASS | ✅ |
| **assessment-svc** | 8889 | Gin | emotion_echo_assessment | 量表/评估/报告 | 6 PASS | ✅ |
| **chat-svc** | 8890 | Gin | emotion_echo_chat | 会话/消息 | 13 PASS | ✅ |
| **ai-svc** | 8891 | Gin | emotion_echo_ai | 情绪分析/语音/人脸 | 16 PASS | ✅ |
| **analytics-svc** | 8892 | Gin | emotion_echo_analytics | 行为事件 | 4 PASS | ✅ |
| **emotion-llm-service** | 8000 | FastAPI | — | Python LLM 分析 | 手动 e2e | ✅ |
| **总计** | — | — | 5 schema × 15 表 | — | **70+ PASS** | ✅ |

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
2. **服务发现**：**无主动注册**（APISIX 直管 etcd upstream）
3. **SkyWalking tracer**：每个 svc 用 go2sky + gRPC Reporter → 自动上报
4. **GORM + schema-qualified 名**：`TableName()` 返回 `emotion_echo_xxx.tbl_name`
5. **Repository 模式**：interface + InMemory（测试替身）+ Postgres（生产）
6. **鉴权**：**信任 APISIX 注入的 X-User-Id**（svc 不做鉴权）
7. **配置**：yaml 文件（无 Nacos 配置中心）
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
# 1. 启动基础设施（不含 Nacos，详见 deploy/docker-compose.infra.yml）
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

# 5. 业务端点验证
curl -H "X-User-Id: 1" http://localhost:9080/api/v1/users/me
curl -X POST -H "X-User-Id: 1" -H "Content-Type: application/json" \
  -d '{"title":"我的会话"}' http://localhost:9080/api/v1/conversations

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