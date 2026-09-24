---
stage: e2e-18
title: 缓存层（ai-svc LRU 行为 + Redis 去留决策）
type: verification
status: in-progress
created: 2026-09-24
depends-on: [e2e-17]
blocks: [e2e-20]
gate: []
related-findings: [E2E-F-08, E2E-F-134, E2E-F-135, E2E-F-136]
---

# E2E-18 缓存层 — 详档

> **类型**：verification —— 两件"不确定"要变"已验证/已决策"：① ai-svc LRU（同 msgID 限流）文档三处说生效、**实际所有部署默认关闭**（本轮计划期实测坐实）；② Redis 是否启用/下线**无任何决策记录**，容器空转。
> **依据**：roadmap 第五批 E2E-18 行 + 账本 E2E-F-08（预探查）+ 架构决策 15（`adr-2026-09-llm-fusion-hardening.md`，标"已生效"）+ **本轮计划期调研校准（2026-09-24，见 §7）**。
> **方法论**：[RUNBOOK.md](../RUNBOOK.md)（状态机 / §4.1 证据有效性 / §7 收口契约 11 项 / §13 审计）+ [anti-patterns.md](../anti-patterns.md)。

---

## 1. 阶段目标

让缓存层从两处"说不清"变成可机械证明：

1. **LRU 行为**：同会话同 messageID 在 TTL 内不会重复触发 LLM fusion（省配额），且该机制**运行时真的开着**（当前：文档说 cap=1024 默认生效，代码 `readEnvInt` fallback=0 ⇒ 不构造 ⇒ 每个部署里机制全死）。
2. **Redis 去留**：从"没人知道为什么在跑"变成 **D-27 决策落定**（保留闲置 / 下线）+ 账本 E2E-F-08 关账 + 决策记录可追溯。

---

## 2. 范围与边界

### 做

**A. LRU 默认值契约修复（范围内 bug，TDD 强制）**

计划期证据（2026-09-24 实测，逐条可复现）：

| 证据 | 位置 | 内容 |
|------|------|------|
| 代码 | `emotion-echo-ai-svc/main.go:498` | `readEnvInt("WORKER_LRU_CAPACITY", 0)` → 未设 env 时 cap=0 → `if cap > 0` 不成立 → **rateLimit=nil，LRU 从不构造** |
| 注释 | `emotion-echo-ai-svc/main.go:496` | 「msgID LRU 限流（**默认 cap=1024** / TTL=4min）」——与代码相反 |
| 决策 | `docs/architecture/decisions.md` 决策 15 | 「同 msgID 限流 = LRU(cap=1024) + TTL=4min」，状态 ✅ **已生效**（ADR `llm-fusion-hardening`） |
| 部署 | `deploy/` 全树 grep `WORKER_LRU` | **零命中**（没有任何 compose/env 设置该变量） |
| 运行时 | `docker logs emotion-echo-ai-svc`（2026-09-24） | `FusionWorker started (tick=5s)` 存在，但**无** `Worker LRU rate limit enabled` 行 ⇒ 实证未启用 |

- 修法（RUNBOOK §8 允许范围内择优）：**默认启用** —— 抽 helper `lruCapacityFromEnv()`（fallback 0→**1024**），`main.go` 改用之；显式 `WORKER_LRU_CAPACITY=0` 仍可关闭（保留逃生门）；TTL helper 同构（fallback 240 不变）。理由：注释 + 决策 15 + ADR 三处文档一致指向"默认生效"，代码是三对一的漂移方；修代码 = 向已决议方向收敛，不改变已决议方向（无需升级）。
- RED：抽 helper 前先写失败测试（默认 1024 / 显式覆盖 / `0` 关闭三态）→ GREEN：实现 helper + `main.go` 接线 → REFACTOR：`main.go:496-503` 内联逻辑收敛到 helper。

**B. LRU 运行时验证（先测后证）**

