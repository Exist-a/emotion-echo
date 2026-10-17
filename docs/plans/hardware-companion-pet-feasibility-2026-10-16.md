---
status: exploratory
type: feasibility-note
priority: low
owner: TBD
created: 2026-10-16
last-updated: 2026-10-16
source: 2026-10-16 会话 —— 用户提出「复用现有后端 + 硬件客户端做端云协同情绪陪伴桌宠」，多轮讨论后收敛为「设备 = 采集/输出层，全部处理在后端」
depends-on: []
related-plans:
  - on-device-hybrid-inference-2026-09-23.md（Lane O「端侧」= 浏览器 WebGPU/WebLLM，与本条的硬件方向**不同轴**，见 §五）
---

# 硬件桌宠（端云协同）可行性探讨（2026-10-16）

> ⚠️ **本文不是计划，也不是待排期项。** 未立项、未排期、**不承诺落地**。
> 文中的"结论"是当前讨论的**收敛点**，不是已决定的事；任何一条都可能被推翻。
> 若日后决定推进：本文应转为正式计划（改 `status: planned` 并补实施分解），
> 或若涉及架构变更则另立 ADR。

---

## §A 上下文与假设

本项目的形态是「浏览器网页端 + 一堆微服务后端」。本条探讨的是：**把"浏览器"换成一块硬件，作为同一后端的第二个客户端**。

动笔前的功课（AGENTS §〇）：已读实现文件 `emotion-echo-web-bff/internal/handler/ai_stream_handler.go`、`emotion-echo-web-bff/internal/handler/prompt_assembly.go`、`emotion-echo-web-bff/internal/handler/prompt_templates.go`、`emotion-echo-web/app/components/digital-human/DigitalHuman.client.vue`、`proto/emotion_query.proto`、`deploy/db/02-create-tables-in-schemas.sql`；测试文件 `emotion-echo-web-bff/internal/handler/d14_emotion_context_test.go`。已查 `docs/architecture/` 与 `docs/closure-roadmap/`（见 §五）。文档类改动，未跑业务 smoke。

**本文假设（与现状对照）**

| # | 假设 | 与现状对照 |
|---|------|-----------|
| A1 | 设备只做"采集 + 输出"，**全部处理留在后端** | 本讨论已收敛到此，且技术上成立（§二、§三） |
| A2 | 设备是"同一账号的另一个客户端"，与网页端共享会话 | ✅ 已核实：非浏览器客户端可用 Bearer token 登录，会话按 `user_id` 归属 |
| A3 | 情绪识别由后端完成，设备不跑模型 | ✅ 已核实：后端已有 FER（图像）与 SenseVoice（语音） |
| A4 | 因此**不需要新增后端接口** | 🟡 大部分成立；但"设备管理页"需要新增后端"设备"概念（§四 D4） |
| A5 | 硬件端与网页端看到同一套会话数据 | ✅ 成立（同一 `user_id` 下同一批 `conversations`/`messages`） |
| A6 | 成本/量产可行性 | ❌ **未验证**。本轮讨论过价格，按要求不入本文档 |

---

## §一 想法概述

**原始设想**（用户 2026-10-16 原话概括）：复用已有的情绪疏导网页 Go 后端，新增硬件客户端制作"端云协同情绪陪伴桌宠"；硬件全部选用预焊排针成品模块、无需焊接；本地做轻量感知，仅传标签/文本上云，不传原图；后端复用既有 LLM 情绪疏导逻辑与会话存储；硬件端本地做语音播报与表情输出，可选舵机云台；网页端与硬件端共用同一套会话数据。

**讨论后收敛的定位**（关键）：**把硬件当作与本项目网页端"同一层级的一个输入/输出层"，真正的处理全在后端。设备负责采集（感）与输出（声/光/动），其余一切复用现有后端。**

这个定位的价值在于：它不是"新造一套架构"，而是**把项目现有的采集层从浏览器搬到硬件上**。网页端今天已经在做完全相同的事——浏览器抓摄像头画面传给后端做情绪识别（`useFaceEmotion` → `/api/v1/multimodal/analyze`）。硬件端只是换了个采集设备。

---

## §二 现状核对：现有后端能直接提供什么

以下均为代码核实结论，附 `file:line`。

