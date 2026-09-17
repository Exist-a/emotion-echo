---
stage: e2e-04
title: 前端工程化门槛
type: transformation
status: pending
created: 2026-09-17
depends-on: [e2e-03]
blocks: [e2e-07, e2e-09, e2e-10, e2e-11, e2e-12, e2e-13, e2e-14, e2e-15, e2e-16, e2e-17]
related-findings: [E2E-F-14, E2E-F-20]
---

# E2E-04 🔧 前端工程化门槛

## 1. 阶段目标

给前端建立机械防护。路线图中 E2E-01/07~17 **全是前端重度阶段**，而当前前端**没有 lint、typecheck 有 96 处历史错误、构建产物从不验证**——每个阶段的改动都在无网防护下进行。本阶段一次补齐。

## 2. 范围与边界

### 做

| # | 项 | 现状（已核实） | 目标 |
|---|----|--------------|------|
| 1 | ESLint | ❌ 无配置；`devDependencies` **无 eslint**；`package.json` **无 `lint` script**（AGENTS.md §2.2 的"合并前 `npm run lint`"是空条款） | 引入 + 配置 + `lint` script + 接入 E2E-03 CI |
| 2 | Prettier | ❌ 无配置、无 `.editorconfig`、无 stylelint | 引入 + 格式化基线（**独立 commit**，避免与逻辑改动混在一起） |
| 3 | typecheck | ⚠️ 脚本存在（`nuxt typecheck` → vue-tsc），但仓库有 **96 处历史错误基线**（见 `docs/stages/stage-85-*.md:51,79`） | 清零；若无法一次清零，落"仅新增文件零错"基线并写入 CI |
| 4 | 构建产物 smoke | ❌ 无。项目是 **SPA 模式**（`nuxt.config.ts` `ssr: false`） | `nuxt build` 后跑产物 smoke（关键静态资源存在、index.html 可加载） |
| 5 | Playwright project | ⚠️ 仅 `chromium-headless-shell` 一个 project | 加 `mobile`（`devices['Pixel 5']`）+ `firefox`（可选回归，默认仍走 chromium） |
| 6 | browserslist | ❌ 无支持范围声明（仅 package-lock 里的 transitive） | 补声明（1 行决策） |
| 7 | a11y 基线 | ❌ 零工具链；仅约 8 个文件有零散手工 `aria-label` | 引入 `@axe-core/playwright`，对 6 个主页面跑基线，**只修 critical/serious** |

### 不做（边界）

- 不重写业务逻辑（格式化基线除外，且独立 commit）
- 不追 100% a11y 合规（只做基线 + critical/serious）
- 不做跨浏览器全矩阵（firefox 作为可选回归即可）
- 不引入 SSR（项目是 SPA 模式的有意决策，见 B-A 系列修复史）

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| E2E-03（CI 就绪，lint/typecheck 要有地方跑） | ⏳ |
| E2E-02（目录清理，避免 lint 扫到散落文件） | ⏳ |
| 确认 96 处 typecheck 错误的构成（真实缺陷 vs 类型定义缺失） | 需先出清单 |

## 4. 测试点清单

| # | 测试点 | 验证方式 | 证据 | 结果 |
|---|--------|---------|------|------|
| 1 | typecheck 错误清单盘点 | 跑 `pnpm typecheck`，分类 96 处（真 bug / 类型缺失 / 第三方） | 错误清单 | ⬜ |
| 2 | typecheck 归零（或落"新增零错"基线） | `pnpm typecheck` 退出码 0 | 输出 | ⬜ |
| 3 | ESLint 可跑且无 error（warning 可留） | `pnpm lint` 退出码 0 | 输出 | ⬜ |
| 4 | lint/typecheck 已接入 CI 并生效 | 故意提交违规 → CI 红 → revert | 红 run 证据 | ⬜ |
| 5 | 构建产物 smoke 通过 | `pnpm build` + 产物断言脚本 | 输出 + 产物清单 | ⬜ |
| 6 | mobile project 可跑 | `pnpm playwright test --project=mobile` 现有 spec 至少在移动端 viewport 不炸 | 截图 | ⬜ |
| 7 | a11y 基线跑出结果 | axe 对 6 主页跑，记录 critical/serious 清单 | 报告 | ⬜ |
| 8 | critical/serious 已修或记账本 | 修复项走 TDD；范围外记 `discovered-unresolved.md` | 清单 | ⬜ |
| 9 | 现有测试无回归 | `pnpm test` + 现有 Playwright spec 全绿 | 输出 | ⬜ |

## 5. 验收标准（DoD）

- [ ] 9 个测试点通过
- [ ] lint + typecheck 在 CI 里能拦住新增问题
- [ ] Prettier 格式化独立 commit，不混逻辑改动
- [ ] a11y 只修 critical/serious，其余记账本

## 6. 已知风险

| 风险 | 应对 |
|------|------|
| 96 个 typecheck 错误里混着真实缺陷，修起来可能触及业务逻辑 | 先分类（测试点 1）；真 bug 类单独列出，若超出本阶段预算则转阶段并记账本 |
| Prettier 全量格式化产生巨大 diff，掩盖后续真实改动 | 独立 commit + 在 `.git-blame-ignore-revs` 登记 |
| a11y 引入新依赖可能与 Nuxt 版本冲突 | 先验证依赖可装；`a11-sidebar-fallback.architecture.test.ts` 的 `a11` 是 **Sprint 111 编号**（与可访问性无关），命名易误判，建议加注释澄清 |
| ESLint 规则过严导致大量既有告警 | 首轮以"不引入新 error"为准，存量 warning 分期 |

## 7. 产出物

- `emotion-echo-web/{eslint.config.*,.prettierrc,.browserslistrc}`
- `package.json` 新增 `lint` script
- `emotion-echo-web/e2e/a11y-baseline.spec.ts`
- `playwright.config.ts` 增 `mobile` project
- 执行记录：`stages/e2e-04-frontend-engineering/report.md`
