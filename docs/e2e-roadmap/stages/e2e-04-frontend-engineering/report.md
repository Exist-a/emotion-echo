---
stage: e2e-04
title: 前端工程化门槛
executed: 2026-09-17
status: done
environment: dev 模式（本地 pnpm dev :3000，Docker 后端 25 容器 healthy，APISIX :19080）
---

# E2E-04 执行记录 — 前端工程化门槛

> **2026-10-13 收口轮**：本报告曾被 R-02 降为 `partial`（2 FAIL + 3 BLOCKED）。
> 本轮**逐点复验**并给出最新证据 ⇒ **9/9 PASS、0 FAIL、0 BLOCKED**（详见 §2 与 §4）。
> 复验手法：真跑命令取证据，**不沿用旧结论**。

## 1. 环境基线

- 启动命令（后端）：Docker 栈 25 容器 healthy（APISIX `:19080` 通）
- 前端：本地 `pnpm dev`（`:3000`）；**产物验证另起** `node .output/server/index.mjs`（`PORT=3998`）
- 声明的配置差异：BFF_DEV_RETURN_CODE=1, BFF_TRUST_APISIX=true

## 2. 测试点结果（2026-10-13 复验）

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | typecheck 错误清单盘点 | [A] | PASS | 原 103 处错误分类完成 | 见 commit 950d907 |
| 2 | typecheck 归零 | [A] | PASS | 真跑 `pnpm typecheck` → **exit 0 / `error TS` 计数 = 0**，且**无 vue-tsc 插件报错** | 2026-10-13；修法 = 依赖对齐（§4） |
| 3 | ESLint 可跑且无 error | [A] | PASS | 真跑 `pnpm lint` → **exit 0 / `0 errors`**（95 warnings） | 2026-10-13；修 9 个 error（§4） |
| 4 | lint/typecheck 已接入 CI 并生效 | [A] | PASS | `.github/workflows/web-test.yml:39-42` 有 `Typecheck` 步骤（`run: pnpm typecheck`）；**CI 实测** `web-test-frontend` job 在 PR #202 / #204 均 `success` | 原 FAIL 已过期（后续轮已接入） |
| 5 | 构建产物 smoke 通过 | [A] | PASS | 真跑 `pnpm build` → **exit 0**；`node .output/server/index.mjs` 后 `GET /login` **200**（13.4KB HTML）、`GET /` **301→/chat**、`favicon.ico` **200**、`_nuxt/*.js` 存在 | 2026-10-13；**原判据 `.output/public/index.html` 对 SSR 项目是错的**（SSR 无静态 index.html） |
| 6 | mobile project 可跑 | [A] | PASS | 真跑 `npx playwright test --project=mobile`（a11y spec）→ **6/6 passed**（对产物 server 亦 6/6） | 2026-10-13；后续阶段亦广泛使用 mobile project |
| 7 | a11y 基线跑出结果 | [A] | PASS | spec 断言行已生效（**不再是 soft-assert**）；真跑 chromium + mobile 各 6 页 = **12/12 passed**（对产物 server） | 2026-10-13；命中并修复 2 类 serious（见 #8） |
| 8 | critical/serious 已修或记账本 | [A] | PASS | **真跑** `npx playwright test e2e/a11y-baseline.spec.ts`（chromium+mobile）→ **12/12 passed**（修前 RED：6/6 页报 `html-has-lang[serious]` + `document-title[serious]`）；产物 server 实取 `curl -s :3998/login | head -c 260` → `<html  lang="zh-CN">` + `<title>情绪回声 Emotion-Echo</title>`。`color-contrast`（5 nodes）**记账本 E2E-F-221**，spec 内为**显式白名单**（`KNOWN_GAPS`） | 2026-10-13；白名单外任何 critical/serious 即红 |
| 9 | 现有测试无回归 | [A] | PASS | 真跑 `pnpm vitest run` → **78 文件 / 658 用例 全绿** | 2026-10-13 |

