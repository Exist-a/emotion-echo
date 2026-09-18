---
status: active
priority: critical
created: 2026-09-18
type: e2e-remediation-plan
---

# 补救排期与执行路径（R 系列）

> **本文档性质：排期与规划，不包含任何已执行的修复。**
> 触发：2026-09-18 对 E2E-01~06 的独立审查。审查结论——**五个已标 `done` 的阶段无一完整满足 [RUNBOOK.md](RUNBOOK.md) §7 收口契约**，另有 1 个安全级缺陷、2 个功能性破坏、1 个持续红的 CI。
> 失真模式已固化为 [anti-patterns.md](anti-patterns.md)（14 类，带证据）。发现逐条登记在 [discovered-unresolved.md](discovered-unresolved.md) 的 `E2E-F-41`~`56`。

## 为什么单独开 R 系列而不改 E2E-NN 编号

- 这些问题**不是新功能工作**，是**既有阶段的欠账**；用独立编号可清楚区分"欠债归还"与"路线图推进"
- 不重排 30 个阶段编号（本项目已因重排两次踩过交叉引用失真的坑）
- R 系列明确「**插入前置**」：未完成 R-01，不得启动 E2E-07 及以后任何阶段

---

## 问题分类

| 类 | 含义 | 数量 | 归属 |
|----|------|------|------|
| **A. 阻断** | 安全漏洞 / 功能不可用 / 提交即 CI 必红 | 6 | **R-01** |
| **B. 契约欠账** | 阶段标 done 但收口契约未满足（报告/截图/账本/状态） | 8 | **R-02** |
| **C. 机制缺失** | 无机械校验、无对账、门禁只报不拦 → 导致 A/B 反复发生 | — | **R-03** |

---

## R-01 🔴 阻断项修复（最高优先）

**目标**：消除安全漏洞与功能性破坏，让 `main` 的 CI 恢复可绿状态。

**依赖**：无 → **可立即启动**
**阻塞**：E2E-07 及以后全部阶段；E2E-06 的 `done` 状态有效性

### 范围

| # | 项 | 账本 | 关键位置 |
|---|----|------|---------|
| 1 | **BFF 密保校验 fail-open**：`Login(username, "dummy")` 当用户存在性探测，永远走 `err != nil` → 恒返回 `success:true`，答案从不校验 | E2E-F-41 | `bff/internal/handler/auth_handler.go:478-481` |
| 2 | **找回密码端点不存在**：BFF HTTP 客户端调 `/api/v1/users/verify-security-answer`，user-svc 无此路由（`main.go:152-168`），HTTP 路径 404 | E2E-F-41 | `bff/internal/downstream/user.go:214` |
| 3 | **注册链路断裂**：后端强制 1~2 个密保，前端仍只发 `{username,password,verificationCode}` → 唯一注册入口 100% 400 | E2E-F-42 | `bff/.../auth_handler.go:172-180`、`authlogic.go:96-102`、`web/app/pages/login/index.vue:147` |
| 4 | **测试未同步 → 编译失败**：26 个改动文件零 `_test.go` 变更；`go vet ./...` 在 user-svc 报 4 个 `unknown field Phone`；BFF avatar 测试的 fake 不满足新 `UserClient` 接口 | E2E-F-43 | `user-svc/internal/{model,repository,logic,handler}/*_test.go`、`bff/internal/handler/avatar_handler_test.go:59` |
| 5 | **Register 非事务**：先建用户再存密保，后者失败则用户行已落库 → 库里留下"无密保用户" | E2E-F-44 | `user-svc/internal/logic/authlogic.go:119-146` |
| 6 | **迁移 checksum 校验是死代码**：返回码 2 被吞掉，`ON CONFLICT DO UPDATE SET checksum` 覆盖新校验和 → "迁移文件被改动"被静默放行 | E2E-F-45 | `deploy/db/migrate.sh:164-173` |
| 7 | **CI 在 main 持续红**：`doc-drift-check` 3 个 job 失败（env 变量 8 项未文档化 / 6 个占位 digest / migration 顺序 Fail 13） | E2E-F-50 | run on `2cb9e58`、`0644988` |

### 执行路径（顺序不可调换）

