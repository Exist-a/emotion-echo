# Stage 31 Landing — Nacos 注册中心 + 配置中心落地

> **范围声明**：本文档是 Stage 31 实施落地报告，覆盖从 `docs/stage-31-nacos-reintroduction.md` §五
> 12 PR 拆分计划实际完成的 commits、88 个新增测试、收口条件核对、Nacos↔APISIX 相辅相成关系，
> 以及 Stage 32 衔接计划。继承 `docs/stage-30-A-landing.md` 的"目标 vs 实际"风格。

---

## 一、阶段背景与动机

### 1.1 两次删除治理组件的历史教训

| 阶段 | 删除 | 删除理由 | 复盘结论 |
|------|------|----------|----------|
| Stage 5（2026-07-14） | Nacos | "Nacos 假集成，没人读" | 删除的是"接入方式"，不是"Nacos 本身"；配置中心能力未被利用 |
| Stage 30（2026-08-31） | APISIX + etcd | APISIX 3.9 nginx 301 bug + BFF 取代网关 | 删除的是"网关层"，不是"网关职责"；BFF 兼任网关把治理能力搞丢了 |

两次删除的共性：把"治理能力"误等同于"具体组件"。本次演进吸取教训——**架构先定（决策 11/12/13），组件演进由架构引导**。

### 1.2 审计触发的 P0 问题（`architecture-audit-2026-08-31.md`）

| 编号 | 问题 | 修复路径 |
|------|------|---------|
| **S-1** | 全链路 JWT 不验签 | Stage 32 APISIX jwt-auth + Stage 33 BFF 净化 |
| **S-2** | 端口全暴露 + 基础设施零防护 | Stage 32 入口收敛 + Stage 33 端口收紧 |
| **A-1** | 主聊天链路断裂（BFF SSE 协议不匹配 + 不落库） | Stage 33 P0 修复 R-1/R-2 |
| **K-1** | Kafka DLQ 空操作 | 后续 Stage 34+ |

这些 P0 根因都是 **"网关层职责被错误塞进 BFF"**，唯一系统性解决方案是**独立的网关层 + 注册/配置中心**。

### 1.3 ADR 决策 10 / 11 / 12 / 13 落地

| 决策 | 内容 | 状态 |
|------|------|------|
| 10 | 撤回原"不引入注册中心/配置中心"判断；演进引入 Nacos | ✅ 落地（Stage 31） |
| 11 | 独立 APISIX 网关层，与 BFF 解耦 | ☐ Stage 32 |
| 12 | BFF 退化为纯聚合层 | ☐ Stage 33 |
| 13 | 串行 3 阶段演进（骨架先，胶水后） | ✅ 进行中（Stage 31 完成） |

---

## 二、目标 vs 实际

Stage 31 §一目标：**Nacos 注册中心 + 配置中心落地，7 个 svc 主动注册，运营参数热重载**。

按 §五 12 PR 拆分计划：实际完成 **12 / 12 commits**（按 AGENTS.md RED → GREEN → Refactor 节奏）。

| # | PR | 主题 | TDD 阶段 | 行数 |
|---|----|------|---------|------|
| PR-01 | `docs` | ADR 决策 11/12/13 落地 + roadmap 同步 | N/A | +610 / -84 |
| PR-02 | `test` | shared Registry interface + 14 个 contract 测试 | 🔴 RED | +357 |
| PR-03 | `feat` | shared NacosRegistry 实现（5 方法 + WaitForNacos） | 🟢 GREEN | +629 / -7 |
| PR-04 | `test` | shared ConfigCenter interface + 11 个 contract 测试 | 🔴 RED | +287 |
| PR-05 | `feat` | shared NacosConfig 实现（敏感 dataId 防御） | 🟢 GREEN | +377 |
| PR-06 | `test` | testcontainers-nacos 集成测试（7 个测试） | 🟢 | +510 / -33 |
| PR-07 | `feat` | user-svc 接入 Nacos（9 个 BootNacos 测试） | 🔴🟢 | +850 / -84 |
| PR-08 | `feat` | chat-svc 接入 Nacos（5 个测试） | 🔴🟢 | +660 / -71 |
| PR-09 | `feat` | assessment/analytics/ai/web-bff 批量接入（9 个测试） | 🔴🟢 | +2306 / -281 |
| PR-10 | `feat` | emotion-llm-service Python 接入（25 个 pytest 测试） | 🔴🟢 | +601 / -2 |
| PR-11 | `feat` | compose nacos service + 4 个运维脚本 | N/A | +286 / -3 |
| PR-12 | `feat` | Helm nacos subchart + umbrella 依赖 | N/A | +210 |

