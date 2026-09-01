# Emotion-Echo · 架构决策记录（ADR）

> **本文档是 Emotion-Echo 微服务架构的"单一事实源"（Single Source of Truth）。**
> 所有 stage 文档、路线图、代码组织、配置都应与本文档一致。
> 决策变更时，**先改本文档，再改代码**，保持文档先行。
> 最后更新：2026-09-03（撤回 2026-08-31 决策 10 "不引入注册中心/配置中心" 判断；新增决策 11/12/13，演进路线 Stage 31/32/33；详见 `adr-2026-09-nacos-reintroduction.md`）

---

## 🎯 项目定位

**目标**：构建一个完整的、生产级 Go 微服务架构（含跨语言 Python LLM），
服务于 Emotion-Echo 情绪分析产品。

**当前阶段**：dev / 本地 docker-compose，未上生产。

**演进路线**：本地 Docker → K8s manifests 准备 → 未来上生产。

---

## ✅ 已敲定的决策（不可变更）

### 决策 1：HTTP 框架 = **Gin**（不再用 go-zero）

> ⚠️ **2026-08-31 审计标注：部分失效**。代码仍硬依赖 go-zero：各 svc 用 `go-zero/core/conf` 读配置、`core/logx` 打日志，shared `jwt_auth.go` 引 `go-zero/rest`（审计 E-2）。"不用 go-zero"仅指弃用 goctl 脚手架；go-zero 本体仍是实际依赖，ADR 说法需与代码一致。

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

> ❌ **2026-08-31 已退役**：Stage 30 删除 APISIX + etcd（commit `e9abac5`），服务入口由 web-bff 承担（决策 9）。"不引入注册中心"的结论不变——服务寻址回归静态 DNS，见决策 10 与审计 §七。

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
| etcd 形态 | ~~dev: 单节点；K8s: 集群（Raft 3 节点）~~ ❌ 2026-08-31 已退役 |

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

> ❌ **2026-08-31 已退役且未落地**：随 APISIX 一起退役。当前全链路 JWT **不验签**（shared `jwt_auth.go` 只 base64 解码 payload，签名验证点随 APISIX 消失），为审计 P0 问题 S-1；鉴权回归 BFF/svc 验签的修复见审计 §八 R-3。

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

> ❌ **2026-08-31 已退役且未落地**：限流/熔断随 APISIX 退役后**未在 BFF 实现**（stage-30 文档明示"当前未实现"）。恢复路径见 `stage-30-apisix-retirement.md` §五：BFF 内 `golang.org/x/time/rate` + `sony/gobreaker`（轻量）。

### 决策 9：服务入口 = **web-bff**（APISIX 已退役）

> ✅ 2026-08-31 生效（Stage 30 落地，commit `e9abac5`）。

| 维度 | 选择 |
|------|------|
| 统一入口 | **web-bff** :8894（`/api/v1/*` 聚合 5 下游 + SSE 流式编排 + CORS） |
| 鉴权 | BFF 透传校验（当前 JWT **不验签**，审计 P0 问题 S-1，修复见审计 §八 R-3） |
| 网关演进 | 需要边缘层时：路 1 = BFF 内限流/熔断；路 2 = 重引 APISIX 3.10+（`stage-30-apisix-retirement.md` §五） |

### 决策 10：配置中心 / 服务注册 = **演进引入 Nacos**（2026-09 撤回原"不引入"判断）

> ⚠️ **2026-09-03 撤回**原 2026-08-31"不引入"措辞。原判断把"当前 dev 单实例静态寻址"误当作"设计目标"，违背项目"分布式微服务架构"定位；且 Stage 30 把 BFF 当网关用导致 P0 问题（JWT 不验签 S-1、限流熔断缺失、CORS 错位 SSE 协议错位 A-1 等，详见 `architecture-audit-2026-08-31.md`）。本决策**改回引入**，分 Stage 31/32/33 演进；详见 `adr-2026-09-nacos-reintroduction.md`。

