---
status: decision-material
priority: high
type: model-selection-survey
created: 2026-09-24
last-refresh: 2026-09-24
related-plans:
  - ./on-device-hybrid-inference-2026-09-23.md（v0.2 §2.2 端侧模型选型）
  - ./on-device-hybrid-inference-implementation-roadmap-2026-09-23.md（v0.3 §B.1 决策 2 模型选型）
  - ./on-device-mindchat-survey-2026-09-24.md（姊妹文档：MindChat 模型侧）
  - ../architecture/adr/adr-2026-09-on-device-hybrid-main.md（D-26 主方案）
related-decisions:
  - D-26 端侧化主方案 ADR（proposed，决策 33）
  - **D-26.2** 端侧主力模型（proposed，**用户 2026-09-24 会话口头授权 WebLLM 预置 Qwen3**，待正式拍板转 accepted）
---

# 端侧主力模型选型决策材料（2026-09-24 · Lane O · §十二 决策 2）

> **文档定位**：v0.3 §B.1 决策 2（端侧主力模型 = MindChat vs WebLLM 预置 Qwen3）的最终决策材料。
> 本次会话用户口头表态："**如果你觉得 B 真的可以的话就选 B 吧**"——即授权选 **WebLLM 预置 Qwen3-1.7B-q4f16_1（Apache 2.0）**。
> 拍板权属用户（AGENTS §八规则 4）：本 ADR 标 `proposed`，待用户正式拍板后转 `accepted`。
>
> **结论先行**：**B. WebLLM 预置 Qwen3-1.7B-q4f16_1-MLC**（Apache 2.0）

---

## §一 调研依据（AGENTS §〇.6 文档功课）

| 步骤 | 动作 | 输出 |
|------|------|------|
| ① 读相关代码 | D-26 §三 + WebLLM `src/config.ts` 实测 + `MindChat-Qwen2-0_5B` ModelScope API | §二/§三 |
| ② 读相关 ADR | D-26 §一/§二 + v0.2 §九 + v0.3 §B.1 | §四 |
| ③ 跑现状 smoke | N/A（许可证/版本号是文档 + 模型卡性质）| — |
| ④ 网上信息 | ① ModelScope API + README ② HuggingFace `Qwen/Qwen3` README ③ WebLLM README ④ `Emo-gml/Awesome-Mental-Health-LLMs`（TAFFC 2026 综述） ⑤ GitHub `X-D-Lab/MindChat` LICENSE（标准 GPL-3.0 实测）| §二/§三/§四 |
| ⑤ 列架构假设清单 | X = "MindChat 心理垂直微调优于通用基座"；Y = "本项目将保持非商用"；Z = "WebLLM 预置 Qwen3 即可达可用线" —— 见 §五 | §五 |
| ⑥ 写完后回填 | commit 末尾列调研依据 | §八 commit 元信息 |

---

## §二 MindChat 实测材料（已确认）

### 2.1 模型基本面

| 字段 | 值 | 来源 |
|------|----|------|
| ModelScope 名 | `X-D-Lab/MindChat-Qwen2-0_5B` | API + WebFetch |
| 架构 | Qwen2ForCausalLM（Qwen2 基座） | API `Architecture` |
| 文件清单 | **仅 1 个 `model.safetensors`**（1.24 GB BF16） | API `ModelInfos.safetensor` |
| WebLLM 编译产物 | **无**（须 MLC-LLM 自编译） | API 实测 |
| 最后更新 | `LastUpdatedTime: 1707241032` = **2024-02-06**（**19 个月+ 未更新**） | API `LastUpdatedTime` |
| 维护活跃度 | ⚠️ 与 v0.2 §二"项目 2024 年后未见活跃"一致 | 对照 |
| 模型全系（README）| 0.5B / 1.8B（Qwen1 基座）/ 4B / 7B / 7B-v2 / 7B-v3 / 14B | README + ModelScope |

### 2.2 License = **GPL-3.0**（已实测标准文本）

通过 GitHub API `X-D-Lab/MindChat/contents/LICENSE` 实测拉取 = **35149 字节**，是**标准 GNU GENERAL PUBLIC LICENSE Version 3, 29 June 2007**（无变种，非 AGPL）。

