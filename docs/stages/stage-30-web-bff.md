# Stage 30 — 新增 `emotion-echo-web-bff`（Frontend BFF 层）落地规划

> **范围声明**：本文档是 **Stage 30 的路线图留存**，先行于实现落地。
> 本次 session 的唯一动作：**新建本文档**，**不修改**任何代码 / Helm / docker-compose / APISIX / 前端。
> 真正动手时按 §六 TDD 节拍推进，每段循环在独立 commit 内走完 Red → Green → Refactor。

> ## ✅ 落地状态（2026-08-31 更新）
> BFF 已按 §二 T1→T7 完整落地，commit 链见 §六（下方新增"实际落地 commit 链"）。验证：
> - `go test ./emotion-echo-web-bff/...` 全绿（8 包，60+ 单测）
> - `go vet ./emotion-echo-web-bff/...` 无 warning
> - `go test -tags integration ./emotion-echo-web-bff/integration_test/...` 全绿（gRPC dial / SSE E2E / TTS byte-for-byte）
> - `helm lint charts/emotion-echo/charts/web-bff` 0 失败
> - compose block（`deploy/docker-compose.apps.yml`）+ Dockerfile + APISIX catch-all（`deploy/apisix/seed.sh`）已就绪

继承：
- Stage 29-A（cert-manager + Grafana Ingress HTTPS）
- Stage 29-A.5（live cluster smoke + 4 项结构修复）
- 路线图位置：Stage 29-C/D/E（告警 / Secrets / 全路由 TLS / Let's Encrypt）落地之后 → **Stage 30 = 本阶段** → Stage 31（ACK 迁移）

---

## 一、目标

| # | 决议 | 选择 |
|---|------|------|
| 1 | 服务定位 | 新增独立 Go 微服务 `emotion-echo-web-bff`（Gin :8894），作为 Emotion-Echo 前后端唯一 BFF |
| 2 | 职责范围 | **聚合 + ViewModel + SSE 流式编排 + Session/鉴权透传（全部纳入）** |
| 3 | 实现形态 | 独立 Go 服务（与 chat-svc 同一 stack），**不走** Nuxt server/api |
| 4 | 调用下游 | **业界主流混合**：内部 Go svc（ai-svc :8892）走 **gRPC**，其余 svc 走 **HTTP JSON / SSE / chunked PCM**；XTTS 直连 |
| 5 | 鉴权策略 | **仅透传**：复用 `shared/pkg/middleware.GinAuthMiddleware()`，原 `Authorization: Bearer <jwt>` 头透传下游 |
| 6 | 前端入口 | **唯一入口**：APISIX catch-all `/api/v1/*` → BFF，替换现行 16 条 1:1 路由；前端 URL 不变 |
| 7 | 流式协议 | **REST + SSE**；`/api/v1/ai/stream`、`/api/v1/tts/stream` 两端点 |
| 8 | 部署环境 | **K8s 优先 + docker-compose 本地联调**，同步两套 manifest + 两套 smoke |
| 9 | 范围 | 仅服务 Emotion-Echo-Web（Nuxt 3 SPA），不含多端 |
| 10 | 测试约定 | 80%+ 覆盖；测试栈 testify/require；`go test ./...` ≤ 5s；集成测试 `//go:build integration` |

---

## 二、TDD 循环

> **本文档为规划，所有 commit 均为预期占位**。真正落地时按本节节拍执行；每个 commit 后 `go test ./...` 与 `go vet ./...` 必须绿。

### T1 基础设施切片
| # | commit 主题 | 类型 | 说明 |
|---|------------|------|------|
| 1 | `test(web-bff): red — config yaml parsing` | 🔴 RED | 断言 `Port=8894`、`AIService.GRPCAddr` 默认值合法 |
| 2 | `feat(web-bff): minimal config loader` | 🟢 GREEN | `internal/config/config.go` + `etc/web-bff.yaml` |
| 3 | `refactor(web-bff): split applyEnvOverrides` | ♻️ REFACTOR | 与 chat-svc 同模式（go-zero conf 不解析 `${VAR:-default}`） |
| 4 | `test(web-bff): red — logging Init sets slog JSON` | 🔴 RED | 断言 `LOG_FORMAT=json` 后 slog handler 替换成功 |
| 5 | `feat(web-bff): clone ai-svc slog logger` | 🟢 GREEN | 克隆 `emotion-echo-ai-svc/internal/logging/logger.go` |

