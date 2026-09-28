---
stage: e2e-20
title: 多实例并发正确性（登录锁定 / 验证码防枚举 / 限流跨实例）
type: transformation
status: done
started: 2026-09-28
created: 2026-09-28
depends-on: [e2e-18]
blocks: []
gate: []
related-findings: [E2E-F-25, E2E-F-96]
---

# E2E-20 多实例并发正确性 — 详档

> **类型**：transformation —— 3 处 in-memory 单实例假设防护在多实例下**静默失效**，本阶段修为跨实例正确 + 双实例并发实测。
> **依据**：roadmap §第五批 E2E-20 行（"修 in-memory 限流/登录锁定/验证码防枚举的多实例失效 + APISIX limit-count 跨实例 + 双实例并发验证"）+ 账本 E2E-F-25 + **D-27 已决议（Redis 保留并接入业务，E2E-20 即其第一个接入点）**。
> **方法论**：[RUNBOOK.md](../RUNBOOK.md)（状态机 / §4 判定分级 / §6 TDD 修复纪律 / §7 收口 11 项 / §13 审计）+ [anti-patterns.md](../anti-patterns.md)。

---

## 1. 阶段目标

让"多实例部署"从**三处静默失效**变成**实测跨实例正确：

1. **登录失败锁定**（5 次错密码锁 15 分钟）—— 现状 BFF `loginFailures` map 是**单进程内存**，实例 A 锁的用户打实例 B **不锁**（锁可被绕过）
2. **验证码 60s 防枚举**—— 现状 `verificationCodes` map 单进程，实例 A 发过码，实例 B 再发**不拦**（防枚举失效）
3. **APISIX limit-count 限流**（60 req/min/IP）—— 现状 `policy: local` **每节点各自计数**，2 节点总配额放大到 120（决策 3 = 单机多实例，必须正确）

修法走 **D-27 决议**：`RedisLimiterBackend`（limiter.go:137 TODO 早已抽象好接口）+ BFF 两个 map Redis 化 + APISIX `policy: redis`。

---

## 2. 范围与边界

### 做

**A. 多实例失效实测（先测后证，RED）**

- 双 BFF 实例（compose 起第 2 个 web-bff 容器或 `docker compose up --scale`），实测三处失效各 1 次：
  - 锁定绕过：实例 A 打 5 次错密码 → 用户在 A 锁 → 请求发到 B → **B 仍接受登录尝试**（不锁）
  - 验证码绕过：实例 A 发码（60s 内）→ 实例 B 再发 → **B 再次生成**（不拦）
  - 限流放大：2 APISIX 或 BFF 实例下连续打，配额实测 > 单实例配额

**B. 修复（TDD 强制）**

| 修复项 | 位置 | 方案 |
|--------|------|------|
| `RedisLimiterBackend` 实现 | `shared/pkg/middleware/limiter.go:137` TODO | 实现 `LimiterBackend` 接口（D-27 已铺好抽象 + Redis 容器现成），TDD 表驱动：allow/deny/TTL/Redis 不可达降级 |
| BFF 登录锁定 Redis 化 | `auth_handler.go:68-93`（loginFailures map） | `loginMaxFailures/loginLockWindow` 计数与锁定写 Redis（key 带 TTL），in-memory 降级保留（Redis 挂时行为 = 现状单实例语义） |
| BFF 验证码防枚举 Redis 化 | `auth_handler.go:70`（verificationCodes map） | 60s 窗口 key `SET NX EX 60`；Redis 挂降级 in-memory |
| APISIX limit-count 跨节点 | `deploy/apisix/seed.sh:366-447`（`policy: local`） | `policy: redis` + redis server 配置（APISIX redis-limiter 插件，需 seed.sh 持久化 + F-139 seed↔admin 关系注明） |

**C. 双实例并发验证（GREEN 实测）**

- 修后同 3 处复测：跨实例锁定生效 / 验证码 60s 拦截 / 限流总配额 = 单实例配额（不放大）
- 并发正确性：双实例同用户并发 10 次错密码 → 总锁定态唯一、计数不漂移

**D. 账本对账**

- **E2E-F-25 关账**（多实例 3 处静默失效 → 本阶段修复 + 实测）
- E2E-F-96（user-svc 降级启动）—— **不修**（user-svc 代码改动属编排健壮性，非本阶段；若本阶段双实例实测复现则转记）

**E. 测试基建**

- Go 单测：`limiter_redis_test.go`（mock Redis / miniredis）+ BFF auth Redis 化契约测试
- Playwright 回归钉：`e2e/multi-instance-smoke.spec.ts`（登录锁定语义主链路）
- 视觉证据（[V]）：锁定提示页截图

### 不做（边界）

| 不做项 | 理由 |
|--------|------|
| 分布式事务 / 跨服务 saga | roadmap 明确边界 |
| Redis Cluster / 哨兵高可用 | 单机 docker redis 足够，HA 归运维阶段 |
| E2E-F-96 user-svc 降级自愈 | 归 E2E-06/E2E-23（编排 + 服务发现），非限流范畴 |
| JWT 轮换（E2E-F-28） | E2E-29 专属 |
| BFF 多副本生产部署编排 | dev 双实例仅验证语义，编排归 deploy 阶段 |
| d2d/端侧化 | Lane O 独占，禁触 |

