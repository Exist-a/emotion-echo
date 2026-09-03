# Stage 20 · 容器化（Dockerfile + docker-compose + 监控 + 可靠性）

**日期**：2026-07-15
**目标**：emotion-llm-service + emotion-echo-ai-svc 两个核心服务支持生产级容器化部署。
**前置依赖**：Stage 11-19（gRPC 双协议 + mTLS + ai-svc gRPC server）。

---

## 一、本阶段总览

本阶段由 8 个子任务组成，端到端验证时发现 P0-1 仅部分完成（yaml 容器内服务发现走的是 env 注入到容器环境变量，**但 ai-svc 内部 ai-api.yaml 里的 `${VAR:-default}` 不会被 go-zero 解析**），已在 Stage 22-B 修复（main.go 启动期 `os.Getenv` 覆盖）。详见 [stage-24](stage-24-endpoint-verification-and-bugfix.md)。

| 子阶段 | 内容 | 状态 |
|--------|------|------|
| Stage 20-0 | Dockerfile + docker-compose（multi-stage + tini + non-root） | ✅ |
| Stage 20-1 | Python SIGTERM graceful shutdown | ✅ |
| Stage 20-2 | 结构化 JSON 日志（Python + Go） | ✅ |
| Stage 20-3 | ai-svc HTTP/gRPC Graceful Shutdown | ✅ |
| Stage 20-4 | ai-svc slog JSON handler | ✅ |
| Stage 20-5 | ai-svc Prometheus `/metrics` | ✅ |
| Stage 20-P0-1 | 容器内服务发现（docker compose env 注入到 ai-svc） | ⚠️ → ✅ | 
| Stage 22-B | env override 修复（go-zero conf 不支持 `${VAR:-default}` 实际在 Stage 24 完成） | ✅ |
| Stage 20-P0-2 | emotion-llm-service `/metrics` | ✅ |
| Stage 20-P0-3 | 启动时 fail-fast（依赖不可用立即退出） | ✅ |
| Stage 20-P0-4 | 资源限制 + 镜像 tag pinned（v0.1.0） | ✅ |

**端到端验证 `scripts/verify_e2e.py` 6/6 PASS**

---

## 二、产出物清单

### Dockerfile / 配置
| 文件 | 作用 |
|---|---|
| `emotion-llm-service/Dockerfile` | Python multi-stage (builder + runtime)，tini PID 1，UID 65532 |
| `emotion-llm-service/entrypoint.sh` | 双进程拉起 + 互相监控（任一退出整体退出） |
| `emotion-llm-service/.dockerignore` | 排除日志/缓存/证书 |
| `emotion-llm-service/logging_setup.py` | JSON / text 格式可选的 Python 日志 |
| `emotion-llm-service/metrics_setup.py` | Prometheus metrics：HTTPRequestsTotal + Duration + ANALYZE_TOTAL + GRPC_REQUESTS_TOTAL |
| `emotion-echo-ai-svc/Dockerfile` | Go multi-stage 静态二进制，alpine + tini + non-root |
| `emotion-echo-ai-svc/.dockerignore` | 排除 exe/log/test/coverage |
| `emotion-echo-ai-svc/etc/ai-api.yaml` | 用 `${VAR:-default}` 支持 env 注入 + BrokersCSV |
| `emotion-echo-ai-svc/internal/bootstrap/deps.go` | 启动期 fail-fast 依赖检查 |
| `emotion-echo-ai-svc/internal/bootstrap/deps_test.go` | 4 unit tests |
| `emotion-echo-ai-svc/internal/logging/` | slog JSON handler |
| `emotion-echo-ai-svc/internal/metrics/` | Prometheus 中间件 + handler |

### Compose
| 文件 | 作用 |
|---|---|
| `deploy/docker-compose.infra.yml` | postgres + redis + kafka + skywalking-oap + apisix，含 `app-network` |
| `deploy/docker-compose.apps.yml` | emotion-llm-service + emotion-echo-ai-svc，含资源限制 + DNS env |

### 验证
| 文件 | 作用 |
|---|---|
| `scripts/verify_e2e.py` | 6 项端到端冒烟测试 |

