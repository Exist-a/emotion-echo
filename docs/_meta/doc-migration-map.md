---
purpose: Round 0 产出 · 文档重构迁移总表
date: 2026-09-03
scope: .trae/ 全部 + 根目录文档 + docs/ 顶层文件
status: 待确认（Round 0 收口，下一步进入 Round 1 骨架搭建）
---

# 文档重构迁移总表（Round 0）

> 本表是 Round 0 的唯一交付。每份历史文档的最终归宿都已在此表内锁定。
> **判定档位**：🟢 已落地（功能在 stage/ADR/commit 中有证据） / 🟡 已偏移（大方向对但执行细节已变） / 🟠 仍有效（未被任何 stage 取代） / ⚫ 历史价值（仅作决策回顾）

---

## 一、.trae/documents/ · 顶层 24 份

### 1.1 单文件计划（15 份）

| 原文件 | 判定 | 归宿（Round 2 搬迁目标） | 证据 / 说明 |
|--------|------|------------------------|-------------|
| `ai-response-structured.md` | 🟠 仍有效 | `docs/plans/ai-response-structured.md` | AI 回复结构化 + Markdown 渲染。属于"未来功能"风格，未见对应 stage 收口报告；前端 `marked` 已装，但分支表格 `study_help`/`tech_help` 等分类未在 stage 文档中出现 |
| `context_management_plan.md` | 🟡 已偏移 | `docs/legacy-plans/shifted/context-management-plan-langchain.md` | 写的是基于 `tmc/langchaingo v0.1.14` 的 TokenBuffer 上下文方案；Stage 35 LLM Fusion 已用自有实现 + ADR-15 收口，原文作历史回顾保留 |
| `digital-human-feature-plan.md` | 🟢 已落地 | `docs/legacy-plans/landed/digital-human-phase1-plan.md` | 3D 数字人 Phase 1 计划；对应 `docs/stage-XX`（隐含在 stage-22~25）+ `.trae/specs/digital-human-phase1/tasks.md` 全 ✓ |
| `llm-container-deployment-plan.md` | 🟢 已落地 | `docs/legacy-plans/landed/llm-container-deployment-plan.md` | FER/SenseVoice/XTTS Docker 化方案；对应 Stage 22-A `stage-22-ai-services-containerization.md` + `deploy/docker-compose.apps.yml` 已配置 :8002/:8003/:8004 |
| `phase3-implementation-plan.md` | 🟢 已落地 | `docs/legacy-plans/landed/xtts-lipsync-phase3-plan.md` | XTTS 集成 + 口型/表情同步 Phase 3；归属 Stage 24~25 多模态融合 + 数字人 Phase 2 区间，对应 stage-25-final-* 系列 |
| `pitch-ppt-requirements.md` | 🟡 已偏移 | `docs/legacy-plans/shifted/pitch-ppt-requirements.md` | 路演 PPT 需求；技术栈写"SQLite + 本地 LLM"，与当前 PostgreSQL + Kimi 远程 LLM 严重不符。作历史归档 |
| `three-vrm-usage-reference.md` | 🟠 仍有效 | `docs/plans/three-vrm-usage-reference.md` | `@pixiv/three-vrm` 骨骼/表情 API 参考手册；非"计划"而是"工具书"，没有 stage 覆盖；当前数字人组件已在用，作为前端开发参考长期保留 |
| `tts_boundary_fix_plan.md` | 🟢 已落地 | `docs/legacy-plans/landed/tts-boundary-fix-plan.md` | 静音按钮 + 发送新消息中断 TTS + 语速 0.75；与 stage-35 production-hardening 时间窗对齐，作为该期修复的前身保留 |
| `tts_optimization_plan.md` | 🟢 已落地 | `docs/legacy-plans/landed/tts-optimization-plan.md` | XTTS 语速/音色/口型/表情时长优化；与上一条同窗期 |
| `上下文管理和消息存储问题检查与修复计划.md` | 🟢 已落地 | `docs/legacy-plans/landed/context-mgmt-message-storage-fix.md` | 重复消息 + 上下文丢失修复；Stage 30-A/B 区间内的修复项；保留作回顾 |
| `上下文管理问题分析和修复方案.md` | 🟢 已落地 | `docs/legacy-plans/landed/context-mgmt-analysis-fix.md` | langchaingo LoadMemoryVariables bug 修复；同上区间 |
| `后端工作流分析.md` | 🟡 已偏移 | `docs/legacy-plans/shifted/backend-workflow-analysis-pre-textpkg.md` | 三个工作流重复分析；Stage 35 + `.trae/specs/text-workflow-refactor` 收口，workflow root 已删 |
| `后端拆分规划.md` | 🟡 已偏移 | `docs/legacy-plans/shifted/monolith-service-split-plan.md` | `emotion-echo-gin` 单体 service 拆分；该单体已迁 `legacy/emotion-echo-gin/`，整体拆分为 5 微服务；本文作"单体时代最后一次拆分尝试"回顾 |
| `工作流边界分析.md` | 🟢 已落地 | `docs/legacy-plans/landed/workflow-boundary-analysis.md` | workflow root / chat / assessment 边界；同 `后端工作流分析`，已被 text-workflow-refactor 解决 |
| `微信QQ登录和文件上传实施计划.md` | 🟠 仍有效 | `docs/plans/wechat-qq-login-and-upload.md` | QQ OAuth 未实现、通用文件上传未完全收口；前端 `ChatFile` 组件在 `docs/stage-30-C-kafka-ext-backlog.md` 之前的 backlog 中未确认收口 |

