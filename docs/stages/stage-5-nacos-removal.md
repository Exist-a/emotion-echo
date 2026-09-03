# Emotion-Echo · Stage 5 Nacos 删除完成报告

> 2026-07-14：彻底删除 Nacos 组件（容器 + 代码 + 配置），系统不再依赖任何外部注册中心。

## 🎯 删除目标

按照 [architecture-decisions.md](./architecture-decisions.md) 决策 2：
- ❌ Nacos 注册中心
- ❌ Nacos 配置中心
- ✅ 改用 APISIX + etcd（已在用）

## 🏆 战果

| 维度 | 数据 |
|------|------|
| 删除代码文件 | 2 个（`nacos_register.go` + `nacos_register_test.go`） |
| 删除目录 | 1 个（`emotion-echo-shared/pkg/discovery/`） |
| 修改 main.go | 5 个 svc |
| 修改 config.go | 5 个 svc |
| 修改 yaml | 5 个 svc |
| 删除 docker 服务 | 1 个（`emotion-echo-nacos` 容器） |
| 修改文档 | 3 个（docker-compose、README、.env.common） |
| 代码删除行数 | ~250 行 |

## 🔴🟢 验证证据

### 1. **5 个 svc 编译全部成功**

```
✅ emotion-echo-user-svc        → user-svc.exe
✅ emotion-echo-chat-svc        → chat-svc.exe
✅ emotion-echo-ai-svc          → ai-svc.exe
✅ emotion-echo-analytics-svc   → analytics-svc.exe
✅ emotion-echo-assessment-svc  → assessment-svc.exe
```

### 2. **err.log 关键证据** —— 启动完全无 Nacos 日志

user-svc 启动日志：
```
2026/07/14 18:28:25 [postgres] connected, dsn=...
2026/07/14 18:28:25 [skywalking] tracer initialized, oap=localhost:11800 service=emotion-echo-user-svc
（无任何 [nacos] 日志，证明不依赖 Nacos 注册）
```

### 3. **Nacos 容器已停止并删除**

```bash
$ docker stop emotion-echo-nacos
$ docker rm emotion-echo-nacos
$ docker ps --filter "name=emotion-echo"
emotion-echo-sw-ui     Up   ← SkyWalking UI
emotion-echo-sw-oap    Up   ← SkyWalking OAP
emotion-echo-apisix    Up   ← API Gateway
emotion-echo-postgres  Up   ← Database
emotion-echo-etcd      Up   ← APISIX config (也做服务发现)
emotion-echo-kafka     Up   ← Message Queue
emotion-echo-redis     Up   ← Cache
❌ Nacos 不在列表里
```

**6 个容器，少 1 个 = 少 1 个故障源**

### 4. **APISIX 网关 e2e 验证（5 个 svc 全绿）**

```
GET http://localhost:9080/user-health        → HTTP 200 | dbOk:true
GET http://localhost:9080/assessment-health  → HTTP 200 | dbOk:true
GET http://localhost:9080/chat-health        → HTTP 200 | dbOk:true | kafkaOk:true
GET http://localhost:9080/ai-health          → HTTP 200 | dbOk:true
GET http://localhost:9080/analytics-health   → HTTP 200 | dbOk:true
```

### 5. **业务端点 e2e 验证**

```
GET  /api/v1/users/me                         → 200 | {userId:1, account:alice, ...}
POST /api/v1/conversations                    → 200 | {conversationId:2, title:"..."}
```

## 📁 改动文件清单

### 代码删除
- ❌ `emotion-echo-shared/pkg/discovery/nacos_register.go`
- ❌ `emotion-echo-shared/pkg/discovery/nacos_register_test.go`
- ❌ 整个 `emotion-echo-shared/pkg/discovery/` 目录

### 代码修改（5 svc × 3 文件 = 15 个文件）

