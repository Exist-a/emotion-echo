---
stage: e2e-29
title: 横切：异常与安全（JWT 过期刷新 / IDOR / 限流 / CORS / 越权 / 密钥轮换 / 错误透出与留痕）
type: transformation
status: done
created: 2026-10-07
last-updated: 2026-10-08（**done**：20/20 测试点全 PASS；5 个执行期 [M] 决策点全部落定（D-46/47/48 + M4 + M5 由证据关闭）；修复 7 项（L1/D-47/F-203/F-182/M4/D-48/F-200）+ 回归钉 9/9 首跑绿；账本名下 7 条全 ✅，新发现 E2E-F-204 不回挂。**补验轮（2026-10-08）**：补做 §6 要求的 IAB 实测（过期令牌真实浏览器行为）→ 更正 #5 证据、`jwt-expiry.spec.ts` 加严 4 passed、新登 E2E-F-207（前端 `10002` 续期分支为死代码））
depends-on: []
blocks: []
gate: []            # 无开工前阻塞决策门；5 个执行期 [M] 决策点见 §4
related-findings: [E2E-F-27, E2E-F-28, E2E-F-182, E2E-F-200, E2E-F-201, E2E-F-202, E2E-F-203]
---

# E2E-29 横切：异常与安全 — 详档（任务书）

> **类型**：transformation —— 鉴权/限流/CORS 的**配置与代码全部存在且日常在跑**（APISIX `jwt-auth` consumer 验签 + X-User-Id 注入、`limit-count` 60/60s、`cors` 插件、BFF `GinAuthMiddlewareWithOpts`、前端 401 自动续期），但它**当前不能证明自己是安全的**：
> ① **计划期实测抓到认证绕过**：`POST /api/v1/auth/refresh` **匿名调用返回 200 并发放 `user_id=1` 的 24h 有效 JWT**（E2E-F-201）——该端点同时被 BFF 前缀白名单与 APISIX route 113 放行，而 `refresh` 在无有效令牌时**回落默认身份 `userID = 1`**；
> ② **可信链默认 fail-open**：BFF `BFF_TRUST_APISIX` 默认 `false`（任何来源的 `X-User-Id` 都被接受），而 `8894` 端口映射到宿主、prod overlay 只以**注释**提醒"应设 true + 移除端口"（E2E-F-202）；
> ③ **无 JWT 密钥轮换机制**（E2E-F-28）：BFF 与 APISIX consumer 共用同一 `BFF_JWT_SECRET`，轮换必须原子一致，现有守卫只断言"三处默认值一致"；
> ④ **错误透出与真因留痕缺失**（E2E-F-200）：摄像头失败落进 `useFaceEmotion.ts` 的 TypeError 宽兜底分支，文案误导（"刷新页面"）且 `error.name` 无落痕。
> **本阶段把"配了鉴权/限流/CORS"变成"边界逐点可断言、越权有实测拒绝、密钥可轮换、错误可归因"。**

> **依据**：roadmap §第八批 E2E-29 行（"JWT 过期刷新/IDOR/限流/CORS/越权 + JWT 密钥轮换机制（BFF+APISIX 原子性）"，边界"渗透测试"）+ 本阶段名下 4 条留账（F-27 生产化封装 / F-28 密钥轮换 / F-182 smoke 恒 401 / F-200 摄像头错误透出）+ 本轮计划期新登 3 条（F-201 / F-202 / F-203）。
>
> **方法论**：[RUNBOOK.md](../../RUNBOOK.md)（§4 判定分级 / §4.1 证据有效性 / §6 TDD 纪律 / §7 收口 11 项 / §13 审计）+ [anti-patterns.md](../../anti-patterns.md)。
> **前置阶段**：无依赖前置。上一阶段 E2E-28（性能与延迟基线）✅ done（2026-10-06）；F-198 TTS API 专项 ✅ 完整闭环（2026-10-07，F-199 四子项全过）。
> **阶段边界声明**：本阶段**不做渗透测试工具链引入**（不进 zap/nuclei），只做"逐点可断言的边界验证 + 范围外只记账"。

---

## 0. 计划期调研（**先摆事实，再排计划**）

> 本节全部为本轮**亲自回读的文件 / 亲自执行的探针**，非引用历史结论（AGENTS.md §〇 文档功课）。执行期若发现与本节不符，**以实测为准并回填本节**（AP-02）。

