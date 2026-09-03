# Emotion-Echo 微服务拆分 · 落地规划（执行版）

> ⚠️ **架构最终决策以 [architecture-decisions.md](./architecture-decisions.md)（ADR）为单一事实源**。
> 本文档保留为**历史规划记录**，描述当时的拆分思路。
> **当前架构变更**（2026-07-14）：
> - go-zero → Gin（ADR 决策 1）
> - Nacos → 删除（ADR 决策 2）
>
> 配套文档：[distributed-roadmap.md](./distributed-roadmap.md)（5-Phase 总体路线） + [stage-0-learnings.md](./stage-0-learnings.md)（已落地的事）。
> 本文档专注于 **"把 emotion-echo-gin 单体拆成微服务"** 这一目标，把执行路径、依赖、验证、回滚讲透。
> 一句话：**拆 5 个微服务，但用 5 个月一步步来，任何一步出问题能立刻停下回滚**。

---

## 一、起点与终点

### 起点（2026-07-13 当前状态）

```
✅ 已经完成：
  - Phase 0.1  基础设施（8 个容器，APISIX/Nacos/SkyWalking/Kafka 都跑着）
  - Phase 0.3  Gin 接 SkyWalking（HTTP entry span）
  - Phase 0.4  GORM + Redis trace（DB/Redis exit span）
  - Phase 1.1  go-zero user-svc 脚手架（/health 通）

⚠️ 现状痛点：
  - emotion-echo-gin 14 个 handler 共享同一进程、同一份 DB、同一份 JWT
  - emotion-echo-user-svc 仅 1 个 /health 端点，未接 DB/Redis/Nacos
  - APISIX 路由是占位配置（指到 default-upstream）
  - 无任何服务向 Nacos 注册
```

### 终点（半年后目标）

```
emotion-echo-system/
├── emotion-echo-api-gateway       (APISIX，已就绪)
├── emotion-echo-user-svc          (Go-zero, 20001)
├── emotion-echo-chat-svc          (Go-zero, 20002)
├── emotion-echo-ai-svc            (Go-zero, 20003)
├── emotion-echo-assessment-svc    (Go-zero, 20004)
├── emotion-echo-analytics-svc     (Go-zero, 20005)
└── emotion-echo-monolith          (旧 Gin, 跑历史遗留路由, 标记 deprecated)

数据层：
  postgres: emotion_echo_user | emotion_echo_chat | emotion_echo_ai | emotion_echo_assessment
  redis: 共享缓存 + 分布式锁
  kafka: 跨服务事件总线

控制层：
  nacos: 所有 svc 注册 / 配置
  skywalking: 全链路追踪
  apisix: 路由 + 限流 + 鉴权

业务指标：
  - 单 svc 启动时间 < 5s
  - 跨服务调用链 P99 < 200ms（不包括 AI）
  - 任意 svc 崩溃不影响其他 svc
  - 单体 Gin 最终只跑 legacy 路由，所有新功能都不进它
```

---

## 二、3 个根本障碍（必须先解决才能拆）

### 🚧 障碍 A：共享数据库

**症状**：14 个 handler 都连 `postgres@emotion_echo`，跨域 JOIN 随意写。

**业务影响**：
- 用户表的 schema 改了，所有 svc 都受影响
- chat-svc 不小心 JOIN 了 users 表 → 拆开后这条 SQL 直接挂
- 数据备份/恢复只能整库操作 → 单点风险

**怎么解（前置工作）**：

| 步骤 | 输出 | 验收 |
|------|------|------|
| A1 | 列出每张表的"拥有者"（见下表） | 文档 |
| A2 | 物理上分 schema（`emotion_echo_user` 等） | sql 脚本 |
| A3 | gorm 注入时按 schema 分库连接 | 启动正常 |
| A4 | 跨域 SQL 全部改成 RPC 调用 | grep 验证 0 JOIN |

**数据归属表**：

| 数据 | 拥有者 | 其他 svc 怎么拿 |
|------|--------|---------------|
| `users` | user-svc | `user_rpc.GetUser(id)` |
| `refresh_tokens` | user-svc | `user_rpc.ValidateToken(token)` |
| `conversations` | chat-svc | `chat_rpc.ListConversations(userId)` |
| `messages` | chat-svc | `chat_rpc.ListMessages(convId)` + Kafka 事件 |
| `surveys` (题目) | assessment-svc | `assessment_rpc.GetSurvey(id)` |
| `survey_results` | assessment-svc | `assessment_rpc.GetUserResults(userId)` |
| `mental_health_*` | assessment-svc | RPC + Kafka |
| `emotion_analysis` | ai-svc | RPC + Kafka |
| `user_behavior_log` | analytics-svc | 仅自己写入 |

