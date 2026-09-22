---
status: proposed
priority: high
created: 2026-09-22
type: e2e-testing-plan
depends-on: [e2e-16-multimodal, e2e-17-digital-human-tts]
related-findings: [E2E-F-99, E2E-F-110, E2E-F-113, E2E-F-114]
---

# E2E-16 多模态（音频/面部）实测边界计划

> **背景**：用户 2026-09-22 反馈"上个阶段好像音频和面部你没法测试"。本文件**显式列出** E2E-16 阶段（含 E2E-17 数字人/TTS）中**我能机械验证的** / **必须用户手工验证** / **永远跑不了** 三类边界，并给出"按约定进行测试"的执行协议。
> **触发**：E2E-F-113 audioUrl 修复（PR #59）+ 用户请求"先进行之前问题的修复，随后再进行计划制定"。

---

## 0. 现实条件（基线）

| 工具/角色 | 能力 | 限制 |
|----------|------|------|
| **我（自动化 ZCode）** | 读/写代码、跑 Go/JS/TS 测试、运行容器、curl/sql/psql/mc 查数据 | 没有真实物理麦克风/摄像头；浏览器测试只能容器内运行 |
| **IAB（内置浏览器，Docker Desktop）** | 真实 Chromium 内核；能 `getUserMedia`/`MediaRecorder`（默认 fake device） | 与用户真实浏览器仍有差距（E2E-F-38：cookie jar 独立、`document.cookie` API 受限、`fetch` 在 about:blank 不工作） |
| **Playwright headless** | 双 project（chromium + mobile/Pixel 5）；fake media via `--use-fake-device-for-media-stream` | fake 音频是 1kHz 正弦=常量灰度，无语义内容；fake 摄像头是彩色滚动条纹=纯视觉噪声 |
| **真实浏览器（用户机器上的 Chrome/Safari/Firefox）** | 真麦克风/真摄像头；能真播放真音频；能真看到 AI 体验 | **不可被我自动化控制**，只能由用户手工跑 |
| **现有 AI 模型** | LLM 可判文本情感；FER 可判真脸；SenseVoice 可转写中文 ASR | 都没有"听起来自然吗"/"看起来准确吗"的"真值"——纯机械指标可能与用户感受错位 |

---

## 1. 我能机械验证的（18 个测试点，跨 3 条链路）

### 1.1 音频链路（端到端，#1~#8）

| # | 测试点 | 判定 | 验证方式 |
|---|--------|------|---------|
| 1 | 录音后能否 POST 到 `/voice/upload` → 200 + `data.transcript` 非空（**值断言，非存在性**） | [A] | curl 真录音文件 + 解析 JSON + 字符串长度 > 0 |
| 2 | `data.audioUrl` 走 APISIX → BFF → MinIO 200（**两视角**：宿主 curl + web 容器内部 curl） | [A] | 两条 curl；避开 E2E-F-113 同型盲区 |
| 3 | GET audioUrl → 200 + `Content-Type: audio/webm` + `Content-Length` 真实字节 + body 与 MinIO 流对齐 | [A] | 字节对比 + header 读取 |
| 4 | MinIO 对象真实存在（`mc ls avatars/voice/` 看到对应 objectKey） | [A] | mc CLI 命令 |
| 5 | 音频消息上屏 + DB `messages.content_type=audio` + `file_name` 非空 | [A] | Playwright 读 DOM + psql 查行 |
| 6 | 真模型 vs 降级区分：`response.model` 前缀 = `"sensevoice:"`（避免 E2E-F-103 同型弱断言） | [A] | 字符串前缀断言 |
| 7 | 边界：413（>10MB）/415（拒之 mime）/401（缺 X-User-Id） | [A] | 4 个 curl 状态码 |
| 8 | 失败用例：ai-svc 不可达时 503（不是 500）+ 前端不静默 | [A] | docker stop ai-svc + 录音 + 断言 toast 可见 |

### 1.2 表情链路（端到端，#9~#15）

| # | 测试点 | 判定 | 验证方式 |
|---|--------|------|---------|
| 9 | 摄像头开启后 ≥3 次捕获（6 秒）全部 200，`response.model` 前缀 = `"fer:"` | [A] | Playwright fake video + 网络日志 |
| 10 | **周期捕捉不落库**（D-12 决议）：开摄像头 10s 前后 `face_emotion_results` 行数不变 | [A] | 两次 psql count() 断言 |
| 11 | **发送时落一条且绑定该消息**：发送文字消息后 `face_emotion_results` 新增 1 行 + `message_id` 绑定 + `primary_emotion` = 当时捕捉值 | [A] | psql 查新增行 |
| 12 | **3 秒窗口语义**：捕捉后 3s 内发送 ⇒ 落库；隔 >3s 再发送 ⇒ 不落库 | [A] | 两个用例各查一次 DB |
| 13 | **融合真的有 face 贡献**：`fused_emotions.modality_contrib::jsonb` 含 `face > 0`（D-14 前半） | [A] | psql 查 JSONB 字段 |
| 14 | **注入 prompt 且值正确**：抓 BFF → LLM 报文，system prompt 含情绪上下文段 + 值 = `faceEmotion` 字段值 | [A] | LLM_BASE_URL 临时指向本地捕获服务器（E2E-14 §8 用过的） |
| 16 | 无脸帧 → 返 `neutral` fallback 而非 500 | [A] | 上传纯色图 + 断言 200 + neutral |