| 维度 | 选择 |
|------|------|
| 注册中心 | **Nacos 2.4.x**（`github.com/nacos-group/nacos-sdk-go/v2` ≥ v2.3.5；`nacos-sdk-python` ≥ 3.1.0 避开 3.0.x 断线重注册缺陷） |
| 配置中心 | 同 Nacos（一体化部署，避免引入多组件） |
| 配置中心**范围** | **仅放运营参数**（feature flag、限流阈值、模型路由表、Kafka 重试次数、A/B 分组）。`etc/*.yaml` 仍是启动默认值，Nacos 在启动后覆盖；**JWT secret、DATABASE_DSN、LLM_API_KEY 等敏感配置不进 Nacos** |
| 服务发现 | Nacos 主动注册 + 心跳；客户端定时拉取 + watch（30s 间隔）。**BFF 也参与发现**（Stage 32 APISIX `nacos-discovery` 上游拉取） |
| 命名空间 | `emotion-echo-dev` / `emotion-echo-prod`；group `DEFAULT_GROUP`；dataId `{service-name}`（注册）+ `{service-name}.ops.yaml`（运营参数） |
| 健康检查 | grpc health 探活（5s/次，连续 3 次失败自动摘除） |
| 演进路径 | Stage 31 注册+运营参数 → Stage 32 API 网关回归 → Stage 33 P0 修复 + BFF 净化 |

### 决策 11：API 网关 = **APISIX**（独立网关层，与 BFF 解耦）

> ✅ 2026-09-03 生效。纠正 Stage 30 "BFF 取代 APISIX"的错误归一；网关层独立。

| 维度 | 选择 |
|------|------|
| 网关 | **APISIX 3.18.0-debian**（Apache 顶级、OpenResty、插件丰富）+ `etcd v3.5.x` 配置后端 |
| 网关层职责（**仅本层做**） | 路由、鉴权（jwt-auth）、限流（limit-count/limit-req）、熔断（api-breaker）、CORS、全局日志、TLS 终结 |
| 3.9 SSL bug 绕过 | changelog 3.10–3.18 仍无 `ssl_certificate_by_lua_block` 修复条目（核实见 `stage-32-apisix-reintroduction.md` §三）；dev 走纯 HTTP，prod TLS 由前置 `nginx:alpine` 终结 |
| BFF 与网关关系 | BFF 是 APISIX 的 upstream（静态 upstream），BFF 不再承担任何网关职责 |

### 决策 12：BFF = **纯聚合层**（不再兼任网关）

> ✅ 2026-09-03 生效。澄清 BFF 与网关的边界。

| 维度 | 选择 |
|------|------|
| BFF 职责（**仅做**） | 多服务聚合、字段裁剪、SSE 流式编排、多端适配（PC/移动）、业务上下文（会话级） |
| BFF 不再做 | JWT 验签、CORS、限流、熔断、TLS 终结、全局路由表（迁至 APISIX） |
| BFF 入口 | 容器内 `web-bff:8894`，仅 APISIX 内部访问；宿主机不再直接映射（收敛入口） |
| BFF 内部鉴权 | 不做（信任 APISIX 已验签 + 注入 X-User-Id），通过 shared 中间件透传 |

### 决策 13：演进路线 = **串行 3 阶段**（骨架先，胶水后）

> ✅ 2026-09-03 生效。Stage 编号 31/32/33，每 Stage 含独立 PR + TDD + 收口文档。

| Stage | 范围 | 依赖 |
|-------|------|------|
| 31 · Nacos 演进 | 注册中心 + 配置中心落地；6 Go svc + 1 Python svc 接入；compose + Helm subchart | 无 |
| 32 · APISIX 回归 | 独立网关层 + etcd；BFF 退出鉴权/CORS/限流；3.9 SSL bug 绕开 | Stage 31（路由配置可推送到 Nacos） |
| 33 · P0 修复 + BFF 净化 | 修 SSE 协议、恢复消息落库、真实登录、端口收紧；BFF 移除所有网关职责 | Stage 32（APISIX 接管鉴权后 BFF 才能移除） |