**已读实现文件**（≥3，AGENTS §〇 ①）：
`emotion-echo-web-bff/main.go`（鉴权装配 :210-320 + `authPathBypass` / `noAuthPathPrefixes`）、`emotion-echo-web-bff/internal/auth/jwt.go`（全文 140 行）、`emotion-echo-web-bff/internal/handler/auth_handler.go`（`refresh` :212-234 / `setAccessTokenCookie` :297-302 / `verificationCode` :245-273）、`emotion-echo-web-bff/internal/handler/analytics_handler.go`（`userIDQuery` :47-72）、`emotion-echo-web-bff/internal/config/config.go`（:340-360 env 覆盖）、`emotion-echo-shared/pkg/middleware/gin_auth.go`（全文）、`deploy/apisix/seed.sh`（jwt-auth consumer :300-341 / 插件链 :417-531 / 白名单 :588-660）、`deploy/docker-compose.apps.yml`（web-bff :654-723 / apisix-seed :764-767）、`deploy/compose.prod.yml`（:33-60 差异说明）。

**已读测试文件**（AGENTS §〇 ①）：
`emotion-echo-web-bff/internal/handler/analytics_handler_test.go`（IDOR 403 三例 :401-440）、`scripts/test_bff_jwt_secret.sh`（3 断言全文）、`emotion-echo-shared/pkg/middleware/gin_auth_test.go`、`emotion-echo-web/e2e/jwt-expiry.spec.ts`（现有到期跳登录回归钉）。

**已查 ADR / 决策 / stage**（AGENTS §〇 ②）：
`docs/architecture/decisions.md` 决策 7（jwt-auth，退役注记）/ 决策 8（限流熔断）/ 决策 11（APISIX 复职）/ 决策 12（BFF 纯聚合层）/ 决策 18（§2 #22、Decision9：BFF 信任链）/ 决策 40（TTS API，与本阶段无关）；`docs/e2e-roadmap/decisions.md` D-27（Redis 保留接入业务）/ D-28（验证码回退 in-memory，登录锁定保留 Redis）/ D-30（节点由 discovery 拉取）/ D-35（`checks.active` 落地）；`docs/stages/stage-109a-apisix-jwt-401-fix-2026-09-16.md`（`TrustAPISIX=true` + `APISIXCIDRs=[]` 配置矛盾的历史修法）、`stage-112`（`userIDQuery` 强依赖 query 的 IDOR 修法）。

**smoke / 运行时探针（AGENTS §〇 ③，本轮实测）**：`python scripts/e2e_stage_audit.py --all` → **30 阶段 0 FAIL**；`oneline curl` 安全探针 6 条（见 §0.1 F1/F3/F6/F7/F8/F9）；`docker ps` → 17 容器 healthy。

**外部依赖官方文档（AGENTS §〇 ④）**：
- APISIX `jwt-auth`：<https://apisix.apache.org/docs/apisix/plugins/jwt-auth/>（`key`/`secret`/`algorithm` 属 **consumer**；route 侧只放传输层参数；`store_in_ctx` 供后续插件读 payload）——与 `seed.sh:300-341` 现状一致。
- APISIX `cors`：<https://apisix.apache.org/docs/apisix/plugins/cors/>（`allow_origins` 为逗号分隔或 `*`；`allow_credential=true` 时**不得**同时用 `*`）——本轮实测与之一致（恶意 origin 无 ACAO 头）。
- APISIX `limit-count`：<https://apisix.apache.org/docs/apisix/plugins/limit-count/>（`policy=redis` 时必填 `redis_host/port`；`rejected_code` 缺省 503）——F-177（白名单路由漏配 `rejected_code` 致 429/503 不一致）即此条。
- ⚠️ 官方文档**未提供**"JWT 密钥轮换"的现成机制（consumer 单一 `secret` 字段）⇒ 轮换方案需自建（§4 M2）。

### 0.0 假设清单（本文假设，与现状对比见 §0.1）

| # | 本文假设 | 依据 / 现状 |
|---|---------|------------|
| 1 | "受保护端点缺 token 一律 401" | **成立**（F4：网关 `Missing JWT token in request` 401）——但 `refresh` 例外，见 F1 |
| 2 | "IDOR 已有 403 守卫，只需钉回归" | **部分成立**（F7：`user_id` 不匹配 403），但守卫只认字面参数名，别名未纳入（F8） |
| 3 | "限流 60/60s 生效" | **成立**（F6 实测 70 次 = 60 通过 + 10×429） |
| 4 | "CORS 拒绝未授权 origin" | **成立**（F9：恶意 origin 无 ACAO） |
| 5 | "cookie 是 HttpOnly + SameSite=Lax" | **不成立**：实际 Set-Cookie 无 `SameSite`、无 `Secure`（F10），与代码注释相反 |
| 6 | "JWT 密钥有轮换机制" | **不成立**（F11：无轮换机制，只有静态一致性守卫） |
| 7 | "dev 与 prod 的信任链差异是文档化已知项" | **不成立**：prod 默认值本身 fail-open，overlay 仅注释提醒（F3） |

### 0.1 计划期实测事实表