- 修后 rebuild `emotion-echo-ai-svc`（独立 tag，**不覆盖** web:v0.1.5 / bff:v0.1.28，协议 §三.资源1）→ 启动日志 grep `Worker LRU rate limit enabled: cap=1024`（补抓"修前无此行"基线已在 §2.A）。
- ⚠️ **运维铁律（E2E-F-107 教训）**：重建任何 svc 后必须重跑 `apisix-seed` + 重启 `web-bff`（BFF gRPC 连接在 Nacos 取一次就缓存）。
- `/metrics` 断言 `worker_tick{result="skipped_lru"}` 指标**已注册**（注册级验证；运行时触发 skip 依赖 DB pending 行竞态，由单测覆盖，不伪造运行态）。

**C. Redis 现状盘点 + 去留决策（本阶段的"决策"半场）**

- 盘点三件套（全为只读）：① 容器 healthy + `redis-cli client list` 仅健康检查级连接（计划期基线：`connected_clients:1` 即探测自身）；② 活跃 6 svc + BFF 非测试代码 `redis.NewClient` / `InitRedis` 引用 = **0**（`go-redis` 仅 shared 直连、其余 5 svc 全 `// indirect`）；③ `limiter.go:137` `RedisLimiterBackend: TODO` 与 `skywalking/init.go:37`「5 svc main.go 调一次」注释的 **0 caller** 核实（注释漂移证据）。
- **[M] 去留决策升级用户**（建议：**D-27 = 保留闲置**，为 E2E-20 的 `RedisLimiterBackend` 现成铺路——roadmap E2E-20 明言「Redis 容器现成」；下线则 E2E-20 要重拉容器，纯负收益）。拍板后：
  - 登记 `docs/e2e-roadmap/decisions.md` **D-27**（Lane E 从 D-27 起，协议 §三.资源3）；
  - 若 `check_adr_gate.sh` 因「存储」类关键词命中 → 补 ADR `docs/architecture/adr/adr-2026-09-e2e-redis-retention.md`（Lane E 前缀 `adr-2026-09-e2e-*`）+ `docs/architecture/decisions.md` **决策 34**（Lane E 从 34 起；33 号被 Lane O 占用）；
  - **账本 F-08 关账**：决策落定即关（"用不用"解决）；其携带的「Redis 后端实现 TODO」实施部分本就归 E2E-20（与 F-25 同源），关账行内注明转移动作。

**D. 账本对账（收口契约 §7.9 的前置）**

- **F-134 / F-135 / F-136 归属修正**：三行 owner 现为「E2E-18 或后续」，但内容全是 TTS 切段/双端点/多 worker——**与缓存层无关**，是 E2E-17 收口时的临时停放。若不转出，A5 会因「属本阶段未解决条目存在」拦截 done。动作：owner 改挂 **E2E-28**（TTS/性能复测批次，roadmap 载明 E2E-28 依赖 E2E-17「TTS 改造后复测」），行内注明「2026-09-24 自 E2E-18 转入：TTS 非缓存范围」。账本只增不删，属状态/归属更正。

**E. 测试基建**

- Go 契约测试：`lruCapacityFromEnv` 三态表驱动（RED 先行）。
- Playwright 回归钉：`emotion-echo-web/e2e/cache-layer-smoke.spec.ts` —— 登录 → 发消息 → 收到 AI 回复（钉住「LRU 默认启用 + ai-svc rebuild 后主链路不回归」），双 project 首跑绿。
- 视觉证据：聊天主链路截图 1 张（修后状态）。

### 不做（边界）

