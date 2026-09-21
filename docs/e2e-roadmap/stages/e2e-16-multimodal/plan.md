---
stage: e2e-16
title: 多模态（语音 / 表情 / 文件上传）
type: transformation
status: pending
created: 2026-09-22
depends-on: [e2e-10]
blocks: [e2e-17]
gate: []
related-findings: [E2E-F-86, E2E-F-99, E2E-F-31]
---

# E2E-16 多模态（语音 / 表情 / 文件上传）— 详档

> **类型**：transformation —— 三条链路目前**都有 UI 入口、都有后端端点，但都在同一处静默断链**（响应信封），必须先修才能"端到端可验证"。
> **依据**：2026-09-22 本阶段开工前的代码探查（§4，全部条目附 `文件:行号` 证据）+ 账本 E2E-F-86 的同型先例。
> **方法论**：[RUNBOOK.md](../RUNBOOK.md)（状态机 / §4.1 证据有效性 / §7 收口契约 11 项 / §13 审计）+ [anti-patterns.md](../anti-patterns.md)（14 类反例）。

---

## 1. 阶段目标

让三条链路从「**界面有按钮、后端有端点、但用户动作没有任何可观察结果**」变成「**用户动作 → 服务端处理 → 结果回到界面且可机械验证**」。

探查结论（§4）：三条链路**卡的其实是同一个缺陷**——BFF 手写 `c.JSON(200, gin.H{...})` 缺 `data` 包装，而前端 `useApi` 统一 `return data.data`（`emotion-echo-web/app/composables/useApi.ts:346,353`）⇒ 前端拿到 `undefined`。`voice` 路径**无 else 分支**（静默无反馈），`upload` 路径**带着 `undefined` 继续发消息**（被 chat-svc 以 `content is required` 拒绝，用户看到"发送失败"但文件其实已写进 MinIO）。这与已修复的 **E2E-F-86**（头像上传"成功却提示失败"）**完全同型**，且 `handler/resp.go` 的包注释已明文规定"BFF 所有 handler 用 OK/Fail 包装"——两个 handler 是例外。

---

## 2. 范围与边界

### 做

**A. 语音输入（ASR）**
1. 修 `/api/v1/voice/upload` 响应信封：`voice_handler.go:85-93` 的裸 `gin.H` → `OK(c, gin.H{...})`
2. 修前端静默失败：`useVoiceRecorder.ts:148` 的 `if (result)` 补 else 分支（对齐 `[id].vue:419-421` 已有的报错写法）
3. 打通转写文本：`analyzer.EmotionResult` 增 `Text` 字段（`analyzer.go:20-25`）→ `analyzer/multimodal.go:103-119` 回填 → `logic/multimodalanalyzelogic.go:74-79` 优先用 ASR 文本
4. 语音消息上屏并落库：`[id].vue:329` 的 `skipUserMessage: true` 改为真实入库（否则刷新即丢，且无法验证"消息 = API 值"）
5. 语音情绪不再被丢弃：BFF `/ai/stream` 请求结构体补 `voiceEmotion` 字段（`ai_stream_handler.go:215-225`，现在 JSON 解析静默忽略）

**B. 表情识别（FER）**
6. 摄像头抓帧链路端到端复验（`useFaceEmotion.ts` → BFF `/multimodal/analyze` → ai-svc → FER）
7. 消除"静默降级伪装成功"：断言响应 `model` 前缀（`fer:` / `sensevoice:`）而非只看 HTTP 200
8. 表情**保持"仅实时展示、不落库"**（§8 议题 B 采用口径）：补一条隐私契约钉（表行数不变）+ 账本说明下游不可达

**C. 文件上传**
9. 修 `/api/v1/uploads/:kind` 响应信封：`upload_handler.go:156-163` 裸 `gin.H` → `OK(c, ...)`
10. 附件消息真实送达（消息落库 `file_name` 非空 + content = MinIO URL）
11. 附件被 AI 感知复验（BFF `collectFileAttachments` / `fileSourceURL`）

**D. 入口一致性**
12. `/chat/conversation/new` 页多模态入口补齐（§8 议题 C 采用口径）：语音改真录音（`new.vue:184-191`）、附件改真上传（`new.vue:180-182`），接线方式照搬 `[id].vue` 已验证路径

