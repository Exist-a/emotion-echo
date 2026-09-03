---
status: landed
date: 2026-09-03
branch: fix/stage-36-post-test-cleanup (HEAD = ff9b3b9)
---

# Stage 38-A · dev 走 APISIX 端到端打通（Landing Report）

> 状态：**dev 模式端到端跑通** · 日期：2026-09-03
> 关联 commit：`ff9b3b9` + 5 个手 PUT APISIX 容器配置（不持久化，重启需重跑）

---

## 一、目标

用户决策：dev 模式前端走 APISIX（`localhost:19080` → BFF `localhost:8894`），不走 BFF 直连。

理由：dev / 生产路径一致；CORS / JWT 注入 / 限流 / 熔断全由 APISIX 一手包；BFF 回到"纯聚合层"原始定位（main.go 注释也是这么写的）。

---

## 二、为什么走 APISIX 反复触礁（10 个连续失配）

每次"修一个"立刻暴露下一个，按发生顺序：

| # | 失配 | 真因 |
|---|---|---|
| 1 | APISIX 启动后 routes 全空 | etcd 不持久化配置，容器重启只保留 etcd volume（deploy/docker-compose.infra.yml 里有 etcd-data）；实际看到 routes=0/upstreams=0/global_rules=0 → seed.sh 没自动重跑 |
| 2 | seed.sh 跑失败（route 110 起） | `cors.allow_origins` / `allow_methods` / `allow_headers` 用 JSON 数组，APISIX 3.18 schema 期望**字符串**（逗号分隔） |
| 3 | consumer 没注册 → jwt-auth `missing user key` | seed.sh 设计只配 plugin，**没有 `PUT /consumers`** |
| 4 | consumer 配上后仍 `missing user key` | BFF token payload 缺 `key` claim——`json:"key,omitempty"` 把零值吃了 |
| 5 | 修后变 `Invalid user key` | APISIX jwt-auth `key=user` 表示**从 token 的 `key` claim 读 credential key**；BFF token `key` 是 string "user" 但被解释为 int 3 → 值不匹配 |
| 6 | consumer 改后 `missing X-User-Id` | APISIX jwt-auth 通过后**不自动注入 X-User-Id**——只注入 `X-Consumer-Username` / `X-Credential-Identifier` |
| 7 | proxy-rewrite `X-User-Id: $jwt_claim_user_id` 不生效 | APISIX 3.18 `proxy-rewrite` 只解析 nginx 变量表；`jwt_auth_payload` 没注册成 nginx var → `$ctx.jwt_auth_payload.user_id` / `$jwt_claim_user_id` 都被当字面量 |
| 8 | serverless-pre-function `phase: rewrite` 不生效 | jwt-auth 在 **access** 阶段才设 `ctx.jwt_auth_payload`，rewrite 跑得比 access 早 |
| 9 | serverless-pre-function `phase: access` 后 OPTIONS 头不全 | `cors.allow_credentials` 字段名错——APISIX 3.18 用**单数** `allow_credential`；schema 校验静默忽略多余字段，plugin 回退默认值 |
| 10 | OPTIONS 预检仍缺 Allow-Credentials | route 100 catch-all 的 cors plugin 在 OPTIONS 时 `rewrite` 阶段 `return 200` 不写全部响应头；社区方案：OPTIONS 拆**独立 route**（只 match OPTIONS） |

每个失配都"没报错"——APISIX 静默回退到默认值（`Allow-Origin: *` / `Allow-Methods: GET,POST,PUT,DELETE,OPTIONS` / `Max-Age: 5` / `Allow-Headers: *`）。

**根本教训**：APISIX 3.18 是"沉默失败"系统——字段名错、phase 错、phase 时机错都不会报错，只回退默认值。任何时候看到响应头是默认值，要怀疑**配置被静默丢弃**。

---

## 三、最终修复方案

### 3.1 后端改动

| 文件 | 改动 |
|---|---|
| `emotion-echo-web-bff/internal/auth/jwt.go` | `Claims` 加 `User string` + `Key string` 字段，`Sign` 同时填 `user="user"` + `key="user"`（APISIX jwt-auth 用 `user` claim 找 consumer）|
| `emotion-echo-web-bff/main.go` | 注释说明 dev 走 APISIX；移除之前回滚的 `devCORSMiddleware` / `devBearerToUserIDMiddleware` |
| `deploy/apisix/seed.sh` | cors 字段改 string（逗号分隔）|

### 3.2 前端改动

| 文件 | 改动 |
|---|---|
| `Emotion-Echo-Web/.env` | `NUXT_PUBLIC_API_BASE_URL=http://localhost:19080/api/v1`（走 APISIX）|
| `Emotion-Echo-Web/nuxt.config.ts` | `vite.optimizeDeps.include` 加 `@element-plus/icons-vue`（修 FSL 30s）|
| `Emotion-Echo-Web/package.json` + `pnpm-lock.yaml` | 加 `@element-plus/icons-vue ^2.3.2` 依赖 |
| `Emotion-Echo-Web/app/composables/useConversationSender.ts` | 删 `sendAIStream` 调用时多余的 `{`（修 chat 发送 JS 报错）|