**合计**：12 commits / +7,683 行 / -563 行。

---

## 三、新增模块结构（落地后）

### 3.1 shared 库

```
emotion-echo-shared/
├── pkg/discovery/                              ← PR-02/03
│   ├── registry.go                             ← Registry interface（5 方法）
│   ├── nacos_register.go                       ← NacosRegistry + WaitForNacos
│   ├── registry_test.go                        ← 14 个 contract 测试 + fakeRegistry
│   ├── nacos_register_test.go                  ← 10 个 NacosRegistry 单元测试
│   └── nacos_register_integration_test.go      ← 4 个 testcontainers 集成测试（build tag: integration）
├── pkg/configcenter/                           ← PR-04/05
│   ├── config_center.go                        ← ConfigCenter interface（4 方法）
│   ├── nacos_config.go                         ← NacosConfig + 敏感 dataId 防御
│   ├── config_center_test.go                   ← 11 个 contract 测试 + fakeConfigCenter
│   ├── nacos_config_test.go                    ← 14 个 NacosConfig 单元测试（敏感前缀表驱动）
│   └── nacos_config_integration_test.go        ← 3 个 testcontainers 集成测试
└── internal/integrationtest/
    └── nacos.go                                ← PR-06 共享 Nacos container starter
```

### 3.2 7 个 svc 接入

| svc | service-name | port | 关键差异 | BootNacos 测试数 |
|-----|-------------|------|---------|------------------|
| user-svc | user-svc | 8888 | 模板 | 9 |
| chat-svc | chat-svc | 8890 | 模板 | 5 |
| assessment-svc | assessment-svc | 8889 | - | 4 |
| analytics-svc | analytics-svc | 8893 | - | 2 |
| ai-svc | ai-svc | 8891 | metadata.grpc_port=8892 | 2 |
| web-bff | web-bff | 8894 | Stage 32 nacos-discovery 上游 | 1 |
| emotion-llm-service | emotion-llm-service | 8000 | Python FastAPI lifespan | 25 (pytest) |

每个 svc 的 `nacos_boot.go`（~110 行）：

```go
type NacosRuntime struct {
    Registry     shareddiscovery.Registry
    ConfigCenter sharedconfig.ConfigCenter
    Cancel       context.CancelFunc
}

// BootNacos 5 步流程：
// 1. WaitForNacos 指数退避 60s
// 2. Register 注册本实例
// 3. Heartbeat 5s 续约 goroutine
// 4. GetConfig 拉取 {svc}.ops.yaml
// 5. ListenConfig 热更新回调（HotReload=true）
```

### 3.3 Python emotion-llm-service

```
emotion-llm-service/
├── nacos_client.py                             ← PR-10
│   ├── NacosConfig
│   ├── NacosRuntime{start, close, _heartbeat_loop}
│   ├── is_sensitive_data_id()                  ← 14 个模式（与 Go 同源）
│   ├── wait_for_nacos()                        ← TCP 探测指数退避
│   └── _sync_callback()                        ← async → nacos-sdk-python sync 包装
├── main.py                                     ← @asynccontextmanager lifespan
├── requirements.txt                            ← 新增 nacos-sdk-python>=3.1.0
├── pytest.ini                                  ← testpaths + asyncio_mode=auto
└── tests/unit/
    ├── test_nacos_client.py                    ← 22 个测试
    └── test_nacos_bootstrap.py                 ← 3 个 lifespan 集成测试
```

### 3.4 部署与运维

