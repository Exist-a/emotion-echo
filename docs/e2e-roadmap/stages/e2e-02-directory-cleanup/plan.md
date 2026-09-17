---
stage: e2e-02
title: 项目目录清理
type: transformation
status: done
created: 2026-09-17
depends-on: []
blocks: [e2e-03, e2e-05]
gate: []
related-findings: [E2E-F-16, E2E-F-17, E2E-F-18]
---

# E2E-02 🔧 项目目录清理

## 1. 阶段目标

消除仓库噪音，让 `git status` 只反映真实改动，让后续阶段的"文件是否被跟踪"判断不再被污染。本阶段**不碰业务代码**。

## 2. 范围与边界

### 做

| # | 对象 | 现状（已核实） | 处置 |
|---|------|--------------|------|
| 1 | `.mimosa/`（根、`deploy/apisix/`、`emotion-echo-web/` 三处） | hook 运行时状态，**未被 gitignore** → 每次 `git status` 出现 `?? .mimosa/` | 加入 `.gitignore` |
| 2 | `emotion-echo-web;D`（根） | **0 字节空目录**，`mkdir` 时 shell 分号未转义产物 | 删除 |
| 3 | `docker-images-before.txt`（根） | 一次性镜像快照（2026-09-09），已被 gitignore | 删除 |
| 4 | `gui-test-screenshots/`（根） | **25 个文件已被 git 跟踪**，测试证据散落根目录 | 归档到 `docs/evidence/`（保留 git 跟踪），并更新引用它的文档 |
| 5 | `tmp/`（根） | 已 gitignore | 确认可清空 |
| 6 | 根 `node_modules/` | 已 gitignore，但根目录无 `package.json` | 确认可删 |
| 7 | `.zcode/plans/` | 会话计划 md，已 gitignore | **保留**（工具产物） |

### 不做（边界）

- 不动任何业务代码、配置、测试
- 不重命名现有目录结构
- 不清理 `.git` 历史（历史归档另议）

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| 无依赖阶段 | ✅ 可立即执行 |
| 确认 `gui-test-screenshots/` 是否被代码/文档引用 | 需先 grep |

## 4. 测试点清单

本阶段是改造，验证点是"变更后仓库仍健全"。

判定标记：`[A]` 自动可判 · `[V]` 视觉判定 · `[M]` 需人工裁定（详见 [RUNBOOK.md](../../RUNBOOK.md) §4）。

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 1 | 清理前基线记录 | [A] | `git status --short` 输出存档 | 输出文本 | ⬜ |
| 2 | `.mimosa/` 不再出现在 `git status` | [A] | 清理后重跑 `git status --short`，断言无 `.mimosa` | 输出对比 | ⬜ |
| 3 | 空目录与一次性产物已删 | [A] | `ls` 断言不存在 | 输出 | ⬜ |
| 4 | `gui-test-screenshots/` 迁移后无引用断裂 | [A] | grep 全仓引用，断言无悬空路径 | grep 输出 | ⬜ |
| 5 | 清理后测试仍全绿 | [A] | `go test ./...` + `pnpm test` + `pnpm playwright test` | 退出码 | ⬜ |
| 6 | 清理后 dev 环境仍可启动 | [A] | compose up 后 6 服务 healthy | `docker ps` | ⬜ |

## 5. 验收标准（DoD）

- [ ] 6 个测试点通过
- [ ] `git status` 干净（除有意保留项）
- [ ] 清理后全量测试与 dev 启动无回归
- [ ] 引用变更（如截图路径）已在文档中同步

## 6. 已知风险

| 风险 | 应对 |
|------|------|
| `gui-test-screenshots/` 移动会改 25 个文件的 git 路径，可能破坏历史文档中的相对引用 | 移动前先 grep 引用；优先用 `git mv` 保留历史 |
| 误删仍被使用的文件 | 每项删除前先在 report 里记录"已 grep 确认无引用" |

## 7. 产出物

- 更新后的 `.gitignore`
- 执行记录：`stages/e2e-02-directory-cleanup/report.md`（含清理前后 `git status` 对比）