| # | 事实 | 证据（`文件:行号` / 探针输出） |
|---|------|--------------------------|
| **F1** | 🔴 **`POST /api/v1/auth/refresh` 匿名返回 200 并发放 `user_id=1` 的 24h 有效 JWT（认证绕过）** | 探针：`curl -i -X POST http://localhost:19080/api/v1/auth/refresh`（无 cookie、无 Authorization）→ `HTTP/1.1 200` + `Set-Cookie: access_token=eyJ…` + body `{"code":0,"data":{"accessToken":"…","expiresIn":86400,"user":{"id":"1","username":"user"…}}}`。**该 token 经网关读 `/users/me` 实测返回 `userId":1,"account":"echo"`** ⇒ 非仅"发了个 token"，而是**可直接用的会话**。根因三层：① BFF `main.go:307-313` `noAuthPathPrefixes` 用**前缀**放行整个 `/api/v1/auth/`；② APISIX `seed.sh:632` `put_auth_route 113 "/api/v1/auth/refresh"`（无 jwt-auth）；③ `auth_handler.go:215` `var userID int64 = 1` —— cookie/header 解析失败时**静默回落默认身份**（连"传了过期 token"也照样发新 token） |
| **F2** | 🔴 **令牌类型隔离靠"恰好"而非显式契约** | `jwt.go:109-139` reset token（`Subject="reset-password"`、无 `UserID`）与 access token **共用同一 secret**；`Parse`（:82-97）要求 `UserID != 0` ⇒ reset token 确实过不了 access 路径（**当前安全**），但反向（access token 当 reset token）仅靠 `ParseResetToken` 的 `Subject` 校验兜住，且 **APISIX consumer 对 `key` claim 的匹配未测**——无任何测试钉住双向隔离 |
| **F3** | 🔴 **BFF 可信链默认 fail-open + 端口暴露** | 探针：`curl -H "X-User-Id: 2" http://localhost:8894/api/v1/users/me` → **200** 返回 `smoke_user`（零认证）；`docker-compose.apps.yml:662` `BFF_TRUST_APISIX: ${BFF_TRUST_APISIX:-false}`（**默认关**）+ `:701` `- "8894:8894"`（宿主映射在位）；`compose.prod.yml:33-37` 只有注释"prod 应设 true / 应移除 8894"。`gin_auth.go:130-140` 语义：`RequireAPISIXIP=true` 且 CIDR 为空 ⇒ `cidrs==nil` ⇒ **一律拒绝**（fail-closed），`compileCIDRs` 失败同样退化空白名单 ⇒ **结论：风险不在中间件，而在"默认 false + 端口暴露 + prod 无强制"**。**补正（同日实测）**：`compose.dev.yml:28-36` 的 dev 覆盖本会置 `true` 并给 CIDR `172.18.0.0/16`，而该网段**含 docker 网关 IP** ⇒ 宿主经映射直连 8894 时 RemoteAddr 落在段内，**即便加载 overlay 也照样能伪造**；当轮运行栈未加载该文件（见 F18），实际生效 apps.yml 默认 `false` |
| **F4** | ✅ **缺 token 访问受保护端点被网关拦下** | 探针：`curl -i http://localhost:19080/api/v1/users/me` → `401` + `{"message":"Missing JWT token in request"}` + `WWW-Authenticate: Bearer realm="jwt"`（APISIX jwt-auth 生效） |
| **F5** | ✅ **X-User-Id 由 APISIX 无条件覆盖注入（防伪造）** | `seed.sh:417-531`（serverless-post-function 在 jwt-auth **之后**执行，覆盖客户端传入的 `X-User-Id`）——与探针 F3 的对照：**经网关**不可伪造，**直连 8894** 可伪造 |
| **F6** | ✅ **网关限流实测生效 60/60s（key=remote_addr）** | 探针：连续 70 次 `POST /api/v1/auth/login` → **60 个非 429 + 10 个 429**；响应头 `X-RateLimit-Limit: 60` / `X-RateLimit-Remaining: 59` / `X-RateLimit-Reset: 60`（`seed.sh:597` 白名单链、:457-465 catch-all 链，`policy=$LIMIT_POLICY`=redis） |
| **F7** | ✅ **reports 端点 IDOR 守卫存在并生效** | 探针：`GET /api/v1/reports/daily?user_id=2`（身份=user 1）→ **403** `{"code":1,"message":"forbidden: user_id mismatch with authenticated user"}`；实现 `analytics_handler.go:47-72`；测试 `analytics_handler_test.go:401-440`（三例：无 query 回退身份 / 不一致 403 / 一致 200） |
| **F8** | 🟡 **IDOR 守卫是"参数名白名单"语义：别名不被识别** | 探针：同一 token 下 `?userId=2` → **200**、`?id=2` → **200**，且**返回体与不带 query 时逐字相同**（对比 head 300 字符一致）⇒ 当前**无数据泄漏**（别名被 handler 忽略）；风险是"守卫只认面量 `user_id`，且 `analytics_handler.go:52` 注释自述"无认证头（单测直连）→ 200（向后兼容）"——一旦后续新增读 query 别名的端点，守卫不覆盖 |
| **F9** | ✅ **CORS 拒绝未授权 origin，且 credentials 与具体 origin 同用** | 探针：`OPTIONS` 带 `Origin: http://evil.example.com` → `200` 但**无任何 `Access-Control-Allow-*`**（浏览器会拦）；`Origin: http://localhost:3000` → `ACAO: http://localhost:3000` + `ACAM: GET,POST,PUT,DELETE,OPTIONS` + `ACAH: Content-Type,Authorization,X-User-Id,X-Trace-Id` + `ACAC: true` + `Access-Control-Max-Age: 5`（`seed.sh:598`） |
| **F10** | 🟡 **cookie 属性与注释相反：无 `SameSite`、无 `Secure`** | 探针实测 `Set-Cookie: access_token=…; Path=/; Max-Age=86400; HttpOnly`（**无 SameSite/Secure**）；`auth_handler.go:298` 注释写"SameSite=Lax: 允许顶层导航携带 cookie"、:301 `c.SetCookie("access_token", token, int(maxAge), "/", "", false, true)`——**gin 的 `SetCookie` 没有 SameSite 形参**，注释描述的能力从未存在（logout `:242` 同型） |
| **F11** | 🔴 **JWT 密钥无轮换机制（F-28 成立）** | `seed.sh:91` `JWT_SECRET="${BFF_JWT_SECRET:-dev-jwt-secret-local-only}"`（consumer secret 来源）+ `apps.yml:694/767` 同值默认；`scripts/test_bff_jwt_secret.sh` 只断言"web-bff 与 apisix-seed 默认值一致 + 非空"（**静态文本**，非轮换机制）；BFF 侧 `jwt.go:48-57` 单 secret、无 kid/多密钥窗口 ⇒ 轮换必致全部在途 token 失效，且需 BFF 与 APISIX **原子一致** |
| **F12** | 🟡 **前端 refresh 契约与后端实现漂移** | `useApi.ts:157-159` 注释称"后端要求回传当前 AccessToken 的 `jti`，用于黑名单/轮换校验"并真的发 `{jti}`；BFF `refresh` **零 jti 逻辑**（全仓 grep 无消费点）⇒ 注释描述的黑名单/轮换**不存在**（与决策 18 Decision9 的"注释承诺从未实现"同型） |
| **F13** | 🔴 **F-182 成立：smoke 契约 4/7 恒 401** | `scripts/smoke_bff_chat_grpc.sh` 契约 4/7 只带 `X-User-Id` 不带 `Authorization: Bearer` ⇒ 经网关被 jwt-auth 拒（`Missing JWT token in request`），脚本恒 FAIL（该脚本编写于 PR-GRPC-6 时代，未随鉴权要求补登录取 token 步骤） |
| **F14** | 🔴 **F-200 成立：错误透出与真因留痕缺失** | `emotion-echo-web/app/composables/useFaceEmotion.ts:97` 是 **TypeError 宽兜底**（非"权限拒绝/无设备/被占用"三条具名分支），文案"摄像头组件未就绪，请刷新页面或稍后重试"对"环境无 `navigator.mediaDevices`"情形属误导，且 `error.name` 无落痕；FER 容器本轮 `healthy`（非环境缺件） |
| **F15** | 🟡 **F-27 生产化封装未做（dev 演练已过）** | 2026-09-27 实测：PG `pg_dump -Fc` + `pg_restore --clean --if-exists`，备份 245KB → 真 DROP `emotion_echo_chat.messages` CASCADE → 恢复 417→417 一致；**全仓无封装脚本、无 cron、无异地**（生产化封装留归本阶段） |
| **F16** | ✅ **环境基线可用（计划期实测）** | `docker ps`：17 容器 healthy（含 postgres/redis/kafka/nacos/etcd/minio/apisix/6 应用服务/2 模型服务）；`deploy/.env.local` 存在；无 `deploy/.devmode-session`（双轨锁空闲）；`main` = `a7429e3` 干净 |
| **F17** | 🟡 **§2.5 残留：两个已合并远端分支未删** | `git branch -r` 仍有 `origin/fix/f199-lipsync-pinyin`（PR #166 squash 源）与 `origin/feat/f199-tts-speed-config`（PR #167 squash 源，diff=0）；**2026-10-07 用户批准后已双双删除**，远端现仅 `origin/main` |
| **F18** | 🔴 **运行栈的归属与形态与 RUNBOOK §2.1 不一致（环境基线必须记明）** | ① 容器标签 `com.docker.compose.project.config_files` = `D:\源码\Emotion-Echo-f198\deploy\{infra,apps}.yml` ⇒ 17 个后端容器由**另一个仍在磁盘上、但已非注册 worktree 的目录**创建（`git worktree list` 只剩主目录）；② **未加载 `compose.dev.yml`**（`BFF_TRUST_APISIX=false`、无 `BFF_APISIX_CIDRS`，容器启动日志 `[warn] TrustAPISIX=false; dev mode, any X-User-Id accepted`）；③ `:3000` **不是容器**——`emotion-echo-web` 容器 `Exited (0) 2 days ago`，端口由**宿主 `node.exe`（PID 1800）跑 `nuxt dev`** 提供，其命令行指向**主 worktree** `D:\源码\Emotion-Echo\emotion-echo-web`（⇒ 前端跑的是当前 main 代码，后端不是）。**后果**：任何从本 worktree 执行 `docker compose up` 都会因 `container_name` 固定而冲突/并行起第二套栈；`[V]` 测试前必须按 F-196 铁律先验服务身份。**处理**：不在本轮强拆（会打断用户正在跑的前端 dev server），列为下次会话开工第一步（§0.2 #1） |