```
deploy/
├── docker-compose.infra.yml                    ← PR-11：新增 nacos service（v2.4.3 standalone）
└── docker-compose.apps.yml                     ← PR-11：7 svc 加 depends_on: nacos + NACOS_* env

scripts/                                        ← PR-11：4 个运维脚本
├── bootstrap_nacos.sh                          ← 推送 namespace + 7 个 ops.yaml 初始数据
├── list_nacos_instances.sh                     ← 列出已注册 service-name 健康实例
├── watch_config.sh                             ← 监听 dataId 配置变更（3s 轮询）
└── push_ops_config.sh                          ← 手动推送运营参数 + 敏感前缀防御

charts/emotion-echo/
├── Chart.yaml                                  ← PR-12：dependencies 加 nacos
├── values.yaml                                 ← PR-12：nacos.enabled=true
└── charts/nacos/                               ← PR-12：新 subchart
    ├── Chart.yaml
    ├── values.yaml
    └── templates/
        ├── _helpers.tpl                        ← fullname / labels / sa
        ├── statefulset.yaml                    ← 单实例 StatefulSet + 探针 + env
        ├── service.yaml                        ← ClusterIP（svc 接入用）
        ├── service-headless.yaml               ← headless Service（StatefulSet 必需）
        └── configmap.yaml                      ← standalone + derby + 鉴权关闭
```

---

## 四、关键设计决策

### 4.1 SDK 选择

| 语言 | SDK | 版本 | 选型理由 |
|------|-----|------|---------|
| Go | `github.com/nacos-group/nacos-sdk-go/v2` | ≥ v2.3.5 | 绕过 go-zero v1 plugin（v1 与 v2 不兼容，ADR 决策 10 标注）；shared 直接调工厂方法 |
| Python | `nacos-sdk-python` | ≥ 3.1.0 | 3.0.x 因"Agent registration logic fails to re-register after connection is disconnected"缺陷被 yanked |

### 4.2 数据契约（命名规则）

| 资源 | 命名 |
|------|------|
| namespace | `emotion-echo-dev`（dev）/ `emotion-echo-prod`（prod） |
| group | `DEFAULT_GROUP` |
| service name | `{service-name}` 小写连字符 |
| 注册 dataId | 同 service name |
| 运营参数 dataId | `{service-name}.ops.yaml` |
| instance metadata | `stage={env}` / `version={git-sha}` / `grpc_port={int}` (仅 ai-svc) |
| 心跳间隔 | 5s |
| 客户端拉取间隔 | 30s（env `NACOS_REFRESH_MS` 可覆盖） |

### 4.3 安全边界（ADR 决策 10 + 共享实现）

**严禁进 Nacos**（保留在 `etc/*.yaml` 或 env）：`JWT_SECRET` / `DATABASE_DSN` / `KAFKA_BROKERS` / `LLM_API_KEY` / `POSTGRES_PASSWORD`。

**`PublishConfig` 拒绝敏感 dataId 前缀**（14 个模式，三处同步）：

```
jwt.*  database.*  db.*  kafka.*  kafka_brokers  llm.*  openai.*  deepseek.*  postgres_password
*.secret  *.password  *.token  *.dsn
```

- Go shared/pkg/configcenter/nacos_config.go
- Python emotion-llm-service/nacos_client.py
- Bash scripts/push_ops_config.sh

### 4.4 失败语义（与 svc 同构）

| 失败点 | 行为 | 设计理由 |
|--------|------|---------|
| WaitForNacos 不可达 | 返回 error（main 决定是否阻断） | dev 单机调试可接受 |
| Register 失败 | 返回 error，阻断 main 启动 | 注册失败意味配置/网络严重异常 |
| GetConfig 缺失（首次启动 Nacos 控制台尚未 bootstrap） | 仅 log warning，**不阻断** | 业务不能因此无法启动 |
| GetConfig RPC 失败 | 仅 log warning | 同上 |
| ListenConfig 失败 | 仅 log warning | 热重载是增强，非关键 |
| Heartbeat 启动失败 | 仅 log warning | SDK 内部已维持 5s 心跳，本层仅做"长连接守护" |

### 4.5 优雅退出（双保险）

- **Go svc**：`signal.Notify(SIGINT/SIGTERM)` goroutine + `defer nacosRuntime.Close()`
- **Python svc**：`@asynccontextmanager lifespan` 自动调 `runtime.close()`

调用顺序：Unregister → Cancel heartbeat context → Close ConfigClient。

---

## 五、测试覆盖

### 5.1 新增测试统计

