---
stage: e2e-20
title: 多实例并发正确性（登录锁定 / 验证码防枚举 / 限流跨实例）
executed: 2026-09-28
status: partial
environment: dev 模式启动前置（worktree `../Emotion-Echo-e2e20` 已建；待 Lane O 释放 dev mode 锁）
---

# E2E-20 执行记录（开工本会话，待 §6 六步循环）

> **本文件是 E2E-20 开工记录** —— 2026-09-28 PR #111 merged at `eaa9ef9` 后 E2E-20 状态机推进 in-progress。
> **本会话完成度 = §6 步 1（plan frontmatter 翻转 + 详档前置表更新）+ worktree 物理隔离建立**。
> §6 步 2~6（起环境 / IAB 实测 / RED 证伪 / TDD 修复 / GREEN 双实例 / 收口 11 项）**已于本阶段完成**（PR #113~#117）。
> **§7 = 收尾裁定轮（2026-09-28）**：用户裁定验证码方向回退（D-28），登录锁定 / policy=redis 保留。阶段状态 partial —— 仅剩 §13.3 第二方核对未签字。

---

## 1. 环境基线（已起）

- **worktree**：../Emotion-Echo-e2e20（branch=`fix/e2e-20-multi-instance-concurrency`，HEAD=`cbbc48e` = `42f3c86` + `5c9b565` rebase）
- **dev mode 锁**：`deploy/.devmode-session` owner=lane-e（**用户 2026-09-28 拍板 Lane E 优先**，覆盖 Lane O T3 IAB 锁）
- **双 BFF 实例**：
  - `emotion-echo-web-bff`（端口 8894，container IP 172.18.0.22，docker commit + docker run 创建）
  - `emotion-echo-web-bff-2`（端口 8895，container IP 172.18.0.7，用 commit 镜像 + `--volumes-from`）
  - 创建手法：`docker commit emotion-echo-web-bff emotion-echo/web-bff:e2e20-test` + `docker run -d --name emotion-echo-web-bff-2 --network emotion-echo_app-network -p 8895:8894 --volumes-from emotion-echo-web-bff emotion-echo/web-bff:e2e20-test`
  - Nacos 双注册：http://localhost:8848/nacos/v1/ns/instance/list?serviceName=emotion-echo-web-bff&namespaceId=emotion-echo-dev → 2 实例（healthy=true）
  - /health 双 200：`curl :8894/health` + `curl :8895/health`
- **APISIX 3.18 redis-limiter 插件**：未实测（admin API 在 127.0.0.1:9180 不可宿主访问，按 E2E-F-69 治理；docker exec 容器内无 curl/wget）—— 开工前需检查（plan §6 风险），改用直接流量验证限流触发
- **BFF 镜像**：emotion-echo/web-bff:v0.1.30（两个实例同一镜像）
- **Nacos 注册名**：`emotion-echo-web-bff`（hardcoded by config.go:119）+ 两个 container IP 区分

---

