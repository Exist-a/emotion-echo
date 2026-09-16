---
status: planned
priority: medium
owner: TBD
created: 2026-09-16
last-refresh: 2026-09-16
type: known-issues-backlog
depends-on:
  - stage-26-T-test-backlog.md (历史版格式参考)
  - docs/plans/test-coverage-tracker-2026-09-16.md §四 (架构债登记)
related-issues:
  - A2 assessment-svc handler/logic 无单测
  - A3 reports E2E 无 Playwright
  - A4 chartData=[] 历史 bug 未实测复现
  - A5 OAP 9.x queryDuration bug
  - A6 Stage 44 §四残余（sw-oap / Nacos / 运维 SQL）
  - 顺手小问题 1: 字号防抖
  - 顺手小问题 2: useApi 401 重试同 ts 双记录
  - Sprint 110-115 后续规划
---

# Known Issues Backlog 2026-09-16

> **目的**：收集所有**未阻塞端到端 chat 链路**的已知问题，按优先级排期，作为 Sprint 109 之后的 backlog。
>
> **不在本 backlog**：阻塞 Sprint 109a (APISIX 401) 的 chat 主链路相关项已拆为 sprint-109a/109b/109c 独立 plan，本 backlog 只收剩余项。

---

## 一、调研依据（AGENTS.md §〇 硬规则）

| 文件 | 用途 |
|---|---|
| `docs/plans/test-coverage-tracker-2026-09-16.md §四` | 架构债 A2-A6 + §四 A7（已拆 109a）+ A1（已 FIXED Sprint 108）|
| `docs/stages/stage-107-chat-new-conversation-fix-2026-09-16.md §四 顺手小问题` | 字号防抖 + useApi 401 双记录 |
| `docs/stages/stage-26-T-test-backlog.md` | 历史测试 backlog 格式参考 |
| `docs/stages/stage-44-observability-sprint-b.md` | observability Sprint B 残余（sw-oap / Nacos / 运维 SQL）|
| `docs/stages/stage-93-analytics-svc-sw8-propagation-2026-09-14.md` | OAP 9.x queryDuration bug 残余 |
| `docs/plans/test-coverage-tracker-2026-09-16.md §五` | Sprint 110-115 推进顺序 |

### 架构假设清单

| 假设 | 验证 |
|---|---|
| 这些问题不阻塞 chat 端到端 | 🟡 A2 不阻塞 chat；A3/A4 不阻塞 chat；A5/A6 不阻塞 chat；顺手 1/2 不阻塞 chat |
| 工作量估计合理（每个 ≤ 1 个 sprint）| 🟡 按 A2/A3/A4/A5/A6 各 0.5-1 个 sprint 估 |
| 优先级 medium | 🟡 chat 端到端是 critical，这些是 medium |

---

## 二、backlog 清单（按优先级）

### Item 1 · A2: assessment-svc handler/logic 无单测（🔴 high）

| 维度 | 内容 |
|---|---|
| **现象** | `emotion-echo-assessment-svc` 仅有 `bootstrap_test.go` + `config_test.go`，无 `internal/handler/` 和 `internal/logic/` 单测 |
| **影响** | E2E-3 assessment 链路无回归保护，handler/logic 改动无法快速验证 |
| **DoD** | assessment-svc 每个 handler 至少 3 用例 + 每个 logic 至少 5 用例；覆盖率 ≥ 80% |
| **工作量** | 0.5-1 sprint |
| **优先级** | high（端到端 E2E-3 前置） |
| **建议排期** | Sprint 111 |

**实施步骤**：
1. 列 assessment-svc 所有 handler + logic 函数
2. 按 chat-svc `internal/logic/*_test.go`（8 个）模式补单测
3. 跑 `go test ./...` 验证 ≥ 80% 覆盖率

### Item 2 · A3: reports E2E 无 Playwright（🟡 medium）

| 维度 | 内容 |
|---|---|
| **现象** | `emotion-echo-web/e2e/` 只有 `login-flow.spec.ts`，无 `reports-flow.spec.ts` |
| **影响** | E2E-4 reports 链路无回归保护 |
| **DoD** | Playwright spec 覆盖 daily / trend / user-behavior 3 个报表的 happy path + 异常路径 |
| **工作量** | 0.5 sprint |
| **优先级** | medium（chat 优先，reports 次之） |
| **建议排期** | Sprint 112 |

**实施步骤**：
1. 模仿 `login-flow.spec.ts` 写 `e2e/reports-daily.spec.ts`
2. 覆盖：登录 → 跳转 dashboard/dailyReport → 验证 summary + chartData
3. 同样写 trend + user-behavior spec

### Item 3 · A4: chartData=[] 历史 bug 未实测复现（🔴 high）

| 维度 | 内容 |
|---|---|
| **现象** | Stage 36-FU 报告"dev 模式 4 dashboard `chartData.length === 0`"，但 Stage 103/107 没复跑验证 |
| **影响** | 数据契约 §4 (reports chartData 非空) 仍可能 FAIL，dashboard 用户体验差 |
| **DoD** | 浏览器实测 daily report 页面 chartData.length > 0，且 dev 模式触发业务事件后 chartData 真更新 |
| **工作量** | 0.5-1 sprint（看是真 bug 还是历史回归）|
| **优先级** | high（数据契约 §4 是合并前硬门槛） |
| **建议排期** | Sprint 112（与 A3 一同做） |

