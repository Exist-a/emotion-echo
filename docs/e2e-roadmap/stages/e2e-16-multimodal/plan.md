---
stage: e2e-16
title: 多模态（语音 / 表情 / 文件上传）
type: transformation
status: in-progress
created: 2026-09-22
revised: 2026-09-22 (四项范围决议 D-11~D-14 由用户裁定后改稿：音频落 MinIO / 只发送时落一条 / new 页补齐 / 融合结果注入 prompt)
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

让三条链路从「**界面有按钮、后端有端点、但用户动作没有任何可观察结果**」变成「**用户动作 → 服务端处理 → 结果回到界面且可机械验证**」，并让多模态**真正参与情绪计算**（用户 2026-09-22 四项裁定，见 §8）：

1. **语音**：录音 → 转写文本上屏 + **音频落 MinIO 可在气泡内回放**
2. **表情**：周期捕捉（每 2 s）只留最新不落库；**发送消息时把最近一次捕捉落一条并绑定该消息**；该表情经现有异步融合链路（FusionWorker）**参与情绪融合**，并由 BFF **注入 system prompt** 让 AI 回复体现
3. **文件上传**：附件真实送达并被 AI 感知

探查结论（§4）：三条链路**卡的其实是同一个缺陷**——BFF 手写 `c.JSON(200, gin.H{...})` 缺 `data` 包装，而前端 `useApi` 统一 `return data.data`（`emotion-echo-web/app/composables/useApi.ts:346,353`）⇒ 前端拿到 `undefined`。`voice` 路径**无 else 分支**（静默无反馈），`upload` 路径**带着 `undefined` 继续发消息**（被 chat-svc 以 `content is required` 拒绝，用户看到"发送失败"但文件其实已写进 MinIO）。这与已修复的 **E2E-F-86**（头像上传"成功却提示失败"）**完全同型**，且 `handler/resp.go` 的包注释已明文规定"BFF 所有 handler 用 OK/Fail 包装"——两个 handler 是例外。

---

## 2. 范围与边界

### 做

**A. 语音输入（ASR）**
1. 修 `/api/v1/voice/upload` 响应信封：`voice_handler.go:85-93` 的裸 `gin.H` → `OK(c, gin.H{...})`
2. 修前端静默失败：`useVoiceRecorder.ts:148` 的 `if (result)` 补 else 分支（对齐 `[id].vue:419-421` 已有的报错写法）
3. 打通转写文本：`analyzer.EmotionResult` 增 `Text` 字段（`analyzer.go:20-25`）→ `analyzer/multimodal.go:103-119` 回填 → `logic/multimodalanalyzelogic.go:74-79` 优先用 ASR 文本
4. 语音消息上屏并落库：`[id].vue:329` 的 `skipUserMessage: true` 改为真实入库（否则刷新即丢，且无法验证"消息 = API 值"）
5. **音频落 MinIO**（§8 决议 A）：`voice_handler.go:91` 的 `audioUrl: ""` TODO 落地 —— 复用 `storage.PutObject`（与头像/通用上传同款，`upload_handler.go:148-153` 的模式），key 前缀 `voice/`；URL 回填响应
6. **语音气泡可回放**：`[id].vue:13-22` 的 `VoiceMessage` 分支接线，渲染 `<audio>` 可播放（否则该分支永不可达）

**B. 表情识别（FER）——"只发送时落一条 + 参与计算"**（§8 决议 B/D）
7. 周期捕捉**保持不落库**：`useFaceEmotion.ts:132` 的 `persist=false` 保留，但**补注释说明这是有意设计**（每 2 s 一行会无限堆积）；捕捉只更新内存 `currentEmotion`（现状已是覆盖式）
8. **发送消息时落一条**：接线现有的 `getRecentEmotion()`（3 秒有效窗口，`useFaceEmotion.ts:164-172`，当前**零调用**死代码）→ 在发送流程里以 `persist=true` + `messageId` 上报该次捕捉 → 写一条 `face_emotion_results`（`persistmodalanalyzelogic.go:105-116` 的 persist 路径已就绪，**只是前端从不触发**）
9. 前端把该结果随消息带给 BFF：`ai_stream_handler.go:215-225` 的 `aiStreamReq` 补 `faceEmotion` / `faceConfidence` 字段（前端本就要发，现被 `json.Unmarshal` 静默忽略）
10. **BFF 注入 system prompt**：`buildSystemPrompt`（`ai_stream_handler.go:73-87`）当前只拼 `baseSystemPrompt` + 人格画像段 ⇒ 新增情绪上下文段（镜像 `personalitySource` 的依赖注入模式：新增 `emotionSource`，nil = 不注入）。**必须带护栏**：不点破来源、不贴标签、不过火（与人格画像同款约束）
11. 消除"静默降级伪装成功"：断言响应 `model` 前缀（`fer:` / `sensevoice:`）而非只看 HTTP 200（§3.3）

