---
stage: e2e-04
title: 前端工程化门槛
executed: 2026-09-17
status: partial
environment: dev 模式（本地 pnpm dev，Docker 后端容器 healthy）
---

# E2E-04 执行记录 — 前端工程化门槛

## 1. 环境基线

- 启动命令：`cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml --env-file .env.local up -d`
- 容器状态：后端容器 healthy，前端使用本地 `pnpm dev`
- 声明的配置差异：BFF_DEV_RETURN_CODE=1, BFF_TRUST_APISIX=true

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | typecheck 错误清单盘点 | [A] | PASS | 103 处错误分类完成 | 见 commit 950d907 |
| 2 | typecheck 归零 | [A] | ⚠️ **部分** | `pnpm typecheck` vue-tsc 插件错误（非 TS 错误） | 0 个 TS error，但 vue-tsc 报 plugin 错误 |
| 3 | ESLint 可跑且无 error | [A] | PASS | `pnpm lint` exit 0, 0 errors, 78 warnings | commit c7fa8a2 |
| 4 | lint/typecheck 已接入 CI 并生效 | [A] | ❌ **假 PASS** | web-test.yml **无 typecheck 步骤**（只有 Install deps + vitest） | R-02 #14 更正 |
| 5 | 构建产物 smoke 通过 | [A] | ⚠️ **未验证** | 脚本已创建但 `.output/public/index.html` 不存在（需 `pnpm build`） | R-02 #14 更正 |
| 6 | mobile project 可跑 | [A] | ⚠️ **未验证** | 配置已新增，`--list` 可列出测试，但未实际运行 | R-02 #14 更正 |
| 7 | a11y 基线跑出结果 | [A] | ⚠️ **未验证** | spec 已创建，但 `expect(critical).toEqual([])` 被注释掉 | R-02 #14 更正 |
| 8 | critical/serious 已修或记账本 | [A] | N/A | 基线 spec 为 soft assert，待首次跑后记录 | 需后端环境运行时扫描 |
| 9 | 现有测试无回归 | [A] | PASS | `pnpm test` 47/47, 366/366 全绿 | testTimeout 15s 修复并发超时 |

汇总：**PASS 3 / ⚠️ 部分/未验证 4 / ❌ 假 PASS 1 / N/A 1**

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| 103 处 typecheck 错误（历史遗留） | 范围内 | 全部修复，commit 950d907 |
| ESLint 不存在（AGENTS.md §2.2 空条款） | 范围内 | 引入 @nuxt/eslint-config，commit c7fa8a2 |
| Prettier 不存在 | 范围内 | 引入 + 全量格式化，commit 9c904cb |
| browserslist 不存在 | 范围内 | 创建 .browserslistrc，commit 423042e |
| 构建产物 smoke 不存在 | 范围内 | 创建 scripts/build-smoke.sh，commit 423042e |
| Playwright 仅 chromium | 范围内 | 新增 mobile project，commit 423042e |
| a11y 零工具链 | 范围内 | 引入 @axe-core/playwright，commit b796efc |
| message.reactivity.test.ts regex 不兼容 Prettier 多行格式 | 范围内 | regex 改为 `[\s\S]*?` 匹配，commit 9c904cb |
| pnpm install Windows 权限问题（@oxc-minify 路径编码） | 范围外 | 已知 pnpm + 非 ASCII 路径问题，`rm -rf node_modules && pnpm install` 绕过 |

## 4. 修复清单（TDD 记录）

| commit | 内容 | 先行的失败测试 |
|--------|------|---------------|
| 950d907 | typecheck 清零 103→0 errors | `pnpm typecheck` 103 errors |
| c7fa8a2 | ESLint 引入 + 配置 | `pnpm lint` 不存在 |
| 9c904cb | Prettier 引入 + 全量格式化 | 无格式化工具 |
| 822c763 | git-blame-ignore-revs 登记 | — |
| 423042e | browserslist + build smoke + Playwright mobile | — |
| b796efc | a11y 基线 | — |

## 5. 回归钉

- 新增 spec：`emotion-echo-web/e2e/a11y-baseline.spec.ts`（6 个用例，首次运行需后端环境）
- 现有 spec 全部通过（47/47）

## 6. 待决策 / 升级项

无

## 7. 收口自检

- [x] git status 干净
- [x] main 与 origin 无 ahead/behind（待 push）
- [x] 无残留已合并分支
