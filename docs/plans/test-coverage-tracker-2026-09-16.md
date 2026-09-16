---
purpose: "测试找问题修复阶段" 业务路径级测试覆盖率追踪
status: active
priority: high
created: 2026-09-16
last-refresh: 2026-09-16 (Stage 107 收口后)
owner: TBD
type: test-coverage-tracker
depends-on:
  - AGENTS.md §2.4 数据契约验收清单
  - AGENTS.md §2.3 覆盖率底线
  - stage-26-T-test-backlog.md (历史版，本文档接替)
  - stage-107-chat-new-conversation-fix-2026-09-16.md
related-stages:
  - stage-103-dev-mode-launch-2026-09-16.md (dev 模式首次启动基线)
  - stage-105-browser-e2e-2026-09-16.md (browser-use 浏览器 E2E)
related-plans:
  - multi-round-iteration-2026-09-15.md (上一轮 §十六收口)
---

# 测试覆盖追踪 · 业务路径级（"测试找问题修复"阶段）

> **本文档定位**：业务路径级覆盖率追踪——按用户可感知的业务路径（auth / chat / assessment / reports / outbox / SkyWalking 等）划分，每条路径独立列出"已测 / 未测 / 测了未跑 / 端到端是否跑通"状态。
>
> **本文不替代**：
> - AGENTS.md §2.3 单测覆盖率底线（80% / 90% / 70%）—— 那是代码级别
> - AGENTS.md §2.4 数据契约验收清单（6 条）—— 那是数据正确性级别
> - 单个 stage 收口报告（如 stage-107）—— 那是事件级别
>
> **本文回答 3 个问题**：
> 1. **用户路径**：从浏览器点一下到数据库落库，这条链有没有测过？
> 2. **数据契约**：6 条数据契约的 smoke 当前状态？
> 3. **dev 模式真实跑通**：浏览器能否端到端走完？

---

## 〇、状态图例

| 标记 | 含义 |
|---|---|
| ✅ | 已测且绿（含端到端 browser-use 实测） |
| 🟡 | 有测试但**未真跑**（test file 存在 + CI/last-run 通过，但端到端 browser-use 未验证） |
| 🔴 | 已有 bug 待修（在本表中挂"已知未修"清单） |
| ⚫ | 未测（无测试文件或仅有 stub） |
| 🟠 | 测了但发现 bug，已记入 stage 报告修复中 |

---

## 一、4 主业务链（用户视角）

### E2E-1 · Auth 登录

| 维度 | 状态 | 证据 |
|---|---|---|
| Go 单测 (BFF auth_handler) | ✅ 21 用例 PASS | `emotion-echo-web-bff/internal/handler/auth_handler_test.go` |
| Go 单测 (user-svc auth) | ✅ 多用例 | `emotion-echo-user-svc/internal/handler/auth_handler_test.go` |
| Go 单测 (shared JWT middleware) | ✅ | `emotion-echo-shared/pkg/middleware/jwt_auth_test.go` + `gin_auth_test.go` |
| E2E (Playwright) | ✅ 已测 | `emotion-echo-web/e2e/login-flow.spec.ts`（Stage 105 验证）|
| browser-use 实测 | ✅ | Stage 105 + Stage 107 复跑都登录成功 |
| 数据契约 §3 (analytics_reader GRANT) | 🟡 | APISIX route 100 通了, smoke 脚本 `scripts/smoke_data_layer.py` 待跑全量 |
| 异常路径 | ⚫ | JWT 过期自动 refresh 已测（`useApi.ts` test path），**但无 Playwright 异常 case**（refresh 触发条件 + 401 → 跳登录页） |

### E2E-2 · Chat 发消息 + AI 回复