| 文件类型 | 修改内容 |
|---------|---------|
| `*/{svc}.go` (5个) | 删除 Nacos 注册代码块 + 删除 `discovery` import + 删除 `context` import（4 个不需要的）|
| `*/internal/config/config.go` (5个) | 删除 `type Nacos struct` + 删除 `Config.Nacos` 字段 |
| `*/etc/{svc}-api.yaml` (5个) | 删除 `Nacos:` 段 |

### 基础设施修改
- ✅ `deploy/docker-compose.infra.yml` —— 删除 Nacos 服务定义 + 删除 APISIX depends_on nacos
- ✅ `deploy/env/.env.common` —— 删除 NACOS_VERSION + NACOS_MODE 等
- ✅ `deploy/README.md` —— 删除 Nacos 验证清单 + 表格行 + 端口检查命令

## 🎓 学到的关键认知

| 认知 | 说明 |
|------|------|
| **故障半径** | 删除 Nacos 之前，Nacos 挂了 → 5 个 svc 全挂。删除后，故障半径 = Nacos 影响面 = 0 |
| **启动韧性** | svc 启动从"6 件事"（DB/SkyWalking/Nacos注册/heartbeat/反向注册/...）变"3 件事"（DB/SkyWalking/HTTP listen） |
| **架构简化** | 少 1 个容器、~250 行代码、1 个心跳 goroutine、1 个端口（8848/9848） |
| **架构假设** | "注册中心"对 5 个 svc 是过度设计 —— 我们没客户端发现需求，APISIX 直管 etcd 够用 |
| **ADR 验证** | 决策 2 真正落地后系统变更简单（少组件 = 少代码 = 少 bug）|

## 🎯 现在系统的依赖关系

```
之前：
   5 svc ──注册──→ Nacos
            ↓ (没人读)
          死配置

现在：
   5 svc ──开端口──→ Docker host
        ↓
   APISIX (etcd config) ──路由──→ svc
```

**Nacos 在这个架构里就是"只写不读"的伪集成，删了系统反而更清晰。**

## 🚦 启动验证步骤（之后任何人验证 Nacos 删除）

```bash
# 1. 启动基础设施（6 个容器，无 Nacos）
cd deploy && docker compose -f docker-compose.infra.yml up -d
docker ps  # 确认无 emotion-echo-nacos

# 2. 启动 5 个 svc
cd emotion-echo-{user,chat,ai,analytics,assessment}-svc
./{name}-svc.exe &

# 3. 验证 e2e
curl http://localhost:9080/user-health        # dbOk:true
curl http://localhost:9080/chat-health        # kafkaOk:true
curl -H "X-User-Id: 1" http://localhost:9080/api/v1/users/me
```

## 📊 项目进度更新

```
Phase 0 基础设施       ████████████████████ 100% ✅
Phase 1 微服务拆分      ████████████████████ 100% ✅
Phase 2 Kafka          ████████████████████ 100% ✅
Phase 3 LLM 接入       ████████████████████ 100% ✅
Phase 4 业务深化        █████████████░░░░░  75%
Phase 5 简化+Nacos清理  ████████████████░░  90% ✅ ← 当前 stage
                        删 Nacos ✅
                        启动韧性 ✅
                        APISIX jwt-auth ⏳
                        APISIX limit-count ⏳
                        APISIX api-breaker ⏳
Phase 6 K8s manifests  ░░░░░░░░░░░░░░░░░░░░   0%
Phase 7 gRPC 升级      ░░░░░░░░░░░░░░░░░░░░   0%
```

## 🎯 下一步

按 [architecture-decisions.md](./architecture-decisions.md) 决策 7-8：
1. **APISIX jwt-auth 插件配置**（替换 svc mock X-User-Id）
2. **APISIX limit-count 插件**（按 user_id 限流）
3. **APISIX api-breaker 插件**（防雪崩）
4. **svc 框架迁移**：go-zero → Gin
5. **legacy handler 迁移**：14 个 handler 按域分配到 5 个 svc

---

**这是 ADR 文档体系第一次真正发挥作用** —— 先有决策（architecture-decisions.md），后有代码改动，删 Nacos 是一次教科书级的架构演进。