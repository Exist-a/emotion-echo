# Stage 87 — LLM 意图重分类（规则式兜底，key 可用时消歧）

> 日期：2026-09-13
> 类型：feat + fix（TDD：RED `937941f` → GREEN `ecce136` → fix `3cb4f1a`）
> 关联：roadmap open 清单第 2 项、Stage 82 §三 open「LLM 式分类增强」、
> [intent-classification-6-types.md](../legacy-plans/landed/intent-classification-6-types.md) residuals
> 背景：本 session 先调查下一步工作——发现上轮据以推荐的 grpc-inter-service-migration
> 「剩余」段是过期信息（#32 已在 Stage 64 关闭、BFF error 映射已在 Sprint G 关闭、
> PinConversation 已在 Stage 72 落地），先做文档销账（`c68c883`），再从真实 open 清单
> 选定本项。

## 一、设计

**决策序**（`emotion-llm-service/intent_llm.py::classify_intent_adaptive`）：

1. 规则式（Stage 82 `intent.classify_intent`）先分类；
   **高置信**（非 other 且 confidence ≥ 0.5，即 ≥3 次关键词命中）→ 直接返回，零 LLM 开销
2. env `LLM_INTENT_RECLASSIFY` 关闭（"0"/"false"/"no"）→ 返回规则结果
3. `LLM_API_KEY` 缺失 → 返回规则结果（**离线路径与 Stage 82 行为完全一致**，§契约 6）
4. LLM 非流式调用重分类（temperature=0、max_tokens=20、timeout=5s、**max_retries=0**）
   得合法非 other 标签 → `(label, 0.9)`
5. 其余（LLM 失败 / 输出 other / 输出非法）→ 原样返回规则结果（弱规则命中不降级成 other）

**消费点**：`ClassifyIntent` RPC（BFF sendMessage 发送前标注意图 → 落库）与
`ChatCompletion` with_intent（风格指令注入 + 首帧回带）两处切自适应分类。
**RPC 契约不变，Go 侧（chat-svc / BFF / analytics）零改动**。

**为什么弱置信（0.4）也送 LLM**：2 次命中的结果（Stage 82 e2e 中 career/emotional 都在此档）
恰是最容易错路由的区间——这正是"歧义消歧"的目标场景；prompt 约束 6 类单选 +
LLM 判 other 时保留规则结果，双重防误路由。

## 二、e2e 揪出的真问题：SDK 重试放大降级耗时 3 倍

容器 e2e 首跑实测：**上游不可达时降级耗时 16.2s**（预期 ≤5s）。
根因：openai SDK 默认 `max_retries=2`，per-request 传 `timeout=5` 只限单次尝试，
最坏 5s×3+连接开销。且 per-request `max_retries` kwarg 在 SDK 2.x 不支持
（`Completions.create() got an unexpected keyword argument`）。

修复：`intent_llm` 自建 `_default_openai_client`（`OpenAI(..., max_retries=0)`），
不与 chat 流式共用构造（后者重试语义不动）。复测降级 **2.1s**。
回归锁：`test_client_factory_disables_retries` 断言 `client.max_retries == 0`
（该测试先红后绿，属 GREEN 后追加的 RED-GREEN 微循环）。

## 三、验收

- pytest **156/156**（原 135 + 新 21）：
  - `test_intent_llm.py`（18）：高置信跳过 LLM / 模糊重分类 / 调用契约
    （temperature=0、小配额、6 类 prompt）/ 非法标签与 LLM-other 与异常三路降级 /
    无 key 跳过 / env 关闭 / 弱置信非 other 消歧 / client 工厂禁用重试 /
    `_extract_label` 表驱动 8 例
  - `test_grpc_intent_llm.py`（3）：真实 gRPC——模糊文本 LLM 重分类 /
    **无 key 离线回归锁**（与 Stage 82 纯规则逐字段一致）/ with_intent 首帧消费自适应结果
- **真实容器 e2e**（v0.1.0 重建镜像，mTLS + 内部 API key）：
  ```
  离线路径（dev 无 key，行为不变）：
    confident-rule: intent=tech_help confidence=0.62 elapsed=0.0s
    ambiguous-llm:  intent=other    confidence=0.00 elapsed=0.0s
    weak-rule-llm:  intent=other    confidence=0.00 elapsed=0.0s
  LLM 路径（容器内 mock OpenAI 端点）：
    llm-reachable:   intent=emotional_support confidence=0.90 elapsed=2.0s
    llm-unreachable: intent=other    confidence=0.00 elapsed=2.1s  ← 优雅降级
  ```
- Dockerfile 打包契约测试通过（intent_llm.py 双阶段 COPY）
- §2.4 触发表：本批不触 chat-svc 事件链 / analytics SQL / BFF 报表端点 /
  schema / dev 模式开关——无需跑数据契约 smoke

## 四、本批未做（open）

| 项 | 说明 | 去向 |
|---|---|---|
| LLM 消歧效果评估 | 当前 LLM 标签置信度固定 0.9；真实 key 上线后可对比规则/LLM 分歧率 | 观测项，无代码动作 |
| ClassifyIntent 延迟指标 | 重分类命中率和耗时暂无 metrics（GRPC_REQUESTS_TOTAL 已覆盖 ok/err） | 可选 |

## 五、调研依据

- 已读：`emotion-llm-service/{intent.py,intent_llm.py(新),grpc_server.py,chat_completion.py,
  Dockerfile}`、`tests/unit/{test_intent,test_grpc_server,test_dockerfile_contract}.py`、
  `proto/emotion_llm.proto`、`emotion-echo-web-bff/internal/handler/chat_handler.go`
  （sendMessage 标注链路，确认 Go 侧零改动）
- 已查：Stage 82 §三 open 表、roadmap open 清单（2026-09-13 版）、
  intent-classification-6-types.md residuals、plans/README.md 销账记录、决策 4/18
- 容器实证：e2e 输出（§三所录）；mock 端点 200 响应 `{choices:[{message:{content:"emotional_support"}}]}`

---

> 最后更新：2026-09-13 by Stage 87 实施 session
> 关联：intent-classification-6-types.md、stage-82/83/85、llm-chat-real-pipeline（全线 landed）
