---
status: landed
priority: high
owner: zcode-bot
created: 2026-10-13
last-refresh: 2026-10-13
type: stage-landing
related-plans:
  - docs/legacy-plans/landed/frontend-message-cache-offline-resilience.md （游标分页本轮部分落地；前端本地缓存仍未接线）
  - docs/legacy-plans/landed/observability-business-metrics-wiring.md （Grafana 业务/可靠性/日志看板仍未建）
  - docs/plans/dlq-hardening-and-replay.md （未动）
  - docs/plans/outbox-trace-continuity.md （未动）
---

# Stage 113 — 情绪分析主链路修复 + 演示环境/前端修复（面试演示准备轮）

> **起点**：用户为线上面试演示做环境准备，报告一串问题；过程中又挖出**情绪分析主链路整体断线**
> 与**年报情绪分布塌成单色**两个根因级缺陷。
> **结果**：情绪分析已端到端接真实 LLM（实证 `happy/sad|deepseek-chat`）；年报分布已真实化
> （实证多情绪）；前端 9 项 UI 修复；消息历史改为"最新优先 + 游标分页（服务端已落地、BFF 透传待部署）"。
> **纪律说明**：本轮按用户指示**不走 E2E 阶段流程**（用户："不要走 e2e 那一套了，很急"），
> 因此**未走 E2E-F 账本编号**；缺陷根因与证据记录于本文。

---

## 一、情绪分析主链路（根因级缺陷，已修 + 端到端实证）

### 1.1 现象
`emotion_echo_ai.emotion_analysis` 自 **2026-09-22 起全部** `primary_emotion=neutral / model=keyword-stub-v1`
（此前 09-20/21 还有 `gpt-4` 的真实情绪）。用户表现为"年报情绪分布全是中性"。

### 1.2 根因（四层叠加，逐层实测确认）

| # | 层 | 根因 | 证据 |
|---|---|---|---|
| 1 | ai-svc → llm-service gRPC | ai-svc **挂了客户端证书却没设 `TLS_ENABLED/TLS_CA_CERT/TLS_CLIENT_CERT/TLS_CLIENT_KEY`**，按明文拨号打 mTLS 端口 → 握手失败 | ai-svc 日志 `llm gRPC dial failed…`；llm-service 日志 `Handshake failed…WRONG_VERSION_NUMBER`；`deploy/docker-compose.apps.yml` ai-svc 段缺 4 个 env（web-bff 有） |
| 2 | llm-service gRPC 鉴权 | `AuthInterceptor` **把健康检查也拦了**（`/grpc.health.v1.Health/Check` 不带业务 key）→ ai-svc `WaitForReady("emotion.LLM")` 恒失败 | llm-service 日志刷屏 `AUTH REJECTED: missing api key for /grpc.health.v1.Health/Check` |
| 3 | 回退 HTTP 分析器 | dial 失败后回退的 `HTTPAnalyzer` 把 `/analyze` 发给 **`c.LLM.BaseURL`＝deepseek**（该 env 与融合 `LLMFuser` 共用，指向 deepseek）且**不带任何 Authorization/内部 key** → 401 | ai-svc 日志 `analyzer primary failed…llm returned 401: Authentication Fails (governor)` |
| 4 | **分析器本身** | 即便前 3 层都通，`llm-service/main.py` 的 `analyze()` **也只是关键词分析**（`model="keyword-v1"`），从未调用 LLM | `emotion-llm-service/main.py`（旧）`analyze()` 全文为关键词命中；gRPC `Analyze` 亦复用该函数（`grpc_server.py`） |

> 结论：**文本情绪分析"从未真正接 LLM"**，长期由关键词桩产出中性；2026-09-22 起的 TLS 回归使它彻底退化为 `keyword-stub-v1`。