---

## 三、Dockerfile 设计要点

### emotion-llm-service

| 维度 | 方案 |
|---|---|
| 基础镜像 | `python:3.12-slim`（debian，glibc，tini 用 glibc 编译 OK） |
| 依赖安装 | `pip install --prefix=/install`（独立 layer，易缓存） |
| pip mirror | 清华源 `https://pypi.tuna.tsinghua.edu.cn/simple`（容器内 PyPI 被墙） |
| 双进程 | `entrypoint.sh` 后台拉起 `python main.py` + `python grpc_server.py` |
| 进程监控 | POSIX `kill -0` 轮询（dash 不支持 `wait -n`） |
| PID 1 | `tini v0.19.0`（从 github release 下载到 `/usr/local/bin/tini`） |
| 用户 | UID/GID 65532（非 root） |
| 时区 | `TZ=Asia/Shanghai` env + `/etc/localtime` |
| 端口 | `EXPOSE 8000 50051` |
| 健康检查 | `python -c "import urllib.request; ..."` |

### emotion-echo-ai-svc

| 维度 | 方案 |
|---|---|
| 基础镜像 builder | `golang:1.26-alpine` |
| GOPROXY | `https://goproxy.cn,direct`（容器内 `proxy.golang.org` 被墙） |
| 基础镜像 runtime | `alpine:3.20`（含 ca-certificates + tzdata + wget） |
| tini 来源 | `apk add --no-cache tini`（alpine 官方包，musl 兼容；**不能用 github tini 静态下载，因是 glibc 编译**） |
| 二进制 | `CGO_ENABLED=0 -trimpath -ldflags="-s -w"` |
| 依赖 | build context 必须为仓库根（ai-svc go.mod 引用 `../emotion-echo-shared`） |
| 用户 | UID/GID 65532（非 root） |
| 端口 | `EXPOSE 8891 8892` |
| 健康检查 | `wget --spider http://localhost:8891/health` |

---

## 四、容器内服务发现（Stage 20-P0-1）

**问题**：容器内 `localhost` 指容器自己，Kafka / SkyWalking / LLM 必须用容器 DNS 名。

**方案**：`ai-api.yaml` 用 go-zero conf `${VAR:-default}` 语法，docker-compose `environment:` 注入：

```yaml
# ai-api.yaml
SkyWalking:
  OAPAddr:     ${SKYWALKING_OAP_ADDR:-localhost:11800}
Kafka:
  BrokersCSV:  ${KAFKA_BROKERS:-localhost:9092}
LLM:
  GRPCAddr:    ${LLM_GRPC_ADDR:-localhost:50051}
  BaseURL:     ${LLM_BASE_URL:-http://localhost:8000}
```

```yaml
# deploy/docker-compose.apps.yml
environment:
  POSTGRES_DSN:        "host=emotion-echo-postgres port=5432 ..."
  KAFKA_BROKERS:       emotion-echo-kafka:9092
  SKYWALKING_OAP_ADDR: emotion-echo-sw-oap:11800
  LLM_BASE_URL:        "http://emotion-llm-service:8000"
  LLM_GRPC_ADDR:       emotion-llm-service:50051
```

**关键改造**：
1. `Kafka.Brokers []string` → `BrokersCSV string`（go-zero conf **不支持 list 字段的 env 替换**），main.go 用 `kafkaBrokers()` helper split
2. bool 字段写死 `true`（go-zero conf 解析 `${VAR:-true}` 时默认是 string "true"，与 bool 字段 type mismatch；用 1 又会被解析成 number）
3. `ai-api.yaml` 通过 `volumes:` mount 覆盖镜像内置版本，方便本地修改

**设计取舍**：
- ✅ 本地开发：env 不设 → 用 `localhost` 默认值（向后兼容）
- ✅ 容器内：compose env 注入容器 DNS 名
- ⚠️ `KAFKA_BROKERS` 注入 CSV 字符串（如 `emotion-echo-kafka:9092` 或 `kafka1:9092,kafka2:9092`）

---

## 五、emotion-llm-service /metrics（Stage 20-P0-2）

**新增文件** `emotion-llm-service/metrics_setup.py`：