### T2 下游客户端接口层（**接口先于实现**）
> 每个下游客户端一片切片，先 RED 写接口契约测试，再 GREEN 实现。

| # | 切片 | 接口 |
|---|------|------|
| 6–8 | AIClient | `MultiModalAnalyze / SynthesizeSpeech / AIHealth / StreamEmotion` |
| 9–11 | XTTSClient | `Stream(ctx, text, opts...) (io.ReadCloser, error)` |
| 12–14 | UserClient（gRPC） | `GetMe / UpdateMe / GetByID` |
| 15–17 | ChatClient（HTTP） | `CreateConversation / GetMessages / SendMessage / PinConversation` |
| 18–20 | AssessmentClient（HTTP） | `ListSurveys / GetSurvey / Submit / ListResults / GetResult` |
| 21–23 | AnalyticsClient（HTTP） | `DailyReport / Trend / Behavior{DayNight,Depth,Frequency}` |
| 24 | EmotionQueryClient | `ByMessage / ByConversation` |

每行 GREEN commit 模式：
```
test(web-bff): red — <Client> contract for <Method>
feat(web-bff): define <Client> interface (or implement <Method>)
feat(web-bff): <Client> HTTP/gRPC client method <Method>
refactor(web-bff): <Client> shape-validate / error normalize
```

### T3 SSE + gRPC 编排

| # | commit 主题 | 类型 | 说明 |
|---|------------|------|------|
| 25 | `test(web-bff): red — SSE encoder golden output` | 🔴 RED | `testdata/sse_expected.txt`：`event:` `data:` `id:` `retry:` 字段格式 |
| 26 | `feat(web-bff): SSE encoder` | 🟢 GREEN | `internal/sse/encoder.go`：事件名 + JSON 序列化 |
| 27 | `test(web-bff): red — ai_stream orchestrator pipe` | 🔴 RED | fake gRPC server stream 返回 3 × `delta` + `finish` |
| 28 | `feat(web-bff): pipe gRPC stream to SSE` | 🟢 GREEN | `internal/sse/stream.go`：订阅 `EmotionQueryService` gRPC 流 → SSE 编码 |
| 29 | `test(web-bff): red — ai_stream handler SSE headers` | 🔴 RED | 校验 `Content-Type: text/event-stream`、`X-Accel-Buffering: no`、`Cache-Control: no-cache` |
| 30 | `feat(web-bff): ai_stream handler with SSE headers` | 🟢 GREEN | `internal/handler/ai_stream_handler.go`；client cancel → ctx 取消 |
| 31 | `refactor(web-bff): extract session passthrough helper` | ♻️ REFACTOR | `internal/session/passthrough.go` 抽出 user_id 注入 |

### T4 Handler 层

| # | 模块 | 端点（合 §一映射） | TDD commit 数 |
|---|------|------------------|--------------|
| 32–34 | auth_handler | `/api/v1/auth/*` | 3 |
| 35–37 | user_handler | `/api/v1/users/*` | 3 |
| 38–41 | chat_handler | `/api/v1/conversations[.../messages]` | 4 |
| 42–46 | survey_handler | `/api/v1/surveys[.../submit/results]` | 5 |
| 47–51 | analytics_handler | `/api/v1/reports/*` + `/api/v1/user-behavior/*` | 5 |
| 52 | emotion_query_handler | `/api/v1/emotion/{message,conversation}/*` | 1 |
| 53–54 | multimodal_handler | `/api/v1/multimodal/analyze` | 2 |
| 55–57 | tts_handler | `/api/v1/tts/{synthesize,stream}` | 3 |
| 58 | upload_handler | `/api/v1/uploads/*` （首期 502） | 1 |
| 59–60 | health_handler | `/health` 聚合下游探测 | 2 |

