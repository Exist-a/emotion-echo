# Stage 29-D — Per-Family TLS Retrofit for the 15 Business ApisixRoutes

> **范围声明**：本文档是 **Stage 29-D 的落地总结**，记录 Grafana 之外的 15 条业务路由全部上 TLS 的实施过程。
> 路线图位置：Stage 29-A.5（Grafana 单点 TLS）→ **Stage 29-D（本文）** → Stage 29-E（Let's Encrypt）→ Stage 30（Web BFF）→ Stage 31（ACK 迁移）。

继承：
- Stage 29-A.5（cert-manager 3-piece subchart + self-signed ClusterIssuer + Grafana Ingress TLS via Certificate + ApisixTls + ApisixRoute 三件套）—— **本文档的核心模式沿用 Grafana precedent**。
- `docs/stage-29-A.5-tls-live-smoke.md` §八（deferral list：「其余 15 条 ApisixRoute TLS retrofit → Stage 29-D」）—— **本文档正是兑现该 deferral**。

---

## 一、目标

| # | 决议 | 选择 |
|---|------|------|
| 1 | 路由范围 | 15 条业务 ApisixRoute（不含 Grafana / mock-ping），Grafana 已在 Stage 29-A.5 上 TLS |
| 2 | SNI 颗粒度 | **按服务族分组（5 族）**：user / chat / analytics / assessment / ai |
| 3 | 证书数量 | **5 张** Certificate + 5 个 ApisixTls（每族一张，共享同族多 SNI 路由） |
| 4 | ClusterIssuer | 复用 `selfsigned-issuer`（dev）；prod 由 Stage 29-E 切 Let's Encrypt |
| 5 | 路由可访问性 | **双轨（dual-stack）**：HTTP `:9080` 仍可达；HTTPS `:9443` 经 SNI 终止后同后端；便于灰度滚动 |
| 6 | cert-manager 最佳实践 | `rotationPolicy: Always` + `renewBeforePercentage: 33`（2024+ 推荐，**不再用绝对** `renewBefore: 720h`） |
| 7 | namespace | 所有 5 族 Cert + ApisixTls 在 `ee-app`（与 ApisixRoute 同 ns） |
| 8 | 测试 | render-assert（`k8s` tag） + live smoke（`integration` tag） + smoke 脚本泛化 |
| 9 | 不动 | mock-ping 路由（r-ping）保留纯 HTTP——TLS for static mock 是 over-engineering |
| 10 | 文档 | 本文 + landing commit + README badge 29-A → 29-D |

---

## 二、业界依据（为什么 5 族，不是 1 张或 15 张）

| 方案 | 优 | 劣 | 适用 |
|------|----|----|------|
| **单网关主机** `api.emotion-echo.local`（1 张 Cert） | 运维极简；1 个 ACME challenge | 1 张 SAN 失败炸 15 路由；不能按服务族 scope 鉴权 | 单团队小项目 |
| **按服务族** 5 张 Cert（**采用**） | 失败 blast radius 限一族；cert-manager `dnsNames` 语义最契合；APISIX `ApisixTls.hosts` 原生支持多 SNI | 5 张而非 1 张 | 中型项目、多团队 |
| **每条路由** 15 张 Cert（**否决**） | 隔离最细 | 15 个 ACME challenge（Let's Encrypt rate-limit 50/周）；运维开销大；客户端配置爆炸 | 大型合规场景 |

**核心引用**：
- APISIX 官方：`https://apisix.apache.org/blog/2021/10/22/cert-manager-in-ingress/` —— Certificate + ApisixTls + Secret 是 end-to-end 路径，ApisixTls.hosts 是数组。
- cert-manager 官方：`https://cert-manager.io/docs/usage/certificate/` —— 一证书一生命周期；shared SAN cert 耦合坏主机。
- cert-manager 2024+：`renewBeforePercentage` 优于绝对 `renewBefore`（避免 issuer 时钟偏移）。

---

## 三、TDD 循环（5 个 commit，完整节奏）

### 29-D.1 🔴 RED：render-assert

**文件**：`k8s/tests/stage_29d_render_test.go`（`//go:build k8s`）