```python
HTTP_REQUESTS_TOTAL = Counter(
    "llm_http_requests_total",
    "Total number of HTTP requests processed, labeled by method, path, status.",
    ["method", "path", "status"],
)

HTTP_REQUEST_DURATION = Histogram(
    "llm_http_request_duration_seconds",
    "Histogram of HTTP request latency in seconds.",
    ["method", "path"],
    buckets=(0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5),
)

ANALYZE_TOTAL = Counter(
    "llm_analyze_total",
    "Total /analyze invocations, labeled by emotion result and status.",
    ["emotion", "status"],
)

GRPC_REQUESTS_TOTAL = Counter(
    "llm_grpc_requests_total",
    "Total gRPC requests, labeled by method and status.",
    ["method", "status"],
)
```

**集成点**：
- `main.py`：`app.add_middleware(MetricsMiddleware)` + `@app.get("/metrics")` + analyze endpoint 加 counter
- `grpc_server.py`：Analyze / AnalyzeBatch handler 加 counter

**MetricsMiddleware**：纯 ASGI 级别，包装 `send` 捕获 status code，`/metrics` 自循环不计数。

**输出样例**（`curl :8000/metrics`）：
```
llm_http_requests_total{method="GET",path="/health",status="200"} 74
llm_http_requests_total{method="POST",path="/analyze",status="200"} 7
llm_analyze_total{emotion="happy",status="ok"} 7
```

---

## 六、启动时 fail-fast（Stage 20-P0-3）

**新增包** `emotion-echo-ai-svc/internal/bootstrap`：

```go
// deps.go
func ShouldFailFast() bool              // STARTUP_STRICT=true?
func IsRequired(dep string) bool        // dep 在 STARTUP_STRICT_DEPS 列表？
func CheckTCP(ctx, addr, timeout) error // TCP 探活
func CheckMultiple(ctx, addrs, timeout) map[string]error  // 并行
```

**4 unit tests**（PASS）：`TestShouldFailFast` / `TestIsRequired` / `TestCheckTCP_LiveAddr` / `TestCheckTCP_DeadAddr`

**main.go 集成**：
```go
checks := map[string]string{
    "postgres":   "emotion-echo-postgres:5432",
    "kafka":      "emotion-echo-kafka:9092",
    "skywalking": "emotion-echo-sw-oap:11800",
    "llm":        "emotion-llm-service:50051",
}
results := bootstrap.CheckMultiple(depCtx, checks, 3*time.Second)
for name, err := range results {
    failFastIfRequired(name, err, checks[name])
}
```

**环境变量**：
- `STARTUP_STRICT=true|false`（默认 `false`，dev 兼容）
- `STARTUP_STRICT_DEPS=postgres,kafka,skywalking,llm`（CSV，默认前三）

**fail-fast 流程**：
1. TCP 探活所有声明的依赖（3s timeout）
2. 失败的依赖 → 如果 `STARTUP_STRICT=true` 且 dep 在白名单 → `logging.Fatalf()` 立即退出
3. 否则 → warn，继续启动（dev mode 兼容）

---

## 七、资源限制 + 镜像 tag pinned（Stage 20-P0-4）

`deploy/docker-compose.apps.yml`：
```yaml
services:
  emotion-llm-service:
    image: emotion-echo/llm-service:v0.1.0    # P0-4: pinned tag
    deploy:
      resources:
        limits:        { cpus: "1.0", memory: 512M }
        reservations:  { cpus: "0.2", memory: 128M }
  emotion-echo-ai-svc:
    image: emotion-echo/ai-svc:v0.1.0          # P0-4: pinned tag
    deploy:
      resources:
        limits:        { cpus: "1.0", memory: 512M }
        reservations:  { cpus: "0.2", memory: 128M }
```

**tag pinned 优势**：
- 每次 deploy 行为一致（不随 latest 漂移）
- rollback 简单（`docker compose up v0.1.0` → 切回 `v0.0.9`）
- K8s deployment 可以明确指定 `image: ...:v0.1.0` + 镜像 digest 锁定

---

## 八、Graceful Shutdown（Stage 20-1 / 20-3）

