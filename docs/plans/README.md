---
purpose: 当前有效、未来排期的功能计划（只列**尚未落地**的项；已落地的在 legacy-plans/landed/）
status: **测试找问题修复阶段** · 2026-10-13 计划清查轮：已做完的 sprint/backlog 计划迁 `legacy-plans/landed/`，本索引重建为"真实待办"
last-refresh: 2026-10-13（计划清查轮：按真实代码逐个核对"疑似过期"计划 → 已做完归档 12 份、半成品改 partial 2 份、失效改 superseded 1 份；本表只保留仍未落地的真实待办）
---

# 当前有效的计划

> **📌 下一大阶段 =「闭环」阶段（Phase Closure · CL）** —— 主线"用户实测找问题 + 链路闭环"。**阶段路线图目录**：[`docs/closure-roadmap/`](../closure-roadmap/README.md)（阶段排序 + 每阶段"做到什么程度"见 [roadmap.md](../closure-roadmap/roadmap.md)）。
>
> 这些计划**尚未落地**，等待排期与实施。
> **已落地的计划**在 [`legacy-plans/landed/`](../legacy-plans/landed/)（只作回顾，不在本表）。
> **核对口径**：本表状态以**真实代码 / 产物**为准（非文档自述）。2026-10-13 清查轮发现的历史"`status: planned` 但其实做完"的项已归档。

## 当前待排期条目（20）

### A. 对话 / Prompt / 前端体验（"闭环"主线）

| 文件 | 主题 | 优先级 |
|---|---|---|
| `user-custom-agent-persona.md` | **用户自定义 Agent（背景/人格 → 提示词约束）**：现状人设由测评五维自动推断、用户不可填。目标 = 用户可设名字/背景/语气/边界 + 注入 prompt（含越狱护栏） | high |
| `chat-emotion-realtime-display.md` | **对话页「用户当前情绪」实时回显**：融合产物 `fused_emotions` 算了但前端零消费；顶部状态条 + 消息融合标签 + 打通数字人情绪 | high |
| `frontend-message-cache-offline-resilience.md` | 前端消息链路：游标分页 + 本地缓存（IndexedDB）+ 状态机 + 断网重试。现 `messageCache.ts` 全仓零调用 | high |
| `fusion-result-consumption.md` | 融合产物 `fused_emotions` 被**统计/图表/报表**消费（`daily_emotion_fused_v` + 日报融合分布 + 注入 prompt） | medium |
| `conversation-memory-pending-decision-2026-09-24.md` | 会话记忆系统缺失（同会话历史 / 跨会话记忆形态，**待决策**） | high |

### B. 可靠性 / 异步管线

| 文件 | 主题 | 优先级 |
|---|---|---|
| `dlq-hardening-and-replay.md` | DLQ 硬化（兜底表）+ 半自动回放。现只有基础 DLQ publisher + `scripts/replay_dlq` | medium |
| `outbox-trace-continuity.md` | outbox→relay→Kafka 的 trace 断点修复（outbox 无 trace 列 + relay 用 `context.Background()`） | medium |
| `kafka-pipeline-pending-decisions.md` | Kafka 管线待决策清单 | medium |
| `known-issues-backlog-runtime-bugs-2026-09-17.md` | **partial**（2026-10-13 核查）：R-09~R-16 / R-01~06 / R-14 已解决或属运维类；**残留 = R-13**（chat-svc 标题异步生成，核查发现当前无 GenerateTitle 实现，条目前提疑似失效） | low |

### C. 可观测性

| 文件 | 主题 | 优先级 |
|---|---|---|
| ~~`observability-business-metrics-wiring.md`~~ | ✅ **已落地**（阶段 CL-01，2026-10-14）→ [legacy-plans/landed/](../legacy-plans/landed/observability-business-metrics-wiring.md) | — |
| ~~`observability-instrumentation-and-dashboards.md`~~ | ✅ **已落地**（阶段 CL-01，2026-10-14）→ [legacy-plans/landed/](../legacy-plans/landed/observability-instrumentation-and-dashboards.md)（一处豁免：BFF `downstream/chat.go` 属 C 轨，随 CL-02） | — |
| `grafana-dashboard-gaps-2026-09-30.md` | Grafana 面板覆盖度缺口分析（B1~B9） | medium |
| `redis-two-layer-cache.md` | Redis 双层读缓存 | medium |

### D. 架构 / 契约

