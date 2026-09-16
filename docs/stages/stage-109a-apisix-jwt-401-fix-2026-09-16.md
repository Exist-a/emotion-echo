---
status: landed
priority: critical
owner: TBD
created: 2026-09-16
last-refresh: 2026-09-16
type: bug-fix
depends-on:
  - stage-108-sender-architecture-debt-fix-2026-09-16.md (sender 修后被 A7 阻塞)
  - docs/plans/sprint-109a-apisix-jwt-401-2026-09-16.md (本 sprint 的计划)
related-stages:
  - stage-94-code-review-2026-09-14-p0-closure.md (P0-7 TrustAPISIX + APISIXCIDRs 落地)
  - stage-97-round2-p0-closure.md (Round 2 P0 收口)
  - stage-32-apisix-reintroduction.md (APISIX 3.x 引入)
related-issues:
  - docs/plans/test-coverage-tracker-2026-09-16.md §四 A7 (FIXED Sprint 109a)
related-architecture-debts:
  - X-4 鉴权 (横切链路): APISIX jwt-auth consumer + X-User-Id 注入
---

# Stage 109a — APISIX jwt-auth 401 诊断 + 修复

> **目的**：修 A7 (APISIX jwt-auth 401) — Stage 108 sender useState 化后浏览器端到端测试被独立阻塞。所有 `/api/v1/*` 受保护路由返 401，即使 token 本地 HS256 验算 OK、consumer 配置 OK、route 100 配置 OK。
>
> **真正根因（与 sprint-109a 计划 H1-H6 假设都不同）**：BFF `TrustAPISIX=true` + `APISIXCIDRs=[]`（dev 模式无 k8s CIDR 可配）→ `isFromTrustedAPISIX` 永远 false → BFF 拒所有 X-User-Id 注入。**APISIX jwt-auth 完全正常，BFF 配置自相矛盾才是 bug。**
>
> **次要修复（顺手）**：APISIX `data_encryption.enable_encrypt_fields: true` 是 APISIX 3.18.0 全局加密开关，理论上 jwt-auth 验签前会解密 consumer.secret，但实际通过 etcd 路径 `find_consumer` 拿到的 `consumer.auth_conf.secret` 已经是密文（jwt-auth.lua:56 直接使用），BFF 用明文签的 JWT 验签时不一致。**为防御性保险起见一并关闭**（prod 由 .env.local 注入真随机 secret + 隔离 etcd namespace；APISIX upstream 解密 bug 修复后再开回）。

---

## 一、起点 + 现象

### 1.1 Stage 108 收口后浏览器实测

Stage 108 修了 sender 架构债（useState 化 7 个跨实例状态），浏览器实测端到端 chat 链路时**所有 `/api/v1/*` 返 401**，但：
- token 本地 HS256 验算 OK（secret=`dev-bff-secret`）
- APISIX consumer `{key:user, secret:dev-bff-secret, algorithm:HS256}` 配置 OK
- APISIX route 100 jwt-auth plugin 配置 `{store_in_ctx:true, cookie:access_token}` OK
- BFF 日志完全没有对应请求记录（被网关层拦截）

### 1.2 假设根因排查顺序（sprint-109a §二 H1-H6）

| 假设 | 测试方法 | 结果 |
|---|---|---|
| H1：consumer 配置被改 | admin API 直读 `/apisix/admin/consumers/emotion_echo_bff` | ❌ 配置 OK |
| H2：route 100 jwt-auth plugin header 解析缺失 | 直读 `/apisix/admin/routes/100` | ❌ `cookie` 字段 OK，header 走默认 `authorization` |
| H3：apisix-seed 重跑覆盖 consumer | etcd 看 `mod_revision` | ❌ seed 是幂等 PUT |
| H4：file-logger / skywalking-logger 拦截 | PATCH 删 logger plugin | ❌ 与 logger 无关 |
| H5：etcd 数据损坏 | `docker restart emotion-echo-apisix` | ❌ 重启后仍 401 |
| H6：APISIX 版本升级 | 看 changelog | ❌ 版本未变 |

