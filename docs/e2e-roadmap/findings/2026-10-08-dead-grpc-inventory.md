# 内部 gRPC「构建了但没人用」清点（2026-10-08，只读排查）

> **性质**：只读排查报告，不含任何代码改动。**执行环境：未使用 docker**（纯 grep/read + `helm`/`node` 等本地只读工具）。
> **触发**：用户「之前排查发现过构建完成但是没有使用的功能，需要你进行排查，我记得当时说的是 gRPC 相关代码」。
> **关联**：[ADR-18 渐进式 RPC 化](../../architecture/adr/adr-2026-09-incremental-rpc-adoption.md)、[ADR survey-http-bypass](../../architecture/adr/adr-2026-09-survey-http-bypass.md)、账本 `E2E-F-208`。

## 0. 结论速览

| 分类 | 数量 | 性质 |
|------|------|------|
| **A. 完全无调用方的 RPC**（服务端实现 + 客户端方法都在，但没人调） | 8 | 真死代码 |
| **B. 只有服务端实现、完全没有客户端方法** | 6 | 死代码（含 3 个恒返空/占位的占位实现） |
| **C. BFF gRPC 客户端里的不可达分支** | 1 整客户端（5 方法） | 真死代码（分支条件恒真） |
| **D. 白 dial 的连接**（dial 了但零 RPC） | 1 | 资源浪费 + 误导 |
| **E. 其他预留/死代码候选** | 3 | 部分为设计预留（非缺陷） |

**最大一块**：**BFF → assessment-svc 的整条 gRPC 通道**（客户端 5 RPC + 服务端 251 行实现 + 3 个测试文件 + :8886 监听 + 一条白 dial 的连接）——成因是 ADR-2026-09-survey-http-bypass 让 survey 5 端点改走 HTTP，且 personality 取数硬编码 HTTP。

**与已记录 ADR 的关系**：ADR-2026-09-survey-http-bypass §后果已**如实**写明"survey 端点的 gRPC 路径变成死代码"，并列了后续（扩展 proto 支持完整 JSONB）。⇒ 这**不是文档漂移**，是**已决策但未清理**的技术债（ADR 未含清理/删除动作）。

---

## A. 完全无调用方的 RPC

| RPC（proto:行） | 服务端实现 | 客户端方法 | 判定依据 |
|---|---|---|---|
| `ChatService.StreamMessages` `proto/chat.proto:65` | `chat-svc/internal/grpcserver/chat_server.go:289`（`:293` 返 `Unimplemented`） | BFF `chat_grpc.go:170`（**不在 `ChatClient` 接口内**，`chat.go:88-104` 无此方法） | 全仓唯一调用点是方法自身；无 handler 调用 |
| `EmotionLLMService.AnalyzeBatch` `proto/emotion_llm.proto:18` | `emotion-llm-service/grpc_server.py:337`（真实 stream 实现） | 无 | 仅测试引用（`grpc_analyzer_test.go:67` 等） |
| `AssessmentService.ListMyResults` `proto/agent.proto:46` | `assessment-svc/internal/grpcserver/agent_server.go:201` | 无（BFF `assessment_grpc.go:94` 是恒返 error 的 stub） | `cli.ListMyResults(` 全仓零命中 |
| `AssessmentService.GetSurveyResult` `proto/agent.proto:49` | `agent_server.go:222` | 无（BFF `assessment_grpc.go:99` 同型 stub） | `cli.GetSurveyResult(` 全仓零命中 |
| `AnalyticsService.MentalHealthHistory` `proto/metric.proto:59` | `analytics-svc/internal/grpcserver/metric_server.go:323`（**恒返空 `Records:nil` 占位**） | 无（BFF `AnalyticsClient` 无此方法） | 全仓零命中 |
| `AnalyticsService.MentalHealthTrigger` `proto/metric.proto:62` | `metric_server.go:332`（真实实现） | 无 | 同上 |
| `AnalyticsService.MentalHealthTrend` `proto/metric.proto:65` | `metric_server.go:353`（真实实现） | 无 | 同上 |
| `UserService.Logout` `proto/user.proto:74` | `user-svc/internal/grpcserver/user_server.go:216` | BFF `user_grpc.go:108`（**不在 `UserClient` 接口内**） | `/api/v1/auth/logout` 只清 cookie（`auth_handler.go:274-288`）⇒ 零调用 |
| `UserService.VerifySecurityAnswer` `proto/user.proto:79` | `user_server.go:230` | BFF `user_grpc.go:167`（**在接口内**） | 接口方法存在但**零 handler 调用点** |

## B. 只有服务端实现、完全没有客户端方法

`AnalyzeBatch` / `ListMyResults` / `GetSurveyResult` / `MentalHealthHistory` / `MentalHealthTrigger` / `MentalHealthTrend`（证据同 A）。

> 注：`Logout` / `VerifySecurityAnswer` 有客户端方法（只是无人调），故列 A 不列 B。

## C. BFF gRPC 客户端里的**不可达**分支（assessment 整条链）

**事实**：`h.assessmentBase` **恒非空** ⇒ `if h.assessmentBase != "" { …HTTP… return }` **恒真**，其后 gRPC 分支恒不可达。

- `config/config.go:190` `setHTTPServiceDefaults(&c.AssessmentService, "http://localhost:8889")`（`:309-312` 仅在 BaseURL 空时补默认 ⇒ 恒非空；`config_test.go:46` 已钉）。
- `main.go:483` `NewSurveyHandler(s.Assessment).WithAssessmentBase(c.AssessmentService.BaseURL)` —— 传入的正是上面那个非空默认。

