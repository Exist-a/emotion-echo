---
stage: e2e-03
title: CI/CD 门槛（落地 + 严格化）
type: transformation
status: done
created: 2026-09-17
last-refresh: 2026-09-17 (补阶段 2「严格化」——现有 3 份模板不严谨，实测缺陷清单见 §2.5)
depends-on: [e2e-02]
blocks: [e2e-04, e2e-05]
gate: []   # 原「GitHub token 缺 workflow scope」已实测解除（2026-09-17）
related-findings: [E2E-F-20, E2E-F-30, E2E-F-31, E2E-F-32, E2E-F-33, E2E-F-34, E2E-F-35]
---

# E2E-03 🔧 CI/CD 门槛（落地 + 严格化）

## 1. 阶段目标

让"测试通过"从**人的自觉**变成**机器的门禁**，并且这个门禁要**真的能拦、真的跑全**。当前全仓零 CI：285 个 Go 测试、47 个前端测试、10 个校验脚本全靠人记得跑。这是其余 29 个阶段的**防退化机制**。

本阶段分两段：**阶段 1 落地**（把模板变成真 CI）、**阶段 2 严格化**（模板实测不严谨，逐条加固）。

## 2. 范围与边界

### 2.1 做（阶段 1）：落地 3 份模板

| 模板 | 内容 | 触发 |
|------|------|------|
| `docs/ci-workflows/go-test.yml` | 7 个 Go 模块（6 svc + shared）矩阵，`go vet` + `go test` | push/PR to main |
| `docs/ci-workflows/llm-test.yml` | `emotion-llm-service` pytest | 同上（paths 过滤） |
| `docs/ci-workflows/web-test.yml` | pnpm + vitest | 同上（paths 过滤） |

步骤：`mkdir -p .github/workflows && cp docs/ci-workflows/*.yml .github/workflows/` → commit → push → 取得**首条绿 run 证据**。

### 2.2 阻塞已解除（2026-09-17 实测）

原阻塞"PAT 缺 `workflow` scope"**已不成立**。实测证据：

- GitHub API `GET /user` 响应头：`x-oauth-scopes: gist, repo, workflow, write:packages` ✅
- **真实 push 验证**：在临时分支提交 `.github/workflows/scope-probe.yml` 并 push → **成功（退出码 0）**，随后分支已清理

若未来再次遇到 `refusing to allow a Personal Access Token to create or update workflow`，权限对照：

| Token 类型 | 需补权限 |
|-----------|---------|
| Classic PAT（`ghp_`） | 勾选 **`workflow`** scope |
| Fine-grained PAT（`github_pat_`） | Repository permissions → **Workflows: Read and write**（另需 Contents: Read and write、Metadata: Read） |

### 2.3 做（阶段 2）：严格化 —— 实测缺陷清单

> 以下是**实测**（非推测）出的模板缺陷，逐条给出具体做法。**本阶段不执行改造，仅登记排期。**

#### A. Go workflow（`go-test.yml`）

| # | 缺陷 | 证据 | 具体怎么做 |
|---|------|------|-----------|
| A1 | **Go 版本不匹配** | CI 写死 `go-version: '1.22'`，而全部 7 个 `go.mod` 都是 `go 1.26.1` | 改为 `go-version-file: 'go.mod'`（矩阵下各服务目录各自解析），或显式 `1.26.1`。否则依赖 GOTOOLCHAIN 自动下载，CI 变慢且可失败 |
| A2 | `GOFLAGS: -mod=mod` 削弱可重现性 | 该 env 允许测试期间**改写 go.mod/go.sum** | 删除该 env，恢复默认 `-mod=readonly`；若确实需要，改为显式 `go mod download` 前置步骤 |
| A3 | **无 `-race`** | 项目有大量 goroutine（outbox relay、Kafka consumer、fusion worker），CI 不跑竞态检测 | 增 `go test -race -count=1 ./...`。注意耗时翻倍，可拆为独立 job 或在 `main` push 才跑 |
| A4 | **覆盖率底线不可执行** | AGENTS.md §2.3 定义 80%/90%/70% 底线，CI **完全不测覆盖率** | 增 `go test -coverprofile=cover.out ./...` + `go tool cover -func` 输出；按 AGENTS.md 分档设阈值（核心业务包 80% / pkg 90% / 适配层 70%），低于阈值 fail。首轮建议先"只报告不拦截"，摸清基线后再收紧 |
| A5 | **集成测试永不执行** | 23 个 `*_integration_test.go` 挂在 `//go:build integration`，CI 跑 `go test ./...` 不含该 tag | 增独立 job：`go test -tags integration ./...`。⚠️ 这些测试依赖真实 PG/Kafka，需 GitHub Actions `services:` 起 postgres/kafka 容器，或先只跑不依赖外部的子集 |
| A6 | **无格式检查** | `go vet` 不检查格式 | 增 `test -z "$(gofmt -l .)"` 或引入 `golangci-lint`（后者更全：staticcheck/ineffassign 等） |
| A7 | 无 `timeout-minutes` | 默认单 job 最长 6 小时，挂死会白烧额度 | 每 job 设 `timeout-minutes: 15` |
| A8 | 无 `concurrency` | 同一分支连推会并行跑多个 run，浪费额度且结果互相覆盖 | 增 `concurrency: {group: '${{ github.workflow }}-${{ github.ref }}', cancel-in-progress: true}` |
| A9 | 无 `permissions` | 默认可能授予过宽权限 | 增 `permissions: {contents: read}`（只读即可） |