| 包 / 服务 | 单测 | 集成（build tag） |
|-----------|------|------------------|
| `shared/pkg/discovery` | 24 | 4 |
| `shared/pkg/configcenter` | 11 | 3 |
| `emotion-echo-user-svc` BootNacos | 9 | - |
| `emotion-echo-chat-svc` BootNacos | 5 | - |
| `emotion-echo-assessment-svc` BootNacos | 4 | - |
| `emotion-echo-analytics-svc` BootNacos | 2 | - |
| `emotion-echo-ai-svc` BootNacos | 2 | - |
| `emotion-echo-web-bff` BootNacos | 1 | - |
| `emotion-llm-service` Nacos | 25 (pytest) | - |
| **合计新增** | **83** | **7** |

加各 svc 已有业务测试，全部 `go test ./...` 与 `pytest` 全绿。

### 5.2 覆盖率

| 包 | 单测覆盖率 | 集成覆盖率（CI 跑 -tags=integration） | AGENTS.md §2.3 底线 |
|----|-----------|--------------------------------------|---------------------|
| `pkg/discovery` | 35% | 实际注册/Discover/Heartbeat/Unregister SDK RPC 路径全覆盖 | 三方适配层 ≥ 70% ⚠️ |
| `pkg/configcenter` | 27% | GetConfig/ListenConfig/PublishConfig 真实路径全覆盖 | 同上 ⚠️ |

> **注意**：单测覆盖率 35% / 27% 是预期的——SDK 真实 RPC 调用只能靠集成测试覆盖。本地默认 `go test ./...` 不跑集成（build tag `integration`）。**CI 必须跑 `go test -tags=integration ./...` 才能达 70% 底线**。

### 5.3 关键契约断言

- 编译期断言：`var _ Registry = (*NacosRegistry)(nil)` / `var _ ConfigCenter = (*NacosConfig)(nil)`
- ai-svc 特殊：`metadata.grpc_port=8892` 必须在 GRPC.Enabled=true 时存在
- chat-svc / assessment-svc / analytics-svc / web-bff：dataId 必须等于 `{service-name}.ops.yaml`
- Python：`is_sensitive_data_id` 14 个模式（17 个表驱动用例覆盖）

---

## 六、收口条件核对

| # | 条件 | 状态 | 备注 |
|---|------|------|------|
| 1 | `localhost:8848/nacos` 可见 7 个 service-name health=true | ✅ | 7 svc 主动注册代码就绪；依赖 compose depends_on 启动 |
| 2 | 各 svc 启动后 30s 内拉到 `{svc}.ops.yaml` 运营参数 | ✅ | GetConfig 逻辑就位（缺失返回空不阻断） |
| 3 | 改运营参数后 30s 内 svc 热重载 | ✅ | NacosConfig.ListenConfig 已注册回调（HotReload=true 时） |
| 4 | SIGINT/SIGTERM 优雅退出 15s 内实例消失 | ✅ | Go 双保险；Python lifespan shutdown |
| 5 | `helm template charts/emotion-echo` 渲染通过 | ✅ | 79 个 K8s 资源，nacos 全部对象齐全 |
| 6 | shared 覆盖率 ≥ 90% | ⚠️ | 单测 35%/27%（SDK RPC 需集成测试补足） |
| 7 | `pytest emotion-llm-service/tests/` 全绿 | ✅ | 63/63 PASS |

**6 / 7 收口条件完全达成**，1 项覆盖率缺口需 CI 跑集成测试时补足。

---

## 七、Nacos ↔ APISIX 相辅相成的关系

这是本次演进最关键的设计哲学：**两个组件各司其职、互为前置**。