| 测试 | 断言 | RED 原因 |
|------|------|---------|
| `TestStage29D_AllFamiliesCertificatesRender/01_user_certificate_renders` | 渲染输出含 `Certificate/user-family-tls` + `dnsNames: [user.echo.local]` | 模板尚未生成 |
| `TestStage29D_AllFamiliesCertificatesRender/02_user_apisixtls_renders` | 渲染输出含 `ApisixTls/user-family-tls` + `hosts: [user.echo.local]` | 同上 |
| `TestStage29D_AllFamiliesCertificatesRender/03_user_routes_have_hosts/r-user-me` | `ApisixRoute/r-user-me` 的 `match.hosts` 含 `user.echo.local` | routes.yaml 未加 hosts |
| （×5 族 × 3 门 = 15 subtest）| 同上 | 同上 |
| `TestStage29D_AllFamiliesRouteCountUnchanged` | ApisixRoute 仍 = 17（16 business + 1 grafana） | 防止误加新路由 |
| `TestStage29D_NoNewCertBeyondFiveFamilies` | Certificate = 6（1 grafana + 5 family）；ApisixTls = 6 | 防止证书蔓延 |

**Commit**：`test(stage-29-D): red — render-assert for 5 family TLS routes`

### 29-D.2 🟢 GREEN：chart 实施

**4 个文件改动**：

| 文件 | 改动 |
|------|------|
| `apisix-routes/values.yaml` | 新增 `tls.enabled` / `tls.clusterIssuer` / `tls.rotationPolicy` / `tls.renewBeforePercentage` / `tls.families[]`（5 元素） |
| `apisix-routes/templates/tls-routes.yaml`（**新文件**） | `range .Values.tls.families` → 每族输出 Certificate + ApisixTls |
| `apisix-routes/templates/routes.yaml` | 顶部构造 `$familyHosts` 字典；每路由 `match:` 内 `{{- if hasKey ... }}` 注入 `hosts:` |
| `values-dev.yaml` | `apisix-routes.tls.enabled: true` |

**关键技术点**：
1. **子图 values 命名空间**：`apisix-routes.tls.enabled`（不是 `apisixRoutes`）——子图读自己命名空间，错的命名 helm 会静默忽略。
2. **Helm template 行尾换行**：`tls-routes.yaml` 末尾 `{{- end }}` 必须**不**吃尾部 newline，否则下一个 doc 的 `---` 会紧贴上 doc 末尾，YAML 解析失败（`did not find expected key`）。
3. **helm dependency build**：umbrella 渲染前需要 `helm dependency build charts/emotion-echo`——否则子图变更不会进入渲染。

**Commit**：`feat(stage-29-D): per-family TLS — 5 Certs + 5 ApisixTls + dual-stack routes`

### 29-D.3 🔴 RED：live smoke

**文件**：`k8s/tests/stage_29d_smoke_test.go`（`//go:build integration`）

| Gate | 检查 |
|------|------|
| `01_cert_manager_controller_available` | pre-flight：controller 必须 Ready |
| `family_<name>_certificate_ready` × 5 | `kubectl wait certificate/<name> -n ee-app` Ready |
| `family_<name>_apisixtls_present` × 5 | `kubectl get apisixtls` 含 name |
| `family_<name>_https_handshake` × 5 | 调 `07-tls-smoke.sh` 带 TLS_HOST env |

**Commit**：`test(stage-29-D): red — 16 live-smoke gates for 5 family TLS`

### 29-D.4 🟢 GREEN：smoke 脚本泛化

**文件**：`k8s/scripts/07-tls-smoke.sh`

改动：
- 新增 `HEALTH_PATH` env（默认 `/api/health` 保留 Grafana 兼容）
- 5 族业务 svc 用 `/health`（各自 env override）

**Commit**：`feat(stage-29-D): generalize 07-tls-smoke.sh — HEALTH_PATH env for 5-family probes`

### 29-D.5 ♻️ REFACTOR + DOCS

**4 个改动**：
1. README badge 29-A → 29-D
2. README status block 加入 29-D 进展
3. 本 landing 文档
4. 不动代码（chart 已 GREEN，无重命名 / 抽象必要）

**Commit**：`docs(stage-29-D): landing — per-family TLS retrofit for 15 routes`

---

## 四、交付物（实际清单）

### Helm chart 改动

| 文件 | 状态 |
|------|------|
| `charts/emotion-echo/charts/apisix-routes/values.yaml` | 加 `tls:` block |
| `charts/emotion-echo/charts/apisix-routes/templates/tls-routes.yaml` | **新增** |
| `charts/emotion-echo/charts/apisix-routes/templates/routes.yaml` | 15 路由注入 `match.hosts` |
| `charts/emotion-echo/values-dev.yaml` | `apisix-routes.tls.enabled: true` |

### 测试改动

| 文件 | 行数 | 角色 |
|------|------|------|
| `k8s/tests/stage_29d_render_test.go` | 255 | render-assert（`k8s` tag） |
| `k8s/tests/stage_29d_smoke_test.go` | 162 | live smoke（`integration` tag） |

### 脚本改动

| 文件 | 改动 |
|------|------|
| `k8s/scripts/07-tls-smoke.sh` | 加 `HEALTH_PATH` env（默认 `/api/health`） |