#### B. Web workflow（`web-test.yml`）

| # | 缺陷 | 证据 | 具体怎么做 |
|---|------|------|-----------|
| B1 | **lint 步骤是装饰** | 该步骤自带守卫 `grep -q '"lint"' package.json`，而 `package.json` **无 lint script** → 永远走 `echo "lint script not configured — skipping"` | 由 **E2E-04** 引入 ESLint 后删除守卫，改为硬执行 `pnpm lint`（失败即 fail） |
| B2 | **typecheck 不跑** | `scripts.typecheck` 存在（`nuxt typecheck`），但 CI 无该步骤；仓库有 96 处历史错误 | 由 **E2E-04** 清错后接入 `pnpm typecheck` |
| B3 | **无 Playwright E2E** | 整条 E2E 路线图的回归钉都写在 `e2e/*.spec.ts`，CI 不跑 → **钉了也不生效** | 增 E2E job：起 dev 依赖（compose）或用 `playwright.config.ts` 的 webServer，跑 `pnpm playwright test`；上传 `playwright-report/` 为 artifact。⚠️ 依赖后端，成本高——可先只跑不依赖后端的 spec |
| B4 | **无构建验证** | 项目是 SSR（`ssr: true`），`nuxt build` 从未在 CI 验证 | 增 `pnpm build` 步骤（E2E-04 的产物 smoke 可挂此处） |
| B5 | 版本声明缺失 | CI 写死 `node-version: '20'` / `pnpm version: 9`，而 `package.json` **无 `engines`、无 `packageManager`** | 补 `packageManager: "pnpm@9.x"` 与 `engines.node`，CI 改用 `node-version-file`/`corepack` 自动对齐 |
| B6 | 无 `timeout-minutes` / `concurrency` / `permissions` | 同 A7-A9 | 同 A7-A9 |

#### C. LLM workflow（`llm-test.yml`）

| # | 缺陷 | 证据 | 具体怎么做 |
|---|------|------|-----------|
| C1 | **依赖未锁版本** | `requirements.txt` 全部用 `>=`（`fastapi>=0.100.0`、`openai>=1.40.0`…）→ CI 每次装最新，**同一 commit 可绿可红** | 生成锁定文件（`pip-compile` → `requirements.lock.txt`，或 `pip freeze` 快照），CI 装锁定文件；上游升级走显式 PR |
| C2 | **models 测试套件零覆盖** | `emotion-echo-models` 有 **20 个项目 pytest 文件**（FER 5 / FER-tflite 5 / sensevoice 4 / XTTS 6），CI 完全不跑 | 增 job（可矩阵）跑 `pytest`。注意镜像/依赖重，可先只跑不加载真实模型的单测 |
| C3 | 无覆盖率 | 同 A4 | 增 `pytest --cov`，先报告后拦截 |
| C4 | 无 `timeout-minutes` / `concurrency` / `permissions` | 同 A7-A9 | 同 A7-A9 |

#### D. 仓库设置（workflow 文件之外，但属"门禁"必需）

| # | 缺陷 | 证据 | 具体怎么做 |
|---|------|------|-----------|
| D1 | **main 无分支保护** | GitHub API `GET /branches/main/protection` → `404 Branch not protected`。**2026-09-17 已修复部分**（见下方 §2.3.1） | **已开**（安全子集）：`allow_force_pushes=false` + `allow_deletions=false` + `enforce_admins=true`。**待开**（阶段 2，CI 落地后）：`required_status_checks` 指向本阶段 job；`required_pull_request_reviews`（单人项目可选，开了会失去直推自由） |
| D2 | **README 的声称不成立** | `docs/ci-workflows/README.md` 写"任何 test 失败 → PR 不可 merge"——但 D1 未配置完整，该效果**不存在** | 待 D1 的 status checks 落地后该声称才成立；同时校正 README 措辞（属 E2E-05 的失真更正范畴） |
| D3 | 仓库为 **public** | API 返回 `visibility: public` | 仅作记录：public 仓库的 Actions 额度更宽松，但**任何误提交的密钥会立即公开**。与 AGENTS.md §四"密钥不进版本库"红线叠加，建议在 E2E-05 加 secret 扫描（如 gitleaks）到 CI |
| D4 | 其它 4 套测试零覆盖 | `scripts/test_*.py`（4）、`k8s/tests/*_test.go`（6）、`deploy/*.test.js`（1）、`legacy/`（1）| `scripts/` 与 `deploy/` 的应接入（成本低）；`k8s/tests` 需 `helm` 二进制 + build tag，可单独 job；`legacy/` 明确排除并记录理由 |

