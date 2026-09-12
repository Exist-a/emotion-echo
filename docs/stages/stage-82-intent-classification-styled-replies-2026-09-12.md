# Stage 82 — llm-chat-real-pipeline PR-3a：意图分类 6 类 + 按意图风格指令

> 日期：2026-09-12
> 来源：Stage 81 收口后 roadmap 首项 → 用户"可以，开始pr3"
> 计划：[docs/plans/intent-classification-6-types.md](../plans/intent-classification-6-types.md)（重写版）+
> [docs/plans/ai-response-structured.md](../plans/ai-response-structured.md) 阶段 2
> 性质：TDD（RED `0d7c64d` → GREEN `9488e05`），2 commit
> **范围说明**：PR-3 完整版含报表链路（analytics 意图分布 + 前端饼图），涉及
> schema/事件/报表/前端四层——拆为 PR-3b 下批实施，本批只做对话侧（用户可感知部分）。

## 一、落地内容

### proto（`emotion_llm.proto`）

- `ClassifyIntent(ClassifyIntentRequest) returns IntentResult`（unary）
- `ChatCompletionRequest.with_intent`：true = 服务端对最后一条 user 消息分类、
  注入风格指令、首帧回带 intent
- `ChatChunk.intent`（仅首帧携带）；双端 regen，`check_proto_layout` ✅

### Python（`intent.py` 新模块）

- 规则式 6 类分类：关键词打分（中文为主 + 常见英文），**零命中或并列冠军 → other**
  （避免误路由）；confidence = hits/(hits+3) 平滑——与 `main.analyze` 情绪分析同款
  风格，确定性、无 LLM 依赖、离线可用
- `STYLE_INSTRUCTIONS`：每意图回复风格指令（原文取自 ai-response-structured.md
  §阶段 2 的 per-intent prompt）
- `inject_style`：已有 system 就地扩写、缺失则前置；不变异入参
- servicer 接线：`ClassifyIntent` RPC + ChatCompletion with_intent 集成（分类 →
  注入 → 首帧回带）；Dockerfile 双阶段补 COPY（契约测试前置拦截，零遗漏）

## 二、验收

- 单测 **135/135**：表驱动分类 20 例（5 类各 2 例 + other 兜底 4 例）+ 注入 3 例 +
  真实 gRPC 集成 3 例（首帧带 intent / 未启用为空 / ClassifyIntent RPC）
- dev 栈真实容器 e2e（mTLS + auth）：
  ```
  ClassifyIntent [我的 Python 代码报错了怎么调试] -> tech_help (0.62)
  ClassifyIntent [我今天心情很低落想找人聊聊]     -> emotional_support (0.4)
  ClassifyIntent [面试被拒了怎么改进简历]         -> career_help (0.4)
  ClassifyIntent [周末有什么轻松电影推荐]         -> lifestyle (0.5)
  ChatCompletion with_intent → first_intent='tech_help', done=True
  ```
- 效果路径说明：风格指令注入的是**发给上游 LLM 的 system prompt**——dev 无 key 时
  走 mock 看不到风格差异，配真实 `LLM_API_KEY` 后即按意图格式化回复（前端 Markdown
  渲染已就绪，直接消费代码块/列表/加粗）

## 三、本批未做（open）

| 项 | 说明 |
|---|---|
| **PR-3b 报表链路** | 意图分布进报表：intent 落库（chat-svc 消息列 or 事件透传）→ analytics `intentDistribution`（替代旧单体 EmotionalSupportRate）→ BFF → 前端饼图；涉及 §契约 5（新枚举列）——独立批次 |
| LLM 式分类增强 | 当前规则式；可后续在 key 可用时用 LLM 重分类、规则式做兜底 |
| llm-service Nacos 注册 / prod 独立 bff-client 证书 | stage-81 遗留，不变 |
| Kafka P3 / §1.4 | 不变 |

## 四、调研依据

- 已读：emotion-llm-service/{main.py（规则式风格参照）,grpc_server.py,intent.py（新）,
  Dockerfile,tests/unit/{test_intent,test_grpc_server}.py}、proto/emotion_llm.proto、
  docs/plans/{intent-classification-6-types,ai-response-structured}.md（含核查注记）
- 已查：stage-81（挂载点结论）、stage-78 核查（6 分类定义复用结论）
- 命令证据：pytest 135/135；容器 e2e 输出（4 分类 + 首帧 intent，上录）；
  check_proto_layout ✅；shared go build ✅
- 关联：决策 4、stage-80/81（PR-1/2 前置）、§契约 6（离线可用哲学）

---

> 最后更新：2026-09-12 by Stage 82 实施 session
> 关联：llm-chat-real-pipeline.md、intent-classification-6-types.md、ai-response-structured.md
