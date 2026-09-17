---
stage: e2e-03
title: CI/CD 门槛
type: transformation
status: pending
created: 2026-09-17
depends-on: [e2e-02]
blocks: [e2e-04, e2e-05]
gate: [github-token-workflow-scope]   # 见 RUNBOOK §9，未落定不得开工
related-findings: []
---

# E2E-03 🔧 CI/CD 门槛

## 1. 阶段目标

让"测试通过"从**人的自觉**变成**机器的门禁**。当前全仓零 CI：285 个 Go 测试、47 个前端测试、10 个校验脚本全靠人记得跑。这是其余 29 个阶段的**防退化机制**——没有它，本路线图每个阶段的产出（Playwright 回归钉）都不会被自动执行。

## 2. 范围与边界

### 做

1. 把 `docs/ci-workflows/` 的 3 份模板落地为真实 `.github/workflows/`：

| 模板 | 内容 | 落地后触发 |
|------|------|-----------|
| `docs/ci-workflows/go-test.yml` | 6 个 Go svc matrix + shared，`go vet ./...` + `go test -count=1 ./...` | push/PR to main |
| `docs/ci-workflows/llm-test.yml` | `emotion-llm-service` pytest（含 Internal-API-Key 4 用例、gRPC、Nacos bootstrap） | 同上 |
| `docs/ci-workflows/web-test.yml` | pnpm + vitest | 同上 |

2. 解除阻塞：模板无法 push 的原因是 **PAT 权限不足**。GitHub 拒绝写入 `.github/workflows/*.yml` 的报错是：

   ```
   ! [remote rejected] ... (refusing to allow a Personal Access Token
     to create or update workflow `.github/workflows/xxx.yml` without `workflow` scope)
   ```

   **需要补的权限（二选一，取决于 token 类型）**：

   | Token 类型 | 需补权限 | 说明 |
   |-----------|---------|------|
   | **Classic PAT**（`ghp_...`） | 勾选 **`workflow`** scope | 在 Settings → Developer settings → Personal access tokens → Tokens (classic) → 编辑该 token 勾上 `workflow`。原有的 `repo` scope 保留 |
   | **Fine-grained PAT**（`github_pat_...`） | Repository permissions → **Workflows: Read and write** | 同时确认 **Contents: Read and write**（推送代码用）、**Metadata: Read**（必选，自动勾）。在 Settings → Developer settings → Personal access tokens → Fine-grained tokens → 编辑 |

   补充说明：
   - `workflow`（或 Workflows 写权限）是 GitHub 的**独立安全门槛**，与 `repo`/Contents 权限分开授权——这就是"能推代码但推不了 workflow"的原因
   - 若不想动 token，也可**在 GitHub 网页界面手工创建**这 3 个文件（Actions 页签 → New workflow → 粘贴内容）
   - 修改后可用 `git push` 重试推送 `.github/workflows/` 验证是否解除

3. 拿到**首条绿 run 的证据**（截图或 run URL）

4. 把 `docs/ci-workflows/` 降级为归档（移入 `docs/legacy-plans/landed/` 或就地加"已落地"标记），避免与真实 workflow 双份维护

### 不做（边界）

- 不做部署自动化（CD 部分；prod 部署已被决策 3/23 冻结）
- 不做 Docker 镜像构建推送（后续可加，非本阶段）
- 不引入复杂流水线（矩阵缓存优化等）

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| E2E-02 完成（避免 CI 扫到散落文件导致假红） | ⏳ |
| 具备写 `.github/workflows/` 的 token 权限 | ⚠️ **需用户操作**（见 §2 做 2 的权限对照表） |
| `go test ./...` 本地全绿 | 需先确认基线 |

## 4. 测试点清单

| # | 测试点 | 验证方式 | 证据 | 结果 |
|---|--------|---------|------|------|
| 1 | 本地基线确认：7 个 Go 模块 `go test ./...` 全绿 | 逐模块跑，记录退出码 | 输出 | ⬜ |
| 2 | 本地基线确认：`pnpm test`（vitest）全绿 | 记录通过数 | 输出 | ⬜ |
| 3 | 本地基线确认：llm-service pytest 全绿 | `pytest` 输出 | 输出 | ⬜ |
| 4 | workflow 文件成功推送到远端 | GitHub 网页可见 Actions 页签 | 截图 | ⬜ |
| 5 | push 触发首个 run | Actions 页出现 run | run URL | ⬜ |
| 6 | run 全绿 | 断言各 job 结论 success | run URL + 截图 | ⬜ |
| 7 | 故意制造失败验证门禁生效 | 提交一个必失败断言 → run 变红 → revert | 红 run 证据 | ⬜ |
| 8 | PR 场景也触发 | 开一个 PR 观察 checks | 截图 | ⬜ |

## 5. 验收标准（DoD）

- [ ] `.github/workflows/` 存在且被 GitHub 识别
- [ ] 至少 1 条全绿 run 有据可查
- [ ] 门禁确实能拦（测试点 7 证明）
- [ ] `docs/ci-workflows/README.md` 更新状态（阻塞原因已解除）

## 6. 已知风险

| 风险 | 应对 |
|------|------|
| 首次接入可能暴露大量既有失败（285 个测试未必全绿） | 先本地跑基线（测试点 1-3），把既有失败单列，**不因 CI 引入而顺手修**（防范围蔓延）——若失败量大，可先落"仅新增代码门禁"过渡方案 |
| GitHub Actions 在私有仓库有免费额度限制 | 先量测单次 run 耗时，必要时裁剪 matrix |
| Windows 本地开发的路径/换行差异导致 CI（Linux）假红 | 首次 run 后逐一甄别环境差异 vs 真问题 |

## 7. 产出物

- `.github/workflows/{go-test,llm-test,web-test}.yml`
- 首条绿 run 的 URL/截图 → `stages/e2e-03-ci-gate/report.md`
- **被依赖**：E2E-04（前端工程化 lint/typecheck 接入）、E2E-05（10 个校验脚本接入）都挂在本阶段的 CI 上