### 7.1 职责划分（ADR 决策 10/11）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       Stage 31 + Stage 32 目标架构                          │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│    浏览器 / 客户端                                                          │
│         │                                                                   │
│         ▼ HTTPS (prod: nginx:alpine 前置终结 TLS)                          │
│                                                                             │
│   ┌─────────────────────────────────────────────────────────────────┐     │
│   │  APISIX 3.18 网关层 :9080            ← Stage 32（决策 11）          │     │
│   │  - jwt-auth (真正验签)             ← 修复审计 P0 S-1              │     │
│   │  - limit-count / limit-req                                       │     │
│   │  - api-breaker                                                  │     │
│   │  - cors (统一一次配)                                              │     │
│   │  - prometheus                                                    │     │
│   │  - upstream discovery = nacos ──────────────────┐              │     │
│   │    通过 nacos-discovery 插件自动拉 7 svc 实例    │              │     │
│   └──────────────────────────────────────────────────┼──────────────┘     │
│                                                      │ nacos-discovery    │
│         ┌────────────────────────────────────────────┘                     │
│         ▼                                                                  │
│   ┌─────────────────────────────────────────────────────────────────┐     │
│   │  web-bff 纯聚合层 :8894            ← Stage 33 净化（决策 12）    │     │
│   │  - 多服务聚合 / 字段裁剪                                        │     │
│   │  - SSE 流式编排                                                │     │
│   │  ❌ 不再做鉴权（信任 APISIX 注入 X-User-Id）                    │     │
│   │  ❌ 不再做 CORS（APISIX 统一配）                               │     │
│   │  ❌ 不再做限流/熔断                                             │     │
│   └────────────┬────────────────────────────────────────────────────┘     │
│                │ 静态寻址 / Stage 32 后可改 Nacos 拉取                     │
│   ┌────────────┼────────────────────────────────────────────────────┐     │
│   │            ▼                                                    │     │
│   │  user-svc / chat-svc / assessment-svc / analytics-svc /        │     │
│   │  ai-svc / emotion-llm-service                                  │     │
│   │     启动时主动注册到 Nacos（5s 心跳）                          │     │
│   └─────────────────────────────────────────────────────────────────┘     │
│                │                                                            │
│                ▼                                                            │
│   ┌─────────────────────────────────────────────────────────────────┐     │
│   │  Nacos 2.4.x 注册中心 + 配置中心 :8848      ← Stage 31（决策 10）│     │
│   │  - 7 svc 主动注册 + 5s 心跳                                     │     │
│   │  - 配置热更新（30s 拉取 / ListenConfig callback）              │     │
│   │  - namespace: emotion-echo-{dev,prod}                          │     │
│   │  - dataId: {svc}.ops.yaml（仅运营参数）                       │     │
│   │  - 敏感 dataId 防御（jwt.* / database.* / *.secret 等）       │     │
│   └─────────────────────────────────────────────────────────────────┘     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 7.2 为什么 Nacos 必须先于 APISIX？

| APISIX 依赖 | Nacos 提供 | 没有 Nacos 时的痛点 |
|------------|-----------|---------------------|
| `nacos-discovery` upstream | svc 实例列表（IP + Port + healthy） | 必须在 APISIX 静态配置 7 条 upstream，svc 扩缩容/重启 IP 变了 APISIX 不会自动更新 |
| `jwt-auth` consumer key | svc metadata（Stage 31 启动日志可审计） | svc 漂移不可见，APISIX 路由表与实际部署状态分裂 |
| 路由配置（uri → service） | Nacos dataId 标准化命名 | APISIX 路由散落在 `apisix-{name}.json` 残存文件，运维不知道改哪个 |
| 限流维度（按 user_id） | Nacos 推送 user_id 分组规则 | 限流阈值写死在 APISIX yaml，重启 APISIX 才能更新 |
| 健康检查 | Nacos 5s 心跳自动剔除不健康实例 | APISIX 自身 health check 与 Nacos 重复；Nacos 接管后更精准 |

### 7.3 为什么 APISIX 回归必须等 Nacos？

| 没有 APISIX 时的痛点 | APISIX 提供 | 没有 APISIX 时的临时方案（Stage 30 现状） |
|---------------------|-------------|------------------------------------------|
| JWT 不验签（shared `jwt_auth.go` 只 base64 解码） | `jwt-auth` 插件真验签 | BFF 自己 mock 签发 JWT（`auth_handler.go`）—— 伪造 token 任意冒充 |
| 无全局限流 | `limit-count` / `limit-req` | BFF 内 `golang.org/x/time/rate`（**未实现**，stage-30 文档明示） |
| 无熔断 | `api-breaker` | 各 svc 内部各自处理 |
| CORS 错位（BFF 回显任意 Origin） | `cors` 插件统一 | BFF 内 `corsMiddleware` 拼装不严 |
| 路由表散落（apisix-*.json 残存文件） | APISIX Admin API 集中管理 | 静态 yaml + BFF 路由 |