```
① 测试编译修复（#4）
     └─ 先让 go vet ./... 在 user-svc / web-bff 通过
     └─ 这是 TDD 补课的第一步：删字段的断言要同步删、改签名要同步改
② 功能修复（#1 #2 #3 #5）
     └─ 密保校验改为真实校验路径：#2 的端点缺失必须先解决（要么补 HTTP 路由，要么让 BFF 只走 gRPC）
     └─ 注册：前端补密保录入 或 把强制校验降级留给 E2E-09（二者择一，需用户决定）
     └─ Register 包进事务
③ 迁移脚本正确性（#6）
     └─ 显式判 `-eq 2` → die；并给 `deploy/db/test_migrations_contract.sh` 加一条 schema_migrations 断言
④ 让 CI 转绿（#7）
     └─ 先跑本地基线，把 3 个 job 的失败逐项定性：是脚本错 / 数据错 / 文档错
     └─ 定性后分别修（注意：不能给 job 加 continue-on-error 来"变绿"——那是 AP-11）
⑤ 复跑 CI 断言全绿 + 一次故意红线验证门禁
```

### 验收标准（DoD）

- [ ] `go vet ./...` 在 7 个 Go 模块全部通过（含 `_test.go` 编译）
- [ ] `pnpm typecheck` 0 errors、`pnpm test` 全绿
- [ ] 密保校验**真的有校验**：错误答案必须被拒（负向测试先行）
- [ ] 注册流程端到端可用（或已明确降级 + 用户批准）
- [ ] migrate.sh 校验和不符**必须报错**（负向用例）
- [ ] main 上 `go-test` / `web-test` / `llm-test` / `doc-drift-check` **全部绿**
- [ ] 所有修复走 TDD：先写失败测试（`test:` commit）→ 实现（`fix:` commit）
- [ ] 每个修复项在 report 里有对应行；发现登记账本

### 边界（不做）

- 不做 CI 严格化的 14 项未完成项（归 E2E-03 阶段 2 / R-03）
- 不做 E2E-07 的找回密码流程改造（R-01 只保证"校验是真的"）
- 不补历史阶段的报告（归 R-02）
- 不顺带重构 user-svc 其它模块

---

## R-02 🔧 收口补账（契约补齐）

**目标**：让 E2E-01~06 的状态**诚实**——要么补齐契约，要么把状态降为 `partial`。

**依赖**：R-01（补账要基于事实，而事实现在是错的）
**阻塞**：R-03（先知道要审计什么）

### 范围

| # | 项 | 涉及阶段 | 账本 |
|---|----|---------|------|
| 1 | **report 按 `_REPORT_TEMPLATE.md` 重写**：E2E-03（非模板、无环境基线、无汇总、无判定列）、E2E-06（缺环境基线/发现分类/修复清单/回归钉四节） | E2E-03 / E2E-06 | E2E-F-53 |
| 2 | **补齐 `[V]` 测试点截图**：E2E-01/03/04/05 的 screenshots 目录为空或缺失 | E2E-01/03/04/05 | E2E-F-53 |
| 3 | **账本对账**：E2E-F-21/22/23/24/30 全部仍挂 🔴 未解决，而其所属阶段已标 done → 逐条翻状态或在 report 写明理由 | E2E-03/04/05 | E2E-F-54 |
| 4 | **状态三处对齐**：E2E-05 `plan.md` 至今 `status: pending`；roadmap/report/plan 三处须一致 | E2E-05 等 | E2E-F-57 |
| 5 | **撤下 E2E-06 的 `done`**：`report.md:169` 的 `[ ] 补 integration test` 未勾选而标 done（违反状态机）→ 改 `partial` 或补齐 | E2E-06 | E2E-F-53 |
| 6 | **更正 SSR 失效文档 + 补 ADR**：7+ 处仍写"项目是 SPA"（含 `e2e-04/plan.md:38`「不引入 SSR」这句与 `done` 状态并存）；`docs/architecture/adr/` 与 `decisions.md` 均无渲染模式条目 | 全局 | E2E-F-52 |
| 7 | **修坏链与占位符**：`roadmap.md:13` 的 `e2e-07-forgot-password` → 实际 `e2e-07-password-recovery`；E2E-05 report 汇总行 `PASS x` 占位符 | — | E2E-F-57 |
| 8 | **孤儿产出物归置**：`06-create-schema-migrations.sql`（永不执行）删除或改为真正被执行的路径；`e2e/helpers/auth.ts`（无引用）接入或删；`.git-blame-ignore-revs` 移到仓库根 | E2E-04/06 | E2E-F-48 / E2E-F-58 |
| 9 | **演示账号与 initdb 解耦**：`03-seed-default-users.sql` 仍硬编码 `echo/echo123`，而 cleanup 默认目标正是 `echo` → "可删"不成立。改为"契约账号由 initdb 负责、不属可删演示账号"或改用其它用户名 | E2E-06 | E2E-F-47 |
| 10 | **清理脚本补全**：`cleanup-demo-account.sh` 漏 6 张含 `user_id` 的表；DB 不可达时须非零退出（现伪装成功） | E2E-06 | E2E-F-47 |
| 11 | **死字段从权威 DDL 移除**：`02-create-tables-in-schemas.sql:10,11,17` 仍有 `phone`/`email`/`status`，与 `chat-svc/migrations/008_p0r27_ddl_drift_test.go` 的"表定义唯一源=02"契约冲突 | E2E-06 | E2E-F-46 |
| 12 | **同类污染复发清理**：`deploy/db/migrate.sh;D/` 空目录（E2E-F-18 同类问题） | — | E2E-F-59 |
| 13 | **`main.go` 注释与代码相反**：注释仍写"Stage 77 退避重试"，代码已换单次连接 → 要么恢复重试，要么改注释 | E2E-06 | E2E-F-49 |
| 14 | **E2E-04 假 PASS 更正**：4 个测试点（typecheck 进 CI / build smoke / mobile project / a11y 基线）的证据重填为真实状态，或降为未完成 | E2E-04 | E2E-F-51 |
| 15 | **`-race` 状态改判**：`E2E-F-40` 把"移除 `-race`"标为已解决 → 改为 `🟡 降级并记录`（含放弃理由、批准人、残留风险），或在 R-01 后真正加回 | E2E-03 | E2E-F-55 |