| 位置 | 不可达调用 | 对应 gRPC 方法 |
|---|---|---|
| `handler/survey_handler.go:72`→`:76` | `h.assessment.ListSurveys` | `assessment_grpc.go:30` |
| `survey_handler.go:121`→`:125` | `h.assessment.GetSurvey` | `assessment_grpc.go:49` |
| `survey_handler.go:188`→`:197` | `h.assessment.SubmitSurvey` | `assessment_grpc.go:61` |
| `survey_handler.go:246`→`:250` | `h.assessment.ListResults` | `assessment_grpc.go:94`（**本身恒返 error stub**） |
| `survey_handler.go:295`→`:299` | `h.assessment.GetResult` | `assessment_grpc.go:99`（同上） |

⇒ **双重死代码**：`ListResults` / `GetResult` 的 gRPC 实现只 `return fmt.Errorf("… not implemented …")`，且唯一调用方又在恒真分支之后。

**接口外的孤岛方法**（即便分支可达也无人经接口调用）：`chat_grpc.go:170 StreamMessages`（不在 `ChatClient`）、`user_grpc.go:108 Logout`（不在 `UserClient`）。

**对照组（这些 gRPC 分支可达且被真实调用，不是死代码）**：chat 其余 7 / user 其余 8 / analytics 6 / ai 3 / emotion_query 3 / llm 2 —— 见 `chat_handler.go`、`user_handler.go`、`analytics_handler.go`、`multimodal_handler.go`、`emotion_query_handler.go`、`ai_stream_handler.go` 等的调用点。

## D. 白 dial 的连接

| 连接 | dial | 装配 | 判定 |
|---|---|---|---|
| BFF → assessment-svc gRPC | `main.go:378` | `main.go:393-397` `SetAssessment(GRPCConn: assessmentGRPCConn)` | 唯一消费者 `s.Assessment` 只被 SurveyHandler 用，而 5 个 gRPC 分支恒不可达（C）⇒ **连接被维持但零 RPC**。personality 取数走**另建**的 `Transport=HTTP` 客户端（`main.go:528-532`），不经此连接 |
| BFF → ai-svc emotion-q 独立连接 | `main.go:420` | `main.go:421` `SetEmotionQ` | **非死代码**（`emotion_query_handler.go` 真实调用） |

其余 4 条 dial（user/chat/analytics/ai）均被真实使用。跨服务 dial（chat-svc→ai-svc `chat-svc/main.go:202`；ai-svc→llm-service `ai-svc/main.go:360`）亦均被使用。

**各服务端 gRPC server 均真实启动**：user :8887 / chat :8892 / ai :8892 / analytics :8885 / **assessment :8886** / llm-service :50051。

## E. 其他预留 / 死代码候选

| 项 | 位置 | 判定 |
|---|---|---|
| `StreamMessages` proto 预留 | `proto/chat.proto:58-65` + `chat_server.go:293` Unimplemented + `chat_grpc.go:170` | 死代码（同 A） |
| `proto/chat_events.proto` | 生产端 `chat-svc/internal/events/proto_marshal.go`；消费端 `ai-svc/internal/consumer/proto_decode.go:44`、`analytics-svc/internal/kafka/proto_decode.go:48` | **不是死代码**（Kafka 事件双写用） |
| `proto/metric.proto` service | 9 RPC 中 6 被 BFF 调、3 死（A/B） | 部分死 |
| `proto/agent.proto` service | 5 RPC 中 3 客户端不可达（C）、2 无调用方（A） | **整条链实质死** |
| `ai-svc/internal/analyzer/grpc_analyzer.go:193` `AnalyzeWithAuth` | 同文件 `:193-196` | 无生产调用方（实际鉴权走 `auth_wrapped.go:26`） |
| `scripts/grpc_smoke/main.go` | `:99,112,146,175` | 手工冒烟脚本，非生产路径（其调用的 RPC 另有生产调用方） |
| `chat-svc NoopAIClient` / `ai-svc HTTPAnalyzer` 保留分支 | `chat-svc/internal/grpcclient/ai_client.go:37-39` 等 | **条件降级实现，非死代码** |

## 清理选项（供裁定，未执行）

| 选项 | 内容 | 代价 / 风险 |
|---|---|---|
| ① 保留 + 标注 | 在死代码处加"已知未接线（ADR-…）"注释 + 守卫断言"不得新增调用方" | 零风险；死代码留存 |
| ② 删除客户端死代码 | 删 BFF `assessment_grpc.go` 5 方法 + `SetAssessment` 的 gRPC 装配 + assessment dial；接口保留 HTTP 实现 | 需确认无外部（scripts/测试）依赖；`chat_grpc.go StreamMessages`、`user_grpc.go Logout` 同类处理 |
| ③ 补全（改走 gRPC） | 扩 proto `SurveyQuestion` 支持完整 JSONB（ADR 已给方向）后切回 gRPC | 工作量大；与 ADR-18 §B「非阻塞 backlog」一致，属 P3 |
| ④ 服务端侧 | assessment-svc :8886 若无任何客户端，考虑停起该 gRPC server（省资源） | 需确认 integration test 是否依赖真 server |

**未做任何删除/修改**（本轮为只读排查）。裁定见账本 `E2E-F-208`。
