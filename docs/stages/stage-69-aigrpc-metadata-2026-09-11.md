---
status: landed
stage: 69
date: 2026-09-11
target: ai-svc dev fallback 缺 x-user-id metadata bug（chat-svc→ai-svc UpsertNeutralEmotion Unauthenticated）
related:
  - docs/stages/stage-67-devevent-publisher-2026-09-11.md（§四 登记的最后一项独立 bug）
  - emotion-echo-shared/pkg/grpcinterceptor/auth.go（NewServerUserIDInterceptor）
  - emotion-echo-chat-svc/internal/grpcclient/ai_client_grpc.go（错误注释源头）
  - emotion-echo-chat-svc/internal/logic/sendmessagelogic.go（maybeUpsertNeutralEmotion 调用链）
---

# Stage 69 — ai-svc dev fallback x-user-id metadata bug 修复（2026-09-11）

> **状态**：🟢 **RED → GREEN 全过 · chat-svc v0.1.6 容器化 · docker 端到端验证**
> **触发**：[stage-67 §四 登记的最后一项独立 bug](stage-67-devevent-publisher-2026-09-11.md)（"ai-svc dev fallback 缺 x-user-id metadata"）
> **关联**：Stage 36-A3.2（dev fallback UpsertNeutralEmotion）+ Stage 32 PR-16（X-User-Id 透传拦截器）

---

#### 一、问题（实测 docker logs）

```
$ docker logs emotion-echo-chat-svc 2>&1 | grep "missing x-user-id"
"time":"2026-09-11T11:17:38.211299217+08:00","level":"ERROR",
 "msg":"dev fallback UpsertNeutralEmotion failed",
 "msg_id":11,"event_id":"6de56b8d-4c77-47c5-99c3-fca9d814ffb2",
 "err":"rpc error: code = Unauthenticated desc = missing x-user-id metadata"
```

**根因链**：