**E. 测试基建**
13. 新增 Playwright `e2e/multimodal.spec.ts`（fake media device，双 project）
14. 新增 `useVoiceRecorder` 行为测试（当前**零测试**）
15. 修正 `voice_handler_test.go` / `upload_handler_test.go` 中把**错误结构固化为契约**的断言（§4.4）

**F. 账本**
16. 把本次预探查的 3 条新发现登记为 `E2E-F-103~105`（§4.5），**先登记再修**

### 不做（边界）

| 不做项 | 理由 |
|--------|------|
| 数字人 + TTS 口型同步 | 归 E2E-17（D-03 已决议） |
| 音频持久化到 MinIO / 语音回放 | §8 议题 A 采用"不做"口径 ⇒ 语音链路止于"转写文本上屏"；`audioUrl` 留空并记账，气泡显示占位（**明确不做**，不是 N/A 掩盖） |
| 表情情绪落库 + modality 报表 | §8 议题 B 采用"不落库"口径；下游 view/analytics 侧本就无任何消费方（`grep -i modality emotion-echo-web-bff/` 零命中）⇒ 明确记为不可达 |
| i18n | D-04 候选未决 |
| 多实例下 in-memory 限流/锁定 | 归 E2E-20 |
| 拖拽上传、多文件并发上传 | 现状无实现，非本阶段目标 |

---

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| E2E-10 done（聊天核心链路，多模态入口位于聊天页） | ✅ 2026-09-21 取证补拍轮恢复 done |
| `e2e_stage_audit.py --all` 0 FAIL | ✅ 2026-09-22 实测（30 阶段 0 FAIL） |
| `deploy/.env.local` 存在（LLM key 唯一存放点） | ✅ 2026-09-22 实测存在（1689 B）——**严禁删除/覆盖**（AGENTS.md §四红线） |
| ai profile 镜像已在本地 | ✅ `emotion-echo/fer-tflite:v0.1.0`（538 MB）+ `emotion-echo/sensevoice:v0.1.0`（4.07 GB）均在本地 |
| 冷 build 缓存下的重建能力 | ⚠️ **Build Cache = 0 B**（2026-09-22 实测），基础镜像本地已有 → 重建无需联网拉 base，但**首次构建耗时显著变长**（纪律见 §3.2） |

**本表全绿方可把状态改为 `in-progress`**（RUNBOOK §1 开工前置检查）。三个待裁定议题（§8）**不属于开工前置**——它们只影响测试点 6/11/17 的判定口径，可在执行到该点前裁定。

### 3.1 环境启动（固定动作）

```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local up -d
```

**多模态额外要求**：本阶段必须显式启用 `ai` profile，否则 FER / SenseVoice 容器不存在：

```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local --profile ai up -d \
  emotion-echo-fer emotion-echo-sensevoice
```

> 只点名这两个服务 —— `emotion-echo-xtts`（`ai4all/coqui`，11.3 GB）同属 `ai` profile 但本阶段**不需要**（TTS 归 E2E-17），不必为它付启动成本。

### 3.2 🔴 冷 build 缓存下的重建纪律（2026-09-22 用户提醒 + E2E-F-99 教训）

**现状实测**：`docker system df` → `Build Cache 0 B`；`docker ps` 为空（4 个 `decisioncourt-*` 容器是无关项目的停止态）。本地镜像清单：

| 镜像 | 构建时间 | 备注 |
|------|---------|------|
| `emotion-echo/web:v0.1.4` | 18 小时前 | |
| `emotion-echo/web-bff:v0.1.23` | 18 小时前 | |
| `emotion-echo/analytics-svc:v0.1.8` | 18 小时前 | 含 E2E-F-10 的 Save 路径 |
| `emotion-echo/assessment-svc:v0.1.4` | 23 小时前 | |
| `emotion-echo/user-svc:v0.1.4` | 2 天前 | |
| `emotion-echo/chat-svc:v0.1.14` | 3 天前 | |
| `emotion-echo/ai-svc:v0.1.8` | 5 天前 | **本阶段大概率要重建**（改 analyzer/logic） |
| `emotion-echo/llm-service:v0.1.2` | 5 天前 | |
| `golang:1.26-alpine` / `alpine:3.19` / `node:20-alpine` / `python:3.12-slim` | 4 天 ~ 5 月前 | base 已在本地，冷构建不必联网拉 |

**重建步骤（照做，别自创）**：

