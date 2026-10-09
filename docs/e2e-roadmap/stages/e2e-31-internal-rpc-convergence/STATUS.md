# E2E-31 内部 RPC 收敛 — 会话状态（STATUS）

> 用途：AGENTS.md §八「收工三查」第 1 条的**本轨状态记录**。**已做/未做分列，禁止美化。**
> 快照时间：**2026-10-09（收口轮）**。上一版快照为同日「执行轮收尾、按用户指示不 push」，本版为**收口轮**结果。
> 任务书：[plan.md](plan.md) · 执行记录：[report.md](report.md)（含 §8/§8.1 两轮第二方核对、**§9 收口轮**）。

---

## 1. 已做 ✅

### 1.1 用户裁定（AskUserQuestion，2026-10-09）

| # | 裁定 |
|---|---|
| 1 | **E2E-F-214 去向 = ① 本阶段内修**（不转挂 spec/CI owner） |
| 2 | **E2E-31 = 修完按标准测试一遍，没问题即判 `done`**（要求"测试要按照标准来"） |
| 3 | 冷启动 gRPC 首调 `DeadlineExceeded` = **并入 E2E-F-115 同型治理**（不新开编号） |

### 1.2 F-214 根因复核（含更正上一轮归因笔误）

- 限流**在 APISIX 侧**（上一轮写"BFF 限流"是笔误，已原文改写）：route100（`/api/v1/*`）与 route110（auth 白名单）**各挂** `limit-count`：`count=60 / time_window=60 / key=remote_addr`（Redis 键 `plugin-limit-count:v1:/apisix/routes/100:<addr>`）。
- 连跑实测：route100 **112** 次、route110 **50** 次（= 24 用例 × 2 project 各登录一次 + 2）；`GET /surveys` 42 次、详情 26 次。
- **按标准单命令复现**（改动前）：`47 passed / 1 failed`（`[mobile] personality #4`），同一分钟 route100 **72** > 60 ⇒ 429。

### 1.3 修复（spec 层请求预算；**产品限流配置一字未改**）

新增 `emotion-echo-web/e2e/helpers/gateway.ts`，三个 spec 全部接入：

1. `loginOnce` —— 令牌 worker 内复用（TTL 24h）⇒ route110 **50 → 2**
2. 只读种子数据缓存（`/api/v1/surveys`、`/api/v1/surveys/{id}`；**`/surveys/results*` 禁缓存**）⇒ 每 project 命中 15 次
3. 滑动窗口预算 `BUDGET=55`/60s；`gwGet`/`gwPost` 覆盖显式调用 + `page.route('http://localhost:19080/**')` 覆盖浏览器自身请求

### 1.4 验证结论

| 项 | 证据 |
|---|---|
| **标准单命令连跑** | `npx playwright test quiz+survey-scoring+personality`（双 project）→ **48/48 × 连续 4 轮**（1.7/1.7/1.6/1.7m） |
| 闸门统计（`E2E_GATE_DEBUG=1`，每 project） | `issued=42 blocked=0 waitedMs=0 cacheHits=15`（修前 `57 / blocked=5 / 14383ms`） |
| APISIX 侧（独立证据） | 每分钟峰值 **54（<60）**；**368×200 + 9×404，0 × 429** |
| 回归钉 + 负向对照 | `scripts/test_e2e_gateway_budget.sh` → PASS；对真实 `quiz.spec.ts` 注入裸 `page.request` → `rc=1`，还原后 md5 一致 → `rc=0` |
| 门禁 | `vitest 657 passed`（78 文件）；`nuxt typecheck` rc=0；e2e 四文件定向 `tsc` rc=0；`e2e_stage_audit.py --all` **31 阶段 0 FAIL** |

### 1.5 文档与账本

- 账本：`E2E-F-214 → ✅ 已解决`（并原文更正"BFF 限流"笔误为 APISIX）；冷启动 gRPC 首调**并账至 E2E-F-115**（机制已定位：`main.go:376` 跨下游统一兜底 `ClientDialOptions(..., 5*time.Second)` → `ClientTimeoutInterceptor`）
- `report.md`：status → `done`；§0 清单清空（T-1~T-5 全销账）；§2 #15/#16 改标准口径；§5 增守卫行；§6 两条收尾；§7 第 5/10 行；**新增 §9 收口轮**（根因/修复/证据/覆盖度诚实记录）
- `plan.md`：`status: done` + last-updated + P4/report 行 + 「一句话状态」
- `roadmap.md`：E2E-31 行 → ✅ done；「当前激活」链改为 **E2E-30**（E2E-31 移入「上一轮激活（已完成）」）；last-refresh
- `.github/workflows/e2e-guards.yml`：新增 **守卫 24/24**

---

## 2. 未做 ❌

| # | 未做项 | 归属 / 说明 |
|---|--------|-----------|
| 1 | **本轮 commit / PR 尚未推送** | 本轮收口动作的最后一步；完成后按 §2.5 三连自检 + 删源分支 |
| 2 | **E2E-30 本体未开工**：L1 smoke 可跑化（`:8894` → 网关 + Bearer）/ helm 渲染回归 / 名下属账本 | E2E-30（📝 已建档）——已在 roadmap 置为「当前激活」 |
| 3 | **E2E-F-212 / 决策 D-50 三项影响面**（readiness 摘除 xtts、cloud 失败的用户可见行为、编排与 15.3GB 镜像去留） | 用户已定方向、**未落地**；架构级 ⇒ 须 ADR + 架构决策登记（归 E2E-30） |
| 4 | **冷启动 gRPC 首调 5s deadline 本身未修** | 按用户裁定**只并账**到 E2E-F-115，不在本阶段改 |
| 5 | `ASSESSMENT_TRANSPORT=http` 回滚位是否彻底删除 | 执行者按计划「保留」；如需删须用户明示 |
| 6 | E2E-F-210 / F-211（BFF readiness 硬依赖 xtts / 网关无 BFF health 路由） | E2E-30（M2 族，须与 F-186 一并收敛口径） |

> 归属本阶段（E2E-31）的账本条目 **F-208 / F-213 / F-214 全部 ✅**，**无未解决项** —— 满足 A5「判 done 前账本须干净」的前提。

---

## 3. 环境现状

- dev 栈 **19 容器全 healthy**（`xtts`/`sensevoice`/`llm-service` 由 `ai` profile 起，非本会话所起）；`:3000` 由 web 容器（生产构建）服务；网关 `:19080`。
- `deploy/.devmode-session` **未被占用**（无锁文件；AGENTS §八 收工三查第 2 条满足）。
- 被验镜像：`emotion-echo/web-bff:v0.1.37` + `emotion-echo/assessment-svc:v0.1.5`（本轮**未重建镜像**——改动只在 e2e spec / 守卫 / 文档）。

---

## 4. 恢复时的下一步

1. 若接着做：**E2E-30 数据契约收口**（roadmap 当前激活）。开工先按 [plan §0.2](../e2e-30-data-contract-closure/plan.md) 复核清单恢复环境基线，**第一件事是 L1**（smoke 基址 `:8894` → 网关 + Bearer，健康前置换判据）。
2. 注意 E2E-30 开工前需处理 **F-210/F-211**（标准起栈 BFF 必 unhealthy + 网关无 BFF health 路由），否则"环境基线可复现健康"这条无法成立。
