---
stage: e2e-05
title: 文档与代码一致性收口
type: transformation
status: pending
created: 2026-09-17
depends-on: [e2e-02, e2e-03]
blocks: [e2e-30]
related-findings: [E2E-F-20, E2E-F-21, E2E-F-22, E2E-F-23]
---

# E2E-05 🔧 文档与代码一致性收口

## 1. 阶段目标

本项目长期受"文档说已落地、代码实际未落地"之害（ADR-18 专为此设立），已有 **10 个校验脚本**却**全部无人在跑**——因为 CI 不存在（E2E-03 才补上）。本阶段把这些防线真正接通，并更正本次排查实测出的 4 处新失真。

> **本阶段是自指的**：本路线图本身就是文档，其阶段边界/依赖判断若建立在失真的旧文档上就会连锁出错。这 10 个脚本是唯一能自动发现此类问题的手段。

## 2. 范围与边界

### 做 1：把 10 个校验脚本接入 E2E-03 的 CI

| # | 脚本 | 校验对象 |
|---|------|---------|
| 1 | `scripts/check_routes_alignment.sh` | 前端 `apiRoutes.ts` ⊆ BFF `main_test.go` 的 `wantRoutes`（三方路径对齐） |
| 2 | `scripts/check_view_consistency.py` | 同名 SQL 视图跨 3 文件定义 diff（`deploy/db/04-create-views.sql` vs `analytics-svc/migrations/a001` vs `ai-svc/migrations/i005`）。文档字符串写着"CI 阶段跑：发现 diff 即 fail"——**该前提从未成立** |
| 3 | `scripts/lint_env_vars.sh` | `apps.yml` 的 `${VAR}` ⊆ `.env.local.example` |
| 4 | `scripts/yaml_lint.py` | 禁止 `${VAR:-literal}` 字面默认值（"dev 拼通的 prod 配置"漂移） |
| 5 | `scripts/check_git_layout.py` | 仓库布局 7 项断言 |
| 6 | `scripts/check_docker_digests.sh` | FROM 行 digest pin（**有假绿问题，见做 2**） |
| 7 | `scripts/test_docs_update.sh` | `deploy/configuration.md` / `QUICKSTART.md` 内容断言（8 项） |
| 8 | `scripts/test_migrations_contract.sh` + `test_migrations_no_service_order.sh` | 迁移契约 |
| 9 | `scripts/test_user_oauth_zero_ref.sh` | 验证"oauth 已删"的 ADR 结论在代码中零引用 |
| 10 | `scripts/test_bff_jwt_secret.sh` | BFF 与 APISIX seed 的 JWT secret 一致 |

### 做 2：修 `check_docker_digests.sh` 的假绿

**问题**：该脚本只断言 `FROM` 行的**格式**含 `@sha256:`，而 `Dockerfile.digests.lock` 中 **7 个 digest 是 `sha256:000...000` 占位值**（文件头 `:20-22` 自述），因此**形式通过、实质为空**——检查器给了假绿，比文档失真更隐蔽。

**目标**：脚本增加断言"digest 非全零/占位值"；若确实无法回填（沙箱网络不可达 docker.io），则在锁文件与 CI 输出中**显式标注为已知缺口**而非静默通过。

### 做 3：更正 4 处实测失真

| # | 出处 | 文档写的 | 实测事实 |
|---|------|---------|---------|
| 1 | `docs/stages/stage-21-k8s-strategy.md:27` | "Secret 泄露 · `deploy/tls/*.key` **提交进 git**" | `git ls-files deploy/tls/` 为空；`.gitignore:125-126` 已忽略 `*.key`/`*.crt`。**未提交** |
| 2 | `docs/ci-workflows/web-test.yml` 步骤 3 | 名为 `lint`，隐含 AGENTS.md §2.2 的 `npm run lint` 门槛成立 | `package.json` 无 `lint` script、devDependencies 无 eslint（E2E-04 落地后才成立） |
| 3 | `deploy/env/.env.common` | `APISIX_VERSION=3.9.0-debian`；自称"后续演进版本统一来源" | 实际 APISIX 3.18.0；`GIN_BACKEND_HOST` 指向已迁入 `legacy/` 的 Gin；compose 不引用。**已腐烂未被发现**——恰因 `lint_env_vars.sh` 只校验 `.env.local.example` 不校验它 |
| 4 | `docs/stages/stage-21-k8s-strategy.md` 等 | （同上 #1，同类"未复跑即记录"失真） | — |

### 做 4：把 ADR-18 的失真登记从"人工"改为"脚本 fail 即强制登记"

### 不做（边界）

- 不做通用"文档声称 X，代码是否为 Y"检查器（成本过高，10 个定制钉已覆盖主要历史事故）
- 不追查全部历史文档（只修本次实测发现的 4 处 + CI 能自动抓的）
- 不重建 ADR-18 治理体系（只接通与补齐）

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| E2E-03（CI 存在，脚本才有地方挂） | ⏳ **强依赖** |
| E2E-02（`check_git_layout.py` 的断言结果受目录清理影响） | ⏳ |
| 10 个脚本本地逐个跑通，确认各自当前是否绿 | 需先做基线 |

## 4. 测试点清单

| # | 测试点 | 验证方式 | 证据 | 结果 |
|---|--------|---------|------|------|
| 1 | 10 个脚本本地基线盘点 | 逐个跑，记录退出码与输出 | 清单 | ⬜ |
| 2 | 全部接入 CI 并在 push 时执行 | CI 日志出现各脚本步骤 | run URL | ⬜ |
| 3 | 脚本能拦（故意制造 drift） | 临时改 `apiRoutes.ts` 加一个不存在路由 → CI 红 → revert | 红 run | ⬜ |
| 4 | digest 假绿已修 | 断言脚本对占位值报错（或显式标注已知缺口） | 输出 | ⬜ |
| 5 | 4 处失真已更正 | 逐处 diff 复核 | diff | ⬜ |
| 6 | 更正的失真已登记 ADR-18 表 | 读 `adr-2026-09-doc-drift-registry.md` | 文档 | ⬜ |

## 5. 验收标准（DoD）

- [ ] 10 个脚本全部在 CI 中执行
- [ ] 至少 3 个脚本演示过"能拦住"（测试点 3 模式）
- [ ] `check_docker_digests.sh` 不再假绿
- [ ] 4 处失真更正 + 登记
- [ ] ADR-18 更新：失真登记与脚本 fail 挂钩

## 6. 已知风险

| 风险 | 应对 |
|------|------|
| 脚本接到 CI 后可能立即报红（长期未跑，drift 已积累） | 先跑基线（测试点 1），报红项按"是文档错还是代码错"分类；**不因接入 CI 而顺手改业务代码**（防范围蔓延，超范围的记账本） |
| `check_view_consistency.py` 可能真报出视图定义不一致（历史 A4 类 bug） | 若报出真实不一致，**单列为新发现**记入账本，不塞进本阶段修 |
| 平台差异（脚本在 Windows Git Bash vs CI Linux） | 逐个甄别；必要时容器化执行 |

## 7. 产出物

- 更新后的 `.github/workflows/`（新增校验 job）
- `scripts/check_docker_digests.sh` 修正
- 4 处文档更正 diff
- `docs/architecture/adr/adr-2026-09-doc-drift-registry.md` 更新
- 执行记录：`stages/e2e-05-doc-code-consistency/report.md`