```bash
# 1. 构建（脚本自带 base 预拉重试 + 逐服务重试，冷缓存下用它而不是裸 compose build）
bash scripts/build_dev_images.sh ai-svc web-bff        # 只建本阶段要改的
bash scripts/build_dev_images.sh web                   # 改了前端且需要容器内产物时才建

# 2. 起容器（必须带 --env-file .env.local）
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
  -f compose.dev.yml --env-file .env.local up -d
```

**🔴 硬规则：验收任何修复前，先核对镜像时间 > 修复 commit 时间。** E2E-F-99 就是因为 web 镜像 07:17:15 早于修复 commit 09:35:38，导致 PR #48 宣称的「24/24 PASS」**实际验的是修复前的旧代码**，而 spec 的弱断言把它放过了（同型已第三次复发）。

```bash
docker inspect emotion-echo-web-bff --format '{{.Image}}' | xargs docker inspect --format '{{.Created}}'
git log -1 --format='%cI' <修复的 commit>
# 断言：镜像 Created 晚于 commit 时间；否则 rebuild 后重测
```

> 前端另有更省的路径：本阶段前端改动（`useVoiceRecorder.ts` / `useApi` 消费侧）可用**本地 `pnpm dev`** 承接（RUNBOOK §2.1b），不必每次重建 web 镜像 —— 但**必须先 `docker stop emotion-echo-web` 释放 3000 端口**，且要意识到"宿主机 dev server 与容器产物是两套东西"，验收结论里写明用的是哪套（E2E-F-99 的过时认知就出在这里）。

### 3.3 「静默降级」陷阱（本阶段最容易被误判为 PASS 的地方）

`deploy/docker-compose.apps.yml:390-392` **无条件**注入 `FER_BASE_URL` / `SENSEVOICE_BASE_URL` / `XTTS_BASE_URL`（默认指向容器 DNS 名），而 `emotion-echo-ai-svc/main.go:131-136` 只要 env 非空就构造客户端。后果：**即使 `ai` profile 没起**，client 也非 nil，只是每次调用连接失败 → `analyzer/multimodal.go:78-81,99-102` **静默回落到关键词分析器**，接口照样返 200。

⇒ 本阶段所有多模态测试点**必须断言 `model` 字段前缀**（真模型为 `fer:<src>` / `sensevoice:<src>`，`analyzer/multimodal.go:90,116`），只看 HTTP 200 会把"降级"判成 PASS。

---

## 4. 代码探查摘要（2026-09-22 实测，全部附证据）

> 本节为撰写本文档前按 AGENTS.md「文档撰写前必须做的功课」实读所得。**已读关键实现文件**：`emotion-echo-web-bff/internal/handler/{voice_handler.go,upload_handler.go,resp.go,avatar_handler.go}`、`emotion-echo-web/app/composables/{useVoiceRecorder.ts,useFileUpload.ts,useFaceEmotion.ts,useApi.ts}`、`emotion-echo-ai-svc/internal/{analyzer/analyzer.go,analyzer/multimodal.go,logic/multimodalanalyzelogic.go,config/config.go}`、`emotion-echo-ai-svc/main.go`、`deploy/docker-compose.apps.yml`、`emotion-echo-web/app/pages/chat/conversation/{[id].vue,new.vue}`；**已读测试文件**：`voice_handler_test.go`、`upload_handler_test.go`、`useFaceEmotion.test.ts`、`analyzer/multimodal_test.go`；**已查**：账本（归属 E2E-16 = 0 条）、roadmap 依赖声明、RUNBOOK §2/§3、decisions.md 索引、审计器 `e2e_stage_audit.py` 的 A1~A11 判定。

### 4.1 语音链路

