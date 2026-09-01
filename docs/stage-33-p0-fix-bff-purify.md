# Stage 33 · P0 修复 + BFF 退化为纯聚合层

> **配套文档**：`architecture-audit-2026-08-31.md`（P0 问题清单）、`stage-31-nacos-reintroduction.md`、`stage-32-apisix-reintroduction.md`。

---

## 一、目标

修复审计 `architecture-audit-2026-08-31.md` 暴露的 4 个 P0 问题，让"主聊天功能真正可用"，同时把 BFF 改造为**纯聚合层**（决策 12）。

| 编号 | P0 问题 | 修复方式 |
|------|---------|----------|
| **A-1** | 主聊天链路：消息不落库 + SSE 协议不匹配 | R-1 + R-2 |
| **S-1** | JWT 不验签 | Stage 32 已修（APISIX jwt-auth），本 Stage 改 BFF mock 登录为真实登录 |
| **S-2** | 端口全暴露 + 基础设施零防护 | R-4 |

收口：BFF 仅做"多服务聚合 + SSE 流式编排"，不再承担任何网关职责（决策 12）。

---

## 二、R-1 · SSE 协议对齐（修复 A-1 之一）

### 2.1 问题

- BFF 输出 OpenAI 兼容格式：`data: {"choices":[{"delta":{"content":...}}]}` + `data: [DONE]`（`emotion-echo-web-bff/internal/handler/ai_stream_handler.go:95-102,139`）
- 前端按 `data.type === 'start'|'delta'|'finish'` 解析（`Emotion-Echo-Web/app/composables/useAIStreamHandler.ts:119-167`）
- 协议互不认识 → delta 被静默丢弃、`[DONE]` 触发解析错误、`onFinish` 永不触发 → 用户看到永远"streaming"的空气泡

### 2.2 修复决策

**前端按 OpenAI 兼容格式解析**（BFF 保持 OpenAI 格式不变）。理由：
- BFF 与未来 DeepSeek / OpenAI 兼容客户端格式一致，扩展性强；
- 前端改造成本可控（30 行）；
- 避免双向改动导致协议再次走偏。

### 2.3 改动

| 文件 | 改动 |
|------|------|
| `Emotion-Echo-Web/app/composables/useAIStreamHandler.ts` | 替换 `switch (data.type)` 为 OpenAI 格式解析：`choices?.[0]?.delta?.content`；`[DONE]` 触发 `onFinish` |
| `Emotion-Echo-Web/app/composables/useAIStreamHandler.test.ts` | **新建**（TDD）：mock fetch 返回 OpenAI 格式 SSE，断言 `onDelta/onFinish` 被正确调用 |
| `Emotion-Echo-Web/app/composables/useConversationSender.ts` | `onStart` 回调适配（移除 `data.conversationId` / `data.userMessageId` 字段——OpenAI 格式没有，由 R-2 在 stream 前返回） |

### 2.4 验证

- `npm run test -- useAIStreamHandler.test.ts` 通过；
- 浏览器手测：发消息 → 1-2s 内开始显示 AI 字（delta 触发）→ 完整回复（onFinish 触发，气泡状态置 sent）。

---

## 三、R-2 · 恢复聊天写路径（修复 A-1 之二）

### 3.1 问题

前端主链路 `[id].vue:155` → `useConversationSender.sendToExistingConversation` → `sendAIStream → POST /api/v1/ai/stream`，BFF 该端点只流式回复，不调 chat-svc 写库。唯一会落库的 `messageStore.sendMessage`（`stores/message.ts:73`）无任何调用方。结果：
- 消息与 AI 回复只存在前端内存，刷新即丢；
- `message.created` 事件不产生 → ai-svc 情绪分析、analytics-svc 行为事件**整条 Kafka 管线悬空**。

### 3.2 修复设计

**写库前移到 stream 调用前**：

