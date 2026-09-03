# Emotion-Echo · 架构审计报告（2026-08-31）

> **背景**：Stage 30 收尾（APISIX + etcd 退役、web-bff 成为唯一入口）之后，对仓库做一次全量静态架构审计。
> **范围**：Go 微服务（user/chat/assessment/analytics/ai-svc + web-bff + shared）、Python AI 服务（emotion-llm-service + FER/sensevoice/XTTS）、Nuxt 前端、docker-compose 编排、Helm/K8s chart、文档与工程流程。
> **方法**：静态代码走读 + 关键结论逐行核实（JWT 鉴权、SSE 协议、Kafka 消费者、前端发送链路均直接读过实现，非仅凭搜索）。
> **结论先行**：**主聊天链路当前是断的（不落库 + SSE 协议不匹配）、JWT 鉴权形同虚设（不验签）、数据管线缺乏可靠性兜底（DLQ 空操作、无迁移、无幂等）、全仓库零 CI**。不涉及任何 P0 功能/安全问题的新增组件（配置中心/服务注册中心）当前不需要引入，见 §七。

- 审计日期：2026-08-31
- 关联文档：[architecture-decisions.md](architecture-decisions.md)（同步修订）、[distributed-roadmap.md](distributed-roadmap.md)（3.2 条目评审）、[stage-30-apisix-retirement.md](stage-30-apisix-retirement.md)
- 状态说明：本文档为**发现清单**，§八给出修复顺序；修复实施由后续 PR 推进（遵循 AGENTS.md TDD 约定）

---

## 一、执行摘要

