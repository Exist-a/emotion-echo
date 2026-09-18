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
| [discovered-unresolved.md](discovered-unresolved.md) | 已发现未解决账本（E2E-F-01~67，含"不列入阶段的候选"评估表） |
| [findings/](findings/) | 建档/阶段预探查的代码级发现全景（带文件行号证据） |
| [stages/](stages/) | 每阶段详档 `plan.md` + 执行记录 `report.md` + 截图 |

## ⚠️ 2026-09-18 状态：路线图被 R 系列阻断

对 E2E-01~06 的独立审查发现**五个已标 `done` 的阶段无一完整满足收口契约**，另有 1 个安全级缺陷（BFF 密保校验 fail-open）+ 2 个功能性破坏 + CI 在 main 持续红却拦不住。
**E2E-07 及以后全部暂缓**，先执行 [remediation.md](remediation.md) 的 R-01 → R-02 → R-03。
根因诊断：**规范只在"应当"层（文本 + 人工闸门），缺少"强制"层（可执行校验 + 真实门禁）**。

## 给执行者（大模型或人）的入口顺序

> **⚠️ 2026-09-18：当前不能直接执行 E2E-07 及以后阶段。** 路线图被 R 系列阻断，见下节「交接说明」。

1. 读 [RUNBOOK.md](RUNBOOK.md)（执行协议，必读；含 §4.1 证据有效性、§7 收口契约 11 项、§13 收口审计）
2. 读 [anti-patterns.md](anti-patterns.md)（**14 类已真实发生的失真反例，收口前必读**）
3. 读 [roadmap.md](roadmap.md) 顶部「⚠️ 当前被阻断」+「R 系列」+「决策门」
4. 读 [remediation.md](remediation.md) 确认当前该做哪一步（R-00 ✅ → **R-01 未完成（#2 网关路由）** → R-02 收尾 → R-03 收尾）
5. 读目标阶段的 `stages/<e2e-NN-slug>/plan.md`
6. 按 RUNBOOK §1 开工前置检查 → §2 起环境 → §3 六步循环 → §7 收口契约 → **§13 审计**

**验收口径**：每个测试点必须给出 `PASS` / `FAIL` / `BLOCKED` / `N/A` 四种合法结果之一并附证据。**证据必须是执行输出（命令+输出/退出码/截图），"已创建/已新增/已配置"不算证据，只给文件路径也不算**（RUNBOOK §4.1）。`BLOCKED` 超过 1/3 不得判 done。禁止用 `N/A`/`BLOCKED` 掩盖未做的工作。

**收口前必须跑审计器**（这是"完成"的判定者，不是执行者自己）：

```bash
python scripts/e2e_stage_audit.py --stage <阶段号>   # 单阶段
python scripts/e2e_stage_audit.py --all              # 全部
python scripts/e2e_stage_audit.py --selftest         # 校验审计器本身可信
```

**执行者不得自行宣布阶段 `done`** —— 必须审计器无 FAIL，且（过渡期）由第二方按 §13.3 核对。

## 交接说明（2026-09-18 第二方核对后更新，给下一个执行 agent）

**当前状态**：R-00 ✅ 完成且自校验通过；**R-01 未完成**（6/7 项实测为真，**#2 不可达**）；R-02 / R-03 为**部分完成**（非"基本完成"）。工作区**干净**（`git status --porcelain` 为空），但 `main` 领先 `origin/main` 1 个 commit 未 push，且 `chore/trigger-ci` 分支残留在本地与远端。

**开工前必须先修的三件（均已在 [remediation.md](remediation.md) 标出，账本编号 E2E-F-60~67）**：

1. **补 APISIX 白名单路由**（阻断级）：`deploy/apisix/seed.sh` 增加 `put_auth_route 116 "/api/v1/auth/verify-security-answer"`，同时把它从 Step 4.5 的 `for drift_id in 116` 漂移清理里移除，并更正 578 行的 route 计数文案。改完重跑 `seed.sh`，再用 curl 实测**未登录 200 / 错答案 401** 两条路径。
2. **补 BFF 层密保负向测试**：负向测试目前只在 user-svc logic 层，原缺陷所在的 `bff/.../auth_handler.go` 一层零测试。
3. **回填账本**：`E2E-F-46/47/48/49/58/59` 磁盘上已修复但账本仍标"未解决" ⇒ 会让审计器 A5 产生假 FAIL（这正是 AP-04 复发）。

**其余待收尾（不阻塞 #1）**：R-02 的 #1~#3（report 模板化 / `[V]` 截图 / 账本对账）与 **SSR 的 ADR + `decisions.md` 登记**（`docs/architecture/adr/` 现存 15 个 ADR 无一条涉及渲染模式）；`-race` 须按 **D-06 的 3 步可复现调查**重做（现结论基于本地 Windows 证据，而 CI 是 Linux，D-06 已明文判定该证据链不成立）；R-03 的 #5 门禁接通（实测 `required_status_checks` 为空集）、#7 脚本负向用例、#10 CI 严格化。**修审计器 A4 的误报再信它的结论**（见 E2E-F-64）。

**不要做的事**：不要跳过上述 #1 直接做 E2E-07 —— 注册虽已恢复可用，但**找回密码链路在网关层仍 401**，E2E-07 正是"密保找回"流程，开工即建在断路上。

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
