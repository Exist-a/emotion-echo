# ADR-2026-10：JWT 密钥轮换机制 = 双密钥并存窗口

- **状态**：accepted（2026-10-07 用户经 AskUserQuestion 拍板；E2E-29 D-48）
- **归属**：E2E-29（横切：异常与安全）测试点 #17；账本 **E2E-F-28**
- **关联**：决策 D-48（e2e-roadmap/decisions.md）· 架构决策登记（docs/architecture/decisions.md）· 实现 PR（E2E-29 D-48）

## 1. 背景（问题是什么）

`BFF_JWT_SECRET` 是**一把** HS256 密钥，同时被两处使用：

| 使用方 | 位置 | 作用 |
|--------|------|------|
| BFF | `emotion-echo-web-bff/internal/auth/jwt.go` | 签发 access token（`Sign`）+ 校验（`Parse`，主要服务 `/auth/refresh`） |
| APISIX | `deploy/apisix/seed.sh` 的 jwt-auth **consumer** | 验签网关上的每个受保护请求（`key_claim_name` 默认读 token 的 `key` claim 找 consumer） |

后果（E2E-F-28 登记的事实）：**单密钥下轮换必然让所有在途 token 失效**——已签发的 token 用旧密钥签名，网关找不到能验它的 consumer ⇒ 全员被踢回登录页。而"轮换"恰恰是密钥泄露/人员变动时**必须能做**的动作。现有 `scripts/test_bff_jwt_secret.sh` 只是"三处默认值一致"的静态检查，**不是轮换机制**。

## 2. 决策

**双密钥并存窗口 + key-id 路由**：

1. **BFF 侧**：`auth.Manager` 持"当前"与"上一把"两套 `(keyID, secret)`。
   - `Sign` 一律用**当前**密钥，并把**当前 keyID** 写进 token 的 `key`/`user` claim。
   - `Parse` 按 token 的 `key` claim 选密钥（命中 prevKeyID ⇒ 用上一把），未知/缺失则先试当前、再试上一把。
   - `ParseResetToken` 同样接受两把（reset token 只有 5 分钟寿命，恰逢窗口时不该失效）。
   - **过期语义不放宽**：无论用哪把密钥，`exp` 过期一律拒绝。
2. **网关侧**：窗口期为"上一把 keyID"**额外建一条 consumer**（`emotion_echo_bff_prev`）。APISIX 一个 consumer 只有一个 secret，所以靠 **token 里的 key claim** 区分新旧：旧 keyID → 旧 consumer，新 keyID → 新 consumer。
3. **窗口收尾**：清掉 `BFF_JWT_KEY_ID_PREV`/`BFF_JWT_SECRET_PREV` 并重跑 `seed.sh` —— 脚本会**自动删除** prev consumer（防止"窗口已关但旧密钥还能用"的静默残留）。

**轮换步骤（4 步）**：

| 步 | 动作 | 期望 |
|----|------|------|
| 1 | 在密钥存放处（dev = `deploy/.env.local`；prod = 密钥管理系统）把**当前**密钥改为**上一把**：`BFF_JWT_KEY_ID_PREV=<旧 keyID>`、`BFF_JWT_SECRET_PREV=<旧密钥>`；`BFF_JWT_KEY_ID=<新 keyID>`、`BFF_JWT_SECRET=<新密钥>` | 四个变量成对出现 |
| 2 | 重跑 `bash deploy/apisix/seed.sh` | 出现两条 consumer（`emotion_echo_bff` + `emotion_echo_bff_prev`） |
| 3 | 重建/重启 BFF，验证**新旧 token 都可用**（旧 token 未过期前不得失效） | 旧 token → 网关 200；新登录 token → 网关 200 |
| 4 | 等 ≥ 一个 token TTL（默认 24h）后清掉两个 `_PREV` 变量并重跑 seed | prev consumer 被删除；旧 token 至此失效 |

辅助脚本：`scripts/rotate_jwt_secret.sh`（`--status` / `--plan <新密钥>` / `--finalize`）——只做**编排与校验**，不读写 `.env.local`（AGENTS §四红线：密钥只进 gitignored 的 `.env.local`，脚本不代写）。

## 3. 为什么不选其它方案

| 备选 | 否决理由 |
|------|---------|
| **一次性原子轮换**（换密钥 + 重启 + 重跑 seed，接受存量失效） | 用户可感知（全员重登），且容器编排下"原子"难保证——BFF 与 APISIX 之间总有窗口，窗口内**新旧都不通**才是最难排查的形态。D-48 已否决 |
| **让 APISIX consumer 支持多 secret** | APISIX jwt-auth 的 consumer schema 只有单个 `secret`，无此能力；改插件 = 引入自维护分支，收益不成比 |
| **外部 KMS / 密钥服务** | 超出本项目 dev/单机部署范围（决策 3），且需要凭据与网络依赖 |
| **不做轮换（只做一致性检查）** | 等于承认"密钥泄露无法处置"（E2E-F-28 原始状态） |

## 4. 影响与边界

- **无 schema 变更**、无新服务、无新依赖（只用既有 `golang-jwt`）。
- **向后兼容**：不配 `BFF_JWT_KEY_ID` 时 keyID 缺省为历史值 `user`（`legacyKeyID`），既有 token 与既有 consumer 全部继续可用。
- **窗口必须有限**：`_PREV` 停留时间不得超过 token TTL（否则旧密钥长期可用 = 轮换白做）。本 ADR 明确"步 4"为**必须执行**项，且 seed.sh 在未配置 `_PREV` 时会主动删除 prev consumer（把"忘记收尾"变成可自愈）。
- **不做的事**：不做 token 黑名单/主动吊销（属 D-46 已记录的另一议题：`refresh` 硬 401 后服务端仍无吊销表）；不引入 `kid` header（key claim 已足够路由，且 APISIX 读的是 claim 不是 header）。
- **验证边界（如实）**：dev 环境实测覆盖"双 consumer 并存 + 旧 token 仍可用 + 新 keyID token 可用 + 收尾删除"；**prod 真轮换未演练**（需真部署环境，属运维范畴）。

## 5. 落地清单

| 项 | 位置 |
|----|------|
| 双密钥 Manager（TDD：`jwt_multikey_test.go` 6 例） | `emotion-echo-web-bff/internal/auth/jwt.go` |
| 三个 env（`BFF_JWT_KEY_ID` / `BFF_JWT_SECRET_PREV` / `BFF_JWT_KEY_ID_PREV`） | `internal/config/config.go` + `main.go`（`NewManagerMulti`） |
| 窗口期 prev consumer（含自动清理） | `deploy/apisix/seed.sh` Step 2.6 |
| 契约断言 | `deploy/apisix/seed_test.js`（窗口分支存在 + 收尾分支存在） |
| 编排/校验脚本 | `scripts/rotate_jwt_secret.sh` |
| 账本 | `E2E-F-28` → ✅（含 dev 实测证据） |