### 🚧 障碍 B：共享鉴权

**症状**：JWT 由 auth_handler.go 签发，所有 handler 用同一个 jwt middleware 解析。

**怎么解**：

```
Phase 0.5（本阶段完成）：
  - user-svc 暴露 AuthRpc gRPC：
    rpc ValidateToken(ValidateTokenReq) returns (ValidateTokenResp)
    rpc IssueToken(IssueTokenReq) returns (IssueTokenResp)
  - 旧 Gin 改造：auth_handler.go 改成"调 user-svc RPC"而非本地签 JWT
  - 新 svc 一律通过 user-svc RPC 验 token，不本地解析 JWT
```

**关键决定**：JWT secret 仍然统一（共用密钥），但**只有 user-svc 是 issuer**，其他 svc 都是 verifier via RPC。

### 🚧 障碍 C：隐式跨服务调用

**症状**：例如"发消息"流程可能跨 user/chat/ai 三个域。

**怎么解**：

| 调用类型 | 同步/异步 | 通信方式 |
|---------|----------|---------|
| GetUser / ValidateToken | 同步 | gRPC |
| SendMessage → UpdateConversation | 同步 | gRPC |
| SendMessage → AnalyzeEmotion | 异步 | Kafka |
| SubmitSurveyResult → UpdateMentalHealth | 异步 | Kafka + Saga |
| 用户行为日志 | 异步 | Kafka |

**事件 schema（Kafka topic 规划）**：

```
emotion-echo.user.events         { user.created, user.updated, user.deleted }
emotion-echo.chat.events         { conversation.opened, message.sent, conversation.closed }
emotion-echo.ai.events           { emotion.analyzed, voice.transcribed }
emotion-echo.assessment.events   { survey.completed, assessment.generated }
emotion-echo.analytics.events    { behavior.tracked }
```

每个事件统一 CloudEvents 规范：`{id, source, type, time, data}`。

---

## 三、5 个微服务的最终蓝图

```
┌──────────────────────────────────────────────────────────────────┐
│                       Browser / App / 3rd Party                  │
└─────────────────────────┬────────────────────────────────────────┘
                          │  HTTPS
┌─────────────────────────▼────────────────────────────────────────┐
│  APISIX Gateway (localhost:9080)                                  │
│  - 路由     - JWT 鉴权     - 限流     - CORS                      │
│  - 灰度     - 链路透传 SkyWalking                                │
└──┬─────────┬─────────┬─────────┬─────────┬──────────────────────┘
   │         │         │         │         │
   ▼         ▼         ▼         ▼         ▼
┌──────┐ ┌──────┐ ┌──────┐ ┌──────┐ ┌──────────┐
│ user │ │ chat │ │  ai  │ │assess│ │analytics │
│ 20001│ │ 20002│ │ 20003│ │ 20004│ │   20005  │
└──┬───┘ └──┬───┘ └──┬───┘ └──┬───┘ └────┬─────┘
   │        │        │        │          │
   ▼        ▼        ▼        ▼          ▼
┌─────┐ ┌──────┐ ┌──────┐ ┌────────┐ ┌────────┐
│user │ │ chat │ │  ai  │ │assess- │ │analytics│
│ pg  │ │  pg  │ │  pg  │ │ment pg │ │  pg     │
└─────┘ └──────┘ └──────┘ └────────┘ └────────┘
              ▲           ▲         ▲
              │           │         │
              └───────────┴─────────┘
                    Kafka (异步事件)
```

---

## 四、6 阶段执行计划（按月推进）

### Stage 0：先桥后路（1~2 周）

> **目标**：APISIX 真正接管流量，所有 svc 在 Nacos 注册，但**业务逻辑不动**。

| Step | 工作 | TDD 产物 | 验证 |
|------|------|---------|------|
| 0.1 | user-svc 接 Nacos | 测试：注册后能 listServices 找到 | Nacos 控制台看到 user-svc |
| 0.2 | user-svc 接 SkyWalking | 测试：trace 上报到 OAP | SW UI 看到 user-svc 入口 |
| 0.3 | APISIX 路由指向 user-svc | 测试：APISIX upstreams 中节点来自 Nacos | 浏览器访问网关触发 user-svc |
| 0.4 | 旧 Gin 也接 Nacos（双注册） | 测试：旧 Gin 在 Nacos 出现 | Nacos 同时有 2 个 entry |