```
前端 sendToExistingConversation(conversationId, content):
  1. POST /api/v1/chat/conversations/:id/messages   ← 落库 + Kafka outbox
       body: { content, role: 'user', client_msg_id: <uuid> }
       resp: { messageId, conversationId, status: 'persisted' }
  2. POST /api/v1/ai/stream                         ← SSE 流式回复
       body: { message: content, conversationId, messageId, ... }
       resp: SSE delta/finish；finish 事件含 ai_message_id
  3. 前端拿 ai_message_id 落到 messageStore
```

**幂等保护**：`client_msg_id` 客户端生成 UUID，chat-svc `messages` 表加 `client_msg_id UNIQUE` 约束（Stage 33 顺手做 I-1 部分）。

### 3.3 改动

| 文件 | 改动 |
|------|------|
| `Emotion-Echo-Web/app/composables/useConversationSender.ts` | `sendToExistingConversation` 流程改为：先 `await messageStore.sendMessage(...)`（落库）→ 再 `sendAIStream(...)`；`tempUserMessage.id` 用 `client_msg_id` |
| `Emotion-Echo-Web/app/stores/message.ts` | `sendMessage` 流程确保调用（当前未被调用） |
| `emotion-echo-web-bff/internal/handler/ai_stream_handler.go` | 接收 `messageId` 参数，SSE `start` 事件（兼容）携带 `userMessageId` |
| `emotion-echo-chat-svc/internal/logic/sendmessagelogic` | 增加 `client_msg_id` 字段处理 + 唯一约束（`ON CONFLICT DO NOTHING`） |
| `emotion-echo-chat-svc/migrations/002_add_client_msg_id.sql` | **新建**：`ALTER TABLE messages ADD COLUMN client_msg_id UUID UNIQUE` |
| `emotion-echo-chat-svc/internal/logic/sendmessagelogic_test.go` | **新建**（TDD）：相同 `client_msg_id` 重复提交应返回原 messageId 而非新建 |

### 3.4 验证

- `go test ./emotion-echo-chat-svc/... -tags=integration` 通过；
- 端到端：浏览器发消息 → DB `messages` 表有新增行（带 `client_msg_id`）→ Kafka `chat-events` topic 收到事件 → ai-svc 情绪分析落库 → analytics-svc 行为事件落库。

---

## 四、R-3 · JWT 真实登录（修复 S-1 BFF 侧）

### 4.1 问题

BFF `/api/v1/auth/login` 是 mock（`emotion-echo-web-bff/internal/handler/auth_handler.go`：账号密码非空即签发 JWT），`/verification-code` 空转。Stage 32 后 APISIX jwt-auth 已真正验签，但 BFF 仍签发 mock token——任何人可拿任意账号的合法 token。

### 4.2 修复

| 文件 | 改动 |
|------|------|
| `emotion-echo-web-bff/internal/handler/auth_handler.go` | `login` 改为：查 user-svc 校验用户名 + 密码哈希（bcrypt 校验）；`register` 调 user-svc 创建用户 + 密码哈希入库；`verification-code` 加 60s 缓存 + IP 限流（用 shared `pkg/middleware/limiter.go` 内存令牌桶） |
| `emotion-echo-web-bff/internal/handler/auth_handler_test.go` | **新建**（TDD）：错密码 401 / 锁定 5 次 / 正常 200 / 验证码 60s 内不重复 |
| `emotion-echo-web-bff/etc/web-bff.yaml` | 增加 `UserService.BaseURL` 字段（与 APISIX upstream 一致） |

### 4.3 验证

- `go test ./emotion-echo-web-bff/internal/handler/...` 通过；
- 端到端：错误密码登录返回 401、验证码 60s 内第二次请求被限流、5 次错密码锁定账号。

---

## 五、R-4 · 收紧端口暴露（修复 S-2）

### 5.1 问题

