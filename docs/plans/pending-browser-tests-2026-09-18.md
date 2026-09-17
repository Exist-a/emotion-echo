---
status: pending
priority: high
created: 2026-09-18
type: test-plan
related:
  - sprint-110-a8-a9-a10-fix-2026-09-17.md
  - stage-110-a8-a9-a10-fix-2026-09-17.md
---

# 待执行：浏览器验收测试（A8/A9/A10）

## 背景

Sprint 110 代码修复已完成，Playwright E2E 测试 4/4 PASS，但 **IAB 内置浏览器验收测试未执行**。

原因：2026-09-18 会话中 `mcp__node_repl__js` MCP 工具不可用（browser-use 插件运行时桥接器未加载）。

## 待测试项

| # | 测试项 | 验证点 | 优先级 |
|---|--------|--------|--------|
| 1 | 登录流程 | 演示账号登录 → 跳转到 /chat/conversation/new | 高 |
| 2 | A8：聊天 + AI 回复 | 发送消息后 5s 内看到 .dialog-ai + 非空文字 | 高 |
| 3 | A9：sidebar 布局 | 标题与 fold-btn 间距均匀，无挤压 | 中 |
| 4 | A10：voice-btn 样式 | 浅绿色 + 32x32 正方形 | 中 |

## 测试步骤

### 测试 1：登录
1. 打开 `http://localhost:3000/login`
2. 点击「用演示账号快速体验」
3. **预期**：跳转到 `/chat/conversation/new`

### 测试 2：A8 验证
1. 输入框输入：`你好，请用一句话介绍你自己`
2. 点击「发送」
3. **预期**：5s 内看到 AI 回复气泡，内容非空
4. **截图**：保存到 `docs/evidence/gui-test-screenshots/a8-verified.png`

### 测试 3：A9 验证
1. 查看左侧 sidebar
2. **预期**：标题「会话」与折叠按钮间距均匀

### 测试 4：A10 验证
1. 查看聊天输入框右侧语音按钮
2. **预期**：浅绿色、32x32 正方形

## 执行方式

在 ZCode 桌面应用中执行：
```
用内置浏览器打开 http://localhost:3000/login，执行以下测试：
1. 用演示账号登录
2. 发送"你好"测试 AI 回复（A8）
3. 检查 sidebar 布局（A9）
4. 检查 voice-btn 样式（A10）
完成后截图保存到 docs/evidence/gui-test-screenshots/
```

## 前置条件

- 后端容器 healthy（`docker ps` 检查）
- 前端可达（`curl -s -o /dev/null -w "%{http_code}" http://localhost:3000/` 返回 301）

## 验收标准

- [ ] 4 项测试全部 PASS
- [ ] 截图已保存
- [ ] 更新 stage-110 状态（如需要）