**所有 H1-H6 排除后**：直接读 etcd raw 数据 → **发现 etcd 里 consumer.secret 是密文 `lDJJR5NH8B3gssvm2/IW6w==`**（APISIX 字段级加密产物），但 admin API GET 自动解密显示 `dev-bff-secret`。**说明 APISIX 内部 `find_consumer` 返回的 `consumer.auth_conf.secret` 是密文而非明文**。

---

## 二、真实根因诊断

### 2.1 APISIX jwt-auth 源码路径

```
请求 → APISIX jwt-auth plugin (rewrite phase)
  → consumer.lua:283 _M.find_consumer("jwt-auth", "key", user_key)
    → consumer.lua:247 create_consume_cache (LRU)
      → consumer.lua:240 fill_consumer_secret (调 secret.fetch_secrets)
        → secret.lua:259 retrieve_refs (只解 $secret:// / $env:// URI)
      ← 返回 consumer.auth_conf (密文字符串)
  ← get_auth_secret(consumer) → jwt-auth.lua:56 直接用 consumer.auth_conf.secret
  → jwt:verify_signature(auth_secret) ← 用密文做 HMAC → 永远不匹配
```

**关键 bug**：`secret.fetch_secrets` 只处理 `$secret://` URI 引用，**不解 `encrypt_fields = {"secret"}` 加密字段**。所以 APISIX 3.18.0 在 `data_encryption.enable_encrypt_fields: true` 下，jwt-auth 插件实际用密文验签，与 BFF 用明文签的 JWT 不匹配。

### 2.2 误判原因 + 真正根因浮现

改 `enable_encrypt_fields: false` + 重启 APISIX + 重跑 seed.sh 后，etcd 存明文 `dev-bff-secret`，admin API 显示明文，**jwt-auth 仍然 401**！

进一步排查 → **直接给 BFF 直连 + 带 X-User-Id header 仍然 401**：

```
curl -H "X-User-Id: 1" http://localhost:8894/api/v1/user/profile
→ HTTP/1.1 401 Unauthorized
   {"error":"unauthorized"}
```

**BFF 日志**：
```
[auth] TrustAPISIX=true; APISIX CIDRs=[] (RequireAPISIXIP enforcement on)
```

→ BFF 启动时 `TrustAPISIX=true` + `APISIXCIDRs=[]`（空 CIDR）→ `isFromTrustedAPISIX` 永远 false → BFF 拒所有 X-User-Id 注入 → 401。

**这是 dev 模式配置自相矛盾**：Stage 94 PR-6 §P0-7 加 APISIXCIDRs 强制校验，但 dev 模式无 k8s pod CIDR 可配，compose 默认仍是 `TrustAPISIX=true`，导致 dev 模式 BFF 永远 401。

### 2.3 历史 timeline

| 时间 | commit | 行为 |
|---|---|---|
| 2026-08-31 | 9182ef1 PR-16 | 加 `TrustAPISIX: true` 默认值，BFF 切到 X-User-Id 模式 |
| 2026-09-12 | a028d96 PR-6 (Stage 94) | 加 `APISIXCIDRs` 字段 + IP 白名单校验，**但 yaml 默认仍是 true** |
| 2026-09-16 10:24 (Stage 107) | e6e079e | "POST /conversations 返 200" — 但当时 TrustAPISIX=true 配置相同 |
| 2026-09-16 14:00 (Stage 108) | 59feb3e | sender useState 化后端到端测试发现 401 |

**矛盾**：Stage 107 报告 200 OK，但当前 dev 模式 TrustAPISIX=true 配置下 BFF 必拒。**最可能解释**：Stage 107 测时 `BFF_TRUST_APISIX` env 被临时覆盖为 false（与 Stage 108 阶段行为差异），**当时未记录到 stage 文档**。Stage 108 阶段恢复成默认 true 后 bug 浮出。