| 不做项 | 理由 |
|--------|------|
| `RedisLimiterBackend` 实现 / 登录锁定 / 验证码 Redis 化 | **E2E-20 专属**（roadmap 修复路径）；本阶段只拍"用不用"，不写后端 |
| LRU 算法/容量调优、LRU 跨实例一致性 | 单实例语义已由决策 15 定案；跨实例归 E2E-20 |
| F-134/135/136 的 TTS 实施（切段/双端点/多 worker） | 归属转出 §2.D，实施归 E2E-28/后续 |
| go-redis 依赖升级、`InitRedis` 补真 caller、SkyWalking redis hook 接线 | 触发条件 = Redis 真接入（multi-round plan §十六.5 已列"触发条件型 backlog"）；D-27=保留闲置则继续挂触发条件 |
| 会话记忆 / 对话历史注入 | 另一议题，已单独落 `docs/plans/conversation-memory-pending-decision-2026-09-24.md`（PR #82），不混入本阶段 |
| 前端存储（localStorage/Dexie）验证 | 归 E2E-11/端侧轨道，非本阶段"缓存层"定义（ai-svc LRU + Redis） |
| i18n | D-04 候选未决 |

---

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| E2E-17 done（前序阶段） | ✅ 2026-09-24 PR #77 收口（用户签字） |
| 不在决策门阻塞列表（RUNBOOK §9） | ✅ §9 仅 D-04 且不阻塞任何阶段 |
| `e2e_stage_audit.py --all` 0 FAIL | ✅ 2026-09-24 实测（PR #81 合并后：30 阶段 0 FAIL） |
| `plan.md` 存在且前置表满足 | ✅ 本文件（合并即满足开工前置 #3） |
| `deploy/.env.local` 存在 | ✅（**严禁删除/覆盖**，AGENTS §四红线；本阶段不读不改其内容） |
| dev 栈健康 | ✅ 2026-09-24 实测：应用容器 Up healthy + Nacos `count:6` 注册齐全 + redis healthy |
| dev mode 锁 | 开工时写 `deploy/.devmode-session`（owner: lane-e），收工删除（协议 §五） |

环境启动命令（**必须带 `--env-file .env.local` 与 `--profile dev`**，RUNBOOK §2.1）：

```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local --profile dev up -d
# LRU 修复后（§2.B）：
docker compose -f docker-compose.apps.yml --env-file .env.local build emotion-echo-ai-svc
docker compose -f docker-compose.apps.yml --env-file .env.local up -d emotion-echo-ai-svc
bash apisix/seed.sh && docker compose -f docker-compose.apps.yml --env-file .env.local restart emotion-echo-web-bff
```

---

## 4. 测试点清单