### 文档改动

| 文件 | 状态 |
|------|------|
| `docs/stage-29-D-tls-all-routes.md` | **新增**（本文） |
| `README.md` | badge 29-A → 29-D + status block |

### commit 链（5 个）

```
test(stage-29-D): red — render-assert for 5 family TLS routes
feat(stage-29-D): per-family TLS — 5 Certs + 5 ApisixTls + dual-stack routes
test(stage-29-D): red — 16 live-smoke gates for 5 family TLS
feat(stage-29-D): generalize 07-tls-smoke.sh — HEALTH_PATH env for 5-family probes
docs(stage-29-D): landing — per-family TLS retrofit for 15 routes
```

---

## 五、Hostname → Secret 映射（dev / self-signed）

| 族 | hostname | ApisixTls + Cert Secret | 路由数 |
|---|---|---|---|
| user | `user.echo.local` | `user-family-tls` | 3 |
| chat | `chat.echo.local` | `chat-family-tls` | 3 |
| analytics | `analytics.echo.local` | `analytics-family-tls` | 1 |
| assessment | `assessment.echo.local` | `assessment-family-tls` | 5 |
| ai | `ai.echo.local` | `ai-family-tls` | 3 |
| (ping) | — | — | 0（HTTP） |
| **总计** | **5 个 hostname** | **5 张 Cert** | **15 路由** |

---

## 六、验证矩阵

| 验证类型 | 命令 | 预期 |
|---------|------|------|
| render-assert | `go test -tags=k8s ./k8s/tests/...` | `TestStage29D_*` 全绿 |
| live smoke | `go test -tags=integration -run TestStage29D_PerFamilyTLSSmoke ./k8s/tests/...` | 16 门全绿（需真实 kind 集群） |
| 手动握手 | `bash k8s/scripts/04-install-chart.sh && bash k8s/scripts/07-tls-smoke.sh` （Grafana） | HTTP 200 |
| 手动握手（族） | `TLS_HOST=user.echo.local HEALTH_PATH=/health bash k8s/scripts/07-tls-smoke.sh` | HTTP 200 |
| helm lint | `helm lint charts/emotion-echo/charts/apisix-routes` | 0 failures |
| helm template | `helm template ee charts/emotion-echo -f values-dev.yaml` | 6 Cert + 6 ApisixTls + 17 ApisixRoute |

---

## 七、不在本次范围（显式 defer）

| 项目 | 原因 | 后续阶段 |
|------|------|---------|
| 切 Let's Encrypt ACME ClusterIssuer | dev 用 self-signed 足够；prod 需 DNS-01 challenge 与公网域名 | Stage 29-E |
| 把 HTTP `:9080` 关闭（强制只走 HTTPS） | 双轨方便灰度；切单轨需运维演练 | Stage 29-D.5 follow-up 或独立 PR |
| `jwt-auth` / `limit-count` / `api-breaker` 插件回填到 Helm `routes.yaml` | 与 TLS 解耦；legacy `deploy/apisix/*.json` 已有 | 独立 PR（与 Stage 29-D 无关） |
| Stage 30 Web BFF | 用户目标未包含 | Stage 30 |
| Stage 31 ACK 迁移 / HPA / PDB / NetworkPolicy | 后续路线 | Stage 31 |
| analytics-svc 业务端点（reports / user-behavior / mental-health） | 用户已选择「仅补 health_handler_test.go」 | 后续独立 TDD cycle |

---

## 八、参考

- `AGENTS.md` §0.1 ALL CODE IS TDD / §1.1 Go 测试栈
- `docs/stage-29-A.5-tls-live-smoke.md` §八（deferral list）
- `docs/stage-29-A-https-grafana.md`（Grafana TLS precedent，本文借鉴其三件套模式）
- `docs/stage-26-T-test-backlog.md`（Stage 26-T 测试 backlog，与本 stage 并行的多 session 推进）
- APISIX + cert-manager 官方：`https://apisix.apache.org/blog/2021/10/22/cert-manager-in-ingress/`
- cert-manager 官方：`https://cert-manager.io/docs/usage/certificate/`
- cert-manager 2024+ renewal：`https://cert-manager.io/docs/usage/certificate/#controlling-when-certificates-are-renewed`
- APISIX 3.x SSL/SNI：`https://www.bookstack.cn/read/apisix-3.7-zh/f9b096a461c54690.md`

---

> 最后更新：2026-08-29 by 当前协作 Agent
> 适用版本：Stage 29-A.5 closure → **Stage 29-D closure**；后续 Stage 29-E（Let's Encrypt）/ Stage 30（Web BFF）/ Stage 31（ACK 迁移）各自独立循环