**C. 文件上传**
12. 修 `/api/v1/uploads/:kind` 响应信封：`upload_handler.go:156-163` 裸 `gin.H` → `OK(c, ...)`
13. 附件消息真实送达（消息落库 `file_name` 非空 + content = MinIO URL）
14. 附件被 AI 感知复验（BFF `collectFileAttachments` / `fileSourceURL`）

**D. 入口一致性**
15. `/chat/conversation/new` 页多模态入口补齐（§8 决议 C）：语音改真录音（`new.vue:184-191`）、附件改真上传（`new.vue:180-182`），接线方式照搬 `[id].vue` 已验证路径

**E. 测试基建**
16. 新增 Playwright `e2e/multimodal.spec.ts`（fake media device，双 project）
17. 新增 `useVoiceRecorder` 行为测试（当前**零测试**）+ `useFaceEmotion` 的"发送时取最近窗口"行为测试（当前只有字面量断言）
18. 修正 `voice_handler_test.go` / `upload_handler_test.go` 中把**错误结构固化为契约**的断言（§4.4）

**F. 账本**
19. 把本次预探查的 3 条新发现登记为 `E2E-F-103~105`（§4.5），**先登记再修**

### 不做（边界）

| 不做项 | 理由 |
|--------|------|
| 数字人 + TTS 口型同步 | 归 E2E-17（D-03 已决议） |
| 周期捕捉逐帧落库 | §8 决议 B 明确"只发送时落一条"；每 2 s 一行会无限堆积（长时间开摄像头对话可达 1800 行/小时），且对融合无增益（融合按 message_id 取 `GetLatestByMessageID`） |
| 心跳/离线摄像头状态同步 | 非本阶段目标 |
| i18n | D-04 候选未决 |
| 多实例下 in-memory 限流/锁定、Redis 启用 | 归 E2E-18 / E2E-20（原设计文档曾用 Redis 存 face 情绪 TTL=3s，但 Redis 当前未启用 ⇒ 本阶段用前端内存，见 §4.6） |
| 拖拽上传、多文件并发上传 | 现状无实现，非本阶段目标 |
| 语音/表情的时序建模（多帧情绪趋势、情绪拐点） | 本阶段只验"单条消息取最近一次捕捉"，时序分析归后续 |

---

## 3. 前置条件