### 2.1 外部（非浏览器）客户端可以接入 ✅

- 登录响应体**直接返回 `accessToken`**（不只是 HttpOnly cookie）⇒ 硬件端可用 `Authorization: Bearer` — `emotion-echo-web-bff/internal/handler/auth_handler.go:41-45`
- 但**必须经 APISIX 网关**（`:19080`）：下游服务不自行验签，只信任网关注入的 `X-User-Id` — `deploy/apisix/seed.sh:633`（catch-all 路由）、`:497-503`（jwt-auth + 注入）
- JWT 有效期 24h ⇒ 设备需自行实现 refresh 或定期重登 — `emotion-echo-web-bff/internal/auth/jwt.go:30-36`

### 2.2 情绪标签**已经是客户端入参** ⇒ 零后端改动 ⭐

这是本次核对最重要的发现：请求体里**已经存在**承载"客户端自带情绪"的字段，且会被直接拼进 system prompt。

- 字段定义 — `emotion-echo-web-bff/internal/handler/ai_stream_handler.go:272-289`：
  `Emotion` / `FaceEmotion` / `FaceConfidence` / `VoiceEmotion` / `VoiceConfidence`
- 注入组装 — 同文件 `:100-110`（`buildEmotionContext`）、`prompt_assembly.go:102-117`（段优先级：face/voice → 文本情绪 → 会话历史）
- 测试证实字段存在且会生效 — `d14_emotion_context_test.go:37-55`

**两个必须知道的细节**：

1. **face/voice 情绪与文本情绪的处理不一致**：文本情绪字段 `neutral` 与空串被当作"无信号"不注入（`defaultTextEmotion` 语义，`prompt_templates.go:136`）；而 **face/voice 只要非空就注入**——测试里 `voiceEmotion: "neutral"` 会生成"平静"（`d14_emotion_context_test.go:52`）。⇒ 设备在**检测到"平静"时应当不发该字段**，否则 prompt 里出现"对方此刻神情看起来平静"这种未测量断言。
2. **prompt 有"不点破来源"护栏**：禁止出现 `摄像头 / 识别 / 分析 / 检测 / 设备 / 传感器 / 面部识别` 等词 — `d14_emotion_context_test.go:65-72`。这意味着系统**本就假设"客户端可能自带情绪标签"**，硬件端不是硬塞进去的；同时也意味着设备上传感知数据这件事不能对用户含糊其辞。

### 2.3 分析接口会把情绪与转写**回传**给调用方 ✅

设备作为"哑终端"能不能拿到情绪？能。

- 多模态分析响应含 `emotion` / `confidence` / `sentiment_score` / `transcript` / `all_scores` — `proto/emotion_query.proto:165-173`
- BFF 语音接口把 `transcript` 与 `emotion` 回给客户端 — `emotion-echo-web-bff/internal/handler/voice_handler.go:165-166`

### 2.4 AI 回复只有 SSE 纯文本；无 WebSocket；无情绪元数据 ⚠️

- 回复仅经 `POST /api/v1/ai/stream`，SSE（`text/event-stream`），逐块 `data: {...}`，以 `data: [DONE]` 结束 — `ai_stream_handler.go:294-340`
- 全仓**无 WebSocket**（grep 零命中）⇒ 原设想里的"HTTP/WebSocket"应改为"HTTP + SSE"
- 回复体**只有文本 delta，没有情绪字段**；也没有"查询融合情绪"的只读端点
  ⇒ 设备的表情只能由**它自己采集到的情绪**（2.3 的回传值）驱动，而不是"云端下发的情绪"

### 2.5 TTS 在服务端；口型数据是近似的

- TTS 端点：`/api/v1/tts/synthesize`（base64 WAV）、`/api/v1/tts/stream`（裸 WAV 流）、`/api/v1/tts/phonemes` — `emotion-echo-web-bff/internal/handler/tts_handler.go:40-45`
- 引擎：本地 XTTS（重，需大内存）或云端 CosyVoice2（需 key）⇒ **设备本地跑不了**，只能调后端
- `phonemes` 是**"总时长 ÷ 字符数"的等分近似**，不是真实声学对齐 — `emotion-echo-models/XTTS/server.py:330-340`
  ⇒ 设备即使换成本地 TTS，也可以用同样算法伪造嘴型时间戳，口型链路不会因换引擎而断