汇总：PASS 9 / FAIL 0 / BLOCKED 0 / N/A 0

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
| **lint 回归到 9 errors**（后续阶段引入） | 范围内 | 2026-10-13 修复（§4），`pnpm lint` 回 0 errors |
| **vue-router 双版本并存**（顶层 4.6.4 + nuxt 4.5.2 依赖的 5.3.1）导致 vue-tsc 的 router volar 插件加载失败 | 范围内 | 2026-10-13 对齐依赖（§4） |
| **a11y serious：`html-has-lang` / `document-title`**（6 页 ×2） | 范围内 | 2026-10-13 修（`nuxt.config.ts` app.head） |
| **a11y serious：`color-contrast`**（5 nodes，需配色设计改动） | 范围外 | 记账本 **E2E-F-221**，spec 显式白名单 |

## 4. 修复清单（TDD 记录）

| commit | 内容 | 先行的失败测试 |
|--------|------|---------------|
| 950d907 | typecheck 清零 103→0 errors | `pnpm typecheck` 103 errors |
| c7fa8a2 | ESLint 引入 + 配置 | `pnpm lint` 不存在 |
| 9c904cb | Prettier 引入 + 全量格式化 | 无格式化工具 |
| 822c763 | git-blame-ignore-revs 登记 | — |
| 423042e | browserslist + build smoke + Playwright mobile | — |
| b796efc | a11y 基线 | — |
| **（2026-10-13 收口轮）** lint 9 errors 修复 | `DigitalHuman.client.vue` 十六进制字面量大写 ×2 / `useApi.ts` prefer-const / `useTTSManager.segment.test.ts` import 置顶 / `useTTSPlayer.phoneme.test.ts` 去 `require()` 改 ESM import / `e2e-11-my-space-contract.architecture.test.ts` 正则非捕获组 + 量词下界 1 | `pnpm lint` 9 errors（exit 1） |
| **（2026-10-13）** a11y 门禁 + 修复 | `a11y-baseline.spec.ts` 断言生效（白名单仅 `color-contrast`）；`nuxt.config.ts` `app.head` 补 `htmlAttrs.lang` + `title` + `description` | RED：6/6 页 `html-has-lang[serious]`+`document-title[serious]` → GREEN：12/12 |
| **（2026-10-13）** 依赖对齐 | `vue-router` `^4.6.3` → **`^5.4.0`**（与 nuxt 4.5.2 的 `vue-router ^5.2.0` 对齐，消除双版本）；`vue-tsc` `^2.1.10` → **`^3.3.13`**（与 vue-router 5 的 volar 插件 API 对齐） | RED：`pnpm typecheck` 报 `Load plugin failed: vue-router/volar/sfc-route-blocks`（旧）/ `ERR_PACKAGE_PATH_NOT_EXPORTED`（半修） → GREEN：exit 0 且无插件报错 |

## 5. 回归钉

- `emotion-echo-web/e2e/a11y-baseline.spec.ts`（6 页 × 2 project = 12 用例）—— **2026-10-13 起为真门禁**（白名单外 critical/serious 即红）
- `emotion-echo-web/e2e/accessibility.spec.ts`（若存在）+ 既有 47 spec
- 既有单测：`pnpm vitest run` = **78 文件 / 658 用例**

## 6. 待决策 / 升级项

**无**（2026-10-13 复验后无未决项）。

## 7. 收口自检

- [x] 9 个测试点全部有结论（**9 PASS / 0 FAIL / 0 BLOCKED / 0 N/A**）
- [x] 范围内缺陷走完 TDD（RED→GREEN，见 §4）
- [x] 回归钉跑过且绿（a11y spec 12/12 + vitest 658/658）
- [x] 改变用户可见行为的修复已实测（a11y 门禁 + 产物 server 真跑）
- [x] 账本对账（`color-contrast` → E2E-F-221，owner = 专项）
- [x] `python scripts/e2e_stage_audit.py --all` → 0 FAIL
- [x] §2.5 收口自检三连
