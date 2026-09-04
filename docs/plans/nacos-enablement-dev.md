---
status: planned
priority: high
owner: TBD
target-stage: Stage 39（候选）
created: 2026-09-04
---

# Plan — Nacos dev 模式启用（从"半启用"到"全链路"）

## 一、现状（与代码事实对齐）

### 1.1 启动 + 注册：✅ 通

- `deploy/docker-compose.infra.yml:142-167` 定义 `nacos: nacos/nacos-server:v2.4.3` standalone + Derby，
  `healthcheck` 走 `/nacos/actuator/health`。当前 `docker ps` 看到 `emotion-echo-nacos` healthy。
- 6 个业务 svc（user / chat / analytics / assessment / ai / web-bff）+ llm-service 都
  `depends_on: nacos: service_started`，并注入 `NACOS_ENABLED=true / NACOS_ADDR / NACOS_NAMESPACE / NACOS_HOT_RELOAD=false`。
- 每个 svc 的 `nacos_boot.go` 模板：WaitForNacos → NewNacosRegistry → Register → Heartbeat(5s) →
  NewNacosConfig → GetConfig(`{svc}.ops.yaml`，dev 通常不存在) → ListenConfig（HotReload=true 时挂）。
- 实测：Nacos console `instance/list?serviceName=ai-svc&namespaceId=emotion-echo-dev` 返回
  `name: DEFAULT_GROUP@@ai-svc`，但 **`hosts: []`**——注册 RPC 成功，但 ephemeral 实例 30s 内被踢出
  Nacos 内部索引（SDK `Ephemeral=true` + 单节点 Derby 启动慢 + Heartbeat 没真正触发 BeatRequest 的现象之一，
  待 PR-1 修）。

### 1.2 消费方：❌ 0 个

- `shared/pkg/discovery/registry.go:39-58` 定义 `Registry` 接口，已包含 `Discover / Subscribe / Heartbeat`
  三个发现 API。`nacos_register.go` 已实现 `Discover(ctx, serviceName)` 与
  `Subscribe(ctx, serviceName, cb)`（`nacos_register.go:170-226`）。
- `shared/pkg/configcenter/config_center.go:39-52` 定义 `ConfigCenter` 接口，已包含
  `GetConfig / ListenConfig / PublishConfig / Close`。`nacos_config.go` 已实现。
- **业务代码 0 处调用** `Registry.Discover / Subscribe / ConfigCenter.ListenConfig`。
  全文搜索 (`grep -rn "Registry.Discover\|.Discover(\|\.Subscribe(\|ConfigCenter.GetConfig\|ListenConfig(" emotion-echo-*/...`)，
  仅命中 6 个 `nacos_boot.go` 内部的 `cc.GetConfig(...)` 拉 ops.yaml 与 `reg.Register(...)`，
  **没有任何** 把 Nacos 用于服务发现/配置热更新的消费方。
- 结果：dev 模式下所有跨服务地址（user→postgres、chat→kafka、bff→ai/llm、ai→llm/fer/sensevoice/xtts）
  全部走 env 注入的硬编码 DNS（`emotion-echo-<svc>:<port>`）。

### 1.3 APISIX upstream：❌ 静态节点

- `deploy/apisix/seed.sh:81-110` `put_upstream` 6 条 `nodes: [{host, port, weight: 1}]` 静态节点。
  注释 `# Stage 34+ 切 nacos-discovery` 明确写过。
- `charts/emotion-echo/charts/apisix/templates/configmap.yaml:39` 也写了
  `# Stage 34+ 切 nacos-discovery 插件时再加 discovery_type: nacos 配置`。
- `docs/stages/stage-32-landing.md §三.1` 已记录"用静态节点是 Stage 32 的架构取舍，Nacos 集成留给 Stage 34+"。

### 1.4 注册名 / 端口 / metadata 一览（事实）

