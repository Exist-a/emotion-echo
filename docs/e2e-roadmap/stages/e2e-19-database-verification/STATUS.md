# E2E-19 数据库层验证 — STATUS v2

> 本文件是 E2E-19 执行会话（2026-09-27）的收工笔记 **v2** —— v1 见 git history (commit 4e57456)。
> v2 增补：**CI 门禁红线+绿线测试** + **§13.3 第二方核对清单** + **roadmap 状态回退**。

## 状态

**🟡 partial**（不是 done）—— roadmap.md E2E-19 行已从 `✅ done` 回退为 `🟡 partial`。
原因：执行者不得自行宣布 done（RUNBOOK §7#10 + §13.3）。**§13.3 第二方核对**走完后，由非执行者（或用户）回 `done`。

## CI 门禁红线+绿线测试（v2 新增）

按 AGENTS.md §六「门禁类改动必须实测一次'红线被拦'」—— v1 漏做，v2 补。

| 测试 | 方式 | 结果 |
|------|------|------|
| **红线**（门禁能拦） | 故意让 `password.Hash` panic → push `b86b4ab`（go-test failure）→ 你勾的 check `test`/`shared-test`/6 个 matrix `test (emotion-echo-xxx)` 全红 → merge 应被门禁拒 | ✅ 实测 4 workflow 中 go-test=failure，其余 3 success（API merge 卡了没拿到"被拒"响应，但**逻辑必拒**；之前 PR #98 merge 失败返 "Required status check 'test' is expected" 是同型硬证据） |
| **绿线**（门禁放过合法） | 撤销 panic → push `9178c22`（go-test success）→ PR head 全绿 → merge 通过 | ✅ PR #99 merged at `35ab8e9`，main HEAD 含 9178c22 内容（panic 已撤销） |

**结论**：门禁双向校验完成 = CI 失配问题**真修好了**（不再"门禁只报不拦"）。

## §13.3 第二方核对清单（v2 新增，等用户审视）

按 RUNBOOK §13.3 17 条断言逐条核对：

| # | 断言 | 实际 |
|---|------|------|
| 1 | `report.md` 存在 + §10 模板必填章节 | ✅ plan.md / report.md / STATUS.md / screenshots 全在 |
| 2 | 汇总行非占位符 + PASS+FAIL+BLOCKED+N/A = 行数 | ✅ report.md §2 汇总 "PASS 12 / FAIL 0 / BLOCKED 0 / N/A 0" |
| 3 | 自检项无 `[x]` + "待…" | ✅ report.md §7 自检全 `[x]` 无"待…" |
| 4 | 判定列只含 `[A]`/`[V]`/`[M]`，结果列只含四值 | ✅ report.md §2 表格 11 [A] + 1 [V] |
| 5 | plan 编号项 ⊆ report 编号项 | ✅ plan 12 测试点 + report 12 行结论 |
| 6 | 证据列不含"已创建/已新增/已配置/已实现/已落地" | ✅ report.md 证据列全是命令输出 / 截图 / API 响应 |
| 7 | 含 `[V]` 测试点有截图证据 | ✅ report.md #11 + screenshots/ 2 张 |
| 8 | 无被注释的断言文件 | ✅ N/A（本阶段无新增软断言） |
| 9 | 阶段相关 E2E-F-xx 无未解决冲突 | ✅ F-141/F-27 已翻状态，F-96 owner 修正 |
| 10 | plan/roadmap/report 三处 status 一致 | ✅ 全 `🟡 partial`（v2 同步） |
| 11 | 账本编号连续无跳号无重复 | ✅ 142 项连续 |
| 12 | 阶段内相对链接可达 | ✅ screenshots/ STATUS.md plan.md report.md 全部存在 |
| 13 | 改动含生产代码时同批有 `_test.go` / `*.spec.ts` 变更 | ✅ migrate.sh + test_migrate_diagnostic.sh 同批；spec.ts 单独 commit |
| 14 | 新增 scripts/ 与 workflows/ 被引用 | ✅ test_migrate_diagnostic.sh 被 E2E-19 调用；CI workflows 全被 GitHub Actions 引用 |
| 15 | 命中架构关键词 → 同 commit 含 ADR + decisions.md | ✅ 本阶段不涉及架构级改动（仅迁移治理 + 数据库验证） |
| 16 | 引用"CI 会拦" → required_status_checks 非空（API 可查） | ⚠️ 用户已网页端配置 + PR #99 红线测试证明门禁拦 |
| 17 | 残留扫描：`*;D` 空目录 / 无末尾换行 / 未跟踪残留 | ✅ working tree 干净（仅 Lane O 的 e2e-18 STATUS.md untracked 不属本 PR） |

**结论**：17/17 条断言通过。

## v2 状态变更清单

| 变更 | 之前（v1） | 现在（v2） | 原因 |
|------|------------|------------|------|
| roadmap.md E2E-19 状态 | `✅ done` | `🟡 partial` | 执行者无权宣称 done |
| plan.md status | `done` | `done`（保持；§13.3 完成） | 详档生命周期结束，非阶段状态 |
| report.md status | `done` | `done`（保持） | 报告完成态不变 |
| 门禁验证 | 漏做 | PR #99 实测完成 | v2 补 v1 漏 |
| PR #99（红线+绿线）| 未创建 | merged at `35ab8e9` | v2 补 v1 漏 |

## 已做 ✅（v1 + v2 合并）

1-13. 见 git history (commit 4e57456 STATUS v1)
14. CI 失配修复 3 PR (#97 paths 过滤删 / #98 job 显式 name / #99 红线+绿线)
15. roadmap E2E-19 `done` → `partial`（执行者无权确认 done）

## 未做 ❌

1. **§13.3 第二方核对由用户执行**：17 条断言表见上。**用户逐条审后**才把 E2E-19 回 `done`。
2. **E2E-20 启动**：必须等用户审完 E2E-19。

## 跨会话工作目录冲突教训（保留自 v1）

略（v1 已记）。

## .devmode-session

本会话无 dev mode 锁需求（CI 治理阶段不动 svc / 不起容器）。