| 文件 | 主题 | 优先级 |
|---|---|---|
| `grpc-deadlink-actualization.md` | **gRPC 死链路坐实与清理**：7 条死 RPC（StreamMessages/Logout/VerifySecurityAnswer/AnalyzeBatch + 3 条 mental-health）逐条裁决"接真/归档/清除"，修正决策 4 的"100% 覆盖"口径，守卫升级 | medium |
| `known-issues-backlog-2026-09-16.md` | **partial**（2026-10-13 核查）：A2/A3/A4/A5/A6 已解决；**残留 = 顺手1 字号防抖**（代码无 debounce） | low |

### E. 端侧化（Lane O）

| 文件 | 主题 | 优先级 |
|---|---|---|
| `on-device-hybrid-inference-2026-09-23.md` | 端侧混合推理方案 v0.2（可行性评估） | medium |
| `on-device-hybrid-inference-implementation-roadmap-2026-09-23.md` | 端侧实施长期路线图 v0.3 | medium |

### F. backlog / 杂项

| 文件 | 主题 | 优先级 |
|---|---|---|
| `todo-pile-2026-09-04.md` | 待办堆栈（dev 未可用 / 未关闭项汇总，**2026-10-13 未逐一核对**） | low |
| `three-vrm-usage-reference.md` | Three-VRM API 参考手册（持续维护型，非任务） | low |

## 状态为 superseded（4，保留原位）

`e2e-16-audio-facial-testing-boundaries-2026-09-22.md`（发现项已随 E2E-16/17 收口）、`observability-compose-gap.md` / `observability-testing-gap.md`（被 `observability-sprint-b` 取代）、`wechat-qq-login-and-upload.md`。详见各自 front-matter。

## 非计划文档（端侧化报告 / 决策材料）

`on-device-STATUS.md`、`on-device-*.md`（baseline-report / cdn-probe-report / compile-cdn / decision-pack / golden-n13-report / mindchat-survey / model-selection-decision-material / t3-iab-report / t3-lessons-learned）、`on-device-findings.md`（active）。这些是 Lane O 的过程产物，非"待排期计划"。

## 2026-10-13 计划清查轮 · 已归档到 `legacy-plans/landed/`（13 份）

| 文档 | 归档依据（真实代码/产物） |
|---|---|
| `sprint-109a-apisix-jwt-401-2026-09-16.md` | stage 文档在；真根因 = BFF 未配 `BFF_APISIX_CIDRS`（R-16 / bc61896） |
| `sprint-109b-end-to-end-chat-2026-09-16.md` | stage + screenshots；`e2e/chat-flow.spec.ts` + `chat-core.spec.ts`；E2E-10 done |
| `sprint-109c-data-contract-smoke-2026-09-16.md` | 由 E2E-30 承接；`scripts/smoke_data_layer.py` 覆盖 §1/2/3/4/6（24/24 exit 0） |
| `sprint-110-a8-a9-a10-fix-2026-09-17.md` | stage 在；A9 `index.vue:407-415`、A10 `[id].vue:859-874` |
| `known-issues-backlog-append-sprint-109b-2026-09-16.md` | A8 → R-10/R-11；A9/A10 同上；7 个 UI bug 均 ✅ |
| `pending-browser-tests-2026-09-18.md` | `tests/verify-a8.spec.ts` 存在；A8/A9/A10 全修 |
| `complete-e2e09-and-cleanup-2026-09-19.md` | roadmap E2E-09 ✅ done；`e2e/registration.spec.ts` |
| `db-migration-auto-apply.md` | `deploy/db/migrate.sh` + 迁移/诊断测试在（commit 0d17e85 起） |
| `multi-round-iteration-2026-09-15.md` | Stage 100/101/§十六 收口（README 原有 landing 记录） |
| `observability-sprint-b.md` | Stage 44~50 全落地（PR-OBS-1~16 + 1 fix） |
| `stage-93-analytics-svc-sw8-propagation.md` | Stage 93 landed（sw8 透传，镜像 v0.1.6） |
| `tts-api-cosyvoice2-f198-2026-10-06.md` | F-198 landed；BFF config 默认 `TTS.Provider=auto`（CosyVoice2 主 + XTTS 回退） |
| `prompt-pipeline-redesign.md` | **CL-04 落地**（PR #213，2026-10-14）：统一组装层 + 同会话历史滑动窗口 + 参数下发 + 文案集中 + 死路径清零；证据见 `docs/closure-roadmap/stages/cl-04-prompt-assembly/report.md` |

## 写入规范

- 新增计划时文件名用 kebab-case 英文
- 文件顶部加 front-matter：`status: planned` / `priority: high|medium|low`
- 引用 ADR 与相关 stage 时用相对路径
- 完成后迁移到 [`legacy-plans/landed/`](../legacy-plans/landed/)（**注意**：迁移会使文件内的相对链接深一层，迁移时须同步修正该文件自身内部链接 + 全仓指向它的入站链接）