判定标记：`[A]` 自动 · `[V]` 视觉 · `[M]` 需裁定。详见 [RUNBOOK.md](../RUNBOOK.md) §4。

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 1 | LRU 单元测试全绿（`lru_test.go` 7 用例：New/miss/hit/TTL 过期/驱逐/recency/并发） | [A] | `go test ./internal/fusion -run TestMsgIDLRU -v` | 测试输出 | ⬜ |
| 2 | worker LRU 接线测试全绿（Touch 命中 skip / RateLimit=nil 不限流） | [A] | `go test ./internal/fusion -run TestWorker -v` | 测试输出 | ⬜ |
| 3 | **LRU 默认值契约 RED→GREEN**：helper 三态（未设 env→启用 cap=1024 / 显式 `512`→512 / 显式 `0`→关闭）；先提交失败测试再实现 | [A] | 先红后绿两次 `go test` 输出 + commit 序列 | 测试输出 + commits | ⬜ |
| 4 | 修后运行时启用证据：rebuild 后 ai-svc 启动日志含 `Worker LRU rate limit enabled: cap=1024 ttl=4m0s`（修前基线：无此行，见 §2.A 运行时行） | [A] | `docker logs emotion-echo-ai-svc \| grep LRU` | 日志片段 + 镜像 `Created` 时间戳晚于修复 commit | ⬜ |
| 5 | `/metrics` 中 `worker_tick` collector 的 `skipped_lru` result 已注册（注册级；不伪造运行时触发） | [A] | `docker exec emotion-echo-ai-svc` 抓 metrics 或 `curl :8891/metrics \| grep skipped_lru` | curl/grep 输出 | ⬜ |
| 6 | Redis 现状盘点：容器 healthy + `client list` 无业务连接（仅 healthcheck/探测自身）+ 活跃 6 svc+BFF 非测试代码 redis 运行时引用 = 0 | [A] | `docker ps` + `redis-cli client list` + `grep -rn "redis.NewClient\|InitRedis(" --include="*.go"`（排除 legacy/_test/shared 定义处） | 三段命令输出 | ⬜ |
| 7 | 注释漂移证据固化：`limiter.go:137 RedisLimiterBackend: TODO` 与 `init.go:37`「5 svc 调一次」caller 数 = 0（grep 输出） | [A] | 同上 grep + 行号回读 | grep 输出 + `文件:行号` | ⬜ |
| 8 | **[M] Redis 去留决策**：升级用户拍板（建议 D-27=保留闲置供 E2E-20；选项：保留 / 下线 / 立即接入） | [M] | 用户答复记录 | 决议 + decisions.md 行 | ⬜ |
| 9 | 决策落定后登记与对账：D-27 入 `e2e decisions.md`（+ 如门禁要求：ADR + 架构决策 34）+ 账本 F-08 翻状态 + F-134/135/136 转挂 E2E-28（行内注明转移） | [A] | 回读 decisions/ADR/账本 行号 + `e2e_stage_audit.py --stage e2e-18` | 文件行号 + audit 输出 | ⬜ |
| 10 | 回归钉：`e2e/cache-layer-smoke.spec.ts` 首跑绿（chromium + mobile 双 project） | [A] | `pnpm playwright test e2e/cache-layer-smoke.spec.ts` | playwright 输出 | ⬜ |
| 11 | 主链路视觉证据：LRU 启用 + ai-svc rebuild 后聊天页发消息收到 AI 回复，截图正常（无布局/功能回归） | [V] | IAB 操作 + 截图并查看 | `screenshots/11-chat-smoke-after-lru.png` | ⬜ |
| 12 | 全量回归：ai-svc+shared `go test ./...` + 前端 `pnpm vitest run` + 本 spec 复跑，全绿 | [A] | 三段命令输出 | 测试输出 | ⬜ |

汇总行（收口时填）：`PASS x / FAIL x / BLOCKED x / N/A x`

---

## 5. 验收标准（DoD）

- [ ] 全部 12 个测试点有结论（发现问题已分类：范围内修复 / 范围外记账）
- [ ] LRU 修复走完 TDD（RED 先提交 → GREEN → REFACTOR，commit 序列可查）
- [ ] #8 [M] 已获用户答复；D-27（+ 触发时 ADR/决策 34）已登记
- [ ] 账本对账：F-08 已翻状态；F-134/135/136 已转挂（否则按 §7.9 只能 partial）
- [ ] 回归钉 `cache-layer-smoke.spec.ts` 存在且首跑绿
- [ ] `e2e_stage_audit.py --stage e2e-18` 0 FAIL（收口时 `--all` 仍 0 FAIL）
- [ ] report.md 按 §10 模板 + roadmap 状态更新 + §2.5 自检三连
- [ ] 第二方核对（§13.3）——执行者不得自行宣布 done

---

## 6. 已知风险

