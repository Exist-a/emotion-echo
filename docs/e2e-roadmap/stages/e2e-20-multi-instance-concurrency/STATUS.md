# E2E-20 多实例并发正确性 — STATUS v3（收尾裁定完成，待 §13.3 签字）

> **阶段状态：🟡 partial —— 唯一剩余项 = §13.3 第二方核对签字。**
> 依据 RUNBOOK §7#10：执行者不得自行宣布 done；等用户对 §13.3 核对清单签字后翻 done。
>
> v1（开工记录）见 git history（commit `a15d855` PR #113）；v2（收尾记录，未推送）见本会话 git history。

---

## 一、状态结论

| 项 | 值 |
|----|-----|
| 阶段状态 | 🟡 **partial（待 §13.3 签字）** |
| roadmap / plan / report 三处 status | 均为 `partial`（一致，audit A9 通过） |
| audit `--all` | 合并前需复跑（本分支合并时跑） |
| §13.3 第二方核对 | **未执行**（核对清单见 §五，等用户签字） |
| 判 partial 的理由 | ① §13.3 未签字；② #4 限流跨节点放大**架构性未实测**（E2E-F-145 归 E2E-25） |

---

## 二、已完成（有证据）

### 修复（5 个 PR，均已合入 main）

| PR | commit | 内容 |
|----|--------|------|
| #113 | `a15d855` | 开工报告 + STATUS v1 |
| #114 | `879caee` | shared `RedisLimiterBackend`（Lua 原子 token bucket）+ 11 条 miniredis 单测 |
| #115 | `727c5dd` | BFF `authlock` 子包（LoginLockStore 接口 + InMemory/Redis 双实现）+ handler 重构 + 18 条单测 |
| #116 | `5b85ab0` | seed.sh `limit-count policy=redis` + 6 env vars + 3 条结构断言（48/48） |
| #117 | `7b50fcb` | **GREEN 步实测抓到的 2 个真缺陷**（F-142 / F-143）+ 回归钉 + 截图 + 全量回归 |

### 收尾裁定轮（2026-09-28 本会话，commits 496a810 / 96d02a9 / f3a9a57 / 4d48455）

用户裁定（详见 §四）：**验证码 Redis 化方向回退，登录锁定 + policy=redis 保留**。落实：

1. **RED**（`496a810`）：`TestInterfaceShrink_VerificationCodeStore_NotRedis` 契约测试——RedisStore 不得实现验证码存储契约（先红）
2. **GREEN**（`96d02a9`）：`LoginLockStore` 收缩为登录锁定 3 方法；`VerificationCodeStore` 独立接口仅 in-memory；`NewAuthHandler` 增第 4 参 + nil fail-fast；web-bff 全量测试 + vet 绿
3. **回归钉**（`f3a9a57`）：spec 删 #11b（验证码跨实例语义不存在可钉），#11a/#11c 保留
4. **账本 + ADR**（`4d48455`）：E2E-F-144（遗留端点未按 D-01 删除）/ E2E-F-145（policy=redis 架构性未实测）/ F-25 补裁定备注；ADR `adr-2026-09-e2e-20-multi-instance-state-sharing.md` = **D-28**

### 测试点结果（12 项，逐条证据见 [report.md](report.md) §2；裁定后 #3/#7/#11b 作废）

| 组 | 测试点 | 结论 |
|----|--------|------|
| 前置 | #1 双 BFF 实例 + Nacos 双注册 | PASS |
| RED 复现 | #2 登录锁定跨实例失效 | PASS（RED 复现 + 修后 BFF-2 第 7 次 423） |
| RED 复现 | #3 验证码防枚举跨实例失效 | RED 复现 PASS；**裁定后作废**（端点为 D-01 遗留物，E2E-F-144） |
| 架构性 | #4 限流跨节点放大 | 架构性未实测（E2E-F-145 归 E2E-25） |
| TDD 修复 | #5/#6/#8 | PASS（保留有效） |
| TDD 修复 | #7 验证码 Redis 化 | **裁定后回退作废** |
| GREEN | #9 双实例并发 10 次锁定态唯一 | PASS（5×401 + 5×423 精确阈值触发） |
| 视觉 | #10 锁定提示 toast | PASS（截图 `screenshots/10-lock-prompt-login.png`） |
| 回归钉 | #11 `multi-instance-smoke.spec.ts` | PASS（首跑 6/6；裁定后 = #11a+#11c 双 project） |
| 全量 | #12 go / shared / vitest / spec | PASS（裁定轮 web-bff 全绿 + vet 复验） |

### GREEN 步抓到的两个真缺陷（已当轮闭环）

1. **F-142 defer 作用域错误**：`authLockStore` 装配在 `registerRoutes` 内，`defer closer.Close()` 在函数返回时关闭 Redis 客户端 → 静默降级 → keys 恒空 → 跨实例锁定"看起来修好了实际没生效"。诊断三轮对照实验。
2. **F-143 超时预算不足**：100ms 装不下首次 TCP 拨号 + DNS + EVAL。