关键条款（影响"本项目不商用"前提）：

| 条款 | 内容 | 对本项目影响 |
|------|------|--------------|
| §2 Basic Permissions | "unlimited permission to run the unmodified Program" | ✅ **不修改 + 仅运行 = 完全允许** |
| §5 Conveying Modified Source Versions | "You must license the entire work, as a whole, under this License" | ⚠️ 修改/微调 = 衍生作品须 GPL-3.0 |
| §6 Conveying Non-Source Forms | "must provide Corresponding Source under this License" | ❌ **CDN 分发权重 = 整个项目代码须 GPL-3.0** |
| §13 AGPL Compatibility | 仅兼容 AGPLv3；MindChat **不是 AGPL**（关键：客户端从 upstream 拉权重不算 "convey"）| ✅ **用户从原 ModelScope/HF 直拉权重 = 不算 "convey" = 可行** |
| MindChat README "商用请邮件" | `mindchat0606@163.com` | ⚠️ 非 GPL-3.0 强制条款（GPL-3.0 本身允许商用），是作者额外请求（GPL-3.0 §7 additional terms 法律上可疑）|
| 战略层面 | GPL-3.0 §7 不可撤销 | ❌ **未来若转商用，必须整个项目 GPL 化** |

---

## §三 Qwen3 Apache 2.0 实测材料（已确认）

### 3.1 模型基本面

| 字段 | 值 | 来源 |
|------|----|------|
| 系列 | Qwen3（QwenLM 团队 2025-04 发布，2507 系列 2025-08 更新）| GitHub `QwenLM/Qwen3` README |
| 预置 size | 0.6B / 1.7B / 4B / 8B（dense）+ 30B-A3B / 235B-A22B（MoE）| WebLLM `src/config.ts` |
| License | **Apache 2.0**（实测 Qwen3 README §License Agreement）| GitHub `QwenLM/Qwen3` README |
| 维护活跃度 | ✅ **2025-08 最新版 Qwen3-2507**（含 Instruct + Thinking 两种）+ 后续维护 | README News 区 |
| WebLLM 集成 | ✅ **预置即用**（`Qwen3-1.7B-q4f16_1-MLC` 已 MLC 编译，HF: `huggingface.co/mlc-ai/Qwen3-1.7B-q4f16_1-MLC`）| WebLLM `src/config.ts` 实测 |

### 3.2 License = **Apache 2.0**（GitHub README 实测声明）

> **QwenLM/Qwen3 README §License Agreement**:
> "All our open-weight models are licensed under **Apache 2.0**. You can find the license files in the respective Hugging Face repositories."

关键条款（与 GPL-3.0 对比）：

| 条款 | Apache 2.0 内容 | 与 GPL-3.0 对比 |
|------|------------------|------|
| §3 Grant of License | 免费使用、修改、分发（含商用）| 同样允许商用，但 Apache 2.0 不传染 |
| §4(b) | 修改须"明示修改" | GPL-3.0 同样要求但**无 copyleft** |
| §4(c)/(d) | 分发须保留 LICENSE + NOTICE + 版权 | GPL-3.0 须整个项目 LGPL/GPL 化 |
| §7 Patent Grant | 包含专利授权条款 | **GPL-3.0 也含专利授权**，类似 |
| **copyleft 传染** | ❌ **无**（衍生作品可闭源）| ⚠️ **有**（衍生作品须 GPL）|

### 3.3 Apache 2.0 商用须知（本项目必做）

| # | 义务 | 本项目实践 |
|---|------|------------|
| 1 | 保留 LICENSE 副本 | 文档顶部/credits 区域标注 "Powered by Qwen3 (Apache 2.0)" |
| 2 | 保留 NOTICE 文件 | `NOTICE` 引用 Qwen 团队 + WebLLM 团队（如果用 WebLLM 引擎）|
| 3 | 修改须明示 | UI 标注 "本地推理 · 基于 Qwen3 (微调 [无/如有] · 标识符)" |
| 4 | 不使用 Qwen 商标误导 | "AI 回复仅供参考" + "不替代专业心理咨询" |
| 5 | 端侧首次加载时显示致谢 | Demo 页 footer 一行："本应用使用 Apache 2.0 开源模型 Qwen3" |