### 1.2 中文 + 业务名子目录（7 个）

| 原文件/子目录 | 判定 | 归宿（Round 2 搬迁目标） | 证据 / 说明 |
|----------|------|------------------------|-------------|
| `ai-stream-cancel-feature/plan.md` | 🟢 已落地 | `docs/legacy-plans/landed/ai-stream-cancel/plan.md` | AI 回复截断/取消按钮；与 `.trae/specs/ai-stream-cancel-fix` 同期，对应前端 `message.ts` 中 `cancelAIStream()` |
| `ai-stream-cancel-feature/` 目录下的 checklist/tasks | ⚫ 重复 | **不搬迁**（与 `.trae/specs/ai-stream-cancel-fix/` 重复，统一保留后者） | 见 1.3 |
| `conversation-title-generation/{spec,checklist,tasks}.md` | 🟡 已偏移 | `docs/legacy-plans/shifted/conversation-title-generation.md` | 三份合并为一份，前端会话列表显示由 BFF 阶段实现，标题 AI 生成未见对应 stage 收口；作回顾 |
| `frontend-refactor/elementplus-replacement.md` | 🟢 已落地 | `docs/legacy-plans/landed/elementplus-to-native.md` | 标题已写"已落地 · 2026-07-17"，对应 Stage 26-O frontend redesign |
| `frontend-refactor/frontend-refactor-plan.md` | 🟡 已偏移 | `docs/legacy-plans/shifted/frontend-v2-nuxtui-plan.md` | 原计划 Nuxt UI v4，最终方案是"完全不下 UI 库"；与 elementplus-replacement.md 合并阅读；保留原方案作决策回溯 |
| `multimodal-emotion-feature/plan.md` | 🟢 已落地 | `docs/legacy-plans/landed/multimodal-emotion-plan.md` | 多模态情绪功能整体规划；对应 `docs/stage-34-multimodal-fusion.md` + ADR-15 |
| `multimodal-emotion-feature/backend-plan.md` | 🟢 已落地 | `docs/legacy-plans/landed/multimodal-emotion-backend.md` | 后端实现细节（face 0.5/voice 0.3/text 0.2 权重）；同上 stage |
| `voice-message-feature/{spec,checklist,tasks}.md` | 🟢 已落地 | `docs/legacy-plans/landed/voice-message-feature.md` | 三份合并为一份；语音消息录制/上传/识别，Stage 30-A 范围内 |

---

## 二、.trae/specs/ · 3 套共 9 份（spec + checklist + tasks）

| 原文件 | 判定 | 归宿（Round 2 搬迁目标） | 证据 / 说明 |
|--------|------|------------------------|-------------|
| `ai-stream-cancel-fix/{spec,checklist,tasks}.md` | 🟢 已落地 | `docs/legacy-plans/landed/ai-stream-cancel-fix.md`（三合一） | 取消成功提示 + AI 记忆历史；与 `documents/ai-stream-cancel-feature/` 内容 90% 重叠，取更完整的此套作主源 |
| `digital-human-phase1/{spec,checklist,tasks}.md` | 🟢 已落地 | `docs/legacy-plans/landed/digital-human-phase1.md`（三合一） | 数字人 Phase 1 全 ✓；与 `documents/digital-human-feature-plan.md` 互为补充 |
| `text-workflow-refactor/{spec,checklist,tasks}.md` | 🟢 已落地 | `docs/legacy-plans/landed/text-workflow-refactor.md`（三合一） | text 包 + 4 节点重构；workflow root 已删；与 `documents/后端工作流分析.md` 互补 |

---

## 三、根目录文档（4 份关键 + 缺失文件）

| 文件 | 判定 | 处理（Round 3 顺手处理） | 说明 |
|------|------|-----------------------|------|
| `README.md` | 🟠 仍有效 | **保留根目录**，更新文档导航小节指向新 `docs/` 结构 | 项目门面；保留作唯一根入口 |
| `QUICKSTART.md` | 🟠 仍有效 | **保留根目录**，Round 4 校验端口与现状一致性 | 启动流程主入口 |
| `AGENTS.md` | 🟠 仍有效 | **保留根目录**，Round 4 末尾追加一行："未来新增功能计划写到 `docs/plans/`" | 强约束协作约定，不动主体 |
| `LICENSE` | ❌ **缺失** | Round 4 创建占位 `MIT` LICENSE（README 写 "MIT" 但文件不存在） | 仓库 README 标注 MIT，但仓库无该文件 → 建议补 |
| `docker-compose.yml`（根目录，3692B） | ⚠️ 需检查 | Round 3 验证是否被 `deploy/` 取代；若是则归档到 `docs/deployment/legacy-yml/` | 顶层文件但内容像迁移期产物 |
| `err.log` / `out.log` / `msg1.json` / `msg2.json` / `pg_log.txt` / `apisix-*.json`（根散落） | ⚫ **运维残留** | Round 4 列入 `.gitignore` 候选 + 报告"应清理"清单，**不主动删**（按"不删除任何文件"原则） | 与文档重构无关，仅记录 |

