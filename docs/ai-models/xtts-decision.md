# XTTS 云端 API 选型决策记录（ADR-001 v1 — ❌ Retired）

**日期**：2026-07-17（v1）→ 2026-09-10（v2 替代）→ 2026-09-15（确认云 API 彻底废弃）
**状态**：❌ **Retired**
**关联**：[`adr-2026-09-xtts-v2-decision.md`](../architecture/adr/adr-2026-09-xtts-v2-decision.md)（v2 最终决策）· [`stage-60-pr-tts-vendor-landing.md`](../stages/stage-60-pr-tts-vendor-landing.md)（vendor 落地）

---

> ⚠️ **本文档已 retired。**
>
> v1 决策（阿里云/OpenAI 云 API）从未写过一行代码。Stage 60 确认 vendor `ai4all/coqui:latest` 为唯一最终方案——端到端调通（133KB WAV），音色满足需求，无切换云 API 的必要。
>
> **本文档保留仅作历史参考，不应作为实施依据。** 当前 TTS 架构见 v2 ADR。

---

## 一、v1 决策（原始，已废弃）

采用阿里云智能语音交互作为 Primary，OpenAI TTS 作为 Fallback。

| 角色 | Provider | 触发条件 |
|------|----------|---------|
| Primary | 阿里云 | 国内用户 / 默认 |
| Fallback | OpenAI TTS | 阿里云 5xx / 境外客户端 IP |
| Future | 火山引擎 / ElevenLabs | 用户显式选择 |

## 二、废弃原因

| v1 依据 | 实际情况 |
|---------|---------|
| 本地 XTTS build 4 次失败 | Stage 36 build 成功过；Stage 60 vendor Coqui 端到端跑通 |
| 阿里云音色更适合情感陪伴 | 云 API 从未落地，无对比数据；vendor Coqui 音色当前可用 |
| 月成本 20 元 | dev 阶段本地容器零成本，prod 规模化再评估 |

## 三、最终方案

**vendor `ai4all/coqui:latest` 本地容器 = 唯一 TTS 推理路径。**

详见 [`adr-2026-09-xtts-v2-decision.md`](../architecture/adr/adr-2026-09-xtts-v2-decision.md)。

---

> 最后更新：2026-09-15 — 确认云 API 路径彻底废弃，标记 retired
