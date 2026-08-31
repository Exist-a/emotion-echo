# Stage 30-B：全链路容器化修复（Phase B 文档落地）

> **对应 commit 范围**（按时间顺序，2026-08-30）：
> - `e33ac31` feat(compose): infra 网络修复 + 前端 + BFF 全链路编排 + TTS 走 BFF
> - `199fe0b` fix(compose): analytics 容器内 8893 + ai-api.yaml MaxRetries 数字 + alpine 3.19
> - `47dc2f3` fix(user/assessment-svc): applyEnvOverrides — env 覆盖占位符
> - `3e40c15` fix(ai-svc): Kafka.Topics tag 缺闭合引号 → topics=[] 消费不了

> **背景**：Stage 30 T1-T7 完成 BFF 落地后，需要把整套业务容器编排起来做浏览器测试。Phase B 修复了 6 个容器化阻塞问题（infra 网络、env 占位符、kafka advertised、alpine 3.20 拉取受限、Topics tag 语法、analytics 端口错位），最终跑通 14 容器。

---

## 一、最终容器编排（修复后）

```bash
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d --build
```

| 类别 | 服务 | 容器内端口 → 宿主端口 |
|------|------|------------------------|
| 数据 | postgres | 5432 → 5432 |
| 数据 | redis | 6379 → 6379 |
| 消息 | kafka (KRaft) | 9092 → 9092 |
| 观测 | skywalking-oap | 11800/12800 → 11800/12800 |
| 观测 | skywalking-ui | 8080 → 18080 |
| 业务 Go | user-svc | 8888 → 8888 |
| 业务 Go | chat-svc | 8890 → 8890 |
| 业务 Go | assessment-svc | 8889 → 8889 |
| 业务 Go | analytics-svc | 8893 → 8904 |
| 业务 Go | ai-svc | 8891/8892 → 8891/8892 |
| 业务 Go | web-bff | 8894 → 8894 |
| 业务 Python | llm-service | 8000/50051 → 8000/50051 |
| AI 可选 | fer / sensevoice / xtts | 8004/8002/8003 |

> 注：Stage 30 E 阶段后，**APISIX + etcd 已退役**（见 `stage-30-apisix-retirement.md`）。

---

## 二、6 个修复与根因

### 2.1 infra 网络修复（`e33ac31`）

**症状**：业务容器起后连不上 postgres / redis / kafka。

**根因**：`docker-compose.infra.yml` 中 postgres / redis / kafka / skywalking-ui 没有声明 `networks: [app-network]`，导致它们落在 compose 默认网络，业务 svc 在 `emotion-echo_app-network` 解析不到容器 DNS。

**修复**：
```yaml
postgres:
  # ...
  networks:
    - app-network
```
（同样加到 redis / kafka / skywalking-ui）

**附加**：kafka `KAFKA_ADVERTISED_LISTENERS` 从 `localhost:9092` 改为 `emotion-echo-kafka:9092`（容器内消费者需要用容器 DNS）。

### 2.2 APISIX 3.20 alpine 拉取受限（`199fe0b`）

**症状**：`FROM alpine:3.20` build 时 `pull access denied`（Docker Hub 限流 / 网络问题）。

**修复**：6 个 Dockerfile `alpine:3.20` → `alpine:3.19`（验证可拉取）。

> 后续 Stage 30 E 已删除 APISIX（`deploy/apisix/` + helm），此修复仅在当时过渡期间需要。

### 2.3 analytics 端口错位（`199fe0b`）

**症状**：BFF 调用 analytics-svc 的 `dial tcp 172.19.0.8:8904: connect: connection refused`。

**根因**：analytics-svc 容器内监听 `8893`（避让 ai-svc 8892），compose `ports: "8904:8893"`。但 BFF env `ANALYTICS_SVC_URL=http://emotion-echo-analytics-svc:8904`——这是**容器间地址**，不能用宿主端口。

**修复**：
```yaml
ANALYTICS_SVC_URL: "http://emotion-echo-analytics-svc:8893"
```

### 2.4 ai-svc MaxRetries 占位符崩溃（`199fe0b`）