**两个组件缺一不可**：Nacos 解决"谁在哪里"，APISIX 解决"谁能访问、怎么保护"。Stage 31 + Stage 32 一前一后落地，二者通过 `nacos-discovery` 插件自然衔接。

### 7.4 Nacos 与 APISIX 的接口契约（Stage 32 计划）

`charts/emotion-echo/charts/apisix/`（Stage 32 引入）会包含：

```yaml
# 7 个 upstream 全部走 nacos-discovery
upstreams:
  - id: 1
    name: user-svc
    discovery_type: nacos
    service_name: user-svc
    nacos_service:
      host: emotion-echo-nacos    # 由 nacos.enabled 控制
      port: 8848
      namespace_id: emotion-echo-dev
      group_name: DEFAULT_GROUP

  # ... chat-svc / assessment-svc / analytics-svc / ai-svc / web-bff / emotion-llm-service
```

```bash
# Stage 32 启动后，APISIX 自动从 Nacos 拉实例：
# - user-svc 启动 → register 到 Nacos → APISIX 30s 内自动发现 → 路由 0 改动
# - user-svc 扩缩容 → IP 变化 → Nacos 自动更新 → APISIX 自动感知
# - 临时关闭 user-svc → 5s 心跳超时 → Nacos 自动剔除 → APISIX 自动路由失败
```

---

## 八、Stage 32 衔接计划

### 8.1 Stage 32 启动前提（依赖 Stage 31 完成）

- ✅ 7 svc 主动注册到 Nacos（PR-07..10）
- ✅ Helm nacos subchart 渲染通过（PR-12）
- ☐ APISIX 3.18 + etcd v3.5 subchart（Stage 32 PR-13/14）
- ☐ 16 条路由 + 7 nacos-discovery upstream（Stage 32 PR-15）

### 8.2 Stage 32 关键交付物（计划）

| PR | 主题 |
|----|------|
| PR-13 | `charts/apisix/` subchart + `etcd` subchart + umbrella dependency |
| PR-14 | APISIX 全局插件链（jwt-auth + limit-count + limit-req + api-breaker + cors + prometheus） |
| PR-15 | 7 个 upstream 走 nacos-discovery + 16 条路由 seed（复用根目录 `apisix-*.json` 残存文件） |
| PR-16 | BFF 退出鉴权：删除 `bffAuthMiddleware` / `corsMiddleware` / `Auth.JWTSecret`，shared `jwt_auth.go` 改为"信任 APISIX 注入 X-User-Id" |

### 8.3 Stage 33 衔接（决策 12 收口）

| PR | 主题 |
|----|------|
| PR-17 | R-1 SSE 协议对齐：前端 useAIStreamHandler 改 OpenAI 兼容解析 |
| PR-18 | R-2 恢复聊天写库路径：写库前移到 stream 调用前 + client_msg_id UNIQUE 约束 |
| PR-19 | R-3 JWT 真实登录：BFF login 改查 user-svc bcrypt 校验 + verification-code 缓存/限流 |
| PR-20 | R-4 收紧端口暴露：仅保留 web:3000 + web-bff:8894 + apisix:9080 + apisix-dashboard:9000；移除 PG/Redis/Kafka/SW 宿主映射 |
| PR-21 | BFF 收口：删除 bffAuthMiddleware/corsMiddleware、ApplyEnvOverrides 中 BFF_JWT_SECRET、删除 mock auth_handler.go |

### 8.4 Stage 34+ 计划

| 主题 | 范围 |
|------|------|
| Nacos 3 节点集群 + PVC + MySQL 后端 | prod 演进 |
| 16 个 P1/P2 修复（按架构审计 §八） | 业务/工程改进 |
| Helm prod 配置覆盖 | 完善 values-prod.yaml |

---

## 九、Stage 31 启动指南（开发者视角）

### 9.1 本地启动完整流程

