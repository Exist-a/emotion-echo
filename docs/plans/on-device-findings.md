---
purpose: 端侧化 stage1（Lane O）期间发现的问题账本（OND-F-xx 续号）
date: 2026-09-24
status: active
scope: docs/_meta/parallel-tracks.md §三.资源3 —— 仅 Lane O 会话写入
last-refresh: 2026-09-24（T1 收口新增 OND-F-02）
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

| ID | 发现日期 | 阶段归属 | 描述 | 状态 | 证据 |
|----|----------|----------|------|------|------|
| OND-F-01 | 2026-09-24 | 阶段一（T0 收口） | `scripts/on-device-golden/` 的 13 条 pytest **不在任何 CI workflow 覆盖内**：`llm-test.yml` paths 不含 `scripts/` 且只跑 `tests/unit/`；`go-test`/`doc-drift` 虽必跑但不执行 Python 测试。golden set 回归（OC-11）当前唯一执行点 = 本地手工 pytest | open（修法候选：llm-test.yml 扩 paths + 加 `python -m pytest scripts/on-device-golden/` step——**属共享文件 `.github/workflows`，须协议 §六握手 + 与 Lane E 协调 PR 时序**，不擅动） | 4 workflow paths/grep 实测（STATUS.md §三 CI 覆盖现状表） |
| OND-F-02 | 2026-09-24 | 阶段一（T1 收口） | `scripts/on-device-perf/` 24 条 pytest **与 OND-F-01 同型 CI 缺口**：`llm-test.yml` paths 不含 `scripts/on-device-perf/`；性能基线回归（OC-11~13）当前唯一执行点 = 本地手工 pytest。**两缺口合并修**：llm-test.yml paths 加 `scripts/on-device-*/` 后两条均解决——但 `.github/workflows` 属共享列，须协议 §六握手 | open（与 OND-F-01 同修法候选；不擅动） | pytest 24/24 本地 PASS；4 workflow paths/grep 实测（STATUS.md §三 CI 覆盖现状表） |
