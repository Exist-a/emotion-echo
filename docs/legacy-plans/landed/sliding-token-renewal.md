---
status: landed
landed: 2026-10-08（PR #178）
priority: high
owner: User（决策）+ Lane E（执行）
created: 2026-10-08
type: feature
source: E2E-29 补验轮发现 **E2E-F-207**（前端 `code===10002` 自动续期为死代码、滑动续期能力实为缺失）→ 用户 2026-10-08 经 AskUserQuestion 拍板「**实现真正的滑动续期**」
depends-on: []
related-stages:
  - e2e-29-security-exceptions（发现来源；该阶段本身已 done，本条为其收口后的 follow-up，不回挂）
related-decisions:
  - D-49（docs/e2e-roadmap/decisions.md，本计划决策）
---

# Plan — 访问令牌滑动续期（sliding token renewal）

## §A 上下文（本文假设）

**已读代码（AGENTS §〇 功课 ①）**：
- `emotion-echo-web/app/composables/useApi.ts`（`refreshToken` `:161`、401 处理 `:302-338`、SSE `:453-477`）
- `emotion-echo-web/app/stores/user.ts`（`setAccessToken` `:169`、`clearToken` `:186`、`isTokenExpired` `:208`、`init` `:389`）
- `emotion-echo-web/app/plugins/init.ts`（`userStore.init()` `:45`；`// 这里可以触发自动刷新逻辑` 占位 `:60`）
- `emotion-echo-web/app/lib/clientAccessToken.ts`（token 读取范式）
- `emotion-echo-web-bff/internal/handler/auth_handler.go`（`refresh` `:227-248`：**有效令牌 → 新令牌 + `setAccessTokenCookie` 重发 cookie**）
- `deploy/apisix/seed.sh`（jwt-auth 读 cookie `access_token`）

**已查决策/ADR（功课 ②）**：e2e-roadmap `decisions.md` D-46（refresh 硬 401）/ D-47 / D-48；`docs/frontend/design.md:321`（`TOKEN_EXPIRED: 10002` —— 声明过、**从未实现**）；账本 `E2E-F-207`。

**本文假设（与现状对照）**：
1. **假设**后端 refresh 对**有效**令牌返回全新 TTL 的令牌 —— **实测成立**（curl：`iat/exp` 差 = 86400）。
2. **假设**后端 refresh 会重发 HttpOnly cookie —— **成立**（`auth_handler.go:246`）。故滑动续期**无需改后端**。
3. **假设**响应式续期（靠 401 触发）在本拓扑**不可行** —— 成立：APISIX 在过期令牌上先返 401 且**不带业务 code**，BFF 永远收不到该请求（E2E-F-207）。故只能**主动**（过期前）续期。

## §B 问题

令牌过期 → 前端无任何续期动作 → `clearAuth()` + 跳 `/login`。用户每次过期都要重登；`useApi.ts` 里那条 `code===10002` 续期分支是**死代码**（无后端下发 10002）。

## §C 方案

**主动续期（proactive renewal）**：令牌仍有效时，前端在其寿命 **75%** 处调 `POST /api/v1/auth/refresh` 换新令牌。

- **触发点**：① 客户端启动（`plugins/init.ts`，已登录时）；② 页面可见性恢复（`visibilitychange → visible`，覆盖"长时间后台后计时器被节流"）；③ 每次续期成功后重排下一次。
- **阈值**：`renewAt = exp - max(0.25 * (exp - iat), 60s)`；`iat` 缺失时退化为"剩余时间的 25% 处"。
- **失败语义**：续期失败（网络/401）**不**清登录态、**不**跳转——留待真正用到该令牌时由 401 兜底（与现状一致）。
- **删除死代码**：`useApi.ts` 的两处 `code===10002` 分支删除；`refreshToken` 改名导出 `refreshAccessToken` 供调度器调用（401 一律 `clearAuth()` + 跳 `/login`，行为等价）。
- **多标签页**：各标签独立调度、各自续期；后写 cookie 生效。可接受（已在 §E 记录）。

**被否备选**：① 改 APISIX 让过期令牌透传到 BFF 再续期（网关语义改动，风险大、收益仅为兼容死分支）；② 服务端会话（session id）替换 JWT（超本次范围）。

## §D 验收（DoD）

- [ ] 纯逻辑单测：`parseJwtTimes` / `nextRenewalDelayMs` / 调度器（注入时钟与定时器）全绿
- [ ] `useApi.ts` 无 `10002` 残留；`refreshAccessToken` 有真实调用方
- [ ] 全量 `vitest` 绿
- [ ] 运行时：IAB 实测"已登录 → 调度器已排程（delay ≈ 寿命 25%）→ 手动触发续期 → `/auth/refresh` 200 + 新 cookie + 令牌轮换"
- [ ] Playwright 回归钉：已登录时调度器已安装且续期调用可达

## §E 边界与已知取舍

- 续期只在**客户端**进行；SSR 不做（沿用现状）。
- 用户离线/离开超过 TTL ⇒ 仍会过期登出（预期，滑动续期只覆盖"活跃会话"）。
- 多标签页各自续期 ⇒ 可能产生多次 refresh（幂等，无副作用；`refreshPromise` 锁是**单标签**内并发去重）。
- 阈值 75% 为**可调常量**（`RENEW_AT_FRACTION`），如需更保守（如 50%）改一处即可。

## §F 执行记录（2026-10-08，PR #178）

**落地**：新增 `app/lib/tokenRenewal.ts`（`parseJwtTimes` / `nextRenewalDelayMs` / `createTokenRenewal`）+ `app/lib/tokenRenewal.test.ts`（16 例）；`plugins/init.ts` 接线；`useApi.ts` 删两处 `code===10002` 死分支并把 `refreshToken` 导出为 `refreshAccessToken`；新增回归钉 `e2e/token-renewal.spec.ts`。

**验收**：`vitest` 全仓 **657 passed**；`tsc --noEmit` 干净；Playwright `token-renewal` + `jwt-expiry` 两 spec × chromium/mobile = **8 passed**；IAB 实测 `renewNow()` → `/auth/refresh` **200** + 新令牌 `exp - iat = 86400`（全新 TTL，滑动生效）+ 调度器重排。

**执行期抓到并修掉的 2 个真问题**（都是"只跑单测测不出、必须运行时/端到端才暴露"）：
1. **调度器被 `isAuthenticated` 挡住**：该 computed 还要求 `userInfo.id`，而 userInfo 是页面元数据、`init()` 后可能尚未恢复 ⇒ 调度器从不排程（IAB 实测 `window.__tokenRenewal` 缺失定位）。修法：改为**无条件调用**（无令牌时 `schedule()` 内部自行跳过），与 `auth.global.ts` v2 只看 accessToken 的口径一致。
2. **SPA 登录后不排程**：插件只在整页加载时跑一次，登录是 SPA 跳转 ⇒ 登录后调度器一直是未排程态（Playwright 端到端实测抓到）。修法：`watch(() => readToken(), () => renewal.schedule())`，登录/续期改变令牌即重排。

**遗留**：多标签页各自调度（见 §E）；续期句柄 `window.__tokenRenewal` 仅 dev 构建暴露（回归钉依赖此前提）。
