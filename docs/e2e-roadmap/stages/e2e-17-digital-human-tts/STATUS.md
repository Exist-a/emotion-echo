# E2E-17 step 5 收口 — STATUS（2026-09-23 17:xx，session 强制收尾）

> **本会话至此的实际状态**。诚实记录，不要美化。

## 一、本会话整体收口

| 项 | 状态 | 备注 |
|----|-----|------|
| PR #68 plan 校准 | ✅ merged `2db2cfe` | |
| PR #69 A0 镜像基础设施 | ✅ merged `e1e68e7` | |
| PR #70 F-127 BFF `/tts/phonemes` | ✅ merged `c7ff203` | |
| PR #71 F-128 前端 phoneme 驱动 | ✅ merged `51d11f9` | |
| PR #72 F-129 段间断点队列化 | ✅ merged `ee03688` | |
| main | `ee03688` | working tree 干净 |
| e2e_stage_audit.py --all | **30 阶段 0 FAIL** | |

## 二、E2E-17 step 5 收口（plan §6）—— 当前真实状态

### 已做 ✅

1. **Playwright 回归钉 spec 已写好**：
   - `emotion-echo-web/e2e/digital-human-tts.spec.ts`（3 用例 × 双 project = 6 测试用例）
   - 用例 #3：BFF `/api/v1/tts/phonemes` 契约（audio base64 + phonemes + duration）
   - 用例 #10/#17：发消息 → 跳转 → 数字人 wrapper 可见 + TTS 请求断言
   - 用例 #18：mobile project 数字人 wrapper 可见
   - 含重试纪律（E2E-F-115 同型）+ click 等 enabled（Vue :disabled 时序）+ strict mode first()（[id].vue 与组件两层 wrapper）
2. **手动 curl 直证 BFF → phonemes 端到端通**：200 + RIFF header + 18.7s warm path（v0.1.28 后）
3. **Playwright 5/6 通过**（chromium 全过 + mobile 2/3 过）：#10/#17/#18 双 project 端到端 TTS 链 + 数字人 wrapper 断言全绿
5. **BFF 镜像 v0.1.28 重建并跑通**（修了 30s → 90s 超时，与 v0.1.27 区别）
6. **web 镜像 v0.1.5 重建并跑通**（含 F-128/F-129 前端修复，原镜像 v0.1.4 跑旧 bundle —— E2E-F-99 同型真 bug 已修）
7. **compose 镜像源切换**：web v0.1.4→v0.1.5，BFF v0.1.27→v0.1.28（**本次会话内未 commit**——见未做 §1）

### 未做 ❌（本会话 session 限制强制收尾）

1. **Playwright #3 mobile project 持久失败 6/6 次**（含 5s 暖身 + 10s 重试间隔 + 180s timeout × 3 次）：
   - **直接 curl 同端点返 200 + 18.7s warm + RIFF** — 端点本身工作
   - Playwright `page.request.post` 在 mobile project 下持续 502 — **这是 Playwright API client 与 chromium client 的行为差异**（可能与 mobile viewport 的 network stack / connection pool / chromium IPC 有关，**与产品无关**）
   - 直接 curl 走 Linux 网络栈，Playwright page.request 走 Chromium headless 内部（mobile viewport）—— 可能是 chromium headless 在 mobile 模拟下对 BFF 上游 keepalive 的处理差异
2. **Playwright spec commit 还未 push**（spec 文件改了但还在工作树）
3. **IAB 视觉证据**（plan #7/#12/#16 截图）—— **完全未做**
4. **report.md §10 模板收口** —— **完全未做**
5. **F-05 / F-06 账本回填**（标记 ✅ + 引用证据）—— **完全未做**
6. **plan.md status: in-progress → done** —— **未做**
7. **roadmap E2E-17 row status in-progress → done** —— **未做**
8. **最终审计 e2e_stage_audit.py --all** —— 上次跑通 30 阶段 0 FAIL（commit #72 后），但本轮 web/v0.1.5 + BFF/v0.1.28 + compose bump 未 commit → 这次 commit 后审计未重跑
9. **session 记忆 + memory 文件** —— **未写**

## 三、根因诚实交代

### 3.1 E2E-F-99 同型真 bug（spec 写出来后才暴露）

最初 #10/#17 跑不通 → 加 console 诊断 → 发现前端日志是**旧代码**（"[TTS Stream] Playing:..."是改造前的字面）。根因：web 容器跑 v0.1.4（13:13 构建）< F-128 merge 13:54 < F-129 merge 14:11。

**记忆里有这条**：E2E-16 #99 —容器跑旧代码。修了但本轮又踩。

教训：CI/CD 应该自动 rebuild web 镜像 + restart BFF（plan §6 step 0/4 都该触发 git 操作），目前 workflow 缺这一步 — **这是项目层 infra gap，记入 #F-130 候选**。

### 3.2 mobile #3 持续 502（未根除）

直接 curl 通、chromium Playwright 通、mobile Playwright 持续 502 → 三态差异。猜测：Chromium mobile 模拟下 `page.request.post` 与 host 网络栈差异（可能 keepalive / connection race）。