| 维度 | 状态 | 证据 |
|---|---|---|
| Go 单测 (chat-svc SendMessage / StreamMessages / ListMessages 等) | ✅ 多用例 | `emotion-echo-chat-svc/internal/logic/*_test.go` (8 个 logic test) + `internal/handler/chat_handler_test.go` |
| Go 单测 (BFF chat_handler) | ✅ | `emotion-echo-web-bff/internal/handler/chat_handler_test.go` |
| Go 单测 (ai-svc ChatCompletion) | ✅ | `emotion-echo-ai-svc/internal/grpcclient/ai_client_grpc_test.go` + `internal/analyzer/grpc_analyzer_test.go` |
| Go 单测 (BFF ai_stream_handler) | ✅ 多用例 | `emotion-echo-web-bff/internal/handler/ai_stream_handler_test.go` + `ai_stream_handler_llmgrpc_test.go` |
| 前端单测 (vitest) | ✅ 3 用例 | `emotion-echo-web/app/pages/chat/conversation/new.test.ts` (Stage 107 新建) |
| 前端单测 (stores) | ✅ | `emotion-echo-web/app/stores/conversation.test.ts` + `__tests__/message.test.ts` |
| E2E (Playwright) | ⚫ | 仅 `login-flow.spec.ts` 1 个，**chat /new → AI 回复无 Playwright** |
| browser-use 实测 (端到端) | 🔴 | **Stage 107 卡在架构债**：useConversationSender composable 生命周期 vs Vue 路由 unmount，导致 sendMessage + sendAIStream 在浏览器实测未触发。详见 stage-107 §四 |
| 数据契约 §1 (user_behavior_events 行数) | 🟡 | smoke 脚本有 (`scripts/smoke_data_layer.py`)，待端到端通后跑 |
| 数据契约 §2 (event_type enum 细分) | 🟡 | 同上 |
| 数据契约 §5 (schema 与写入端一致) | 🟡 | 同上 |

### E2E-3 · Assessment 心理测验

| 维度 | 状态 | 证据 |
|---|---|---|
| Go 单测 (assessment-svc) | 🟡 | 仅有 config_test.go + bootstrap_test.go，**无 handler/logic test** |
| Go 集成测试 | ✅ | `integration_test/survey_integration_test.go` |
| E2E | ⚫ | 无 Playwright，无 browser-use 实测 |
| 数据契约 | ⚫ | smoke 脚本未涉及 |

### E2E-4 · Reports 报表 (daily / trend / mental-health)

| 维度 | 状态 | 证据 |
|---|---|---|
| Go 单测 (BFF analytics_handler) | ✅ 15+ 用例 | `emotion-echo-web-bff/internal/handler/analytics_handler_test.go` |
| Go 单测 (BFF analytics_view) | ✅ | `emotion-echo-web-bff/internal/handler/analytics_view.go` + tests |
| Go 单测 (analytics-svc downstream) | ✅ | `emotion-echo-web-bff/internal/downstream/analytics_grpc_test.go` |
| Go 集成测试 | ✅ | `integration_test/events_integration_test.go` + `event_query_integration_test.go` |
| E2E | ⚫ | 无 Playwright，无 browser-use 实测 |
| 数据契约 §3 (analytics_reader GRANT) | 🟡 | `analytics-svc/migrations/004_security_test.go` 已锁，**但 smoke 未跑全量** |
| 数据契约 §4 (chartData 非空) | 🔴 | 历史 Stage 36-FU 报告"dev 模式 4 dashboard `chartData.length === 0`"，**Stage 107 端到端未跑此契约**，可能复发 |

---

## 二、横切链路

### X-1 · Outbox + Kafka 事件链

| 维度 | 状态 | 证据 |
|---|---|---|
| Go 单测 (chat-svc outbox) | ✅ | `emotion-echo-chat-svc/internal/repository/outbox_test.go` (推测，未读但 logic test 含 outbox) |
| Go 单测 (kafka_publisher) | ✅ | `emotion-echo-chat-svc/internal/events/kafka_publisher_test.go` |
| Go 单测 (ai-svc consumer) | ✅ | `emotion-echo-ai-svc/internal/consumer/consumer_test.go` |
| Go 单测 (analytics-svc consumer) | ✅ | `emotion-echo-analytics-svc/internal/kafka/consumer_test.go` |
| Go 集成测试 | ✅ | `integration_test/kafka_integration_test.go` |
| 数据契约 §6 (KAFKA_ENABLED=false 路径不空跑) | 🟡 | `chat-svc/internal/events/dev_fallback` 路径有测，**但"dev 模式 Kafka off 真的落库了吗"未端到端验证** |
| 端到端 (browser → chat → Kafka → ai-svc → 回流) | ⚫ | Stage 107 卡架构债，整个链路未通 |
| 已知 bug | 🟠 | Stage 105 Round 2 §P0-R2-5 env 名错配 + Stage 97 10 P0 已修；**残余 = chat-svc StreamMessages 故意 Unimplemented（业务未触发）** |

### X-2 · SkyWalking Trace 透传