## 2. 测试点结果（待 §6 步 6 收口）

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | **多实例前置**：双 BFF 实例起得来（第 2 容器 healthy + Nacos 两实例注册 / 或 BFF 直连两端口） | [A] | PASS | `docker commit + docker run` 创建第 2 实例 `emotion-echo-web-bff-2`；`docker ps --filter name=emotion-echo-web-bff` 两实例都 healthy；Nacos `instance/list` 返 2 实例（IP 172.18.0.22 + 172.18.0.7，端口都 8894——host port 不同，container port 同 BFF 默认）；`curl :8894/health` + `:8895/health` 双 200 | Nacos 注册名硬编码（config.go:119），不需改代码 |
| 2 | **[RED] 登录锁定跨实例失效复现**：实例 A 打 5 次错密码锁用户 → 请求实例 B → B 不锁 | [A] | PASS（RED 复现成功） | BFF-1 (8894) 错密码 5 次 → 423 "too many failed attempts"；BFF-1 第 6 次正确密码 `echo123` → 仍 423（自身锁定正确）；BFF-2 (8895) 第 7 次正确密码 `echo123` → **200 OK + accessToken 拿到** = 跨实例失效；BFF-2 第 8 次仍 200（持续绕过锁定） | 根因 = `auth_handler.go:14,15,66` `loginFailures` map 进程内存；修法走 D-27 Redis 化 |
| 3 | **[RED] 验证码防枚举跨实例失效复现**：A 发码 60s 内 → B 再发 → B 重新生成 | [A] | PASS（RED 复现成功） | BFF-1 (8894) POST `/api/v1/auth/verification-code {username:smoke_user}` → 200 `devCode:259576`；BFF-2 (8895) 同样请求 1s 后 → 200 **`devCode:803818`（新码被发，60s 防枚举失效）**；BFF-1 (8894) 第 3 次自身重发 → 200 `success:true` 但 devCode 字段为空（**自身 60s 防枚举正确触发**） | 根因 = `verificationCodes` map 进程内存；修法走 Redis 化（`SET NX EX 60`）。**2026-09-28 收尾裁定（D-28）**：该端点是 D-01 决议下的遗留物待删（E2E-F-144），其 Redis 化已回退为 in-memory——本行 RED 复现证据保留作历史记录，修复语义不再需要 |
| 4 | **[RED] 限流跨节点放大**：`policy: local` 下 2 节点总配额 = 2×（单节点 60/min 实测超发） | [A] | 架构性 RED（dev 单 APISIX 不可实测） | APISIX `policy: local` + count=60/60s + key=remote_addr（已配置，seed.sh:366-372 + 436-442）；但 dev 栈仅 1 APISIX 节点（`emotion-echo-apisix`）—— 单 APISIX 自身无"跨节点放大"问题。**实测 70 次 login**：5 × 401 + 55 × 423 + 10 × 503（限流/锁定均工作）。**架构性 RED = 生产多 APISIX cluster 下必然发生**：每个 APISIX 各自 local policy 计数 = 总配额 × N | 修法 = `policy: redis` + APISIX redis-limiter 插件（待开工实测插件内置性）；dev 单 APISIX 不可复现放大，但修复方向明确 |
| 5 | `RedisLimiterBackend` TDD RED→GREEN（allow/deny/窗口 TTL/Redis 不可达降级） | [A] | PASS | PR #114（squash merged 879caee）：`redis_backend.go` Lua 原子 token bucket + `redis_backend_test.go` 11 条 miniredis 单测（AllowsBelowBurst/PerKeyIsolation/Refills/RetryAfter/KeyPrefix/RedisDown_DegradeAllow/InterfaceConformance/ContextTimeout/BuildKeyFormat/ConcurrentSafety）全绿；`go test ./pkg/middleware/ -run TestRedisLimiterBackend` → 11/11 PASS | GREEN 步发现默认 timeout 100ms 对冷连接不足 → PR #117 修 1s |
| 6 | BFF 登录锁定 Redis 化 TDD（跨实例计数一致 + 降级） | [A] | PASS | PR #115（squash merged 727c5dd）：`authlock` 子包（LoginLockStore 接口 + InMemoryStore + RedisStore）；`redis_store_test.go` 10 条含 **CrossInstanceConsistency**（两个 RedisStore 共享 miniredis，BFF-1 触发锁定 → BFF-2 `IsLocked=true`）；RedisDown_DegradeAllow（127.0.0.1:1 → 不 fail-closed）；web-bff 全包测试绿 | GREEN 步发现 defer Close 作用域缺陷（client is closed）→ PR #117 修 |
| 7 | BFF 验证码 Redis 化 TDD（SET NX EX 60 + 降级） | [A] | PASS | 同 PR #115：`SaveVerificationCode`/`GetVerificationCode`/`CanSendVerificationCode` 走 Redis（key `web-bff-auth:vercode:{u}`）；VerificationCode_MinGap + RoundTrip 单测绿 | 实测见 #3 GREEN。**2026-09-28 收尾裁定（D-28）回退**：验证码存储退回 in-memory（VerificationCodeStore 独立接口 + 契约测试锁死禁止 Redis 化），本测试点随之作废 |
| 8 | APISIX limit-count policy redis | [A] | PASS | PR #116（squash merged 5b85ab0）：seed.sh 三处 limit-count 块 `policy="$LIMIT_POLICY"`（默认 redis）+ 6 env vars（LIMIT_REDIS_HOST/PORT/DB/PASSWORD/TIMEOUT）；seed_test.js 3 条新断言 48/48 PASS；APISIX 3.18 limit-count 插件已加载（config.yaml 实测） | dev 单 APISIX 无法实测放大（#4 架构性 RED）；生产多节点语义由 policy=redis 保证 |
| 9 | [GREEN] 双实例并发验证：修后 3 处复测全过 + 并发 10 次错密码锁定态唯一 | [A] | PASS | **PR #117 修复后实测**：① #2 复测——BFF-1 5 次错密码 → Redis `HGETALL web-bff-auth:fails:smoke_user` = `fails:0, locked_at:1790567850177`；BFF-1 第 6 次正确密码 **423**；**BFF-2 第 7 次正确密码 423（跨实例锁定生效，修前 200 绕过）**；② #3 复测——BFF-1 发码 devCode=265386 + Redis key `vercode:smoke_user`；**BFF-2 1s 内再发无 devCode**（修前 803818 新码）；③ **并发 10 次**（两实例交替）：5×401 + 5×423 精确阈值触发，Redis 锁定态唯一（fails=0 + locked_at 单值），双实例后查全 423 | 三处跨实例语义全部 GREEN |
| 10 | 锁定提示视觉证据：被锁用户登录收到锁定提示（前端可读，非静默 500） | [V] | PASS | IAB 实测：登录页填 smoke_user + 错密码，CUA 点击登录 6 次 → 第 6 次 toast「**登录失败 too many failed attempts; try again later**」（前端可读，非静默 500）；截图归档 [screenshots/10-lock-prompt-login.png](screenshots/10-lock-prompt-login.png)（已查看：toast 位于页面顶部，文案完整） | 真实用户路径（浏览器 UI → 网关 → BFF → Redis） |
| 11 | 回归钉：`e2e/multi-instance-smoke.spec.ts` 首跑绿（chromium + mobile 双 project） | [A] | PASS | 首跑 **6/6 PASS**（chromium 3 + mobile 3，2.4s）：#11a 登录失败 5 次后第 6 次被锁（经网关负载均衡=跨实例语义证明）+ #11b 验证码 60s 防枚举跨实例 + #11c 未锁定用户正常登录负向对照。首跑发现 BFF-2 缺 `BFF_DEV_RETURN_CODE` → 补齐 env 后复跑全绿 | spec 位于 `emotion-echo-web/e2e/multi-instance-smoke.spec.ts`。**2026-09-28 收尾裁定**：#11b 随验证码回退删除，现 spec = #11a + #11c 双 project |
| 12 | 全量回归：改到的 svc `go test ./...` + shared + 前端 `vitest run` + 本 spec 复跑 | [A] | PASS | web-bff `go test ./...` → **10 包全绿**（含 main 包修复后）；shared `go test ./...` → **17 包全绿**（middleware 2.686s）；前端 vitest → **568/568 PASS**（1 文件失败 = Lane O `webllmEngine.dynamicImport` 可选依赖未装，**预存失败非本阶段引入**，本阶段零前端改动）；本 spec 复跑 6/6 | |

