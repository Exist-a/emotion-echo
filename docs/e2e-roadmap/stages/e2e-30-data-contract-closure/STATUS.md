# E2E-30 数据契约收口 — 会话状态（STATUS）

> 用途：AGENTS.md §八「收工三查」第 1 条的**本轨状态记录**。**已做/未做分列，禁止美化。**
> 快照时间：**2026-10-12**（执行轮续，**阶段未收口**）。任务书：[plan.md](plan.md) · 执行记录：[report.md](report.md)（**§0 未完成清单是唯一真相源**）。

---

## 1. 已做 ✅

### 1.1 交付（本轮 2 个 PR）

| PR | 内容 | CI |
|---|---|---|
| [#194](https://github.com/Exist-a/emotion-echo/pull/194) | **起栈阻断修复**：`analytics-svc` 冷启动 SIGSEGV（`nil.WithMaxRetries`）→ 拖垮 `docker compose up`；修 nil-guard（TDD）+ compose `depends_on: kafka: service_healthy`。新帐 **E2E-F-216** | 36/36 ✅ 已合并 |
| [#195](https://github.com/Exist-a/emotion-echo/pull/195) | **执行轮续**：#14（F-187）/ #15（F-204）/ §8 判据（F-215）/ #18（F-210）+ report/STATUS/plan + 账本对账 | 见 PR |

### 1.2 测试点结论（本轮新增 3 项 PASS）

- **#14（F-187）PASS**：voice `Register` 补 `r.HEAD`；`PutObject` 加 3s deadline + `isStorageUnavailableErr→503`。TDD 3 用例 RED→GREEN；**运行验证**（网关 HEAD 返回我们 handler 的 JSON 404，非 gin `text/plain`）。
- **#15（F-204）PASS**：`BootstrapSkyWalkingTracerRetry`（有限重试+退避+ctx 取消即退）；6 调用点接入。TDD 3 用例 RED→GREEN；shared 模块全绿。
- **#18（F-210/M2）PASS（口径落定）**：xtts 移出 readiness（`/health` 仍可观测）；**运行验证**：`/health` deps 含 xtts、`/health/ready` 不含。**storage 只观测项未实现**（F-186 保留 🟡）。
- **T-1（F-215/§8）已修**：判据改为「镜像构建时间 ≥ 该服务源码最近 commit」+ 读**镜像**；同源修 `check_image_freshness.sh`。
- **#6（§契约 5）PASS**：按权威 DDL 清点「`VARCHAR(32) NOT NULL` 枚举列」= 2 列（`messages.intent` / `fused_emotions.primary_emotion`），各加 testcontainers 真库往返 integration test；顺带修 `model.Message.Intent` tag `size:16`→`32`（DDL↔model 漂移）+ 新登 **E2E-F-217**（`user_oauth` 残留表）。
- **#7（§契约 6）PASS**：以 `KAFKA_ENABLED=false` 重建 chat-svc 走 `DevEventPublisher` 直写 → smoke §6 `OK: 近 10 分钟内 15 行`。**顺带修**：compose chat-svc `KAFKA_ENABLED` 硬编码 `"true"` → `${KAFKA_ENABLED:-true}`（§6 此前不可触发）；smoke §6 判据 `event_id LIKE 'smoke-%'` 恒 FAIL（event_id 全 UUID）→ 改时间窗（**新登 E2E-F-218**）。
- **#19（F-195/F-197）PASS（判据）**：定向 + `--repeat-each=4`×2 project **8/8 PASS**（gap 365~682ms）⇒ 未复现；**判据 = 现有 #12/mobile 用例不覆盖缺陷时序路径**（跨路由实例 unmount-stop / `fill` 后事件分发）⇒ 需新写 spec 才能作门禁。
- **#20（F-189 全量基线）PASS（分类）**：全量实跑 **66 failed / 252 passed / 2 skipped（37.6m）**，**E2E-30 域内 0 条**；**26 条同源于陈旧探针直连 `:8894`（新登 E2E-F-219）**、2 条环境（Grafana）、4 条夹具、~26 条负载/时序、4 条 E2E-15 数据断言。
- 汇总：**PASS 19 / FAIL 0 / BLOCKED 1 / N/A 0**（BLOCKED 5%）。
- **IAB 实测（2026-10-12）**：#14 触及语音链路 ⇒ 真实浏览器实测 —— 登录 → 会话详情 → 语音气泡经网关反代**加载并播完**（`readyState=4`/`duration=1.74s`）；**浏览器 origin 下 HEAD/GET 均 200**（CORS 正常）；我的空间页正常渲染。**未覆盖**：真实麦克风录音→上传（IAB 无麦克风权限）。见 [report.md](report.md) §9.4。

### 1.3 裁定（**用户 2026-10-12 已逐条追认**）

T-1/§8=**保持致命**；M1=严格非空（证据关闭）；M2=**storage 不翻转即关闭 F-186**；M4=**转专项**（计划文档 `docs/plans/outbox-trace-continuity.md`）；M5=**转运维 runbook**（F-192/F-196 已写入 RUNBOOK §2.4）；M6=**暂不接 CI**。详见 [plan.md](plan.md) §4.1 与 [report.md](report.md) §9。

### 1.4 账本

F-180 / F-187 / F-188 / F-189（本轮分类完成）/ F-204 / F-209 / F-210 / F-211 / F-215 / F-216 / F-218 + **F-186**（用户裁定关闭）/ **F-192 / F-196**（转运维）→ ✅；**新登 E2E-F-217**（user_oauth 残留表，🔴 只登记）、**E2E-F-219**（3 个回归钉直连已收掉的 `:8894` ⇒ 全量 26 条失败同源，🔴 只登记）。F-183（#16）按 M4 转专项；F-184 / F-191 / F-195 / F-197 / F-212 保留（见 report §4/§7）。

---

## 2. 未做 ❌

| # | 未做项 | 归属 / 说明 |
|---|--------|-----------|
| 1 | ~~**#6（§契约 5）**~~ | ✅ **已做（2026-10-12）**：2 列 integration test + intent tag 漂移修复 |
| 2 | ~~**#7（§契约 6）**~~ | ✅ **已做（2026-10-12）**：`KAFKA_ENABLED=false` 真跑 + 两处修复 |
| 3 | ~~**#19 / #20（组 E）**~~ | ✅ **已做（2026-10-12）**：#19 判据（§9.7）；#20 全量分类（§9.8）；F-192/F-196 已转运维（RUNBOOK §2.4） |
| 4 | ~~**F-186 storage 可观测**~~ | ✅ 用户裁定**不做**（M2 追认「不翻转」即关闭 F-186） |
| 5 | **T-6 收口** | report §8 收口自检勾选、plan/roadmap 状态三处一致（partial→done）、§13.3 第二方核对、§2.5 收口自检 |

> **已知属性（非「未做项」，不计入未做）**：§8 为「致命」判据 ⇒ **任何触及服务源码的提交/合并后 §8 立即 STALE**（需重建镜像才全绿）；`emotion-echo-web` 前端镜像另有**预存在**的 `npm install` 失败（非本阶段引入），按 RUNBOOK §2.1b 停该容器后 §8 会 SKIP 它。

---

## 3. 环境现状

- 栈：infra + 6 应用服务；本轮修 F-216 后 **`docker compose up` 可正常完成**（此前冷启动必 EXIT=1）。
- 镜像：`web-bff:v0.1.37` / `analytics-svc:v0.1.11` / `chat-svc:v0.1.16` / `user-svc:v0.1.5` / `ai-svc:v0.1.10` / `assessment-svc:v0.1.5` 已重建；`emotion-echo-web` 未能重建（见 §2 #6）。
- **已知坑**：并发 `docker compose build` 多个 Go 服务会因内存峰值使 daemon 报 `Failed to receive status: rpc error ... EOF` ⇒ **逐个构建**。
- `deploy/.devmode-session` 未占用（本会话未登记；如需长跑请按双轨协议登记）。

---

## 4. 恢复时的下一步

1. 收口：report §8 收口自检勾选 + plan/roadmap 状态三处一致（partial→done）+ §13.3 第二方核对通过 + §2.5 三连。（smoke 结论已回填 report §9；§8 的「提交后即 STALE」属已裁定语义。）
2. 推进 #5（integration test）/ #6/#7（`KAFKA_ENABLED=false` 栈）/ #19 / #20。
3. 收口：report §4~§6 补全 + plan/roadmap 状态三处一致 + 账本全量对账 + `audit --all` 0 FAIL + §13.3 第二方核对 + §2.5 三连。