### Python（Stage 20-1）

`emotion-llm-service/entrypoint.sh`（POSIX /bin/sh 兼容，dash 不支持 `wait -n`）：

```sh
#!/bin/sh
python main.py &      HTTP_PID=$!
python grpc_server.py & GRPC_PID=$!

cleanup() {
    kill -TERM ${HTTP_PID} ${GRPC_PID} 2>/dev/null || true
    wait ${HTTP_PID} ${GRPC_PID} 2>/dev/null || true
}
trap cleanup TERM INT

while true; do
    kill -0 ${HTTP_PID} 2>/dev/null || { cleanup; exit 1; }
    kill -0 ${GRPC_PID} 2>/dev/null || { cleanup; exit 1; }
    sleep 1
done
```

`grpc_server.py`：SIGTERM → 把 health 置 NOT_SERVING → `server.stop(grace=5)`

### Go（Stage 20-3）

`main.go`：
```go
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
select {
case sig := <-quit:
    rootCancel()                              // gRPC GracefulStop
    shutdownCtx, _ := context.WithTimeout(context.Background(), 10*time.Second)
    srv.Shutdown(shutdownCtx)                 // HTTP GracefulShutdown
}
```

**关键点**：
- 必须用 `http.Server` 而不是 `r.Run()`（gin.Engine 没 Shutdown 方法）
- K8s pod 终止 / docker stop 都会发 SIGTERM
- `preStop hook` 之外，业务层需要主动优雅退出（10s 是默认 terminationGracePeriod）

---

## 九、结构化日志（Stage 20-2 / 20-4）

### Python `logging_setup.py`

```python
def setup_logging():
    handler = logging.StreamHandler(sys.stdout)
    if os.environ.get("LOG_FORMAT", "json") == "json":
        handler.setFormatter(jsonlogger.JsonFormatter(
            "%(asctime)s %(levelname)s %(name)s %(message)s"
        ))
    root = logging.getLogger()
    root.handlers = [handler]
    root.setLevel(os.environ.get("LOG_LEVEL", "INFO"))
```

输出示例：
```json
{"asctime":"2026-07-15T16:15:00","levelname":"INFO","name":"main","message":"analyzed: text_len=4 emotion=happy score=0.6"}
```

### Go `internal/logging`

```go
slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
    Level: slog.LevelInfo,
})))
```

输出示例：
```json
{"time":"2026-07-15T16:15:00.123Z","level":"INFO","msg":"[http] ai-svc HTTP server listening on 0.0.0.0:8891"}
```

**env 切换**：`LOG_FORMAT=json|text`、`LOG_LEVEL=INFO|DEBUG`

---

## 十、Prometheus /metrics（Stage 20-5）

### ai-svc

`internal/metrics/`：
```go
HTTP_REQUESTS_TOTAL = Counter(
    "ai_svc_http_requests_total",
    "Total HTTP requests.",
    ["method", "path", "status"],
)

HTTP_REQUEST_DURATION = Histogram(
    "ai_svc_http_request_duration_seconds",
    "HTTP latency in seconds.",
    ["method", "path"],
    buckets=prometheus.ExponentialBuckets(0.001, 2, 14),  // 1ms → 8s
)

func GinMetricsMiddleware() gin.HandlerFunc { ... }
func PromHTTPHandler() http.Handler { return promhttp.Handler() }
```

**集成**：
```go
r.Use(metrics.GinMetricsMiddleware())                 // 计数器
r.GET("/metrics", gin.WrapH(metrics.PromHTTPHandler()))  // /metrics 路由
```

**GinAuthMiddleware 白名单**：`/health` + `/metrics` 不需要 JWT（`emotion-echo-shared/pkg/middleware/gin_auth.go`）

### llm-service

见 §五（Stage 20-P0-2）。

---

## 十一、docker-compose 设计

### 网络拓扑

```
emotion-echo_app-network (bridge, external)
├── postgres          (infra)
├── redis             (infra)
├── kafka             (infra)
├── skywalking-oap    (infra)
├── emotion-llm-service   (apps) ← HTTP 8000, gRPC 50051
└── emotion-echo-ai-svc    (apps) ← HTTP 8891, gRPC 8892
```