| svc | serviceName（Nacos） | 端口 | metadata | 备注 |
|---|---|---|---|---|
| user-svc | `user-svc`（**测试用短名**，生产 config 默认 `emotion-echo-user-svc`） | 8888 | stage, version | ⚠️ 测试/默认值不一致（见 PR-0） |
| chat-svc | `chat-svc` / 默认 `emotion-echo-chat-svc` | 8890 | stage, version | 同上 |
| analytics-svc | `analytics-svc` / 默认 `emotion-echo-analytics-svc` | 8893 | stage, version | 同上 |
| assessment-svc | `assessment-svc` / 默认 `emotion-echo-assessment-svc` | 8889 | stage, version | 同上 |
| ai-svc | `ai-svc` / 默认 `emotion-echo-ai-svc`（HTTP 8891 注册，metadata.grpc_port=8892） | 8891 (+ gRPC 8892) | stage, version, grpc_port | AI 唯一双端口 |
| web-bff | `web-bff` / 默认 `emotion-echo-web-bff` | 8894 | stage, version | 注册仅为供 APISIX nacos-discovery 用 |
| emotion-llm-service | `emotion-llm-service`（llm-service 已加 `BootNacos`，但 Python 端未实现注册；见 PR-3） | 8000 (+ gRPC 50051) | n/a | Nacos 上注册名存在但 `hosts: []` |

> **结论**：dev 模式下 Nacos **容器在跑、业务 svc 注册成功**，但没有任何消费方，"Nacos 启用了一半"。

---

## 二、为什么"没用"

| 原因 | 证据 |
|---|---|
| 接口已就绪但没人调 | `Registry.Discover / Subscribe` 与 `ConfigCenter.ListenConfig` 接口完整、SDK 实现完整、契约测试覆盖完整（`nacos_register_integration_test.go` / `nacos_config_integration_test.go`），**仅缺业务侧调用方** |
| APISIX 切 nacos-discovery 没做 | seed.sh 写死 6 条静态 upstream；`stage-32-landing.md §三.1` 显式延期到 Stage 34+ |
| HotReload 默认关闭 | 所有 svc 注入 `NACOS_HOT_RELOAD=false`，即使挂上 `ListenConfig` 也不生效 |
| serviceName 测试 vs 默认值不一致 | 6 个 `nacos_boot_test.go` 用短名（`ai-svc`），业务 config 默认长名（`emotion-echo-ai-svc`）——即使消费方想按短名 `Discover("ai-svc")` 也找不到 |
| ephemeral 实例被踢 | 实测 Nacos `instance/list` 返回 `hosts: []`（SDK 心跳未生效或 Derby 启动慢）；这是阻塞 dev 注册表可信度的 P0 |
| BootNacos 失败降级太狠 | `ai-svc/main.go:384-388` 等所有 `main.go` 都只 log 不退出，**注册失败没人能感知**——smoke 永远抓不到 |

---

## 三、目标（dev 模式"真正用起来"的验收条件）

1. **dev 启动后** `curl http://localhost:8848/nacos/v1/ns/instance/list?serviceName=user-svc&namespaceId=emotion-echo-dev`
   返回 `hosts` 至少 1 条，IP 是 `emotion-echo-user-svc`，healthy=true。
2. **服务间调用**至少 3 处走 `Registry.Discover`（不再是 env 硬编码 DNS）：
   - `web-bff` → `ai-svc`（`downstream/ai.go`）改 `baseURL` 来源
   - `web-bff` → `chat-svc`（`downstream/chat.go`）改 `baseURL` 来源
   - `web-bff` → `analytics-svc`（`downstream/analytics.go`）改 `baseURL` 来源
   - 其它（assessment/user/emotion_query/llm/xtts）作为 Round 2。
3. **APISIX upstream** 6 条全部切 `nacos-discovery` 插件；svc 扩缩容/重启 APISIX 自动感知。
4. **HotReload** 至少 1 处（web-bff 限流阈值）`NACOS_HOT_RELOAD=true` + `ListenConfig` 真实回调 + `limit-count` 阈值可热更新。
5. **契约 smoke**（`scripts/smoke_data_layer.py` 必须新增 §契约 7）：
   - 起 dev compose → 触发 1 个 `/api/v1/user/health` → 等 5s → `curl http://localhost:8848/nacos/v1/ns/instance/list`
     断言 `hosts.length > 0 && healthy=true`，否则退出非 0。

---

## 四、修复计划（按 TDD 节奏拆 PR）

> 原则：所有 PR 必须 Red→Green→Refactor；先写契约测试再改实现；不允许"先改代码再补测试"。

### PR-0（5 分钟 · 修一致性）— 测试不一致的 serviceName

