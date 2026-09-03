# Emotion-Echo · Stage 4 情绪查询闭环

> ⚠️ **架构决策请看 [architecture-decisions.md](./architecture-decisions.md)（ADR）**。
> 本文档保留为历史过程记录（2026-07-14 当时状态）。

> 2026-07-14：ai-svc 提供情绪分析查询端点；前端可查询任意消息/会话的分析结果。

## 🏆 战果

| 项 | 内容 |
|----|------|
| 新增端点 | `GET /api/v1/emotion/message/:messageId`<br>`GET /api/v1/emotion/conversation/:conversationId` |
| 业务闭环 | 消息发出去 → ai-svc 异步分析 → 用户随时查询分析结果 |
| TDD 测试 | repository +5 (GetByMessageID/ListByConversationID)；logic +4 |
| 端到端验证 | 通过 APISIX 网关返回 4 条历史情绪分析 |

## 🔴🟢 TDD 闭环

```
🔴 RED：undefined: GetByMessageID / ListByConversationID
🟢 GREEN：
  ├─ TestEmotionRepo_InMemory_GetByMessageID PASS
  ├─ TestEmotionRepo_InMemory_GetByMessageID_NotFound PASS
  ├─ TestEmotionRepo_InMemory_ListByConversationID PASS
  ├─ TestEmotionRepo_InMemory_Ping_OK PASS
  ├─ TestGetEmotionByMessageLogic_Existing PASS
  ├─ TestGetEmotionByMessageLogic_NotFound_Returns404 PASS
  ├─ TestListEmotionByConversationLogic_ReturnsAll PASS
  └─ TestListEmotionByConversationLogic_EmptyConv_ReturnsEmptyList PASS

e2e 证据：APISIX 网关返回 4 条 emotion_analysis 数据
```

## 🎯 端到端 e2e 验证

```
$ curl http://localhost:9080/api/v1/emotion/conversation/1

{
  "emotions": [
    {"id":1,"messageId":1,"conversationId":1,"userId":1,
     "primaryEmotion":"happy","sentimentScore":0.60,
     "model":"keyword-stub-v1","createdAt":1784001542519},
    {"id":2,"messageId":2,"conversationId":1,"userId":1,
     "primaryEmotion":"anxious","sentimentScore":-0.40,
     "model":"keyword-stub-v1","createdAt":1784001542753},
    {"id":3,"messageId":3,"conversationId":1,"userId":1,
     "primaryEmotion":"neutral","sentimentScore":0.50,
     "model":"keyword-v1","createdAt":1784002106362},
    {"id":4,"messageId":4,"conversationId":1,"userId":1,
     "primaryEmotion":"anxious","sentimentScore":0.00,
     "model":"keyword-v1","createdAt":1784002106666}
  ]
}
```

**业务解读**：
- msg 1-2: Stage 2 时期用 Go keyword-stub-v1 分析
- msg 3-4: Stage 3 时期用 Python emotion-llm-service keyword-v1 分析
- 一个会话可看到情绪变化趋势

## 📁 新增/修改文件

```
emotion-echo-ai-svc/
├── ai.api                                ← 加 2 个查询端点
├── internal/
│   ├── repository/
│   │   ├── emotion_repository.go         ← 加 GetByMessageID/ListByConversationID
│   │   └── emotion_repository_test.go    ← 加 4 个测试
│   └── logic/
│       ├── getemotionbymessagelogic.go   ← 新端点实现
│       ├── listemotionbyconversationlogic.go
│       └── querylogic_test.go            ← 4 个测试

emotion-echo-ai-svc/etc/ai-api.yaml       ← 已含 Postgres/Kafka/LLM 配置

apisix-r-emo-by-msg.json                 ← GET /api/v1/emotion/message/*
apisix-r-emo-by-conv.json                ← GET /api/v1/emotion/conversation/*
```

## 🎓 白盒审计要点

1. **godoc 完整** — EmotionRepo 接口每个方法都有注释
2. **nil-safe 返回** — GetByMessageID 不存在返回 (nil, nil) 而非 error
3. **非 nil slice** — ListByConversationID 返回 `make([]T, 0, n)` 即使空也是空 slice
4. **404 显式** — GetEmotionByMessage 找不到返回 errors.New("not found: ...")
5. **in-memory 索引** — byMessageID map 加速查询

## ⚠️ 这一轮踩到的坑

1. **docker-compose 全部停机** — Nacos 挂了导致 ai-svc log.Fatalf；docker ps 一片空白
2. **APISIX 路由全部丢失** — etcd 容器重启清空状态；需要重新注册所有路由
3. **Write 工具的 Chinese 路径** — 在 PS 脚本中 `D:\源码` 被替换为 `D:\婧愮爜`；改用 `$env:EE_ROOT` 内联传递
4. **PowerShell `$svc:` 变量解析** — PS 把 `$svc:` 解释为驱动器；改用 `${svc}:` 大括号
5. **PowerShell `?:` 三元不支持** — 用 if-else 替代
6. **proxy-rewrite no-op 触发 301** — APISIX 把同路径 rewrite 视为重定向；删除该插件

## 📊 当前进度

```
Phase 0 基础设施    ████████████████████ 100% ✅
Phase 1 go-zero     ████████████████████ 100% ✅
Phase 2 Kafka       ████████████████████ 100% ✅
Phase 3 韧性         ███░░░░░░░░░░░░░░░░  20%  ← 仍缺 DLQ
Phase 4 业务深化      █████████████░░░░░  75% ← 查询闭环完成
Phase 5 K8s          ░░░░░░░░░░░░░░░░░░░░   0%
```

## 🎯 TDD 累计

```
70 PASS（+ 2 集成）
shared:        3 + 5 + 2 集成
user-svc:      5 + 3
assessment-svc: 5 + 1
chat-svc:      5 + 8
ai-svc:        5 + 5 + 4 + 4  ← 仓库 +4 / logic +4
analytics-svc: 3 + 1
```

## 🚀 下一步候选

- **A**：DLQ + 重试（Phase 3 韧性补完）
- **B**：analytics-svc 接 consumer（行为分析）
- **C**：docker-compose 加 emotion-llm-service（容器化）
- **D**：前端 WebSocket 推送新分析（实时）
- **E**：接真实 LLM API

A / B / C / D / E？