---
purpose: 历史计划归档（仅作回顾）
status: Round 1 占位 · Round 2 首批内容迁入
---

# 历史计划归档

> 仓库演进过程中的功能计划、实施方案、设计稿。**仅作回顾，不再生效**。
> 这些文档作为个人作品集的决策历史被保留。

## 三档判定

| 档位 | 标识 | 含义 | 子目录 |
|------|------|------|--------|
| 🟢 已落地 | `status: landed` | 功能在 stage/ADR/commit 中有证据，已被取代 | `landed/` |
| 🟡 已偏移 | `status: shifted` | 大方向对但执行细节已变（如 Nacos 重引入、APISIX 退役、技术栈升级） | `shifted/` |
| ⚫ 历史价值 | `status: historical` | 记录"为什么当初这样想"的决策思路，本身已被 ADR 取代 | `historical/` |

## 当前条目

详见 [doc-migration-map.md](../_meta/doc-migration-map.md)。

### landed/（Round 2 迁入）

- `digital-human-phase1-plan.md`
- `llm-container-deployment-plan.md`
- `xtts-lipsync-phase3-plan.md`
- `tts-boundary-fix-plan.md`
- `tts-optimization-plan.md`
- `context-mgmt-message-storage-fix.md`
- `context-mgmt-analysis-fix.md`
- `workflow-boundary-analysis.md`
- `ai-stream-cancel/plan.md`（被 `.trae/specs/ai-stream-cancel-fix/` 替代）
- `elementplus-to-native.md`
- `multimodal-emotion-plan.md`
- `multimodal-emotion-backend.md`
- `voice-message-feature.md`
- `ai-stream-cancel-fix.md`
- `digital-human-phase1.md`
- `text-workflow-refactor.md`

### shifted/（Round 2 迁入）

- `context-management-plan-langchain.md`
- `pitch-ppt-requirements.md`
- `backend-workflow-analysis-pre-textpkg.md`
- `monolith-service-split-plan.md`
- `frontend-v2-nuxtui-plan.md`
- `conversation-title-generation.md`

### historical/（Round 2 期间视需要使用）

> 当前没有归到 ⚫ 档的文档。

## 阅读建议

- 当你需要了解"为什么这个功能当时是这样实现的"，看 `landed/`
- 当你需要了解"原本的计划和最终落地有什么差异"，看 `shifted/`
- 当你需要回顾某次决策的初衷，看 `historical/`