**症状**：`nacos_boot_test.go` 用 `Name: "ai-svc"` 等短名，业务 `config.go` 默认 `Name: emotion-echo-ai-svc`。
APISIX nacos-discovery 按 serviceName 拉实例，短/长不一致会 404。

**🔴 RED**：写 `TestNacosRegistry_Discover_RespectsFullServiceName`：mock 一个长名 `emotion-echo-ai-svc` 实例，
断言 `Discover("emotion-echo-ai-svc")` 返回；再 `Discover("ai-svc")` 断言空。

**🟢 GREEN**：统一 `config.Name` 默认值为 `emotion-echo-<svc>`（已经是事实标准），并把 6 个 `nacos_boot_test.go`
的 `Name:` 改成 `"emotion-echo-<svc>"`。`BootNacos` 注册时也用同一份 `cfg.Name`（已经这样）。

**♻️ REFACTOR**：把所有 serviceName 常量挪到 `emotion-echo-shared/pkg/discovery/servicenames.go`，
单测与生产共用一份。

**验收**：`go test ./emotion-echo-shared/pkg/discovery/... ./emotion-echo-user-svc/...` 全绿。

### PR-1（30 分钟 · 修 ephemeral hosts=[]）— 让 dev 注册表真的可信

**症状**：实测 `instance/list` 返回 `hosts: []`，说明 SDK 注册后 ephemeral 实例被踢。原因之一是
`nacos_register.go:78` `WithBeatInterval(5000)` 但 `Heartbeat`（PR-03）启动的是"重新注册"goroutine
而非 BeatRequest，叠加 Nacos 2.4.3 standalone Derby 启动慢、心跳间隔内已被 GC。

**🔴 RED**：扩 `nacos_register_integration_test.go` 加 `TestNacosRegistry_InstanceSurvives30s`
（真起 Nacos container → Register → Sleep 35s → SelectInstances 断言非空）。

**🟢 GREEN**：
- 在 `NacosRegistry.Heartbeat` 改成调用 SDK 的 BeatInstance（`nacosnaming.Client` 不直接暴露，
  改用 `nacosclients.NewNamingClient` 后调用 `instance.Beat` 或在 SDK 配置里
  `WithBeatInterval(5000)` 已自动 Beat；问题在 SDK v2.4.3 与 Nacos 2.4.3 server 之间对
  ephemeral 健康判定不一致）。
- 把 `nacos_register.go:78` 的 `WithBeatInterval(5000)` 调到 `2000`，并显式 `WithNotLoadCacheAtStart(true)`
  已经存在；额外加 `WithUpdateThreadNum(2)`。
- 若仍踢出，临时把 `Ephemeral: true`（`nacos_register.go:141`）改 `false` 走持久实例（不需心跳），但需
  在 docstring 标注"dev only 兜底，prod 改回"。

**♻️ REFACTOR**：抽出 `buildInstance(...)` 把 metadata 集中。

**验收**：dev `instance/list` 返回 `hosts.length >= 1`；PR 之前必须跑的契约 smoke §7 全绿。

### PR-2（半天 · Web-BFF 走 Nacos 发现下游）— 把"没用"改成"有用"的关键 PR

**目标**：`web-bff` 启动时 `Subscribe("emotion-echo-ai-svc")` 等订阅；调用 `downstream/ai.go` 时 baseURL
来自订阅缓存（首选），env 注入作为兜底（dev 不变）。

**🔴 RED**：写 `TestDownstreamAI_ResolveBaseURL_FromNacosSubscription`：
- 注入 fakeRegistry（已有契约测试在 `emotion-echo-shared/pkg/discovery/registry_test.go`）。
- fake `Discover("emotion-echo-ai-svc")` 返回 `[{Host: emotion-echo-ai-svc, Port: 8891}]`。
- 断言 NewAI client 的 `baseURL` 字段 = `http://emotion-echo-ai-svc:8891`，而不是 yaml 默认。

**🟢 GREEN**：
- 新增 `emotion-echo-web-bff/internal/discovery/resolver.go`：暴露 `Resolver interface { Resolve(ctx, svcName) (host, port, error) }`。
- `nacos_boot.go` 在 `BootNacos` 后，把 `nacosRuntime.Registry` 注入到 `svc.ServiceContext.Resolver`。
- `downstream/{ai,chat,analytics,assessment,user,emotion_query,xtts}.go` 的 `New*` 构造器签名加 `Resolver` 字段；
  当 `opts.BaseURL == ""` 时调 `Resolver.Resolve`，env 注入仍优先生效。