### 1.3 文件上传（#17~#18）

| # | 测试点 | 判定 | 验证方式 |
|---|--------|------|---------|
| 17 | 上传附件 → 200 + `data.url` 非空 + MinIO 对象存在 | [A] | curl + `mc ls` |
| 18 | 附件消息真实送达：`messages.content_type=file/image` + `file_name` 非空 + `content = data.url` | [A] | psql 查行 + 与 API 值比对 |
| 19 | 附件被 AI 感知：抓 BFF → LLM 报文含 `files` 字段（`collectFileAttachments`） | [A] | 抓包 |

> **判定标记**：所有测试点都是 `[A]`（自动可判）。无 `[V]`（视觉）/ `[M]`（需裁定）—— 这些是必须用户跑的部分，§2 列。

---

## 2. 必须用户跑的（我能写用例但不能跑）

### 2.1 真实音频质量（5 项）

| 测试点 | 描述 | 你的跑法 |
|--------|------|---------|
| U-A1 | 录音清晰度 | 录 10 秒 / 安静环境 / 听：是否听得清自己说的 |
| U-A2 | 转写准确度（中文） | 念一段有方言/俚语/数字的电话号码，看 TSAP 是否一致 |
| U-A3 | 转写准确度（英文） | 念一段带 accent 的英文 |
| U-A4 | 录音回放 | 发一条语音消息到对话，点气泡的播放按钮：听得到、流畅、无爆音 |
| U-A5 | 静音 / 噪声 鲁棒性 | 录 5 秒纯静音 + 5 秒背景噪声，看 TSAP 是否空 / 噪声词 |

### 2.2 真实面部表情（6 项）

| 测试点 | 描述 | 你的跑法 |
|--------|------|---------|
| U-F1 | 摄像头预览清晰度 | 开摄像头，看预览画面是否清晰、有无延迟 |
| U-F2 | 微表情识别 | 眨眼/挑眉/微笑，看 `primary_emotion` 是否跳 |
| U-F3 | 戴口罩 / 戴眼镜 / 侧脸 | 看模型推断 |
| U-F4 | 多张人脸 | 多人合照，看是否抛错（FER 应该单脸） |
| U-F5 | 暗光 / 强逆光 | 关灯 / 背窗，看模型是否降级（应报 "no face" 而非报错） |
| U-F6 | 心理一致性 | 故意想"我很生气"时看模型判定；故意"我很平静"时看模型判定 |

### 2.3 跨浏览器 / 跨设备（4 项）

| 测试点 | 描述 | 你的跑法 |
|--------|------|---------|
| U-X1 | Firefox | 录 + 回放 + 截图，看是否一致 |
| U-X2 | Safari (macOS) | 同上 |
| U-X3 | iOS Safari | 真实移动设备：开摄像头、录音、看气泡布局 |
| U-X4 | Windows MediaRecorder | Windows Chrome vs Edge vs Firefox 录音格式差异 |

### 2.4 真实使用体感（3 项）

| 测试点 | 描述 | 你的跑法 |
|--------|------|---------|
| U-S1 | 录音 5 分钟不卡死 | 长时使用 |
| U-S2 | 摄像头开 30 分钟不爆资源 | 长时使用 |
| U-S3 | 表情 / 语音 是否回环到聊天上下文 | AI 回复风格是否反映你的情绪 |

---

## 3. 永远跑不了的（机器判定无标准答案）

### 3.1 听觉感受
- "听起来自然吗" / "音调是否反映情绪" / "与数字人口型是否同步" — **依赖人耳**
- 与 XTTS 数字人的协同效果（E2E-17 范围）

### 3.2 视觉感受
- "表情识别准不准" — FER 是机器学习模型，没有 ground truth 对人
- "用户是否接受被记录" — 隐私伦理判断，**产品决策**

### 3.3 心理 / 体验
- "用摄像头会不会让用户更紧张" — 心理学评估
- "AI 是否真的理解了用户情绪" — LLM 自我评估无意义

---

## 4. 执行协议（按项目测试约定）

### 4.1 我会做的（按 RUNBOOK TDD 节奏）

