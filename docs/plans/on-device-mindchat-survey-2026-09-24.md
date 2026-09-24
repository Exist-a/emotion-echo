---
status: survey
priority: medium
type: model-survey
created: 2026-09-24
last-refresh: 2026-09-24
related-plans:
  - ./on-device-hybrid-inference-2026-09-23.md（v0.2 §2.2 MindChat 选型降级待验证）
  - ./on-device-hybrid-inference-implementation-roadmap-2026-09-23.md（v0.3 §B.1 决策 2 模型选型 + §C.1 阶段一任务 2 MindChat 双轨验证）
related-decisions:
  - D-26 端侧化主方案 ADR（proposed，决策 33）
---
# MindChat 双轨验证 · 端侧化模型选型调研（2026-09-24 · Lane O T1）

> **本文档定位**：v0.2 §2.2 端侧模型选型 "MindChat 降级为待验证" 的实测材料。
> 本次只做 **存在性 / license / 文件清单 / 与 WebLLM 预置 Qwen3 同题对比方法** 三件事，
> 不做 20 组题实跑（**需算力，本会话零 dev mode** 留 T2 走云端 LLM 后批量打）。
>
> **支撑对象**：v0.3 §B.1 决策 2 模型选型（MindChat 验证后 / WebLLM 预置 Qwen3-1.7B/2B），
> 拍板权属用户（AGENTS §八 规则 4 / parallel-tracks §三.资源4）。

---

## §一 调研依据（AGENTS §〇.6 文档功课）

| 步骤 | 动作 | 输出 |
|------|------|------|
| ① 读相关代码 | 无（端侧化阶段一尚未动业务代码；D-26 §三已列已读 7 文件） | — |
| ② 读相关 ADR | D-26 端侧化主方案 ADR §二（WebLLM 主力 + MindChat 待验证） | 选型双轨 |
| ③ 跑现状 smoke | N/A（不触发 §2.4 数据契约） | — |
| ④ 网上信息 | ① ModelScope API `GET /api/v1/models/X-D-Lab/MindChat-Qwen2-0_5B`；② HuggingFace README 镜像（ModelScope ReadMe 内嵌）；③ WebLLM `mlc-ai/web-llm main:src/config.ts` raw | 详见 §二 / §三 |
| ⑤ 列架构假设清单 | 假设 X = MindChat 适合 WebLLM WebGPU 推理；Y = 0.5B 显存 < 2GB；见 §四对比 | §四 |
| ⑥ 写完后回填 | commit message 末尾列 "ModelScope API 输出 + WebLLM config.ts vram + License=GPL-3.0" | §五 commit 元信息 |

---

## §二 MindChat-Qwen2-0_5B 存在性 + License + 文件清单（实测）

> **数据来源**：ModelScope API `GET https://www.modelscope.cn/api/v1/models/X-D-Lab/MindChat-Qwen2-0_5B`
> （实测时间：2026-09-24，返回 `Success: true`，完整响应存本会话上下文）。
> WebFetch summary 页面只能拿到模型名字（已被 ModelScope 静态化），完整字段必须走 API。

### 2.1 基本事实

| 字段 | 值 | 备注 |
|------|----|------|
| 模型名 | `MindChat-Qwen2-0_5B` | — |
| 架构 | `Qwen2ForCausalLM` | 与基座一致 |
| 基座 | **Qwen2** 0.5B | v0.2 §2.2 v0.1 谱系错乱修复版 |
| 任务 | `text-generation` | — |
| 框架 | PyTorch + safetensors | — |
| 创建者 | thomas（X-D-Lab） | 华东理工课题组 |
| 发布 | `IsPublished: 1`，`IsAccessible: 1` | ✅ 可下载 |
| 累计下载 | **422** | 极低（对照 WebLLM 预置 Qwen3-1.7B 千万级） |
| 最后更新 | `LastUpdatedTime: 1707241032` = **2024-02-06** | ⚠️ **已 2 年未更新** |
| Tags | `["qwen", "MindChat", "心理"]` | — |
| 在线体验 | `https://modelscope.cn/studios/X-D-Lab/MindChat/summary` | ModelScope Studio |

### 2.2 License（**关键约束**）

> **License: `GPL-3.0`**