### 1.3 修复
| 文件 | 改动 |
|---|---|
| `deploy/docker-compose.apps.yml`（ai-svc 段） | 补 `TLS_ENABLED: "1"` + `TLS_CA_CERT/TLS_CLIENT_CERT/TLS_CLIENT_KEY`（证书此前已挂载，仅缺 env） |
| `emotion-llm-service/grpc_server.py` | `AuthInterceptor.intercept_service` 放行 `/grpc.health.v1.Health/`（health 只暴露 SERVING/NOT_SERVING，无泄露风险） |
| `emotion-llm-service/main.py` | `analyze()` 改为**先调 LLM**（复用 `chat_completion` 的 OpenAI 客户端 + JSON 分类 prompt，标签 `happy/sad/angry/neutral/calm/anxious/surprised/disgusted/fearful`），**失败/无 key 回落关键词**（`_keyword_analyze`）；新增 `_llm_analyze()` |
| `emotion-echo-ai-svc/internal/analyzer/grpc_analyzer.go` | 调用超时 `3s → 8s`（LLM 分类 1~3s，3s 会误判超时回落关键词） |
| `emotion-echo-ai-svc/internal/analyzer/http_analyzer.go` | `NewHTTPAnalyzer` 变参加 `apiKey`（发 `x-internal-api-key` 头）；超时 `3s → 8s` |
| `emotion-echo-ai-svc/main.go` | 回退 HTTP 分析器传入 `c.LLM.InternalAPIKey` |

### 1.4 实证（非单测）
- **直测 gRPC Analyze**：`难过→sad(-0.9)`、`开心→happy(0.9)`、`焦虑→anxious(-0.6)`、`无所谓→neutral(-0.2)`，全部 `model=deepseek-chat`。
- **端到端**：以 `echo/echo123` 经网关发两条消息 → `emotion_analysis` 新增 `happy|deepseek-chat`、`sad|deepseek-chat`（此前为 `keyword-stub-v1|neutral`）。

---

## 二、年报情绪分布塌成单色（已修 + 实证）

### 2.1 根因（两个独立缺陷叠加）
1. **分桶退化**：`analytics-svc` 的 `trendTypeBucket` 为 `weekly=7d / monthly=30d / yearly=365d`，**正好等于各自的自然窗口** ⇒ 整段区间只落进 **1 个桶**（年报只有 1 个数据点/1 个扇区）。见 `emotion-echo-analytics-svc/internal/repository/report_repository.go`。
2. **分布口径错误**：BFF 的情绪分布是拿"**每个时间桶的主导情绪**"聚合（`toFrontendTrendReport` 的 `emotionTotal`），不是真实情绪计数分布；且空桶 `PrimaryEmotion=""` 被默认成 `neutral`（`emotion-echo-web-bff/internal/handler/analytics_view.go`）。

### 2.2 修复（含 proto 变更）
| 层 | 改动 |
|---|---|
| `proto/metric.proto` | `ReportsTrendResponse` 新增 `repeated EmotionDistribution emotion_distribution = 6;`（**真实分布**） |
| 生成 | `bash proto/gen.sh metric.proto`（宿主有 protoc/protoc-gen-go/protoc-gen-go-grpc）→ 更新 `emotion-echo-shared/pkg/emotionanalytics/metric.pb.go` |
| `analytics-svc/repository/report_repository.go` | ① 分桶改细：`weekly/monthly → 1d`、`yearly → 30d`；② `TrendReport` 新增 `EmotionCounts`，`GetTrendReport` 增加 `GROUP BY primary_emotion` 的真实分布查询 |
| `analytics-svc/grpcserver/metric_server.go` | `ReportsTrend` 透传 `EmotionDistribution: toProtoEmotionDistribution(resp.Report.EmotionCounts, total)` |
| `web-bff/internal/downstream/analytics.go` + `analytics_grpc.go` | `TrendReport` 增 `EmotionCounts`；从 proto 映射 |
| `web-bff/internal/handler/analytics_view.go` | 新增 `trendEmotionDistribution()`：**优先真实分布，缺省回落桶主导**（向后兼容） |

### 2.3 实证
- `GET /api/v1/reports/trend?type=yearly&year=2026` → `emotionDistribution=[中性247, 开心7, negative4, 难过4, positive2]`（**多情绪**），趋势点 `1 → 13`。
- 月报 `points=31`。
- **注意**：历史数据仍以中性为主（旧关键词桩产物，不可回改）；新消息起为真实 LLM 情绪，分布随之改善。