### 决策 14：多模态融合 = **LLM-as-Fusion + LateFusion 兜底**（Stage 34）

> ✅ 2026-09-01 生效。详见 `stage-34-multimodal-fusion.md`。

| 维度 | 选择 |
|------|------|
| 主路径 | LLMFuser（DeepSeek/OpenAI 兼容协议，复用 BFF `LLM_BASE_URL`） |
| 兜底 | WeightedLateFuser（加权平均 + 模态缺失重分配） |
| 调度 | FusionWorker 5s tick，遍历 `fused_emotions` 中 `pending` 行 |
| 写库 | `FusedEmotionRepo.Upsert`（`ON CONFLICT message_id DO UPDATE`） |

### 决策 15：LLM Fusion 生产加固策略（Stage 35）

> ✅ 2026-09-03 生效。详见 `adr-2026-09-llm-fusion-hardening.md`。

| 维度 | 选择 |
|------|------|
| LLM 输出包装容错 | `unwrapLLMContent` 三段去包（``` ``` ``` ``` + 空白 + 双重 JSON） |
| LLM 输出校验 | 白名单 emotion + sentiment ∈ [-1,1] + modality_contrib 总和 ≈ 1 |
| 同 msgID 限流 | LRU(cap=1024) + TTL=4min，单实例内存 |
| LLM 超时 | 默认 3s（可 env `LLM_TIMEOUT` 覆盖） |
| LLM 雪崩保护 | 三态熔断器（Closed→Open→HalfOpen），连续 5 失败开 30s；**不重试** |
| 可观测 | 4 个 Prometheus collector（LLM call / latency / fallback / worker tick） |
| yaml 配置 | `${VAR:-default}` 占位符恢复；main.go `applyEnvOverrides` 补 `LLM_TIMEOUT` / `LLM_MODEL` / `LLM_BREAKER_*` / `WORKER_TICK_INTERVAL` |

### 决策 16：Stage 35 系统缺口正式登记（8 项不再 deferred）

> ✅ 2026-09-03 生效。详见 `adr-2026-09-known-gaps.md` + `stage-36-fixes-roadmap.md`。

8 项已知缺口全部纳入 Stage 36 修复日程（不再 deferred 到 Stage 36+）：

| # | 缺口 | 严重度 | 批次 |
|---|------|--------|------|
| G1 | 4 个 Go svc yaml 占位符（user/chat/analytics/assessment） | 🟡 中 | 36-A |
| G2 | chat-svc 缺 list conversations 端点 | 🟡 中 | 36-B |
| G3 | BFF 缺 analytics / assessment 路由聚合 | 🔴 高 | 36-A |
| G4 | Kafka off 时消息无自动情绪分析 | 🔴 高 | 36-B |
| G5 | 真实 LLM endpoint 未配 | 🔴 高 | 36-C |
| G6 | FER / SenseVoice `profile: ai` 镜像未构建 | 🟡 中 | 36-C |
| G7 | APISIX dashboard 镜像不可拉 | 🟡 中 | 36-D |
| G8 | Nacos 配置中心未启 | 🟢 低 | 36-D |

**原则**：Stage 36 之后所有缺口默认进入修复日程；唯一例外是需外部资源（API key / 付费服务）且无 dev 环境的项，标记为 **blocked-external**。

---

## 🏗 当前架构全景

