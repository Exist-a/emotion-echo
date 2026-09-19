---
status: planned
priority: high
created: 2026-09-19
type: sprint-plan
related:
  - e2e-roadmap/stages/e2e-09-registration/plan.md
  - e2e-roadmap/stages/e2e-09-registration/report.md
  - e2e-roadmap/roadmap.md
---

# E2E-09 收口 + 分支清理 + E2E-10 规划

## 背景

E2E-09（注册流程）已实现核心功能（验证码删除 + 密保弹框 + 注册成功），Playwright 回归钉 10/10 PASS，Vitest 契约测试 20/20 PASS。但 4 个测试点未覆盖：

| # | 测试点 | 缺口原因 | 可行方案 |
|---|--------|---------|---------|
| 5 | 重复用户名返回错误 | 需 dev DB | ✅ 可补：Playwright 注册 → 再注册同名 → 断言错误提示 |
| 10 | 密保答案不明文落库 | 需 DB 断言 bcrypt | ✅ 可补：契约测试验证代码使用 bcrypt.hash() |
| 12 | 注册后可直接找回密码 | 需端到端联动 | ⬜ 留账：需 dev 环境 + E2E-07 联动 |
| 13 | 并发注册同名 | 需并发 + DB UNIQUE | ⬜ 留账：需 dev 环境并发测试 |

## 执行计划

### Step 1: 补完 #5（重复用户名）— Playwright

在 `e2e/registration.spec.ts` 新增测试：
- 注册用户 `e2e_dup_${Date.now()}`
- 等待跳转成功
- 回到注册页，用相同用户名再注册
- 断言：错误提示出现（如"用户名已存在"），不跳转

### Step 2: 补完 #10（bcrypt 契约测试）— Vitest

在 `registration-security-question.test.ts` 新增 describe：
- 读取 BFF register handler 源码（`auth_handler.go` 或 `authlogic.go`）
- 断言使用了 `bcrypt.GenerateFromPassword` 或 `bcrypt.Hash`
- 断言未出现明文存储（`security_answer` 字段赋值处不是直接 `.Answer = answer`）

### Step 3: 更新 E2E-09 report.md

- 更新测试点表格：#5/#10 标 PASS
- 更新摘要：12/14 PASS
- 保留 #12/#13 为"留账"并关联 E2E-F 编号

### Step 4: 合并分支到 main

```bash
git checkout main
git merge feat/e2e-09-registration --no-ff -m "feat(e2e-09): 注册流程完整实现（12/14 测试点）"
```

### Step 5: 清理分支

```bash
# 本地
git branch -d feat/e2e-09-registration
# 远端
git push origin --delete feat/e2e-09-registration
# 其他已合并远端分支
git push origin --delete docs/r-series-closure fix/r-series-blockers fix/secrets-and-ci-hardening fix/secrets-coverage
```

### Step 6: 收口自检

```bash
git status                    # working tree 干净
git status -sb                # main 与 origin/main 无 ahead/behind
git branch --merged main      # 除 main 外应为空
```

### Step 7: 更新 roadmap.md

- E2E-09 状态：`partial` → `done`（12/14，2 项留账）
- 激活阶段指针：E2E-09 → E2E-10

### Step 8: 登记遗留到 discovered-unresolved.md

- E2E-F-xx: #12 注册后找回密码联动验证（阻塞条件：dev 环境可用）
- E2E-F-xx: #13 并发注册同名验证（阻塞条件：dev 环境可用）

## 范围外（不修，只记账）

- R-02/R-03 契约欠账（report 模板化 / [V] 截图 / 账本对账）
- E2E-F-68 main 分支写保护自锁死
- A8/A9/A10 浏览器验收（留到 E2E-10 前执行）

## 验收标准

- [ ] #5 Playwright 测试 PASS
- [ ] #10 Vitest 契约测试 PASS
- [ ] E2E-09 report.md 更新为 12/14
- [ ] 分支合并到 main
- [ ] 所有已合并分支清理完毕（本地 + 远端）
- [ ] 收口自检三连全过
- [ ] roadmap.md 状态更新
- [ ] 遗留 2 项登记到 discovered-unresolved.md