### 0.2 开工复核清单（第一天执行，防止任务书事实表过期）

| # | 复核项 | 通过标准 |
|---|--------|---------|
| 1 | **运行栈归属与形态**（F18，**先于一切**） | 决定"接管现栈"还是"重起基线栈"：现栈来自 `Emotion-Echo-f198` 且无 `compose.dev.yml`，与 RUNBOOK §2.1 不一致。**注意 `container_name` 固定**，直接 `up` 会冲突；建议先 `docker compose ... down`（确认前端不依赖）再按 §2.1 全量起（含 `-f compose.dev.yml --env-file .env.local --profile dev`）；管理 `deploy/.devmode-session` |
| 2 | 环境基线（RUNBOOK §2.1） | 6 应用服务 + infra healthy；`docker inspect emotion-echo-db-migrate` ExitCode=0；Nacos `count:6` |
| 3 | **重跑 F1 探针** | 若匿名 refresh 已 401（L1 已修，需栈重建后生效）则组 A #1 判 PASS 并记录；禁止默认其仍坏 |
| 4 | 重跑 F3 探针（直连 8894 伪造 header） | 复核 `BFF_TRUST_APISIX` 运行时取值（`docker exec emotion-echo-web-bff env` + 启动日志 `[auth]`/`[warn]` 行）与 8894 是否仍暴露 |
| 5 | 重跑 F6/F9 探针 | 429 阈值与 CORS 头与本节一致（配额可能被前序探针消耗，先等窗口重置） |
| 6 | 服务身份先验（F-196 铁律） | `:3000` 由本地 dev server 还是 web 容器服务（F18③ 实测为宿主 dev server，跑主 worktree 代码）；`[V]` 结论必须绑定被验对象 |
| 7 | 账本编号连续性 | 新登编号从 **E2E-F-201** 起（当前最大 F-200；本轮已登 201~203） |