compose 把全部 11 个业务 svc + 4 个中间件映射宿主端口，叠加无鉴权 → 局域网内任意直连。

### 5.2 修复

| 文件 | 改动 |
|------|------|
| `deploy/docker-compose.apps.yml` | **仅保留** `web:3000` + `apisix:9080`（如果用 compose 直暴露）或 `web-bff:8894`（保留 dev 临时直连）+ `apisix-dashboard:9000`（仅 dev）映射宿主；其余 5 个 Go svc、4 个 Python svc、llm-service、ai-svc gRPC 全部**移除宿主映射** |
| `deploy/docker-compose.infra.yml` | Postgres / Redis / Kafka / SkyWalking-OAP 全部**移除宿主映射**（dev 用 `docker exec` 或 `kubectl port-forward`）；**Etcd 仅 dev 暴露 2379**（APISIX Admin API 通过 apisix:9180 间接访问） |
| `deploy/docker-compose.infra.yml` Postgres | 密码改 `${POSTGRES_PASSWORD:-}` 强制注入；启用 `sslmode=prefer`（dev 证书由 cert-manager 签发，prod） |
| `deploy/docker-compose.infra.yml` Kafka | 加 `KAFKA_LISTENERS: PLAINTEXT://0.0.0.0:9092,CONTROLLER://0.0.0.0:9093` + `KAFKA_ADVERTISED_LISTENERS: PLAINTEXT://kafka:9092` 强制容器内访问 |
| SkyWalking-OAP/UI | 移除宿主映射；前端通过 OAP `:11800`/`:12800` 服务名访问 |
| `docs/deploy/README.md` | 更新验证步骤（端口列表变化） |

### 5.3 验证

```bash
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d --build
docker compose ps --format "table {{.Names}}\t{{.Ports}}"
# 期望仅 web、web-bff、apisix、apisix-dashboard 有宿主端口映射
```

---

## 六、BFF 退化为纯聚合层（决策 12 收口）

### 6.1 移除项（Stage 32 部分已完成，本 Stage 收口）

| 文件 | 移除内容 |
|------|----------|
| `emotion-echo-web-bff/main.go` | `bffAuthMiddleware` 挂载（已 Stage 32 移除）；`corsMiddleware`（已 Stage 32 移除） |
| `emotion-echo-web-bff/internal/handler/auth_handler.go` | 整个 mock 文件（由 R-3 真实登录替代） |
| `emotion-echo-web-bff/etc/web-bff.yaml` | `Auth.JWTSecret` 字段（APISIX 接管） |
| `emotion-echo-web-bff/internal/config/config.go` | `ApplyEnvOverrides` 中 `BFF_JWT_SECRET` 处理 |
| `deploy/docker-compose.apps.yml` | `BFF_JWT_SECRET` env（由 APISIX secret 注入） |

### 6.2 保留项（BFF 仍负责）

| 职责 | 文件 |
|------|------|
| 多服务聚合 | `internal/downstream/*.go` 7 个 client |
| 字段裁剪 | 各 handler 的 viewmodel 映射 |
| SSE 流式编排 | `internal/handler/ai_stream_handler.go` |
| 多端适配（PC/移动） | `internal/handler/multimodal_handler.go` |
| 业务上下文（会话级） | `internal/session/*` |

### 6.3 shared 调整

`emotion-echo-shared/pkg/middleware/jwt_auth.go`：注释改为"信任 APISIX 注入 X-User-Id（决策 12）"；行为不变（base64 解码 → context user_id）。

### 6.4 验证

- `go test ./emotion-echo-web-bff/...` 通过；
- BFF 启动日志不再打印 "bffAuthMiddleware registered" / "corsMiddleware registered"；
- 端到端：浏览器 → APISIX :9080 → BFF :8894（容器内）→ user-svc :8888（容器内），全程无 Authorization 头被 BFF 处理。

---

## 七、端到端冒烟（Stage 33 收口条件）