| 风险 | 应对 |
|------|------|
| **默认启用 LRU 改变运行时行为**（fusion skip 路径从死变活） | 单测 7+2 条钉语义；#10/#11 主链路 smoke；显式 `WORKER_LRU_CAPACITY=0` 保留逃生门 |
| **rebuild ai-svc 后 BFF gRPC 连接缓存失效** → 聊天 503（E2E-F-107 同型） | 铁律：rebuild 后必跑 `apisix-seed` + restart `web-bff`（§3 命令块已内置） |
| **镜像时间戳**：验收对象跑旧代码（E2E-F-70/99） | `docker inspect emotion-echo-ai-svc --format '{{.Created}}'` 晚于修复 commit 才验，写入 report 环境基线 |
| **ADR 门禁**：commit 命中「存储」类关键词被 `check_adr_gate.sh` 拦 | 已有决策 15/ADR 可引用；D-27 如需新 ADR 用 Lane E 前缀（§2.C），开工先跑门禁本地复现（memory：audit 本地先跑秒级复现） |
| **#8 [M] 未答复** → F-08/D-27 挂起 | 12 点中仅 1 个 [M]（≤1/3 不拦收口），但 done 需 F-08 翻状态 ⇒ 未答复时阶段如实标 partial 并在 report「待决策」节列明 |
| **audit 对 in-progress 无 report 的行为** | ✅ 已实测（2026-09-24 开工步）：无 report 时走 A0 WARN 提前返回、A9 不触发、0 FAIL；但 `roadmap_state_kind` 不识别 in-progress（`scripts/e2e_stage_audit.py:189-204` 回落 pending，仅显示失真、方向保守无假 PASS）→ **不动审计器**（范围外），改用操作约束：**report.md 必须与 plan/roadmap 状态翻转（partial/done）同批提交**，避开 A9 三处 status 不一致窗口；ADR 门禁实测只扫非文档改动的文件路径（`check_adr_gate.sh` 排除 `docs/`+`.md`）→ 本阶段 docs commit 与 `main.go` 路径均不命中关键词 |
| F-134/135/136 不转出 → A5 拦 done | §2.D 列为收口硬前置，audit 复跑验证 |
| 账本 6 列硬契约（parse_ledger <6 cells 静默 skip） | 转挂 F-134/135/136 时保持 6 列完整（memory：E2E-17 教训） |

---

## 7. 产出物

- Go：`lru_capacity_env_test.go`（或同位新测试文件，RED 先行）+ `main.go` helper 改动
- Playwright：`emotion-echo-web/e2e/cache-layer-smoke.spec.ts`
- 截图：`stages/e2e-18-cache-layer/screenshots/11-chat-smoke-after-lru.png`
- 决策：`e2e decisions.md` D-27（+ 触发时 `adr-2026-09-e2e-redis-retention.md` + 架构决策 34）
- 账本：F-08 状态翻转；F-134/135/136 归属转挂
- 执行记录：`stages/e2e-18-cache-layer/report.md`

## 8. 调研依据（AGENTS.md §〇.6 — 计划期 2026-09-24）

- **已读实现（6）**：`emotion-echo-ai-svc/internal/fusion/lru.go`（全）、`worker.go`（LRU 接线段）、`main.go:440-515`（fusion 装配 + LRU gate）、`emotion-echo-shared/pkg/middleware/limiter.go`（全，LimiterBackend/TODO）、`skywalking/init.go`（InitRedis）、`ai_stream_handler.go`（会话记忆插单时读，确认与本阶段无关）。
- **已读测试（3）**：`fusion/lru_test.go`（7 用例清单）、`fusion/worker_test.go`（RateLimit 接线 3 处）、`ai-svc/main_apply_default_fallbacks_test.go`（package main 测试先例——helper 可测性依据）。
- **已查 ADR/决策（4）**：`decisions.md` 决策 15（LRU 已生效）/决策 16、`adr-2026-09-llm-fusion-hardening.md`（放弃 Redis 限流理由）、`e2e decisions.md`（D-25 已用/D-26 保留/D-27 起）、协议 §三.资源3（决策 N Lane O 占 33、Lane E 从 34）。
- **已查账本**：owner 含 `E2E-18` 的行 = F-08 + F-134/135/136（"或后续"停放）。
- **运行时实测（计划期基线）**：`docker logs ai-svc`（无 LRU enabled 行）、`redis-cli info clients`（connected_clients=1）、Nacos `count:6`、`WORKER_LRU` deploy 全树 grep 零命中。
- **git 溯源**：`git log -S WORKER_LRU_CAPACITY` → `bf476c1`（引入即 fallback=0，注释与代码同期矛盾，非后续漂移）。
- **smoke**：不适用（本阶段不触及 §2.4 六契约任一管道；LRU 属 fusion worker 后台路径，非事件发布/视图/报表链）。
- **外部信息**：不适用（无外部依赖版本涉入；redis:7-alpine / go-redis v9 版本不在本阶段变更）。