1. chat-svc 在 KAFKA_ENABLED=false 时，SendMessageLogic 同步调 `svcCtx.AIClient.UpsertNeutralEmotion` 写 emotion_analysis（Stage 36-A3.2 dev fallback）
2. AIClient 实现 [ai_client_grpc.go:53](emotion-echo-chat-svc/internal/grpcclient/ai_client_grpc.go#L53) 直接传 `ctx` 给 gRPC client
3. ai-svc gRPC server 拦截器 [shared/pkg/grpcinterceptor/auth.go:87 NewServerUserIDInterceptor](emotion-echo-shared/pkg/grpcinterceptor/auth.go#L87) `metadata.FromIncomingContext` 找 `x-user-id` → 没找到 → 返 `codes.Unauthenticated`
4. 修复前注释（错误的）：

> ai_client_grpc.go:9-10：
> "不带 client-side user-id metadata（chat-svc 是 producer 不是 consumer；ai-svc 那边的 x-user-id 拦截器对 producer RPC 不要求；Stage 32 PR-16 注释说明）"

**事实**：任何调 ai-svc gRPC server 的 RPC 都需要 `x-user-id` metadata，**与是不是 consumer / producer 无关**——拦截器一视同仁。

---

#### 二、修复

### 2.1 RED 阶段（commit `a2fa4d2`）

[sendmessagelogic_test.go](emotion-echo-chat-svc/internal/logic/sendmessagelogic_test.go)：
- 改造 `fakeAIClient` 加 `gotCtx` 字段（之前是 `_ context.Context` 占位）
- 加新测试 `TestSendMessageLogic_DevFallback_InjectsXUserIDMetadata`
- 断言 `metadata.FromOutgoingContext(ai.gotCtx)` 能取到 `x-user-id` 且值 = ctx uid

RED 失败原因：`ctx` 没 outgoing metadata → `metadata.FromOutgoingContext` 返 `ok=false`。

### 2.2 GREEN 阶段（commit `85b3aff`）

[sendmessagelogic.go:243](emotion-echo-chat-svc/internal/logic/sendmessagelogic.go#L243)：

```go
// Stage 69：注入 x-user-id metadata 让 ai-svc 拦截器能读到 user_id
outCtx := metadata.AppendToOutgoingContext(l.ctx, "x-user-id", strconv.FormatInt(uid, 10))
if _, err := l.svcCtx.AIClient.UpsertNeutralEmotion(outCtx, req); err != nil {
    slog.ErrorContext(l.ctx, "dev fallback UpsertNeutralEmotion failed", ...)
}
```

**修复对比**：

| 维度 | 修复前 | 修复后 |
|---|---|---|
| `metadata.AppendToOutgoingContext(ctx, "x-user-id", ...)` | ❌ 未调用 | ✅ 调用 |
| ai-svc server 读 incoming metadata | 空 | `x-user-id: <uid>` |
| gRPC status code | `Unauthenticated` | `OK` |
| emotion_analysis 落库 | 0 行 | 1 行（dev fallback） |

### 2.3 测试结果

```
=== RUN   TestSendMessageLogic_DevFallback_InjectsXUserIDMetadata
--- PASS (0.00s)
PASS  ok  emotion-echo-chat-svc/internal/logic  0.567s
```

**chat-svc 全包测试**（无回归）：

```
ok  emotion-echo-chat-svc/internal/events     0.851s
ok  emotion-echo-chat-svc/internal/grpcserver 0.961s
ok  emotion-echo-chat-svc/internal/handler  1.008s
ok  emotion-echo-chat-svc/internal/logic    0.715s
ok  emotion-echo-chat-svc/internal/model    (cached)
ok  emotion-echo-chat-svc/internal/outbox   (cached)
ok  emotion-echo-chat-svc/internal/repository (cached)
```

**0 FAIL**。

---

#### 三、docker 端到端实测（commit `fe577dc`）

### 3.1 rebuild + restart

```
docker build -f emotion-echo-chat-svc/Dockerfile -t emotion-echo/chat-svc:v0.1.6 .
sed -i 's|emotion-echo/chat-svc:v0.1.5|emotion-echo/chat-svc:v0.1.6|' deploy/docker-compose.apps.yml
docker compose ... up -d emotion-echo-chat-svc
```

### 3.2 启动日志确认

```
{"msg":"[events] using DevEventPublisher (KAFKA_ENABLED=false, dev-only path)","svc":"chat-svc"}
{"msg":"[ai-grpc] connected, addr=emotion-echo-ai-svc:8892 (dev fallback enabled)","svc":"chat-svc"}
{"msg":"[outbox] relay started","svc":"chat-svc"}
{"msg":"[outbox-relay] started: interval=1s batchSize=100","svc":"chat-svc"}
```

### 3.3 业务流实测

```bash
TOK=$(curl -s -X POST -H "Content-Type: application/json" \
  -d '{"username":"echo","password":"echo123"}' \
  http://localhost:8894/api/v1/auth/login | jq -r .data.accessToken)

# 创建对话（conv_id=12）
CID=$(curl -s -X POST -H "Content-Type: application/json" -H "X-User-Id: 1" \
  -d '{"title":"Stage 69 E2E"}' \
  http://localhost:8894/api/v1/conversations | jq -r '.data.conversation.id // .data.id')

# 发消息（dev fallback 触发 UpsertNeutralEmotion）
curl -s -X POST -H "Content-Type: application/json" -H "X-User-Id: 1" \
  -d '{"role":"user","content":"今天心情一般"}' \
  "http://localhost:8894/api/v1/conversations/$CID/messages"
# HTTP 200
```

### 3.4 db 落库验证

```sql
SELECT message_id, conversation_id, primary_emotion, model, created_at
  FROM emotion_echo_ai.emotion_analysis WHERE conversation_id = 12
  ORDER BY created_at DESC LIMIT 3;

 message_id | conversation_id | primary_emotion |     model     |          created_at
------------+-----------------+-----------------+---------------+------------------------------
         14 |              12 | neutral         | sync-fallback | 2026-09-11 03:30:03.09934+00
```

**关键证据**：`model='sync-fallback'` —— ai-svc UpsertNeutralEmotion 路径标记，**证明 chat-svc → ai-svc gRPC 真走通了**（修复前是 `codes.Unauthenticated` 拦截，emotion_analysis 无新行）。

### 3.5 错误日志验证

```bash
$ docker logs emotion-echo-chat-svc 2>&1 | grep -c "missing x-user-id"
0
```

**修复后 0 行 missing x-user-id 错误**（修复前每次发消息都报）。

---

#### 四、调研依据

| 项 | 文件 / 命令 |
|---|---|
| 拦截器读 metadata key | | [emotion-echo-shared/pkg/grpcinterceptor/auth.go:87](emotion-echo-shared/pkg/grpcinterceptor/auth.go#L87) `metadata.FromIncomingContext` |
| 错误注释源头 | | [emotion-echo-chat-svc/internal/grpcclient/ai_client_grpc.go:9-10](emotion-echo-chat-svc/internal/grpcclient/ai_client_grpc.go#L9) |
| 调用链 | | [sendmessagelogic.go:227 maybeUpsertNeutralEmotion](emotion-echo-chat-svc/internal/logic/sendmessagelogic.go#L227) |
| 实测错误串 | | docker logs 2026-09-11 11:17:38.211 'missing x-user-id metadata' |
| RED 测试 | | [sendmessagelogic_test.go:TestSendMessageLogic_DevFallback_InjectsXUserIDMetadata](emotion-echo-chat-svc/internal/logic/sendmessagelogic_test.go) |
| GREEN 修复 | | [sendmessagelogic.go:243 metadata.AppendToOutgoingContext](emotion-echo-chat-svc/internal/logic/sendmessagelogic.go#L243) |
| 镜像版本 | | chat-svc v0.1.5 → v0.1.6 |
| 端到端验证 | | docker logs / SQL SELECT emotion_echo_ai.emotion_analysis |

---

#### 五、本批未做（剩余 6 项 — Stage 68 / 67 收口报告已登记）

| # | 项 | 来源 | 工作量 |
|---|---|---|---|
| 1 | Kafka P1：ai-svc consumer 外层 5s 重试 + DLQ 真实 topic | kafka-reliability-gaps.md §1.2/§1.3 | 1~1.5 天 |
| 2 | Kafka P2：consumer lag 监控 + Protobuf 迁移 | kafka-reliability-gaps.md §1.4/§1.5 | 3~4 天 |
| 3 | BFF 路由三方对齐（C8 todo-pile）| todo-pile §C8 | 1.5~2 天 |
| 4 | chat-svc PinConversation / StreamMessages gRPC | 决策 4 ADR §八 | 2 天（业务未触发）|
| 5 | Helm chart 与 compose dev 全面对齐 | todo-pile §D6 | 1 天 |
| 6 | gRPC mTLS（dev insecure → prod mTLS）| 决策 4 ADR §八 | 1 周 |

---

#### 六、commit 时间线

```
fe577dc fix(chat): rebuild chat-svc v0.1.6 + x-user-id metadata 修复在容器生效
85b3aff fix(chat): Stage 69 GREEN dev fallback x-user-id metadata 注入
a2fa4d2 test(chat): Stage 69 RED dev fallback 缺 x-user-id metadata bug
83c5787 docs(stage): Stage 68 收口报告
99d6518 feat(scripts,analytics): Stage 68 dev dashboard seed + SQL 修复
```

---

> 最后更新：2026-09-11 by Stage 69 实施 session
> 关联：stage-67 §四 + Stage 36-A3.2 dev fallback 模式 + Stage 32 PR-16 X-User-Id 拦截器