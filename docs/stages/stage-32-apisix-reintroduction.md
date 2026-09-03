# Stage 32 · APISIX API 网关层回归

> **配套文档**：`adr-2026-09-nacos-reintroduction.md`、`stage-31-nacos-reintroduction.md`（前置）、`stage-33-p0-fix-bff-purify.md`（后续）。

---

## 一、目标

1. 启动独立 API 网关层（APISIX 3.18 + etcd v3.5），**与 BFF 解耦**；
2. 全局插件链落地：jwt-auth（真正验签）/ limit-count / limit-req / api-breaker / cors / prometheus；
3. **upstream 通过 APISIX `nacos-discovery` 插件自动发现**（企业级）：BFF + 6 后端 svc 都注册到 Nacos，APISIX 自动拉实例 + grpc 健康检查自动摘除；
4. 16 条路由复用根目录 `apisix-*.json` 残存文件（Stage 30 退役时未清理，可作为 route seed）；
5. 修复 Prometheus configmap 的死 scrape job（`charts/emotion-echo/charts/prometheus/templates/configmap.yaml:71`）；
6. BFF 退出网关职责（`bffAuthMiddleware`/`corsMiddleware` 移除），仅保留 APISIX 注入的 X-User-Id 透传；
7. **默认前置 nginx:alpine 反代终结 TLS**（dev 与 prod 一致），APISIX 仅作 HTTP 网关。

---

## 二、关键风险：APISIX 3.9 SSL bug 仍未修复

### 2.1 核实结论

通过 `https://github.com/apache/apisix/blob/release/3.18/CHANGELOG.md` 核实：

- **3.10–3.18 全段 changelog 无 `ssl_certificate_by_lua_block` / 301 redirect / `ngx_tpl.lua` 相关修复条目**；
- bug 表现：APISIX 3.9.0-debian 镜像 `ngx_tpl.lua` 在 `ssl.enable=false` 时仍会注入 `ssl_certificate_by_lua_block`，导致 `/etc/resolv.conf` 等路径返回 301 重定向到 `about:blank`；
- 官方 changelog 已知未修（Stage 32 启动时跳过实测——决策"不论复现与否都启 nginx 前置"，实测无新增决策价值）。

### 2.2 绕过方案（默认启用 nginx 前置）

不论 dev 或 prod，APISIX 之前都加一层 `nginx:alpine` 反代：

| 流量 | 路径 |
|------|------|
| 浏览器 → nginx:443 (TLS 终结) → APISIX:9080 (纯 HTTP) → BFF:8894 → 下游 |
| APISIX 自身仅暴露 :9080 HTTP，TLS 不进入 APISIX 数据面，绕开 `ssl_certificate_by_lua_block` bug |

dev 模式下 nginx 反代可选择 `profile: tls` 不启用（直连 APISIX:9080 演示），prod 默认启用。**统一形态**：dev 默认 profile 直连 APISIX，prod profile 走 nginx 反代——bug 在两种形态下都不触发。

---

## 三、改动清单

### 3.1 compose 编排（`deploy/docker-compose.infra.yml`）

新增 3 个 service（顺序：etcd → apisix → apisix-dashboard）：

```yaml
etcd:
  image: quay.io/coreos/etcd:v3.5.13
  container_name: emotion-echo-etcd
  command:
    - etcd
    - --advertise-client-urls=http://etcd:2379
    - --listen-client-urls=http://0.0.0.0:2379
    - --data-dir=/etcd-data
  volumes: [etcd-data:/etcd-data]
  networks: [app-network]
  healthcheck:
    test: ["CMD", "etcdctl", "endpoint", "health"]
    interval: 10s

apisix:
  image: apache/apisix:3.18.0-debian
  container_name: emotion-echo-apisix
  depends_on:
    etcd: { condition: service_healthy }
  volumes:
    - ../deploy/apisix/config.yaml:/usr/local/apisix/conf/config.yaml:ro
  ports:
    - "9080:9080"      # HTTP data plane（dev 用）
    - "9180:9180"      # Admin API（仅 dev 暴露）
    - "9091:9091"      # prometheus exporter
  networks: [app-network]

apisix-dashboard:
  image: apache/apisix-dashboard:3.18.0-alpine
  container_name: emotion-echo-apisix-dashboard
  depends_on: [apisix]
  ports:
    - "9000:9000"      # dashboard UI（仅 dev）
  networks: [app-network]

# 可选：prod TLS 场景启用
nginx:
  image: nginx:alpine
  container_name: emotion-echo-nginx-proxy
  profiles: ["tls"]    # 默认不启动
  volumes:
    - ../deploy/tls/certs:/etc/nginx/certs:ro
    - ../deploy/nginx/nginx.conf:/etc/nginx/nginx.conf:ro
  ports:
    - "443:443"
  networks: [app-network]
```