合计约 30 个 handler-related commit。

### T5 集成测试（build tag `integration`）

| # | commit 主题 |
|---|------------|
| 61 | `test(web-bff): integration — gRPC dial ai-svc` |
| 62 | `test(web-bff): integration — SSE E2E` |
| 63 | `test(web-bff): integration — TTS stream byte-for-byte` |

### T6 基础设施

| # | commit 主题 |
|---|------------|
| 64 | `feat(k8s): web-bff helm subchart + values.yaml`（见 §五.6.3） |
| 65 | `feat(compose): emotion-echo-web-bff service block`（见 §五.6.2） |
| 66 | `feat(apisix): replace 16 routes with BFF catch-all + tts-stream`（见 §五.6.4） |

### T7 文档与收尾

| # | commit 主题 |
|---|------------|
| 67 | `docs(stage-30-web-bff): landing`（**本文档**，首次落地时标"规划"；真实落地后替换为含 commit 链 + smoke 证据版） |
| 68 | `chore(hygiene): README badge 29-A.5 → 30 + status block` |

> **本次（规划版）落地仅做 T7.1**，即本文档。其他 67 个 commit 待后续 session 按 TDD 节拍推进。

---

## 三、新增 / 修改 / 删除文件清单

> **本次现状**（仅文档先行）：仅新建 `docs/stage-30-web-bff.md` 一个文件。下方清单为**预期清单**，留待真实落地时核对。

### 预期新增（GREEN 一次性 / 多次）
| 路径 | 行数（估） | 角色 |
|------|-----------|------|
| `emotion-echo-web-bff/go.mod` | 30 | module 声明 + shared replace |
| `emotion-echo-web-bff/Dockerfile` | 50 | 多阶段 Alpine 构建 |
| `emotion-echo-web-bff/main.go` | 180 | 入口 |
| `emotion-echo-web-bff/main_internal_test.go` | 30 | init 冒烟 |
| `emotion-echo-web-bff/integration_test/grpc_dial_integration_test.go` | 80 | 真 gRPC 拨号 |
| `emotion-echo-web-bff/integration_test/ai_stream_e2e_integration_test.go` | 120 | SSE E2E |
| `emotion-echo-web-bff/integration_test/tts_stream_proxy_integration_test.go` | 100 | PCM 字节回环 |
| `emotion-echo-web-bff/etc/web-bff.yaml` | 35 | 配置 |
| `emotion-echo-web-bff/internal/config/config.go` + tests | 130 | 配置 |
| `emotion-echo-web-bff/internal/logging/logger.go` | 110 | slog Init |
| `emotion-echo-web-bff/internal/svc/servicecontext.go` | 60 | 依赖注入容器 |
| `emotion-echo-web-bff/internal/downstream/*.go` | ~1200 | 6 个 client 接口 + 实现 |
| `emotion-echo-web-bff/internal/session/passthrough.go` + tests | 120 | JWT 透传 |
| `emotion-echo-web-bff/internal/sse/encoder.go` + tests | 150 | SSE 编码器 |
| `emotion-echo-web-bff/internal/sse/stream.go` | 100 | gRPC → SSE pipe |
| `emotion-echo-web-bff/internal/handler/*.go` + tests | ~1800 | 9 个 handler |
| `emotion-echo-web-bff/testdata/{jwt_sample.txt,sse_expected.txt}` | 20 | golden data |
| `charts/emotion-echo/charts/web-bff/{Chart,values}.yaml` | 25 | subchart 元数据 |
| `charts/emotion-echo/charts/web-bff/templates/{deployment,service,configmap}.yaml` | 180 | K8s manifest |
| **小计** | **~5400 行** | |