---

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| E2E-18 依赖（Redis 决策） | ✅ D-27 已决议 Redis 保留（2026-09-24）+ E2E-18 阶段本身 ✅ done（2026-09-28 §13.3 第二方核对批准，PR #110 main=d3b0fe3）→ 状态机满足 in-progress 前置 |
| E2E-19 done（前序） | ✅ 2026-09-28 §13.3 用户批准（commit a8978df） |
| 不在决策门阻塞（RUNBOOK §9） | ✅ 仅 D-04 不阻塞任何阶段 |
| `audit --all` 0 FAIL | ✅ 2026-09-28 实测（PR #110 合并后） |
| Redis 容器 healthy | dev 栈现成（E2E-18 盘点确认） |
| `deploy/.env.local` | ✅ 严禁删除/覆盖（AGENTS §四红线） |
| dev mode 锁 | ⚠️ **Lane O T3 IAB 锁 until=2026-09-28 12:00 仍未释放**（按协议 §三.资源1 不得强占）→ 开工前协商 Lane O 释放或等待；worktree `../Emotion-Echo-e2e20` 已建（branch=`fix/e2e-20-multi-instance-concurrency`） |

环境启动（RUNBOOK §2.1，**必带 `--env-file .env.local --profile dev`**）：

```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local --profile dev up -d
```

---

## 4. 测试点清单

判定标记：`[A]` 自动 · `[V]` 视觉 · `[M]` 需裁定（RUNBOOK §4）。

| # | 测试点 | 判定 | 验证方式 | 结果 |
|---|--------|------|---------|------|
| 1 | **多实例前置**：双 BFF 实例起得来（第 2 容器 healthy + Nacos 两实例注册 / 或 BFF 直连两端口） | [A] | `docker ps` + Nacos 实例列表 + curl 双端口 /health | PASS |
| 2 | **[RED] 登录锁定跨实例失效复现**：实例 A 打 5 次错密码锁用户 → 请求实例 B → B 不锁（仍处理登录/计数归零） | [A] | curl 双实例 5 次错密码 + B 侧第 6 次观测 | PASS |
| 3 | **[RED] 验证码防枚举跨实例失效复现**：A 发码 60s 内 → B 再发 → B 重新生成（不拦） | [A] | curl 双实例 verification-code | RED 复现 PASS；2026-09-28 裁定后作废（D-28，端点为 D-01 遗留物） |
| 4 | **[RED] 限流跨节点放大**：`policy: local` 下 2 节点总配额 = 2×（单节点 60/min 实测超发） | [A] | seed.sh 回读 + 打压观测 429 阈值 | 架构性未实测（E2E-F-145 → E2E-25） |
| 5 | **`RedisLimiterBackend` TDD RED→GREEN**：allow / deny / 窗口 TTL / Redis 不可达降级（表驱动 ≥5 用例） | [A] | `go test ./pkg/middleware -run TestRedisLimiter -v` 先红后绿 + commit 序列 | PASS |
| 6 | **BFF 登录锁定 Redis 化 TDD**：跨实例计数一致 + Redis 挂降级 in-memory（≥4 用例） | [A] | `go test ./internal/handler -run TestLoginLockRedis -v` + miniredis | PASS |
| 7 | **BFF 验证码 Redis 化 TDD**：SET NX EX 60 语义 + 降级（≥3 用例） | [A] | `go test` 先红后绿 | 2026-09-28 裁定后回退作废（D-28） |
| 8 | **APISIX limit-count policy redis**：seed.sh 改 `policy: redis` + 重跑 seed + preflight 429 阈值正确（不放大） | [A] | seed.sh grep + 实际打压 429 计数 | PASS（结构断言 48/48；跨节点运行时实测归 E2E-25） |
| 9 | **[GREEN] 双实例并发验证**：修后 3 处复测全过 + 双实例并发 10 次错密码锁定态唯一不漂移 | [A] | curl 双实例 + psql/redis KEYS 观测 | PASS |
| 10 | **锁定提示视觉证据**：被锁用户登录收到锁定提示（前端可读，非静默 500） | [V] | IAB 操作 + 截图并查看 | PASS |
| 11 | **回归钉**：`e2e/multi-instance-smoke.spec.ts` 首跑绿（chromium + mobile 双 project） | [A] | `pnpm playwright test e2e/multi-instance-smoke.spec.ts` | PASS（裁定后 #11a+#11c 双 project，收口复跑 4/4） |
| 12 | **全量回归**：改到的 svc `go test ./...` + shared + 前端 `vitest run` + 本 spec 复跑 | [A] | 各段命令输出 | PASS |

汇总：12 项（11 [A] + 1 [V]）。**2026-09-28 收口**：10 PASS + 1 裁定作废（#7）+ 1 架构性留账（#4→E2E-F-145）；#3 RED 证据保留作历史。

---