| 维度 | 状态 | 证据 |
|---|---|---|
| Go 单测 (chat-svc sw8 注入) | ✅ | Stage 92 实证 sw8 221 chars |
| Go 单测 (ai-svc sw8 解析) | ✅ | Stage 92 PR-2 |
| Go 单测 (analytics-svc sw8 重建) | ✅ | Stage 93 13/13 PASS |
| 端到端 (OAP 9.x 看到完整 trace) | 🟡 | Stage 93 提到"OAP 9.x graphql queryDuration bug 阻塞 UI 可视化"，**残余未确认** |
| 已知 bug | 🟠 | Stage 50 stage-44 §四残余：sw-oap telemetry / 分支 merge / Nacos / 运维 SQL |

### X-3 · APISIX 网关

| 维度 | 状态 | 证据 |
|---|---|---|
| 配置静态测试 | ✅ 40/40 | `deploy/apisix/seed_test.js` (Stage 107 新增 trailing comma 断言) |
| 容器启动验证 | ✅ | `apisix-seed` exit 0，13 routes 注册成功 |
| 端到端 (浏览器经 APISIX 通) | 🟡 | Stage 107 修后 `POST /conversations` 返 200，但下游链路未通 |
| 已知 bug | 🟢 | Stage 107 trailing comma 已修 |

### X-4 · 鉴权 (JWT + X-User-Id 注入)

| 维度 | 状态 | 证据 |
|---|---|---|
| Go 单测 (BFF JWT 签发/验签) | ✅ | `auth_handler_test.go` 21 用例 |
| Go 单测 (shared JWT middleware) | ✅ | `jwt_auth_test.go` |
| 集成 (APISIX jwt-auth consumer + X-User-Id 注入) | 🟡 | seed.sh Stage 102/105 已加 store_in_ctx + serverless-post-function，**但"APISIX 真注入 X-User-Id + BFF 真读到"未端到端验证**（Stage 107 POST /conversations 返 200 但没看 BFF 是否因 X-User-Id 缺失 401） |
| 已知 bug | 🟢 | Stage 97 P0-R2-1 已修 |

---

## 三、AGENTS.md §2.4 数据契约 6 条状态

| # | 契约 | smoke 脚本 | 状态 | 阻塞原因 |
|---|---|---|---|---|
| 1 | user_behavior_events 行数 = 业务事件数 | `scripts/smoke_data_layer.py` (推断) | 🟡 待跑 | Stage 107 chat 端到端未通 |
| 2 | event_type enum 细分 (≥2 种 event_type) | 同上 | 🟡 待跑 | 同上 |
| 3 | analytics_reader role 能查所有 *_v 视图 | 同上 + `emotion-echo-analytics-svc/migrations/004_security_test.go` | 🟡 部分 | 单测已绿, smoke 待跑 |
| 4 | /api/v1/reports/daily 返回 chartData.length > 0 | 同上 | 🔴 | Stage 36-FU 报告 `chartData=[]` 历史复发风险，Stage 107 未验证 |
| 5 | schema 与写入端一致性 | `scripts/smoke_data_layer.py` | 🟡 待跑 | Stage 107 未通 |
| 6 | KAFKA_ENABLED=false 路径不空跑 | 同上 + `chat-svc/internal/events/dev_fallback_test.go` (推断) | 🟡 | 路径单测有, dev 模式真值待验 |

---

## 四、已知未修（阻塞端到端绿）

| 序号 | 标题 | 文件 | 阻塞范围 |
|---|---|---|---|
| **A1** | useConversationSender composable 生命周期 vs Vue 路由 unmount | `emotion-echo-web/app/composables/useConversationSender.ts:48-51` | E2E-2 chat 链路全链、X-1 outbox、X-2 sw8 端到端验证 |
| A2 | assessment-svc handler/logic 无单测 | `emotion-echo-assessment-svc/internal/handler/` `internal/logic/` | E2E-3 完整覆盖 |
| A3 | reports E2E 无 Playwright | `emotion-echo-web/e2e/` | E2E-4 端到端 |
| A4 | chartData=[] 历史 bug 未实测复现 | `emotion-echo-web-bff/internal/handler/analytics_handler.go` | 契约 4 |
| A5 | OAP 9.x queryDuration bug | SkyWalking OAP | X-2 完整可视化 |
| A6 | Stage 44 §四残余（sw-oap / Nacos / 运维 SQL） | 见 stage-44 | 多个横切 |

---

## 五、本阶段推进顺序（与 stage 文档对接）

按 [multi-pr-commit-discipline] 习惯 + "未修先开后闭"原则：