**🎯 退出条件**：浏览器走网关，请求被路由到 user-svc，但**旧 Gin 仍然在跑所有真实业务**。

### Stage 1：拆分 assessment-svc（2 周）

> **目标**：把心理评估（Survey/Report/MentalHealth）整个域拆出去。理由：读多写少、依赖少、价值低。

| Step | 工作 | TDD 产物 | 验证 |
|------|------|---------|------|
| 1.1 | 拆 `emotion_echo_assessment` schema | 集成测试：assess-svc 连自己 DB | psql 列 schema |
| 1.2 | 创建 `assessment-svc` go-zero 项目（goctl api new） | HealthLogic 测试 | 跑起来 |
| 1.3 | 迁 Survey 模型 + 表 | 模型测试：Create/Save/Find 行为不变 | 单测 + 集成测试 |
| 1.4 | 迁 SurveyResult 模型 | 同上 | 同上 |
| 1.5 | 迁 MentalHealth 模型 | 同上 | 同上 |
| 1.6 | 暴露 RPC（后续给 chat-svc 用） | RPC test | grpcurl 通 |
| 1.7 | APISIX 切流：assessment 路径走新 svc | e2e：原接口返回值不变 | 灰度 1% → 100% |
| 1.8 | 旧 Gin assessment_handler.go 标记 deprecated | 注释 | — |

**🎯 退出条件**：所有 `/api/v1/surveys/*` 请求 100% 走 assessment-svc，旧 Gin 不再处理这部分。

### Stage 2：拆分 chat-svc（3 周）

> **目标**：拆会话 + 消息。这一步要解决"跨服务调用"——chat-svc 需要调 user-svc。

| Step | 工作 | TDD 产物 | 验证 |
|------|------|---------|------|
| 2.1 | 拆 `emotion_echo_chat` schema | 集成测试 | psql |
| 2.2 | chat-svc 项目脚手架 | HealthLogic 测试 | 跑起来 |
| 2.3 | user-svc 暴露 `GetUser` / `GetUsers` RPC（先 Stage 0 已搭好链路） | RPC 测试 | grpcurl |
| 2.4 | chat-svc 通过 RPC 拿用户信息（不查 user 表） | 集成测试 + mock test | 单测 |
| 2.5 | 迁 Conversation / Message 模型 | 模型测试 | 单测 + 集成测试 |
| 2.6 | Kafka producer 发布 `message.sent` 事件 | producer TDD（已建） | 真实发到 localhost:9092 |
| 2.7 | APISIX 切流 chat 路径 | e2e | 灰度 |

**🎯 退出条件**：所有 `/api/v1/conversations/*` 和 `/api/v1/messages/*` 走 chat-svc，跨服务调用走 RPC 或 Kafka。

### Stage 3：拆分 ai-svc（2 周）

> **目标**：AI 域天然异步，特别适合事件驱动。

| Step | 工作 | TDD 产物 | 验证 |
|------|------|---------|------|
| 3.1 | ai-svc 项目 | HealthLogic 测试 | 跑起来 |
| 3.2 | 迁 EmotionAnalysis 模型 | 模型测试 | 单测 |
| 3.3 | Kafka consumer 订阅 `message.sent`，触发分析 | consumer TDD | 真实消费 |
| 3.4 | 情绪分析完成后发 `emotion.analyzed` 事件 | producer TDD | 真实发 |
| 3.5 | APISIX 切流 ai 路径 | e2e | 灰度 |

**🎯 退出条件**：发消息后 5s 内 ai-svc 触发分析，OAP UI 上能看到 ai-svc 节点的 span。

### Stage 4：拆分 user-svc + analytics-svc（3 周）

> **目标**：user-svc 是最难拆的（所有 svc 依赖它），必须最稳。analytics-svc 顺便拆出。

| Step | 工作 | TDD 产物 | 验证 |
|------|------|---------|------|
| 4.1 | user-svc 接 Postgres 真 GetMe | 集成测试：DB 中造数据 → GetMe 返回 | e2e |
| 4.2 | user-svc 接 Redis（限流、token 黑名单） | 单测 | 单测 |
| 4.3 | user-svc 暴露全量 RPC（CRUD + Auth） | RPC 测试 | grpcurl |
| 4.4 | 旧 Gin auth/user/oauth handler 改为 RPC 调用 | 单测 | 单测 |
| 4.5 | analytics-svc 接 Kafka 订阅所有事件 | consumer TDD | 真实消费 |
| 4.6 | 全部 APISIX 切流 | e2e | 全量 |