## 5. 验收标准（DoD）

- [x] 12 测试点全有结论（#7 裁定作废 / #4 架构性留账 E2E-F-145）
- [x] #2-#4 RED 复现证据 + #5-#8 GREEN 修复先红后绿（commit 序列可查）
- [x] #9 双实例并发实测过（5×401 + 5×423 精确阈值，Redis keys 实写非降级）
- [x] 账本 F-25 关账（含裁定备注）；F-96 未复现不记账（归 E2E-06/E2E-23）
- [x] 回归钉 `multi-instance-smoke.spec.ts` 存在 + 首跑 6/6 绿 + 收口复跑 4/4 绿（v0.1.31 镜像，裁定后 #11a+#11c）
- [x] `audit --all` 0 FAIL（合并前后都跑）
- [x] report.md 按 §10 模板 + 三处 status 一致 + §2.5 自检三连
- [x] **§13.3 第二方核对** —— 用户 2026-09-28 批准（核对清单 11 条呈报后用户指示「如果检查无误，就收尾吧」）

---

## 6. 已知风险

| 风险 | 应对 |
|------|------|
| **双实例本地资源**（19 容器 + 第 2 BFF） | BFF 轻量；WSL memory 8GB（2026-09-22 调过）；错峰起 |
| **APISIX redis-limiter 插件**是否 APISIX 3.18 内置 | 开工先查 `apisix plugins list`；不内置则查限流插件替代 + 升级方案升级用户 |
| **Redis 挂时降级语义** | 明确：降级 = 现状单实例行为（不更糟）；不 fail-closed 拒登录（否则 Redis 成新单点）——**[M] 若两可升级用户** |
| **seed.sh 改动触发 F-139 同型**（seed ↔ admin 覆盖） | 持久化在 seed.sh；重跑 seed 验证不丢；E2E-25 范畴注明 |
| **TDD 倒置**（AP-09） | 每个修复独立 RED commit → GREEN commit；`go vet ./...` 编译测试文件 |
| **门禁 ADR**（AP-08）：多实例限流架构关键词 | 若 `check_adr_gate.sh` 命中 → 补 ADR `adr-2026-09-e2e-*.md` + 架构决策号（Lane E 从 34+1 起，避开 Lane O 的 33/34 已用号——**开工时先查两套编号**） |
| **工作目录被 Lane O 切走**（E2E-19 教训） | 用 worktree 物理隔离：`git worktree add ../emotion-echo-e2e-20 <branch>` |
| **§13.3 拖延**（E2E-19 教训：执行者自宣 done 后被纠回 partial） | 收口即写 partial + STATUS v2 核对表，等用户审 |

---

## 7. 产出物

- Go：`limiter_redis_test.go` + `limiter.go` RedisLimiterBackend 实现 + BFF auth Redis 化 + 测试
- 部署：`seed.sh` limit-count policy redis + 重跑验证
- Playwright：`emotion-echo-web/e2e/multi-instance-smoke.spec.ts`
- 截图：`stages/e2e-20-multi-instance-concurrency/screenshots/10-lock-prompt-*.png`
- 账本：F-25 关账；新发现续号（F-142 起）
- 报告：`report.md`（§10 模板）

## 8. 调研依据（AGENTS §〇.6 — 计划期 2026-09-28）

- **已读实现（4）**：`emotion-echo-web-bff/internal/handler/auth_handler.go`（行 68-93 in-memory 两 map + 注释自述"单实例假设；多实例 BFF 留 Stage 34+ Redis 迁移" + 行 315-348 锁定逻辑 + 行 248 验证码防枚举）、`emotion-echo-shared/pkg/middleware/limiter.go`（行 133-149 LimiterBackend 接口 + RedisLimiterBackend TODO）、`deploy/apisix/seed.sh`（行 349-558 limit-count `policy: local` 共 4 处）、`roadmap.md` §第五批 E2E-20 行。
- **已读测试（1）**：`emotion-echo-shared/pkg/middleware/limiter_test.go`（既有表驱动范式参考）。
- **已查 ADR/决策**：`docs/e2e-roadmap/decisions.md` D-27（2026-09-24 已决议：Redis 保留 + E2E-20 是 RedisLimiterBackend 首个接入点；用户原话「redis 正常需要接入业务」）；AGENTS §八 双轨协议（Lane E 独占 deploy/、限流归 Lane E）。
- **已查账本**：E2E-F-25（多实例 3 处失效 → 本阶段关账）、E2E-F-96（user-svc 降级 → 不修）。
- **运行时实测（计划期）**：dev 栈 19 容器（E2E-19 阶段后仍 healthy）；Redis `connected_clients:1`（E2E-18 盘点）。
- **git 溯源**：`limiter.go:137` TODO 自 Round 4.3 抽象时即在（接口早铺好，实现从未写）。
- **smoke**：不触发 §2.4 六契约（不改事件发布/视图/报表链）；但改动 BFF 鉴权路径 → 收口时回归登录链路。
- **外部信息**：APISIX 3.18 redis-limiter 插件内置性**待开工实测**（列为风险）。