| 编号 | 严重级 | 类别 | 一句话摘要 | 状态 |
|------|--------|------|-----------|------|
| [A-1](#a-1-p0--主聊天链路断裂) | 🔴 P0 | 功能 | 主聊天不落库 + SSE 协议不匹配，AI 回复显示不出来，分析/报表管线悬空 | ☐ |
| [S-1](#s-1-p0--jwt-从不验签) | 🔴 P0 | 安全 | JWT 只 base64 解码不验签（APISIX 退役后验签点消失），伪造 token 可冒充任意用户 | ☐ |
| [S-2](#s-2-p0--端口全暴露与基础设施零防护) | 🔴 P0 | 安全 | 全部端口暴露宿主机、Kafka/PG/Redis/SkyWalking 零防护、compose 下 mTLS 配置错位 | ☐ |
| [D-1](#d-1-p1--数据库无迁移机制) | 🟠 P1 | 可靠性 | 无迁移工具/版本表，user/assessment 建表靠手工 SQL，chat 启动内联建表 | ☐ |
| [K-1](#k-1-p1--kafka-可靠性缺口) | 🟠 P1 | 可靠性 | analytics DLQ 是空操作、outbox 无限重试无告警、按 UUID 分区乱序、无重放工具 | ☐ |
| [I-1](#i-1-p1--无端到端幂等) | 🟠 P1 | 可靠性 | 消息自增主键、POST 无幂等键，重复点击重复入库 | ☐ |
| [C-1](#c-1-p1--情绪分析实为关键词器且存在双算) | 🟠 P1 | 一致性 | llm-service 是关键词器非真 LLM，与 DeepSeek 两套算法，同文本可双算结果不一致 | ☐ |
| [E-1](#e-1-p1--零-cicd) | 🟠 P1 | 工程 | 无任何自动化管道，本地 main 领先 origin 133 提交从未推送 | ☐ |
| [E-2](#e-2-p1--adr-已腐烂) | 🟠 P1 | 工程 | "单一事实源" ADR 停在 7-14，go-zero/APISIX 断言与代码互相矛盾 | ☐ |
| [T-1](#t-1-p1--前端测试严重偏科) | 🟠 P1 | 工程 | 19 个页面 0 测试、13 组件仅 2 个被测；e2e 仅 1 条 login 用例 | ☐ |
| [E-3](#e-3-p2--依赖卫生) | 🟡 P2 | 工程 | shared go-zero v1.6.0 落后服务 v1.10.x 三个大版本；Python 无锁文件 | ☐ |
| [O-1](#o-1-p2--k8s-探针缺失) | 🟡 P2 | 运维 | web-bff / web / xtts 的 deployment 无任何 probe | ☐ |
| [O-2](#o-2-p2--web-bff-skywalking-默认关闭) | 🟡 P2 | 运维 | web-bff 的 `Enabled: false`，唯一聚合层不在 trace 链路内 | ☐ |
| [O-3](#o-3-p2--redis-纯闲置) | 🟡 P2 | 运维 | compose/Helm 都部署了 Redis，但没有任何服务连接它 | ☐ |
| [O-4](#o-4-p2--前端-env-残留-已退役-地址) | 🟡 P2 | 运维 | `.env` 指向已退役的 APISIX `:9080` | ☐ |
| [O-5](#o-5-p2--nuxt-默认直连-user-svc-绕过-bff) | 🟡 P2 | 运维 | `API_BASE_URL` 兜底直指 `localhost:8888`，绕过 BFF 边界 | ☐ |
| [O-6](#o-6-p2--仓库杂物) | 🟡 P2 | 卫生 | 根目录残留 apisix-*.json、err.log、pg_log.txt 等 | ☐ |

---

## 二、🔴 P0 · 功能

### A-1 · P0 · 主聊天链路断裂

**现象**：

1. **写路径断裂**：前端主聊天页 `Emotion-Echo-Web/app/pages/chat/conversation/[id].vue:155` 通过 `useConversationSender.sendToExistingConversation` 发送，链路是 `useConversationSender.ts → useAIStreamHandler.sendAIStream → POST {API_BASE_URL}/ai/stream`（`Emotion-Echo-Web/app/composables/useAIStreamHandler.ts:64,69`）。BFF 端点 `emotion-echo-web-bff/internal/handler/ai_stream_handler.go` 只做 mock/DeepSeek 流式回复，**全程不调用 chat-svc、不写 messages 表**。
2. **落库旁路无人用**：唯一会写库的 `sendMessage`（`Emotion-Echo-Web/app/stores/message.ts:73` 定义，走 `POST /conversations/:id/messages` → chat-svc）**没有页面/组件调用**（全仓 grep 无命中）。
3. **SSE 协议不匹配**：BFF 输出 OpenAI 格式 `data: {"choices":[{"delta":{"content":...}}]}` + `data: [DONE]`（`ai_stream_handler.go:9,95-102,139`）；前端按 `data.type === 'start'|'delta'|'finish'` 解析（`useAIStreamHandler.ts:119-167`）。OpenAI 格式没有 `type` 字段 → 每个 delta 被静默丢弃；`[DONE]` 不是合法 JSON → 触发解析错误计数（`:168-174`），`onFinish` 永不触发。

**影响**：

- 用户发出的消息与 AI 回复只存在前端内存（`messageStore`），**刷新即消失**；
- `message.created` 事件永远不产生 → ai-svc 情绪分析、analytics-svc 行为事件**整条 Kafka 管线悬空**，报表无数据；
- AI 回复气泡永远停留在 "streaming" 状态（`onFinish` 不触发，`tempAiMessage.status` 无法置为 `sent`），实际表现为**聊天功能不可用**；
- 与 `docs/stage-30-apisix-retirement.md:62`"发消息 → BFF → chat-svc 落库（已验证通过）"的表述直接矛盾——文档验证的是旁路（`sendMessage`），不是前端真实路径。

**证据**：

- `Emotion-Echo-Web/app/composables/useConversationSender.ts`（sendAIStream 调用）
- `Emotion-Echo-Web/app/composables/useAIStreamHandler.ts:64,69,119`（协议）
- `emotion-echo-web-bff/internal/handler/ai_stream_handler.go:61-163`（无 chat-svc 调用）
- `Emotion-Echo-Web/app/stores/message.ts:73,154`（sendMessage 定义/导出，无调用者）

**建议修复**（含测试前置）：

1. 统一 SSE 协议：**前端按 OpenAI 兼容格式解析**（或 BFF 同时发 `type` 字段），用 e2e 或组件测试覆盖 `onDelta/onFinish` 触发；
2. 恢复写路径：`sendAIStream` 完成后把消息回写 chat-svc（或 BFF 在流式前先落库 + 返回 `messageId`，参考 `useConversationSender` 里 `onStart` 已预留的 `userMessageId` 契约）；
3. 对齐 `stage-30-apisix-retirement.md` 的验证描述或标注其仅覆盖旁路。

---

## 三、🔴 P0 · 安全

### S-1 · P0 · JWT 从不验签

**现象**：`emotion-echo-shared/pkg/middleware/jwt_auth.go:61-99` 的 `extractUserIDFromJWT` **只 base64 解码 JWT payload 取 `user_id`，不验证签名**。文件头注释（`:3-11`）写明前提是"已被 APISIX jwt-auth 验过"——但 APISIX 已在 Stage 30 退役（commit `e9abac5`），**全链路现在没有任何一层做签名验证**。

**影响**：

- 构造 `Authorization: Bearer <header>.<payload>.随便` 即可冒充任意 `user_id` 调用全部服务；5 个 Go svc 全部挂载该中间件（`user-svc/main.go:98`、`chat-svc/main.go:158`、`assessment-svc/main.go:77`、`analytics-svc/main.go:170`、`ai-svc/main.go:307`）；
- BFF 的 `login`/`register` 是 mock（`emotion-echo-web-bff/internal/handler/auth_handler.go`：账号密码非空即签发 token），`verification-code` 端点空转；JWT secret 默认 `dev-bff-secret`（`deploy/docker-compose.apps.yml:455`）；
- ai-svc 的 gRPC :8892 **连假鉴权都没有**（`emotion-echo-ai-svc/internal/grpcserver/server.go:46-56` 只有 tracing/logging/recovery）；
- BFF CORS 回显任意 Origin 且带 `Allow-Credentials`（`emotion-echo-web-bff/main.go:86-101`），叠加假 token，危害进一步放大。

**证据**：`emotion-echo-shared/pkg/middleware/jwt_auth.go:61-99`；`docs/stage-6-apisix-security.md`（trust APISIX 模型的历史依据）。

**建议修复**（安全回归，先测试后实现）：

1. 在 shared 加**真正的 JWT 验签中间件**（HMAC，secret 由 env 注入），替换现有 base64 解码逻辑；`bffAuthMiddleware` 与 5 个 svc 统一使用；
2. BFF 侧：真实登录（校验密码哈希）、验证码限流、secret 强默认值；
3. ai-svc gRPC server 加鉴权 interceptor；
4. 修复后删除"trust APISIX"注释与相关文档段落。

### S-2 · P0 · 端口全暴露与基础设施零防护

**现象**：

- **端口映射到宿主机**（`deploy/docker-compose.apps.yml`）：user-svc 8888 (L46)、chat-svc 8890 (L87)、analytics-svc 8904:8893 (L131)、assessment-svc 8889 (L171)、llm-service 8000+50051 (L221-222)、ai-svc 8891+8892 (L275-276)、web-bff 8894 (L462)、web 3000 (L503)；FER/sensevoice/XTTS 8004/8002/8003（`--profile ai`）；
- **基础设施零防护**（`deploy/docker-compose.infra.yml`）：postgres 5432 (L24，密码 `postgres`、`sslmode=disable`)、redis 6379 (L45，无密码)、kafka 9092 (L77，`PLAINTEXT` 无 SASL/ACL，`AUTO_CREATE_TOPICS` 开启)、skywalking-oap 11800/12800 (L100-101) + UI 18080 (L118，无鉴权可看全部调用链)；
- **mTLS 配置错位**：llm-service 服务端开 `TLS_ENABLED: "1"` + 要求客户端证书（`apps.yml:215`），`INTERNAL_API_KEY_REQUIRED` 默认 `0`（`:213`）；但 **ai-svc 的 compose 环境没传 `TLS_ENABLED`**，`emotion-echo-ai-svc/internal/analyzer/grpc_analyzer.go:132` 默认明文 → 两端协议不匹配（gRPC 握手失败退降级）。且 `deploy/tls/` 目录不在仓库（compose 挂载 `../deploy/tls/*.crt` 会直接起不来），Helm 侧 `ai-svc-tls` secret 是占位符。

**影响**：局域网内任何主机可绕过 BFF 直连全部业务与基础设施；结合 S-1 的假鉴权，等于对局域网开放了所有数据与调试接口。SkyWalking 暴露还意味着业务消息明文轨迹可被任意查看。

**证据**：`deploy/docker-compose.apps.yml`、`deploy/docker-compose.infra.yml`（行号见上）；`emotion-echo-ai-svc/internal/analyzer/grpc_analyzer.go:132`。

**建议修复**：

1. compose 只映射 `web:3000` 与 `web-bff:8894`，其余服务去掉 `ports:`（容器内用 service 名互通）；
2. 基础设施加最简防护：PG 强密码、Kafka SASL/SCRAM 或至少限制监听地址、SkyWalking 不映射宿主；
3. 统一 mTLS：ai-svc 补 `TLS_ENABLED` env，或把 llm-service 退回明文并清理错位配置；生成 `deploy/tls/` 证书（脚本入库）或改 K8s cert-manager 签发。

---

## 四、🟠 P1 · 数据可靠性

### D-1 · P1 · 数据库无迁移机制

**现象**：无 goose/golang-migrate/AutoMigrate/版本表。基线建表靠 `deploy/db/01-create-schemas.sql` + `02-create-tables-in-schemas.sql` 手工执行；**user-svc 与 assessment-svc 无任何 migrations 目录**；chat-svc 在 `main.go:191` 用 gorm `Exec` 启动时内联建 outbox 表（注释自认"生产应该用 migrate 工具；当前学习/开发期 OK"）；其余 svc 各有 1-6 个散落的 `migrations/` 目录。

**影响**：表结构变更不可追踪、不可回滚、无单一入口；换环境（新库）需要手工重放全部 DDL；多 schema 同库（`emotion_echo` + 5 schema）下风险叠加。

**建议修复**：引入 golang-migrate（或 goose），把 `deploy/db/` 与各 svc migrations 统一收口为版本化迁移；加集成测试（`//go:build integration`）验证迁移可在空库上完整跑通。

### K-1 · P1 · Kafka 可靠性缺口

**现象**：

1. **analytics-svc 的 DLQ 是空操作**：`emotion-echo-analytics-svc/main.go:149` 调 `kafka.NewConsumer(...)` 后未调 `.WithDLQ(...)`，`internal/kafka/consumer.go:64-65` 默认 `NoopDLQPublisher` → 毒消息重试超限后**只打日志 + 提交 offset，静默丢弃**；
2. **outbox 无限重试**：chat-svc `internal/outbox/relay.go` 1s 轮询 pending 行，失败仅 attempts+1 保持 pending，**无最大尝试、无告警**；
3. **事件乱序**：producer 用 `NewHashPartitioner`、key=**事件 UUID**（`internal/events/events.go`），不是会话 id → 同一会话的消息事件无顺序保证；
4. **无重放手段**：全仓库无 DLQ/outbox 回放工具；ai-svc 的 attempts 计数是进程内 map（`internal/consumer/consumer.go`），rebalance/重启即清零。

**影响**：消息丢失无感知（analytics 侧）、失败事件无限滞留（chat 侧）、同会话分析乱序、故障恢复只能人工处理。

**建议修复**：给 analytics-svc 接真实 DLQ（配置 `DLQTopic`）；outbox 加最大尝试数 + 告警日志；producer key 改为会话/用户 id；补充 DLQ 消费与回灌工具或脚本。

### I-1 · P1 · 无端到端幂等

**现象**：`messages` 表主键是 `BIGSERIAL` 自增（`deploy/db/02-create-tables-in-schemas.sql`）；`POST /messages` 无客户端幂等键；前端去重仅覆盖 GET（`Emotion-Echo-Web/app/composables/useApi.ts` 的 `if (method === "GET")`）；唯一幂等兜底在消费端（`emotion_echo_ai.emotion_analysis` 的 `event_id` UNIQUE + `ON CONFLICT DO NOTHING`）。

**影响**：网络重试 / 重复点击会重复入库；消费端幂等管不到写入侧，messages 表可能膨胀与重复。

**建议修复**：客户端生成消息 UUID + 服务端唯一约束（或 Idempotency-Key）；把现有 `event_id` 幂等模式推广到 `messages` 表。

### C-1 · P1 · 情绪分析实为关键词器且存在双算

**现象**：`emotion-llm-service/main.py` 是**关键词 + 情感词典匹配**（`model=keyword-v1`），不是真 LLM。聊天回复走 BFF → DeepSeek（付费），情绪标签走 ai-svc → llm-service 关键词器——**同一段文本被两套完全不同的算法各处理一次**。且 `/api/v1/multimodal/analyze` 同步分析不落库，可与 Kafka 管线对同一文本再算一次；llm-service 故障时 `ChainedAnalyzer` 降级本地 `KeywordAnalyzer`，正常/降级结果无一致性保证。

**影响**：情绪标签与回复情绪可能不一致；同步/异步两条路径结果可能互相矛盾；"情绪分析"实际是关键词分类，业务上需要明确这是有意为之还是待升级。

**建议修复**：明确 llm-service 定位（关键词器 or 真 LLM）并在文档/README 声明；消除对同一文本的双算路径（`multimodal/analyze` 与 Kafka 管线二选一，或统一语义）；降级路径在结果上打标记。

---

## 五、🟠 P1 · 工程治理

### E-1 · P1 · 零 CI/CD

**现象**：全仓库无 `.github/workflows`、`.gitlab-ci.yml`、`Jenkinsfile`、根/服务级 Makefile（仅 `legacy/emotion-echo-gin/Makefile` 与 vendored `XTTS/TTS/Makefile`）；`scripts/` 只有手动冒烟脚本；本地 main **领先 origin 133 个提交且从未推送**。

**影响**：AGENTS.md 的 `go test ./...` + `go vet` + `npm run lint` 合并前门禁**无人机械执行**；TDD 红绿分离纪律实际靠自觉（git log 中存在大量 `feat(xxx): ...（RED+GREEN 合并）` 提交）；P0 的 A-1（聊天链路断裂）没有任何测试抓住，正是零集成验证的直接代价。

**建议修复**：先落一条最小 CI（GitHub Actions）：`go test ./...` + `go vet ./...` + `npm run test` + `helm lint`；再逐步加 pytest 与一条前端 e2e 冒烟（覆盖登录 → 发消息 → SSE 收到 delta）。

### E-2 · P1 · ADR 已腐烂

**现象**：`docs/architecture-decisions.md` 自称"单一事实源"、要求"先改文档再改代码"，但最后更新停在 2026-07-14；决策 1 断言 go-zero 废弃（代码仍硬依赖 `go-zero/core/conf`、`core/logx`、shared 里 `go-zero/rest`）；决策 2/7/8 的 APISIX+etcd / jwt-auth / 限流熔断已随网关退役；完全未提及 web-bff、Helm umbrella、SkyWalking、TLS。

**影响**：文档与代码互为矛盾，"文档先行"的机制已失效；新人按 ADR 会得出错误结论。

**建议修复**：本次审计已同步修订 ADR（标注废弃决策 + 新增决策 9/10，见 `architecture-decisions.md`）；后续约定：架构决策变更必须同步 ADR，否则 PR 打回。

### T-1 · P1 · 前端测试严重偏科

**现象**：19 个测试文件 = 18 unit + 1 e2e（仅 login）；覆盖：23 utils 测 13、19 composables 测 4、6 stores 测 2、13 components 测 2、**19 pages 测 0**。

**影响**：核心交互（聊天、SSE、路由）完全无测试——A-1 的协议断裂就是这样漏过的。

**建议修复**：优先补 `useAIStreamHandler`（SSE 解析）与 `useConversationSender`（发送链路）的单元测试，再补一条聊天页 e2e。

### E-3 · P2 · 依赖卫生

**现象**：go-zero 版本三足鼎立——`emotion-echo-shared` v1.6.0 vs 5 个业务 svc v1.10.2 vs web-bff v1.10.3；Python 侧 4 份 requirements.txt 全为宽松 `>=` 版本、无锁文件。

**影响**：shared 老版本与 svc 行为差异难排查；Python 依赖不可复现。

**建议修复**：shared 与各 svc 统一 go-zero 版本；Python 引入 `uv pip compile`/`pip-tools` 生成锁文件。

---

## 六、🟡 P2 · 运维细节

### O-1 · P2 · K8s 探针缺失

web-bff / web / xtts 三个 `charts/emotion-echo/charts/*/templates/deployment.yaml` **无任何 probe**（其余服务均有 startup/readiness/liveness 三探针）。BFF 是全链路单点，缺失探针意味着故障时流量继续打进已挂的 BFF。建议补齐三探针。

### O-2 · P2 · web-bff SkyWalking 默认关闭

`emotion-echo-web-bff/etc/web-bff.yaml:19` `Enabled: false`（其余 5 个 svc 均开启并挂 GinSkywalkingMiddleware）。唯一聚合层不在 trace 链路内，跨服务调用链断在入口。建议默认开启并验证端到端 trace。

### O-3 · P2 · Redis 纯闲置

compose 与 Helm 都部署了 Redis，但**没有任何 Go/Python 服务连接它**（全仓仅 `emotion-echo-shared/pkg/skywalking/redis_tracing.go` 一个无人调用的 hook）。注意：这意味着将来 BFF 限流若做多实例共享计数，必须引入 Redis（或降级为单实例内存限流）。

### O-4 · P2 · 前端 .env 残留已退役地址

`Emotion-Echo-Web/.env:2` 仍指向 `http://localhost:9080/api/v1`（已退役的 APISIX）。本地未配环境变量时会把请求打到不存在的地方。建议改为 `:8894/api/v1`（BFF）。

### O-5 · P2 · Nuxt 默认直连 user-svc 绕过 BFF

`Emotion-Echo-Web/nuxt.config.ts:15`：`API_BASE_URL || "http://localhost:8888/api/v1"` 兜底直连 user-svc（APISIX 3.9 bug 时期的遗留）。未设 env 时 SPA 直接绕过 BFF 边界。建议默认改为 BFF `:8894`。

### O-6 · P2 · 仓库杂物

根目录残留 `apisix-*.json`、`err.log`、`pg_log.txt` 等已退役产物；`playwright-report/`、`test-results/` 为未跟踪目录。建议清理并加入 `.gitignore`。

---

## 七、架构决策结论：配置中心 / 服务注册

> 回答 Stage 30 之后遗留的问题：**当前是否需要配置中心、服务注册中心？**

### ⚠️ 2026-09-03 撤回原"不需要"判断

原判断（2026-08-31）把当前 dev 单实例静态寻址误当成设计目标，违背项目"分布式微服务架构"定位；且 Stage 30 把 BFF 兼任网关的代价（JWT 不验签 / 限流熔断缺失 / CORS 错位 / SSE 协议错位）正是本审计 §二 P0 的根因之一。

**演进路径**（详见 `adr-2026-09-nacos-reintroduction.md`）：
- Stage 31 引入 Nacos（注册 + 配置）
- Stage 32 回归 APISIX 独立网关层
- Stage 33 修 P0 + BFF 退化为纯聚合层

ADR 决策 10 已撤回（详见 `architecture-decisions.md`），新增决策 11/12/13 描述目标架构与演进路线。

### 2026-08-31 原结论（已撤回，仅作历史参考）

| 维度 | 原判断 | 撤回理由 |
|------|--------|----------|
| **服务注册中心** | ❌ 不引入 | 原判断基于 dev 现状；项目设计目标是分布式架构，必须演进引入 |
| **服务寻址** | ✅ 静态 DNS | 短期可用；多副本 + 配置热更新后必须替换 |
| **配置中心** | ❌ 不引入 | 当前仅 dev/prod 两环境；但 feature flag / 限流阈值动态化是长期需求 |

**触发条件**（原 2026-08-31）已全部命中：分布式项目定位 + 治理能力叙事 + P0 修复需要独立治理层。

---

## 八、修复行动清单（按优先级）

> 每项遵循 AGENTS.md：先写失败测试（RED）→ 最小实现（GREEN）→ 重构。commit 用 `feat:/fix:/test:` 前缀。

**P0 — 功能/安全（建议 1-2 周）**

- [ ] **R-1** 修复 SSE 协议不匹配：统一 OpenAI 兼容格式解析，e2e 覆盖 `onDelta/onFinish`（对应 A-1）
- [ ] **R-2** 恢复聊天写路径：消息回写 chat-svc + `messageId` 契约，让 Kafka 管线有数据（对应 A-1）
- [ ] **R-3** 实现 JWT 验签（shared 新中间件 + BFF + 5 svc 统一替换），补真实登录校验（对应 S-1）

> **2026-09 修订**：R-3 实际归属调整为 **Stage 32 + Stage 33**：
> - **Stage 32**：APISIX `jwt-auth` 插件统一验签（替换 shared `jwt_auth.go` 的"信任 APISIX 已验过"注释，改为"信任 APISIX 注入 X-User-Id"）；
> - **Stage 33 R-3**：BFF 登录端点（`auth_handler.go`）从 mock 改造为真实校验密码哈希 + 验证码限流。
> 详见 `stage-32-apisix-reintroduction.md` 与 `stage-33-p0-fix-bff-purify.md`。
- [ ] **R-4** 收紧端口暴露（仅 3000/8894 映射宿主）+ 基础设施最简防护 + 统一 mTLS 配置（对应 S-2）

**P1 — 可靠性（建议 3-4 周）**

- [ ] **R-5** 引入数据库迁移工具并收口全部 DDL（对应 D-1）
- [ ] **R-6** 接通 analytics DLQ、outbox 封顶重试 + 告警、producer 按会话分区（对应 K-1）
- [ ] **R-7** 消息写入幂等（客户端 UUID + 唯一约束）（对应 I-1）
- [ ] **R-8** 明确 llm-service 定位并消除双算路径（对应 C-1）

**P1 — 工程（建议 4 周内）**

- [ ] **R-9** 落地最小 CI（go test/vet + vitest + helm lint），先于一切新功能（对应 E-1）
- [ ] **R-10** 补前端核心链路测试：`useAIStreamHandler` + `useConversationSender`（对应 T-1）
- [ ] **R-11** ADR 同步机制固化：架构决策变更必须改 ADR，PR 检查项（对应 E-2）

**P2 — 运维（随手清）**

- [ ] **R-12** web-bff/web/xtts 补 K8s 探针（O-1）；web-bff 开启 SkyWalking（O-2）
- [ ] **R-13** 清理 `.env:9080` 残留（O-4）、`nuxt.config.ts` 默认地址（O-5）、仓库杂物（O-6）

---

> 变更完成时间：2026-08-31  
> 对应文档：本审计报告 + `architecture-decisions.md` 同步修订（决策 9/10）+ `distributed-roadmap.md` 3.2 评审标注