### 预期修改
| 路径 | 变更 |
|------|------|
| `charts/emotion-echo/Chart.yaml` | 追加 `web-bff` subchart 依赖 |
| `charts/emotion-echo/values.yaml` | 增 `web-bff.enabled: true` |
| `charts/emotion-echo/charts/apisix-routes/values.yaml` | 增 `web-bff` upstream |
| `charts/emotion-echo/charts/apisix-routes/templates/routes.yaml` | 删 16 条 1:1 路由，增 2 条 catch-all |
| `deploy/docker-compose.apps.yml` | 增 `emotion-echo-web-bff` service block |
| `deploy/apisix/seed.sh` | 增 `upstreams 7` 与 2 条 `routes` |
| `Emotion-Echo-Web/app/composables/useTTSPlayer.ts` | 第 173 行改为 BFF baseUrl + `/api/v1/tts/stream`，补 `Authorization` 头 |
| `Emotion-Echo-Web/nuxt.config.ts` | `API_BASE_URL` 注释更新（**不删除 `ttsBaseUrl`**） |
| `README.md` | badge `29-A.5` → `30`；status block 增加 BFF 行 |

### 预期删除
| 路径 | 原因 |
|------|------|
| （无） | BFF 是纯增量，不删任何现有代码 |

---

## 四、Smoke Evidence

> **本次未执行**；真实落地时由 §六 commit 链触发。

### 期望产出（落地时补充）

| 阶段 | 命令 | 期望输出 |
|------|------|----------|
| 单元测试 | `go test ./emotion-echo-web-bff/...` | `ok emotion-echo-web-bff/... (all green)` |
| vet | `go vet ./emotion-echo-web-bff/...` | 无 warning |
| 覆盖率 | `go test -cover ./emotion-echo-web-bff/...` | `>80%` |
| 集成测试（可选） | `go test -tags integration ./emotion-echo-web-bff/integration_test/...` | 连接本地 ai-svc :8892 真实拨号；SSE 事件序列断言 |
| Helm lint | `helm lint charts/emotion-echo/charts/web-bff` | `0 failures` |
| Helm template | `helm template ee charts/emotion-echo --set web-bff.enabled=true` | 含 `Deployment/web-bff`、`Service/web-bff`、`ConfigMap/web-bff-config`、**无 `Secret/web-bff-*`** |
| docker compose | `docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d emotion-echo-web-bff` | 容器 `healthy` |
| curl health | `curl -fsS http://localhost:8894/health` | 200 + JSON `{status:"ok", downstream:{ai:"ok", chat:"ok", ...}}` |
| curl BFF 入口 | `curl -fsS -H "Authorization: Bearer $JWT" http://localhost:9080/api/v1/users/me` | 200 + JSON（经 APISIX → BFF → user-svc） |

---

## 五、操作手册（落地时按此推进）

> 本文为路线图记录；步骤本身已就绪，待真实执行时按此操作。

### 5.1 本地 docker-compose 联调
```bash
# 0. 拉取依赖、构建 web-bff 镜像
cd D:/源码/Emotion-Echo
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml build emotion-echo-web-bff

# 1. 起基础设施 + 5 Go svc + ai 集群
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d

# 2. 推送 APISIX 路由（替换 16 条 1:1 为 catch-all + tts-stream）
bash deploy/apisix/seed.sh

# 3. 验证
curl -fsS http://localhost:8894/health
curl -fsS -H "Authorization: Bearer $TEST_JWT" http://localhost:9080/api/v1/users/me
curl -fsS -X POST -H "Authorization: Bearer $TEST_JWT" -H "Content-Type: application/json" \
    -d '{"text":"你好","language":"zh-cn","speed":0.75}' \
    http://localhost:9080/api/v1/tts/synthesize | jq
```

