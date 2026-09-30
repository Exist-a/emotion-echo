---
purpose: ADR —— 汇总门禁（doc-drift-gate）替代 23 条点名式 required checks
status: accepted
date: 2026-09-30
related-stage: e2e-23
---

# ADR-2026-09 汇总门禁：用聚合 job 替代点名式 required status checks

## 一、背景

2026-09-30 用户为 `main` 启用了分支保护，把 **33 条** job 名逐条填进
"Require status checks to pass before merging"。其中 23 条来自 `doc-drift-check.yml`，
全部是**中文 job 名**（`路径对齐（前端 ⊆ BFF）`、`TDD 门禁`、`E2E 收口审计`…）。

这带来两个问题：

1. **维护成本**：33 条要手工维护，任何一条拼错就失效。
2. **静默失效（更严重）**：GitHub 的 required checks 是**精确字符串匹配**。
   任何人改动 `doc-drift-check.yml` 里某个 job 的 `name:`（哪怕只改一个字），
   那条检查**从此不再被拦**——GitHub 不报错、门禁设置页看起来一切正常、
   CI 依然在跑、只是没人再要求它通过。
   这正是 [`anti-patterns.md`](../../e2e-roadmap/anti-patterns.md) **AP-11「门禁只报不拦」**
   的一个尚未发生的变体：**门禁的覆盖面依赖人肉同步**。

同类问题本阶段已犯过一次并已修：守卫脚本
`test_healthcheck_readiness.sh` 的 compose 段用子串 glob
（`case ... in *"/health/ready"*`），把 URL 改成 `/health/readyXYZ` 仍报 GREEN。
**"看起来在拦"和"真的在拦"是两件事。**

## 二、决策

给 `doc-drift-check.yml` 增加两个 job：

| job | display name | 作用 |
|-----|--------------|------|
| `doc-drift-gate` | `文档守卫总闸` | `needs` 全部 23 个检查 job + `if: ${{ always() }}`；用 `toJSON(needs)` 读出每个上游的 `result`，任一不是 `success` 即 `exit 1` |
| `doc-drift-needs-sync` | `汇总门禁 needs 覆盖校验` | 跑 `scripts/check_doc_drift_gate_needs.sh`，断言 gate 的 `needs` **覆盖全部检查 job**且无失效引用 |

分支保护改为**只填 `文档守卫总闸` 一条**。其余 8 条（go-test 7 + llm/web/e2e-guards）保持不变。

### 三条关键设计约束

1. **`if: ${{ always() }}` 不能省。** 没有它，任一上游失败时 gate 会被
   **skipped**；而 GitHub 里 `skipped` 的 required check 会被当作"通过"，
   反而开出"检查失败反而没人拦"的漏洞。
2. **`skipped` 必须判为不通过。** 脚本里判定条件是 `result != "success"`，
   刻意包含 `skipped` / `cancelled` / 空值。
3. **`doc-drift-needs-sync` 必须独立于 gate 跑。** 若把它也放进 gate 的 `needs`，
   gate 漏掉自己就没人发现 —— **校验汇总门禁的那个 job，不能被汇总门禁覆盖**。
   这条与本阶段 `test_integration_tag_compiles.sh` 里
   "守卫自身的存在必须可被验证"是同一条纪律。

### 新引入的风险与它的处置

硬编码的 `needs` **会漂**：有人新增一个检查 job 却忘了加进 `needs`，
那个检查就永远不进 gate ⇒ 门禁看着生效、实际漏检。

**处置**：`doc-drift-needs-sync` 就是为此存在，且它同时查两侧——
① 有 job 不在 `needs` 里；② `needs` 里有 workflow 中已不存在的 job
（后者会让 GitHub 直接判定 workflow 无效）。脚本另有一条"反向健全性"：
`needs` 少于 20 项即判红，拦截"被误改成空壳门禁"。

**该脚本的负向对照已实测**：
- 从 `needs` 漏掉一个 job → `rc=1`，报出漏掉的 job 名
- `needs` 里留一个已删的 `ghost-job` → `rc=1`，报出幽灵引用
- 还原 → `rc=0`

## 三、后果

**正面**

- 分支保护从 33 条降到 **9 条**（gate 1 + go-test 7 + llm 1... 实为 10 条，见下）
- 新增检查只要被 gate 覆盖就**自动纳入**，不需要回来改设置
- 改 job 的 `name:` 不再使门禁静默失效（gate 的名字固定，不依赖被检查 job 的名字）
- 门禁覆盖面本身有了机器校验（`doc-drift-needs-sync`）

**负面 / 代价**

- 多一个 job 的排队时间（但它只是等已有的 23 个并行 job，**不增加墙上时间**）
- 汇总 job 里的 `toJSON(needs)` 逻辑**本地无法完整验证**（GitHub 表达式只能在 CI 求值）。
  缓解：脚本体本身抽出来在本地实跑过三种输入（全成功 / 有失败 / 有跳过），
  且用 `env:` 传值而非在 shell 里插 `${{ }}`，避开了多层花括号转义。
- 一次性迁移成本：分支保护里要**删掉 23 条中文项、加上 1 条 `文档守卫总闸`**。
  漏删不会造成风险（多几条 required 而已），**漏加才是风险**。

## 四、关联

- [anti-patterns.md AP-11](../../e2e-roadmap/anti-patterns.md) —— 门禁只报不拦
- [RUNBOOK.md](../../e2e-roadmap/RUNBOOK.md) §13.3 收口契约
- 账本 [E2E-F-162](../../e2e-roadmap/discovered-unresolved.md) —— 门禁接线
- [docs/ci-workflows/README.md](../../ci-workflows/README.md) —— CI 现状说明
- E2E-23 report §9.9

## 五、调研依据

- 分支保护规则 ID `83308753`，`/settings/branch_protection_rules/83308753`
  （**经典保护**，非 Rulesets —— 这两者是不同的 API 面）
- `.github/workflows/doc-drift-check.yml` 23 个 job 的 `name:` 实际取值
- Actions API 实测 `e2e-guards` run `4efec2b` = `success`（证明 ubuntu runner 预装 helm）
- `scripts/check_doc_drift_gate_needs.sh` 的两组负向对照输出