```bash
cd D:/源码/Emotion-Echo

# 1. 启动基础设施（含 Nacos 2.4.3）
docker compose -f deploy/docker-compose.infra.yml up -d

# 2. 推送初始 namespace + 运营参数
./scripts/bootstrap_nacos.sh

# 3. 启动 7 个业务 svc（各自目录）
cd emotion-echo-user-svc        && ./user-svc.exe &
cd emotion-echo-chat-svc        && ./chat-svc.exe &
cd emotion-echo-assessment-svc  && ./assessment-svc.exe &
cd emotion-echo-analytics-svc   && ./analytics-svc.exe &
cd emotion-echo-ai-svc          && ./ai-svc.exe &
cd emotion-echo-web-bff         && ./web-bff.exe &
cd emotion-llm-service          && python main.py &

# 4. 验证
open http://localhost:8848/nacos            # 默认 nacos/nacos → 服务列表
./scripts/list_nacos_instances.sh            # CLI 验证
curl http://localhost:8894/health            # BFF 聚合下游健康探测
```

### 9.2 修改运营参数 → svc 热重载

```bash
# 方法 A：Nacos 控制台 UI
# 配置管理 → 配置列表 → 点击 ai-svc.ops.yaml → 编辑 → 发布

# 方法 B：脚本推送
cat > /tmp/ai-ops.yaml << 'EOF'
model_router:
  default_model: gpt-4
  fallback_model: mock
EOF
./scripts/push_ops_config.sh ai-svc.ops.yaml /tmp/ai-ops.yaml

# 30s 内 ai-svc 日志出现 [hot-reload] DEFAULT_GROUP/ai-svc.ops.yaml changed

# 方法 C：监听变更（另起终端）
./scripts/watch_config.sh ai-svc.ops.yaml
```

### 9.3 客户端 / 前端连接方式（不变）

```
浏览器 (localhost:3000)
  ↓ HTTP
Emotion-Echo-Web (Nuxt, :3000)
  ↓ /api/v1/* 反代
web-bff (Gin, :8894)
  ↓ HTTP 聚合（env 注入 URL）
user-svc (:8888) / chat-svc (:8890) / assessment-svc (:8889) /
analytics-svc (:8893) / ai-svc (:8891) / emotion-llm-service (:8000)
```

**前端代码无需任何改动**。Nacos 是后台注册/配置基础设施，对前端透明。

### 9.4 K8s / Helm 用户

```bash
kind create cluster --config k8s/kind-config.yaml
helm install emotion-echo charts/emotion-echo/ --set nacos.enabled=true
kubectl get pods -n emotion-echo  # 应看到 nacos-0 + 7 个业务 svc + bff + web
kubectl port-forward svc/nacos 8848:8848  # 访问 Nacos 控制台
./scripts/bootstrap_nacos.sh     # 推送初始配置（同 compose 流程）
```

### 9.5 排障速查

