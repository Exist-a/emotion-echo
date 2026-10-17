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

## 当前待排期条目（**2026-10-17 重排：每条必须标宿主阶段**）

> 🔴 **新增硬规则（2026-10-17 教训）**：计划若**没有宿主阶段**，就永远不会被任何阶段的验收检查到，
> 也会在"按账本清销"时被漏掉 —— `camera-preview-floating-window` 就是这样漂了 2 天（设计已定稿、契约已占位，但 roadmap 无阶段）。
> **今后新增计划必须同时：① 指定宿主 CL 阶段；② 登进 `docs/closure-roadmap/discovered-unresolved.md`（CL-F-xx）。**

### A. 对话 / Prompt / 前端体验

| 文件 | 主题 | 宿主阶段 | 优先级 |
|---|---|---|---|
| `user-custom-agent-persona.md` | **用户自定义 Agent**（多角色 + 会话级切换 + 音色 + 安全信封） | **CL-06** | high |
| `../legacy-plans/landed/chat-emotion-realtime-display.md`（**已落地**） | 对话页「用户当前情绪」实时回显 | CL-07 ✅ | — |
| `../legacy-plans/landed/fusion-result-consumption.md`（**已落地**） | 融合产物被统计/图表/报表消费 | CL-07 ✅ | — |
| `conversation-memory-pending-decision-2026-09-24.md` | 会话记忆系统（跨会话 RAG + 用户事实/情绪模式/情节/管理页/风险信号） | **CL-08a~e** | high |

### B. 前端体验（**CL-11**，2026-10-17 用户裁定单开）

| 文件 | 主题 | 宿主阶段 | 优先级 |
|---|---|---|---|
| `camera-preview-floating-window.md` | **摄像头预览：悬浮可拖可折叠**（设计已定稿；**当前是内联块、零改动**） | **CL-11** | medium |
| `mental-health-history-trend-entry.md` | 「我的空间」评估历史 / 趋势 / 立即评估入口 | **CL-11** | low |
| `known-issues-backlog-2026-09-16.md` | **partial**：残留 = 字号调整防抖（实测确认未做） | **CL-11** | low |
| ——（无独立计划文件） | `CL-F-28`：web 镜像构建 ERESOLVE（**CL-11 的前置**：不修则本阶段无法做 UI 验收） | **CL-11 前置** | high |

### C. 后端可靠性（**CL-12**，2026-10-17 用户裁定单开）

| 文件 | 主题 | 宿主阶段 | 优先级 |
|---|---|---|---|
| `kafka-pipeline-pending-decisions.md` | Kafka 管线未决：**D2**（outbox 行无清理归档）/ **D3**（attempts 不跨重启）/ **D4**（maxRetries 硬编码）；D1 已由 CL-03 落地 | **CL-12** | medium |
| `redis-two-layer-cache.md` | Redis 双层读缓存（实测**未落地**；属优化项非缺陷） | **CL-12** | medium |
| ——（无独立计划文件） | BFF 登录限流 `isLocked`/`recordFailure` 补单测 | **CL-12** | low |

### D. 可观测性

| 文件 | 主题 | 宿主阶段 | 优先级 |
|---|---|---|---|
| `grafana-dashboard-gaps-2026-09-30.md` | Grafana 面板覆盖度缺口（`type: gap-analysis`，B1~B9 **残余未逐条判定**） | **CL-09**（触发：下次动观测面板） | medium |

### E. backlog / 杂项

| 文件 | 主题 | 宿主阶段 | 优先级 |
|---|---|---|---|
| `known-issues-backlog-runtime-bugs-2026-09-17.md` | **残留已清**（R-13 于 2026-10-17 销账：BFF 已实现 `maybeGenerateTitle`，前提失效） | 可迁 landed（**未逐条独立复核**） | low |
| `todo-pile-2026-09-04.md` | 待办堆栈（**2026-10-17 逐条核对中**，多数已过期） | **CL-09**（核对后并账） | low |
| `test-coverage-tracker-2026-09-16.md` | 业务路径级测试覆盖率**追踪表**（治理工具，非功能任务） | 不排期 | — |
| `three-vrm-usage-reference.md` | Three-VRM API 参考手册（持续维护型） | 不排期 | low |

### F. 端侧化（Lane O，**CL-10 deferred**）

| 文件 | 主题 | 宿主阶段 | 优先级 |
|---|---|---|---|
| `on-device-hybrid-inference-2026-09-23.md` + `…implementation-roadmap-2026-09-23.md` | 端侧混合推理方案与实施路线图 | **CL-10**（deferred） | medium |
| `hardware-companion-pet-feasibility-2026-10-16.md` | 硬件桌宠可行性探讨（`status: exploratory`，**未立项**） | 不排期 | low |

## 状态为 superseded（4，保留原位）

`e2e-16-audio-facial-testing-boundaries-2026-09-22.md`（发现项已随 E2E-16/17 收口）、`observability-compose-gap.md` / `observability-testing-gap.md`（被 `observability-sprint-b` 取代）、`wechat-qq-login-and-upload.md`。详见各自 front-matter。

## 非计划文档（端侧化报告 / 决策材料）

`on-device-STATUS.md`、`on-device-*.md`（baseline-report / cdn-probe-report / compile-cdn / decision-pack / golden-n13-report / mindchat-survey / model-selection-decision-material / t3-iab-report / t3-lessons-learned）、`on-device-findings.md`（active）。这些是 Lane O 的过程产物，非"待排期计划"。

## 非计划文档（探索 / 可行性讨论）

`hardware-companion-pet-feasibility-2026-10-16.md` —— **硬件桌宠（端云协同）可行性探讨**（front-matter `status: exploratory`）。**未立项、未排期、不承诺落地**；记录"设备 = 采集/输出层、处理全在后端"这一定位的现状核对（含代码 `file:line` 证据）与四个待拍板分叉（输入模态 / 平台档次 / 表情形态 / 配置页归属；价格对比按要求未入档）。**不计入上方待排期条目。**

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