### 验收标准（DoD）

- [ ] 15 项逐条有结论（补齐 / 降级 / 转阶段），**不允许"已知不改"而无记录**
- [ ] E2E-01~06 的状态与账本、report、plan 三处完全一致
- [ ] 存在未解决账本条目的阶段**不得标 `done`**
- [ ] 所有 `[V]` 测试点有截图（或明确降级为 `[A]` 并说明）
- [ ] SSR 有 ADR 且 `decisions.md` 有登记；7 处失效文档全部更正

### 边界（不做）

- 不重新执行 E2E-01~05 的实测（只补证据与状态）
- 不做 R-01 范围外的功能修复

---

## R-03 🔧 约束机制建设（防复发）

**目标**：把 [anti-patterns.md](anti-patterns.md) 的 14 条硬规则**机械化**，让"契约未满足"能被机器发现，而不是靠第二方审查（人工审查不可持续，下一个 25 阶段不会每次都有人审）。

**依赖**：R-02（先补账，才知道审计该抓什么、基线在哪）

### 范围

| # | 机制 | 反例 | 具体做法 |
|---|------|------|---------|
| 1 | **`scripts/e2e_stage_audit.py`** —— 阶段收口审计器 | 全部 | 输入阶段号，机械校验：report 存在且含必填章节；汇总行非占位符且与表格行数一致；自检项无 `[x] + "待…"`；`[V]` 点有截图；plan 编号集合 ⊆ report 编号集合；账本编号连续；账本状态与 roadmap 状态无交集冲突（AP-04）；plan/roadmap/report 三处 status 一致；相对链接可达 |
| 2 | **证据有效性校验** | AP-01/02 | 报告"证据"列禁止出现"已创建/已新增/已配置/已实现"；`[A]` 项必须含可复现命令或输出片段；检出 `soft-assert`（被注释的断言）告警 |
| 3 | **TDD 门禁** | AP-09 | 改动含生产代码路径时，校验同 commit 是否含 `_test.go`/`*.spec.ts`；CI 必须跑 `go vet ./...`（编译测试）而非仅 `go build` |
| 4 | **ADR 门禁** | AP-08 | 维护"架构关键词清单"（渲染模式/框架/存储/协议/认证）；改动命中时校验 commit 是否含 ADR 文件 + `decisions.md` 变更 |
| 5 | **门禁真的能拦** | AP-11 | 为 `required_status_checks` 接通审计 job；**先解决 `enforce_admins: true` 下"check 不上报即锁死"的风险**（建议：审计 job 设为必填、其余为报告；或阶段性放宽 enforce_admins）。要求实测一次"红线 PR 被拒" |
| 6 | **孤儿产出物检测** | AP-10 | 新增 `scripts/*` 与 `.github/workflows/*` 必须被引用；新增 helper 必须有引用方 |
| 7 | **脚本负向用例要求** | AP-13 | 迁移/清理/种子类脚本必须附负向测试（校验和不符报错 / 依赖不可达非零退出 / 清理范围以"所有含 user_id 表"为下限自动核对） |
| 8 | **残留物扫描** | AP-14 | 扫空目录（如 `*;D`）、无末尾换行文件、`git status` 之外未跟踪的残留 |
| 9 | **反例集回填** | 全部 | 审计器上线后，把 [anti-patterns.md](anti-patterns.md) 每条标注"已机械化 / 仍靠人工 + 理由" |
| 10 | **CI 严格化剩余 14 项** | — | 见下面「与既有阶段的衔接」 |