---

## 三、前端 UI 修复（9 项）

> 部署方式见 §五「环境状态」：**当前由本地 `pnpm dev` 提供（含全部前端修复）**；容器版 `emotion-echo-web` 为较早镜像。

| # | 问题 | 根因 | 修法（file） |
|---|---|---|---|
| 1 | 量表页**两个滚动条** | 导航布局 `.page-content` 用 `height: calc(100dvh - 88px)` 硬编码头部高度，而头部实际更高 ⇒ header+content 超过 100dvh，文档层多出一根滚动条 | `app/layouts/nav.vue`：`.page-content` 改 `flex: 1`（精确填充剩余空间） |
| 2 | 周报**时间区间选不了** | 用单向 `:date`（未接 `v-model`），选择不回流；`change` 先于 `v-model` 触发 | `app/pages/chat/dashboard/weeklyReport.vue`：`v-model:date` + 取数改用事件载荷 |
| 3 | 周报**应限 ≤7 天、默认最近 7 天** | 无跨度约束、默认 `[today,today]` | 同上：默认 `[today-6, today]`；新增 `clampRange()` 把跨度收在 7 天内并回写 UI |
| 4 | **0 条仍渲染空报表** | 后端为空桶补点 ⇒ `series` 非空，`chartData.length>0` 恒真 | `app/utils/trendReportCharts.ts`：新增"全 0 则返回 `[]`"守卫（+ 回归钉） |
| 5 | **摄像头点不开**（构造性死锁） | `<video v-if="isCameraOn">` 未挂载 ⇒ `cameraVideoRef=null` ⇒ `startCamera` 抛"组件未就绪" ⇒ 永远开不了 | `app/pages/chat/conversation/[id].vue` 与 `new.vue`：`v-if` → **`v-show`**（video 常挂载） |
| 6 | **附件 chip 渲染错位** | `.composer` 是 flex **行** ⇒ chip 与输入框并排；文件名是行内 `<span>` ⇒ `text-overflow` 失效 | `[id].vue`：`.composer` 改纵向；`.attachment-chip` 限宽 + `.attachment-name` 转 flex 子项 |
| 7 | **切会话数字人重复重载** | Nuxt 默认页面 key 含 `:id` ⇒ 切会话整页重挂载（Three.js 重建） | `[id].vue`：`definePageMeta({ key: route => route.name })` 稳定 key + `watch(route.params.id)` 驱动会话切换 |
| 8 | **侧边栏渲染重复会话** | 插件 `init()`（重置 cursor）先跑，页面 `onMounted` 又调 `fetchConversations()`（cursor 非空时为**追加**）⇒ 第一页被追加两遍 | `app/pages/chat/conversation/index.vue`：挂载改调 `conversationStore.init()` |
| 9 | 消息**历史不显示最新**（含分页） | `PostgresConversationRepo.ListMessages` 为 `Order("id ASC").Limit(limit)` ⇒ 返回**最旧** N 条；无 cursor/hasMore | 见 §四 |

---

## 四、消息历史：最新优先 + 游标分页

### 4.1 根因
- `chat-svc` 仓储 `Order("id ASC")` ⇒ 聊天展示的是**最旧**的一批（对聊天语义是错的）。
- BFF/chat-svc 的 `ListMessages` **无 cursor/hasMore**；前端 store 已发 `cursor` 并读 `{list,cursor,hasMore}`，参数被静默忽略 ⇒ 无分页。