---

## 四、docs/ 顶层文件（Round 3 归位表）

| 原路径 | 归位目标 | 类型 |
|--------|---------|------|
| `docs/architecture-decisions.md` | `docs/architecture/decisions.md` | 单一事实源 ADR |
| `docs/architecture-positioning.md` | `docs/architecture/positioning.md` | 分布式定位说明 |
| `docs/distributed-architecture.md` | `docs/architecture/distributed.md` | 分布式架构总览 |
| `docs/distributed-roadmap.md` | `docs/architecture/roadmap.md` | 演进路线图（历史过程记录） |
| `docs/adr-2026-09-*.md`（6 份） | `docs/architecture/adr/adr-2026-09-*.md` | ADR 编号 |
| `docs/git-layout.md` | `docs/deployment/git-layout.md` | 仓库布局 |
| `docs/deploy/README.md` | `docs/deployment/docker-compose.md` | 部署说明（迁移自 deploy/） |
| `docs/ai-images-build-guide.md` | `docs/ai-models/build-guide.md` | AI 模型构建指南 |
| `docs/xtts-cloud-api-decision.md` | `docs/ai-models/xtts-decision.md` | XTTS 决策 |
| `docs/xtts-cloud-api-integration.md` | `docs/ai-models/xtts-integration.md` | XTTS 集成 |
| `docs/stage-0..stage-25*.md`（约 30 份） | `docs/stages/stage-XX-*.md` | 历史演进 |
| `docs/stage-26..stage-36*.md`（约 50 份） | `docs/stages/stage-XX-*.md` | 同上 |
| `docs/stage-3{4,5}-*-runbook.md` | `docs/deployment/runbook/stage-3X-*.md` | 运维手册 |
| `docs/microservice-decomposition-plan.md` | `docs/architecture/decomposition-plan.md` | 微服务拆分 |
| `docs/microservices-architecture.md` | `docs/architecture/microservices.md` | 微服务架构 |
| `docs/architecture-audit-2026-08-31.md` | `docs/architecture/audit-2026-08-31.md` | 架构审计 |
| `Emotion-Echo-Web/docs/DESIGN.md` | `docs/frontend/design.md` | 前端设计 |
| `docs/learn/`（13 份） | **原位保留** | 学习教程 |

---

## 五、汇总计数

| 类别 | 总数 | 🟢 landed | 🟡 shifted | 🟠 still-valid | ⚫ historical |
|------|-----|----------|-----------|---------------|--------------|
| .trae/documents/ 顶层单文件 | 15 | 8 | 5 | 2 | 0 |
| .trae/documents/ 子目录（含 1 重复不搬） | 7 项 → 7 份 | 5 | 1 | 1 | 0 |
| .trae/specs/ 套件（合并三合一） | 3 → 3 份 | 3 | 0 | 0 | 0 |
| 根目录文档 | 3 | 0 | 0 | 3 | 1（LICENSE 缺失） |
| docs/ 顶层文件（Round 3 准备） | 100+ | — | — | — | — |
| **.trae 总共待搬迁** | **24 份**（25 份子目录 - 1 份重复） | **16** | **6** | **3** | **0** |

---

## 六、判定边界与例外

### 6.1 不删除任何文件的原则

- 即便部分文档技术栈严重过时（如 `pitch-ppt-requirements.md` 的 SQLite/本地 LLM），仍作为历史归档保留
- `.trae/` 整体删除只在 Round 2 末尾、所有内容已搬迁到 `docs/legacy-plans/` 或 `docs/plans/` 之后执行

### 6.2 重复内容处理

- `documents/ai-stream-cancel-feature/` 与 `specs/ai-stream-cancel-fix/` 内容 90% 重叠 → **取 specs 套件**作主源（spec 字段更全），documents 下的不搬迁
- `documents/digital-human-feature-plan.md` 与 `specs/digital-human-phase1/` 互补 → **两份都搬**，合并阅读

### 6.3 文件头部状态头模板（Round 2 写入每份归档文件顶部）

```markdown
---
status: shifted   # 🟢 landed / 🟡 shifted / 🟠 still-valid / ⚫ historical
superseded-by: docs/stage-XX-...
original-path: .trae/documents/后端拆分规划.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
---
```

### 6.4 暂未判定（Round 4 处理）

- `docs/stage-0-learnings.md` 与 `docs/learn/00-index.md` 重复内容
- 多份 `stage-25-*` 收口报告之间是否需要精简合并
- 根目录散落文件（`err.log`/`apisix-*.json` 等）的 `.gitignore` 处理

---

## 七、待用户确认

无重大歧义。可直接进入 Round 1（搭建 `docs/` 新骨架空目录）。如对某份归档文件归宿有异议，单独指出即可，本表是可调整的"工作文档"。
