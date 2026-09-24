---
stage: e2e-18
title: 缓存层（ai-svc LRU 行为 + Redis 去留决策）
executed: 2026-09-24
status: partial
environment: dev 模式（应用容器 healthy，infra+apps+dev compose + .env.local；ai-svc v0.1.8 含 LRU 修复）
---

# E2E-18 执行记录

## 1. 环境基线

- 启动命令：`cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml --env-file .env.local --profile dev up -d`（栈开工前已在跑；本阶段增量 = `build emotion-echo-ai-svc` → `up -d emotion-echo-ai-svc`）
- **被验镜像时间戳**（E2E-F-70/99 铁律）：`emotion-echo/ai-svc:v0.1.8` Created `2026-09-24T02:16:28Z`（= 本地 10:16:28）**晚于** GREEN commit `56ae8a7`（10:14:49）✓
- 容器状态：ai-svc healthy（`docker start` 绕 depends_on，见 §3 F-141）；redis healthy；Nacos 注册 `count:6`；apisix-seed 重跑成功（6 upstreams + 15 routes）；web-bff 重启后网关 `/auth/captcha` 401（鉴权路由正常）
- 声明的配置差异：dev 模式（`BFF_DEV_RETURN_CODE=1`、`BFF_TRUST_APISIX=false`、CORS localhost 双 host）——本阶段验证的是 dev 配置
- ⚠️ `emotion-echo-db-migrate` 保持 `Exited(1)`（F-141 checksum 漂移，归 E2E-19）——本阶段用 `docker start` 绕过，**每次 `compose up` 复现**

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | LRU 单元测试全绿（7 用例） | [A] | PASS | `go test ./internal/fusion -run TestMsgIDLRU -v` → `PASS ok ... 0.809s`，7 个 `--- PASS`（NewNonNil/MissFirst/HitAfterAdd/TTLExpired/Eviction/Recency/ConcurrentSafe） | |
| 2 | worker LRU 接线测试全绿（3 用例） | [A] | PASS | `go test -run TestWorker_Tick_LRU -v` → 3 `--- PASS`；日志含 `msgID=700 skipped (LRU hit)` + `LRUNilNoEffect` | |
| 3 | LRU 默认值契约 RED→GREEN（三态） | [A] | PASS | RED commit `5a82280`（`undefined: lruCapacityFromEnv` 构建失败）→ GREEN commit `56ae8a7` → `TestLRUCapacityFromEnv_*` 3/3 PASS（未设→1024 / 512→512 / 0→0） | 先红后绿两段 commit 可查 |
| 4 | 修后运行时启用日志 | [A] | PASS | `docker logs emotion-echo-ai-svc`（10:38:35）`Worker LRU rate limit enabled: cap=1024 ttl=4m0s`；镜像 v0.1.8 Created 10:16:28 晚于 GREEN 10:14:49；修前基线：同日 10:16 前日志**无**此行 | |
| 5 | `/metrics` worker_tick collector 注册 | [A] | PASS | `wget :8891/metrics` → `# HELP emotion_echo_fusion_worker_tick_total` + `emotion_echo_fusion_worker_tick_total{outcome="ok"} 10`；`skipped_lru` 取值路径 = `worker.go:124` + #2 单测断言（Prometheus 动态 label 不预列出，按 plan 注册级口径） | 注册级；运行时触发依赖 DB pending 竞态不伪造 |
| 6 | Redis 现状盘点三件套 | [A] | PASS | ① `docker ps` redis `Up (healthy)`；② `redis-cli info clients` → `connected_clients:1`（即 redis-cli 自身，业务连接 0）；③ 6 svc+BFF `grep -rn "redis\.NewClient\|InitRedis("` 排除 _test → **零命中** | |
| 7 | 注释漂移证据固化（0 caller） | [A] | PASS | `InitRedis(` 全仓仅 `init.go:40` 注释示例、无调用；`LimiterBackend` callers=0；原文回读 `limiter.go:137-140`（`RedisLimiterBackend: TODO`）+ `init.go:37-41`（「5 svc main.go 调一次」） | |
| 8 | [M] Redis 去留决策升级用户 | [M] | PASS | 用户 2026-09-24 拍板：**「redis 正常需要接入业务，所以要保留。比如可以和记忆系统进行操作，还有 token 的获取」** → D-27 已登记 `decisions.md` | 备选=下线（被否）/立即接入（超边界） |
| 9 | 决策登记与账本对账 | [A] | PASS | `decisions.md:292` D-27 行（回读 ✓）；账本 F-08 → ✅已解决（决策层）+ 实施转 E2E-20 注明（回读 `discovered-unresolved.md:36` ✓）；F-134/135/136 owner → **E2E-28** 行内注明转入（:218-220 ✓）；`e2e_stage_audit.py --all` → 30 阶段 0 FAIL | |
| 10 | 回归钉双 project 全绿 | [A] | PASS | `npx playwright test e2e/cache-layer-smoke.spec.ts` → **4 passed**（chromium 2 + mobile 2）；复跑第二次仍 4 passed | 用例：#10a 主链路 + #10b 无 429/5xx |
| 11 | 主链路视觉证据 | [V] | PASS | `screenshots/11-chat-smoke-after-lru-chromium.png`（65KB）+ `-mobile.png`（131KB），**两张均已查看**：chromium AI 回复气泡「我在这里，陪你慢慢说…」正常；mobile 回复「我在呢，随便说一句也可以…」存在 | mobile 侧栏默认展开遮挡系既有行为（此前 mobile spec 均在），非本阶段回归 |
| 12 | 全量回归 | [A] | PASS | ai-svc `go test ./...` → exit=0、14 包 ok、0 FAIL；shared → exit=0、21 包 ok；前端 `vitest run` → **526 passed / 65 files**；本 spec 复跑 4 passed | |