---

## 1. 范围

### 做

- **认证边界**：`/auth/refresh` 匿名/无效令牌语义、令牌类型隔离（access ↔ reset）、logout 语义、过期→续期端到端（复用并加严 `jwt-expiry.spec.ts`）
- **越权与 IDOR**：reports 端点守卫回归钉、**资源级归属校验**（会话/消息/测评结果/文件跨用户直取）、越权写拒绝、参数别名面
- **限流**：网关 `limit-count`（catch-all + 白名单两条链）、跨实例登录锁定（E2E-20 回归钉）、`rejected_code` 一致性（F-177 域）
- **CORS**：origin 白名单双向验证、`allow_headers` 面（是否应暴露 `X-User-Id`）、credentials 组合
- **JWT 密钥轮换（F-28）**：机制设计 + 落地 + 守卫（含 BFF 与 APISIX consumer 的原子性）
- **备份/恢复生产化封装（F-27 follow-on）**：封装脚本 + 在 dev 可验证的自动化形态
- **错误透出与真因留痕（F-200）**：`error.name` 落痕 + 分支化文案 + `client-error` 上报可达性

### 不做（边界）

- **渗透测试工具链引入**（zap/nuclei/burp）——按 roadmap 边界，本阶段只做逐点断言
- 真实 OAuth/短信邮件通道、真实 refresh-token 双令牌表（是否立项属 §4 M1）
- 业务功能改造（聊天/测评/报表逻辑本体）、schema 变更
- prod 真实部署与 TLS 终止（prod 差异**只做配置语义断言 + 决策升级**，不部署）
- `deploy/.env.local` 内容读打印、密钥进仓（AGENTS §四红线）

---

## 2. 测试点清单（20 个）

判定标记：`[A]` 自动可判 · `[V]` 视觉判定（需截图并被查看）· `[M]` 需人工/设计裁定（必须升级给用户）。详见 [RUNBOOK.md](../../RUNBOOK.md) §4。