### 3.3 APISIX 容器配置（手 PUT 不持久化）

| Route | 用途 | 关键 plugin |
|---|---|---|
| **95**（OPTIONS-only）| 修 OPTIONS 预检 CORS 头 | `cors`（field names：`allow_credential` 单数）|
| 100（catch-all）| `/api/v1/*` | `jwt-auth` (key_claim_name=user, store_in_ctx=true) + `serverless-pre-function` (phase=access) 注入 X-User-Id + `cors` + `limit-count` |
| 110-114 | auth 白名单 | `cors` + `limit-count`（**字段名修对**） |
| 115 | /api/v1/health | `cors` + `limit-count` |

**serverless-pre-function 函数**（最终生效）：
```lua
return function(conf, ctx)
  local p = ctx.jwt_auth_payload
  if p and p.user_id ~= nil then
    local core = require('apisix.core')
    core.request.set_header(ctx, 'X-User-Id', tostring(p.user_id))
  end
end
```

### 3.4 dev 容器重启后必须重跑

APISIX 配置不持久化（etcd 数据在但路由需重 seed）。**未来 Stage 38+ 任务**：把当前 6 步手 PUT 写成 `seed_v2.sh` 一次性脚本（含 consumer 注册 + route 95 + 修所有路由 cors 字段名）。

---

## 四、端到端实测

```
$ curl -X POST http://localhost:19080/api/v1/auth/login -d '{"username":"echo","password":"echo123"}'
HTTP 200 + JWT ✓

$ curl -X OPTIONS http://localhost:19080/api/v1/auth/login -H "Origin: http://localhost:3000" ...
Access-Control-Allow-Origin: http://localhost:3000
Access-Control-Allow-Methods: GET,POST,PUT,DELETE,OPTIONS,PATCH
Access-Control-Max-Age: 600
Access-Control-Expose-Headers: X-User-Id
Access-Control-Allow-Headers: Content-Type,Authorization,X-User-Id
Access-Control-Allow-Credentials: true                              ← 关键

$ curl -H "Authorization: Bearer $TOKEN" http://localhost:19080/api/v1/user/profile
HTTP 200 ✓ (APISIX → jwt-auth 验签 → serverless 注 X-User-Id=3 → BFF 返 user profile)
```

| 接口 | 结果 |
|---|---|
| OPTIONS /api/v1/auth/login | ✅ 全 CORS 头 |
| OPTIONS /api/v1/conversations | ✅ 全 CORS 头 |
| POST /auth/login | ✅ 200 + JWT |
| GET /user/profile（带 token）| ✅ 200 |
| GET /conversations | ✅ 200 |
| GET /reports/daily | ✅ 200 |
| POST /messages | ⚠️ 500（chat-svc 路由问题，APISIX 透传正确） |

---

## 五、为什么反复触礁——核心方法论教训

按 AGENTS.md §0.2 列假设：
- **APISIX 3.18 不是"装上就能用"**：3.18 有大量字段名（`allow_credential` 单数 / `credentials` 复数）、phase（`rewrite` 早于 `access`）、schema（多余字段静默忽略）陷阱
- **APISIX 不报错的 silent-fail**：每次配错都被默认值兜底——`Allow-Origin: *`、`Allow-Methods` 5 元素、`Max-Age: 5` 都是默认值
- **本地代码本可以更早暴露问题**：如果 dev 模式一开始就走 APISIX（不绕过），10 个失配在 Stage 32 PR-15 就会暴露，而不是现在

**未来 Stage 38+ 应该做的**：

1. **`seed_v2.sh` 一次性脚本**（写死正确的字段名 + 全部 route + consumer 注册），dev compose up 后自动跑——避免每次重启 dev 都要手 PUT
2. **APISIX docker 健康检查**——`/usr/local/apisix/logs/error.log` 应该有 "schema check failed" 日志，可以监控
3. **`allow_credential` 单数 vs `allow_credentials` 复数**——APISIX 3.18 vs 2.x 的命名差异，是 dev 模式配置最大的踩坑点
4. **ADR-20 文档失真清单归档**——把"roadmap 描述与代码失真"累计清单正式记录

---

## 六、引用

- commit `ff9b3b9` · fix(apisix-dev-path): dev 走 APISIX 端到端打通
- 前置：[stage-37-A-landing.md](/docs/stages/stage-37-A-landing.md) · [stage-37-B-landing.md](/docs/stages/stage-37-B-landing.md) · [stage-38-A-landing.md](/docs/stages/stage-38-A-landing.md)
- 路线图：[stage-37-fixes-roadmap.md](/docs/stages/stage-37-fixes-roadmap.md)
- APISIX 官方文档：https://apisix.apache.org/docs/apisix/plugins/jwt-auth/ · /plugins/cors/ · /plugins/proxy-rewrite/
- AGENTS.md §0.2 文档撰写前功课规则：[AGENTS.md](/AGENTS.md)