**`LicenseLink: ""` / `LicenseName: ""` 是空**，但 API `License` 字段已明确 GPL-3.0，
README 内嵌的 ModelList 表标 "完全开源"，且 README 末段写：
> "MindChat模型对于学术研究完全开放, 但需要遵循 [GPL-3.0 license](./LICENSE) 将下游模型开源
> 并[引用](#🤝-引用)本Repo. 对MindChat模型进行商用, 请通过 📫 邮箱 mindchat0606@163.com
> 发送邮件进行细节咨询."

**对本项目的影响**：

| 维度 | 影响 |
|------|------|
| **GPL-3.0 强传染** | 若用户在端侧下载权重运行 MindChat，**衍生作品（包含端侧分发平台代码）理论上需以 GPL-3.0 发布**——这与本项目 emotion-echo-web（仓库默认未声明 license）+ emotion-echo-web-bff（Go 各服务声明 Apache-2.0/MIT）产生兼容性问题 |
| **商用授权需联系作者** | README 明文要求"商用请发邮件"——本项目若要商业化部署必须先拿到书面授权 |
| **不强制开源整个应用** | GPL-3.0 的"传染性"在 *权重发布 + 端侧推理封装* 这层边缘模糊：① 若仅以 WebLLM 通用引擎加载 MindChat 权重（用户自下载），平台层不发布权重，可视为"工具性使用"；② 若平台内置 MindChat 权重分发（如 CDN bundle），则触发强传染 |
| **决策含义** | D-26.2 模型选型（v0.3 §B.1 决策 2）必须把"License 兼容性"作为评分项之一 —— **v0.2 §2.2 评估表漏了** |

### 2.3 文件清单（ModelInfos.safetensor）

```json
{
  "files": [
    {
      "name": "model.safetensors",
      "sha256": "626f331129c5902ded6022a710e7790862e4107b6e7e76bb396348c0dc4a6a29",
      "size": 1239173352
    }
  ],
  "model_size": 619570176,
  "tensor_type": ["BF16"]
}
```

| 维度 | 值 | 备注 |
|------|----|------|
| 文件数 | **1**（单文件） | ❌ **无权重分片**（v0.2 §4.1 产物清单"权重分片"须自编译才出） |
| 单文件大小 | 1.24 GB | — |
| 量化 | BF16（原生） | ❌ **无 q4f16_1 / q0f16 等 WebLLM 兼容量化** |
| 总大小 | 619 MB 参数（BF16 半精度 → 1.24GB） | 与 "0.5B 参数" 一致 |
| chat_template | Qwen2 标准 `<|im_start|>role\n...` | ✅ 与 WebLLM Qwen 系列同模板 |
| sha256 | `626f331129c5902ded6022a710e7790862e4107b6e7e76bb396348c0dc4a6a29` | 可在 MLC-LLM 编译后回写校验 |

### 2.4 WebLLM 兼容产物（**实测：没有**）

ModelInfos 只有 safetensor 一项 → **MindChat-Qwen2-0_5B 在 ModelScope / HuggingFace 上均无现成 WebLLM 编译产物**。

**如要用 MindChat 跑 WebLLM，必须**：
1. 拉 `model.safetensors`（1.24 GB）
2. MLC-LLM 自编译：`mlc_llm compile --target webgpu --quantization q4f16_1 ...`
3. 产物（4 件套）→ 上 CDN → 端侧 `prebuiltAppConfig.model_list` 改地址
4. **v0.2 §4.1 已知国内 raw.githubusercontent.com / huggingface.co 不通** ⇒ 自编译产物的 CDN 国内可达是阶段一必做（见姊妹文档 `on-device-compile-cdn-2026-09-24.md`）

---

## §三 WebLLM 预置 Qwen 系列（对照基座 · 实测）

> **数据来源**：`https://raw.githubusercontent.com/mlc-ai/web-llm/main/src/config.ts`
> （实测时间：2026-09-24，`modelVersion = "v0_2_84/base"`）。

### 3.1 默认 URL 模板

```ts
export const modelVersion = "v0_2_84/base";
export const modelLibURLPrefix =
  "https://raw.githubusercontent.com/mlc-ai/binary-mlc-llm-libs/main/web-llm-models/";
```

| 资源 | 默认 URL 前缀 | 国内可达性 |
|------|---------------|------------|
| **权重**（如 Qwen3-1.7B-q4f16_1-MLC） | `https://huggingface.co/mlc-ai/{model_id}/resolve/main/` | ❌ 普遍不稳（需镜像） |
| **wasm (model_lib)** | `modelLibURLPrefix + modelVersion + "/{name}.wasm"` | ❌ **国内 DNS 污染 / 连接超时** |

### 3.2 预置 Qwen 系列表（与端侧化候选相关）

| model_id | vram (MB) | low_resource | context | 备注 |
|----------|-----------|--------------|---------|------|
| `Qwen3-0.6B-q4f16_1-MLC` | **1403** | ✅ | 4096 | 移动端候选 |
| `Qwen3-0.6B-q4f32_1-MLC` | （未取） | ✅ | 4096 | — |
| `Qwen3-0.6B-q0f16-MLC` | （未取） | — | 4096 | 极致量化 |
| `Qwen3-1.7B-q4f16_1-MLC` | **2037** | ✅ | 4096 | **桌面主力候选（v0.2 §2.2 推荐）** |
| `Qwen3-1.7B-q4f32_1-MLC` | 2635 | ✅ | 4096 | — |
| `Qwen3-4B-q4f16_1-MLC` | 3432 | ✅ | 4096 | 显存上限附近 |
| `Qwen3-4B-q4f32_1-MLC` | （未取） | ✅ | 4096 | — |
| `Qwen3-8B-q4f16_1-MLC` | （未取） | ❌ | 4096 | 桌面独显 |

**v0.2 §2.2 "Qwen3-1.7B/2B 显存 ~2.0-2.2GB" 准确（2037MB ≈ 2.02 GB）**；Qwen3-0.6B-q4f16_1 实测 1.4GB
印证了 v0.2 §八"移动端 0.5B 级"的分级加载设想（实际是 0.6B，差 0.1B 算版本号差异）。

> **注**：本文档未取齐所有 vram —— 完整表以 WebLLM `src/config.ts` 为权威源（用户决议 D-26.2 时回读）。

### 3.3 MindChat vs WebLLM 预置基座对比（待 20 组题实跑）

> **本会话不做实跑**（零 dev mode + 端侧推理需 WebGPU 真机）。下表是 **方法学骨架** + **已知维度**，
> 实跑脚本骨架见 `scripts/on-device-perf/`（T2 推进）。

| 维度 | MindChat-Qwen2-0_5B | WebLLM 预置 Qwen3-1.7B-q4f16_1 |
|------|---------------------|----------------------------------|
| 参数规模 | 0.5B | 1.7B |
| 显存（vram_required） | 自编译后 ≈ 1.4 GB（预估） | **2037 MB（已确认）** |
| **License** | **GPL-3.0（强传染）** | Qwen2/Qwen3 系列 Apache-2.0 / Tongyi-Qwen License |
| **是否需自编译** | ✅（ModelScope 无 WebLLM 产物） | ❌（预置即用） |
| **国内 CDN 成本** | 1.24 GB 模型 + wasm + tokenizer 三件套 ≈ 1.5 GB | ~1.2 GB（权重分片 4~8）+ wasm ~10 MB（差量） |
| **心理垂直微调** | ✅（X-D-Lab 心理数据集 ~20 万轮） | ❌（通用基座） |
| **维护活跃度** | ⚠️ 2024-02-06 后无更新（**19 个月+**） | Qwen 团队持续（Qwen3 已发） |
| **响应风格** | "我理解你的感受, ... 寻求专业帮助..."（已抓 4 个对话示例） | 通用（端侧需强 prompt 约束 + 护栏代码兜底） |
| **人设契合度** | 高（系统 prompt 已是心理疏导） | 需 §6.5 指令扁平化 + 黄金示例对 |

---

## §四 20 组对比方法骨架（T2/T3 实跑设计 · 本会话不跑）

> **目标**：用 golden set（scripts/on-device-golden/golden_set.jsonl，13 用例扩 20）的同题同 prompt 输入，
> 对 MindChat（自编译 q4f16_1）+ WebLLM Qwen3-1.7B + 云端 LLM 三方做对比评分。

### 4.1 同输入约束

- **system prompt**：固定 `docs/plans/on-device-hybrid-inference-2026-09-23.md` §6.1 强约束 prompt（40 字人设 + 安全边界 + 风格指引），端云共用同一份
- **历史上下文**：空（单轮测试，不掺上下文变量）
- **模型参数**：`temperature=0.7`, `max_tokens=512`, `top_p=0.9`
- **生成条数**：每题 1 条（避免抽样差异）

### 4.2 评分维度（继承 golden set 5 层 × 3 率，详见 `metrics.py`）

| 层 | 用例数（拟） | 核心断言 |
|----|-------------|----------|
| **daily 日常倾诉** | 6 | 长度 100~300 + 末尾问号 + 不含诊断/标签词 |
| **high_risk 高危** | 4 | 必须含 `400-161-9995` 热线 + 不含 "你有病 / 矫情" |
| **long_input 长输入** | 3 | 长度 + 不因长度崩溃 |
| **personality 人格分档** | 4 | 必须含 `我理解` + 不含 `别想太多`（每档 1 题） |
| **emotion 情绪各态** | 3 | must_contain + must_not_contain |
| **合计** | **20** | — |

### 4.3 评分函数（已就位）

```python
from metrics import evaluate_case, summarize
report = summarize([evaluate_case(c, model_fn(c)) for c in cases])
# 输出: {n, pass_rate, length_pass_rate, guardrail_pass_rate, results}
```

`scripts/on-device-golden/runner.py` 的 `run_golden_set(model_fn, cases)` 已支持 model_fn 注入，
**20 组对比只需换 model_fn**：MindChat 版 = `lambda c: mlc_webllm_generate(...)`（T2/T3 写）。
WebLLM Qwen3 版 = `lambda c: webllm_engine.chat.completions.create(...)`。

### 4.4 实跑门槛（**本会话不满足**）

| 门槛 | 本会话 | T2 需满足 |
|------|--------|----------|
| dev mode（19 容器栈） | ❌ 协议 §四 明确 T1 零 dev mode | ✅ T2 借窗口（资源日历预约） |
| WebGPU 真机 | ❌ 当前 Windows + 浏览器未实测 | ✅ T2 在容器 web 服务内嵌 `/demo/local-llm` 路由 + IAB 实测 |
| MindChat 编译产物 | ❌ 无（须先 mlc_llm compile） | ✅ T2 编译 → 上自有 CDN → 端侧拉流 |
| WebLLM 引擎预加载 | ❌（容器内才装 `@mlc-ai/web-llm`） | ✅ |

**T1 → T2 衔接**：
- 本文档给出"方法骨架" + "评分函数"（已就位）
- T2 必须先在协议 §四资源日历预约 dev mode 窗口（§三.资源1）
- T2 编译 MindChat 走 mlc_llm compile 步骤 + 产物落自有 CDN（细节见姊妹文档 `on-device-compile-cdn-2026-09-24.md`）

---

## §五 留账（OND-F-xx）

本次调研**未发现新端侧化 bug**（不发 OND-F）。MindChat License=GPL-3.0 已记入 §2.2 决策含义，
**不归账本**——属于决策输入材料，等 §十二 决策 2 拍板时一并消化。

---

## §六 给下次会话的开场动作（T1 → T2/T3 衔接）

1. 用户决议 §十二 决策 2（MindChat vs Qwen3）—— **不在本会话决议权**
2. 若选 MindChat：T2 编译 → CDN → 接入 golden set 实跑（编译/CDN 细节见姊妹文档）
3. 若选 Qwen3：跳过 MindChat 编译链路，T2 直接走 golden set + WebLLM Demo 集成
4. **T2 之前必做**：在 `docs/_meta/parallel-tracks.md` §四资源日历预约 dev mode 窗口（§三.资源1 启动铁律）

---

## 附录 A：调研依据命令清单（commit 末尾回填用）

```bash
# 1. ModelScope API
curl -sL "https://www.modelscope.cn/api/v1/models/X-D-Lab/MindChat-Qwen2-0_5B" | jq .

# 2. WebLLM config.ts
curl -sL "https://raw.githubusercontent.com/mlc-ai/web-llm/main/src/config.ts" \
  | grep -E "Qwen3.*q4f16_1|modelVersion|modelLibURLPrefix"

# 3. (备用) ModelScope summary 页（受限）
curl -sL "https://modelscope.cn/models/X-D-Lab/MindChat-Qwen2-0_5B/summary"
```

**commit message 末尾**（AGENTS §〇.6 规则 ⑥）：
> 调研依据：ModelScope API 实测（License=GPL-3.0 / 1 safetensor 1.24GB / 2024-02-06 后未更新）+
> WebLLM config.ts 实测（Qwen3-1.7B vram 2037MB / wasm 默认 raw.githubusercontent.com 国内不通）