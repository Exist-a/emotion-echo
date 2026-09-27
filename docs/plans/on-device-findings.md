---
purpose: 端侧化 stage1（Lane O）期间发现的问题账本（OND-F-xx 续号）
date: 2026-09-24
status: active
scope: docs/_meta/parallel-tracks.md §三.资源3 —— 仅 Lane O 会话写入
last-refresh: 2026-09-27（T2#6 收口新增 OND-F-07）
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
| OND-F-04 | 2026-09-27 | 阶段一（T2#4 收口） | T2#4 云端基线跑分 = `scripts/on-device-baseline/` 14 条 pytest **与 OND-F-01/02 同型 CI 缺口**：`llm-test.yml` paths 不含 `scripts/on-device-baseline/`；T2#4 baseline 回归当前唯一执行点 = 本地手工 pytest。**三缺口合并修**：llm-test.yml paths 加 `scripts/on-device-*/`（**涵盖 baseline + perf + golden**）后三条均解决——但 `.github/workflows` 属共享列，须协议 §六握手 | open（三合一修法候选 + OND-F-01/02 合并候选；不擅动） | pytest 14/14 本地 PASS；4 workflow paths/grep 实测（STATUS.md §三 CI 覆盖现状表） |
| OND-F-05 | 2026-09-27 | 阶段一（T2#4 收口） | baseline 实跑 N=7 与 D-26.2 ADR §五 "13 用例"目标有 6 用例差距。**扩 case 不在 Lane O T2#4 决议权**（golden set 编排属另一份 TDD + 决策权属用户 §十二 决策 2 拍板后），T3+ 真机基线复核时同步扩 case | open（T3+ 真机基线复核时同步解决；不在 T2#4 决策权） | ADR D-26.2 §五"13 用例"目标 vs `golden_set.jsonl` 实际 N=7（`docs/plans/on-device-baseline-report-2026-09-24.md` §二 路径说明） |
| OND-F-06 | 2026-09-27 | 阶段二（T2#5 收口） | `createDynamicEngine()` 仅验证 dynamic import 链路通（架构就绪），**未**真创建 MLCEngine（避免 ~1GB 权重下载）。**T3 IAB 验证时**需借 dev mode 半天窗口 + 真实 GPU：① 调用 `engine.chat.completions.create({ stream: true })` 接真引擎流式；② `pnpm build` 后 grep dist/ 验证 dynamic chunk 实际大小；③ §十二决策 1 拍板后选 worker 入口 / vite external / CDN 任一隔离策略 | open（T3 任务；不在 T2#5 决议权） | `webllmEngine.dynamicImport.test.ts` 17 用例架构契约 PASS；chat() 抛 `Real chat() not wired in T2·Lane O. T3 will wire ...` |
| OND-F-07 | 2026-09-27 | 阶段二（T2#6 收口） | §十二 5 项决策材料已交付（PR #95 7aa8eed），**但用户拍板权属用户，Lane O 不擅自决议**。**未拍板前阶段二（v0.3 §C.3 混合架构开发）不可开工**（v0.3 §A.3 + §B.1 强约束）。T3 期间：① 借 dev mode 半天窗口做 IAB 验证；② 决策拍板后 Lane O 立 D-26.1/3/4/5 ADR + D-26 accepted 状态流转；③ §十二任一项未拍板前不进阶段二 | open（拍板权属用户；不在 Lane O 决议权） | `docs/plans/on-device-decision-pack.md` §六 拍板请求 + §七 给下次 Lane O 开场动作 |
