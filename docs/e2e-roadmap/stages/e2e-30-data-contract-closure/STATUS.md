# E2E-30 数据契约收口 — 会话状态（STATUS）

> 用途：AGENTS.md §八「收工三查」第 1 条的**本轨状态记录**。**已做/未做分列，禁止美化。**
> 快照时间：**2026-10-09**（执行轮，**阶段未收口**）。任务书：[plan.md](plan.md)（含 §0.1c 本轮实测回填）· 执行记录：[report.md](report.md)（进行中，§0 未完成清单是唯一真相源）。

---

## 1. 已做 ✅

### 1.1 交付（本轮 2 个 PR 合入 main，`main = 88c3548`）

| PR | 内容 | CI |
|---|---|---|
| [#190](https://github.com/Exist-a/emotion-echo/pull/190) | **第一批**：**L1** §2.4 smoke 可跑化（基址 → APISIX 网关 `:19080` + Bearer + 登录前置；新守卫 `check_smoke_gateway.sh`）+ **L6** helm 主 chart 渲染回归（`test_helm_main_render.sh`，三不变量 + 负向对照；接 CI 新 job `helm-main-render`）+ **#17** 审计器计划解析器标题容错（`test_audit_plan_parser.sh`）+ AGENTS §2.4 文案纠偏（含双轨协议 §六 握手行） | 36/36 |
| [#191](https://github.com/Exist-a/emotion-echo/pull/191) | **第二批**：**#13** digest 守卫扩扫 compose `image:` 行 + 钉 4 个浮动 `:latest`（E2E-F-188） | 36/36 |

### 1.2 §0.2 开工复核 7 项（全部实跑）

docker 可用（19 容器全 healthy）+ 锁无占用（本轮登记 `lane-e`）· smoke 原样跑 → **rc=2 死在连接被拒**（F-209 运行时确认）· 环境基线（`db-migrate` ExitCode 0、Nacos `emotion-echo-dev` count=6、网关登录 200）· helm 3039 行 / lint 0 failed / 子 chart 23 · 8894 → `000` · 账本编号（**任务书写"从 F-210 起"已过期**，实际从 F-215 起）。

### 1.3 验证结论（13 个测试点有结论）

- **PASS 13**：#1（smoke 可跑性）· #2~#5（§契约 1~4 真跑全绿：行数 457 / event_type **7 种** / analytics_reader 四视图可读 / reports summary 非空+分布非空）· #8~#10（helm 渲染/lint/守卫）· #11（视图一致性 4/4）· #12（迁移契约 + 服务顺序独立性 80 pass）· #13（digest 扩扫）· #17（审计器 A3）
- **BLOCKED 7**（按 AP-03，未做一律 BLOCKED，非 N/A）：#6/#7（需 integration test / `KAFKA_ENABLED=false` 栈形态）· #14/#15/#16/#18（组 D 剩余，其中 #16/#18 是 [M]）· #19/#20（组 E，含 [M] M5）
- **§8 镜像新鲜度 8 项 STALE**：新账 **E2E-F-215** —— 判据粗（拿全局最新 commit 比、且读**容器** `.Created` 却写成"镜像构建于"），在非产物提交上恒红。**本轮未擅自改断言**
- 门禁：`e2e_stage_audit.py --all` **31 阶段 0 FAIL**；新守卫 **25/25 + 26/26** 接 CI；总闸 `needs` 漏项被 `check_doc_drift_gate_needs.sh` 抓到并修

### 1.4 账本

- **E2E-F-180 → ✅**（A3 静默失效；**根因更正**：不是"子表编号"而是**章节标题**）
- **E2E-F-188 → ✅**（digest 守卫扩扫 compose + 钉 4 个引用）
- **E2E-F-209 → ✅**（smoke 不可跑；已改走网关并真跑）
- **新登 E2E-F-215**（§8 判据粗，待裁定去向）；编号连续至 215
- 状态：`E2E-30 → 🟡 partial`（plan + roadmap + report 三处一致）

---

## 2. 未做 ❌

| # | 未做项 | 归属 / 说明 |
|---|--------|-----------|
| 1 | **§8 口径未定**（T-1 / E2E-F-215） | **需用户裁定**：① 重建镜像+recreate ② §8 改口径 ③ 只在 report 分类。**不定则 smoke 退出码恒 1、M6 无法定** |
| 2 | **组 D 剩余 4 项**：#14（F-187 voice HEAD + PutObject 超时，L3）/#15（F-204 tracer 重试，L4）/#16（F-183 Kafka trace 断链，L5，**需 ADR + [M] M4**）/#18（F-186 readiness 含 storage，**[M] M2**） | E2E-30；#14/#15 需重建镜像验证（与 T-1 耦合） |
| 3 | **组 E 2 项**：#19（F-195/F-197 时序 flaky 根因或判据）/#20（F-189/F-192/F-196 基线与环境面，含 **[M] M5**） | E2E-30 |
| 4 | **#6/#7 证据形态**：§契约 5 需 integration test；§契约 6 需 `KAFKA_ENABLED=false` 栈 | E2E-30 |
| 5 | **[M] M1 / M6 未裁定** | **需用户裁定** |
| 6 | **收口未做**：report 补全（§3~§9）· 账本 13 条转挂逐条对账 · §13.3 第二方核对 · §2.5 收口自检 | E2E-30 |
| 7 | **BLOCKED 7/20（35%）> 收口门槛 1/3** | 中间态；收口前须逐条落定（已在 report §2 显式记录，不掩盖） |

---

## 3. 环境现状

- dev 栈 **19 容器全 healthy**（`xtts`/`sensevoice`/`llm-service` 由 `ai` profile 起，非本会话所起）；`:3000` 由 web 容器服务；网关 `:19080`。
- 被验镜像：`emotion-echo/web-bff:v0.1.37` + `emotion-echo/assessment-svc:v0.1.5` —— **本轮未重建**（本阶段至今只改 smoke/守卫/compose pin/文档）。
- **`deploy/.devmode-session` 已释放**（收工三查第 2 条）。
- 注：`docker-compose.infra.yml` / `apps.yml` 的 4 个第三方镜像已改钉 digest（本机镜像即该 digest，**不影响当前运行栈**）。

---

## 4. 恢复时的下一步

1. **先请用户裁定两条**：① §8 口径（E2E-F-215，三选一）② [M] M1/M2/M4/M5/M6。
2. 若 §8 选"重建"：一次性重建 8 个服务镜像 + `up -d` recreate + 重跑 `apisix-seed` + 重启 `web-bff`（运维铁律），再跑 smoke 取"§2.4 六契约全绿（exit 0）"。
3. 推进 **L3/L4**（#14/#15，需重建后验证）、**L5**（#16，须先有 M4 裁定 + ADR）、#18（M2 裁定后）、#19/#20。
4. 收口：report 补全 + 账本对账（13 条转挂 + 本轮新登）+ `audit --all` 0 FAIL + §13.3 第二方核对 + §2.5 三连。