**实施步骤**：
1. Sprint 109b 端到端验证后立即用 browser 打开 /chat/dashboard/dailyReport
2. 看 chartData 是否空
3. 真为空 → 排查 analytics-svc SQL / outbox / Kafka consumer
4. 真非空 → 标 resolved

### Item 4 · A5: OAP 9.x queryDuration bug（🟡 low）

| 维度 | 内容 |
|---|---|
| **现象** | Stage 93 报告"OAP 9.x graphql queryDuration bug 阻塞 UI 可视化"，trace 列表 UI 看不全 |
| **影响** | 可观测性链路无法直观检查 sw8 trace UI |
| **DoD** | OAP UI 能查 trace + service map 能显示完整调用链 |
| **工作量** | 0.5 sprint（如果只是 UI bug）或 1 sprint（如果要降级 OAP 版本）|
| **优先级** | low（不阻塞业务，只阻塞观测 UI） |
| **建议排期** | Sprint 115 |

**实施步骤**：
1. 看 OAP 9.x release notes 是否承认 queryDuration bug
2. 试降级到 9.0 之前的版本
3. 或等官方 patch

### Item 5 · A6: Stage 44 §四残余（🟡 low）

| 维度 | 内容 |
|---|---|
| **现象** | Stage 44 observability Sprint B 落地后 §四残余：sw-oap telemetry / 分支 merge / Nacos / 运维 SQL |
| **影响** | 观测部分功能未完全 |
| **DoD** | Stage 44 §四 各项残余都收口 |
| **工作量** | 0.5-1 sprint |
| **优先级** | low |
| **建议排期** | Sprint 115 |

### Item 6 · 顺手 1：字号调整 PATCH /users/me ×3 防抖（🟢 low）

| 维度 | 内容 |
|---|---|
| **现象** | 浏览器调整字号触发 3 次 PATCH 请求而非 1 次 |
| **影响** | 性能小问题，不阻塞主链路 |
| **DoD** | 连续改字号 14/16/18px 最终只发 1 次 PATCH |
| **工作量** | 0.5-1 小时 |
| **优先级** | low |
| **建议排期** | 任何空闲 sprint |

**实施步骤**：
1. 找 `userConfig.fontSize` watch 代码
2. 加 lodash debounce 或自实现 setTimeout debounce
3. 浏览器实测：连点 3 次字号 → 1 次 PATCH

### Item 7 · 顺手 2：useApi 401 重试同 ts 双记录（🟢 low）

| 维度 | 内容 |
|---|---|
| **现象** | Stage 107 浏览器实测发现 `POST /conversations` 同 ts 记录两次（无 Auth + 有 Auth），怀疑 useApi 401 重试逻辑在同一 tick 触发 |
| **影响** | 日志噪音，不阻塞主链路 |
| **DoD** | 401 重试真发生只在 token 真过期时（不是首次请求）|
| **工作量** | 1-2 hour（要确认是否真并发）|
| **优先级** | low |
| **建议排期** | 任何空闲 sprint |

**实施步骤**：
1. 读 `emotion-echo-web/app/composables/useApi.ts:282-318` fetch + 401 重试逻辑
2. 看是否真在第一次请求就触发 401 重试
3. 如果是 → 修 token 注入时序
4. 加日志确认

### Item 8 · Sprint 110-115 后续（按 test-coverage-tracker §五）

| Sprint | 内容 |
|---|---|
| 110 | E2E-2 Playwright spec（chat /new → 收到 AI 回复）回归钉子 |
| 111 | E2E-3 assessment Playwright + A2 单测补齐 |
| 112 | E2E-4 reports Playwright + A3/A4 chartData 复测 |
| 113 | 数据契约 §3 §4 全量 smoke（与 109c 一起） |
| 114 | E2E-1 异常路径 Playwright（JWT 过期 → refresh → 跳登录）|
| 115 | X-2 sw8 trace UI + X-1 outbox 端到端 + A5/A6 收口 |

---

## 三、排期建议

按 test-coverage-tracker §五 + 本 backlog：

```
Sprint 109a (修 A7 APISIX 401)
   ↓
Sprint 109b (端到端 E2E-2 chat 跑通)
   ↓
Sprint 109c (数据契约 §1 §2 §5 §6 smoke)
   ↓
Sprint 110 (E2E-2 Playwright)
   ↓
Sprint 111 (E2E-3 + A2 assessment-svc 单测)
   ↓
Sprint 112 (E2E-4 + A3/A4 chartData)
   ↓
Sprint 113 (数据契约 §3 §4 全量)
   ↓
Sprint 114 (E2E-1 异常路径)
   ↓
Sprint 115 (X-1/X-2 横切 + A5/A6)
   ↓
[任何空闲] 顺手 1 + 顺手 2
```

---

## 四、commit 计划

| # | Commit | 文件 |
|---|---|---|
| 1 | `docs(plans): 新建 known-issues-backlog-2026-09-16 登记 A2-A6 + 顺手 1/2 + Sprint 110-115` | `docs/plans/known-issues-backlog-2026-09-16.md` (本文件) |

---

## 五、调研依据未做完（写前必补）

- [ ] A2 assessment-svc handler 实际数量（按 grep 验证）
- [ ] A3 reports E2E 是否真没 Playwright（grep e2e/）
- [ ] A4 chartData=[] 是否真复现（Sprint 112 一并验证）
- [ ] A5 OAP 版本号（看 Dockerfile）
- [ ] A6 Stage 44 §四 实际残余项（读 stage-44 §四原文）
