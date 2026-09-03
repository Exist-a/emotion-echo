# Emotion-Echo · Stage 1 完成报告

> ⚠️ **架构决策请看 [architecture-decisions.md](./architecture-decisions.md)（ADR）**。
> 本文档保留为历史过程记录（2026-07-13 当时状态）。
> **当前已变更**：go-zero → Gin；Nacos → 删除（详见 ADR）。

> 2026-07-13：5 个微服务全部接上真 DB + Nacos + SkyWalking + APISIX 网关。

## 🏆 Stage 1.6~1.8 战果

| svc | DB schema | TDD 测试 | dbOk | e2e 网关 |
|-----|---------|---------|------|---------|
| user-svc | emotion_echo_user | 8 PASS | ✅ | ✅ |
| assessment-svc | emotion_echo_assessment | 5 PASS | ✅ | ✅ |
| chat-svc | emotion_echo_chat | 3 PASS | ✅ | ✅ |
| ai-svc | emotion_echo_ai | 3 PASS | ✅ | ✅ |
| analytics-svc | emotion_echo_analytics | 3 PASS | ✅ | ✅ |

**总计：22 PASS** （assessment-svc +5, chat-svc +3, ai-svc +3, analytics-svc +3）

## 🔴🟢 TDD 全景（累计）

```
emotion-echo-shared/pkg/discovery      3 PASS
emotion-echo-shared/pkg/messaging      5 + 2 集成 PASS
emotion-echo-user-svc/internal/repository  5 PASS
emotion-echo-user-svc/internal/logic   3 PASS
emotion-echo-assessment-svc/repository 5 PASS
emotion-echo-assessment-svc/logic      1 PASS
emotion-echo-chat-svc/repository       3 PASS
emotion-echo-chat-svc/logic            1 PASS
emotion-echo-ai-svc/repository         3 PASS
emotion-echo-ai-svc/logic              1 PASS
emotion-echo-analytics-svc/repository 3 PASS
emotion-echo-analytics-svc/logic       1 PASS
                                  ─────────
                                   35 PASS + 2 集成
```

## 🌐 e2e 验证（5 个 svc 全 HTTP 200 + dbOk:true）

```bash
curl http://localhost:9080/user-health
curl http://localhost:9080/assessment-health
curl http://localhost:9080/chat-health
curl http://localhost:9080/ai-health
curl http://localhost:9080/analytics-health
```

每个响应形如：
```json
{
  "status": "ok",
  "time": 1783999484541,
  "service": "emotion-echo-XXX-svc",
  "version": "0.1.1",
  "dbOk": true
}
```

## 🎯 这一轮踩到的坑（值得记住）

| 坑 | 修复 |
|---|------|
| `gorm.io/gorm/datatypes` 在 v1.31.2 没了 | 自己写 JSONMap 类型 |
| goctl 重新生成会覆盖 config.go / etc yaml | 重新跑后要恢复 Postgres 字段 |
| PowerShell Yaml 字符串里加 Postgres 段会重复 | 重写 yaml 整文件 |
| gorm driver/postgres v1.2.3 与 gorm v1.31 不兼容 | 显式 pin v1.6.0 |
| genproto 在 go mod tidy 时冲突 | 显式锁定 indirect 版本 |

## 📁 新增文件（这一轮）

```
emotion-echo-assessment-svc/internal/
  ├── model/survey.go                  ← Survey + SurveyResult + JSONMap
  ├── repository/survey_repository.go  ← Interface + InMemory + Postgres
  └── repository/survey_repository_test.go  ← 5 PASS

emotion-echo-chat-svc/internal/
  ├── model/conversation.go
  ├── repository/conversation_repository.go
  └── repository/conversation_repository_test.go  ← 3 PASS

emotion-echo-ai-svc/internal/
  ├── model/emotion.go
  ├── repository/emotion_repository.go
  └── repository/emotion_repository_test.go  ← 3 PASS

emotion-echo-analytics-svc/internal/
  ├── model/event.go
  ├── repository/event_repository.go
  └── repository/event_repository_test.go  ← 3 PASS
```

每个 svc 还在以下位置加了 DB 接线：
- `internal/config/config.go` 加 Postgres struct + Config 字段
- `etc/{svc}-api.yaml` 加 Postgres 段
- `{svc}.go` main 加 `openPostgres()` 调用

## 📊 当前进度

```
Phase 0 基础设施    ████████████████████ 100% ✅
Phase 1 go-zero     ████████████████████ 100% ✅ (5/5 svc, 5/5 接 DB)
Phase 2 Kafka       ████████░░░░░░░░░░░  40%
Phase 3 韧性         ░░░░░░░░░░░░░░░░░░░░   0%
Phase 4 业务深化      ░░░░░░░░░░░░░░░░░░░░   0%
Phase 5 K8s          ░░░░░░░░░░░░░░░░░░░░   0%
```

## 🚀 Stage 2 候选

下一步重点 = **Phase 2 Kafka 异步化**：

- [ ] proto 接口定义（chat-svc 内 Conversation ↔ Message + ai-svc.Analyze）
- [ ] Kafka topic 声明：user-events / chat-events / ai-events
- [ ] chat-svc.SendMessage → 异步调 ai-svc（Kafka 事件）
- [ ] ai-svc consumer 消费 chat-events，写 emotion_analysis 表
- [ ] SSE/WebSocket 推送给前端

## 🎓 这次的核心认知

1. **重复是抽象的信号** — user-svc 的 Repository 模式被复制 4 次都没改，说明它已经稳定
2. **goctl 不会保留外部字段** — 它的生成器不知道你加的 Postgres，重跑就丢
3. **gorm schema-qualified** — `TableName() string` 返回 `emotion_echo_user.users` 是反 search_path 陷阱的最稳办法
4. **JSONMap 自己写 20 行** — 比依赖 `gorm.io/gorm/datatypes` 模块更可控
5. **DB health 通过 Ping() 而非 SELECT 1** — Ping 用的是底层 ping，比查询更轻量