**没时间诊断**。report.md 应标记 #3 mobile BLOCKED（产品 #3 chromium + 端到端 E2E 都通了，mobile 仅测试基础设施问题），标注：**待下轮查 Playwright mobile 项目 API client 与 chromium 行为差异**。

### 3.3 session 太长（你说过"失败太多次"——这是真的）

连续多轮 Docker daemon 冻结 + WSL 内存击穿 + docker desktop 重启 + 数据库迁移 idempotency bug + compose up 中止 + db-migrate 503 + apisix Nacos 上游缓存陈旧 + 多容器重启时序……

从 F-128 (~14:xx) 到 F-129 (~14:xx) 到本 step 5 收口尝试，连续超过 3 小时。**轮次过多导致推断疲劳 + 反复试错**。承认这一轮能力边界。

## 四、下次 session 接手时

按 plan §6 step 5 顺序：

1. **读完本 STATUS.md**（这份文档）+ 看现有 spec 文件
2. **先解决 mobile #3 BLOCKED**：把 `page.request.post` 换成 `request.newContext()` 或 `page.evaluate(() => fetch(...))`（页面上下文的 fetch，避开 Chromium API client 差异）—— 应能直接通
3. spec commit + push + CI 合并（spec 已写，commit 是 5 行命令）
4. IAB 视觉证据：用 IAB 打开 chat 页，截图（截图天然是 [V] 证据）
5. report.md §10 模板收口 + F-05/F-06 回填
6. plan.md → done + roadmap → done + audit 重跑 + final commit + merge + 分支清理
7. session 记忆：写一份本会话长度的 memory（E2E-F-128 与 F-129 之间的方法论整合到 memory）

## 五、本会话原 commit 状态盘点（避免丢失）

未 commit 工作树改动：
- `emotion-echo-web/app/lib/apiRoutes.ts`（F-128 已 commit 到 PR #71 ✅）
- `emotion-echo-web/app/composables/useTTSPlayer.ts`（F-128 已 commit 到 PR #71 ✅，F-129 已 commit 到 PR #72 ✅）
- `emotion-echo-web/app/composables/useTTSPlayer.queue.test.ts`（F-129 已 commit 到 PR #72 ✅）
- `emotion-echo-web/app/composables/useTTSPlayer.phoneme.test.ts`（F-128 已 commit 到 PR #71 ✅）
- `deploy/docker-compose.apps.yml` web v0.1.4→v0.1.5（**未 commit**）
- `deploy/docker-compose.apps.yml` BFF v0.1.27→v0.1.28（**未 commit**）
- `emotion-echo-web/e2e/digital-human-tts.spec.ts`（**未 commit**——spec 文件改了重试 + warmup）

**未 commit 改动要保留**：本 STATUS.md 落地后，commit spec + compose bump + STATUS.md 为一个 docs PR（不强求通过 CI 红绿——CI 不会跑 E2E docker，重跑只审计）。

## 六、决策 D-26（commit 时记入）—— ✅ 2026-09-23 下轮实际落地（本节更新）

> **D-26 编号不占用**：本节原预留的四条实质均为状态更新 + 账本项，无新的用户决议；`e2e-roadmap/decisions.md` 的 **D-26 编号让位给 v0.3 计划的"端侧化主方案 ADR"**（[v0.3 §B.2](../../../../plans/on-device-hybrid-inference-implementation-roadmap-2026-09-23.md)，本阶段与端侧化不冲突）。

原四条预留的实际落地（2026-09-23 下轮 session）：

| 原预留 | 实际落地 |
|--------|----------|
| E2E-17 状态 = partial（step 5 收口未完成） | ✅ **done**（PR #75 merged `2c6607b`：19/19 测试点 + Playwright 双 project 6/6 + 4 张截图 + audit 全清） |
| F-126/F-127/F-128/F-129/F-130 按 ledger 处理 | ✅ F-126~129 已修复登记；**F-130 已登记开放**（CI/CD 镜像 rebuild 编排缺口，范围外记账） |
| mobile #3 = 测试基础设施 BLOCKED，留待下轮 | ✅ **已根除**：真因 = about:blank 跨域 fetch 触发 chromium same-origin policy（旧诊断"page.request vs chromium client 差异"是错的）；修 `page.goto('/login')` + `page.evaluate(fetch)` + retry 6×60s（XTTS 单实例串行） |
| web 镜像 / BFF rebuild 流程缺口 = 候选 E2E-CI 改进项 | ✅ 已登记 **E2E-F-130**（候选修法 ① Actions merge-to-main job ② 版本端点自检） |

**下轮新增决议**（本阶段真实修出的四层真因，全部已有 ADR/测试钉住）：
1. **E2E-F-127 yaml/config.go 漂移** → yaml 30000→90000 + `config_test.go` 守卫（**决策 32 / adr-2026-09-apisix-upstream-timeout-web-bff.md**）
2. **APISIX upstream 6 (web-bff) timeout 180s** → seed.sh `put_nacos_upstream` id=6 单独给 180（同 ADR）
3. **mobile #3** → `page.goto('/login')` + `page.evaluate(fetch)`（spec 已 commit）
4. **XTTS 单实例串行** → spec retry 6×60s + test 级 600s（spec 已 commit）