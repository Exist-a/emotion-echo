---
stage: e2e-03
title: CI/CD 门槛（落地 + 严格化）
status: done
created: 2026-09-17
completed: 2026-09-17
---

# E2E-03 执行报告 — CI/CD 门槛

## 一、执行摘要

**目标**：把 `docs/ci-workflows/` 的 3 份 CI 模板落地到 `.github/workflows/`，让"测试通过"从人的自觉变成机器的门禁。

**结果**：3 份 workflow 已落地并被 GitHub Actions 识别，首轮 CI 即暴露真实问题。Phase 2 严格化一次性折叠进 Phase 1，不再分两轮。

| Workflow | 首条绿 run | 当前状态 |
|----------|-----------|---------|
| llm-test | ✅ #1 (42s) | 绿 |
| web-test | ✅ #4 (37s) | 绿 |
| go-test | ❌ 有真实测试失败 | 红（既有失败，非配置问题） |

## 二、CI 严格化改进（Phase 2 折叠进 Phase 1）

| 缺陷 | 原模板 | 修复后 |
|------|--------|--------|
| A1 Go 版本不匹配 | `go-version: '1.22'` | `go-version-file: '${{ matrix.service }}/go.mod'` (自动读 1.26.1) |
| A2 GOFLAGS 削弱可重现性 | `GOFLAGS: -mod=mod` | 已删除 |
| A3 无 -race | `go test -count=1` | `go test -race -count=1` |
| A7 无 timeout | 无 | `timeout-minutes: 15` (所有 job) |
| A8 无 concurrency | 无 | `concurrency: {group, cancel-in-progress: true}` |
| A9 无 permissions | 默认过宽 | `permissions: {contents: read}` |
| B1 lint 装饰 | `grep -q '"lint"'` 守卫 | 已删除（lint script 不存在，不假装） |
| B6 web 无 timeout/concurrency | 无 | 已加 |

## 三、CI 调试过程（真实踩坑记录）

| # | 现象 | 根因 | 修复 |
|---|------|------|------|
| 1 | web-test #1: `pnpm: Unable to locate executable file` | `pnpm/action-setup` 必须在 `actions/setup-node` 之前执行 | 调整 step 顺序 |
| 2 | web-test #2: `Dependencies lock file is not found` | `pnpm-lock.yaml` 在 `emotion-echo-web/` 子目录，`setup-node` cache 在根目录找 | 加 `cache-dependency-path` |
| 3 | web-test #3: `exit code 1` (48s) | E2E-F-39: `clientAccessToken.ts` import `#app` 无 Nuxt alias → vitest 模块解析失败 | 创建 `tests-app-mock.ts` + vitest alias 修复 |
| 4 | web-test #4: ✅ 全绿 | — | — |
| 5 | go-test: 所有 7 个模块 `exit code 1` | **真实测试失败**（非配置问题）：`go vet` 报 unreachable code / context leak；`go test` 有既有的 FAIL | 记账本，不顺手修 |

## 四、E2E-F-39 修复

**问题**：`useAIStreamHandler.test.ts` 因 `#app` import 解析失败导致 vitest 预存失败。

**根因**：`clientAccessToken.ts:17` `import { useCookie } from '#app'`，vitest 环境下 `#app` 解析到 `app/` 目录但该目录不导出 `useCookie`。

**修复**：
- 新建 `tests-app-mock.ts`：提供 `useCookie` / `useState` / `useRuntimeConfig` / `navigateTo` 最小 mock
- `vitest.config.ts`：`#app` 精确匹配 alias 指向 `tests-app-mock.ts`
- `tests-setup.ts`：`useState` 挂到 `globalThis`（Nuxt auto-import）

**结果**：47 files, 366 tests 全绿（之前 46/357 + 1 FAIL）。

## 五、测试点结果

| # | 测试点 | 结果 | 证据 |
|---|--------|------|------|
| 4 | workflow 文件成功推送到远端 | ✅ PASS | `git push` 成功，GitHub Actions 识别 3 个 workflow |
| 5 | push 触发首个 run | ✅ PASS | 9 个 run 记录 |
| 6 | run 全绿 | ⚠️ PARTIAL | web-test ✅, llm-test ✅, go-test ❌ (既有失败) |
| 7 | 门禁能拦 | ✅ PASS | go-test 红 run 证据（测试点 6 的反面） |
| 9 | Go 版本与 go.mod 一致 | ✅ PASS | `go-version-file` 自动读取 1.26.1 |
| 10 | -race 生效 | ✅ PASS | workflow 文件含 `-race` flag |
| 19 | timeout-minutes 生效 | ✅ PASS | 所有 job 含 `timeout-minutes: 15` |
| 20 | concurrency 生效 | ✅ PASS | go-test #2 被 #3 cancel-in-progress |

**既有 Go 测试失败（记账本，不修）**：
- `emotion-echo-chat-svc`: `go vet` unreachable code + context.WithCancel leak
- `emotion-echo-shared`: test FAIL
- `emotion-echo-assessment-svc`: test FAIL
- `emotion-echo-analytics-svc`: test FAIL
- `emotion-echo-user-svc`: test FAIL
- `emotion-echo-ai-svc`: test FAIL

## 六、产出物

| 文件 | 用途 |
|------|------|
| `.github/workflows/go-test.yml` | Go 7 模块矩阵测试 |
| `.github/workflows/web-test.yml` | 前端 vitest 测试 |
| `.github/workflows/llm-test.yml` | Python pytest 测试 |
| `emotion-echo-web/tests-app-mock.ts` | #app mock 模块 (E2E-F-39) |

## 七、与 E2E-04 的衔接

web-test 已绿，但以下改进留给 E2E-04（前端工程化门槛）：
- B2 typecheck 步骤（96 处历史错误需先清）
- B3 Playwright E2E job（依赖后端，成本高）
- B4 构建验证 (`pnpm build`)
- B5 版本声明 (`packageManager` / `engines`)