**症状**：ai-svc 容器崩溃循环，日志 `error: config file /app/etc/ai-api.yaml, type mismatch for field "Kafka.maxretries"`。

**根因**：`MaxRetries int` 字段 yaml 写 `${KAFKA_MAX_RETRIES:-3}`（go-zero conf 不展开 bash 占位符，把字面字符串赋给 int → type mismatch）。

**修复**：写真实数字 `MaxRetries: 3`，env 覆盖由 main.go applyEnvOverrides 处理。

### 2.5 user/assessment-svc 缺 applyEnvOverrides（`47dc2f3`）

**症状**：容器起后 panic `runtime error: invalid memory address`，`panic recovered` 在 `getmelogic.go:46`。

**根因**：user-svc / assessment-svc 没有 `applyEnvOverrides`，go-zero conf 把 yaml 占位符原样加载。Postgres DSN 是字面量 `${POSTGRES_DSN:-...}` → DB 连不上 → GetMe 查 nil panic。

**修复**：两个 svc 都加 `applyEnvOverrides`（POSTGRES_DSN + SKYWALKING_OAP_ADDR），与 chat-svc / ai-svc 同模式（Stage 22-B 范式）。

```go
func applyEnvOverrides(c *config.Config) {
    if v := os.Getenv("POSTGRES_DSN"); v != "" {
        c.Postgres.DSN = v
    }
    if v := os.Getenv("SKYWALKING_OAP_ADDR"); v != "" {
        c.SkyWalking.OAPAddr = v
    }
}
```

### 2.6 ai-svc Kafka.Topics tag 缺闭合引号（`3e40c15`）

**症状**：ai-svc 启动后 `consumer started: ... topics=[] ... consume err: no topics provided`。Kafka 消费从未注册。

**根因**：`Topics []string` 的 struct tag 被截断——A2 阶段改坏（`json:",default=[\"chat-events\"]` 缺闭合引号 → go-zero 无法初始化默认值 → 空 slice）。

**修复**：
```go
Topics []string `json:",default=[\"chat-events\"]"`  // 补引号
```

---

## 三、前端 Dockerfile npmmirror 修复

`e33ac31` 还修复了前端 `npm ci` 网络超时（容器内 npmjs.org 被墙）：

```dockerfile
RUN npm config set registry https://registry.npmmirror.com \
    && npm ci
```

注：本修复在本地开发场景下未验证（前端用宿主 dev 模式跑），仅在完全容器化场景需要。

---

## 四、TTS 流式端点从直连 XTTS 改走 BFF

`e33ac31` 把 `useTTSPlayer.ts` 的 TTS 流式调用从：

```ts
// 原（直连 XTTS）
await fetch('http://localhost:8003/tts_stream', { ... })
```

改为：

```ts
// 新（走 BFF 聚合层）
const base = useRuntimeConfig().public.API_BASE_URL;
const token = localStorage.getItem('access_token') || sessionStorage.getItem('access_token');
const headers = { 'Content-Type': 'application/json' };
if (token) headers['Authorization'] = `Bearer ${token}`;
await fetch(`${base}/tts/stream`, { headers, ... });
```

BFF `/api/v1/tts/stream` 直连 XTTS 容器内（XTTS 无鉴权，BFF 容器间调用）+ 透传 JWT。前端 SSE 接收路径不变。

---

## 五、验证

| 阶段 | 状态 |
|------|------|
| `go test ./...` 三个 svc 全套测试 | ✅ 全绿 |
| `docker compose config` 语法 | ✅ |
| 容器全部 Up | ✅ |
| BFF `/health` 5 Go svc 全 ok | ✅ |
| 前端 `:3000` HTTP 200 | ✅ |
| 浏览器测试全链路（Phase C） | ✅ |

---

## 六、后续

- Stage 30 E 阶段已删除 APISIX，本文档中提及的"前端直连 4 Go svc"已升级为"前端 → BFF → 5 下游"。
- 容器编排 Phase B 的修复主要在过渡阶段（Stage 26-P 起的多容器集成）有价值；后续维护主要靠 Stage 30 E 后的精简栈。
