---
stage: e2e-10
title: 聊天核心链路 — 落地文档
status: done
date: 2026-09-19
type: landing
---

# E2E-10 聊天核心链路 — 落地文档

## 1. 阶段概要

| 维度 | 结果 |
|------|------|
| Playwright 回归钉 | 12/12 PASS（chromium） |
| 全链路验证 | ✅ BFF→chat-svc→llm-service→DeepSeek API |
| 数据库落库 | ✅ user + assistant 消息均持久化 |
| 发现并修复 bug | 1 个（LLM_API_KEY 覆盖，E2E-F-79） |
| §2.4 契约 smoke | 4/6 PASS，1 FAIL（视图不存在），1 WARN（emotionDistribution 空） |
| 边界测试 | 3/3 PASS（空消息/超长/并发） |

## 2. 全链路验证证据

### 2.1 调用链

```
用户输入 → [id].vue handleSubmit
  → useConversationSender.sendToExistingConversation
    → messageStore.sendMessage → POST /api/v1/conversations/:id/messages
      → BFF chat_handler.sendMessage → chat-svc gRPC SendMessage ✅
    → useAIStreamHandler.sendAIStream → POST /api/v1/ai/stream
      → BFF ai_stream_handler.ServeHTTP
        → Tier 1: llm-service gRPC ChatCompletion ✅
          → iter_chat_chunks() → OpenAI SDK → DeepSeek API (200 OK) ✅
        → saveAIMessage → chat-svc gRPC SendMessage ✅
      → SSE: data: {"choices":[{"delta":{"content":"..."}}]} ✅
      → SSE: data: [DONE] ✅
    → onDelta: 累积文字 → 更新 AI 气泡
    → onFinish: 替换消息 ID，标记 sent
```

### 2.2 Docker 日志证据

**llm-service**（真实 LLM 调用）：
```
HTTP Request: POST https://api.deepseek.com/chat/completions "HTTP/1.1 200 OK"
```

**BFF**（gRPC 调用链）：
```
[grpc-client] SendMessage latency=11ms err=nil
[grpc-client] ListMessages latency=1ms err=nil
[grpc-client] SendMessage latency=9ms err=nil  ← saveAIMessage
```

**chat-svc**（持久化）：
```
[grpc-server] SendMessage code=OK err=nil
```

### 2.3 数据库证据

```sql
SELECT id, role, LEFT(content, 60), intent FROM emotion_echo_chat.messages WHERE conversation_id = 242;
-- id=219, role=user, content="hello", intent=other
-- id=220, role=assistant, content="你好呀，很高兴你来到这里。不管今天过得怎样..."  ← 真实 LLM
```

## 3. §2.4 数据契约 smoke

| # | 契约 | 结果 | 证据 |
|---|------|------|------|
| 1 | user_behavior_events 行数 ≥ 业务事件数 | ✅ PASS | 163 行 |
| 2 | event_type ≥ 2 种 | ✅ PASS | 7 种（message.created 70, conversation.created 58, conversation.closed 22 等） |
| 3 | analytics_reader 能查视图 | ❌ FAIL | 视图不存在（0 个 view in emotion_echo_analytics） |
| 4 | 报表 summary 非空 + chartData > 0 | ⚠️ WARN | summary 非空 ✅，emotionDistribution=[] ❌（E2E-F-10 已知） |
| 5 | schema 与写入端一致性 | ✅ PASS | content 长度校验(4096) + 空内容校验生效 |
| 6 | KAFKA_ENABLED=false 路径不空跑 | ✅ PASS | dev 模式消息正常写入 DB（163 行事件） |

## 4. 边界测试

| 测试 | 结果 | 证据 |
|------|------|------|
| 空消息 | ✅ 正确拒绝 | `code=1, "content is required"` |
| 超长消息(5000字) | ✅ 正确拒绝 | `code=1, "content too long (5000 > 4096)"` |
| 并发发送(3路) | ✅ 全部成功 | id=222,223,221 无冲突 |

## 5. 发现的问题

### 已修复
- **E2E-F-79**：LLM_API_KEY 被 HOST 空值覆盖 → 删除覆盖行（PR #14）

### 留账（不在 E2E-10 范围）
- **E2E-F-10**：emotionDistribution 永远为空（analytics 读空表）
- **契约3**：analytics_reader 视图不存在（E2E-03/05 范围）

## 6. 收口自检

- [x] 12/12 Playwright 全绿
- [x] 全链路日志验证通过
- [x] 数据库落库验证通过
- [x] §2.4 契约 smoke 已跑（4/6 PASS）
- [x] 边界测试已跑（3/3 PASS）
- [x] 发现问题已分类（1 修复 + 2 留账）
- [x] PR #13 + #14 已合并
- [x] roadmap 状态更新
- [x] discovered-unresolved.md 更新（79 项）
- [x] 收口自检三连通过