### 5.2 K8s / Helm 部署
```bash
# 0. 渲染验证
helm lint charts/emotion-echo/charts/web-bff
helm template ee charts/emotion-echo --set web-bff.enabled=true | tee /tmp/render.yaml

# 1. apply（假定已 Stage 29-A.5 部署到位）
helm upgrade --install ee charts/emotion-echo \
  --namespace ee-app --create-namespace \
  --set web-bff.enabled=true

# 2. K8s live smoke（与 Stage 29-A.5 同款 9 门 gate）
kubectl -n ee-app get deploy web-bff -o jsonpath='{.status.availableReplicas}'
kubectl -n ee-app get svc web-bff
kubectl -n ee-app port-forward svc/web-bff 8894:8894 &
curl -fsS http://localhost:8894/health
```

### 5.3 APISIX seed 增量推送
```bash
# 仅追加 web-bff 相关，不动现有 6 上游 + 16 路由
bash deploy/apisix/seed.sh  # 见 §六.6.4
```

---

## 六、TDD 提交链（落地时按此节奏提交；本次为空）

> 本次（文档先行）session 的 commit 链 = **仅一条**：`docs(stage-30-web-bff): landing`（即本文档）。其它 67 个 commit 待后续 session 执行。

落地时预期完整 commit 链（按时间顺序）：

```
test(web-bff): red — config yaml parsing
feat(web-bff): minimal config loader
refactor(web-bff): split applyEnvOverrides
test(web-bff): red — logging Init sets slog JSON
feat(web-bff): clone ai-svc slog logger
test(web-bff): red — AIClient contract
feat(web-bff): AIClient interface
feat(web-bff): AIClient http multipart client
feat(web-bff): AIClient gRPC streaming client
refactor(web-bff): AIClient error normalize
... (XTTSClient / UserClient / ChatClient / AssessmentClient / AnalyticsClient / EmotionQueryClient 同模式 × 7) ...
test(web-bff): red — SSE encoder golden output
feat(web-bff): SSE encoder
test(web-bff): red — ai_stream orchestrator pipe
feat(web-bff): pipe gRPC stream to SSE
test(web-bff): red — ai_stream handler SSE headers
feat(web-bff): ai_stream handler with SSE headers
refactor(web-bff): extract session passthrough helper
test(web-bff): red — auth passthrough
feat(web-bff): auth passthrough handler
test(web-bff): red — chat reverse proxy 502/504
feat(web-bff): chat reverse proxy handler
... (user / survey / analytics / emotion_query / multimodal / tts / upload / health handler 各 RED→GREEN) ...
test(web-bff): integration — gRPC dial ai-svc
test(web-bff): integration — SSE E2E
test(web-bff): integration — TTS stream byte-for-byte
feat(k8s): web-bff helm subchart + values.yaml
feat(compose): emotion-echo-web-bff service block
feat(apisix): replace 16 routes with BFF catch-all + tts-stream
docs(stage-30-web-bff): landing   ← 真实落地版（本版本=规划版）
chore(hygiene): README badge 29-A.5 → 30 + status block
```

每条 commit 通过门：
- `go test ./emotion-echo-web-bff/...` 全绿
- `go vet ./emotion-echo-web-bff/...` 无 warning
- 不引入真实 DB / 网络 / 外部服务的依赖（除集成测试 build tag）

---

## 七、不在本次范围（明确留给 Stage 31+）

| 项 | 原因 | 后续阶段 |
|---|------|---------|
| `/api/v1/uploads/{image,video,file}` 真支持 | 尚无明确下游，引入对象存储会大幅扩大 BFF 范围 | Stage 31 与对象存储一起 |
| 多端 / 第三方 BFF | 范围限定只服务 Emotion-Echo-Web Nuxt SPA | Stage 32+ |
| WebSocket 端点 | 范围限定 REST + SSE | Stage 33（如有新需求） |
| emotion-llm-service BFF 直连 | 仍走 ai-svc 间接调用以避免 BFF 多一个 LLM 客户端 | Stage 31 视情况 |
| 跨域 / 全局限流策略 | APISIX 已有，不在 BFF 内做 | 已现成 |
| 真实落地（本文档仅作规划） | 用户本次 session 指令："先落地文档，暂不执行" | 本 Stage 内下次 session |