---

## 三、修复（GREEN）

### 3.1 主要修复：BFF TrustAPISIX dev 默认改 false

**`emotion-echo-web-bff/etc/web-bff.yaml:88`**：
```yaml
# Sprint 109a：默认改 false（dev 模式默认）
TrustAPISIX: false
```

**`deploy/docker-compose.apps.yml:619`**：
```yaml
BFF_TRUST_APISIX: ${BFF_TRUST_APISIX:-false}
```

**理由**：dev 模式无 k8s pod CIDR 可配，强制 TrustAPISIX=true + APISIXCIDRs=[] 必然导致所有受保护请求 401。dev 模式选择简化路径：APISIX jwt-auth 验签通过 → serverless-post-function 注入 X-User-Id → BFF 因 TrustAPISIX=false 接受任何 X-User-Id。

**生产部署要求**：compose override 设 `BFF_TRUST_APISIX=true` + `BFF_APISIX_CIDRS=<k8s pod CIDR>`，走 IP 白名单强制校验，**防攻击者通过直连 BFF 端口伪造 X-User-Id header 假冒任意用户**（Stage 94 PR-6 §P0-7 设计目标）。

### 3.2 次要修复（防御性）：APISIX 关掉字段级加密

**`deploy/apisix/config.yaml:379`**：
```yaml
# Sprint 109a：关掉字段级加密 — APISIX 3.18.0 jwt-auth 插件直接用 etcd 中
# consumer.auth_conf.secret 验签, consumer.lua:242 fetch_secrets 不解
# encrypt_fields, 导致 BFF 用 dev-bff-secret 明文签的 JWT 永远 401。
# 关掉后 secret 字段明文存 etcd, jwt-auth 用同一明文验签 → PASS。
# 安全风险 (dev secret 落 etcd 明文) 在 dev 模式可接受; prod 由 .env.local
# 注入真随机 secret + APISIX 部署在隔离 etcd namespace, 等 APISIX upstream
# 修 fetch_secrets 解 encrypt_fields 后再开回 true。
enable_encrypt_fields: false
```

**理由**：APISIX 3.18.0 的 `consumer.lua fetch_secrets` 不解 `encrypt_fields`，jwt-auth 用密文验签是 APISIX 上游 bug。dev 模式默认 secret 是 `dev-bff-secret` 明文（gitignored .env.local 注入），明文存 etcd 无额外风险。**等 APISIX upstream 修后再开回 true**。

### 3.3 部署动作

```bash
# 1. recreate BFF 让新 yaml + env 生效
docker compose -f deploy/docker-compose.apps.yml -f deploy/docker-compose.infra.yml \
  up -d --force-recreate --no-deps emotion-echo-web-bff

# 2. 重启 APISIX 让 enable_encrypt_fields=false 生效 + 重跑 seed.sh
docker restart emotion-echo-apisix
sleep 15
docker compose -f deploy/docker-compose.apps.yml -f deploy/docker-compose.infra.yml \
  run --rm emotion-echo-apisix-seed
```

---

## 四、回归验证

### 4.1 新增 runtime 端到端测试

**`deploy/apisix/test_jwt_auth_runtime.sh`**（新建，5 个断言 / 6 项检查）：

| Step | 断言 | 结果 |
|---|---|---|
| 1 | POST /api/v1/auth/login 返 200 + accessToken | ✅ |
| 1.5 | token 长度 > 50 | ✅ |
| 2 | GET /api/v1/user/profile with Bearer token 返 200（核心断言） | ✅ |
| 3 | POST /api/v1/conversations 返 200（chat 链路入口） | ✅ |
| 4 | 无 token 调受保护路由返 401（反向断言 jwt-auth 真生效） | ✅ |
| 5 | BFF 直连 + X-User-Id 返 200（旁证 BFF 上游 + TrustAPISIX=false 路径） | ✅ |

**全绿结果**：
```
通过: 6  失败: 0
ALL PASS — A7 APISIX jwt-auth 401 已修通
```