| 条件 | 状态 |
|------|------|
| E2E-10 done（聊天核心链路，多模态入口位于聊天页） | ✅ 2026-09-21 取证补拍轮恢复 done |
| `e2e_stage_audit.py --all` 0 FAIL | ✅ 2026-09-22 实测（30 阶段 0 FAIL） |
| `deploy/.env.local` 存在（LLM key 唯一存放点） | ✅ 2026-09-22 实测存在（1689 B）——**严禁删除/覆盖**（AGENTS.md §四红线） |
| ai profile 镜像已在本地 | ✅ `emotion-echo/fer-tflite:v0.1.0`（538 MB）+ `emotion-echo/sensevoice:v0.1.0`（4.07 GB）均在本地 |
| 冷 build 缓存下的重建能力 | ⚠️ **Build Cache = 0 B**（2026-09-22 实测），基础镜像本地已有 → 重建无需联网拉 base，但**首次构建耗时显著变长**（纪律见 §3.2） |
| **文字情绪链路在 dev 活着**（融合的必备模态） | ✅ **2026-09-22 实测通过**：基线 `emotion_analysis` 114 行（最新 09-21 00:00Z）→ 经网关发 1 条消息后 **115 行（最新 2026-09-22 00:55:53Z，当天）**，`fused_emotions` 同步 110 → **111 行（00:55:56Z，晚 3 s）** ⇒ Kafka → emotion_analysis → FusionWorker 整链在 dev 是活的（`fusion` worker 日志正常 tick）⇒ 融合类测试点可控 |
| 融合 Worker 在 dev 运行 | ✅ 代码层面确认：`main.go:463` 起 `if db != nil` 即启动 goroutine（tick 5 s）；LLM fuser 需 `LLM_BASE_URL` 非空，为空则回落 `WeightedLateFuser(0.4, 0.3, 0.3)`（`main.go:470-494`） |
| MinIO 可写 `voice/` 前缀 | ✅ **2026-09-22 实测通过**：`/uploads/file` 返回 `http://localhost:9000/avatars/uploads/1-a4c25c3a.txt`（注意 `uploads/` 是 **`avatars` bucket 内的 key 前缀**，非独立 bucket）→ 匿名 `curl` 该 URL **200 + 内容一致** ⇒ 同 bucket 下 `voice/` 前缀同样可匿名读，回放链路可行 |
| ai profile 的 FER / SenseVoice 真出结果（非降级） | ⚠️ **一半：FER ✅ / SenseVoice ❌**。FER：`kind=image` 实测 200 且 `model="fer:no-face"`（**真模型**，截图无脸故 neutral）→ 顺带验证测试点 17 的无脸降级语义。SenseVoice：**容器每次 `/analyze` 后重启，推理不可用** → 账本 **E2E-F-106**（torch 1.13.1 vs funasr 1.4.15 不兼容）⇒ ASR 类测试点标 `BLOCKED` |

**本表全绿方可把状态改为 `in-progress`**（RUNBOOK §1 开工前置检查）。§8 的四项范围决议已由用户 2026-09-22 裁定，不再属于"待定"。

### 3.1b 融合链路的两条硬约束（决定测试点 13/14 的写法）

1. **融合是异步的**（FusionWorker tick 5 s），而 **system prompt 是同步构建的**（同一请求内）。⇒ 「把**融合结果**注入 prompt」在时序上不可能。故 §8 决议 D 的落地口径是：**同步注入原始面部情绪**（前端随请求带 `faceEmotion`/`faceConfidence`，与原始设计文档 §4.2 一致），异步融合链路则同时独立验证（`fused_emotions` 行含 face 贡献）。
2. **文字的必备性**：`text == nil` ⇒ 跳过融合（`fusion/worker.go:132-136`）。⇒ 语音消息若没有文字情绪行，融合不会发生；测试点需明确用"文字消息 + 面部情绪"验证融合，语音路径单独验转写。

### 3.1 环境启动（固定动作）