### 2.6 会话共享成立；但没有"设备"概念

- `messages` 表：`conversation_id` + `user_id`，**无 device 列** — `deploy/db/02-create-tables-in-schemas.sql:75-88`
- 全仓无任何"设备"表/字段/接口（grep 核实为空）
  ⇒ "共用会话"成立；但**无法区分哪条消息来自硬件**（要区分须新增列）

### 2.7 3D 数字人的技术栈（决定能否"移植"）

- 实现：`three.js`（v0.184）+ `@pixiv/three-vrm`（v3.5.2）+ `WebGLRenderer`，加载 **26MB 的 VRM 模型** — `emotion-echo-web/app/components/digital-human/DigitalHuman.client.vue:86-88,99`；`public/3d-models/digital-human.vrm`（26M）；`emotion-echo-web/package.json:29-41`
- 它**不是"一个 3D 模型文件"，而是一个运行在浏览器里、由 GPU 渲染的 WebGL 应用**

---

## §三 架构定位（收敛结论）

```
[硬件设备]  采集：语音 / 图像 / 存在感  ──►  输出：语音 / 屏幕表情 / 舵机动作
      │
      │  全部经 APISIX (:19080) + Bearer
      ▼
[现有后端]  分析(FER/SenseVoice) → 会话存储 → LLM 情绪疏导 → TTS
      ▲
      │  同一账号 / 同一批 conversations & messages
[网页端]  （不变）
```

即：**设备 = 输入/输出层；后端 = 全部处理层；网页端与硬件端共享数据。** 接口层面**可以做到零新增**（除 §四 D4 第二层）。

---

## §四 待拍板的分叉（都还没定）

**D1 采集什么？**（影响最大）

- **图像**：设备传图 → 后端 FER 判情绪。设备最简单，但"画面不出设备"的卖点消失；持续传图费流量、费电。
- **语音**：设备收音 → 后端同时做转写与语气情绪（`/api/v1/multimodal/analyze` kind=audio 一个接口两件事）。**天然满足"不传画面"**；只在用户开口时上传，省流量省电；且是桌宠最自然的交互。**（倾向此项）**
- **两者**：摄像头降级为"存在感/注意力检测"（用于云台），不用于判情绪。

> 注意逻辑约束：**"全部处理在后端"与"不传原图"不能同时成立**——只要输入是图像，图就必须上云。

**D2 平台档次？**

| 档 | 特征 | 备注 |
|---|---|---|
| 单片机（ESP32-S3 一类） | 便宜、资源紧、写 C | 本地零推理 ⇒ 门槛比预想低；难点在工具链与调试回路 |
| 廉价 Linux SoC（RV1106 一类） | 能跑 Linux/Python、带 NPU 与摄像头接口 | 可能是性价比最优，且"能跑 Linux"与"便宜"不冲突 |
| 树莓派一类 SBC | 最省心、可原样跑网页 | 成本最高；2026-10 因内存涨价又上调过价 |

**D3 表情形态？**

- 3D 数字人**移不进单片机**（26MB 资产 + WebGL 栈 + 无 GPU），"移植"实际等于"从零写软件 3D 渲染器"。
- 建议**把数字人当"素材源"而非"要移植的组件"**：用现有 VRM 在无头浏览器里把各表情状态烘成**2D 序列帧**，设备只做"按情绪切帧组 + 按文本切嘴型"。角色形象保留，渲染成本归零。
- 3D 数字人的职责（具身存在感）在实体桌宠上**交给舵机更合适**——会真的转头比小屏上的低模 3D 更有效。

**D4 配置页放哪？**（分两层，代价差很多）

- **第一层：配网页（零后端改动）**——用 Web Bluetooth 让网页直连设备写入 WiFi 凭据/音色。代价：仅 Chrome/Edge 支持，需 HTTPS。
- **第二层：设备管理（需新增后端"设备"概念）**——设备列表、在线状态、远程配置。这是**整条链上唯一必须改后端的地方**（现无任何设备表/接口）。前端落点：侧边栏 `emotion-echo-web/app/layouts/nav.vue` + 设置页 `app/pages/chat/setting.vue` 同风格。

---

## §五 与既有规划的关系（不重复）

