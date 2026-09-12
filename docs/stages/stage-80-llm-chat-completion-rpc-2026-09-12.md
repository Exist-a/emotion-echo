# Stage 80 — llm-chat-real-pipeline PR-1：llm-service ChatCompletion 流式 RPC

> 日期：2026-09-12
> 来源：Stage 79 收口后 roadmap 首项 → 用户"按推荐进行"
> 计划：[docs/plans/llm-chat-real-pipeline.md](../plans/llm-chat-real-pipeline.md) §三 PR-1
> 性质：TDD（RED `69a27b0` → GREEN `250498f`），2 commit

## 一、落地内容

### proto（`emotion_llm.proto`）

- `ChatCompletion(ChatCompletionRequest) returns (stream ChatChunk)`（server streaming）
- `ChatMessage{role,content}` / `ChatChunk{delta_content,done,model,fallback_reason}`
- 双端 regen：Go `shared/pkg/emotionllm`（go build 过）+ Python pb2；`check_proto_layout.py` ✅

### Python（`chat_completion.py` 新模块 + servicer 接线）

- `iter_chat_chunks`：OpenAI 兼容流式转发（`client.chat.completions.create(stream=True)`，
  delta.content → chunk）；**任何上游失败整条降级 mock（绝不 half-stream）**；空流同样降级
- env 沿用仓库既有 `LLM_API_KEY/LLM_BASE_URL/LLM_MODEL` 三件套（DeepSeek 等 OpenAI
  兼容端点）——计划原文写 Moonshot，实施时修正为仓库既有约定（ai-svc 同款）
- 无 key / 上游异常 / openai SDK 未安装（延迟 import）→ 每帧 `fallback_reason` 非空、
  `model="mock"`——CI / 离线 demo 全链路可跑（§契约 6 哲学）
- servicer：pb 转换 + GRPC_REQUESTS_TOTAL 指标 + 空 messages → INVALID_ARGUMENT
- compose：llm-service 注入 LLM_* 三件套；requirements 加 `openai>=1.40`；
  Dockerfile 双阶段补 COPY `chat_completion.py`（Stage 39 Dockerfile 契约测试拦截——
  该测试防整类"逐文件 COPY 漏新模块"再次生效）

## 二、验收

- 单测：新增 6 例契约（无 key 降级 / key 解析 / delta→chunk 映射+done 帧 /
  上游错误降级 / messages 透传）+ 既有套件 **112/112 PASS**
- dev 栈真实容器 e2e（mTLS + Stage 17 internal-api-key，容器内 grpc python client）：
  1. **无 key**：3 帧流式 + done=True + model="mock" + `fallback_reason='mock_no_api_key'` ✓
  2. **无效 key**（临时以 `LLM_API_KEY=sk-invalid` 重建容器）：上游真实 401
     （DeepSeek 端点可达）→ `fallback_reason='upstream_error: ...Authentication Fails...'`
     + mock 文案兜底 ✓（验毕已恢复无 key 形态）
  3. 有效 key 路径由单测锁定映射契约（仓库无真实 key）
- 过程修复：测试 fake 装配错误（completions 须挂 `chat` 上，与实现访问路径一致）

## 三、本批未做（open）

| 项 | 说明 |
|---|---|
| PR-2（BFF 切上游） | `AIStreamHandler` mock → llm-service gRPC ChatCompletion 透传（`resolveGRPCAddr` Nacos 模式复用），mock 保留为 env 兜底——llm-service gRPC 需注册 metadata（gRPC 端口 50051 已有，Nacos 注册是否带 metadata 待 PR-2 核查） |
| PR-3 | 意图分类（重写 intent-classification-6-types）+ 结构化回复（ai-response-structured 阶段 2） |
| llm-service 注册 metadata.grpc_port | PR-2 前置核查项（Stage 75 模式） |
| Kafka P3 / §1.4 | 不变 |

## 四、调研依据

- 已读：proto/emotion_llm.proto、emotion-llm-service/{grpc_server.py,main.py,Dockerfile,
  requirements.txt,tests/unit/test_grpc_server.py,test_dockerfile_contract.py}、
  proto/gen.sh、deploy/docker-compose.apps.yml（llm-service/ai-svc 段）
- 已查：docs/plans/llm-chat-real-pipeline.md、stage-39（Dockerfile 契约测试由来）、
  Stage 17 auth（x-internal-api-key）/ Stage 18 mTLS 约定、ai-svc LLM_* env 约定
- 命令证据：pytest 112/112；双容器 e2e 输出（mock_no_api_key / upstream_error 401）；
  compose config 校验过；check_proto_layout ✅；shared go build ✅
- 关联：决策 4（gRPC）、§契约 6（mock 兜底哲学）、stage-79（前置收口）

---

> 最后更新：2026-09-12 by Stage 80 实施 session
> 关联：llm-chat-real-pipeline.md PR-2（下一步）