#### 2.3.1 D1 实施记录（2026-09-17）

**已应用到 `main`**（经 GitHub REST API，`admin: True` 权限）：

```json
{"required_status_checks":null, "enforce_admins":true,
 "required_pull_request_reviews":null, "restrictions":null,
 "allow_force_pushes":false, "allow_deletions":false}
```

**关键经验（踩过的坑）**：

| 观测 | 含义 |
|------|------|
| 首次以 `enforce_admins: false` 应用后，**强制推送仍成功** | 仓库管理员默认**绕过所有规则**；单人项目里唯一的用户就是管理员 ⇒ `enforce_admins: false` 等于**没有保护** |
| 改为 `enforce_admins: true` 后重测，强制推送被拒：`GH006: Protected branch update failed ... Cannot force-push to this branch` | ✅ 保护真正生效 |
| 一次 API 调用返回 `HTTP 000 / SSL_ERROR_SYSCALL` | **api.github.com 从本机间歇性不可达**（5 次复测均 <2s 正常）。这也可能是 **github-mcp 30s 超时的主因之一**（MCP 服务器启动时会调 api.github.com 验 token） |

**测试方法（可复现）**：建临时分支 → 给临时分支打同样规则 → rewind 后重新提交制造非快进 → `git push --force` 断言被拒 → 移除临时保护 + 删分支。**全程不触碰 main**。

**副作用验证**：`enforce_admins: true` 且 status checks / PR 要求均为 `null` 时，**正常快进推送不受影响**（仅强推与删除被拦）——已确认 `git push origin main` 正常。

**⚠️ 阶段 2 注意**：一旦添加 `required_status_checks`，`enforce_admins: true` 会连管理员也拦住；若 check 因故不上报会**卡死无法合并**。届时要么确认 check 稳定上报，要么阶段性放宽 `enforce_admins`。

### 2.4 不做（边界）

- 不做部署自动化（prod 部署已被决策 3/23 冻结）
- 不做 Docker 镜像构建推送（后续可加，非本阶段）
- 不做复杂流水线优化（矩阵缓存调优等）
- 不在本阶段顺手修 CI 暴露出的**既有测试失败**（记账本，防范围蔓延）
- 不引入第三方 SaaS CI（保持 GitHub Actions）

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| E2E-02 完成（避免 CI 扫到散落文件导致假红） | ⏳ |
| 写 `.github/workflows/` 的权限 | ✅ **已验证具备**（真实 push 成功） |
| 本地基线：各模块 `go test ./...` / `pnpm test` / `pytest` 结果 | 需先确认（决定首轮是否"只报告不拦截"） |
| GitHub 仓库 Settings 管理权限（D1 分支保护） | 需用户操作 |

## 4. 测试点清单