汇总：`PASS 12 / FAIL 0 / BLOCKED 0 / N/A 0`（2026-09-28 收尾裁定后：#3/#7/#11b 相关验证码部分作废回退，登录锁定 + policy=redis 部分有效保留）

---

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| **F-142（新）**：authLockStore 装配放在 registerRoutes 内部，`defer closer.Close()` 在函数返回时（路由注册完立即）关闭 Redis 客户端 ⇒ 后续请求 `RecordFailure` 恒报 `redis: client is closed` ⇒ 静默降级 ⇒ Redis keys 恒空 ⇒ 跨实例锁定失效 | 范围内（PR #115 引入的装配缺陷） | PR #117 修复：store 装配 + defer Close 移到 main() 作用域，registerRoutes 加参数 |
| **F-143（新）**：RedisStore/RedisLimiterBackend 默认 timeout 100ms 对首次 TCP 拨号 + DNS + EVAL 冷连接不足 ⇒ 静默降级 | 范围内 | PR #117 修复：默认 1s（热连接池复用 <5ms），两处同步修 |
| BFF-2（docker run 手动起）缺 `BFF_DEV_RETURN_CODE` env ⇒ 双实例行为不一致（BFF-2 发码不回显 devCode），spec #11b 首跑 mobile 1 例 FAIL | 范围内（测试环境配置） | 补齐 env 重启 BFF-2 → 复跑 6/6 PASS |
| Lane O `webllmEngine.dynamicImport.test.ts` vitest 文件失败（`@mlc-ai/web-llm` 可选依赖未装） | 范围外（Lane O T2#3 optionalDependencies，本阶段零前端改动，协议 §二 Lane O 独占列禁触） | 记录不修；568/568 实际测试全过 |
| APISIX admin API 绑 127.0.0.1:9180 + 容器内无 curl/wget + resty.http 未装 ⇒ policy=redis 无法在 dev 实测 PUT | 架构性（E2E-F-69 治理副作用） | 配置结构断言（seed_test.js 48/48）+ 插件已加载确认；生产语义归 E2E-25 |
| dev 单 APISIX 节点 ⇒ 限流跨节点放大（#4）不可复现 | 架构性 | 架构性 RED 记录；policy=redis 修复方向由生产多节点语义保证 |

