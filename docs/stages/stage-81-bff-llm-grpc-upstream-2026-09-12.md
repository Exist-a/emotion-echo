# Stage 81 — llm-chat-real-pipeline PR-2：BFF ai/stream 切 llm-service gRPC 上游

> 日期：2026-09-12
> 来源：Stage 80 收口后 roadmap 首项 → 用户"那就继续吧"
> 计划：[docs/plans/llm-chat-real-pipeline.md](../plans/llm-chat-real-pipeline.md) §三 PR-2
> 性质：TDD（RED `3833ff2` → GREEN `6d7662d`），2 commit

## 一、调研修正（PR-2 实际缺口）

计划初稿假设"AIStreamHandler 是纯 mock"——实施调研发现 BFF 已有一条 **Phase D 路径**
（`BFF_LLM_API_KEY` 非空时 HTTP 直连 DeepSeek）。PR-2 的真实价值是**把 LLM 调用收敛到
llm-service**（key 管理 / 降级 / 指标 / PR-3 意图分类与结构化 prompt 的统一挂载点），
BFF 不再各处直连外部 LLM API。

llm-service **未注册 Nacos**（hosts=[]，Python 端注册从未实现——stage-78 已登记），
BFF 用 env `LLM_SVC_GRPC_ADDR` 直连（对齐 ai-svc 的 `LLM_GRPC_ADDR` 约定）。

## 二、落地内容

- `downstream/llm_grpc.go`（新）：`LLMGRPCClient` + `LLMChatStreamer` 接口（ctx 可取消）。
  mTLS（TLS_ENABLED / TLS_CA_CERT / TLS_CLIENT_CERT / TLS_CLIENT_KEY，server name
  emotion-llm-service）+ `x-internal-api-key` metadata——对齐 ai-svc GRPCAnalyzer 模式
- `ai_stream_handler.go`：上游优先级链 **gRPC → Phase D HTTP 直连（有 key）→ mock**；
  llm-service 侧的降级（PR-1 fallback_reason）随 delta 透传，BFF 不复制降级逻辑
- `main.go`：`LLM_SVC_GRPC_ADDR` 非空时装配 client（init 失败只告警不阻断，降级链兜底）
- `config.go`：`LLM.GRPCAddr` + env；compose：BFF 注入地址 / TLS env /
  ca + ai-client 证书挂载（dev 复用 ai-client，同 CA 签发；prod 应签独立 bff-client）

## 三、验收

- TDD：RED（handler 优先级链 2 例 + downstream bufconn fake servicer 契约）→ 全绿；
  全模块 `go vet` 0 err + test 绿（main_test.go 同步 registerRoutes 新签名）
- **dev 栈端到端**（经 APISIX，OpenAI 兼容格式）：
  ```
  POST /api/v1/ai/stream {"message":"今天上班好累",...}
  → data: {"choices":[{"delta":{"content":"我在呢。愿意和我说说刚才发生了什"}}]}
  → data: {"choices":[{"delta":{"content":"么吗？不用着急，按你的节奏来就好。"}}]}
  → data: [DONE]
  ```
  分帧特征与 llm-service PR-1 mock 文案吻合（非 BFF 旧关键词 mock）；llm-service 日志
  `[trace] span_op=/emotion_llm.v1.EmotionLLMService/ChatCompletion status=OK` +
  `ChatCompletion request: messages=2 model=deepseek-chat` 双实证
- 链路：浏览器 → APISIX → BFF（gRPC mTLS + auth）→ llm-service（无 key → 自降级 mock）
  ——真实 key 时同链路直达 LLM 上游，前端零改动
- 回归：smoke_data_layer 11/11 + §契约 7 6/6；web-bff 镜像重建 + 滚动重启 healthy

## 四、本批未做（open）

| 项 | 说明 |
|---|---|
| llm-service Nacos 注册（Python 端） | BFF 目前 env 直连；注册后可切 resolveGRPCAddr Nacos 优先模式（Stage 75 模式），需补 metadata（grpc_port / 鉴权信息如何注册待设计） |
| PR-3 | 意图分类（重写 intent-classification-6-types）+ 结构化回复（ai-response-structured 阶段 2）——挂载点已就绪（llm-service ChatCompletion 前后插分类/风格指令） |
| prod 独立 bff-client 证书 | dev 复用 ai-client.crt；多机/prod 应为 BFF 签发独立客户端证书 |
| Kafka P3 / §1.4 | 不变 |

## 五、调研依据

- 已读：web-bff/{internal/handler/ai_stream_handler.go(+test),internal/downstream/{llm.go,
  llm_grpc.go,chat_grpc.go},internal/config/config.go,main.go}、
  ai-svc/internal/analyzer/grpc_analyzer.go（mTLS 拨号模式）、
  deploy/docker-compose.apps.yml（llm-service/ai-svc/BFF 段）
- 已查：llm-chat-real-pipeline.md、stage-80 报告、Nacos instance/list 实测（hosts=[]）
- 命令证据：经网关 SSE e2e 输出（上录）；llm-service trace/服务日志双实证；
  smoke 11/11 + §契约 7 6/6；go vet/test 全绿
- 关联：决策 4（gRPC）、决策 10、stage-80（PR-1）、§契约 6（降级哲学）

---

> 最后更新：2026-09-12 by Stage 81 实施 session
> 关联：llm-chat-real-pipeline.md（PR-3 待启动）、stage-80
