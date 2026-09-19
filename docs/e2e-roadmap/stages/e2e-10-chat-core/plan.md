---
stage: e2e-10
title: 聊天核心链路
type: verification
status: partial
created: 2026-09-19
depends-on: [e2e-01, e2e-08]
blocks: [e2e-15, e2e-16, e2e-17]
gate: []
related-findings: []
---

# E2E-10 聊天核心链路

> 详档。执行前必读 [RUNBOOK.md](../RUNBOOK.md)。
> 调研依据：已读 useConversationSender.ts、useAIStreamHandler.ts、ai_stream_handler.go、chat_completion.py、chat-flow.spec.ts、conversation-management.spec.ts、useAIStreamHandler.test.ts、useConversationSender.architecture.test.ts

## 1. 阶段目标

验证聊天核心链路（发送消息 → SSE 流式 AI 回复 → 消息持久化 → 错误处理 → 中断重试）在 dev 模式下端到端可用，将"用户能正常聊天"从"假设"变成"已验证"。

## 2. 范围与边界

### 做

- 发送文本消息 + AI 回复出现（happy path）
- SSE 流式增量累积（文字逐步出现，非一次性）
- 新建会话自动创建 + 跳转
- 消息持久化（刷新后仍在）
- 流式中取消（abort）
- 网络/服务端错误处理
- 重复发送防护
- 长消息布局不破
- 空对话状态

### 不做（边界）

- 多模态（语音/表情/文件上传）→ E2E-16
- 数字人 / TTS 口型同步 → E2E-17
- 情绪分析准确性 → E2E-15
- 消息内容同步 / 跨设备 → E2E-08 已覆盖
- 性能 / 延迟基线 → E2E-28

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| E2E-01 登录会话 | ✅ done |
| E2E-08 历史会话管理 | ✅ done |
| dev 环境容器 healthy | 待执行时验证 |
| 演示账号 `echo` / `echo123` | ✅ 存在 |

环境启动命令（**必须带 `--env-file .env.local`**）：

```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local up -d
```

## 4. 测试点清单

判定标记：`[A]` 自动可判 · `[V]` 视觉判定 · `[M]` 需人工裁定。详见 [RUNBOOK.md](../RUNBOOK.md) §4。

| # | 测试点 | 判定 | 验证方式 | 证据 | 结果 |
|---|--------|------|---------|------|------|
| 1 | 发送消息后 AI 回复出现 | [A] | Playwright: 填写 textarea → 点击发送 → 断言 `.dialog-ai` 出现 + `.bubble-ai` 文字非空（5s 超时） | spec + 截图 | ⬜ |
| 2 | POST /api/v1/ai/stream 请求被触发 | [A] | Playwright: `page.on('request')` 监听，断言发送消息后发起 `POST /api/v1/ai/stream` | spec | ⬜ |
| 3 | 新建会话自动创建 + 跳转 | [A] | Playwright: 在 `/chat/conversation/new` 发送消息 → 断言 URL 变为 `/chat/conversation/:id` + 对话列表出现新条目 | spec + 截图 | ⬜ |
| 4 | 消息持久化（刷新后仍在） | [A] | Playwright: 发送消息 → `page.reload()` → 断言 `.dialog-ai` 仍在 + `.bubble-ai` 文字非空 | spec | ⬜ |
| 5 | SSE 流式增量累积 | [A]+[V] | Playwright: 发送消息 → 轮询 `.bubble-ai` 文字长度 → 断言长度递增（非一次性出现）+ 截图验证文字逐步增长 | spec + 截图 | ⬜ |
| 6 | 网络错误时显示错误状态 | [A] | Playwright: `page.route('**/api/v1/ai/stream', route => route.abort())` → 发送消息 → 断言消息状态变为失败（`.message-failed` 或状态文字） | spec | ⬜ |
| 7 | 流式中取消 | [A] | Playwright: 发送消息 → AI 流式中点击停止按钮 → 断言流停止 + AI 气泡保留已收到的部分内容 | spec + 截图 | ⬜ |
| 8 | 重复发送防护 | [A] | Playwright: AI 流式中再次点击发送 → 断言不发起第二次 `POST /api/v1/ai/stream`（请求计数） | spec | ⬜ |
| 9 | 长消息布局不破 | [V] | Playwright: 发送 500+ 字符消息 → 断言 `.login-card` / 消息容器 `overflow: hidden` + 截图确认无溢出 | spec + 截图 | ⬜ |
| 10 | 空对话状态 | [V] | Playwright: 进入 `/chat/conversation/new` → 断言无历史消息 + 截图确认空态 UI 正常 | spec + 截图 | ⬜ |
| 11 | 已有对话追加消息 | [A] | Playwright: 进入已有对话 → 发送新消息 → 断言消息列表末尾出现用户消息 + AI 回复 | spec | ⬜ |
| 12 | AI 回复 Markdown 渲染 | [V] | Playwright: AI 回复含代码块/粗体时 → 截图确认渲染正确（不验证内容准确性，只验证渲染不崩溃） | 截图 | ⬜ |

**汇总**：12 个测试点，`[A]` 8 个，`[V]` 4 个，`[M]` 0 个。

## 5. 验收标准（DoD）

- [ ] 12 个测试点全部通过（或发现问题已分类：范围内修复 / 范围外记账本）
- [ ] 修复项走完 TDD（Red → Green → Refactor）
- [ ] 已验证行为固化为 Playwright spec 回归钉（`e2e/chat-core.spec.ts`）
- [ ] roadmap 状态更新 + 账本更新
- [ ] §2.5 收口自检三连通过

## 6. 已知风险

| 风险 | 应对 |
|------|------|
| AI 服务不可用（LLM_API_KEY 未配） | BFF 三层降级：gRPC → HTTP LLM → mock。mock 模式下仍能验证前端链路 |
| SSE 流式测试依赖网络延迟 | 使用轮询 + 合理超时（5-10s），非精确时间断言 |
| 取消按钮可能无独立 selector | 用 `[V]` 截图确认，或用 `data-testid` 补标记 |
| 测试 #6 路由拦截可能影响其他请求 | 拦截范围限定为 `**/api/v1/ai/stream` |

## 7. 产出物

- Playwright spec：`emotion-echo-web/e2e/chat-core.spec.ts`
- 执行记录：`stages/e2e-10-chat-core/report.md`
- 截图：`stages/e2e-10-chat-core/screenshots/`