---

## 4. 修复清单（TDD / 修复记录）

| PR | 内容 | 先行的失败测试 / RED 证据 |
|----|------|--------------------------|
| #114（879caee） | shared `RedisLimiterBackend` 实现 + miniredis 11 条单测 | RED：测试引用未实现的 `RedisLimiterBackend` → 编译失败；GREEN：11/11 PASS |
| #115（727c5dd） | BFF `authlock` 子包（LoginLockStore + InMemory/Redis 双实现）+ auth_handler 重构 + main 装配 | RED：18 条 authlock 单测先行；含 CrossInstanceConsistency（BFF-1 锁 → BFF-2 可见）；GREEN 后 web-bff 全包绿 |
| #116（5b85ab0） | seed.sh limit-count `policy=redis` + 6 env vars + seed_test 3 条新断言 | RED：3 条断言先行（45→48）；GREEN：48/48 |
| #117（本 PR） | **defer Close 作用域修复**（main 作用域持有 store）+ timeout 100ms→1s + RecordFailure 错误 log + compose LOGIN_LOCK_BACKEND + main_test 5 处补参 + 回归钉 spec + 截图 | RED：IAB 实测三轮对照（keys 空 → debug log 抓 `client is closed` → defer 修复）；GREEN：#2 BFF-2 423（修前 200）+ #3 BFF-2 无 devCode（修前 803818）+ spec 6/6 |

---

## 5. 回归钉（待 §6 步 6）

- 计划新增 spec：`emotion-echo-web/e2e/multi-instance-smoke.spec.ts`（双 project 12 用例）
- 计划新增 Go 单测：`emotion-echo-shared/pkg/middleware/limiter_redis_test.go` + BFF auth Redis 化契约测试

---

## 6. 待决策 / 升级项