### 4.2 修复
| 层 | 改动 |
|---|---|
| `proto/chat.proto` | `ListMessagesRequest` + `int64 cursor = 4`；`ListMessagesResponse` + `bool has_more = 2`；`bash proto/gen.sh chat.proto` |
| `chat-svc` repository | 新增 **`ListMessagesPage(ctx, convID, limit, cursor) ([]Message, bool, error)`**：`id DESC` 取 `limit+1` 探测 `hasMore`，最后**反转为升序**；`cursor>0` 时 `WHERE id < cursor`（Postgres + InMemory 两实现） |
| `chat-svc` logic/grpc | `listmessageslogic` 改用 `ListMessagesPage` 并回 `HasMore`；`chat_server.ListMessages` 传 `cursor`/回 `has_more` |
| `web-bff` | 接口/grpc/http 三处 `ListMessages` 加 `cursor` 参数与 `hasMore` 返回；`chat_handler.listMessages` 解析 `?cursor=` 并返回 `{list, cursor, hasMore}`（`cursor` = 本页最早一条 id） |
| 前端 | `[id].vue` 新增滚动到顶加载更早（`@scroll` → `loadMoreMessages`）；store 原有 `cursor/hasMore` 消费逻辑即生效 |

### 4.3 部署状态（**重要，未完全生效**）
- ✅ **"最新优先"已生效**：chat-svc 新镜像已重建并 recreate。
- ⚠️ **"游标翻页"未生效**：`web-bff` 的游标透传改动**未重建镜像**（构建时 Docker daemon 挂死，为保演示未重试）。⇒ 前端拿不到 `cursor/hasMore`，**分页未激活**（但"显示最新消息"这条关键修复已生效）。

---

## 五、环境 / 运维（演示环境）

| 事项 | 内容 |
|---|---|
| **项目改名** | `deploy` → `emotion-echo`：两个 compose 文件加顶层 `name: emotion-echo`；**数据卷 `deploy_*` 已拷贝为 `emotion-echo_*`**（旧卷保留作备份）；旧容器移除后按新项目重建 |
| **镜像重建** | 6 个 Go 服务 + web 前端重建为当前 HEAD；`chat-svc`/`ai-svc`/`analytics-svc`/`llm-service`/`web-bff` 本轮多次重建 |
| **Docker daemon 挂死** | 高负载（26 容器 + 并发构建）下 daemon 返 `500`/`EOF`。**恢复手法**：`wsl --shutdown` → 容器自动拉起；随后**端口转发失效**（3000/8848/18080 返 `000`）→ `docker restart emotion-echo-web emotion-echo-apisix emotion-echo-nacos emotion-echo-sw-ui` 恢复 |
| **BFF 缓存旧 IP → 503** | 重建任一**下游**（chat/user/assessment/analytics/ai/llm）后，BFF 仍拨旧容器 IP ⇒ `connection refused` ⇒ 503。**处置**：`docker restart emotion-echo-web-bff` |
| **前端部署方式** | 为快速上前端修复且**不触碰 docker daemon**，当前由**本地 `pnpm dev --port 3000`** 提供（需先 `docker stop emotion-echo-web`）。回容器版：`docker start emotion-echo-web` 并停 pnpm dev |
| **观测栈** | `prometheus/grafana/loki/promtail/alertmanager` 在 daemon 重启后曾 `Exited(255)`，已重新拉起；实测 13000/9090/9093/3100 全通 |

---

## 六、验证证据汇总

| 项 | 证据 |
|---|---|
| 情绪分析 | gRPC 直测 4 例全 `deepseek-chat`；端到端 2 条消息入库 `happy/sad\|deepseek-chat` |
| 年报分布 | `/reports/trend?type=yearly&year=2026` 多情绪 + 13 个点 |
| Go 测试 | `analytics-svc`（repository/grpcserver）✅；`web-bff`（handler/downstream）✅；`chat-svc`（logic/repository/grpcserver）✅ |
| 前端测试 | `nav.test.ts` 7/7；`trendReportCharts` 6/6（含新增"全 0 不渲染"）；对话页/量表 45/45 |
| 端到端可达 | 网关登录 200；`/conversations` `/surveys` `/reports/trend` `/user/profile` 全 200；Grafana/Prometheus/SkyWalking UI 200 |

---

## 七、待做（TODO）

### P0（演示/功能相关，建议尽快）
1. **部署 web-bff 游标透传**：重建 `emotion-echo-web-bff` 镜像 + recreate，激活消息分页（§四 4.3）。
2. **前端回到容器版**（可选）：把本轮前端修复固化进 `emotion-echo-web` 镜像（重建 web），摆脱 dev server 依赖。