两者叠加是**降级语义掩盖缺陷**的典型：单测全绿（mock 可控）、miniredis 全绿，但生产路径静默降级。

---

## 三、未完成（诚实列明）

| # | 未做项 | 原因 | 归属 / 阻塞 |
|---|--------|------|------------|
| 1 | **§13.3 第二方核对签字** | 无用户签字（RUNBOOK §7#10 硬性要求） | **阻塞 done**（核对清单见 §五） |
| 2 | **#4 限流跨节点放大实测** | dev 栈单 APISIX 节点 ⇒ 无法验证跨节点语义 | **E2E-F-145** 归 E2E-25 |
| 3 | **verification-code 端点删除** | D-01 遗留物（E2E-07 只增未删）；超出 E2E-20 范围 | **E2E-F-144** 归 E2E-07 收尾/独立小 PR |
| 4 | **plan.md §5 DoD 未勾** | 按 §7#10 不自宣 done | 随 §13.3 签字一并 |
| 5 | **Lane O 的 vitest 预存失败** | `webllmEngine.dynamicImport.test.ts` 可选依赖未装（Lane O 独占列禁触） | 范围外，记录不修 |

~~worktree 清理~~：4 个 worktree 已清理（`git worktree list` 仅剩主工作区）。
~~devmode-session 锁释放~~：`deploy/.devmode-session` 已不存在。

---

## 四、用户裁定记录（本阶段最高价值事件）

**质疑**（2026-09-28）："可是现在没有验证码功能了啊？你为什么要加。"

**核实**：`/api/v1/auth/verification-code` 端点从项目第一天就存在（不是本阶段新增），但 **D-01（2026-09-17 用户拍板）已决议弃用**该流程（注册验证码步骤删除、找回改密保、落地要点明确"BFF 的 verification-code 端点改为 verify-security-answer"）——E2E-07 落地时只增未删，旧端点成遗留物。本阶段给遗留物做 Redis 化方向错误。

**裁定**（2026-09-28）："主要的问题是手机号、邮箱验证码早就不用了，之前决策说过了，所以目前不应该有验证码。"

**处置**：登记 **D-28**（ADR `docs/architecture/adr/adr-2026-09-e2e-20-multi-instance-state-sharing.md`）——登录锁定 Redis 化保留（D-27 正当接入点）/ 验证码回退 in-memory 且契约测试锁死禁止再 Redis 化 / APISIX policy=redis 保留。

**元教训**：修复前先核实"目标功能本身是否还被决议保留"——否则会给死端点加基础设施（本例 RED 复现、单测、双实例实测全部"正确地做了一件不该做的事"）。

---

## 五、§13.3 第二方核对清单（等用户签字）

| # | 核对项 | 证据位置 |
|---|--------|---------|
| 1 | 登录锁定跨实例修复有效（RED：BFF-2 200 绕过 → GREEN：BFF-2 423） | report.md §2 #2/#9 |
| 2 | 验证码方向按裁定回退，契约测试锁死 | commits 496a810/96d02a9；`TestInterfaceShrink_VerificationCodeStore_NotRedis` |
| 3 | APISIX policy=redis 已持久化（结构断言 48/48）；跨节点实测留账 E2E-F-145 | PR #116；账本 |
| 4 | 并发 10 次锁定态唯一（5×401 + 5×423） | report.md §2 #9 |
| 5 | 视觉证据截图已查看 | screenshots/10-lock-prompt-login.png |
| 6 | 回归钉 spec 存在且首跑绿（裁定后 #11a+#11c） | emotion-echo-web/e2e/multi-instance-smoke.spec.ts |
| 7 | 全量回归绿（web-bff 10 包 + shared 17 包 + vitest 568/568） | report.md §2 #12 |
| 8 | 账本对账：F-25 关账（含裁定备注）/ F-142/F-143 闭环 / F-144/F-145 新登 | discovered-unresolved.md |
| 9 | D-28 ADR 已立并双侧登记 | adr-2026-09-e2e-20-multi-instance-state-sharing.md + decisions.md |
| 10 | 遗留端点删除与跨节点实测不在本阶段范围，已记账（E2E-F-144/145） | 账本 |
| 11 | 三处 status 一致 partial；不自行翻 done | plan/report/STATUS frontmatter |

**用户签字后动作**：plan §5 DoD 勾选 → 三处 status 翻 done → roadmap 主表同步 → `audit --all` 复跑 0 FAIL → §2.5 收口三查。

---

## 六、引用

- [report.md](report.md)（12 测试点逐条证据 + §7 收尾裁定轮）
- [plan.md](plan.md)（详档：12 测试点定义 + §6 风险表）
- [ADR D-28](../../../architecture/adr/adr-2026-09-e2e-20-multi-instance-state-sharing.md)
- [RUNBOOK §7 收口契约 11 项 + §13.3 核对清单](../../RUNBOOK.md)
- [parallel-tracks.md §五 收工三查](../../../_meta/parallel-tracks.md)
