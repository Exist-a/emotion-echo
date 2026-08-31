# Stage 30 收尾 — APISIX 网关退役说明

> **背景**：Stage 30 Phase A-T7 已完整落地 `emotion-echo-web-bff`（:8894）作为前端唯一入口，承担原 APISIX 网关全部职责（鉴权透传、CORS、聚合 5 下游、SSE 编排、5 个 AI 模型直连）。APISIX 3.9 的 nginx 301 已知 bug（Stage 26-Q 记录）从未被修复，因此原前后端均绕过它走直连。

> **本变更**：删除仓库内所有 APISIX 配套代码 + Helm chart，容器层不再起 apisix/etcd。

---

## 一、删除范围

| 类别 | 路径 | 说明 |
|------|------|------|
| **Compose** | `deploy/docker-compose.infra.yml` 中的 `apisix` + `etcd` service | apisix 仅 APISIX 自用，etcd 仅 APISIX 配置存储；BFF 不依赖两者 |
| **Helm** | `charts/emotion-echo/charts/apisix-routes/` | 16 ApisixRoute + 6 ApisixUpstream + TLS 模板 |
| **Helm** | `charts/emotion-echo/charts/apisix-ingress/` | 独立 ingress controller（与 routes chart 配套） |
| **Helm** | `charts/emotion-echo/charts/etcd/` | 仅 APISIX 配置后端 |
| **配置** | `deploy/apisix/`（含 `seed.sh`、`config.yaml`、`tls/cert/`） | APISIX 路由推送脚本 + 配置 + TLS 证书 |
| **Chart 依赖** | `charts/emotion-echo/Chart.yaml` 中 `apisix-routes`、`apisix-ingress`、`etcd` 依赖 | 删除依赖 + 注释说明 |
| **Values** | `charts/emotion-echo/values.yaml` 中 `apisix-routes`、`apisix-ingress`、`apisixAdminKey` 段 | 注释保留历史背景 |
| **Values** | `charts/emotion-echo/values-dev.yaml` 中 Stage 29-D per-family TLS 配置 | 注释保留历史背景 |

## 二、保留范围（不改）

| 类别 | 路径 | 说明 |
|------|------|------|
| **文档** | `docs/stage-26-Q-apisix-fix.md` | 历史 APISIX bug 调查 |
| **文档** | `docs/stage-29-D-tls-all-routes.md` | 历史 TLS retrofit 规划（虽然 APISIX 已退役，文档保留为演进参考） |
| **文档** | `docs/distributed-architecture.md`、`docs/microservices-architecture.md` | 提及 APISIX 网关的历史背景段落保留 |
| **代码** | `emotion-echo-shared/pkg/middleware/gin_auth.go` | shared 中间件继续提供 JWT 解析（APISIX 也复用），BFF 用 `bffAuthMiddleware` 替代 |
| **代码** | `.zcode/plans/*.md` | plan 文件提及 APISIX 作演进参考，保留 |

## 三、BFF 网关替代验证

BFF 完整替代了原 APISIX 网关的全部职责：

| 原 APISIX 职责 | BFF 实现 |
|-----------------|----------|
| 统一入口（路由） | `emotion-echo-web-bff` :8894 |
| JWT 鉴权 | `bffAuthMiddleware` + shared `GinAuthMiddleware`（共享同套 JWT 解析） |
| CORS | `corsMiddleware`（main.go） |
| 限流（user 级） | 当前未实现（**未来按需补**：stage-30-web-bff.md §八风险表标注 BFF 单点，可在此处加分桶限流中间件） |
| 熔断 | 当前未实现（同上，按需补） |
| 聚合 5 下游 + LT8S/Chat/Assessment/Analytics/AI | `internal/downstream/` 7 个 client |
| SSE 流式编排 | `internal/handler/ai_stream_handler.go`（OpenAI 兼容） |
| 真实 LLM 对话 | DeepSeek/OpenAI 兼容，env `BFF_LLM_API_KEY` 注入 |

## 四、验证

```bash
# 1. compose 起全栈
docker compose -f deploy/docker-compose.infra.yml -f deploy/docker-compose.apps.yml up -d --build

# 2. 验证容器列表（无 apisix/etcd）
docker ps --format '{{.Names}}' | grep -E "apisix|etcd" || echo "✓ apisix/etcd 已无"

# 3. helm lint / template
helm lint charts/emotion-echo  # 0 failed
helm template test charts/emotion-echo | grep "kind:" | sort | uniq -c

# 4. 浏览器全链路测试（已通过，见 Phase C 报告）
# - 登录 → BFF mock JWT 签发 → 跳转聊天页
# - 发消息 → BFF → chat-svc 落库
# - AI 流式回复 → BFF → DeepSeek (或 mock fallback)
```

## 五、向后兼容 / 后续恢复路径

如果未来需要恢复网关层（限流/熔断/路由灰度），两条路：

1. **轻量**：在 BFF 中间件链加分桶限流（`golang.org/x/time/rate`）+ `sony/gobreaker` 熔断 → BFF 自身做
2. **重量**：重新引入 APISIX/Caddy 等边缘网关，BFF 仍是 backend 聚合层；选择 3.10+（绕过 3.9 nginx 301 bug）

Stage 30 当前选路 1（轻量在 BFF），需要边缘层时再升级到路 2。

---

> 变更完成时间：Stage 30 收尾（2026-08-31）  
> 对应 commit：`feat(chore): remove APISIX — BFF 替代网关职责`