### P1（用户明确"最后做/不急"）
3. **症状量表结果接入提示词**（用户指定）：PHQ-9/GAD-7 等结果目前**只写 `survey_results`、无人消费**（人格量表已接入 `buildPersonalityGuide`）。用户要求把症状量表作为用户信息拼进 prompt。参考链路：`assessment-svc` `SaveResult` → BFF `PersonalityProfileSource` → `ai_stream_handler.buildSystemPrompt`。
4. **Nacos beat 501 噪声**：BFF 心跳调 `/nacos/v1/ns/instance/beat`（Nacos 2.4.3 已移除，501），现靠 SDK `UpdateInstance` 兜底（`fail_count` 持续增长）。用户："可修但不急"。

### P2（本轮识别、未处理）
5. **回归钉不全**：仅给"趋势全 0 不渲染"补了 1 个钉子；其余 8 项前端修复与后端改动**未补专门测试**（本轮按"很急、不走 E2E"处理）。
6. **Grafana 看板仅 2 个**（Overview + Kafka Lag）；业务/可靠性/日志看板仍在 `docs/legacy-plans/landed/observability-business-metrics-wiring.md`（planned）。
7. **前端本地缓存（IndexedDB `messageCache.ts`）仍未接线**（`docs/legacy-plans/landed/frontend-message-cache-offline-resilience.md`）；本轮只落地了服务端游标 + 前端翻页触发。
8. **工作区未提交**：本轮全部改动（proto 重生成、compose 改名与 TLS env、6 个 Go svc、web 前端、两份新 memory）**均未 commit**。
9. **proto 契约兼容性**：`chat.proto`/`metric.proto` 为**新增字段**（向后兼容），但**两侧生成物需同 PR**（已在本轮一并重生成）。

---

## 八、诚实边界（不要夸大）

- 情绪分析现在是**真 LLM**，但**历史数据不可回改**：年报在旧数据占比高时仍偏中性。
- 消息**分页未激活**（BFF 未部署）；"显示最新"已生效。
- 本轮**未走 TDD 全流程 / 未走 E2E 阶段流程**（用户明示"很急"），测试覆盖为"改动点定向跑绿"，**不是**全量回归。
- 前端当前跑在 **dev server**（非容器镜像），`docker stop emotion-echo-web` 是前提；重启机器后需重跑 `pnpm dev`。

---

## 九、调研依据（AGENTS §〇 文档功课）

- **已读代码**：`emotion-llm-service/main.py`、`grpc_server.py`、`chat_completion.py`；`emotion-echo-ai-svc/{main.go, internal/analyzer/*}`；`emotion-echo-chat-svc/internal/{repository/conversation_repository.go, logic/listmessageslogic.go, grpcserver/chat_server.go}`；`emotion-echo-analytics-svc/internal/{repository/report_repository.go, grpcserver/metric_server.go}`；`emotion-echo-web-bff/internal/{handler/analytics_view.go, handler/chat_handler.go, downstream/analytics*.go, downstream/chat*.go}`；`emotion-echo-web/app/**`（report/conversation/layouts/utils 等）等 ≥20 个文件。
- **已查 ADR/决策**：`docs/architecture/adr/`（TLS/mTLS、APISIX 信任链）、`docs/plans/*`（frontend-message-cache / observability-business-metrics-wiring / outbox-trace-continuity / dlq-hardening-and-replay）。
- **实测**：gRPC Analyze 直测、端到端发消息入库、`/reports/trend` 实测、DB 直查（`emotion_analysis` / `daily_emotion_v` / `survey_results`）、容器日志（ai-svc / llm-service / web-bff / apisix）。
- **外部依赖**：DeepSeek OpenAI 兼容 chat.completions（`LLM_BASE_URL/LLM_API_KEY/LLM_MODEL`，key 实测有效）；`proto/gen.sh` 依赖 protoc + protoc-gen-go(+grpc)（宿主已具备）。

---

> 最后更新：2026-10-13 by zcode-bot（Stage 113 落地）