### 组 A：认证与令牌边界（6）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|---------|
| 1 | [A] | **匿名 `POST /auth/refresh` 必须 401**（F-201 核心，TDD 循环 L1） | 无 cookie/无 `Authorization` 时返 401 且**不设 cookie、不返 accessToken**；同时断言"带过期/伪造签名 token"亦 401（禁止回落默认身份） |
| 2 | [A] | 有效 token 调 refresh 正常续期 | 带合法 cookie → 200 + 新 token 可读 `/users/me` 返回**同一 user_id**（防"续期换身份"） |
| 3 | [A] | 受保护端点匿名枚举 | ≥8 条受保护路径（`/users/me`、`/conversations`、`/reports/daily`、`/reports/trend`、`/surveys`、`/ai/stream`、`/user/avatar`、`/voice/audio/*`）逐条 401；含"非白名单的 auth 动作"（如 `/api/v1/auth/nonexistent` 不应 200） |
| 4 | [A] | 令牌类型隔离双向 | reset token（`Subject=reset-password`）**不得**当 access token 用（经网关 401；直连 BFF 401）；access token **不得**当 reset token 用（`/auth/reset-password` 拒绝） |
| 5 | [A] | 过期语义与前端续期端到端 | exp 到期 → 网关 401；前端 `code===10002` 路径触发 refresh → 换新 token → 原请求重试成功；refresh 失败 → `clearAuth` + 跳 `/login`（`useApi.ts:302-340` 现行为钉住） |
| 6 | [A] | logout 后 cookie 清除语义 | `POST /auth/logout` → `Set-Cookie` 空值 + `Max-Age<0`；**并如实记录"无服务端黑名单 ⇒ 已签发的 token 仍有效至 exp"为已知边界**（是否补黑名单 = §4 M1） |

### 组 B：越权与 IDOR（5）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|---------|
| 7 | [A] | reports 端点 IDOR 守卫回归钉（F7） | `?user_id=<他人>` → 403；无 query → 用认证身份 200；一致 → 200（三例与 `analytics_handler_test.go` 对齐） |
| 8 | [A] | 参数别名不构成越权 | `?userId=` / `?id=` / `?uid=` → 断言响应体与"不带 query"**逐字节等价**（当前实现忽略别名）；若后续改为识别，须走同一 403 守卫 |
| 9 | [A] | **资源级归属校验（本组主测点）** | 用 user A token 直取 user B 的资源：会话详情/消息列表、测评结果（`survey_results/{id}`）、报表（已覆盖）、上传文件/语音音频 key ⇒ 逐条期望 `403`/`404`（**不得 200**）；先枚举路由清单再逐条实测，禁抽样代替 |
| 10 | [A] | 越权写拒绝 | `PATCH /users/me` 只能改自己（改他人 id/username 被忽略或 403）；删/改他人会话被拒；`PUT/POST` 类端点同批覆盖 |
| 11 | [A]+[M] | **BFF 可信链（F-202）** | 断言：dev 直连 8894 伪造 `X-User-Id` 的行为**有明确记录**（当前 200，属 dev 已知面）；prod 语义断言：`BFF_TRUST_APISIX` 默认值、8894 是否暴露、`APISIXCIDRs` 来源 ⇒ 逐项给出**当前取值 + 是否 fail-open**，方案取舍升级 §4 M2 |

### 组 C：限流（3）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|---------|
| 12 | [A] | 网关 `limit-count` 实测（回归钉） | 窗口内第 61 次请求 → 429 + `X-RateLimit-*` 头；重置窗口后恢复 200（catch-all 与白名单两条链**分别**实测） |
| 13 | [A] | 限流拒绝码一致性（F-177 域） | 白名单路由与 catch-all 的限流拒绝**同为 429**（不是 openresty 裸 503），响应体可区分"被限流"与"网关故障" |
| 14 | [A] | 登录锁定跨实例 + 退避头（E2E-20 回归钉） | 连续错密码达阈值 → 423/429（Redis 共享存储，跨 BFF 实例生效）；`Retry-After` 存在且与前端指数退避策略一致（`useApi.ts:213-235`） |

### 组 D：CORS（2）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|---------|
| 15 | [A] | origin 白名单双向 | 未授权 origin 预检无任何 `Access-Control-Allow-*`；`localhost:3000`/`127.0.0.1:3000` 有 ACAO 且与请求 origin 精确相等（非 `*`）；`ACAC: true` 与通配互斥（官方约束） |
| 16 | [A] | `allow_headers` 面与预检时效 | 清单含 `X-User-Id` 是否**必要**（前端本不应自带该头；暴露它等于把伪造面写进契约）⇒ 给出取舍结论；`Access-Control-Max-Age: 5` 的时效是否可接受（每次跨域预检都打网关，与 #12 限流配额相乘） |