**🎯 退出条件**：旧 Gin 不再处理任何"用户/分析"路由，全部 user-svc 化。

### Stage 5：旧 Gin 退役（1 周）

| Step | 工作 | 验证 |
|------|------|------|
| 5.1 | 旧 Gin 跑空（只剩 404 + 日志） | 所有真实业务 100% 走新 svc |
| 5.2 | 移除 `emotion-echo-gin` 目录（或 archive 到 `legacy/`） | 仓库变小 |
| 5.3 | 文档更新：所有 README 指向新仓库结构 | grep 验证 |

---

## 五、严格遵守 AGENTS.md 的 TDD 节奏

每个 Stage 必须按这个流程：

```
1. 🔴 RED   写失败的测试（描述本 Stage 的契约）
2. 🟢 GREEN 写最小实现让测试通过
3. ♻️ REFACTOR 整理代码 / 重命名 / 抽公共
4. CI GATE  go test ./... + go vet + integration test 全过
5. 手动验收 按本 Stage 的"验证"列实测
6. 提交      feat: ... / test: ... / refactor: ... 前缀
```

### 每个 Stage 的最低测试覆盖

| 类型 | 覆盖率底线 |
|------|----------|
| `internal/logic/*` | 90% |
| `internal/handler/*` | 70% |
| RPC 调用 | 100%（必须有 mock test） |
| 集成测试 | 至少 1 个 e2e |

---

## 六、风险矩阵 + 回滚方案

| 风险 | 概率 | 影响 | 缓解 | 回滚 |
|------|------|------|------|------|
| APISIX 切流后旧 Gin 数据不一致 | 中 | 高 | 灰度 1% → 10% → 50% → 100%，每步观察 1h | APISIX 切回旧 Gin 入口 |
| 跨服务 RPC 链路过长（>500ms） | 中 | 中 | 同步 → 异步改造 | 临时调短链路 |
| Kafka 消费积压 | 低 | 中 | 监控 lag、自动扩容 consumer | 重启 consumer / 跳过老消息 |
| 数据库迁移丢失数据 | 低 | 极高 | 只读迁移 → 双写 → 切读 → 停写 → 删旧 | 旧库备份未删前都能回 |
| Nacos 集群全挂 | 极低 | 高 | APISIX 缓存路由表 + 客户端 fallback | 切到静态 upstream |

**核心回滚原则**：**任何 Stage 结束前，旧代码不能删**。APISIX 切流是单向门，进去就别想回去。所以每个 Stage 完成后**保留旧代码 + 旧 schema 至少一个月**。

---

## 七、与分布式路线图（Phase 0~5）的关系

| distributed-roadmap.md 的 Phase | 本文档对应 Stage | 当前状态 |
|-------------------------------|----------------|---------|
| Phase 0 基础设施 | 早期已完成 | ✅ done |
| Phase 1 go-zero 改造 | Stage 0（本规划）+ Stage 1.1 | ✅ 1.1 done / 0 待开始 |
| Phase 2 Kafka 异步化 | Stage 0.6、2.6、3.3 | producer ✅ / consumer pending |
| Phase 3 限流/熔断/配置中心 | Stage 0.1（Nacos）、4.3（RPC 熔断） | 部分 |
| Phase 4 业务域拆分 | Stage 1~5 全部 | 本文档核心 |
| Phase 5 K8s 化 | Stage 5 之后 | 待规划 |

---

## 八、跨阶段一致性约束（不可妥协）

| 约束 | 原因 |
|------|------|
| 所有 svc 必须在 Nacos 注册 | 否则 APISIX 找不到 |
| 所有 RPC 调用必须带 trace 头 | 否则跨服务 trace 断 |
| 所有 svc 必须 emit metrics | Prometheus / SkyWalking |
| 所有 svc 启动 < 5s | K8s readiness probe 容忍上限 |
| 所有 svc 必须有 `/health` 端点 | K8s liveness probe |
| 任何 svc 不能跨库 JOIN | 微服务铁律 |
| 任何 svc 不能"反向依赖"更高层 svc | 依赖方向：API → svc → DB/Kafka，单向 |

---

## 九、协作约定（呼应 AGENTS.md）

每个 PR 必填：
- 关联的 Stage 编号
- 测试通过截图 / 命令
- 灰度切流步骤（如有）