- `main.go` 装配链：`config → clients → ServiceContext → handlers` 之后、`r.Run(...)` 之前
  （参考 `.zcode/plans/plan-sess_54a56bdb-...md:92` 提到的插入点）。

**♻️ REFACTOR**：把 7 个 downstream client 的 host 解析统一走 `Resolver`，删除 yaml 默认值里的 `localhost:*`。
为 round-trip 兼容性保留 env `BFF_*_BASE_URL` 作为"测试/特殊环境"兜底。

**验收**：
- `go test ./emotion-echo-web-bff/...` 全绿（含 7 个 client 的新 contract test）。
- dev 模式 `docker exec emotion-echo-web-bff wget http://localhost:8894/api/v1/ai/health` 仍 200。
- 在 Nacos 控制台把 `emotion-echo-ai-svc` 实例手动 Deregister，web-bff 5s 内后续请求快速失败（验证 Subscribe 真在用）。

### PR-3（半天 · APISIX upstream 切 nacos-discovery）

**目标**：APISIX 6 个 upstream 全部从静态 nodes 改为 `nacos-discovery` 插件。

**🔴 RED**：在 `deploy/apisix/` 加 `seed_nacos_test.sh`：起 etcd + apisix + nacos 三个容器，
注册 fake "web-bff" 2 个实例 → 调 admin API 验证 upstream 实际命中的节点来自 Nacos（可通过
APISIX metrics `upstream_nodes_total{service="web-bff"}` 断言）。

**🟢 GREEN**：
- 修改 `deploy/apisix/seed.sh`：每个 `put_upstream` body 改为
  ```json
  {
    "name": "web-bff",
    "type": "roundrobin",
    "discovery_type": "nacos",
    "service_name": "emotion-echo-web-bff",
    "namespace_id": "emotion-echo-dev",
    "group_name": "DEFAULT_GROUP"
  }
  ```
- 同时 `charts/emotion-echo/charts/apisix/templates/configmap.yaml:39` 取消注释，加 `nacos-discovery` 插件全局配置。
- Helm values 默认开启 dev 模式；prod 由 `nacos-discovery.enabled` 开关。

**♻️ REFACTOR**：seed.sh 抽出 `put_nacos_upstream()` 函数，避免 6 处重复。

**验收**：
- dev 模式 `curl http://localhost:19080/api/v1/user/health` 仍 200。
- 手动 `docker stop emotion-echo-user-svc` 后 30s 内再调 `/api/v1/user/profile` 返回 503，证明 nacos-discovery
  把不健康实例摘除。
- 关 Nacos 容器后 60s 内所有通过 APISIX 的请求 502（验证 fallback 行为符合预期，或显式无 fallback）。

### PR-4（1 小时 · HotReload 真启用）— 配置中心价值兑现

**目标**：`web-bff` 的 `BFF.LimitCount` 从 Nacos `web-bff.ops.yaml` 拉，启动后挂 `ListenConfig`
热更新（实际改 limit-count 阈值）。

**🔴 RED**：`TestConfigCenter_ListenConfig_UpdatesBFFLimitCount`：fake cc 发布 `web-bff.ops.yaml` →
断言 webhook 回调触发 → 断言 svcCtx 里 `LimitCount` 字段变更。

**🟢 GREEN**：
- yaml 模板新增 `LimitCount` 字段，env `BFF_LIMIT_COUNT` 注入。
- `web-bff/nacos_boot.go` 改为 `if cfg.Nacos.HotReload { cc.ListenConfig(...) }`，回调内
  `yaml.Unmarshal([]byte(content), &limitCfg)` 并更新 `svcCtx.LimitCount`（加 mutex）。

**♻️ REFACTOR**：把"ops.yaml → struct"解析统一到 `shared/pkg/configcenter/opsconfig/` 包，避免 6 个 svc 各自实现。

**验收**：
- dev 启动后 `curl -X PUT -d "limit_count: 30" http://localhost:8848/nacos/v1/cs/configs?dataId=web-bff.ops.yaml&group=DEFAULT_GROUP` → 30s 内日志出现 `[hot-reload] web-bff.ops.yaml changed`，`LimitCount` 变更生效。
- 加 `NACOS_HOT_RELOAD=true` env 到 web-bff compose。