### 组 E：密钥轮换与灾难恢复（2）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|---------|
| 17 | [M] | **JWT 密钥轮换机制（F-28）** | 方案落地后实测：轮换期间**在途 token 不失效**（或明确接受失效并文档化）；BFF 与 APISIX consumer 原子一致（守卫断言，非人眼）；含回滚路径与"轮换后旧 token 行为"实测。方案取舍 = §4 M3 |
| 18 | [A]+[M] | **备份/恢复生产化封装（F-27 follow-on）** | 封装脚本（`pg_dump -Fc` + `pg_restore`）+ 自动化形态（cron/入口脚本）在 dev 可验证；**真演练**：备份 → 破坏 → 恢复 → 行数一致（照 2026-09-27 口径，>=417→417）。范围（是否含异地/加密） = §4 M4 |

### 组 F：错误透出与收口（2）

| # | 判定 | 测试点 | 通过标准 |
|---|------|--------|---------|
| 19 | [V]+[A] | **错误透出与真因留痕（F-200）** | `useFaceEmotion` 各失败分支：`error.name`/`error.message` 有落痕（可观测）、文案与真因对应（TypeError 分支不再一律"刷新页面"）；`/api/v1/client-error` 白名单可达（前端错误上报在未登录态也可送达）。证据 = 截图（错误文案）+ 控制台/上报记录 |
| 20 | [A] | 收口与工具修复（含 F-182） | `scripts/smoke_bff_chat_grpc.sh` 契约 4/7 修通（先 login 取 Bearer）；回归钉 spec 首次跑绿；`e2e_stage_audit.py --all` 0 FAIL；账本对账（本阶段名下 F-27/28/182/200/201/202/203 逐条翻状态或在 report 写明仍挂理由） |

汇总行（执行期填写）：PASS x / FAIL y / BLOCKED z / N/A w

---

## 3. TDD 循环划分（RED→GREEN→REFACTOR）

| 循环 | RED（先行失败测试） | GREEN（最小实现） | 影响面 |
|------|-------------------|------------------|--------|
| **L1** | `auth_handler_test.go`：匿名/过期/伪造 refresh → 期望 401（当前实测 200，必红） | `refresh` 不再回落默认身份；白名单收窄为"无需身份的端点"（`refresh` 除外），或要求有效令牌 | 前端 `useApi.refreshToken` 失败路径已兼容（`clearAuth`+跳登录）⇒ **不改前端**；APISIX 白名单 route 113 是否保留 = §4 M1 |
| **L2** | 令牌类型隔离双向契约测试（Go + Playwright 各一条） | 显式 `TokenType` claim（`token_type=access|reset`）并在两侧校验 | `jwt.go` 签发/解析（注意向后兼容旧 token） |
| **L3** | 资源级越权用例（每资源一条，先实测确认现状） | 在归属校验缺失处补校验（BFF 侧读 ctx user_id 与资源 owner 比对） | 逐资源评估是否属"边界外模块"（命中即转账本，不修） |
| **L4** | cookie 属性契约测试（断言 `SameSite` 存在且为 `Lax`/`Strict` + dev/prod 的 `Secure` 策略） | 改用 `http.SetCookie` 显式构造（gin `SetCookie` 无 SameSite 形参）；同步更正 :298 注释 | 前端 `useCookie` 读取不受影响（同站点） |
| **L5** | 轮换守卫（先红：无 kid/双密钥 ⇒ 轮换必致全站 401） | §4 M3 方案落地（双密钥窗口或原子重启 + 守卫断言一致） | BFF + `seed.sh` + compose env；**需 ADR**（架构级：密钥/认证） |
| **L6** | `useFaceEmotion.errors.test.ts` 扩展：各 `error.name` 分支文案与落痕断言 | 分支化文案 + 落痕 + 上报 | 前端 only |

> **边界提醒**：L5 触及"认证/密钥"属 RUNBOOK §13.3 #15 的架构关键词 ⇒ **必须同批附 ADR + `docs/architecture/decisions.md` 变更**（防 AP-08）。L1 若决定收窄 `noAuthPathPrefixes`，须同步 `check_routes_alignment.sh` 契约 3（BFF auth action ⊆ APISIX 白名单）。

---

## 4. 执行期 [M] 决策点