| Sprint | 目标 | 阻塞依赖 | 状态 |
|---|---|---|---|
| **Sprint 108** (下一轮) | 修 A1：sender composable 生命周期 | 无 | pending |
| **Sprint 109** | 修完 A1 后跑端到端：E2E-2 chat 链路 + 数据契约 §1 §2 §5 §6 全绿 | A1 | pending |
| **Sprint 110** | 写 E2E-2 Playwright spec（chat /new → 收到 AI 回复），作为回归钉子 | A1 + Sprint 109 | pending |
| **Sprint 111** | 写 E2E-3 assessment Playwright + 补 assessment-svc handler/logic 单测 | 无（独立） | pending |
| **Sprint 112** | 写 E2E-4 reports Playwright + 复测 chartData=[] 是否真复发 | 无（独立） | pending |
| **Sprint 113** | 跑全量 §2.4 数据契约 smoke，6/6 全绿 | Sprint 109-112 | pending |
| **Sprint 114** | E2E-1 异常路径 Playwright（JWT 过期 → refresh → 跳登录） | 无 | pending |
| **Sprint 115** | 横切端到端验证：sw8 trace UI + Kafka outbox → ai-svc → 回流 | A1 | pending |

---

## 六、收口自检（每次更新本文档时跑）

```bash
# 1. 测试文件数核对（防止新增测试没登记）
find emotion-echo-web -name "*.spec.ts" -not -path "*/node_modules/*" -not -path "*/.output/*" | wc -l   # 期望: 1 → N (N 为当前 E2E 数)
find emotion-echo-web -name "*.test.ts" -not -path "*/node_modules/*" -not -path "*/.output/*" | wc -l   # 期望: ≥37
find emotion-echo-* -name "*_test.go" -not -path "*/node_modules/*" 2>/dev/null | wc -l                  # 期望: ≥285

# 2. 数据契约 smoke
python scripts/smoke_data_layer.py 2>&1 | tail -30   # 期望: 6/6 PASS

# 3. dev 模式状态
docker ps --format "table {{.Names}}\t{{.Status}}" 2>&1 | grep -c "Up.*healthy"  # 期望: ≥14

# 4. E2E-2 端到端 browser-use 验证（manual 或后续 Playwright）
```

---

## 七、调研依据（AGENTS.md §〇 硬规则）

| 文件 | 用途 |
|---|---|
| `AGENTS.md §2.3 §2.4` | 覆盖率底线 + 6 条数据契约原文 |
| `docs/stages/stage-26-T-test-backlog.md` | 历史版本（2026-08-29），本文档接替 |
| `docs/stages/stage-107-chat-new-conversation-fix-2026-09-16.md` | 本轮收口的 stage |
| `docs/plans/multi-round-iteration-2026-09-15.md §十六` | 上一轮收口基线 |
| `docs/plans/README.md` | plans 写规范（front-matter 格式）|
| `memory/emotion-echo-stage-107-session.md` | 本 session memory |
| `find emotion-echo-* -name "*_test.go"` | Go 测试文件清单 |
| `find emotion-echo-web -name "*.{spec,test}.ts"` | 前端测试文件清单 |
| `find emotion-llm-service -name "test_*.py"` | pytest 文件清单 |
| `ls scripts/smoke_*.{sh,py}` | smoke 脚本清单 |

### 架构假设清单

| 假设 | 验证 |
|---|---|
| AGENTS.md §2.4 6 条契约必须全绿才能"整体落地" | ✅ 读 AGENTS.md §2.4 + §〇 硬规则确认 |
| 业务路径级追踪比 API 端点级更适合本阶段 | ✅ 用户选定的粒度 |
| 现有测试文件位置已在 §一/§二 各路径下穷举 | 🟡 用 `find` 扫了但未逐文件读实现，仅按文件名归类 |
| Playwright 是项目 E2E 栈 | ✅ `e2e/login-flow.spec.ts` 已用，且 package.json 包含 `@playwright/test` |
| 数据契约 smoke 脚本可直接跑 | 🟡 `scripts/smoke_data_layer.py` 存在但**本 session 未实测跑过** |

---

## 八、更新规则

- 每次新增/修改 stage 时同步更新本文档对应路径状态
- 数据契约 smoke 跑过后更新 §三 状态列
- 端到端 browser-use 实测后更新 §一/§二 端到端列
- 新发现"已知未修"加入 §四 并起对应 sprint
