# docs/closure-roadmap — 「闭环」阶段（Phase Closure · CL）

> 建立时间：**2026-10-13**。承载"下一大阶段"：**用户实测找问题 + 链路闭环**。
> 接棒 `docs/e2e-roadmap/`（Lane E 收尾中）；纪律与证据口径**继承** E2E，本目录自带精简执行协议（[RUNBOOK.md](RUNBOOK.md)），**不硬依赖**外部目录。
> 取代旧的单文件 `docs/plans/closure-phase-roadmap.md`。

## 目录结构

| 文件 | 用途 |
|------|------|
| [roadmap.md](roadmap.md) | **阶段总表 + 排序路径 + 每阶段"做到什么程度"**（唯一权威，状态在此表） |
| [RUNBOOK.md](RUNBOOK.md) | **执行协议**：状态机 / 环境准备 / 执行循环 / 判定分级 + 证据有效性 / 账本契约 / 收口契约 |
| [parallel-contract.md](parallel-contract.md) | **并行执行契约**：多会话并行时的**轨道 / 文件面隔离** + **占位登记**（**开工前必读、开工时登记占位、收工删除**） |
| [decisions.md](decisions.md) | 本阶段决策记录（**CL-D-xx**） |
| [discovered-unresolved.md](discovered-unresolved.md) | 本阶段发现账本（**CL-F-xx**，范围外问题只记账） |
| `stages/<cl-NN-slug>/` | 每阶段详档 `plan.md` + 执行记录 `report.md`（just-in-time 撰写） |

## 给执行者的入口顺序

1. 读 [roadmap.md](roadmap.md) 顶部「阶段总表」+「排序路径」→ 确认当前该做哪一阶段
2. 读 [RUNBOOK.md](RUNBOOK.md)（执行协议，必读）
3. **（并行时）读 [parallel-contract.md](parallel-contract.md)** → 确认目标轨**无人占用** + **登记占位**
4. 读目标阶段的 `stages/<cl-NN-slug>/plan.md`（如未写，先写；**just-in-time**，不提前批量写）
5. 按 RUNBOOK：开工前置检查 → 起环境 → 六步循环 → 收口契约

## 核心纪律（继承 E2E）

1. **用户实测优先**：每阶段以用户亲手实测发现的问题为输入，不预设"有没有问题"。
2. **范围外问题只记账**（→ [discovered-unresolved.md](discovered-unresolved.md) 的 CL-F-xx），不就地修，防解决顺序混乱。
3. **证据有效性**：结果必须四值 `PASS/FAIL/BLOCKED/N/A` 之一 + **执行输出证据**（命令+输出/退出码/截图）；"已创建/已新增/已配置"**不算证据**；只给文件路径也不算。
4. **`BLOCKED` 超 1/3 不得判 done**；禁止用 `N/A`/`BLOCKED` 掩盖未做。
5. **每阶段收口必须落回归钉**（Playwright / vitest / Go）。
6. 修复遵循 AGENTS.md TDD（Red → Green → Refactor）。
7. **执行者不得自行宣布阶段 done**（过渡期由第二方按 RUNBOOK §收口契约 核对）。
8. 落地计划迁 `docs/legacy-plans/landed/`（迁移须同步修内部 + 入站链接）。

## 与现有体系的关系

| 现有体系 | 关系 |
|---|---|
| `docs/e2e-roadmap/`（Lane E） | 上游：纪律与证据口径的来源；E2E 收尾后本目录接棒 |
| `docs/plans/README.md` | 库存源：本阶段各 CL 的待排期计划都来自这里 |
| `docs/plans/on-device-*`（Lane O） | 独立轨：**CL-10（本阶段 deferred，先不做）** |
| `AGENTS.md §2.4` 数据契约 | 涉及数据写入的阶段（如 CL-03 Kafka 可靠性、CL-07 融合）沿用其契约 |
