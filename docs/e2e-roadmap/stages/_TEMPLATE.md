---
stage: e2e-NN
title: <阶段名称>
type: verification | transformation
status: pending | in-progress | done
created: 2026-09-17
depends-on: []
blocks: []
related-findings: []
---

# E2E-NN <阶段名称>

> 详档模板。撰写时机：该阶段启动前 1 个阶段时（见 [roadmap.md](../roadmap.md) §详档约定）。
> 执行完成后，在同一目录追加 `report.md` 记录实测结果、修复清单、截图证据。

## 1. 阶段目标

一句话说明这个阶段要让什么从"不确定"变成"已验证"（或从"坏"变成"好"）。

## 2. 范围与边界

### 做

- 具体到文件/端点/行为

### 不做（边界）

- 明确列出，防止范围蔓延

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| 依赖阶段完成 | E2E-XX |
| 环境（dev 模式容器 / 特定 profile） | |
| 数据准备（种子数据 / 测试账号） | |

环境启动命令（**必须带 `--env-file .env.local`**，见 AGENTS.md §四）：

```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local up -d
```

## 4. 测试点清单

每个测试点必须有**代码验证**（只读 DOM/接口）与**视觉验证**（截图并被查看）双重证据。

| # | 测试点 | 验证方式 | 证据 | 结果 |
|---|--------|---------|------|------|
| 1 | | | 截图 + 断言 | ⬜ |

## 5. 验收标准（DoD）

- [ ] 全部测试点通过（或发现问题已分类：范围内修复 / 范围外记账本）
- [ ] 修复项走完 TDD（Red → Green → Refactor）
- [ ] 已验证行为固化为 Playwright spec 回归钉
- [ ] roadmap 状态更新 + 账本更新
- [ ] §2.5 收口自检三连通过

## 6. 已知风险

| 风险 | 应对 |
|------|------|

## 7. 产出物

- Playwright spec：`emotion-echo-web/e2e/<name>.spec.ts`
- 执行记录：`stages/<本目录>/report.md`
- 截图：`stages/<本目录>/screenshots/`
