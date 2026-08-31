# Stage 31 · Nacos 注册中心 + 配置中心落地

> **配套文档**：`adr-2026-09-nacos-reintroduction.md`（决策依据）、`stage-32-apisix-reintroduction.md`（下一步）、`stage-33-p0-fix-bff-purify.md`（最后阶段）。

## 实施状态（2026-09）

| 阶段 | 状态 | 说明 |
|------|------|------|
| 文档收口（PR-01） | ✅ 已完成 | ADR 决策 10/11/12/13 + roadmap + microservices-architecture + 本文档 状态标识 |
| shared 库（PR-02..06） | ☐ 未启动 | discovery / configcenter interface + Nacos 实现 + testcontainer 集成测 |
| svc 接入（PR-07..10） | ☐ 未启动 | 6 Go svc + 1 Python svc 主动注册 + 运营参数拉取 |
| compose + Helm（PR-11..12） | ☐ 未启动 | nacos v2.4.3 standalone + Helm subchart (StatefulSet + emptyDir) |

**PR 拆分详情**：见 [Stage 31 实施计划](#)（共 12 个 PR，每 PR 独立可合并，TDD 强约束）。

---

## 一、目标

1. 启动 docker-compose 服务（standalone，单机即可）；
2. **7 个 svc** 启动时**主动注册**到 Nacos，每 5s 心跳：
   - 6 个 Go svc（user / chat / assessment / analytics / ai-svc / **web-bff**）
   - 1 个 Python svc（emotion-llm-service，**BFF 也参与服务发现**，与 Stage 32 APISIX `nacos-discovery` 上游配套）；
   - FER / sensevoice / XTTS 在 `profile:ai` 启动时同样接入；
3. **配置中心仅放运营参数**——feature flag、限流阈值、模型路由表、Kafka 重试次数、A/B 分组等。**`etc/*.yaml` 仍是启动默认值**，Nacos 在启动后覆盖；**JWT secret、DATABASE_DSN 等敏感配置不进 Nacos**；
4. **验收**：Nacos console (`localhost:8848/nacos`，默认账号 `nacos/nacos`) 可见 7+ 个健康实例；改 Nacos 中某运营参数后 svc 在 30s 内重载。

---

## 二、改动清单

### 2.1 文档

| 文件 | 类型 | 说明 |
|------|------|------|
| `docs/architecture-decisions.md` | 改 | 决策 10/11/12/13（已合入） |
| `docs/adr-2026-09-nacos-reintroduction.md` | 新建 | 决策论证（已合入） |
| `docs/stage-31-nacos-reintroduction.md` | 新建 | 本文档 |
| `docs/distributed-roadmap.md` | 改 | Stage 3.2 评审撤回；插入 Stage 31/32/33 占位 |
| `docs/microservices-architecture.md` | 改 | 架构图更新（Nacos 出现） |

### 2.2 shared 库

| 文件 | 类型 | 行数估算 | 说明 |
|------|------|----------|------|
| `emotion-echo-shared/pkg/discovery/registry.go` | 新建 | ~80 | `Registry` interface：`Register/Unregister/Discover/Subscribe` |
| `emotion-echo-shared/pkg/discovery/nacos_register.go` | 新建 | ~150 | nacos-sdk-go/v2 v2.3.5+ 实现，含 5s 心跳 goroutine、3 次失败自动注销、优雅退出（监听 SIGINT/SIGTERM） |
| `emotion-echo-shared/pkg/discovery/nacos_register_test.go` | 新建 | ~100 | testify mock + 启动 nacos testcontainer 集成测试（`//go:build integration`） |
| `emotion-echo-shared/pkg/configcenter/config_center.go` | 新建 | ~60 | `ConfigCenter` interface：`GetConfig/ListenConfig/PublishConfig`（**范围仅运营参数**） |
| `emotion-echo-shared/pkg/configcenter/nacos_config.go` | 新建 | ~120 | Nacos 实现，含 `ListenConfig` callback 钩子；**不存放 JWT secret / DSN 等敏感配置** |

### 2.3 Go svc 接入（5 个 + web-bff，每个 ~40 行）

每个 svc 的 `main.go` 增加：
- 启动后调 `discovery.Register(ctx, svcName, host, port, metadata)` 同步等待 Nacos 返回 OK；
- 启动 `configcenter.GetConfig(svcName+".ops.yaml")` 拉取运营参数覆盖默认值；
- 注册 `ListenConfig` callback 写日志（dev 默认关闭热重载，prod 启用）；
- 优雅退出调 `Unregister` + 关闭 configcenter。

涉及的 svc：

| svc | 注册名（service_name） | 运营参数 dataId | main.go 改动点 |
|-----|------------------------|------------------|----------------|
| user-svc | `user-svc` | `user-svc.ops.yaml` | `main.go` 启动注册段 |
| chat-svc | `chat-svc` | `chat-svc.ops.yaml` | 同上 |
| assessment-svc | `assessment-svc` | `assessment-svc.ops.yaml` | 同上 |
| analytics-svc | `analytics-svc` | `analytics-svc.ops.yaml` | 同上 |
| ai-svc | `ai-svc` | `ai-svc.ops.yaml` | 同上 |
| **web-bff** | **`web-bff`** | `web-bff.ops.yaml` | **同上（BFF 也参与服务发现，供 Stage 32 APISIX `nacos-discovery` 上游拉取）** |

**TDD 约束**：每个 svc 必须先有"启动时注册到 Nacos"的失败测试（用 nacos testcontainer 或 mock Registry interface），再写实现。AGENTS.md 强制。

### 2.4 Python svc 接入

`emotion-llm-service/grpc_server.py` 启动钩子：
- 用 `nacos-sdk-python` ≥ 3.1.0；
- FastAPI `@app.on_event("startup")` 注册、`@app.on_event("shutdown")` 注销；
- 配置项从 Nacos 拉取运营参数（如 `MODEL_VERSION`、`KAFKA_RETRIES`）热更新；
- 单测：mock nacos client 验证注册流程；集成测试（`//go:build integration` 思路对应 `pytest -m integration`）。

FER/sensevoice/XTTS 在 `compose.apps.yml` 启动时同样接入（profile:ai 下）。

### 2.5 compose 编排（`deploy/docker-compose.infra.yml`）

新增 nacos service：

```yaml
nacos:
  image: nacos/nacos-server:v2.4.3
  container_name: emotion-echo-nacos
  environment:
    - MODE=standalone
    - JVM_XMS=256m
    - JVM_XMX=512m
  ports:
    - "8848:8848"   # HTTP console
    - "9848:9848"   # gRPC client
    - "9849:9849"   # gRPC server
  networks: [app-network]
  healthcheck:
    test: ["CMD", "curl", "-f", "http://localhost:8848/nacos/actuator/health"]
    interval: 10s
    timeout: 3s
    retries: 10
```

`apps.yml` 各 svc 增加 `depends_on: nacos: { condition: service_healthy }`。

### 2.6 Helm chart

`charts/emotion-echo/charts/nacos/` 新建 subchart：
- `deployment.yaml`（StatefulSet，单实例，emptyDir 持久化）
- `service.yaml`（ClusterIP 8848/9848/9849）
- `configmap.yaml`（启动配置）
- `values.yaml`（`enabled: true` 默认）

`charts/emotion-echo/values.yaml` 加入 nacos subchart 依赖；`Chart.yaml` 添加 chart 依赖。

### 2.7 配套脚本

- `scripts/bootstrap_nacos.sh`：本地首次启动时调用 Nacos OpenAPI 推送 namespace `emotion-echo-dev` 与每个 service 的初始 dataId（**仅运营参数**，feature flag / 限流阈值 / 模型路由表）；
- `scripts/list_nacos_instances.sh`：列注册实例（替代手动登录 console）；
- `scripts/watch_config.sh`：监听某 service 配置变更（`tail -f` Nacos 推送日志）；
- `scripts/push_ops_config.sh`：手动推送某运营参数（CI 或运维用）。

---

## 三、namespace / group / dataId 约定

| service-name | namespace | group | 注册 dataId | 运营参数 dataId |
|--------------|-----------|-------|--------------|------------------|
| user-svc | emotion-echo-dev | DEFAULT_GROUP | `user-svc` | `user-svc.ops.yaml` |
| chat-svc | emotion-echo-dev | DEFAULT_GROUP | `chat-svc` | `chat-svc.ops.yaml` |
| assessment-svc | emotion-echo-dev | DEFAULT_GROUP | `assessment-svc` | `assessment-svc.ops.yaml` |
| analytics-svc | emotion-echo-dev | DEFAULT_GROUP | `analytics-svc` | `analytics-svc.ops.yaml` |
| ai-svc | emotion-echo-dev | DEFAULT_GROUP | `ai-svc` | `ai-svc.ops.yaml` |
| **web-bff** | emotion-echo-dev | DEFAULT_GROUP | `web-bff` | `web-bff.ops.yaml` |
| emotion-llm-service | emotion-echo-dev | DEFAULT_GROUP | `emotion-llm-service` | `emotion-llm-service.ops.yaml` |

### 3.1 运营参数示例（`{service}.ops.yaml`）

```yaml
# 仅放运营参数（feature flag / 限流阈值 / 模型路由表）
feature_flags:
  new_chat_ui: false
  emotion_v2_enabled: true
rate_limit:
  user_per_minute: 60
  global_rps: 1000
model_router:
  default_model: deepseek-chat
  fallback_model: mock
kafka:
  retry_max: 3
  retry_backoff_ms: 200
```

**不放进 Nacos**（敏感配置保留在 `etc/*.yaml` 或 env）：`JWT_SECRET`、`DATABASE_DSN`、`KAFKA_BROKERS`、`LLM_API_KEY`、`POSTGRES_PASSWORD`。

---

## 四、验证（按 AGENTS.md TDD）

### 4.1 单元测试（CI 必跑）

```bash
go test ./emotion-echo-shared/...  # Registry/ConfigCenter interface + Nacos 实现 mock 测
go test ./emotion-echo-user-svc/... -tags=integration  # testcontainers 起 Nacos
go test ./emotion-echo-chat-svc/... -tags=integration
# ... 其余 4 svc 同
pytest emotion-llm-service/tests/ -m integration  # pytest-nacos testcontainer
```

### 4.2 compose 端到端

```bash
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d --build
# 1. 等待 nacos healthcheck 通过
docker compose ps | grep nacos
# 2. 列出注册实例
./scripts/list_nacos_instances.sh
# 期望看到 7 个健康实例（5 Go svc + 1 BFF + 1 emotion-llm-service）
# 3. 浏览器登录 Nacos console http://localhost:8848/nacos (nacos/nacos)
# 服务管理 → 服务列表 → 应看到全部 service-name 且 health=true
# 4. 配置热更新验证：修改 chat-svc.yaml 中某项 → 30s 内 chat-svc 日志打印 "config reloaded"
```

### 4.3 端到端冒烟（Stage 33 之前可用性）

登录 → 进聊天页 → 发消息 → 验证 AI 回复（不依赖落库，链路仅到 SSE 流）—— **A-1 P0 仍在，A-1 SSE 协议修复在 Stage 33**。Stage 31 仅证明"治理层就绪，业务功能未修复"。

---

## 五、风险与缓解

| 风险 | 缓解 |
|------|------|
| Nacos 单节点脑裂 | 单机足够；prod 路线 3 节点（Stage 34+） |
| shared 抽象层过度设计 | interface 仅暴露 4 个方法（Register/Unregister/Discover/Subscribe + GetConfig/ListenConfig/PublishConfig），与 SDK 1:1 |
| 各 svc 启动时 Nacos 未就绪 | shared `WaitForNacos` 指数退避（最长 60s），客户端不依赖 compose depends_on |
| Python 3.0.x SDK 缺陷 | 文档与 requirements.txt 锁定 ≥ 3.1.0 |
| Helm subchart 部署顺序 | values.yaml 显式声明 nacos 在所有业务 svc 之前 |

---

## 六、不做的事

- 不动 Kafka / Postgres / SkyWalking / Redis
- 不动前端
- 不引入新监控
- 不修 P0（A-1/S-1/S-2 留到 Stage 33）
- 不引入 APISIX（Stage 32）

---

> 阶段计划完成时间：2026-09 启动  
> 预计 PR 数：~6（shared 库 2 + 5 svc 接入 4 + Python svc 1）  
> 收口条件：compose 启动后 Nacos console 可见全部注册实例 + 端到端冒烟通过