`scripts/smoke_stage33.sh` 必须全部通过：

```bash
#!/usr/bin/env bash
set -euo pipefail

# 1. 真实登录
TOKEN=$(curl -s -X POST http://localhost:9080/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"username":"testuser","password":"testpass"}' | jq -r .data.token)
[[ -n "$TOKEN" ]] || { echo "login failed"; exit 1; }

# 2. 伪造 JWT 被 APISIX 拒绝
curl -sf http://localhost:9080/api/v1/user/profile \
  -H "Authorization: Bearer a.eyJ1c2VyX2lkIjoxfQ.b" && {
  echo "forged jwt accepted!"; exit 1; } || echo "ok: forged jwt rejected"

# 3. 发消息 → 落库 → Kafka 事件 → 情绪分析
MSG_ID=$(uuidgen)
curl -sf -X POST http://localhost:9080/api/v1/chat/conversations/c1/messages \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"content\":\"我今天心情不错\",\"client_msg_id\":\"$MSG_ID\"}" | jq -r .data.messageId

# 4. 等 SSE 流完整
timeout 30 curl -sf -X POST http://localhost:9080/api/v1/ai/stream \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"message\":\"我今天心情不错\",\"conversationId\":\"c1\"}" | head -c 200

# 5. 验证落库
sleep 5
docker exec emotion-echo-postgres psql -U postgres -d emotion_echo \
  -c "SELECT count(*) FROM emotion_echo_chat.messages WHERE client_msg_id='$MSG_ID';" | grep -q "1" || {
    echo "message not persisted"; exit 1; }

# 6. 验证情绪分析落库
docker exec emotion-echo-postgres psql -U postgres -d emotion_echo \
  -c "SELECT count(*) FROM emotion_echo_ai.emotion_analysis WHERE event_id IS NOT NULL;" | grep -q "[1-9]" || {
    echo "emotion analysis not persisted"; exit 1; }

# 7. 验证端口暴露收敛
EXPOSED=$(docker compose ps --format "{{.Ports}}" | grep -v "0.0.0.0:0" | wc -l)
[[ "$EXPOSED" -le 5 ]] || { echo "too many exposed ports: $EXPOSED"; exit 1; }

echo "✅ Stage 33 smoke passed"
```

---

## 八、风险与缓解

| 风险 | 缓解 |
|------|------|
| SSE 协议修复引入新 bug | TDD 强制：先测后码；e2e 冒烟必跑 |
| 写路径恢复导致 chat-svc 压力大 | 单实例足够；prod HPA 在 Stage 34+ |
| 端口收紧破坏某些 dev workflow | 文档化"如何用 docker exec 进入容器调试" |
| BFF 改造面大、改动集中 | 每个文件独立 PR；TDD 单元测试先写 |
| 前端 SSE 解析需要重新测 | `useAIStreamHandler.test.ts` 覆盖 OpenAI 格式用例 |
| 用户体验：旧消息丢失 | 历史数据仅前端可见，无 DB 持久化——本次同步写库；无迁移负担 |

---

## 九、不做的事

- 不引入新监控
- 不上 K8s 集群（仍为 docker-compose 本地 + kind K8s 验证）
- 不改 Kafka 管线（Stage 31–33 沿用）
- 不动 emotion-llm-service 关键词器（C-1 双算问题留给 Stage 34+）
- 不修 P1 D-1/K-1/I-1（数据库迁移、Kafka 可靠性、端到端幂等的其他部分）—— Stage 34+ 计划

---

> 阶段计划完成时间：2026-09+（依赖 Stage 31/32）  
> 预计 PR 数：~6（R-1 前端、R-2 后端 + migration、R-4 compose、auth 改造、BFF 收口、smoke 脚本）  
> 收口条件：`scripts/smoke_stage33.sh` 全绿 + BFF 仅承担纯聚合职责 + APISIX dashboard 显示完整路由与插件