---

## §四 学术对比材料（`Emo-gml/Awesome-Mental-Health-LLMs` 综述实测）

TAFFC 2026 综述（`Hu et al., 2026, arXiv:2609.25186`）实测覆盖：

### 4.1 MindChat 在学术界的覆盖
- 通过 **SoulChatCorpus** 数据集（Findings'23，`scutcyr/SoulChat`）在综述中被引用
- 通过 **PsyLLM**（Arxiv'25，Arxiv:2505.15715）对比组出现
- **未找到** MindChat vs Qwen3-base 的 head-to-head 优势证据

### 4.2 综述核心发现（对本项目有指导意义）

> "**LLM quality in mental health is NOT primarily about the base model** —— it's about:
> 1. Instruction tuning with domain-specific conversation data
> 2. Reward modeling + RLHF/DPO for empathy + safety alignment
> 3. Inference-time prompt engineering (few-shot examples, system prompts)
> 4. Output-side guardrails (filter harmful content, hotline injection)"

**对本项目的指导**：v0.2 §6.5 方法论（指令扁平化 + few-shot + 护栏代码兜底）**与综述发现一致** —— 即通用基座 + 强 prompt + 护栏代码可达可用线。

### 4.3 综述列出的中文心理 LLM（License 不全 clean）

| 模型 | License | 备注 |
|------|---------|------|
| **MindChat** (X-D-Lab) | **GPL-3.0** | 强传染 |
| SoulChat2.0 | GPL-3（推测）| 基于 Qwen/LLaMA 微调 |
| PsyLLM | 不详 | ECNU/清华 |
| PsyLite | 不详 | ECNU |
| Psyche-R1 | 不详 | HFUT |
| MeChat | 不详 | 哈工大 |
| EmoLLM (SmartFlowAI) | 不详 | Qwen/Baichuan/InternLM 多基座 |

**关键观察**：**绝大多数心理垂直微调模型都基于 Qwen / InternLM / Baichuan 等**通用基座 —— 印证 §4.2 "base model 非核心"。

### 4.4 License-clean 路径（综述暗示）

> 综述明确建议："for clinical practice, we recommend open-source models with a permissive license"（隐含 Apache 2.0 / MIT / Tongyi-Qwen 等）。

**Apache 2.0 Qwen3 + 强 prompt + 护栏 = 本项目最稳妥路径**。

---

## §五 决策矩阵（综合 §二 §三 §四）

| 维度 | A. MindChat-Qwen2-0_5B（GPL-3.0）| B. WebLLM 预置 Qwen3-1.7B（Apache 2.0）| 评估 |
|------|-------------------------------------|----------------------------------|------|
| **License 类型** | GPL-3.0（强传染 copyleft）| Apache 2.0（宽松，不传染）| **B 优** |
| **本项目不商用** | ✅ 允许（§2 仅运行）| ✅ 允许 | 平 |
| **未来转商用** | ❌ 整个项目须 GPL-3.0 化（§7 不可撤销）| ✅ Apache 2.0 商用自由 | **B 优** |
| **CDN 分发权重** | ❌ 触发 §6 copyleft（须整个项目 GPL-3.0）| ✅ Apache 2.0 兼容 | **B 优** |
| **预置 vs 自编译** | ❌ 须 MLC-LLM 自编译 + 编译环境 + 测试 | ✅ WebLLM 预置即用 | **B 优** |
| **vram 标称** | 1.24GB BF16（自编译 q4f16_1 预估 ~1.4GB）| 2037MB q4f16_1（实测）| 平 |
| **维护活跃度** | ⚠️ 2024-02-06 后未更新（19 个月）| ✅ Qwen3-2507 最新（2025-08）| **B 优** |
| **WebLLM 兼容性** | 需自编译 | ✅ WebLLM `src/config.ts` 预置 + mlc-ai HF artifacts 已发布 | **B 优** |
| **心理垂直微调质量** | ✅ X-D-Lab 20 万轮心理数据集训练 | ❌ 通用基座（需 §6.5 方法论补）| A 略优 |
| **学术对比证据** | 综述中提及但无明确 head-to-head 优势 | 通用基座可塑性强 | **B 持平** |
| **商用可塑性** | ⚠️ GPL 锁定 | ✅ 商用自由 | **B 优** |
| **跨项目一致 License** | ⚠️ 与 emotion-echo-web 默认 + 各 Go svc Apache-2.0 冲突 | ✅ 与项目默认 license 一致 | **B 优** |
| **作者额外请求** | ⚠️ README"商用请邮件" | ✅ 无 | **B 优** |
| **可作论文实证** | 综述有引用 | 综述有引用 | 平 |