### PR-5（半天 · BootNacos 失败必须 fail-fast）— 把"注册失败没人感知"的兜底变硬

**目标**：`BootNacos` 失败时业务 svc 不启动（当前只 log），因为 dev 模式下 Nacos 不可达意味着
整套服务发现不可信——继续跑等于"假装一切正常"。

**🔴 RED**：`TestBootNacos_FailFast_ExitsMain` 之类的契约（在 `nacos_boot_test.go` 已有的
`TestBootNacos_WaitForNacosFailurePropagates` 基础上扩）。

**🟢 GREEN**：
- 6 个 `main.go` 改为：
  ```go
  if err != nil { os.Exit(1) }
  ```
- 仅保留"config 缺失时 GetConfig 失败 → 继续"这种**已知可降级**的分支。

**♻️ REFACTOR**：把 BootNacos 的"硬错误 vs 软错误"分类用 sentinel error 标注。

**验收**：
- `docker stop emotion-echo-nacos && docker restart emotion-echo-user-svc`，用户 svc 启动失败
  退出码非 0，重启循环不会陷入"假活着"。
- Nacos 恢复后重启 svc 正常起来。

---

## 五、容器清理（按用户指示）

> 范畴：**dev compose 当前 18 个容器里没人用 / 不在关键路径上的**。
> 判定标准：没有 env 被引用 / yaml config 引用 / 代码 import / Nacos 注册表提及的，列为"待删"。

### 5.1 待删容器（启动后立刻 drop）

| 容器 | 当前状态 | 删除依据 |
|---|---|---|
| `emotion-echo-sensevoice` | Up 5 min healthy | compose `emotion-echo-ai-svc` 注入 `SENSEVOICE_BASE_URL=http://emotion-echo-sensevoice:8002`，但 `emotion-echo-ai-svc/internal/analyzer/multimodal.go` 与 `internal/aiclient/sensevoice.go` 仅在 `BaseURL != ""` 时启用。**当前 dev 默认 SENSEVOICE_BASE_URL 留空**（`ai-api.yaml:73-75` `BaseURL: ""`），即 compose 里 env 被 env 占位符展开成 `""` 直接覆盖。看 `docker-compose.apps.yml:334` `SENSEVOICE_BASE_URL: ${SENSEVOICE_BASE_URL:-http://emotion-echo-sensevoice:8002}` —— ⚠️ 这里会被 `.env.local` 没设时**默认注入了**！需先确认 `.env.local` 没设 SENSEVOICE_BASE_URL 再删；否则先改 yaml `BaseURL: ""` 为默认。 |
| `emotion-echo-fer` | Up 5 min healthy | 同上，`FER.BaseURL: ""` 默认空。compose env `${FER_BASE_URL:-http://emotion-echo-fer:8004}` 同问题，需先确认 `.env.local`。 |
| `emotion-echo-xtts` | Up 5 min healthy | 同上。**额外确认**：`emotion-echo-web-bff/internal/config/config.go:122` `c.XTTS.BaseURL = v` 行为，env 注入路径。 |
| `emotion-echo-sw-oap` | Up 5 min (无 healthcheck) | SkyWalking OAP 9.7 接收 trace，但 `grep -rn "skywalking\.\|SkyWalking"` 全是 enable/disable 配置 + env；dev 没接 SDK 上报 trace（`shared/pkg/skywalking/skywalking.go` 用法待查）。 |
| `emotion-echo-sw-ui` | Up 5 min (无 healthcheck) | SkyWalking UI，dev 没人访问（仅 ops 用）。 |

> 待二次确认后逐项执行 `docker rm -f <name>` + `docker network` 清理。

### 5.2 不动容器（关键路径 / 真正用）

| 容器 | 用途 |
|---|---|
| `emotion-echo-postgres` | 全部 svc DSN 用 |
| `emotion-echo-redis` | session / rate-limit 用 |
| `emotion-echo-kafka` | chat-svc → analytics-svc → ai-svc 链路用 |
| `emotion-echo-etcd` | APISIX config_provider 用 |
| `emotion-echo-apisix` | 网关入口 |
| `emotion-echo-nacos` | 本计划主语 |
| `emotion-echo-user-svc / chat-svc / analytics-svc / assessment-svc / ai-svc / web-bff / llm-service` | 7 个业务 svc |

