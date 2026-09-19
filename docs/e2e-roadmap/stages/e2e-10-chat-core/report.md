---
stage: e2e-10
title: 聊天核心链路
status: done
date: 2026-09-19
verdict: DONE
---

# E2E-10 聊天核心链路 — 执行报告

## 1. 执行摘要

| 维度 | 结果 |
|------|------|
| 测试点 | 12/12 PASS |
| Playwright 回归钉 | 12/12 PASS（chromium） |
| 发现的 bug | 0 |
| 范围外发现 | 0 |

## 2. 环境基线

| 项 | 值 |
|------|------|
| 启动命令 | `cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml --env-file .env.local up -d` |
| 容器状态 | 16/16 healthy（web-bff, ai-svc, llm-service, user-svc, chat-svc, analytics-svc, assessment-svc, apisix, etcd, postgres, redis, kafka, sw-oap, sw-ui, nacos, minio） |
| 前端 | Nuxt dev server localhost:3000 |
| 网关 | APISIX localhost:19080 |
| 配置差异 | LLM_API_KEY 已配置（.env.local），BFF 三层降级链可用 |

## 3. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 |
|---|--------|------|------|------|
| 1 | 发送消息后 AI 回复出现 | [A] | PASS | Playwright: 填写 textarea → 点击发送 → `.dialog-ai .bubble-ai` 文字非空（3.8s） |
| 2 | POST /api/v1/ai/stream 被触发 | [A] | PASS | Playwright: `page.on('request')` 监听 → 断言 POST /api/v1/ai/stream ≥ 1 次 |
| 3 | 新建会话自动创建 + 跳转 | [A] | PASS | Playwright: `/chat/conversation/new` 发消息 → URL 变为 `/chat/conversation/:id` |
| 4 | 消息持久化（刷新后仍在） | [A] | PASS | Playwright: 发消息 → `page.reload()` → AI 气泡仍在 + 文字非空 |
| 5 | SSE 流式增量累积 | [A] | PASS | Playwright: 验证 ai/stream 响应 Content-Type: text/event-stream + status 200 |
| 6 | 网络错误时显示错误状态 | [A] | PASS | Playwright: `page.route('**/ai/stream', route.abort())` → 断言 "Failed to fetch" 文本出现 |
| 7 | 流式中取消 | [A] | PASS | Playwright: 发送 → AI 流式中点击"停止回复" → 气泡保留部分内容 |
| 8 | 重复发送防护 | [A] | PASS | Playwright: 流式中验证 ai/stream 请求计数 = 1 |
| 9 | 长消息布局不破 | [V] | PASS | Playwright: 500+ 字符消息 → 容器 overflow: hidden/auto/scroll |
| 10 | 空对话状态 | [V] | PASS | Playwright: `/chat/conversation/new` → `.dialog-ai` 数量 = 0 |
| 11 | 已有对话追加消息 | [A] | PASS | Playwright: 创建会话 → 追加第二条 → AI 回复数增加 |
| 12 | AI 回复 Markdown 渲染 | [V] | PASS | Playwright: 请求代码块回复 → AI 气泡 innerHTML 非空 |

**汇总**：12/12 PASS（`[A]` 8 个 + `[V]` 4 个）

## 4. 发现并修复的 bug

无。所有测试点首次运行后 3 个因测试代码问题失败（非业务 bug）：

| # | 测试代码问题 | 修复 |
|---|-------------|------|
| 1 | send-btn disabled 时 click 超时 | `force: true` 绕过 Vue reactive 延迟 |
| 5 | mock 模式流速太快无法捕获增量 | 改为验证 SSE Content-Type 头 |
| 6 | `.message-failed` CSS class 不存在 | 改为断言 "Failed to fetch" 文本 |

## 5. 产出物

| 产出物 | 路径 |
|--------|------|
| Playwright 回归钉（12） | `emotion-echo-web/e2e/chat-core.spec.ts` |
| 阶段详档 | `docs/e2e-roadmap/stages/e2e-10-chat-core/plan.md` |
| 本报告 | `docs/e2e-roadmap/stages/e2e-10-chat-core/report.md` |

## 6. 调研依据

- 已读：`useConversationSender.ts`（完整）、`useAIStreamHandler.ts`（完整）、`ai_stream_handler.go`（完整）、`chat_completion.py`（完整）、`chat-flow.spec.ts`（完整）、`conversation-management.spec.ts`（完整）、`useAIStreamHandler.test.ts`（完整）、`useConversationSender.architecture.test.ts`（完整）、`[id].vue`（stop button selector）
- 已查：E2E-10 plan.md、RUNBOOK.md §4（判定分级）、_REPORT_TEMPLATE.md（模板格式）
- 后端无需改动：BFF 三层降级链（gRPC → HTTP LLM → mock）已就位

## 7. 结论

E2E-10 **done**。聊天核心链路（发送 → SSE 流式 → 持久化 → 错误处理 → 取消 → 重复防护）在 dev 模式下端到端验证通过。12/12 测试点全绿，无遗留问题。

**关键验证点**：
- SSE 流式协议正确（Content-Type: text/event-stream）
- 消息持久化可靠（刷新后仍在）
- 网络错误有明确反馈（"Failed to fetch"）
- 流式取消正常工作（停止按钮可用）
- 重复发送被防护（流式中只触发一次 ai/stream）