| # | 决策 | 背景 | 备选 |
|---|------|------|------|
| **M1** | `refresh` 在"无有效令牌"时的语义 | F1 实测匿名发 token；前端依赖 `refresh` 做 401 续期 | ① **硬 401**（最简单、安全；前端已兼容失败路径）② 双令牌（短期 access + 长期 refresh token 表，工程量大）③ 保留宽限（接受"刚过期"token 换新，禁止无 token）—— ✅ **已裁定 = ① 硬 401（D-46，2026-10-07 用户拍板）** |
| **M2** | prod 信任链默认值 | F3：`BFF_TRUST_APISIX` 默认 `false` + 8894 暴露 + prod 仅注释 | ① dev/prod 默认值分离（prod 默认 true + CIDR 必填 fail-fast）② 中间件在 non-dev 且 CIDR 空时拒绝启动 ③ 移除 8894 宿主映射（dev 用 `docker exec`/容器网）—— ✅ **已裁定 = ①+②+③ 三件一起做（D-47，2026-10-07 用户拍板）** |
| **M3** | JWT 密钥轮换形态 | F11：无 kid/双密钥，轮换必致全站 401 | ① 双密钥并存窗口（BFF 验两个、签新的；APISIX consumer 需两条或插件能力核实）② 一次性原子轮换 + 接受存量失效（文档化）③ 外部 KMS/密钥服务（超范围）—— ✅ **已裁定 = ① 双密钥并存窗口（D-48，2026-10-07 用户拍板；落地方案属架构级，须附 ADR）** |
| **M4** | 备份生产化封装范围 | F15：dev 演练已过，封装未做 | ① 只做封装脚本 + dev 演练钉（最小）② 加 cron/定时（dev 可验，prod 转交运维）③ 含异地/加密（需存储凭据，超 dev） |
| **M5** | 组 B #9 发现"资源级越权"（若实测存在）的处置归属 | 可能触及 chat/analytics/assessment 多模块 | ① 属本阶段（越权横切）则修 ② 若为单模块业务缺陷 → 记账本转该模块 stage ③ 若需 schema（owner 列） → 升级 |

---

## 5. 收口门槛

- [ ] 20 个测试点全部有结论（PASS/FAIL/BLOCKED/N/A 四值，BLOCKED ≤ 1/3）
- [ ] 范围内缺陷走完 TDD（L1~L6 各自 RED→GREEN 记录）
- [ ] 回归钉：新增 `security-boundaries.spec.ts`（或按组拆分），**跑过且绿**
- [ ] **凡"改变前端用户可见行为"的修复（如 L1 改续期路径），必须 IAB 实测真实场景**（不能只跑单测/curl）——2026-10-08 补验轮教训：该要求原只落在 §6 风险表、未进本清单，致收口轮漏做
- [ ] §4 的 M2/M3/M4 决策**已升级给用户并落定**（未落定不得判 done）
- [ ] L5 架构改动附 ADR + `docs/architecture/decisions.md`
- [ ] 账本对账：F-27/28/182/200/201/202/203 逐条翻状态或写明仍挂理由
- [ ] `python scripts/e2e_stage_audit.py --all` → 0 FAIL；§13.3 第二方核对通过
- [ ] §2.5 收口自检三连 + 残留分支/worktree 清理

---

## 6. 风险与缓解

| 风险 | 应对 |
|------|------|
| **修 F1 会改变前端续期行为** | 前端失败路径已存在（`clearAuth` + 跳登录）；L1 落地后**必须 IAB 实测**真实过期场景（不能只跑单测） |
| 限流探针消耗配额影响后续测试点 | 每组 C 测试点前等待窗口重置（60s），或改用独立 `remote_addr` 视角；断言带窗口语义 |
| 越权枚举不全（路由清单遗漏） | 以 `main.go` 路由注册表为**唯一来源**逐条列清单再实测，禁止抽样；清单写入 report |
| 密钥轮换（L5）涉及 BFF+APISIX 双侧，改错致全站 401 | 先写守卫（红）→ 小步验证 → 保留回滚路径（旧 secret 可回填）；轮换演练在 dev 全链实测再落定 |
| "dev 配置" 结论被当成 prod 安全结论 | report 明确声明"本阶段验证的是 dev 配置"，prod 语义项标 `N/A + 理由` 或走 [M] 决策 |
| 端点无 `SameSite` 是 gin 能力缺位而非配置错 | L4 修法用 `http.SetCookie` 显式构造；同步更正注释（防 AP-02） |

---

## 7. 引用

- 执行协议：[RUNBOOK.md](../../RUNBOOK.md) · 反例：[anti-patterns.md](../../anti-patterns.md)
- 排期与状态：[roadmap.md](../../roadmap.md) · 账本：[discovered-unresolved.md](../../discovered-unresolved.md)（F-27 / F-28 / F-182 / F-200 / **F-201 / F-202 / F-203 本轮新登**）
- 决策：[decisions.md](../../decisions.md)（D-27 / D-28 / D-30 / D-35）· [architecture/decisions.md](../../../architecture/decisions.md)（决策 7 / 8 / 11 / 12 / 18）
- 执行记录（收口时写）：`stages/e2e-29-security-exceptions/report.md`
- 回归钉：`emotion-echo-web/e2e/security-boundaries.spec.ts`（拟）