| 环节 | 现状 | 证据 |
|------|------|------|
| 前端录音 | ✅ 真实现（`getUserMedia` + `MediaRecorder` opus） | `useVoiceRecorder.ts:74,78-80,88-99` |
| 前端上传 | ✅ 真 POST multipart | `useVoiceRecorder.ts:142-146` → `apiRoutes.ts:78` `/voice/upload` |
| BFF 端点 | ✅ 存在，经 APISIX route 100（`/api/v1/*` catch-all）可达 | `voice_handler.go:36` |
| BFF 响应 | 🔴 **裸 `gin.H` 无 `data`** ⇒ 前端 `undefined` | `voice_handler.go:85-93`；`resp.go:1-7` 明文规定须用 `OK`；`useApi.ts:346,353` |
| 前端消费 | 🔴 **`if (result)` 无 else** → 整段成功分支被跳过且不报错 | `useVoiceRecorder.ts:148` |
| 转写文本 | 🔴 **后端就丢了**：`EmotionResult` 无 Text 字段 | `analyzer.go:20-25`；`analyzer/multimodal.go:103-119`；`multimodalanalyzelogic.go:74-79`（`kind=="audio" && textContent==""` ⇒ `Transcript=""`，而 BFF 从不传 text） |
| 音频 URL | 🔴 恒 `""`（自带 TODO） | `voice_handler.go:91` |
| 消息落库 | 🔴 `skipUserMessage: true` → 只 push 内存 | `[id].vue:329`；`useConversationSender.ts:104-111` |
| 语音情绪入融合 | 🔴 前端发 `voiceEmotion`，BFF 请求结构体无该字段 ⇒ JSON 静默忽略 | `useConversationSender.ts:136` vs `ai_stream_handler.go:215-225` |
| `/new` 页语音 | 🔴 **假按钮**：只翻 `isRecording` + 弹 toast，无 MediaRecorder | `new.vue:184-191` |

### 4.2 表情链路

| 环节 | 现状 | 证据 |
|------|------|------|
| 抓帧 | ✅ 离屏 canvas 320×240 → JPEG(0.7)，每 2 s 一次 | `useFaceEmotion.ts:40-46,54-58,109,112,64-66` |
| 上传 | ✅ multipart `kind=image` | `useFaceEmotion.ts:124-135` |
| BFF 端点 | ✅ 信封**正确**（用 `OK`） | `multimodal_handler.go:34,64-69` |
| FER 服务 | ✅ 端点齐备 `/health` `/analyze` `/metrics` | `emotion-echo-models/FER-tflite/server.py:122,132,137` |
| 结果展示 | ✅ 页面显示「当前表情识别：X（置信度 Y%）」 | `[id].vue:218-226`；`new.vue:118-126` |
| 参与消息 | 🔴 发消息恒传 `'neutral'`；`triggerCapture/getRecentEmotion` **全仓零调用**（死代码） | `[id].vue:362`；`useFaceEmotion.ts:155-172` |
| 落库 | 🔴 固定 `persist=false`（且被测试钉死）⇒ `emotion_echo_ai.face_emotion_results` 生产链路永不写入 | `useFaceEmotion.ts:132`；`useFaceEmotion.test.ts:45` |

### 4.3 文件上传链路

| 环节 | 现状 | 证据 |
|------|------|------|
| 前端入口 | ✅ 回形针 + 隐藏 file input（无拖拽） | `[id].vue:80-87,88-108,382-384` |
| 分类型限额 | ✅ image 5 MB / file 20 MB / video 50 MB | `useFileUpload.ts:8-21,40-50` |
| BFF 端点 | ✅ 真实现（kind 白名单 + mime + size + MinIO） | `upload_handler.go:88,95-106,115-138,148-153` |
| MinIO 写入 | ✅ `PutObject`，key `uploads/<uid>-<hash>.<ext>` | `upload_handler.go:170-181`；`storage/minio.go:87,109-112` |
| BFF 响应 | 🔴 **裸 `gin.H` 无 `data`**（错误分支却用了 `Fail`，只有成功分支是例外） | `upload_handler.go:156-163` |
| 发消息 | 🔴 url=undefined → content 被丢 → chat-svc 400 `content is required` | `useFileUpload.ts:120-127`；`[id].vue:402,409-425`；`sendmessagelogic.go:68-69` |
| chat-svc 附件字段 | ✅ `file_name` 列 + `ContentType`/`FileName` | `c006_add_file_name_to_messages.sql:11`；`model/conversation.go:42,46` |
| `/new` 页附件 | 🔴 显式未实现（弹"附件功能尚未实现"） | `new.vue:180-182` |

### 4.4 现有测试的**结构性盲区**（这正是断链能存活至今的原因）