### 5.3 删除执行（待用户确认后再做）

PR-A（容器清理，与代码改动解耦）：

1. `grep -rn "SENSEVOICE\|FER_BASE_URL\|XTTS_BASE_URL" deploy/env/.env.local.example deploy/docker-compose.apps.yml`，
   确认 `.env.local` 不存在 / 不启用；
2. 改 `docker-compose.apps.yml` 的 `ai-svc` 段，把这三个 env 改为 `BaseURL: ""` 等价（不再注入 URL）；
3. `docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml down` 删旧容器；
4. `docker compose ... up -d --build`（不含 fer/sensevoice/xtts）；
5. `docker ps --format '{{.Names}}'` 复核仅剩 5 个 infra + 7 个业务 = 12 容器。

---

## 六、调研依据（commit message 末尾引用）

- 已读文件：
  - `deploy/docker-compose.infra.yml`（nacos 142-167 / etcd 174 / 其余 infra）
  - `deploy/docker-compose.apps.yml`（6 业务 + llm + ai + 注入 NACOS_* env）
  - `deploy/apisix/seed.sh`（6 静态 upstream + Stage 34+ 注释）
  - `emotion-echo-{user,chat,analytics,assessment,ai,web-bff}-svc/nacos_boot.go`（6 份模板）
  - `emotion-echo-shared/pkg/discovery/{registry,nacos_register}.go`（interface + 实现）
  - `emotion-echo-shared/pkg/configcenter/{config_center,nacos_config}.go`
  - `emotion-echo-web-bff/internal/downstream/{ai,chat,analytics,assessment,user,emotion_query,llm,xtts}.go`（baseURL 注入形态）
  - `emotion-echo-web-bff/internal/config/config.go`（env 注入）
  - `emotion-echo-ai-svc/internal/aiclient/{fer,sensevoice,xtts,ai_client}.go`
  - `charts/emotion-echo/charts/apisix/templates/configmap.yaml`（Stage 34+ 注释）
- 已查 ADR / stage 文档：
  - `docs/architecture/decisions.md:144 / :238`（Nacos 决策）
  - `docs/stages/stage-31-nacos-reintroduction.md` / `stage-31-landing.md`（Nacos 设计）
  - `docs/stages/stage-32-landing.md §三.1` / `stage-32-apisix-reintroduction.md`（APISIX nacos-discovery 设计）
  - `docs/stages/stage-32-cleanup.md:170` / `stage-33-landing.md:224`（延期到 Stage 34+ 的承诺）
- 已跑现状：Nacos `instance/list` 实测返回 `hosts: []`；`docker ps` 当前 18 容器清单。

---

## 七、范围与不在范围

**做**：6 PR（PR-0~PR-5），按 TDD 节奏；删 5 个无人引用的容器（PR-A）。

**不做**（留给后续 stage）：
- Nacos 集群化（dev 仍单节点 Derby）
- Nacos auth 启用（dev 关闭）
- mTLS 加 Nacos SDK
- 业务代码改用 Nacos 服务发现的全部 7 个 client（PR-2 只做 ai/chat/analytics 三个高优，其余 Round 2）
- 限流/熔断配置全部走 Nacos（PR-4 只做 web-bff LimitCount）
- 跨 stage 间 Nacos 与 APISIX / Prometheus 的全链路联动

---

## 八、与现有文档的一致性修复（顺手做）

`docs/stages/stage-32-landing.md` 与 `stage-32-apisix-reintroduction.md` 都把"Nacos 切 APISIX"标为 Stage 34+；
本计划落地后**这两份文档的"未做"列表**需要同步更新（从延期变为已做）。

`docs/architecture/decisions.md:144` 的"客户端定时拉取 + watch（30s 间隔）"目前与代码事实（Nacos SDK 自动）不一致，
落地时一并校对。

---

> 本计划落地后：
> - dev 模式 Nacos **不再是"半启用"**，注册表可信、web-bff 走发现、APISIX 走 discovery、HotReload 至少 1 处生效。
> - smoke §契约 7（注册表 hosts 非空）补进 `scripts/smoke_data_layer.py`，未来 PR 触发即知。