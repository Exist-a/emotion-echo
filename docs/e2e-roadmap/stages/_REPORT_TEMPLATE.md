---
stage: e2e-NN
title: <阶段名>
executed: YYYY-MM-DD
status: done | partial | blocked
environment: dev 模式（N 容器 healthy，compose.dev.yml + .env.local）
---

# E2E-NN 执行记录（report）

> 模板。与本目录 `plan.md` 同目录存放。填写规则见 [RUNBOOK.md](../RUNBOOK.md) §10。

## 1. 环境基线

- 启动命令：<原样记录>
- 容器状态：<healthy 数 / 异常项>
- 声明的配置差异：<如 `BFF_DEV_RETURN_CODE=1`、`BFF_TRUST_APISIX=true` 等 dev-only 覆盖>

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | | [A] | PASS | | |

汇总：**PASS x / FAIL y / BLOCKED z / N/A w**

> `BLOCKED` 数 > 总数 1/3 时不得判 done（RUNBOOK §4）

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| | 范围内 / 范围外 | 修复 commit `<sha>` / 账本 `E2E-F-NN` |

## 4. 修复清单（TDD 记录）

| commit | 内容 | 先行的失败测试 |
|--------|------|---------------|
| | | |

## 5. 回归钉

- 新增 spec：`emotion-echo-web/e2e/<name>.spec.ts`（用例数 n，首次运行结果：绿/红）

## 6. 待决策 / 升级项

<无则写"无">

## 7. 收口自检

- [ ] `git status` 干净（或改动是有意保留的）
- [ ] `git status -sb` main 与 origin/main 无 ahead/behind
- [ ] `git branch --merged main` 除 main 外为空