```
                          浏览器 / 客户端
                                │
                                ▼ HTTP (dev) / HTTPS (prod)
                    ┌─────────────────────────┐
                    │  web-bff :8894           │  ← 唯一前端入口（Stage 30 替代 APISIX；Stage 33 净化为纯聚合）
                    │  /api/v1/* 聚合 + SSE    │     鉴权透传 / CORS / 流式编排
                    └────────────┬────────────┘
                                 │ 静态寻址：compose 容器 DNS / K8s Service FQDN
                                 │ (Stage 31 演进：svc 主动注册到 Nacos，APISIX 通过 nacos-discovery 插件拉实例)
        ┌────────────┬───────────┼───────────┬────────────┬────────────┐
        ▼            ▼           ▼           ▼            ▼            ▼
   user-svc    assessment-svc  chat-svc   ai-svc    analytics-svc   llm-svc
   :8888          :8889         :8890     :8891       :8893        :8000
   (Gin)          (Gin)         (Gin)    (Gin)       (Gin)        (Python FastAPI)
       │             │             │   │       │             │
       │             │             │   ▼       │             │
       │             │             │  Kafka    │             │
       │             │             │  (chat-   │             │
       │             │             │  events)  │             │
       │             │             │   │       │             │
       │             │             │   └───────┤             │
       │             │             │           ▼             │
       │             │             │    emotion-llm-service ◄───────┘
       │             │             │    (gRPC + mTLS)
       │             │             │
       ▼             ▼             ▼           ▼             ▼
   emotion_      emotion_      emotion_   emotion_       emotion_
   echo_user     echo_assess   echo_chat  echo_ai        echo_analyt

   ┌────────────────────────────────────────────────────────────┐
   │  治理层（Stage 31 落地后）                                      │
   │  ✅ Nacos 2.4.x 注册中心（8848/9848/9849）+ 配置中心（运营参数）      │
   │     namespace: emotion-echo-dev / emotion-echo-prod               │
   │     group: DEFAULT_GROUP；dataId: {svc}.ops.yaml                  │
   │  ☐ APISIX 3.18 网关层（Stage 32，依赖 31）                            │
   └────────────────────────────────────────────────────────────┘
   ┌────────────────────────────────────────────────────────────┐
   │  基础设施层                                                  │
   │  Postgres (5 schema) + Kafka + SkyWalking + Redis（闲置）       │
   │  ❌ etcd（Stage 30 已删；APISIX 默认后端，Stage 32 重新引入）       │
   └────────────────────────────────────────────────────────────┘
```

---

## 📋 服务清单（权威）

| svc | 端口 | 框架 | DB schema | 业务职责 | 状态 |
|-----|------|------|-----------|---------|------|
| **web-bff** | 8894 | Gin | — | 唯一前端入口：聚合 5 下游 + SSE + CORS | ✅ Stage 30 完成 |
| **user-svc** | 8888 | Gin | emotion_echo_user | 用户/Auth/上传 | ✅ Stage 1 完成 |
| **assessment-svc** | 8889 | Gin | emotion_echo_assessment | 量表/评估/报告 | ✅ Stage 1 完成 |
| **chat-svc** | 8890 | Gin | emotion_echo_chat | 会话/消息 + outbox | ✅ Stage 1 完成 |
| **ai-svc** | 8891 / gRPC 8892 | Gin | emotion_echo_ai | 情绪分析编排 | ✅ Stage 1 完成 |
| **analytics-svc** | 8893 | Gin | emotion_echo_analytics | 行为事件/报表 | ✅ Stage 1 完成 |
| **emotion-llm-service** | 8000 / gRPC 50051 | FastAPI | — | 文本情绪分析（当前为关键词器） | ✅ Stage 3 完成 |
| **Emotion-Echo-Web** | 3000 | Nuxt 3 | — | 前端 SPA | ✅ |
| **FER / sensevoice / XTTS** | 8004/8002/8003 | FastAPI | — | 人脸/语音识别、语音合成（可选） | ✅ |

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
│   ├── docker-compose.infra.yml             ← 容器编排（含 Nacos，Stage 31）
│   ├── apisix/                              ← 网关配置（Stage 32 引入）
│   └── db/                                  ← schema 脚本
├── emotion-echo-shared/                     ← 共享代码（Gin middleware 等）
│   └── pkg/
│       ├── skywalking/                      ← tracer + gorm + redis hooks
│       ├── messaging/                       ← Kafka Producer/Consumer
│       ├── middleware/                      ← Gin middleware (auth/cors/recover)
│       ├── discovery/                       ← Nacos Registry（Stage 31 PR-02/03）
│       └── configcenter/                    ← Nacos ConfigCenter（Stage 31 PR-04/05）
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
| **外部 API**（浏览器→BFF）| HTTP REST + SSE | JSON | web-bff :8894 | ✅ 已通 |
| **内部 RPC**（svc↔svc）| gRPC | Protobuf | 直接连 | ⏳ 待实施 |
| **异步事件**（svc→svc）| Kafka | JSON | Kafka broker | ✅ chat-events |