汇总：`PASS 12 / FAIL 0 / BLOCKED 0 / N/A 0`

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| LRU 默认值与注释/决策 15 三对一矛盾（所有部署从未启用） | 范围内 | TDD 修复 commit `5a82280`（RED）+ `56ae8a7`（GREEN） |
| **F-141**：db-migrate FATAL「i002 checksum 不一致」卡 `compose up` 依赖链（DB 记录 f5e05bb 来自 09-18 脏工作区、该内容从未入库；今 08:19 被恢复成 HEAD 版 dad2992a） | **范围外**（迁移幂等 = E2E-19 scope） | 只记账不修：**E2E-F-141**（commit `4688ba2`）；规避 = `docker start` 绕 depends_on |
| **共享工作目录并发 git 操作**：reflog 显示 09:41 pull / 10:26 checkout+reset 非本会话发起（Lane O 合 #84/#85 期间在同目录操作，曾把 HEAD 从本轨 fix 分支切走） | 范围外（并行协作层） | 记入 STATUS.md；本轨无未提交损失（切走时 3 commits 已提交）；建议后续会话评估 worktree 物理隔离 |
| `du.exe.stackdump` 崩溃残留 | 环境噪音 | 已删（1KB 无价值，防残留扫描误报） |

## 4. 修复清单（TDD 记录）

| commit | 内容 | 先行的失败测试 |
|--------|------|---------------|
| `5a82280` | test: RED — LRU capacity env 默认值契约（三态） | 本行即 RED（`undefined: lruCapacityFromEnv` 构建失败） |
| `56ae8a7` | feat(ai-svc): GREEN — `lruCapacityFromEnv()` 默认 1024、显式正值覆盖、显式 0 关闭；`main.go` 调用点收敛 | `main_lru_env_test.go` TestLRUCapacityFromEnv_* 3 条先红后绿 |
| `4688ba2` | docs: F-141 记账（迁移 checksum 漂移 → E2E-19） | —（记账非代码） |

## 5. 回归钉

- 新增 spec：`emotion-echo-web/e2e/cache-layer-smoke.spec.ts`（2 用例 × 2 project = 4 条），首次运行 **4 passed**（2.6m），收口前复跑 **4 passed**（2.3m）
- 断言边界（诚实声明）：LRU 属 fusion worker 后台路径，浏览器不可直接观测——启用证据在启动日志（#4）与 /metrics（#5）；本 spec 断言「缓存层改动后主链路不回归 + 限流不误伤正常流量」这一集成面

## 6. 待决策 / 升级项

- **无未决 [M]**：#8 已获用户拍板（D-27）。
- 关联开放项（非本阶段阻塞）：会话记忆 D-a~D-d 见 `docs/plans/conversation-memory-pending-decision-2026-09-24.md`（用户待拍板；D-c 已与 D-27 联动——Redis 为记忆存储候选）。

## 7. 收口自检

- [x] git status 干净（报告提交后）
- [x] main 与 origin 无 ahead/behind（push 后核对）
- [x] 无残留已合并分支（合并后即删）
- [x] 账本对账：owner=E2E-18 的未了结条目 = 0（F-08 已翻状态；F-134/135/136 已转挂 E2E-28）
- [x] `e2e_stage_audit.py --all` 0 FAIL（记账/决策改动后复跑）
- [x] 复读关键断言：D-27 行 / F-08 行 / F-134~136 行 / 汇总行均当场回读（证据列给出处）
- [ ] **第二方核对（§13.3）**——执行者不得自行宣布 done，待非执行者逐条核对后方可将 roadmap `partial → done`

> **阶段状态判 `partial` 而非 `done` 的唯一原因 = §13.3 第二方核对未执行**（RUNBOOK §7#10）。12/12 测试点全 PASS、0 BLOCKED、账本对账完成，无技术欠账。