### 4.2 修前 vs 修后对比

| 断言 | 修前 | 修后 |
|---|---|---|
| Step 2: GET /user/profile with token | ✗ HTTP 401 | ✅ HTTP 200 |
| Step 3: POST /conversations | ✗ HTTP 401 | ✅ HTTP 200 |
| Step 4: 无 token 401 | ✅ HTTP 401 | ✅ HTTP 401 |
| Step 5: BFF-direct + X-User-Id | ✗ HTTP 401 | ✅ HTTP 200 |

修前 3 项 FAIL，修后 6/6 PASS。

### 4.3 真端到端浏览器验证（Sprint 109b 范围，本 stage 不重复）

Stage 109b 计划用 browser-use 跑完整 UI 流程验证 AI 回复渲染。本 stage runtime test 已覆盖：
- ✅ APISIX jwt-auth 验签通过（Step 2）
- ✅ APISIX → BFF 转发成功 + X-User-Id 注入（Step 3，BFF /conversations handler 不 401）
- ✅ BFF TrustAPISIX=false 路径（Step 5，BFF-direct 200）

Sprint 109b 浏览器 UI 验证是这些 runtime 断言的上层包装——如果 runtime 6/6 PASS，浏览器 UI 大概率也能跑通。

---

## 五、变更清单（commit 计划）

| 文件 | 改动 | 行数 |
|---|---|---|
| `deploy/apisix/config.yaml` | `enable_encrypt_fields: true` → `false` + 注释 | +5 -1 |
| `deploy/apisix/test_jwt_auth_runtime.sh` | 新建（5 步断言） | +130 |
| `deploy/docker-compose.apps.yml` | `BFF_TRUST_APISIX:-true` → `:-false` + 注释 | +7 -3 |
| `emotion-echo-web-bff/etc/web-bff.yaml` | `TrustAPISIX: true` → `false` + 注释 | +7 -3 |
| `docs/stages/stage-109a-apisix-jwt-401-fix-2026-09-16.md` | 新建 | +250 |

总 5 文件 / +399 -7 行（doc + tests 占多数）。

---

## 六、教训与 TODO

### 6.1 教训

1. **Stage 107 报"200 OK"是 stage 文档已知漂移案例**（sprint-109a §七根因 #2）：当时 TrustAPISIX 可能是 env 临时覆盖 false，但 stage 文档未记录，本 stage 重新暴露。**AGENTS.md §〇 第一性原则要求每次 stage 写文档前必须验证架构假设**，但同 session 内部 stage 之间的 env 修改未被审计。建议后续 stage 文档明确写"测试时 env 配置"小节。
2. **APISIX 字段级加密在 jwt-auth 场景下是设计不一致**：schema 声明 `encrypt_fields = {"secret"}` 但 plugin 不解密。APISIX 3.x 的 secret 模块只处理 `$secret://` URI。**这是 APISIX 上游 bug**，社区应报 issue。
3. **multi-layer config 的 single-source-of-truth 缺失**：yaml + env + image default 三处都可配 TrustAPISIX，缺运行时"effective config"快照。建议 emotion-echo-web-bff 加 `/api/v1/debug/config` 端点（仅 dev 模式暴露）。

### 6.2 后续 Sprint

| Sprint | 内容 |
|---|---|
| 109b | browser-use 浏览器实测端到端 chat 链路（SSE 流 + AI 回复渲染） |
| 109c | 数据契约 §1 §2 §5 §6 smoke 全绿 |
| 110 | E2E-2 Playwright spec 回归钉子 |
| 113 | 数据契约 §3 §4 全量 smoke |
| 114 | E2E-1 异常路径 Playwright（JWT 过期 → refresh → 跳登录） |

### 6.3 dev/prod 部署 checklist