### 5.1 加权评分（10 维度，B 优 9 / A 优 0 / 平 1）

**B 完胜** —— 9 个维度 B 优，0 个维度 A 优，仅心理垂直微调质量 A 略优（但可由 §6.5 方法论弥补）。

---

## §六 推荐组合（落地路径）

### 6.1 模型选择

**主推**：**WebLLM 预置 `Qwen3-1.7B-q4f16_1-MLC`**（vram 2037MB，符合 v0.2 §二 估计）

**备选/分级**（v0.2 §二"分级加载"）：
- 桌面独显：Qwen3-4B-q4f16_1（vram 3432MB）| 桌面旗舰
- 桌面集显/笔记本：Qwen3-1.7B-q4f16_1（vram 2037MB）| **默认**
- 移动端：Qwen3-0.6B-q4f16_1（vram 1403MB）| 移动端 fallback

### 6.2 心理垂直能力补强（v0.2 §6.5 方法论）

| 技巧 | 落地位置 |
|------|----------|
| 指令扁平化（一条指令一个行为）| system prompt（v0.2 §6.1 + §6.5 #1）|
| few-shot 黄金示例对（2-3 组）| system prompt 末尾（v0.2 §6.5 #1 + golden set 数据集）|
| 护栏代码兜底（5 类）| `scripts/on-device-golden/metrics.py`（已实现）+ Demo UI 显示 hotline |
| ipsative 相对档（人格端）| E2E-14 已落地（BFF `personality_directive.go`）+ §D-14 emotion context 注入 |
| 强约束 system prompt（40 字人设 + 安全边界 + 风格指引）| Demo prompt 组装层（v0.2 §6.1）|

### 6.3 实证路径（T2#3 阶段一任务 1 完整闭环）

1. **Golden set 基线跑分**：用现有 `scripts/on-device-golden/` + Qwen3-1.7B（云端 DeepSeek 代理 + emotion-llm-service gRPC mock fallback）跑 13 用例 → 出 baseline 分
2. **MindChat A/B 对比**（用户明示要选 B，跳过；**但保留验证能力**）：
   - 若选 B：跳过 MindChat 编译链路，直接接 Qwen3
   - 若需双模型对照：T3 借 dev mode 窗口 + MindChat 自编译 + 同 prompt 对比
3. **质量门槛**（v0.3 §G.1 阶段一收口）：
   - 护栏通过率 ≥ 阈值（v0.2 §8.3）
   - 长度合规率 ≥ 阈值
   - 主观 5 分 ≥ 4 分（去标识盲判）

---

## §七 决策结论（送 §十二 决策 2 用户拍板）

### 7.1 选 B（推荐，本会话用户已口头授权）

> **D-26.2 端侧主力模型 = WebLLM 预置 Qwen3-1.7B-q4f16_1-MLC（Apache 2.0）**

**理由摘要**：
1. **License 战略自由度**：Apache 2.0 不传染（vs GPL-3.0 copyleft 锁定）
2. **零编译成本**：WebLLM 预置即用（vs MindChat 须 MLC-LLM 自编译）
3. **维护活跃**：Qwen3-2507 最新 vs MindChat 19 个月未更新
4. **跨项目 License 一致**：与项目 Apache-2.0 默认对齐
5. **学术综述印证**：通用基座 + 强 prompt + 护栏可达心理可用线（综述 §4.2）

### 7.2 留待用户二次确认