### 启动顺序

```bash
# Step 1: 基础设施
docker compose -f deploy/docker-compose.infra.yml up -d

# Step 2: 业务应用（依赖 infra + 容器 DNS）
docker compose -f deploy/docker-compose.apps.yml up -d

# 或一键：
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d
```

### depends_on

| 服务 | 依赖 | condition |
|---|---|---|
| emotion-llm-service | — | — |
| emotion-echo-ai-svc | postgres / kafka / skywalking-oap / emotion-llm-service | `service_healthy` 或 `service_started` |

注意：跨 compose 文件的 `condition: service_healthy` **不会跨项目共享 health 状态**。infra compose 启动的容器在新项目看是 `service_started`。Stage 20-P0-3 的 fail-fast 是真正可靠的 readiness 保障。

### 完整环境变量

| 服务 | 变量 | 默认 | 说明 |
|---|---|---|---|
| emotion-llm-service | `INTERNAL_API_KEY` | dev-key | gRPC auth key |
| emotion-llm-service | `INTERNAL_API_KEY_REQUIRED` | 0 | `1` 强制 key 缺失时 exit |
| emotion-llm-service | `TLS_ENABLED` | 1 | mTLS |
| emotion-llm-service | `LOG_FORMAT` | json | `json` / `text` |
| emotion-echo-ai-svc | `GIN_MODE` | release | gin 模式 |
| emotion-echo-ai-svc | `POSTGRES_DSN` | localhost:5432 | 容器内用 `emotion-echo-postgres:5432` |
| emotion-echo-ai-svc | `KAFKA_BROKERS` | localhost:9092 | 容器内用 `emotion-echo-kafka:9092` |
| emotion-echo-ai-svc | `SKYWALKING_OAP_ADDR` | localhost:11800 | 容器内用 `emotion-echo-sw-oap:11800` |
| emotion-echo-ai-svc | `LLM_BASE_URL` | http://localhost:8000 | 容器内用 `http://emotion-llm-service:8000` |
| emotion-echo-ai-svc | `LLM_GRPC_ADDR` | localhost:50051 | 容器内用 `emotion-llm-service:50051` |
| emotion-echo-ai-svc | `STARTUP_STRICT` | false | fail-fast 总开关 |
| emotion-echo-ai-svc | `STARTUP_STRICT_DEPS` | postgres,kafka,skywalking | 哪些 dep 触发 fail-fast |
| emotion-echo-ai-svc | `LOG_FORMAT` | json | `json` / `text` |

---

## 十二、端到端验证

`scripts/verify_e2e.py`：

```bash
python scripts/verify_e2e.py
```

期望输出：
```
[OK] llm HTTP :8000/health       {'status': 'ok', ...}
[OK] llm HTTP :8000/analyze      emotion=happy score=0.6
[OK] llm HTTP :8000/metrics      6 llm_* series
[OK] ai-svc HTTP :8891/health    dbOk=True
[OK] ai-svc gRPC :8892 emotion.AI   status=1 (SERVING)
[OK] ai-svc /metrics             2 ai_svc_http_requests_total series
=== Summary: 6/6 passed ===
```

### 启动失败诊断

```bash
# 看 ai-svc 日志（JSON 格式）
docker logs --tail 30 emotion-echo-ai-svc

# 看容器内 DNS 是否解析正常
docker exec emotion-echo-ai-svc nslookup emotion-echo-kafka

# 测试容器内是否能连
docker exec emotion-echo-ai-svc wget --spider http://emotion-llm-service:8000/health

# 容器内 YAML 是否正确加载
docker exec emotion-echo-ai-svc cat /app/etc/ai-api.yaml
```

---

## 十三、常见问题（FAQ）

### Q1: ai-svc 镜像构建失败 — `emotion-echo-shared` not found
**A**: build context 必须是仓库根。`docker build -f emotion-echo-ai-svc/Dockerfile .`（最后那个 `.`）。
Dockerfile 第 2 阶段不能 `COPY .`（会把整个 context 套到子目录），必须 `COPY emotion-echo-ai-svc ./`。