**当前阶段细节**：
- ai-svc → emotion-llm-service：gRPC + mTLS（Stage 18 落地）
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
# 1. 启动基础设施（含 Nacos 2.4.x，Stage 31 PR-11）
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

# 4. 验证（通过 BFF；各 svc 自带 /health）
curl http://localhost:8894/health          # BFF 聚合下游健康探测
curl http://localhost:8888/health          # user-svc
curl http://localhost:8890/health          # chat-svc
curl http://localhost:8889/health          # assessment-svc
curl http://localhost:8891/health          # ai-svc
curl http://localhost:8893/health          # analytics-svc

# 5. 验证 Nacos 注册中心（Stage 31 验收）
open http://localhost:8848/nacos           # 默认 nacos/nacos
# 服务管理 → 服务列表 → 应看到 7 个 service-name（user/chat/assessment/analytics/ai/web-bff/emotion-llm-service）且 health=true
./scripts/list_nacos_instances.sh

# 6. 看 trace
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

# Stage 31/32/33 演进（ADR 决策 10/11/12/13）
Stage 31 Nacos 治理层  ████░░░░░░░░░░░░░░░░  20% 🚧 (PR-01 文档收口；后续 PR-02..12 推进)
Stage 32 APISIX 网关   ░░░░░░░░░░░░░░░░░░░░   0% ☐ (依赖 31)
Stage 33 P0 修复+BFF净化 ░░░░░░░░░░░░░░░░░░░░   0% ☐ (依赖 32)
```

---

## 🔮 下一步（按优先级）

> ⚠️ 本节为 2026-07-14 遗留清单，多项已过期；**当前优先行动见 `architecture-audit-2026-08-31.md` §八**（R-1 起）。

1. **删除 Nacos 代码 + docker-compose**（决策 2 落地）— ✅ 已完成
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
| 2026-08-31 | 服务入口 | APISIX :9080 → **web-bff :8894** | APISIX 3.9 bug 未修复，BFF 替代网关（Stage 30，决策 9）|
| 2026-08-31 | 服务发现 | APISIX+etcd → **静态 DNS**（compose/K8s Service） | 网关退役；注册中心无收益（决策 10）|
| 2026-08-31 | 配置中心/注册中心 | （roadmap 未勾选）→ **明确不引入** | 单实例静态寻址已够；触发条件见审计 §七（决策 10）|
| 2026-08-31 | 鉴权位置 | APISIX jwt-auth（已退役）→ **回归 BFF/svc 验签**（待修复） | 全链路 JWT 不验签 = 审计 P0 问题 S-1 |
| 2026-09-03 | 注册中心/配置中心 | 决策 10 不引入 → **演进引入 Nacos**（Stage 31） | 原判断混淆 dev 现状与设计目标；BFF 兼任网关引发 P0；改为 Stage 31 引入 |
| 2026-09-03 | API 网关 | BFF 兼任 → **独立 APISIX 网关层**（决策 11，Stage 32） | 纠正 Stage 30 "BFF 取代 APISIX" 的错误归一；网关职责回归独立层 |
| 2026-09-03 | BFF 定位 | 网关 + 聚合 → **纯聚合层**（决策 12，Stage 33） | 鉴权/CORS/限流/熔断迁出 BFF，BFF 仅做面向前端的业务编排 |
| 2026-09-03 | 演进路线 | 无明确分阶段 → **Stage 31/32/33 串行**（决策 13） | 骨架先胶水后；每 Stage 含 TDD + 收口文档 + 独立 PR |

---

**所有文档（stage-X、roadmap、decomposition-plan）的具体实施细节以本文档为最终裁决。**