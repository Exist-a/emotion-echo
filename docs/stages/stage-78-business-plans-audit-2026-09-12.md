# Stage 78 — 业务功能计划排期核查（三份 planned 计划 vs 当前架构）

> 日期：2026-09-12
> 来源：Stage 77 收口后 roadmap open 项"业务功能计划排期"→ 用户指示"按你推荐的来下一个"
> 性质：**核查 + 重排批次（0 业务代码改动）**。按 AGENTS.md §〇 文档功课，三份 2026-07
> 单体时代计划动笔实施前，先逐一与当前分布式代码库对账。

## 一、核查结论总表

| 计划 | 原假设 | 代码事实 | 结论 |
|---|---|---|---|
| ai-response-structured | 阶段 1 前端 Markdown 未做 | **已实现**：`marked@18` + `vue-dompurify-html@5` 已装已接（`plugins/vueInject.ts`、`[id].vue:29` v-dompurify-html） | 阶段 1 销账；阶段 2 阻塞（`internal/pkg/llm/chain.go` 旧单体路径不存在，聊天回复实为 BFF mock） |
| intent-classification-6-types | 改 Kimi intent prompt + 报表 EmotionalSupportRate | 意图分类**零命中**（ai-svc/analytics-svc/BFF/web 均无）；现有分类是情绪 9 分类（llm-service 规则式）；旧单体链路未迁移 | 整链路需按新架构重写；前置 = 真实 LLM 链路 |
| file-upload-message-extension | 后端 upload 已有、前端全缺 | 后端 upload_handler 已有；**ChatFile.vue 已存在**（image/video/file 三态）；残余 = handleAttachment 空实现 + contentType 类型收窄 + 无上传 composable | 可独立实施，是三份中唯一不被 LLM 链路阻塞的 |

**关键发现**：AI 聊天回复当前是 **BFF mock**（`web-bff/internal/handler/ai_stream_handler.go:22`
头注释自证"当前为 mock 实现（无真实 LLM 对话）"）——这是两份 AI 计划的共同缺失前置，
且该前置从未立过计划 → 新增 [`docs/legacy-plans/landed/llm-chat-real-pipeline.md`](../legacy-plans/landed/llm-chat-real-pipeline.md)。

## 二、产出

1. 三份计划各加 2026-09-12 核查注记（就地更正过期前提，保留原文供历史追溯）
2. 新计划 `docs/legacy-plans/landed/llm-chat-real-pipeline.md`（2026-09-12 迁 landed；PR-1 llm-service ChatCompletion RPC →
   PR-2 BFF 切 gRPC 上游 → PR-3 解锁意图分类与结构化回复）
3. roadmap open 清单重排

## 三、重排后的业务功能实施顺序（推荐）

1. **file-upload-message-extension 收口**（可立即实施，不依赖 LLM；前端 3 处 + chat-svc
   ContentType 枚举 §契约 5 核查）
2. **llm-chat-real-pipeline PR-1/PR-2**（真实 LLM 对话；mock 保留为无 key 兜底）
3. **intent-classification-6-types**（重写版）→ **ai-response-structured 阶段 2**
   （两者被 2 解锁后依次落地）

## 四、本批未做（open）

| 项 | 说明 |
|---|---|
| Kafka P3 / §1.4 可选 | 不变（stage-77 §四） |
| Nacos 深水区（可选） | 不变 |
| file-upload 收口实施 | 上文推荐顺序 1，待下轮开工 |
| llm-chat-real-pipeline PR-1 | 上文推荐顺序 2 |

## 五、调研依据

- 已读：emotion-echo-web/{package.json,app/plugins/vueInject.ts,app/pages/chat/conversation/[id].vue,
  app/components/ChatFile.vue,app/types/api.ts,app/stores/message.ts}、
  web-bff/internal/handler/{ai_stream_handler,upload_handler}.go、
  emotion-llm-service/main.py、ai-svc/internal/{aiclient,consumer} 关键文件
- 已查：docs/plans/ 三份原文 + legacy-plans/landed/backlog-order-2026-09-12.md、
  decisions.md 决策 4、stage-77 §四
- grep 实证：intent/EmotionalSupportRate/kimi/moonshot 零命中清单；
  marked/vue-dompurify-html 装机证据；ChatFile.vue 存在性
- 关联：AGENTS.md §〇（文档功课硬规则——本次即为按规则执行的对账批次）

---

> 最后更新：2026-09-12 by Stage 78 核查 session
> 关联：llm-chat-real-pipeline.md、三份被核查计划、stage-77 §四
