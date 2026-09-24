---
status: proposed
priority: high
created: 2026-09-24
related-plans:
  - ../../plans/on-device-hybrid-inference-2026-09-23.md（v0.2 §2.2 端侧模型选型）
  - ../../plans/on-device-hybrid-inference-implementation-roadmap-2026-09-23.md（v0.3 §B.1 决策 2 + §C.1 阶段一任务 1）
  - ../../plans/on-device-model-selection-decision-material-2026-09-24.md（决策材料全文）
  - ../../plans/on-device-mindchat-survey-2026-09-24.md（姊妹：MindChat 模型侧调研）
related-decisions:
  - D-26 端侧化主方案 ADR（proposed，决策 33）
  - D-26.2 端侧主力模型（本 ADR）
---

# ADR-2026-09 端侧主力模型选型 = WebLLM 预置 Qwen3（Apache 2.0）

> **状态**：🟡 **proposed**（2026-09-24 立项；用户 2026-09-24 会话口头授权"选 B 吧"；待正式拍板后转 `accepted`）
>
> **取代关系**：无（本项目首个端侧模型选型决策；v0.3 §B.1 决策 2 落地）
>
> **关联**：D-26 主方案 ADR（v0.3 §B.2 模板）；D-26.2 = 本 ADR
> **拍板权属用户**（AGENTS §八规则 4）；本 ADR 标 proposed 等用户二次确认
>
> **重要约束**：本 ADR 一旦转 accepted，必须整 D-26（主方案）+ 5 个分项（决策 1/2/3/4/5）全部由用户拍板后，才同步转 accepted（v0.3 §B.1 决议流程）

---

## 一、决策

按 v0.2 §2.2 / v0.3 §B.1 决策 2，端侧主力模型选 **WebLLM 预置 Qwen3-1.7B-q4f16_1-MLC**（Apache 2.0）。

**分级加载方案**（v0.2 §二"分级加载"）：
- **桌面独显**（RTX 3060+）：`Qwen3-4B-q4f16_1-MLC`（vram 3432MB）| 旗舰
- **桌面集显/笔记本**（Intel Iris Xe 等）：`Qwen3-1.7B-q4f16_1-MLC`（vram 2037MB）| **默认**
- **移动端**（骁龙8 Gen3 / A17 Pro）：`Qwen3-0.6B-q4f16_1-MLC`（vram 1403MB）| fallback

**心理垂直能力补强**（v0.2 §6.5 方法论）：
- 指令扁平化 + few-shot 黄金示例对 + 护栏代码兜底（5 类） + 强约束 system prompt

---

## 二、影响面

- **不需要** 自编译 MLC-LLM（Qwen3-1.7B-q4f16_1-MLC 已在 HF 预编译）
- **不需要** 修改 `package.json`（WebLLM 预置即用）；T3 接入 `@mlc-ai/web-llm` 时按 §六握手登记
- **需要** §六握手 + dynamic import 隔离（生产 bundle 不可含 WebLLM 实际运行时）
- **需要** Apache 2.0 NOTICE 致谢（首次加载 footer）
- **不影响** 其他 §十二决策（隐私定位 / 来源告知 / 离线范围 / 摘要存储）—— 这 4 项独立
- **不取代** 任何已有 ADR；仅 D-26 主方案 + 本分项 ADR

---

## 三、调研依据

### 3.1 License 实测（commit 末尾已序列化）

| 模型 | License | 来源 |
|------|---------|------|
| **X-D-Lab/MindChat-Qwen2-0_5B** | **GPL-3.0**（标准，无变种，非 AGPL）| GitHub API `X-D-Lab/MindChat/contents/LICENSE` 实测 = 35149 字节标准文本 |
| **QwenLM/Qwen3 全系** | **Apache 2.0** | GitHub README §License Agreement：_"All our open-weight models are licensed under Apache 2.0"_ |
| **mlc-ai/Qwen3-*-MLC**（MLC 编译产物）| **Apache 2.0**（衍生作品按上游 license 链）| WebLLM README §Integrity Verification + Qwen3 上游 |
| WebLLM 引擎本身 | Apache 2.0 | WebLLM `LICENSE`（Q4F Ruan et al., 2026）|

### 3.2 决策矩阵（10 维度加权）

B（Qwen3 Apache 2.0）完胜 9/10 维度（详见决策材料 §五）：

| 维度 | A MindChat | B Qwen3 | B 优? |
|------|-----------|---------|-------|
| License 类型 | GPL-3.0 强传染 | Apache 2.0 宽松 | ✓ |
| 本项目不商用 | ✅ | ✅ | — |
| **未来转商用** | ❌ 项目须 GPL | ✅ 自由 | ✓ |
| **CDN 分发权重** | ❌ 触发 copyleft | ✅ 兼容 | ✓ |
| **预置 vs 自编译** | ❌ 须 MLC-LLM 编译 | ✅ WebLLM 预置 | ✓ |
| vram 标称 | ~1.4 GB（编译后预估）| 2037 MB（实测）| — |
| 维护活跃度 | 2024-02-06 后未更新（19 个月）| Qwen3-2507（2025-08 最新）| ✓ |
| WebLLM 兼容性 | 须自编译 | ✅ 预置 | ✓ |
| 心理垂直微调质量 | ✅ X-D-Lab 20 万轮 | ❌ 通用基座（§6.5 方法论补）| ✗ |
| **学术综述印证** | 综述无明确 head-to-head 优势 | 通用基座 + 强 prompt + 护栏可达可用线 | ✓ |
| 商用可塑性 | ⚠️ GPL 锁定 | ✅ 自由 | ✓ |
| 跨项目 License 一致 | ⚠️ 与项目 Apache-2.0 冲突 | ✅ 一致 | ✓ |

