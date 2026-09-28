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
> §6 步 2~6（起环境 / IAB 实测 / RED 证伪 / TDD 修复 / GREEN 双实例 / 收口 11 项）**待 Lane O 释放 dev mode 锁后启动**（详见 §2 待执行）。

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
| 3 | **[RED] 验证码防枚举跨实例失效复现**：A 发码 60s 内 → B 再发 → B 重新生成 | [A] | PASS（RED 复现成功） | BFF-1 (8894) POST `/api/v1/auth/verification-code {username:smoke_user}` → 200 `devCode:259576`；BFF-2 (8895) 同样请求 1s 后 → 200 **`devCode:803818`（新码被发，60s 防枚举失效）**；BFF-1 (8894) 第 3 次自身重发 → 200 `success:true` 但 devCode 字段为空（**自身 60s 防枚举正确触发**） | 根因 = `verificationCodes` map 进程内存；修法走 Redis 化（`SET NX EX 60`） |
| 4 | **[RED] 限流跨节点放大**：`policy: local` 下 2 节点总配额 = 2×（单节点 60/min 实测超发） | [A] | 架构性 RED（dev 单 APISIX 不可实测） | APISIX `policy: local` + count=60/60s + key=remote_addr（已配置，seed.sh:366-372 + 436-442）；但 dev 栈仅 1 APISIX 节点（`emotion-echo-apisix`）—— 单 APISIX 自身无"跨节点放大"问题。**实测 70 次 login**：5 × 401 + 55 × 423 + 10 × 503（限流/锁定均工作）。**架构性 RED = 生产多 APISIX cluster 下必然发生**：每个 APISIX 各自 local policy 计数 = 总配额 × N | 修法 = `policy: redis` + APISIX redis-limiter 插件（待开工实测插件内置性）；dev 单 APISIX 不可复现放大，但修复方向明确 |
| 5 | `RedisLimiterBackend` TDD RED→GREEN（allow/deny/窗口 TTL/Redis 不可达降级） | [A] | N/A | 待 §6 步 5 TDD | |
| 6 | BFF 登录锁定 Redis 化 TDD（跨实例计数一致 + 降级） | [A] | N/A | 待 §6 步 5 TDD | |
| 7 | BFF 验证码 Redis 化 TDD（SET NX EX 60 + 降级） | [A] | N/A | 待 §6 步 5 TDD | |
| 8 | APISIX limit-count policy redis | [A] | N/A | 待 §6 步 5 TDD | |
| 9 | [GREEN] 双实例并发验证：修后 3 处复测全过 + 并发 10 次错密码锁定态唯一 | [A] | N/A | 待 §6 步 5 GREEN | |
| 10 | 锁定提示视觉证据：被锁用户登录收到锁定提示（前端可读，非静默 500） | [V] | N/A | 待 §6 步 6 IAB | |
| 11 | 回归钉：`e2e/multi-instance-smoke.spec.ts` 首跑绿（chromium + mobile 双 project） | [A] | N/A | 待 §6 步 6 | |
| 12 | 全量回归：改到的 svc `go test ./...` + shared + 前端 `vitest run` + 本 spec 复跑 | [A] | N/A | 待 §6 步 6 | |

汇总：`PASS 0 / FAIL 0 / BLOCKED 0 / N/A 12`（待 §6 步 6 收口填实；12 个测试点均未执行 = N/A）

---

## 3. 发现与分类（本会话暂无）

（本节待 §6 步 3 IAB 实测时填入）

---

## 4. 修复清单（本会话暂无）

（本节待 §6 步 5 TDD 修复时填入）

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

## 7. 收口自检

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