### Q2: emotion-llm-service gRPC 起不来 — `certificate not found`
**A**: 检查 `deploy/tls/*.crt|*.key` 是否存在；`deploy/tls/generate.py` 可重新生成。
容器内路径由 `TLS_CA_CERT` / `TLS_SERVER_CERT` / `TLS_SERVER_KEY` env 控制。

### Q3: ai-svc `SkyWalking.enabled type mismatch, expect bool, actual number`
**A**: yaml 里的 bool 字段不能用 `${VAR:-1}`（go-zero conf 解析为 number），用 `${VAR:-true}` 又会变成 string。
**写死 `true`**。或者改 config.go 把 bool 改成 string 自己 parse。

### Q4: ai-svc `Kafka.brokers invalid character '$'`
**A**: go-zero conf **不支持 list 字段的 env 替换**。改成 `BrokersCSV string` + `kafkaBrokers()` helper split。

### Q5: alpine 镜像 exec tini 失败 — `no such file or directory`
**A**: tini 静态二进制（glibc 编译）在 alpine（musl）下不能跑。
**用 `apk add --no-cache tini`**（alpine 官方包，musl 编译）。

### Q6: ai-svc 启动报 `ModuleNotFoundError: No module named 'logging_setup'` / `'metrics_setup'`
**A**: Dockerfile builder 阶段必须 COPY 所有业务 .py 文件，runtime 阶段必须 `--from=builder` 复制。
漏 COPY → 镜像里就没有 → import 失败 → container restart loop。

### Q7: depends_on `service_healthy` 不生效 — ai-svc 等不到 llm-service
**A**: 跨 compose 文件 health 状态不共享。改成 `service_started`，或用 Stage 20-P0-3 的 fail-fast（更可靠）。

### Q8: 容器内 wget/healthcheck 报 401
**A**: GinAuthMiddleware 默认拦所有路由。已把 `/health` 和 `/metrics` 加白名单
（`emotion-echo-shared/pkg/middleware/gin_auth.go`）。

---

## 十四、Stage 21+ 候选

### P1-K8s 化（高优先级）
- Helm chart（Deployment / Service / ConfigMap / Secret / Ingress / HPA / PDB / NetworkPolicy / ServiceAccount）
- Liveness / Readiness / Startup Probe 分离
- imagePullSecret + 私有镜像仓库（aliyun / harbor）
- Sealed Secret / Vault（避免 key 提交进 git）

### P2-可观测性
- Kafka consumer trace 上报（目前 ai-svc 启动 consumer 但没接 go2sky span）
- Grafana dashboard（emotion_echo_http_requests_total / llm_analyze_total 等）
- Alertmanager rules（llm_analyze_total{status="err"} rate > 0.1/s）

### P3-可靠性
- 启动时严格依赖检查已做 ✅（Stage 20-P0-3）
- 限流熔断（ai-svc 入口限流 + gRPC client retry budget 已做）
- 灰度发布（Argo Rollouts）
- Chaos engineering（chaos-mesh）

### P4-高级
- 性能压测（k6 / ghz）
- 跨区域多活
- PostgreSQL 流复制 + 自动 failover

---

## 十五、关键经验教训

1. **build context 必须是仓库根**：multi-module 项目（ai-svc 引用 ../shared）很容易踩坑，Dockerfile 内不能 `COPY .` 会破坏路径。
2. **go-zero conf 限制**：env 替换只对 string 字段生效，list / bool 不行（必须自己 helper parse）。
3. **alpine + glibc 二进制**：tini 静态下载（glibc）在 alpine（musl）下 exec 失败，必须用 apk 装的 musl 版本。
4. **跨 compose health 不共享**：`depends_on condition: service_healthy` 在多 compose 文件下失效，应用层 fail-fast 才是真可靠。
5. **SearchReplace 经常失灵**：复杂文件（缩进、嵌套引号）用 SearchReplace 经常改错，应该用 Write 整个文件重写。
6. **Dockerfile 漏 COPY 的隐蔽性**：Docker cache 复用之前 layer 不会重新校验，必须 `--no-cache` 才能验证。

---

**已完成。下一个阶段：K8s 化（Helm chart）。**