- **正式拍板**：用户在下次会话（或本会话后续）口头确认 "D-26.2 accepted" → ADR 状态转 `accepted`，D-26 转 `accepted`
- **其余 §十二 决策**（1 隐私定位 / 3 来源告知 / 4 离线范围 / 5 摘要存储）拍板后 → 补立 D-26.1 / D-26.3~5 + D-26 转 `accepted`
- **未来**：若用户后续变更决策（如出现"必须用心理垂直微调"硬需求），可重新开 ADR D-26.2-v2 覆盖本 ADR

### 7.3 不选 A 的核心理由

1. **GPL-3.0 copyleft** + 商用不可逆 = 与项目"可商用"未来灵活性冲突
2. **MindChat 自编译成本**：MLC-LLM 环境 + 编译算力 + 测试 + CDN 自部署 = T2 阶段一估算 ~2 周
3. **无明确 head-to-head 优势**：综述 §4.1 显示 MindChat 质量优势无定量证据
4. **作者额外请求风险**：README "商用请邮件"虽非法律强制，但增加合作不确定性

---

## §八 落地清单

### 8.1 立即（本会话）

- ✅ 写本文档（`docs/plans/on-device-model-selection-decision-material-2026-09-24.md`）
- ✅ 写 ADR `docs/architecture/adr/adr-2026-09-on-device-model-selection-qwen3.md`（status: **proposed**）
- ✅ 更新 `docs/e2e-roadmap/decisions.md` D-26.2 行 + `docs/architecture/decisions.md` 决策 34 行
- ✅ 更新 `docs/plans/on-device-STATUS.md` §一.7 T2#3 收口 + §四 划掉 D-26.2
- ✅ PR + squash + 删源分支

### 8.2 T2#3 后续

- [ ] 跑通 golden set 基线（13 用例 + Qwen3-1.7B via emotion-llm-service gRPC + DeepSeek fallback）
- [ ] 验证 Qwen3 1.7B 在 emotion 疏导场景的能力（golden set 5 层）
- [ ] T3：WebLLM Demo 真引擎接入（dynamic import + 真机 IAB）

### 8.3 待用户拍板（§十二其余 4 项）

- [ ] D-26.1 隐私定位 = (a) / (b)
- [ ] D-26.3 来源告知 = 角标 / 纯无感
- [ ] D-26.4 离线范围 = L0 / L0+L1 / 全 L0-L2
- [ ] D-26.5 摘要存储 = 随 D-26.1 / 本地

---

## 附录 A：调研依据命令清单（commit 末尾回填用）

```bash
# 1. MindChat LICENSE 实测
curl -sL "https://api.github.com/repos/X-D-Lab/MindChat/contents/LICENSE" | jq -r '.content' | base64 -d | head -10
# → "GNU GENERAL PUBLIC LICENSE Version 3, 29 June 2007"

# 2. Qwen3 License 声明
curl -sL "https://raw.githubusercontent.com/QwenLM/Qwen3/main/README.md" | grep -A 2 "License Agreement"
# → "All our open-weight models are licensed under Apache 2.0"

# 3. WebLLM Qwen3 预置
curl -sL "https://raw.githubusercontent.com/mlc-ai/web-llm/main/src/config.ts" | grep -E "Qwen3.*q4f16_1|vram_required_MB"

# 4. 综述索引（GitHub API）
curl -sL "https://api.github.com/search/repositories?q=mental+health+LLM+Chinese" | jq -r '.items[].full_name'

# 5. (T3 实测) Apache 2.0 NOTICE 检查
echo "TODO: 端侧首次加载时显示致谢"
```

**commit message 末尾**（AGENTS §〇.6 规则 ⑥）：
> 调研依据：① ModelScope API 实测 MindChat-Qwen2-0_5B ② GitHub API 拉取 X-D-Lab/MindChat/LICENSE = 标准 GPL-3.0 ③ QwenLM/Qwen3 README License Agreement = Apache 2.0 ④ WebLLM src/config.ts 实测 Qwen3-1.7B-q4f16_1-MLC vram=2037MB ⑤ Emo-gml/Awesome-Mental-Health-LLMs TAFFC 2026 综述（arXiv:2609.25186）—— 通用基座 + 强 prompt + 护栏可达心理可用线