### 验收标准（DoD）

- [ ] `e2e_stage_audit.py` 可对任一历史阶段运行并**准确报出**已知缺口（用 E2E-03/04/05 做回归样本，应能复现本次审查的结论）
- [ ] 审计器接入 CI 且**实测能拦住**（红 PR 被拒）
- [ ] 14 类反例逐条标注机械化状态
- [ ] `anti-patterns.md` 的每条硬规则都能对应到一条检查项或明确的人工理由

### 边界（不做）

- 不追求"零人工"——`[M]` 类判定与产品决策仍需人
- 不做通用代码质量平台（只做 E2E 收口相关）

---

## 与既有阶段的衔接

| 既有项 | 处置 |
|--------|------|
| **E2E-03 阶段 2（CI 严格化）剩余 14 项** | 未修 14 条：A3(`-race`)/A4(覆盖率)/A5(集成测试)/A6(格式)/B2(typecheck)/B3(Playwright 进 CI)/B4(构建验证)/B5(engines+packageManager)/C1(锁版本)/C2(models pytest)/C3(pytest 覆盖率)/D2(README 措辞)/D3(secret 扫描)/D4(其余测试套件)。归 **R-03 第 10 项**统一收口 |
| **`doc-drift-check.yml` 自身缺加固** | 11 个 job 全无 `timeout-minutes`/`concurrency`/`permissions`（正是刚被记为缺陷的 A7/A8/A9）→ 归 R-03 |
| **E2E-04 未完成项** | 4 个假 PASS 对应的真实工作（build smoke 真跑、mobile project 真跑、a11y 真跑）→ 归 R-02 补账 + 必要时重执行 |
| **E2E-06 状态** | `done` → `partial`（待 R-02 第 5 项处理） |
| **E2E-07 及以后** | **阻塞**，R-01 完成前不得启动 |

---

## 执行路径总览

```
R-01 阻断修复（安全 + 功能 + 编译 + CI 转绿）
  │   出口条件：4 个 workflow 全绿；密保校验有负向测试；测试可编译
  ▼
R-02 收口补账（15 项契约补齐 / 状态诚实化）
  │   出口条件：E2E-01~06 状态与账本一致；截图齐；SSR 有 ADR
  ▼
R-03 约束机制（审计器 + 门禁 + 9 项机制）
  │   出口条件：审计器能复现本次审查结论；门禁实测能拦
  ▼
恢复路线图：E2E-07 找回密码 → … → E2E-30
```

**R-01 必须在任何新阶段之前。** R-02 与 R-03 可与路线图后续阶段并行（但要先于"下一批阶段的收口"）。

---

## 待用户决策（阻塞开工）

| 决策 | 影响 | 选项 |
|------|------|------|
| **注册页密保录入的归属** | R-01 #3 的修法 | ① 在 R-01 内补密保录入 UI（把 E2E-09 的一部分提前）② 把后端强制校验降级为可选，留给 E2E-09 做强校验 |
| **`-race` 的最终处置** | R-02 #15 | ① 真正加回（需先查明 7 模块 exit 1 的真实原因）② 批准永久降级并记录残留风险 |
| **`doc-drift-check` 的当前红态处置** | R-01 #7 / R-03 #5 | ① 先修 3 个 fail 项让它绿 ② 暂设 `continue-on-error` 降为 WARN（须明示这是降级，见 AP-11） |
| **`enforce_admins: true` 下 required_status_checks 的锁死风险** | R-03 #5 | ① 保持 `true` 并确保 check 稳定上报 ② 阶段性放宽为 `false`（则门禁对管理员无效） |