| 测试 | 实际断言 | 为什么抓不到 |
|------|---------|-------------|
| `voice_handler_test.go:65-96` | `got["transcript"]` / `got["emotion"]` / `got["messageId"]` **在顶层读** | **把"缺 data 包装"的错误结构固化成契约** ⇒ 永远绿 |
| `upload_handler_test.go:110,131,150` | `assert.Equal(t, ..., got["url"])` 同样顶层读 | 同上 |
| `analyzer/multimodal_test.go:107-131` | 假 SenseVoice 返 `Text:"我太开心了"`，却**只断言** `PrimaryEmotion`/`Confidence` | 漏掉 `Text` ⇒ transcript 丢失永远不红 |
| `useFaceEmotion.test.ts:26-54` / `useFileUpload.test.ts:16-57` | 5 条**读源文件字符串**的字面量断言 | 零运行时行为（文件头自述"行为测试成本过高，改字面量"） |
| `apiRoutes.test.ts:17,77,81` | 把 `/voice*` `/multimodal*` 列入"允许的 orphan"白名单 | **给缺口开绿灯** |
| `a11y-baseline.spec.ts:50-55` | `expect` 被注释 ⇒ 永真 | 与账本 E2E-F-51 同型 |
| Playwright 15 个 spec | **零** `voice-btn`/`camera-btn`/非头像文件上传用例 | 三条链路无端到端回归钉 |
| `useVoiceRecorder` | **无任何测试** | — |

### 4.5 本阶段预探查的新发现（开工首步登记账本，先登记再修）

| 拟编号 | 现象 | 证据 |
|--------|------|------|
| E2E-F-103 | `/api/v1/voice/upload` 响应缺 `data` 包装 + 前端 `if (result)` 无 else ⇒ **录音后整条链路静默无反馈** | `voice_handler.go:85-93`；`useVoiceRecorder.ts:148` |
| E2E-F-104 | ASR 转写文本在 ai-svc 内部即被丢弃（`EmotionResult` 无 Text 字段）⇒ `/voice/upload` 的 `transcript` **恒为空**，修好信封也上不了屏 | `analyzer.go:20-25`；`multimodalanalyzelogic.go:74-79` |
| E2E-F-105 | `/api/v1/uploads/:kind` 成功响应缺 `data` 包装 ⇒ 附件消息必然被 chat-svc 400 拒（**MinIO 里文件已写入**，用户看到"发送失败"）——与已修 E2E-F-86 **同型** | `upload_handler.go:156-163`；`sendmessagelogic.go:68-69` |

> 同型先例处置可参照 E2E-F-86：改 `OK(c, ...)` 包装 + 同步改测试的顶层断言。**修复必须走 TDD**（AGENTS.md 第一性原则）：先写"响应含 `data.transcript` 且值等于假 client 返回值"的失败测试，再改 handler。

---

## 5. 测试点清单