### 3.2 APISIX 配置（`deploy/apisix/config.yaml`）

```yaml
apisix:
  node_listen: 9080
  enable_ipv6: false
deployment:
  role: traditional
  role_traditional:
    config_provider: etcd
etcd:
  host:
    - http://etcd:2379
  prefix: /apisix
plugin_attr:
  prometheus:
    export_addr:
      ip: 0.0.0.0
      port: 9091
```

### 3.3 路由与插件（`deploy/apisix/seed.sh`）

向 APISIX Admin API (`http://apisix:9180/apisix/admin/routes`) 推送：
- **7 个 upstream**（user / chat / assessment / analytics / ai-svc / **web-bff** / emotion-llm-service）—— BFF 也参与服务发现，是 APISIX 的 upstream 之一；
- 16 路由（按 path 前缀聚合到对应 upstream）；
- 全局插件链在 route 级别挂载。

**upstream 配置：使用 `nacos-discovery` 插件自动发现**（企业级形态）：

```json
{
  "uri": "/api/v1/bff/*",
  "upstream": {
    "type": "service",
    "service_name": "web-bff",
    "discovery_type": "nacos",
    "namespace_id": "emotion-echo-dev",
    "group_name": "DEFAULT_GROUP"
  },
  "plugins": {
    "nacos-discovery": {
      "namespace_id": "emotion-echo-dev",
      "group_name": "DEFAULT_GROUP",
      "service_name": "web-bff"
    },
    "jwt-auth": { ... },
    "limit-count": { ... },
    "api-breaker": { ... }
  }
}
```

- APISIX 每 30s 从 Nacos 拉实例列表；多副本自动负载均衡；
- grpc 健康检查：连续 3 次失败自动摘除；
- 7 个 svc 全部走 `nacos-discovery`（user-svc / chat-svc / assessment-svc / analytics-svc / ai-svc / web-bff / emotion-llm-service）；
- 静态 upstream 仅作为**兜底**（Stage 34+ 评估"全 discovery" vs "discovery+fallback 混合"）。

**Nacos 配置中心范围（与 Stage 31 对齐）**：仅放**运营参数**——feature flag、限流阈值、模型路由表、Kafka 重试次数、A/B 分组等。`etc/*.yaml` 仍是启动默认值，Nacos 仅覆盖。**JWT secret、DATABASE_DSN 等敏感配置不进 Nacos**（避免密钥明文存 Nacos）。

**复用根目录 `apisix-*.json`**：stage-30 退役时留下的 10 个 `apisix-{ai-up,r-ai,...}.json` 结构可直接转换为 APISIX Admin API PUT body，仅需把 `upstream.nodes[].host` 从 `host.docker.internal` 改为 `nacos-discovery` 引用。

### 3.4 全局插件链（每个 route 共享）

| 插件 | 配置 | 作用 |
|------|------|------|
| `nacos-discovery` | `namespace_id: emotion-echo-dev, group_name: DEFAULT_GROUP, service_name: {svc}` | 自动从 Nacos 拉实例 + 健康检查 |
| `jwt-auth` | `key: user, secret: ${BFF_JWT_SECRET}` (从 Nacos 运营参数拉取) | 真正验签（替换 shared `jwt_auth.go` 的"信任 APISIX"模型） |
| `limit-count` | `count: 60, time_window: 60, key: remote_addr, policy: local` | 按 IP 限流 60/min（**阈值从 Nacos 运营参数拉取，热更新**） |
| `limit-req` | `rate: 1000, burst: 100, key: remote_addr, policy: local` | 全局 1000/s 突发 |
| `api-breaker` | `break_response_code: 503, min_requests: 20, error_threshold_ratio: 0.5, open_time: 30` | 下游 5xx > 50% 熔断 30s |
| `cors` | `allow_origins: ["http://localhost:3000"], allow_methods: ["GET","POST","PUT","DELETE","OPTIONS"], allow_credentials: true` | CORS 限定 |
| `prometheus` | 默认 | 上报 metrics 到 OAP |

### 3.5 BFF 改造