1. **先把 §1 的 18 个测试点写成代码**
     - Go 契约测试（fake storage / fake AIClient）
     - Playwright `e2e/multimodal.spec.ts`（fake media + 双 project）
     - SQL/JSON 验证脚本（`scripts/verify_multimodal_db.py`）
     - LLM 抓包测试（`scripts/verify_emotion_prompt.py`，复用 E2E-14 的 capture 手法）

3. **CI 持续跑**：每次 PR 触发上述 18 测试点

4. **阶段收口时**：全 18 点绿 + 你跑 §2 的 18 项真实体感，作为"`[V]` 主观验收"写进 §6 报告

### 4.2 你要做的（按月节奏）

| 节奏 | 内容 |
|------|------|
| 每个 sprint 收口 | 跑 §2 的 18 项真实体感，写 1 段 100 字反馈进 PR 描述 / report §6 |
| 每月一次回归 | 1 小时：2 主跑 + 全部 18 项 U-xxx（每项 ≤ 3 分钟） |
| 每次发布前 | 1 次完整 §2 + §3 抽检 |

### 4.3 触发标准（什么算"完成"）

- **测试维度完成**：§1 的 18 点全 PASS + §2 的 18 项至少跑过 1 次有书面记录
- **真实使用维度完成**：3 名真实用户（开发组外）在 1 周内未报重大问题（"听到/看到不对"）

---

## 5. 与留账的衔接

| 留账 | 本计划处理 |
|------|----------|
| E2E-F-113 | audioUrl 反代修复 PR #59；§1 的 #2/3 覆盖 |
| E2E-F-114 | JWT secret 一致性；**不阻塞本计划实施**，但 §1 端到端实测要等 F-114 修后才能跑通 |
| E2E-F-99 / F-101 / F-91 / F-97 | AP-01 同型教训：§1 所有测试点都用值断言，**不**用存在性断言 |
| E2E-F-38 | IAB cookie 限制：§1 端到端用 cookie 注入绕过 |

---

## 6. 我立刻能开始的事（无需你授权）

按优先级（先 E2E-16 当前阶段必做）：

1. **§1 的 Go 契约测试**（fake storage / fake AIClient） — 我能写，能跑
2. **§1 的 Playwright multimodal.spec.ts**（fake media）— 我能写，能跑（但 fake 内容的"audio 文件"是 1kHz 正弦，无语义，意义仅在验证管道）
3. **§1 的 SQL/JSON 验证脚本** — 我能写，能跑
4. **§1 的 LLM 抓包测试** — 我能写，能跑（用 capture server）

但**真实音频 / 真实面部 / 跨浏览器 / 真实使用** = 必须你跑，这是计划里写明的分工。**我不接受审计意见虚构"测过真实 X"**（这是项目反模式 PR #59 的同型失真）。

---

## 7. 推荐路径

按工程投入产出（先 E2E-16 阶段本身，再 E2E-17，再月度回归）：

| 阶段 | 我做的 | 你做的 |
|------|--------|--------|
| E2E-16 阶段 1.1（本周） | §1 的 18 测试点 PASS + fake media 实测 | §2 的 U-A4 / U-F1 各 1 次（5 分钟） |
| E2E-16 阶段 1.2（次周） | §1 的 LLM 抓包测试 + 跨 fake media 鲁棒性 | §2 的 U-A1~A5 / U-F1~F6 各 1 次（30 分钟） |
| E2E-16 收口（第 3 周） | 全 18 点 + 报告 + 收口 | §2 全部 18 项（2 小时） |
| E2E-17（TTS）开工 | §1 复用于 TTS 场景（fake 音频播放） | §2 U-A4 复述 + U-T1（TTS 听感）|

---

## 8. 反对意见

你可能想："买 cloud 评测服务（如 OpenAI Whisper / AWS Rekognition）作为对照基线"。

**我的判断**：投入产出不匹配。原因：

- 真值 / 假值在"自然度 / 准确度"上**没有 ground truth** —— Whisper 转写准确率也只能"对齐人类标注文本"，对单用户而言不像测
- 项目当前阶段是 dev mode，**还没上线真实用户**，先做主观感受测试足够
- 上线后才有"对照评测"的数据基线

但如果你坚持要 cloud baseline，我建议延后到 E2E-19（数据库层验证之后），那时有真实数据积累。

---

## 9. 引用

- AGENTS.md §〇 第一性原则（TDD）
- AGENTS.md §二 收口契约（17 项必跑）
- `docs/e2e-roadmap/RUNBOOK.md` §4.1 证据有效性
- `docs/e2e-roadmap/anti-patterns.md` AP-01（断言全绿 ≠ 功能可用）
- `docs/e2e-roadmap/stages/e2e-16-multimodal/plan.md` §5 测试点清单
- E2E-F-99 / F-101 / F-91 / F-97 / F-113：弱断言放过真缺陷的同型教训
- `emotion-echo-web/e2e/multimodal.spec.ts`（待新建）
- `scripts/verify_emotion_prompt.py`（E2E-14 复用手法）