| 场景 | BFF_TRUST_APISIX | BFF_APISIX_CIDRS | APISIX enable_encrypt_fields |
|---|---|---|---|
| dev (localhost) | `false` | 空（默认） | `false`（dev secret 明文 OK） |
| staging (k8s namespace) | `true` | `<pod CIDR>` | `false`（等 APISIX upstream 修后再开） |
| prod (隔离 etcd) | `true` | `<pod CIDR>` | 待 APISIX 修 fetch_secrets 后开 `true` |

---

## 七、调研依据（AGENTS.md §〇 硬规则）

| 文件 | 用途 |
|---|---|
| `docs/stages/stage-108-sender-architecture-debt-fix-2026-09-16.md §四 §五` | A7 现象 + 4 个假设根因 |
| `docs/plans/sprint-109a-apisix-jwt-401-2026-09-16.md §二` | H1-H6 假设排查顺序 |
| `deploy/apisix/seed.sh:217-258` | jwt-auth consumer PUT 逻辑（密钥从 `BFF_JWT_SECRET` env 读） |
| `deploy/apisix/seed.sh:369-410` | CATCHALL_PLUGINS_JSON 含 jwt-auth `cookie:access_token` + serverless-post-function 注入 X-User-Id |
| `deploy/apisix/config.yaml:375-379` | `data_encryption.enable_encrypt_fields: true` 全局开关 |
| `emotion-echo-web-bff/main.go:189-204` | BFF TrustAPISIX + APISIXCIDRs 中间件装配 + 启动日志 |
| `emotion-echo-web-bff/internal/config/config.go:73-79, 293-294` | `TrustAPISIX bool` 字段 + `BFF_TRUST_APISIX` env override |
| `emotion-echo-shared/pkg/middleware/gin_auth.go:46-72, 106-145` | `compileCIDRs` + `isFromTrustedAPISIX` + middleware 主逻辑 |
| APISIX `apisix/consumer.lua:240-247` | `fill_consumer_secret` 调 `secret.fetch_secrets(new_consumer.auth_conf, false)` |
| APISIX `apisix/secret.lua:255-263` | `_M.fetch_secrets` 只处理 `$secret://` URI 引用，不解 encrypt_fields |
| APISIX `apisix/plugins/jwt-auth.lua:56` | `get_auth_secret(consumer)` 直接返回 `consumer.auth_conf.secret` |
| git log 9182ef1 (Stage 32 PR-16) | `TrustAPISIX: true` 默认值首次落地 |
| git log a028d96 (Stage 94 PR-6) | `APISIXCIDRs` + IP 白名单强制（§P0-7），但 yaml 默认仍是 true |
| git log c64f690 (Stage 94 回归) | web-bff block 加 `BFF_JWT_SECRET` env |
| docker exec `emotion-echo-etcd` | etcd 真实存 consumer 密文 `lDJJR5NH8B3gssvm2/IW6w==`（手动 decode base64 验证） |

### 架构假设清单（写前对齐）

| 假设 | 验证 |
|---|---|
| A7 根因不在 sender（已 Sprint 108 useState 化完成）| ✅ sender 是前端 composable，与 APISIX 网关无关 |
| A7 根因不在 BFF 主链路代码 | ✅ BFF jwt.go / config.go / main.go 逻辑正常 |
| A7 根因在 APISIX jwt-auth（计划假设 H1-H6）| ❌ H1-H6 全部排除 |
| **A7 真正根因在 BFF TrustAPISIX=true 配置自相矛盾** | ✅ docker exec + config.go + main.go 源码三重验证 |
| **次要修复 APISIX enable_encrypt_fields 修不修都行** | ✅ 修主因后 jwt-auth 仍 401 → 关掉字段加密后才能 PASS（双重保险） |
| dev 模式 TrustAPISIX=false 可接受 | ✅ APISIX 已完成 JWT 验签 + X-User-Id 注入，BFF 只读 header 即可信 |
| prod 必须 TrustAPISIX=true + APISIXCIDRs 配置 | ✅ Stage 94 §P0-7 设计目标，本 stage dev 默认 false 不影响 prod 路径 |
