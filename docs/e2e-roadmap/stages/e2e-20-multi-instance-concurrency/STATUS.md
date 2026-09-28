# E2E-20 多实例并发 — STATUS v1（开工本会话，待 §6 六步循环）

> 本文件是 E2E-20 开工记录（2026-09-28 PR #111 + PR #112 merged 后）。
> 状态机：🟡 in-progress（plan frontmatter）/ 🟡 partial（roadmap 主表行）。
> **本会话完成度 = §6 步 1（状态机推进 + worktree 物理隔离建立）**。
> §6 步 2~6 待 Lane O 释放 dev mode 锁后启动。

---

## 状态

**🟡 in-progress**（2026-09-28）—— §6 步 1 完成；步 2~6 待 dev mode 锁协商后启动。

---

## 已做 ✅（本会话）

1. **状态机推进**：plan.md frontmatter status: pending → in-progress + 加 §3 前置条件表更新（PR #111 merged at `eaa9ef9`）
2. **roadmap 同步**：主表行 E2E-20 状态 partial 化（PR #112 merged at `e58adb3`）
3. **worktree 物理隔离**：`../Emotion-Echo-e2e20` 已建（基于 main `d3b0fe3`，分支 `fix/e2e-20-multi-instance-concurrency` HEAD=`42f3c86` + `5c9b565` 来自 PR #112）
4. **report.md 占位落地**：含 12 测试点结构化表 + 收口自检 7 项 [ ] + 引用
5. **跨会话 memory**：`emotion-echo-e2e-18-v2-closure-and-e2e-20-prep-session.md` 入索引

---

## 未做 ❌

1. **§6 步 2 起环境**：dev mode 锁被 Lane O 占用（until=2026-09-28 12:00）—— 按协议 §三.资源1 不得强占，需用户决定协商策略
2. **§6 步 3 IAB 实测 #1**：双 BFF 实例起得来 + Nacos 注册 2 实例
3. **§6 步 3 RED 实测 #2/#3/#4**：三处跨实例失效复现（登录锁定 / 验证码防枚举 / APISIX 限流）
4. **§6 步 5 TDD 修复 #5/#6/#7/#8**：RedisLimiterBackend + BFF auth_handler Redis 化 + APISIX policy redis
5. **§6 步 5 GREEN 双实例并发 #9**：修后 3 处复测 + 并发 10 次错密码
6. **§6 步 6 收口 11 项**：视觉证据 [V]#10 + 回归钉 #11 + 全量回归 #12 + §13.3 第二方核对

---

## devmode-session 锁状态（关键阻塞）

```
owner: lane-o
devmode-session: t3-iab-demo-verify
until: 2026-09-28 12:00
started: 2026-09-28 00:30
note: T3 Demo IAB 验证 + WebLLM 真引擎接入 + Qwen3-1.7B 真机基线
```

按协议 §三.资源1：**Lane E 不得强占**。需用户决定：
- 方案 A：等 Lane O 主动释放（协议 §五收工三查第 2 条自动删除）
- 方案 B：协商时间片
- 方案 C：用户拍板当前 Lane E 优先（需协议 §六登握手行）

---

## 关键事件 / 教训（本会话期间的发现，诚实记录）

1. **audit A1 partial 阶段要求 report.md**：主表行状态 `partial` 触发 A1 检查要求 report.md 存在；本会话已建占位 report.md 让 A1 PASS（A0 WARN 路径只对未开工 pending 生效）
2. **audit `in-progress` 字面量不识别**：`roadmap_state_kind()` 优先级 partial > blocked > pending > done，无 in-progress 分支；`🟡 in-progress` 回落 pending → 改用 `🟡 partial` 让 audit 识 partial
3. **main 写保护禁止直推**：roadmap 同步必须 PR 模式；本地 commit 后需 `git reset --soft HEAD~1` + 新分支 + 开 PR

---

## §2.5 收工三连

```
$ git status -sb
## main...origin/main
* main  # 与 origin/main 无 ahead/behind（已 FF 至 e58adb3）

$ git status
nothing to commit, working tree clean

$ git branch --merged main
$ # 仅 main（其余已合并删除）
```

worktree `../Emotion-Echo-e2e20` **保留**（E2E-20 §6 步 2 起环境时直接使用）。

---

## 下次会话开场动作（留给接手者）

1. **检查 `deploy/.devmode-session`**：若 Lane O 已释放 → Lane E 写锁（owner: lane-e）+ 启动 dev mode
2. 启动命令（**必带 `--env-file .env.local --profile dev`**，RUNBOOK §2.1）
3. 核对 APISIX 3.18 redis-limiter 插件内置性（plan §6 风险）
4. 入口：cd `../Emotion-Echo-e2e20` + 跑 §6 步 2 起环境
5. 验证 Redis 容器 healthy + 19 容器栈稳定（参考 E2E-19 §13.3 17 断言范式）

---

## 引用

- [plan.md](plan.md)（12 测试点就绪，状态 in-progress）
- [report.md](report.md)（占位 12 行结论 + 7 项收口自检 [ ]）
- [RUNBOOK.md](../../RUNBOOK.md)（§6 六步循环 + §13.3 第二方核对）
- [anti-patterns.md](../../anti-patterns.md)（14 类反例 + AP-04 账本对账）
- [roadmap.md](../../roadmap.md) E2E-20 主表行 🟡 partial