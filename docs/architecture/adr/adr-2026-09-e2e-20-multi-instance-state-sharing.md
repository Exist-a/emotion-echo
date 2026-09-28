# ADR: E2E-20 多实例共享状态方案（登录锁定 Redis 化 + 验证码遗留端点回退 + APISIX policy=redis）

> 状态：🟢 accepted（2026-09-28 用户裁定）
> 编号：**D-28**（E2E 轨，`docs/e2e-roadmap/decisions.md` 同步登记）
> 来源阶段：E2E-20 多实例并发正确性
> 关联：D-01（找回密码方式 → 密保问题，2026-09-17）、D-27（Redis 保留并接入业务，2026-09-24）、账本 E2E-F-25 / F-142 / F-143 / F-144 / F-145

---

## 一、上下文

决策 3 是"本地 Docker **单机多实例**"，但项目有三处 in-memory 单实例假设，多实例下**静默失效**（账本 E2E-F-25）：

1. **BFF 登录失败锁定**（5 次错密码锁 15 分钟）：`loginFailures` map 进程内——实例 A 锁的用户打实例 B 不锁，锁可被绕过（E2E-20 RED 实测复现：BFF-2 正确密码 200 绕过）。
2. **验证码 60s 防枚举**：`verificationCodes` map 进程内——BFF-2 看不到 BFF-1 已发码（E2E-20 RED 实测复现：重复发码 803818 泄漏）。
3. **APISIX limit-count 限流**：`policy: local` 每节点各自计数，N 节点总配额放大 N 倍。

E2E-20 前半程按 D-27 决议做了三处 Redis 化（PR #114/#115/#116/#117），GREEN 双实例实测通过（12 测试点 11 项 PASS）。

**收尾时用户质疑**："可是现在没有验证码功能了啊？你为什么要加。"

## 二、核实的事实

- `/api/v1/auth/verification-code`（注册/找回密码的**邮箱验证码**，非登录图形验证码）端点从项目第一天就存在，**不是 E2E-20 新增**。
- 但 **D-01 决议（2026-09-17 用户拍板）已弃用该流程**：注册验证码步骤删除、找回密码改密保问题、落地要点明确写"BFF 的 `verification-code` 端点改为 `verify-security-answer`（归属 E2E-07）"。
- E2E-07 落地时**只增未删**：新端点 `verify-security-answer` 已上线，旧端点 `verification-code` 一直在路由表里（`auth_handler.go:93`），前端 `apiRoutes.ts:30` 也残留条目 → 成为遗留物。
- E2E-20 对它的 Redis 化是在给一个"按决议不该存在"的端点做加固——方向错误。

## 三、决议（用户 2026-09-28 裁定）

用户原话："主要的问题是手机号、邮箱验证码早就不用了，之前决策说过了，所以目前不应该有验证码。"

据此拆成三部分处置：

| 部分 | 处置 | 依据 |
|------|------|------|
| **登录锁定跨实例** | **保留 Redis 化**（`authlock.LoginLockStore` 3 方法 + `LOGIN_LOCK_BACKEND=redis`） | 这是 D-27 的正当接入点，RED/GREEN 实测有效，防御语义真实存在 |
| **验证码防枚举跨实例** | **回退**——`VerificationCodeStore` 独立接口、仅 in-memory 实现，代码 + 契约测试（`TestInterfaceShrink_VerificationCodeStore_NotRedis`）锁死禁止再 Redis 化；回归钉 #11b 删除 | 端点是 D-01 遗留物待删（新账本 E2E-F-144），不应为它建跨实例共享存储 |
| **APISIX policy=redis** | **保留** | 与验证码无关；限流跨节点放大是真实的多实例问题。跨节点防放大的运行时实测（plan #4）在 dev 单节点环境无法进行，留账 E2E-F-145 归 E2E-25 |

遗留端点 `verification-code` 本身的删除（BFF 路由 + 前端 apiRoutes + proto 字段弃用评估）**不在 E2E-20 范围内**，按"范围外只记账不修"纪律登记 E2E-F-144，归属 E2E-07 收尾 / 独立小 PR。

## 四、后果

**正面**：

- 登录锁定跨实例正确性有 RED→GREEN 实测 + 回归钉（`multi-instance-smoke.spec.ts` #11a/#11c）双重保障。
- 验证码方向的错误在合并前被纠正，避免"给死端点加基础设施"的债继续累积。
- 契约测试把"验证码禁止 Redis 化"变成机器可检查的约束，后续贡献者不会重蹈覆辙。

**负面 / 风险**：

- 遗留端点 `verification-code` 仍存在（in-memory 防枚举语义，单实例下行为不变）；其删除依赖 E2E-F-144 被排期。
- APISIX `policy: redis` 的运行时跨节点语义未实测（E2E-F-145），目前只有 seed.sh 结构断言（48/48）保证配置形状正确。
- F-142/F-143 揭示的"降级语义掩盖缺陷"模式（单测全绿但生产路径静默降级）在 `RedisLimiterBackend` 等其他 Redis 接入点同样可能出现，后续接入点需引以为戒。

## 五、调研依据（AGENTS §〇.6）

- 已读实现：`emotion-echo-web-bff/internal/authlock/{store,inmemory,redis_store}.go`、`internal/handler/auth_handler.go`（路由 case + 验证码调用点）、`main.go`（装配 + defer Close）、`shared/pkg/middleware/limiter.go`。
- 已读决策：`docs/e2e-roadmap/decisions.md` D-01（验证码弃用决议原文）、D-27（Redis 接入业务）；E2E-20 STATUS v2 §五（用户质疑 + 处置选项）。
- 已读账本：E2E-F-25（三处失效）、F-142/F-143（GREEN 实测真缺陷）。
- 全仓触点 grep：`verification-code` 在 BFF / user-svc / web / proto 的分布（见 E2E-F-144 条目）。