- **Lane O「端侧化」≠ 本条**。Lane O 指的是**浏览器内 WebGPU/WebLLM 推理**（`docs/plans/on-device-hybrid-inference-2026-09-23.md`），与本条的"硬件设备"是**不同轴**，不构成重复。
- **CL-10（端侧）已 deferred**（`docs/closure-roadmap/roadmap.md`）。本条是否属 CL-10 范畴**需澄清**——若归入，会与该 deferred 决定产生口径冲突。
- 全仓**无任何**硬件/树莓派/嵌入式/传感器接入的既有规划或代码（grep 核实）。
- **无对外设备契约/OpenAPI**：`proto/` 是内部 gRPC 契约；`docs/stages/stage-30-web-bff.md` 曾明确"多端/第三方 BFF 范围限定只服务网页端"。故硬件接入契约为**自建**。

---

## §六 未决与风险

- **采集质量决定上限**：麦克风信噪比与拾音距离直接决定转写与情绪判准——这是"采集"这个词里最被低估的成本。
- **断网即砖**：瘦终端无网无智能，须预先定义降级行为。
- **供电与可靠性**：舵机瞬时电流会导致电压塌陷（设备重启），需独立供电 + 共地；长期运行稳定性待验证。
- **枚举对齐**：项目情绪标签在多层不一致（LLM/融合 9 类、FER 模型 7 类、BFF 中文表 8 键、前端 5 类 + `unk`）。设备侧标签须映射到其中一套；查表未收录会**原样把英文标签塞进 prompt**（`prompt_templates.go:152-171`）。
- **隐私边界**：若上传音频/画面，须有明确采集范围与"正在采集"的可视信号。护栏已禁止 AI 点破来源，但这约束的是 AI 的措辞，不等于对用户尽到告知义务。
- **平台开发代价**（若选单片机）：难点不在"C 语言"（用户已有基础），而在**调试回路**（编译→烧录→串口→猜，无断点、无 shell、无 traceback）与**资源预算**（引脚/内存/供电互相挤，摄像头+屏+I2S 音频+舵机很难全塞在一块上）。

---

## §七 明确不做 / 边界

- 本文**不排期、不立项**；不改任何代码或部署。
- **不做 3D 数字人向硬件的移植**（§四 D3）。
- 若选定"零后端改动"路线，则**不新增"设备"后端概念**；只有选 D4 第二层才需要新表。
- 成本/价格对比本轮已讨论，**按要求不入本文档**。

---

## §八 依据

**代码（本会话核实）**：`emotion-echo-web-bff/internal/handler/ai_stream_handler.go`（:272-289 字段、:100-110 注入、:294-340 SSE）、`prompt_assembly.go`（:102-117 段优先级）、`prompt_templates.go`（:136 defaultTextEmotion、:152-171 中文标签表）、`auth_handler.go`（:41-45 token）、`voice_handler.go`（:165-166 回传）、`tts_handler.go`（:40-45 端点）、`internal/handler/d14_emotion_context_test.go`（:37-55 字段与注入、:65-72 护栏）、`proto/emotion_query.proto`（:165-173 响应）、`deploy/db/02-create-tables-in-schemas.sql`（:75-88 messages）、`deploy/apisix/seed.sh`（:497-503、:633）、`emotion-echo-web/app/components/digital-human/DigitalHuman.client.vue`（:86-88,99）、`emotion-echo-web/package.json`（:29-41）、`emotion-echo-models/XTTS/server.py`（:330-340）。

**既有文档**：`docs/plans/on-device-hybrid-inference-2026-09-23.md`、`docs/_meta/parallel-tracks.md`、`docs/closure-roadmap/roadmap.md`、`docs/stages/stage-30-web-bff.md`。

**外部检索（2026-10-16）**：Anki Vector 主控为 1.2GHz 四核 Qualcomm Snapdragon（⇒ 有摄像头/上云的一类用 SoC 而非裸单片机）；基于 STM32F103 + OLED 的开源桌宠方案（⇒ 会做表情的便宜桌宠走 2D 显示）；瑞芯微 RV1106/RV1109 规格含 Linux 核 + 独立 MCU 核 + NPU + ISP（⇒ "中间档"廉价 Linux SoC 存在）。
