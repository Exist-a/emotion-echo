---
status: landed
priority: high
created: 2026-10-13
last-refresh: 2026-10-13
type: stage-landing
owner: zcode-bot
related-plans:
  - docs/e2e-roadmap/README.md （顶部「E2E 阶段正式收工」标注）
  - docs/e2e-roadmap/decisions.md （D-51 / D-52）
---

# Stage 115 — E2E 路线图阶段收工（2026-10-13）

> **性质**：长期阶段式 E2E 测试治理（[`docs/e2e-roadmap/`](../e2e-roadmap/README.md)，2026-09-17 建立）
> 的**正式收工记录**。**权威口径在 README 顶部收工标注**，本文只做落地指针 + 一句话演进
> （防双源漂移，AP-14）。

## 一句话

**路线图 31 个阶段：30 `done` + 1 `partial`（E2E-03，owner 裁定「不再推进」）。**
`python scripts/e2e_stage_audit.py --all` → **31 阶段 0 FAIL**（`--selftest` 通过）。

## 收工口径（三值，不含糊）

| 值 | 数量 | 说明 |
|----|------|------|
| `done` | **30** | 逐点复验取证；`BLOCKED ≤ 1/3`、无 FAIL |
| `won't-do`（状态仍 `partial`） | **1** | **E2E-03 CI 严格化**（8 PASS / 13 BLOCKED = 62% >1/3）：用户裁定**不推进**、将整体重构 CI。**13 个 BLOCKED 如实保留**，不标 done（AGENTS §七） |

## 收工前的最后三轮（本 PR 之前）

| PR | 内容 |
|----|------|
| **#203** | **E2E-30** 数据契约收口：A5 名下 **11 条**阻塞账本按用户裁定「转专项」解除（更正原报告"7 条"低估）|
| **#204** | 按约束收口 **7 个历史 partial**（E2E-05/06/08/09/11/15/19，[D-51](../e2e-roadmap/decisions.md)）——**理由已过期 / 纯账本归属 / 补账**三档；报告四值行一字未改 |
| **#205** | **E2E-04** 前端工程化门槛**实做收口**：lint 9 errors→0、typecheck 插件（vue-router 4/5 双版本对齐）、a11y 门禁生效 + 修 2 类 serious、构建产物 smoke（SSR 判据更正）|
| **#206** | **E2E-07** 找回密码**实做收口**：回归钉**首次真跑 12/12** ⇒ 暴露并修 **3 处真缺陷**（匿名 401 被当会话失效→答错密保被踢回登录页；步骤守卫在 SSR 读不到 localStorage→刷新丢流程）|

## 债的处置（收工 ≠ 无债）

- **账本**：未解决条目一律二值化 —— ✅ 已解决，或**转「专项 · &lt;主题&gt;」**（owner 去阶段编号 + 写明**去向与触发条件**）。详见 [`discovered-unresolved.md`](../e2e-roadmap/discovered-unresolved.md) 各条 owner 列。
- **R 系列**：[`remediation.md`](../e2e-roadmap/remediation.md) 仍在（部分条目随本轮收口关闭）。
- **E2E-03 的 13 个 BLOCKED**：明确不做（D-52），**不是"已做"**。
- **CI 门禁重构**：E2E-03 的初衷（把校验接进 CI）与"CI 过多拖慢开发"的新判断冲突 ⇒ 由 owner 统一重构，**本路线图不再承担 CI 门禁扩张**。

## 方法论沉淀（本次收口反复用到的三条硬规则）

1. **`BLOCKED` 超 1/3 不得判 done**（AGENTS §七）——「不做」必须如实记为 won't-do，不能伪装 done。
2. **审计器口径优先于文档叙述**：A5 只读账本 **owner 列**的 `E2E-\d` 字面量，`roadmap_state_kind` 按**关键词**判定（状态单元格里出现 `partial` 字样会把阶段读回 partial）—— 本轮各踩一次。
3. **"从没真正运行过"的回归钉 = 定时炸弹**：E2E-07 的 spec 一跑就红 5 条，暴露出 3 处用户可见缺陷（其中 2 处与"页面/SSR"层有关，API 层单测永远看不到）。

> 最后更新：2026-10-13 by zcode-bot