每周一次"Stage Review"：
- 当前 Stage 是否真的完成？
- 下一个 Stage 风险评估
- 旧 Gin 删代码审计

---

## 十、立即可动手的下一步（本周）

| 优先级 | 任务 | 用时 |
|-------|------|------|
| 🔴 P0 | 确认本规划文档被评审通过 | 1 天 |
| 🟠 P1 | Stage 0.1：user-svc 接 Nacos（带 TDD） | 2 天 |
| 🟠 P1 | 启动 RPC 接口定义 proto 文件，user.proto + assessment.proto | 1 天 |
| 🟡 P2 | 拆分数据库 schema（仅 DDL 脚本，不迁数据） | 1 天 |
| 🟢 P3 | 设置 SkyWalking 告警规则（trace error rate > 5% 通知） | 1 天 |

---

## 附录 A：每个 svc 的 RPC 接口草图

```protobuf
// user.proto
service UserRpc {
    rpc IssueToken(IssueTokenReq) returns (IssueTokenResp);
    rpc ValidateToken(ValidateTokenReq) returns (ValidateTokenResp);
    rpc GetUser(GetUserReq) returns (User);
    rpc GetUsers(GetUsersReq) returns (UserList);
    rpc CreateUser(CreateUserReq) returns (User);
    rpc UpdateUser(UpdateUserReq) returns (User);
    rpc DeleteUser(DeleteUserReq) returns (Empty);
}

// assessment.proto
service AssessmentRpc {
    rpc ListSurveys(ListSurveysReq) returns (SurveyList);
    rpc GetSurvey(GetSurveyReq) returns (Survey);
    rpc SubmitResult(SubmitResultReq) returns (SubmitResultResp);
    rpc GetUserResults(GetUserResultsReq) returns (ResultList);
    rpc GenerateAssessment(GenerateAssessmentReq) returns (Assessment);
}

// chat.proto
service ChatRpc {
    rpc CreateConversation(CreateConvReq) returns (Conversation);
    rpc ListConversations(ListConvReq) returns (ConvList);
    rpc SendMessage(SendMsgReq) returns (Message);
    rpc ListMessages(ListMsgReq) returns (MsgList);
    rpc CloseConversation(CloseConvReq) returns (Empty);
}

// ai.proto
service AiRpc {
    rpc AnalyzeEmotion(AnalyzeEmotionReq) returns (EmotionResult);
    rpc TranscribeVoice(TranscribeReq) returns (TranscribeResult);
    rpc DetectFace(FaceReq) returns (FaceResult);
}
```

---

## 附录 B：Kafka topic 详细 schema

```json
{
  "specversion": "1.0",
  "id": "uuid-v4",
  "source": "/chat-svc",
  "type": "com.emotionecho.chat.message.sent",
  "time": "2026-07-13T10:00:00Z",
  "datacontenttype": "application/json",
  "data": {
    "messageId": "uuid",
    "conversationId": "uuid",
    "userId": 12345,
    "content": "string",
    "sentAt": "2026-07-13T10:00:00Z"
  }
}
```

每个 topic 配 3 个 partition + replication factor 1（单机环境）。

---

## 附录 C：目录结构最终态（半年后）

```
Emotion-Echo/
├── AGENTS.md
├── README.md
├── deploy/                       (不变)
├── docs/
│   ├── distributed-architecture.md
│   ├── distributed-roadmap.md
│   ├── stage-0-learnings.md
│   ├── microservice-decomposition-plan.md   ← 本文档
│   └── architecture-decisions/             (ADR 决策记录)
├── emotion-echo-user-svc/        ← Go-zero
├── emotion-echo-chat-svc/        ← Go-zero
├── emotion-echo-ai-svc/          ← Go-zero
├── emotion-echo-assessment-svc/  ← Go-zero
├── emotion-echo-analytics-svc/   ← Go-zero
├── emotion-echo-front/           ← 不变
├── emotion-echo-ai/              ← 不变（Python AI 服务，注意命名冲突）
├── emotion-echo-shared/         ← 🆕 共享代码（proto、pkg）
│   ├── proto/
│   ├── pkg/skywalking/
│   ├── pkg/messaging/
│   └── pkg/auth/
└── legacy/
    └── emotion-echo-gin/         ← 旧 Gin（标记 deprecated）
```

---

> 最后更新：2026-07-13  
> 适用版本：Phase 1.2 ~ Phase 5 全部完成期间  
> 评审人：项目所有者