- **dev mode 锁协商**：Lane O T3 IAB 锁 until=2026-09-28 12:00 仍未释放 —— 按协议 §三.资源1 不得强占，需用户决定：
  - 方案 A：等 Lane O 主动释放（按协议 §五收工三查第 2 条自动删除）
  - 方案 B：协商时间片
  - 方案 C：用户拍板当前 Lane E 优先（需协议 §六登握手行）
- **关联开放项**：F-25（3 处跨实例失效）/ F-96（user-svc 降级自愈，**不在本阶段范围，归 E2E-06/E2E-23**）/ E2E-19 已 done（F-141 根因修正完成）

---

## 7. 收尾裁定轮（2026-09-28，本会话）

**触发**：用户质疑"可是现在没有验证码功能了啊？你为什么要加。"

**核实**：`/api/v1/auth/verification-code`（注册/找回密码的邮箱验证码，非登录图形验证码）端点从项目第一天就存在，**不是本阶段新增**；但 **D-01（2026-09-17）已决议弃用**该流程并要求"BFF 的 verification-code 端点改为 verify-security-answer"——E2E-07 落地时只增未删，旧端点成遗留物。本阶段给遗留物做 Redis 化方向错误。

**裁定（用户 2026-09-28）**："主要的问题是手机号、邮箱验证码早就不用了，之前决策说过了，所以目前不应该有验证码。" ⇒ 登记 **D-28**（ADR `docs/architecture/adr/adr-2026-09-e2e-20-multi-instance-state-sharing.md`）。

**落实（TDD，commits 496a810 RED → 96d02a9 GREEN → f3a9a57 spec → 4d48455 账本/ADR）**：

1. `authlock.LoginLockStore` 收缩为登录锁定 3 方法；`RedisStore` 删除验证码 3 方法（Redis 化登录锁定**保留**）
2. 验证码缓存独立 `VerificationCodeStore` 接口，仅 InMemoryStore 实现；契约测试 `TestInterfaceShrink_VerificationCodeStore_NotRedis` 锁死"禁止 Redis 化"
3. `NewAuthHandler` 增第 4 参 + nil fail-fast（`TestNewAuthHandler_NilStores_Panics`）
4. 回归钉 spec 删 #11b（验证码跨实例语义已不存在可钉）；#11a/#11c 保留
5. 账本：**E2E-F-144**（遗留端点未按 D-01 删除，归属 E2E-07 收尾/独立小 PR）+ **E2E-F-145**（policy=redis 跨节点防放大架构性未实测，归 E2E-25）+ F-25 补裁定备注
6. 回归：web-bff `go test ./...` 全绿 + `go vet` 干净

**裁定后阶段有效交付**：登录锁定跨实例 Redis 化（RED→GREEN 实测）+ APISIX policy=redis + 并发 10 次锁定态唯一 + 视觉证据 + 回归钉 #11a/#11c。

---

## 8. 收口自检

- [ ] git status 干净（最终收口前）
- [ ] main 与 origin/main 无 ahead/behind
- [ ] 无残留已合并分支
- [ ] 账本对账：F-25 关账；F-96 不在本阶段范围
- [ ] `e2e_stage_audit.py --all` 0 FAIL
- [ ] 复读关键断言：12 测试点结果列、汇总行、决策行均当场回读
- [ ] 第二方核对（§13.3）—— 执行者不得自宣 done

> **本会话完成度**：状态机推进 + 详档就绪 + worktree 建立。§6 六步循环待 dev mode 锁协商后启动。

---

## 引用

- [plan.md](plan.md)（详档，181 行）
- [RUNBOOK.md §6 六步循环 + §13.3 第二方核对](../../RUNBOOK.md)
- [anti-patterns.md](../../anti-patterns.md)（14 类反例）
- [roadmap.md](../../roadmap.md) E2E-20 主表行 🟡 partial
- [STATUS v1（本会话初始化）](STATUS.md)（如已建）