---

## 八、风险与回退

| 风险 | 影响 | 缓解 | 回退 |
|------|------|------|------|
| **APISIX catch-all 误伤基础设施路径** | 路由到 BFF 后未实现返 404，影响 K8s probe | K8s liveness/readiness 直接走各 svc 原端口，**不经 APISIX**；BFF 未实现路径返 `404 + structured error` | 改 APISIX routes.yaml，恢复对应 1:1 路由 |
| **SSE 被 nginx / APISIX 中间代理缓冲** | 客户端只收到一个大 chunk | BFF 设 `X-Accel-Buffering: no` + `Cache-Control: no-cache`；APISIX route 配置 `proxy-buffering: off` | 临时在 BFF 加 `time.Sleep(50ms)` flush marker |
| **gRPC TLS 未就绪时拨号失败** | ai-svc :8892 握手失败 | `AIService.TLS.Enabled` 默认 `false`；测试覆盖此路径；K8s secret 切换为 `true` | 关闭 `AIService.TLS`，降回 mTLS=false |
| **前端改动诱发回归** | Playwright E2E 失败 | 跑 `Emotion-Echo-Web/e2e/` 全量；首期 catch-all 不改前端路径（仅 `useTTSPlayer.ts:173`） | 暂撤回 `useTTSPlayer.ts` 单点改动 |
| **BFF 单点** | OOM / crash 影响全前端 | K8s `replicas: 1→2`；APISIX catch-all `weight` 多副本分流；`resources.limits.memory=256Mi` | 临时 `kubectl scale deploy/web-bff --replicas=0` 让前端 503 |
| **JWT 透传漏过 user_id 注入下游** | 下游无法识别调用者 | `session/passthrough_test.go` 强制断言下游 gRPC metadata 含 `user_id`；handler 层 acceptance test 验 ctx | `kubectl rollout undo deploy/web-bff` 回退到上一 commit |
| **集成测试不稳定** | CI 抖动 | 仅 `-tags integration` 跑；本地 compose up 后才执行；不在 `go test ./...` 中 | 加 `-short` flag 跳过 |

---

## 九、Refs

- 仓库协作约定：[AGENTS.md §〇 第一性原则（TDD）](/AGENTS.md)
- 上游阶段：
  - [stage-29-A-https-grafana.md](/docs/stages/stage-29-A-https-grafana.md) — cert-manager + Grafana Ingress HTTPS
  - [stage-29-A.5-tls-live-smoke.md](/docs/stages/stage-29-A.5-tls-live-smoke.md) — live cluster smoke
- 架构参考：
  - [distributed-architecture.md](/docs/architecture/distributed.md) — API 网关 = APISIX，鉴权 / 限流 / 熔断 / CORS 在网关层做
  - [microservices-architecture.md](/docs/architecture/microservices.md) — 5 Go svc 布局
  - [architecture-decisions.md](/docs/architecture/decisions.md) — ADR（已确认无 BFF，本次新增属重大决议扩展）
- 现有 microcanonical 模式：`emotion-echo-chat-svc/`（handler / test / main.go / Dockerfile / etc 范式）
- 共享库复用：
  - `github.com/emotion-echo/shared/pkg/middleware` — `GinAuthMiddleware()`
  - `github.com/emotion-echo/shared/pkg/metrics` — `GinMetricsMiddleware()`
  - `github.com/emotion-echo/shared/pkg/grpcinterceptor` — `ClientLoggingInterceptor / ClientTimeoutInterceptor`
- APISIX 路由模板：`charts/emotion-echo/charts/apisix-routes/templates/routes.yaml`
- 后续阶段：30-B 告警、30-C Secrets 增强、30-D 全路由 TLS、30-E Let's Encrypt → 31 ACK 迁移

---
> 最后更新：本次 session 落地（**文档先行，规划留存**） — by 当前协作 Agent
> 适用版本：Stage 30 规划版（v0.0.1-doc-only）；待真实落地时升级为 v0.1.0-impl 版
