---
stage: e2e-02
title: 项目目录清理
executed: 2026-09-17
status: done
environment: 本地开发（无需容器，纯文件操作）
---

# E2E-02 执行记录

## 1. 环境基线

- 无需启动容器（纯目录清理阶段）
- 清理前 `git status`：4 个 untracked（`.mimosa/` × 3 + stage-109b 截图）+ 1 个 modified（`chat-flow.spec.ts`）

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | 清理前基线记录 | [A] | PASS | `git status --short` 输出：4 untracked + 1 modified | 见上方环境基线 |
| 2 | `.mimosa/` 不再出现在 `git status` | [A] | PASS | 清理后 `git status --short` 无 `.mimosa` 行 | `.gitignore` 新增 `.mimosa/` 规则 |
| 3 | 空目录与一次性产物已删 | [A] | PASS | `emotion-echo-web;D` 和 `docker-images-before.txt` 均 `ls` 不存在 | |
| 4 | `gui-test-screenshots/` 迁移后无引用断裂 | [A] | PASS | grep 全仓旧路径，仅剩 E2E-02 plan/discovered-unresolved 中的"现状描述"（历史记录保留） | 7 处实际引用已更新 |
| 5 | 清理后测试仍全绿 | [A] | PASS | vitest 46/47 PASS，357/357 tests PASS | 1 个预存失败 `useAIStreamHandler.test.ts`（`#app` 解析），非本次引入 → E2E-F-39 |
| 6 | 清理后 dev 环境仍可启动 | [A] | BLOCKED | 未验证（需 Docker 环境，本地未运行） | 不阻塞收口：改动仅涉及 .gitignore、文件移动和注释，不影响运行时 |

汇总：PASS 5 / FAIL 0 / BLOCKED 1 / N/A 0

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| vitest `useAIStreamHandler.test.ts` 预存失败（`#app` import 无法解析） | 范围外（E2E-04 前端工程化门槛） | 账本 E2E-F-39 |

## 4. 清理清单

| commit | 内容 |
|--------|------|
| 8100c5f | `fix(e2e): chat-flow happy-path-2 SSR hydration timing`（E2E-01 遗留） |
| a44ee4a | `chore(e2e-02): 项目目录清理`（.mimosa gitignore + gui-test-screenshots 归档 + 残留清理） |

## 5. 回归钉

本阶段是目录清理，不涉及业务功能变更，**无需新增 Playwright spec**。
已有 spec（login-flow / dashboard-flow / chat-flow / jwt-expiry）不受影响。

## 6. 待决策 / 升级项

无。

## 7. 收口自检

- [x] git status 干净（仅 roadmap/plan/report/账本更新，属本阶段收口动作）
- [x] main 与 origin 已同步（push 后验证）
- [x] 无残留已合并分支
