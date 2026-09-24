---
purpose: 端侧化 stage1（Lane O）期间发现的问题账本（OND-F-xx 续号）
date: 2026-09-24
status: active
scope: docs/_meta/parallel-tracks.md §三.资源3 —— 仅 Lane O 会话写入
last-refresh: 2026-09-24
---

# 端侧化发现账本（OND-F）

> **规则**（parallel-tracks.md §三.资源3）：
> - 本账本**仅 Lane O（端侧 stage1）会话写入**；Lane E 的问题继续登
>   `docs/e2e-roadmap/discovered-unresolved.md`（E2E-F 续号）。
> - **范围外问题只记账不修**（与 E2E 纪律一致）。
> - stage1 收口时（v0.3 §G.1 阶段一收口契约满足）：未解决项**并入 E2E-F 续号**、
>   本文件改 `status: landed` 迁 `docs/legacy-plans/landed/`（AGENTS.md §七）。
> - Lane O 会话发现的 **E2E 侧**问题不登本表——登 E2E 账本并标注"（发现于 lane-o）"。

## 格式约定

| ID | 发现日期 | 阶段归属 | 描述 | 状态 | 证据 |
|----|----------|----------|------|------|------|

状态取值：`open` / `fixed（PR #）` / `wontfix（理由）` / `migrated（→ E2E-F-xx）`

---

## 账本

（暂无 —— 端侧 stage1 未开工；本文件随第一个 Lane O 会话启用）