判定标记：`[A]` 自动可判 · `[V]` 视觉判定 · `[M]` 需人工裁定（详见 [RUNBOOK.md](../../RUNBOOK.md) §4）。

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 1 | 本地基线：7 个 Go 模块 `go test ./...` 结果 | [A] | 逐模块跑，记录退出码与失败数 | 输出 | ⬜ |
| 2 | 本地基线：`pnpm test`（vitest）结果 | [A] | 记录通过/失败数 | 输出 | ⬜ |
| 3 | 本地基线：llm-service `pytest` 结果 | [A] | 记录通过/失败数 | 输出 | ⬜ |
| 4 | workflow 文件成功推送到远端 | [A]+[V] | GitHub 网页 Actions 页签可见 | 截图 | ⬜ |
| 5 | push 触发首个 run | [A] | Actions 页出现 run | run URL | ⬜ |
| 6 | run 全绿 | [A]+[V] | 各 job 结论 success | run URL + 截图 | ⬜ |
| 7 | **门禁能拦**（阶段 1 关键） | [A] | 提交必失败断言 → run 变红 → revert | 红 run 证据 | ⬜ |
| 8 | PR 场景也触发 | [A] | 开 PR 观察 checks 出现 | 截图 | ⬜ |
| 9 | Go 版本与 go.mod 一致（A1） | [A] | CI 日志中 Go 版本 = 1.26.x | 日志片段 | ⬜ |
| 10 | `-race` 生效（A3） | [A] | CI 日志出现 `-race`；或制造数据竞争验证能抓 | 日志片段 | ⬜ |
| 11 | 覆盖率有输出（A4） | [A] | CI 日志出现每包覆盖率数值 | 日志片段 | ⬜ |
| 12 | 格式检查能拦（A6） | [A] | 故意提交未格式化文件 → 红 → revert | 红 run | ⬜ |
| 13 | web lint 真实执行（B1） | [A] | 日志出现真实 lint 结果（非 skipping） | 日志片段 | ⬜ |
| 14 | web typecheck 真实执行（B2） | [A] | 日志出现 typecheck 结果 | 日志片段 | ⬜ |
| 15 | models 测试有 job（C2） | [A] | Actions 列表出现该 job 且执行 | 截图 | ⬜ |
| 16 | 依赖已锁版本（C1） | [A] | CI 装的是锁定文件；**同 commit 重跑两次结果一致** | 两次 run 对比 | ⬜ |
| 17 | 分支保护已配置（D1） | [A] | `GET /branches/main/protection` 返回 200 而非 404 | API 输出 | ⬜ |
| 18 | 分支保护确实拦（D1） | [A] | 在保护生效后尝试把红 PR merge → 被拒 | 截图 | ⬜ |
| 19 | `timeout-minutes` 生效（A7/B6/C4） | [A] | 每个 job 的 yaml 含该字段 | 文件 diff | ⬜ |
| 20 | `concurrency` 生效（A8） | [A] | 连续两次 push 只有最新 run 存活 | Actions 截图 | ⬜ |
| 21 | 首轮是否"只报告不拦截"策略已记录 | [M] | 若既有失败量大，明确记录过渡策略并写入报告 | report.md | ⬜ |

## 5. 验收标准（DoD）

- [ ] 21 个测试点有结果（PASS/FAIL/BLOCKED/N/A，BLOCKED 不超过 1/3）
- [ ] `.github/workflows/` 存在且被 GitHub 识别，至少 1 条全绿 run 有据可查
- [ ] 门禁确实能拦（测试点 7、12、18 三者至少两项有红 run 证据）
- [ ] 阶段 2 的 A/B/C/D 四组缺陷**全部逐条有结论**（已修 / 明确排除并记录理由 / 转其他阶段）
- [ ] `docs/ci-workflows/README.md` 状态更新（阻塞已解除 + 校正"不可 merge"措辞）
- [ ] 回归钉无处可加的例外：本阶段产物即 CI 本身，用 testing point 4-8 作为验收
- [ ] 按 [RUNBOOK.md](../../RUNBOOK.md) §7 收口契约完成 8 项 + 自检三连

## 6. 已知风险

| 风险 | 应对 |
|------|------|
| 首次接入暴露大量既有失败（285 个 Go 测试未必全绿） | 先跑本地基线（测试点 1-3）；既有失败**单列不顺手修**；量大则先落"仅新增代码门禁"过渡 |
| `-race` 使测试耗时翻倍，可能触及 Actions 配额 | 拆独立 job；或仅在 `main` push 时跑 |
| 集成测试（A5）需要 postgres/kafka 服务容器，CI 成本高 | 首轮只跑不依赖外部的子集，其余记账本 |
| models 测试（C2）依赖重（模型/大镜像） | 先只接不加载真实模型的单测；重测试明确排除并记录理由 |
| 分支保护（D1）需仓库 Settings 权限，执行者可能无权限 | 列入升级项，由用户在 Settings 配置 |
| 仓库为 public（D3），CI 日志可能暴露敏感值 | 不在 workflow 里 echo 任何 secret；建议加 gitleaks 扫描 |
| 平台差异（Windows 本地 vs Linux CI）导致假红 | 测试点 4-6 首次 run 后逐一甄别环境差异 vs 真问题 |

## 7. 产出物

- `.github/workflows/{go-test,llm-test,web-test}.yml`（落地 + 严格化后版本）
- 锁定依赖文件（C1：`requirements.lock.txt` 或 `pip-compile` 产物）
- `package.json` 补 `packageManager` / `engines`（B5）
- main 分支保护规则（D1，用户在 Settings 配置）
- 执行记录：`stages/e2e-03-ci-gate/report.md`（含首条绿 run URL + 红 run 门禁证据）
- **被依赖**：E2E-04（lint/typecheck 接入）、E2E-05（10 个校验脚本接入）