```bash
cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
# ⚠️ --profile dev 不可省（E2E-F-108 实测）：Nacos 声明在 profiles: ["dev"] 下，
# 缺它则 6 个应用服务全部注册失败（analytics-svc 崩溃循环 + up -d 中止）
  -f compose.dev.yml --env-file .env.local --profile dev up -d
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
  -f compose.dev.yml --env-file .env.local --profile dev up -d
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
| 参与消息 | 🔴 发消息恒传 `'neutral'`（`[id].vue:362`）；`getRecentEmotion()`（3 秒窗口）**全仓零调用** | `[id].vue:362`；`useFaceEmotion.ts:155-172` |
| 周期捕捉落库 | 🔴 固定 `persist=false`（且被测试钉死）⇒ `face_emotion_results` 生产链路永不写入 | `useFaceEmotion.ts:132`；`useFaceEmotion.test.ts:45` |
| **落库能力其实已就绪**（只差前端触发） | ✅ `persist=true` 路径完整：`persistmodalanalyzelogic.go:105-116` 按 kind 写 `FaceEmotionResult{UploadID,MessageID,UserID,ConversationID,...}`，`UploadID` 幂等去重（同 upload_id 保留首条，`multimodal_repo_integration_test.go:28-59`） | `logic/persistmodalanalyzelogic.go:60-70,105-116` |
| **融合入口其实已就绪**（只差数据） | ✅ `FusionWorker.processOne` 按 message_id 取三路：`EmotionRepo.GetByMessageID` / `FaceEmotionRepo.GetLatestByMessageID` / `VoiceEmotionRepo.GetLatestByMessageID`（`fusion/worker.go:127-145`）⇒ 面部要参与融合，**只需该消息存在一条 face 行** | `fusion/worker.go:80-145` |
| 融合结果去向 | 🔴 **前端零消费**：BFF 有 `GET /api/v1/emotion/message/:id/fused`（`emotion_query_handler.go:36,60-87`），前端 `grep fused` 零命中；BFF `buildSystemPrompt` 只拼基础人设 + 人格画像（`ai_stream_handler.go:73-87`） | 见左列 |

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

### 4.6 原始设计依据（用户 2026-09-22 裁定所本）

用户裁定 B/D 时称「我记得之前的话就是每隔几秒进行捕捉，只存储新的结果，如果用户在这几秒内进行消息发送的情况下就用最新的捕捉识别进行计算」。据此检索到**确实存在的原始设计文档**：

`docs/legacy-plans/landed/multimodal-emotion-plan.md`（原路径 `.trae/documents/multimodal-emotion-feature/plan.md`，2026-06，front-matter 标注 `superseded-by: stage-34-multimodal-fusion.md + adr-2026-09-llm-fusion-hardening.md`）：

| 原文位置 | 内容 | 与现状对照 |
|---------|------|-----------|
| §2.1 架构图 L31/L105 | 「**每2秒/发送消息时触发**」；周期性抓拍 + **发送消息时立即抓拍** | 周期 2 s 已实现（`useFaceEmotion.ts:30`）；"发送时触发"即 `triggerCapture`（**未被调用**） |
| §2.1 L157 / §3.3 L351 | FER 结果存 **Redis**，key `face:emotion:{userId}:{sessionId}`，**TTL 3 秒** | 未实现（Redis 未启用，账本 E2E-F-08）；等价语义已在前端内存实现（`getRecentEmotion()` 的 3 秒窗口） |
| §2.1 L159 / §3.4 | 「**图片不存储，仅存储分析结果**」；§3.4 明写「面部情绪分析结果**不持久化存储**」 | 后被 Stage 34 扩展为 `face_emotion_results` 落库（按 message_id 关联）⇒ 用户裁定的"只发送时落一条"正是二者的折中：**结果落库、图片不落** |
| §2.2 / §4.2 | 发消息时前端带 `faceEmotion`/`faceConfidence` 字段 → 后端融合参考；「多模态上下文融入 system prompt」 | 字段与注入**均未实现**（`ai_stream_handler.go:215-225` 无该字段；`buildSystemPrompt` 无情绪段）⇒ 决议 D 的落地点 |
| §2.2 融合权重 | Face 0.5 / Voice 0.3 / Text 0.2 | 现实现为 `NewWeightedLateFuser(0.4, 0.3, 0.3)`（`main.go:495`）——**权重已是 LLM 融合失败时的兜底**，主路径是 `LLMFuser`（ADR-15） |

**融合子系统的当前形态**（`emotion-echo-ai-svc/internal/fusion/`，Stage 34/35）：

- `worker.go:103` 候选 = `ListPending` = `emotion_analysis` 中**尚无 `fused_emotions` 行**的 message_id（`fused_emotion_repository.go:195-202`），tick 5 s、LRU 限流（同 msgID 4 min 内不重复）
- `worker.go:127-145` 拼装 `ModalitySnapshot{Text, Voice, Face}`；**text 为必备模态**，缺失即 skip
- 主路径 `LLMFuser`（ADR-15 加固：markdown 去包 / schema 白名单校验 / 熔断 / 超时 3 s），失败回落 `WeightedLateFuser`
- 输出 `fused_emotions.modality_contrib`（JSONB，如 `{"text":0.4,"voice":0.3,"face":0.3}`，`i004_create_fused_emotions.sql:38`）⇒ **这就是"表情是否真的参与计算"的机械判据**

> ⚠️ **本文档撰写前已按 AGENTS.md 功课要求实读**：`fusion/{worker,late_fuser,snapshot}.go`、`logic/{persistmodalanalyzelogic,multimodalanalyzelogic}.go`、`repository/fused_emotion_repository.go`、`model/{fused_emotion,emotion}.go`、`handler/{emotion_query_handler,ai_stream_handler}.go`、`migrations/i004_create_fused_emotions.sql`、`main.go`、原设计文档全文。

---

## 5. 测试点清单

判定标记：`[A]` 自动可判 · `[V]` 视觉判定（须截图且被查看）· `[M]` 需裁定。详见 [RUNBOOK.md](../RUNBOOK.md) §4。
本清单**无 `[M]` 点** —— 原先依赖裁定的四项已由 §8 由用户裁定（2026-09-22）并转成可机械断言的口径。

### 5.1 语音链路（1~7）

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| 1 | 语音上传经 APISIX（带 JWT）传真实 webm → 200，且 **`data.transcript` 为非空字符串**（值断言，非"存在"） | [A] | curl 真录音文件（或 Playwright fake 音频）+ 断言 `data.transcript != ""` | 命令 + 响应体 + 退出码 |
| 2 | **音频落 MinIO 且可访问**：`data.audioUrl` 非空、前缀 `voice/`，`curl -I` 该 URL 得 200（§8-A） | [A] | curl 上传 → 取 URL → `curl -I` | 两条命令输出 |
| 3 | 语音消息**可回放**：气泡内 `<audio>` 元素存在且 `src == data.audioUrl`，`readyState`/`duration` 可读 | [A]+[V] | Playwright 读 DOM attribute + 截图 | 属性断言输出 + 截图 ×2 |
| 4 | 语音消息上屏且**文本 = 测试点 1 的 API 值**（互指标一致性：UI 文本 = API 值） | [A]+[V] | Playwright 读 DOM 文本 + 与 API 响应逐字比对 | 截图 + 断言输出 |
| 5 | 语音消息**可持久**：刷新后仍在（`skipUserMessage` 修复后），且 DB `messages` 行 `content_type=audio` + `file_name`/content 指向音频 URL | [A] | Playwright reload 断言 + `psql` 查行 | 截图 + SQL 输出 |
| 6 | 前端不再静默失败：ai-svc 不可达时页面出现**可见错误提示**（修掉 `if (result)` 无 else） | [A] | 关停 ai-svc → 录音 → 断言错误 toast/文案 | 截图 + 断言输出 |
| 7 | **真模型 vs 降级可区分**：`/multimodal/analyze`（audio）响应的 `model` 以 `sensevoice:` 开头；若回落关键词分析器 ⇒ 本点 FAIL | [A] | 断言 `model` 前缀（`analyzer/multimodal.go:116`） | 响应体 + 断言输出 |

### 5.2 表情链路：捕捉 / 落库语义（8~11）

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| 8 | 摄像头开启：预览可见 + 「当前表情识别：X（置信度 Y%）」有值 | [V] | Playwright（fake video device）+ 截图查看 | 截图 ×2 |
| 9 | 周期捕捉稳定：≥3 帧（≥6 s）全部 200、无 5xx、无前端报错 | [A] | Playwright 等待 ≥3 次请求 + 统计状态码 | 网络日志 + 截图 |
| 10 | **周期捕捉不落库**（§8-B）：开摄像头 10 s（≥5 次捕捉）前后，`face_emotion_results` 行数**不变** | [A] | 两次 `psql count(*)` 断言相等 | 两条 SQL 输出 |
| 11 | **发送时落一条且绑定该消息**（§8-B）：发送一条文字消息后，`face_emotion_results` **新增恰好 1 行**、`message_id` = 该消息 id、`primary_emotion` = 该次捕捉值 | [A] | `psql` 查新增行 + 与捕捉值比对 | SQL 输出 + 值比对 |

### 5.3 表情链路：参与计算与 AI 体现（12~15）

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| 12 | **3 秒窗口语义**（§8-B）：捕捉后 3 s 内发送 ⇒ 落库该情绪；隔 >3 s（或摄像头已关）再发送 ⇒ **不落库**（回落无面部输入） | [A] | 两个用例各查一次 DB 行数/值 | SQL 输出 ×2 |
| 13 | **融合真的把面部算进去了**（§8-D 前半）：发送带面部情绪的消息后，`fused_emotions` 出现该 message_id 行，且 `modality_contrib` 中 **`face` 值 > 0** | [A] | `psql` 查 `fused_emotions` 的 `modality_contrib::jsonb`（`i004_create_fused_emotions.sql:38`） | SQL 输出（含 JSONB 原文） |
| 14 | **融合结果注入 prompt 且值正确**（§8-D 后半）：抓 BFF → LLM 请求报文，system prompt 含情绪上下文段且其中的情绪值 = 前端发的 `faceEmotion` | [A] | 将 `LLM_BASE_URL` 临时指向本地捕获服务器（E2E-14 §8 已用过的手法）+ 断言 prompt 文本 | 抓包报文（脱敏） |
| 15 | **无面部输入时不编造**（降级路径，防"假注入"）：摄像头关闭时发送 → prompt **不含**情绪上下文段 | [A] | 同 14 抓包，断言该段不存在 | 抓包报文 |
| 16 | 单人脸帧 `kind=image` → 200 + `emotion` ∈ FER 标签集 + `confidence ∈ [0,1]` + `model` 前缀 `fer:` | [A] | curl 上传真实帧 | 响应体 + 断言输出 |
| 17 | 无脸帧 → 返 `neutral` fallback 而非 500 | [A] | 上传纯色/无脸图 → 断言 200 + neutral | 响应体 |

### 5.4 文件上传链路（18~21）

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| 18 | 附件上传成功：200 + **`data.url` 非空**，且 MinIO 对象真实存在（匿名 GET 200） | [A] | curl 上传 + `curl -I <url>` 断言 200 | 两条命令输出 |
| 19 | 附件消息真实送达：消息落库 `file_name` 非空且 `content == data.url` | [A] | `psql` 查 `messages` + 与 API 值比对 | SQL 输出 |
| 20 | 附件气泡渲染（图片显示图 / 文件显示文件名） | [V] | Playwright + 截图查看 | 截图 ×2 |
| 21 | 附件被 AI 感知：带文件的消息请求体中 `files` 含该 URL 重写后的 source | [A] | 抓 BFF → LLM 报文（`collectFileAttachments`） | 抓包输出 |

### 5.5 边界、入口一致性与移动端（22~24）

| # | 测试点 | 判定 | 验证方式 | 证据 |
|---|--------|------|---------|------|
| 22 | 边界与鉴权（端到端复验）：超限 413 / 非法 mime 415 / 缺 `X-User-Id` 401 / 未带 JWT 401 | [A] | curl 各 1 次 | 4 条状态码 |
| 23 | `/chat/conversation/new` 页三入口**不再是假入口**（§8-C）：语音真录音上传、附件真可发、摄像头可用 | [A]+[V] | Playwright 在该页操作 → 断言真实请求 + 消息出现 | 网络日志 + 截图 |
| 24 | 移动端（Pixel 5，375px）语音 / 附件 / 摄像头三入口可用且不遮挡 | [V] | Playwright mobile project + 截图 | 截图 ×3 |

> 测试点编号与 report.md 的「测试点结果」编号集合必须一致（审计器 A3 会机械比对）。共 **24 点**。

---

## 6. 验收标准（DoD）

- [ ] **24 个测试点**全部给出 `PASS` / `FAIL` / `BLOCKED` / `N/A` 之一 + **执行证据**（命令 + 输出/退出码/截图）；`BLOCKED` 数 ≤ 1/3
- [ ] 三处断链（E2E-F-103/104/105）修复**走完 TDD**：先提交失败测试（RED）→ 最小实现（GREEN）→ 重构；report §4 附 commit 与先行失败测试对照表
- [ ] **四项决议的新增能力同样走 TDD**（AGENTS.md 第一性原则，无例外）：音频落 MinIO（BFF 写存储 + URL 回填）、发送时落一条 face 行、prompt 情绪段注入（含"无输入不注入"的负向用例）、`new.vue` 三入口接线
- [ ] `voice_handler_test.go` / `upload_handler_test.go` 的**顶层断言**改为 `data.*` 断言（否则旧测试会把修复判红，或反向固化错误结构）
- [ ] 回归钉：新增 `emotion-echo-web/e2e/multimodal.spec.ts`，双 project（chromium + mobile）首次运行记录结果
- [ ] 账本：开工首步登记 `E2E-F-103~105`；收口时逐条回填状态
- [ ] `decisions.md` 登记 D-11~D-14（本文件 §8 的四项裁定）+ 索引表补行
- [ ] 文档同步：roadmap 状态 + 本 plan status + report.md 三处一致（审计器 A9）
- [ ] 若执行期发现 §8 任一口径不可行：**不得自行改判**，按 RUNBOOK §9 升级给用户，并在 report §6 记录
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
| **融合是异步的（tick 5 s），测试点 13 需等待** | 用轮询 SQL（"等 `fused_emotions` 出现该行"）而非固定 sleep；超时上限写进 report；LRU 限流（同 msgID 4 min）意味着**同一消息不要反复触发**，测试用不同消息 |
| **文字情绪链路若在 dev 停更，融合永远无候选** | §3 前置表已列为开工首步实测项；坏则标 BLOCKED 并升级（不得用"融合测试点 N/A"掩盖） |
| **prompt 注入可能污染人格画像段或越界说话** | 注入段必须带护栏（不点破来源 / 不贴标签 / 不过火，与 E2E-14 `buildPersonalityGuide` 同款）；测试点 15 反向钉住"无输入不编造" |
| **「落一条」的幂等性**：重试/双击可能写多行 | `face_emotion_results` 有 `UploadID` 幂等去重（同 upload_id 保留首条，`multimodal_repo_integration_test.go:28-59`）⇒ 前端必须为每次发送生成稳定 nonce；测试点 11 断言"恰好 1 行" |
| 音频落 MinIO 后的**保留策略**未定 | 本阶段只验"可写入 + 可回放"；保留期/清理归 E2E-19（备份恢复演练）与隐私需求，report §6 记为待决策 |

---

## 8. 范围决议（已由用户 2026-09-22 裁定）

> 四项均为**产品/隐私取向**问题，非工程判断。2026-09-22 由用户逐项裁定（编号拟 **D-11 ~ D-14**，落地时登记 `decisions.md`）。
> B/D 两项的裁定依据是仓库内确实存在的原始设计文档 [multimodal-emotion-plan.md](../../../legacy-plans/landed/multimodal-emotion-plan.md)（见 §4.6 逐条对照）。

| 决议 | 用户裁定 | 落地口径 | 对应测试点 |
|------|---------|---------|-----------|
| **A 语音音频**（拟 D-11） | **做，音频落 MinIO 可回放** | `voice_handler.go:91` 的 TODO 落地：复用 `storage.PutObject` 写 `voice/` 前缀，URL 回填 `audioUrl`；前端 `VoiceMessage` 分支渲染 `<audio>` | 2、3 |
| **B 表情的捕捉与存储**（拟 D-12） | **只发送时落一条**（周期捕捉只留最新、不落库） | 周期捕捉维持 `persist=false` 并补注释说明（有意设计）；发送时接线 `getRecentEmotion()`（3 s 窗口）→ `persist=true` + `messageId` 落**恰好一条** `face_emotion_results` | 10、11、12 |
| **C `/chat/conversation/new` 入口**（拟 D-13） | **补齐，两页行为一致** | 把 `[id].vue` 已验证的三入口接线搬到 `new.vue` | 23 |
| **D 融合结果的去向**（拟 D-14） | **还要让 AI 回复体现（prompt 注入）** | 两层：① 同步 —— 前端随 stream 请求带 `faceEmotion`/`faceConfidence`，BFF `buildSystemPrompt` 注入情绪上下文段（护栏同人格画像）；② 异步 —— 该消息的 face 行参与 FusionWorker 融合，`fused_emotions.modality_contrib.face > 0` 独立验证 | 13、14、15 |

> **不再有"待裁定"项**：本阶段范围已闭合，可直接按 §3 前置检查开工。

---

## 9. 产出物

- Playwright spec：`emotion-echo-web/e2e/multimodal.spec.ts`
- 单元/契约测试：`useVoiceRecorder.test.ts`（新增）、`useFaceEmotion.test.ts`（补"发送时取 3 s 窗口"行为用例，现有 5 条字面量断言保留）、`voice_handler_test.go` / `upload_handler_test.go`（断言口径修正）、`analyzer/multimodal_test.go`（补 transcript 断言）、`ai_stream_handler` 侧 prompt 注入单测（含负向）
- 执行记录：`docs/e2e-roadmap/stages/e2e-16-multimodal/report.md`
- 截图：`docs/e2e-roadmap/stages/e2e-16-multimodal/screenshots/`（命名 `<编号>-<简述>.png`）
- 账本：`E2E-F-103~105` 登记与回填；`decisions.md` D-11~D-14（四项裁定）+ 索引表补行
- ADR：若 prompt 注入的护栏/边界形成架构承诺 → 新建 `adr-2026-09-multimodal-prompt-injection.md`（与 E2E-14 的人格注入同族，需说明两者叠加时的优先级与互不干扰）