**净评估**：B 在 9/10 维度优，仅心理垂直微调质量 A 略优（但 §6.5 方法论可补）。

### 3.3 综述印证

> **`Emo-gml/Awesome-Mental-Health-LLMs` TAFFC 2026**（Hu et al., `arXiv:2609.25186`）核心发现：
> - 心理垂直 LLM 质量 = base model × instruction tuning × reward modeling × inference-time prompt × output guardrails
> - 绝大多数心理垂直微调模型基于 **Qwen / InternLM / Baichuan** 通用基座
> - 推荐 License 宽松模型 + 强 prompt + 护栏为临床部署路径

**与本 ADR 一致**：v0.2 §6.5 方法论 = 指令扁平化 + few-shot + 护栏代码兜底，与综述结论契合。

### 3.4 失败模式（v0.3 §F.2 #2）

- **happy-dom mock 失真**：WebGPU/Worker 必须走字面量契约测试（已落实 PR #86）
- **真机测量**：T3 借 dev mode + IAB 验证（本 ADR 不变）

---

## 四、风险

继承 v0.2 §九 11 类风险 + v0.3 §F.2 6 类增量 = **全 17 类仍适用**。本 ADR 缓解：

| 风险 | 本 ADR 影响 | 缓解 |
|------|------------|------|
| 端侧模型回复质量不足（v0.2 §九）| 通用基座非心理垂直 | §6.5 方法论 + few-shot 黄金示例 + 护栏代码 + 实证 golden set 评分 |
| 低端设备运行卡顿 | Qwen3-1.7B 桌面集显 2037MB 可用 | 分级加载（0.6B/1.7B/4B）+ 手动切换 |
| 首次加载等待长（v0.2 §九）| Qwen3-1.7B ~1.2 GB 权重 | CDN 国内可达 + WebLLM cache backend + 进度条 |
| WebGPU 兼容性（v0.2 §九）| WebLLM 检测直走云端 | v0.2 §5.1 路由决策 + fallback_hotline |
| **Apache 2.0 NOTICE 遗漏**（新增）| 首次加载需显示致谢 | §二 §6.5 已列入落地清单 |
| **端云上下文不同步**（v0.2 §九）| 记忆/摘要端云共用 | v0.3 §C.3 + §6.2 同源组装 |
| MindChat A/B 对比缺失（用户决策中）| 本 ADR 跳过自编译 | 综述印证 + T3 可补救（借 dev mode）|

**关键风险控制**：**Apache 2.0 商用须知 5 项义务**（§三 §3.1 列表）已列入 §二 影响面 + §八 落地清单，T3 实测时逐项核对。

---

## 五、验收契约指针

阶段验收按 v0.3 §E OC-01~OC-13 + §G 收口契约 4 张执行。本 ADR 关键检查点：

| 阶段 | 验收 | 阶段一收口判定 |
|------|------|----------------|
| **T2#3（云端基线跑分 + 模型选型落地）** | golden set 13 用例 + Qwen3-1.7B 跑分 ≥ 阈值（待 T2#3 实测定） | `done` / `partial` / `blocked` |
| T3 Demo IAB 验证 | 真引擎接入 + 真机运行 + golden set 端侧 ≥ 阈值 | `done` |
| 阶段三 | 灰度 + 端侧 ≥ 60% + 端云比例埋点（E2E-21 收口后）| `done` |

---

## 六、登记索引

- E2E 轨索引：[e2e-roadmap/decisions.md](../../e2e-roadmap/decisions.md) D-26.2 行（待 STATUS 补账后填）
- ADR 级索引：[decisions.md](../decisions.md) 决策 34 行（待 STATUS 补账后填）
- 决策材料全文：[plans/on-device-model-selection-decision-material-2026-09-24.md](../../plans/on-device-model-selection-decision-material-2026-09-24.md)

---

## 七、关联

- **D-26 端侧化主方案 ADR**：`adr-2026-09-on-device-hybrid-main.md`（proposed）
- **D-26.2** = 本 ADR
- **D-26.1~D-26.5**：决策 1/3/4/5（隐私定位 / 来源告知 / 离线范围 / 摘要存储）—— 尚未拍板
- **状态流转路径**：
  ```
  D-26 proposed (T0 立)
   ↓
  D-26.2 proposed (本 ADR 立项)
   ↓
  §十二 决策 1~5 用户拍板（含本决策 2）
   ↓
  D-26.1 / D-26.3 / D-26.4 / D-26.5 补立 + D-26 + D-26.2 转 accepted
  ```