判定标记：`[A]` 自动可判 · `[V]` 视觉判定（须截图且被查看）· `[M]` 需裁定。详见 [RUNBOOK.md](../RUNBOOK.md) §4。
本清单**无 `[M]` 点** —— 原先依赖裁定的两项已由 §8 采用默认口径后转为 `[A]`（可机械判定），这正是 §8 存在的意义：把"需要人裁定"提前收敛成"可断言"。

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| 1 | 语音上传经 APISIX（带 JWT）传真实 webm → 200，且 **`data.transcript` 为非空字符串**（值断言，非"存在"） | [A] | curl 真录音文件（或 Playwright 内 fake 音频）+ 断言 `data.transcript != ""` | 命令 + 响应体（脱敏）退出码 |
| 2 | 前端不再静默失败：ai-svc 不可达时页面出现可见错误提示 | [A] | Playwright mock/关停 ai-svc → 断言出现错误 toast/文案 | 截图 + 断言输出 |
| 3 | 语音消息出现在会话气泡中且**文本 = 测试点 1 的 API 值**（互指标一致性：UI = API） | [A]+[V] | Playwright + 读 DOM 文本 + 与 API 响应逐字比对 | 截图 + 断言输出 |
| 4 | 语音消息**可持久**：刷新页面 / 重新进入会话后仍在（修复 `skipUserMessage` 后） | [A] | Playwright reload 后断言消息仍在 + DB `messages` 行存在 | 截图 + `psql` 计数 |
| 5 | **真模型 vs 降级可区分**：响应 `model` 以 `sensevoice:` 开头（FER 为 `fer:`）；若回落关键词分析器则本点 FAIL | [A] | 断言 `model` 前缀（`analyzer/multimodal.go:90,116`） | 响应体 + 断言输出 |
| 6 | 语音情绪被 BFF 接收：`voiceEmotion` 不再被 `json.Unmarshal` 静默忽略（`ai_stream_handler.go:215-225` 结构体补字段），且该值进入后续处理 | [A] | Go 单测（字段被解析）+ 端到端抓包观察请求体 | 单测输出 + 抓包报文 |
| 7 | 单人脸帧 `kind=image` → 200 + `emotion` ∈ FER 标签集 + `confidence ∈ [0,1]` + `model` 前缀 `fer:` | [A] | curl/Playwright 上传真实帧 | 响应体 + 断言输出 |
| 8 | 摄像头开启：预览画面可见 + 「当前表情识别：X（置信度 Y%）」有值 | [V] | Playwright（fake video device）+ 截图查看 | 截图 ×2（chromium/mobile） |
| 9 | 连续抓帧稳定：≥3 帧间隔 2 s 成功、无 5xx、无前端报错 | [A] | Playwright 等待 ≥3 次请求 + 统计状态码 | 网络日志 + 截图 |
| 10 | 无脸帧 → 返 `neutral` fallback 而非 500 | [A] | 上传纯色/无脸图 → 断言 200 + neutral | 响应体 |
| 11 | **表情不落库的隐私契约钉**：使用摄像头 + 发送消息后，`emotion_echo_ai.face_emotion_results` 行数**不变**（防"未声明的新增写入"） | [A] | 使用前后各 `psql count(*)`，断言相等 | 两条 SQL 输出 |
| 12 | 附件上传成功：200 + **`data.url` 非空**，且 MinIO 对象真实存在（匿名 GET 200） | [A] | curl 上传 + `curl -I <url>` 断言 200 | 两条命令输出 |
| 13 | 附件消息真实送达：消息落库 `file_name` 非空且 `content == data.url` | [A] | `psql` 查 `messages` + 与 API 值比对 | SQL 输出 |
| 14 | 附件消息在图气泡中渲染（图片显示图 / 文件显示文件名） | [V] | Playwright + 截图查看 | 截图 ×2 |
| 15 | 附件被 AI 感知：带文件的消息请求体中 `files` 含该 URL 重写后的 source | [A] | 抓 BFF → LLM 的报文（`collectFileAttachments`） | 抓包输出 |
| 16 | 边界与鉴权（端到端复验）：超限 413 / 非法 mime 415 / 缺 `X-User-Id` 401 / 未带 JWT 401 | [A] | curl 各 1 次 | 4 条状态码 |
| 17 | 移动端（Pixel 5，375px）录音与附件入口可用且不遮挡 | [V] | Playwright mobile project + 截图 | 截图 ×2 |
| 18 | `/chat/conversation/new` 页语音与附件**不再是假入口**：录音真的走 `MediaRecorder` + 上传、附件真的可选并发消息 | [A]+[V] | Playwright 在该页录音/选文件 → 断言真实请求 + 消息出现 | 网络日志 + 截图 |

> 测试点编号与 report.md 的「测试点结果」编号集合必须一致（审计器 A3 会机械比对）。

---

## 6. 验收标准（DoD）

- [ ] 18 个测试点全部给出 `PASS` / `FAIL` / `BLOCKED` / `N/A` 之一 + **执行证据**（命令 + 输出/退出码/截图）；`BLOCKED` 数 ≤ 1/3
- [ ] 三处断链（E2E-F-103/104/105）修复**走完 TDD**：先提交失败测试（RED）→ 最小实现（GREEN）→ 重构；report §4 附 commit 与先行失败测试对照表
- [ ] `voice_handler_test.go` / `upload_handler_test.go` 的**顶层断言**改为 `data.*` 断言（否则旧测试会把修复判红，或反向固化错误结构）
- [ ] 回归钉：新增 `emotion-echo-web/e2e/multimodal.spec.ts`，双 project（chromium + mobile）首次运行记录结果
- [ ] 账本：开工首步登记 `E2E-F-103~105`；收口时逐条回填状态
- [ ] 文档同步：roadmap 状态 + 本 plan status + report.md 三处一致（审计器 A9）
- [ ] 若执行期改判了 §8 任一口径：同步改本文件 + `decisions.md`（D-11~D-13）并在 report §6 记录改判理由
- [ ] 镜像新鲜度核对记录（§3.2 硬规则）写入 report §1 环境基线
- [ ] `python scripts/e2e_stage_audit.py --stage e2e-16` 无 FAIL；`--all` 仍 0 FAIL
- [ ] §2.5 收口自检三连：`git status` 干净 / `main` 与 `origin/main` 无 ahead-behind / `git branch --merged main` 仅 main

