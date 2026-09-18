# docs/e2e-roadmap — 长期阶段式 E2E 测试路线图

> 建立时间：2026-09-17。本目录承载"按功能块分阶段、由浅入深"的长期 E2E 测试治理。
> 每阶段流程见全局 skill `e2e-stage-testing`（用户级 `~/.agents/skills/e2e-stage-testing/SKILL.md`）。

## 目录结构

| 文件 | 用途 |
|------|------|
| [RUNBOOK.md](RUNBOOK.md) | **执行协议（执行层单一事实源）**：状态机/环境准备/执行循环/判定分级+**证据有效性**/账本契约/收口契约/升级协议/report 模板/护栏/命令速查/**§13 收口审计（机器校验）** |
| [anti-patterns.md](anti-patterns.md) | **反例集（警告）**：14 类已真实发生的失真模式，附证据与可机械检查的硬规则。**收口前必读** |
| [remediation.md](remediation.md) | **补救排期**：R-01 阻断修复 / R-02 收口补账 / R-03 约束机制。当前激活 |
| [roadmap.md](roadmap.md) | 30 阶段总表（🔧 改造阶段）+ R 系列 + 改造决议状态 + 依赖声明 + 决策门 |
| [decisions.md](decisions.md) | 改造项决议记录（D-01 密保问题 / D-02 两种量表并存 / D-03 真口型同步 / D-04 i18n 候选） |
| [discovered-unresolved.md](discovered-unresolved.md) | 已发现未解决账本（E2E-F-01~59，含"不列入阶段的候选"评估表） |
| [findings/](findings/) | 建档/阶段预探查的代码级发现全景（带文件行号证据） |
| [stages/](stages/) | 每阶段详档 `plan.md` + 执行记录 `report.md` + 截图 |

## ⚠️ 2026-09-18 状态：路线图被 R 系列阻断

对 E2E-01~06 的独立审查发现**五个已标 `done` 的阶段无一完整满足收口契约**，另有 1 个安全级缺陷（BFF 密保校验 fail-open）+ 2 个功能性破坏 + CI 在 main 持续红却拦不住。
**E2E-07 及以后全部暂缓**，先执行 [remediation.md](remediation.md) 的 R-01 → R-02 → R-03。
根因诊断：**规范只在"应当"层（文本 + 人工闸门），缺少"强制"层（可执行校验 + 真实门禁）**。

## 给执行者（大模型或人）的入口顺序

1. 读 [RUNBOOK.md](RUNBOOK.md)（执行协议，必读）
2. 读 [roadmap.md](roadmap.md) 顶部「当前激活阶段」与「决策门」
3. 读该阶段 `stages/<e2e-NN-slug>/plan.md`
4. 按 RUNBOOK §1 三项目开工前置检查 → §2 起环境 → §3 六步循环 → §7 收口契约

**验收口径**：每个测试点必须给出 `PASS` / `FAIL` / `BLOCKED` / `N/A` 四种合法结果之一并附证据（`[A]` 断言输出、`[V]` 截图、`[M]` 升级给用户）。`BLOCKED` 超过 1/3 不得判 done。禁止用 `N/A`/`BLOCKED` 掩盖未做的工作。

## 阶段详档（just-in-time）

详档写在 `stages/<e2e-NN-slug>/plan.md`，**在轮到该阶段前 1 个阶段时撰写**，不提前批量写完全部 30 份。
原因：本项目长期受"文档与代码漂移"之害（ADR-18），提前写出的详档会随前面阶段的发现失效，反而制造新失真。

| 阶段 | 详档 | 状态 |
|------|------|------|
| E2E-01 登录会话持久化 | [plan.md](stages/e2e-01-login-session/plan.md) | ✅ 已写 |
| E2E-02 项目目录清理 | [plan.md](stages/e2e-02-directory-cleanup/plan.md) | ✅ 已写 |
| E2E-03 CI/CD 门槛 | [plan.md](stages/e2e-03-ci-gate/plan.md) | ✅ 已写 |
| E2E-04 前端工程化门槛 | [plan.md](stages/e2e-04-frontend-engineering/plan.md) | ✅ 已写 |
| E2E-05 文档与代码一致性 | [plan.md](stages/e2e-05-doc-code-consistency/plan.md) | ✅ 已写 |
| E2E-06 数据库改造 | [plan.md](stages/e2e-06-db-transformation/plan.md) | ✅ 已写 |
| E2E-07 找回/重置密码 | [plan.md](stages/e2e-07-password-recovery/plan.md) | ✅ 已写 |
| E2E-08 历史会话管理 | [plan.md](stages/e2e-08-conversation-management/plan.md) | ✅ 已写 |
| E2E-09 注册流程 | [plan.md](stages/e2e-09-registration/plan.md) | ✅ 已写 |
| E2E-10 ~ E2E-30 | — | ⏳ 轮到前补写 |

模板见 [stages/_TEMPLATE.md](stages/_TEMPLATE.md)。

### 已有 findings

- [2026-09-17-pretest-panorama.md](findings/2026-09-17-pretest-panorama.md) — 建档预探查：① 人格测试→AI 提示词链路（结论：不存在）② 数字人（结论：假口型未同步）③ 横切模块（缓存/数据库/日志/监控/健康检查）④ user 页图表现状 ⑤ 目录与数据库字段清理候选

## 与现有体系的关系

| 现有体系 | 关系 |
|---------|------|
| docs/plans/known-issues-backlog-runtime-bugs-2026-09-17.md（R-xx 编号） | 本目录账本独立追踪"E2E 阶段中发现"的问题；确认为运行时 bug 且修复落地后回填 R 系 |
| docs/plans/test-coverage-tracker-2026-09-16.md | 业务路径覆盖追踪的前身，其 Sprint 111-115 排期已被本 roadmap 吸收对齐 |
| AGENTS.md §2.4 数据契约 | E2E-30 收口阶段目标 = 六项契约全绿 |
| ADR-18（文档失真治理） | E2E-05 是它的落地接通：10 个校验脚本接入 CI |
| docs/stages/ | 阶段收口记录最终按现有生命周期迁移到 docs/stages/ |

## 核心纪律

1. 每阶段开始必须先做 IAB 内置浏览器实测，不预设"有没有问题"
2. 范围外问题只记入账本不修，防止解决顺序混乱
3. 每阶段收口必须写 Playwright 回归钉
4. 修复遵循 AGENTS.md TDD 全流程（Red → Green → Refactor）
5. 详档 just-in-time 撰写，避免制造新的文档失真