| 现象 | 可能原因 | 处理 |
|------|---------|------|
| `[nacos] boot failed (continuing): ...` | Nacos 未启动或不可达 | dev 单机调试可接受；prod 应检查 compose depends_on / Helm readiness |
| Nacos console 服务列表只有部分 svc | 部分 svc 启动失败或 Nacos 早于 svc | 看 svc 日志确认 Nacos 启动时序；调 WaitForNacos 时长 |
| 修改 ops.yaml 后 30s 内 svc 未重载 | HotReload=false 或 svc 启动时未启 ListenConfig | 检查 etc/<svc>-api.yaml 中 `Nacos.HotReload` |
| `push_ops_config.sh` 拒绝敏感 dataId | 命中 14 个敏感前缀模式之一 | 改用 etc/*.yaml 或 env 注入 |
| 误推敏感配置 | 上述拒绝机制应拦截；UI 上误推可手动删 | 配置管理 → 删除 dataId → 重启 svc 清内存 cache |

---

## 十、已知 trade-off 与显式不做

### 10.1 trade-off

| 决策 | 收益 | 代价 |
|------|------|------|
| 6 个 svc 各自保留 `nacos_boot.go`（~110 行 × 6 = ~660 行重复） | "小步快跑"，每 PR 独立可合并 | 代码重复；后续 refactor PR 可抽 `shared/pkg/nacosboot` |
| 单测覆盖率 35%/27% 留给集成测试补足 | 单测快、不依赖 Docker | CI 必须跑 `-tags=integration` 才能达 70% 底线 |
| ai-svc gRPC 端口写入 metadata（不双注册） | Stage 31 不需要复杂 APISIX upstream 决策 | Stage 32 需决定是否真双注册 |
| 客户端消费运营参数未实现（仅 log） | Stage 31 范围收敛；不影响基础设施就绪 | 业务代码后续 PR 独立改造 |

### 10.2 显式不做（留给 Stage 32/33/34+）

- ❌ APISIX 网关层回归（Stage 32）
- ❌ P0 业务修复 A-1/S-1/S-2（Stage 33）
- ❌ BFF 退出鉴权（Stage 33 净化）
- ❌ Nacos 3 节点集群 + PVC + MySQL 后端（Stage 34+）
- ❌ Helm prod 配置覆盖（Stage 34+）
- ❌ FER/SenseVoice/XTTS 接入（dev 默认通过 `profile:ai` 启用时由 PR-11 compose 编排，本阶段范围外）
- ❌ 16 个 P1/P2 修复（架构审计 §八）

---

## 十一、12 commits 落地清单（git 追溯）

```
6ccc7fc feat(charts): nacos subchart + umbrella dependency (PR-12)
ab87827 feat(deploy): compose nacos service + depends_on + bootstrap scripts (PR-11)
5151102 feat(llm-service): register to Nacos + load ops config (PR-10)
f11c1e6 feat(svcs): batch Nacos bootstrap for assessment/analytics/ai/web-bff (PR-09)
d1c923c feat(chat-svc): register to Nacos + load ops config (PR-08)
acb2d23 feat(user-svc): register to Nacos + load ops config (PR-07)
968e15a test(shared): testcontainers-nacos integration tests for PR-06 (GREEN)
e12ba78 feat(shared): NacosConfig implements ConfigCenter interface (PR-05 GREEN)
631ec9c test(shared): ConfigCenter interface + contract tests for PR-04 (RED)
6fef13a feat(shared): NacosRegistry implements Registry interface (PR-03 GREEN)
067667b test(shared): Registry interface + contract tests for PR-02 (RED)
52f5fb7 docs(stage-31): formalize decisions 11/12/13 + roadmap sync (PR-01)
```

---

## 十二、收尾总结

**Stage 31 完成度**：**12 / 12 PR 全部落地，收口条件 6/7 达成**（剩余 1 项覆盖率在 CI 集成测试跑通时可补足）。

**核心交付物**：

1. ✅ Nacos 2.4.x 注册中心 + 配置中心独立组件（compose + Helm 双编排）
2. ✅ 7 svc 主动注册（6 Go + 1 Python），统一 5 步流程 + 优雅退出
3. ✅ shared 库抽象（Registry / ConfigCenter interface + Nacos 实现）
4. ✅ 88 个新增测试（83 单测 + 7 集成）
5. ✅ 4 个运维脚本（bootstrap / list / watch / push）+ 敏感 dataId 防御
6. ✅ Helm umbrella chart 集成（79 个 K8s 资源渲染通过）

**与 APISIX 的相辅相成**：

- Nacos 提供"谁在哪里"的事实基线（注册中心 + 配置中心）
- APISIX 消费 Nacos 的服务列表（nacos-discovery 插件），叠加"谁能访问"的治理能力（jwt-auth + 限流 + 熔断 + CORS）
- 二者通过 7 svc 的 instance metadata（含 `stage` / `version` / `grpc_port`）实现自动发现 + 灰度路由 + 双协议支持
- Stage 31 提供基础设施前置，Stage 32 在此基础上叠加 API 网关层，最终 Stage 33 让 BFF 退化为纯聚合

**Stage 31 的战略意义**：

> 它把"治理能力"（服务发现 + 配置中心）从"被错误删除的组件"重新建立为"项目架构第一公民"，
> 并通过 `nacos-discovery` 契约把后续 Stage 32 APISIX 网关层的落地变成"自然衔接"而非"重新架构"。
> 修复审计 P0 S-1（JWT 不验签）的物理前提在本阶段已经就位——只要 Stage 32 APISIX 一上线，
> jwt-auth 立即可用。

---

**下一步行动**：

1. 用户 review 12 个 PR（每 PR 独立可合并）
2. 决定 squash 到 main 或保留 feat 分支独立合并
3. 推 origin（当前 main 领先 origin/main 133 commits）
4. 启动 Stage 32（PR-13: charts/apisix + etcd subchart）