---

## 7. 已知风险

| 风险 | 应对 |
|------|------|
| **冷 build 缓存**导致重建慢 / 中途失败 | 用 `scripts/build_dev_images.sh`（内含 base 预拉 + 逐服务 3 次重试）；base 镜像本地已有，失败多为网络抖动，重试即可 |
| **改了代码但容器跑旧镜像**（E2E-F-99/70 复发） | §3.2 硬规则：验收前 `docker inspect` 比对镜像 Created 与 commit 时间，写进 report §1 |
| **静默降级伪装 PASS** | 所有多模态点断言 `model` 前缀（§3.3） |
| Playwright headless 无麦克风/摄像头 | 需 `--use-fake-ui-for-media-stream` + `--use-fake-device-for-media-stream` launch args + `grantPermissions(['microphone','camera'])`；在 plan 定稿时已声明，spec 里显式配置 |
| `uv workers: 1` 串行 + fake device 等待 | 用例保持串行；对 2 s 抓帧间隔用"等待 ≥N 次请求"而非固定 sleep |
| FER / SenseVoice 容器首次启动慢（模型加载） | 健康检查全绿再开测（RUNBOOK §2.2），不靠 sleep |
| 范围蔓延到 TTS / 数字人 | §2「不做」明确列边界；发现即记账（E2E-F-1xx）不修 |
| 前端「本地 dev server vs 容器产物」两套并存导致验收对象错位 | report §1 必须写明本次验收用的是哪一套 + 端口归属（E2E-F-99 的过时认知纠正） |

---

## 8. 范围口径（三个产品/隐私取向议题，已采用默认口径）

> 三个议题均为**产品/隐私取向**问题，非工程判断。2026-09-22 撰写本文件时已向用户提出但未获答复，故**按推荐口径定稿**（下表"采用"列），保证 plan 可执行、不因未决而停摆。
> 拟定编号 **D-11 / D-12 / D-13**；用户若追认或改判，按 DoD 末条同步改本文件 + `decisions.md` + report §6。

| 议题 | 问题 | 采用口径 | 若改判的影响 |
|------|------|---------|-------------|
| **A 语音音频是否持久化**（拟 D-11） | 用户录音是否存 MinIO 并在气泡内可回放（`voice_handler.go:91` 现为 `""` TODO） | **不做**：语音链路止于"录音 → 转写文本上屏"，`audioUrl` 留空、气泡显示占位；`[id].vue:13-22` 的 `VoiceMessage` 回放分支本阶段**明确不在范围**且不新增音频存储 | 改判"做"→ 需加 BFF MinIO 音频写入 + 气泡接线 + 1 条 [A]+[V] 用例（测试点 6 附近） |
| **B 表情情绪是否落库**（拟 D-12） | 前端固定 `persist=false`，发消息恒传 `'neutral'`（`useFaceEmotion.ts:132`、`[id].vue:362`） | **不落库**：表情限定为"摄像头开启时的实时辅助展示"；`face_emotion_results` → `i005` 视图 → analytics modality **明确记为不可达**并记账；测试点 11 反向钉住"使用后表行数不变" | 改判"落库"→ 需先补隐私说明 + persist=true + 测试点覆盖落库值 |
| **C `/chat/conversation/new` 入口**（拟 D-13） | 该页语音是假按钮、附件弹"尚未实现"（`new.vue:184-191,180-182`） | **补齐**：把 `[id].vue` 已验证的接线搬到 `new.vue`，两页行为一致（测试点 18 覆盖） | 改判"不补"→ 删除测试点 18，两个缺口登记账本，测试点范围限定为 `/chat/conversation/:id` |

---

## 9. 产出物

- Playwright spec：`emotion-echo-web/e2e/multimodal.spec.ts`
- 单元/契约测试：`useVoiceRecorder.test.ts`（新增）、`voice_handler_test.go` / `upload_handler_test.go`（断言口径修正）、`analyzer/multimodal_test.go`（补 transcript 断言）
- 执行记录：`docs/e2e-roadmap/stages/e2e-16-multimodal/report.md`
- 截图：`docs/e2e-roadmap/stages/e2e-16-multimodal/screenshots/`（命名 `<编号>-<简述>.png`）
- 账本：`E2E-F-103~105` 登记与回填；`decisions.md` D-11~D-13（裁定后）
