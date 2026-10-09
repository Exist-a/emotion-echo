# E2E-31 内部 RPC 收敛 — 会话状态（STATUS）

> 用途：AGENTS.md §八「收工三查」第 1 条的**本轨状态记录**。**已做/未做分列，禁止美化。**
> 快照时间：**2026-10-09**（用户指示「先落地文档，暂时停下，不 push」）。
> 任务书：[plan.md](plan.md) · 执行记录：[report.md](report.md)（含 §8/§8.1 第二方核对两轮）。

---

## 1. 已做 ✅

### 1.1 交付（本轮共 8 个 PR 合入 main，`main = 477da4f`）

| PR | 内容 |
|---|---|
| #181 | 更正 `adr-2026-09-survey-http-bypass` 漂移 + 收窄 findings 措辞 |
| #182 | **立项 E2E-31**（roadmap 第九批 + 账本 F-208 转出 + 新登 F-210/211/212 + 决策 D-50） |
| #183 | 修 E2E-F-97 回归钉固定 sleep 假红（新登 **E2E-F-213**） |
| #184 | 补 **IAB 实测**（改动前基线 4 图）+ plan §0.3/§0.4 |
| #185 | **主体**：proto 契约扩展 + 服务端转换重写 + BFF 补 `ListResults`/`GetResult` + **删 5 处恒真 HTTP 旁路** + 画像切回共享 gRPC（含 report.md、新登 **E2E-F-214**） |
| #186 | 更正 proto `submitted_at` 注释（实测毫秒，非秒） |
| #187 | T-1/T-2 裁定落地 + 第二方核对 4 条件处置 |
| #188 | 修 report 内部数字自相矛盾 + 补 §8.1 复核记录 |

### 1.2 验证结论（20 个测试点）

- **PASS 20 / FAIL 0 / BLOCKED 0 / N/A 0**；`e2e_stage_audit.py --all` → **31 阶段 0 FAIL**
- Playwright 分 project **chromium 24/24 + mobile 24/24**；前端 `vitest 657 passed` + `tsc 0 错`
- **IAB 实测**：详情页（新增描述文案）/ 结果弹窗 27·极重度 / 我的空间人格雷达五维度 → 截图 `screenshots/01~07`，逐张查看，与基线对比无劣化
- 网关实测：列表带 description；详情 `questions` 数组 9 题、`q[0].id=q1`、`options[0]={id,score,text}`；提交 `totalScore 27 / riskLevel extreme / factorScores / scoreKind`
- 回归钉：proto 往返契约、bufconn gRPC 用例、**服务端转换直接单测 5 条**、旁路回归钉（含负向对照）
- **第二方核对两轮**（独立子代理）：首轮「有条件通过」+ 抓到我 2 处过度声明；复核轮 (a)(c)(d)(e) 满足、(b) 报告数字陈旧 → 已修

### 1.3 账本与决策

- `E2E-F-208` → **✅ 已解决**（assessment 链切回 gRPC + 剩余 7 处按裁定"保留+标注+守卫"）
- 新登：`E2E-F-213`（固定 sleep 假红，✅）、`E2E-F-214`（限流致 E2E flaky，⚠️ 未解决）、`E2E-F-210/211/212`（归 E2E-30）
- 新增决策 **D-50**（xtts 暂时停用）；用户裁定 T-1/T-2/T-4
- 新增 CI 守卫 **23/23** `scripts/check_dead_grpc_inventory.sh`（守清单真实性，含负向对照）

---

## 2. 未做 ❌

| # | 未做项 | 归属 | 影响的判据 |
|---|--------|------|-----------|
| 1 | **E2E-F-214 去向未定**（dev 限流 60/60s 致 48 条 E2E 连跑必出 ~2 条假红） | **需用户定**：① 本阶段内修 ② 转挂 spec/CI owner | **这是 E2E-31 判 `partial` 的唯一原因**；未定则不能翻 `done` |
| 2 | E2E-30 的 **L1 smoke 可跑化**（`:8894` → 网关 + Bearer） | E2E-30 | E2E-30 仍 `📝 已建档` |
| 3 | **E2E-F-210**（BFF `/health/ready` 硬依赖 xtts ⇒ 标准起栈必 unhealthy + 级联 seed 不跑）、**F-211**（网关无 BFF health 路由）| E2E-30（M2 族，须与 F-186 一并收敛口径） | 环境基线不可复现健康 |
| 4 | **E2E-F-212 / 决策 D-50** 三项影响面：readiness 摘除 xtts、cloud 失败时的用户可见行为、编排与 15.3GB 镜像去留；**架构级 ⇒ 须 ADR + 架构决策登记** | 用户已定方向，**未落地** | 回退链现为"cloud 单链路" |
| 5 | 冷启动首次 gRPC 调用 `DeadlineExceeded`（重启后首调 5s 超时、重试即 2ms）是否并入 **E2E-F-115** 同型治理 | 需用户定 | 无（已记录） |
| 6 | `ASSESSMENT_TRANSPORT=http` 回滚位是否彻底删除 | 执行者已按计划"保留"；如需删须明示 | 无 |

---

## 3. 待用户决策（恢复时先问这三条）

1. **E2E-F-214 去向**：① 本阶段内修（spec 层共享令牌 / 请求预算 / 分 project 脚本化）② **转挂** spec 与 CI 门禁 owner（改账本归属）→ 选 ② 则 E2E-31 可翻 `done`。
2. **E2E-31 是否判 `done`**（前置 = 第 1 条落定；其余 20 测试点已全 PASS、第二方核对已过）。
3. **冷启动 `DeadlineExceeded`** 是否并入 E2E-F-115。

---

## 4. 环境现状

- dev 栈 **19 容器全 healthy**（其中 `xtts`/`sensevoice`/`llm-service` 由 `ai` profile 起，非本次会话所起）；`:3000` 由 web 容器服务；网关 `:19080`。
- `deploy/.devmode-session` **已释放**（原 lane-e 锁 `until: 2026-10-08 23:59` 已过期）。
- 被验镜像：`emotion-echo/web-bff:v0.1.37`（`sha256:35e3fbac8072`）+ `emotion-echo/assessment-svc:v0.1.5`。

---

## 5. 本地未推送改动（按用户指示：**不 push**）

- 本文件所在分支 **`wip/e2e-31-status`**（1 commit，**未 push**；`main` 保持 `= origin/main = 477da4f`）。
- 除本文件外无其他未提交改动（`git status` 干净）。

---

## 6. 恢复时的下一步

1. 先问 §3 的三条决策（E2E-F-214 去向最关键）。
2. 若继续 E2E-31：按决策处置 F-214 → 翻 `done` → 收口（清 §0 的 `T-5`、更新 roadmap/plan/report 三处 status）。
3. 若转向 E2E-30：按 [plan §0.2](../e2e-30-data-contract-closure/plan.md) 复核清单起栈，先做 L1（smoke 基址 → 网关 + Bearer，健康前置换判据，见 E2E-F-211）。
4. **注意**：`report.md` §0 的 `T-5` 与 `plan.md` 的 `status: partial` 需随阶段翻 `done` 一并改（三处 status 必须一致，否则 audit A9 报 FAIL）。