**删除**：
- `emotion-echo-web-bff/main.go` 的 `bffAuthMiddleware` 与 `corsMiddleware`；
- `emotion-echo-web-bff/etc/web-bff.yaml` 的 `Auth.JWTSecret` 字段（由 APISIX 管理）；
- `deploy/docker-compose.apps.yml` BFF 的 `BFF_JWT_SECRET` env（由 APISIX secret 替代）。

**保留**：
- shared `pkg/middleware/jwt_auth.go`：改为"信任 APISIX 注入 X-User-Id" 模式（APISIX jwt-auth 插件验签后通过 `X-User-Id` header 透传）；
- 所有 `downstream/*` client 的 `applyAuth` 行为不变（仍透传 X-User-Id 给后端 svc）；
- SSE 流式编排逻辑不变。

**TDD 约束**：BFF 改动必须先有"无 Authorization header 仍能处理 X-User-Id"的失败测试，再改实现。

### 3.6 Helm chart

新增 `charts/emotion-echo/charts/apisix/` subchart：
- `deployment.yaml`（apisix + dashboard sidecar）
- `service.yaml`（ClusterIP 9080/9180/9091）
- `configmap.yaml`（config.yaml 模板）
- `etcd` 子 chart（StatefulSet + PVC）
- `values.yaml`（`enabled: true`）

修复 Prometheus configmap 死 scrape job：`configmap.yaml:71` 的 `apisix-gateway.ee-system.svc.cluster.local:9091` 改为新 subchart 的 Service FQDN。

---

## 四、验证

### 4.1 单元测试

```bash
# shared jwt 中间件：信任 X-User-Id 模式
go test ./emotion-echo-shared/pkg/middleware/...
# BFF 移除鉴权中间件后仍能工作
go test ./emotion-echo-web-bff/...
```

### 4.2 compose 端到端

```bash
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d --build

# 1. APISIX dashboard 登录 http://localhost:9000 (admin/admin) 可见 6 upstream + 16 route
# 2. 正常路径：浏览器 → APISIX :9080 → BFF :8894 → 下游
curl -H "Authorization: Bearer <valid-jwt>" http://localhost:9080/api/v1/user/health
# 期望 200

# 3. 伪造 JWT 被拒
curl -H "Authorization: Bearer a.eyJ1c2VyX2lkIjoxfQ.b" http://localhost:9080/api/v1/user/health
# 期望 401（APISIX jwt-auth 拒绝）

# 4. 限流触发
for i in $(seq 1 70); do curl http://localhost:9080/api/v1/user/health; done
# 期望前 60 次 200，之后 429

# 5. 熔断触发（下游全挂）
docker stop emotion-echo-user-svc
for i in $(seq 1 30); do curl http://localhost:9080/api/v1/user/profile; done
# 期望连续 503 → api-breaker 触发 → 后续 fast-fail
```

### 4.3 Helm 渲染

```bash
helm lint charts/emotion-echo
helm template test charts/emotion-echo | grep "kind:" | sort | uniq -c
# 期望看到 Deployment、Service、ApisixRoute、StatefulSet 全部渲染无报错
```

---

## 五、风险与缓解

| 风险 | 缓解 |
|------|------|
| APISIX 3.9 SSL bug 仍未修 | dev 走纯 HTTP；prod 前置 nginx 反代终结 TLS；运维提示 |
| Dashboard 仅 dev 暴露 | prod 不映射 9000 端口 |
| BFF 与 APISIX 重复鉴权可能冲突 | shared 中间件切到 X-User-Id 模式，不再解析 Authorization |
| Etcd 单节点数据丢失 | dev 足够；prod 改 3 节点集群（Stage 34+） |
| 路由配置推送到 Nacos 还是 APISIX Admin API | 本 Stage 用 Admin API（直推 etcd）；Nacos discovery 由 APISIX plugin 消费（Stage 34+ 整合） |

---

## 六、不做的事

- 不修 P0 业务（A-1 SSE 协议、R-2 落库路径）—— 留给 Stage 33
- 不接 Nacos discovery（APISIX upstream 用静态节点，本地 compose 容器名；Nacos 集成留给 Stage 34+）
- 不上 TLS 证书到 dev（仅文档约定路径）
- 不改 BFF 业务逻辑（仅删除网关职责）
- 不动前端协议（Stage 33 改前端解析）

---

> 阶段计划完成时间：2026-09 启动（依赖 Stage 31）  
> 预计 PR 数：~4（compose + Helm + APISIX 配置 + BFF 改造）  
> 收口条件：APISIX dashboard 可见完整路由 + 伪造 JWT 被拒 + 限流/熔断可触发 + Helm 渲染通过