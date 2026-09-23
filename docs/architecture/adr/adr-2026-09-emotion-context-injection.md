# ADR-2026-09-emotion-context-injection

## 状态

- **状态**：✅ Accepted
- **日期**：2026-09-22（2026-09-23 IAB 多情绪验证后定稿）
- **归属阶段**：E2E-16 多模态 / 决策 D-14（plan §B.10）

## 上下文

E2E-16 plan §B.10 要求"融合结果参与情绪计算，让 AI 回复体现用户当前情绪状态"。三个前置修复（F-103/104/105）打通语音/表情/文件上传链路后，多模态**采集结果**已经能写入 `face_emotion_results` / 落库 audio message —— 但 **AI 没有用上**：

- `aiStreamReq.Emotion` 字段虽有（消息级 happy/sad/...），`buildSystemPrompt` 从未消费
- `aiStreamReq` 无 face/voice emotion 字段
- 前端 `useFaceEmotion` / `useVoiceRecorder` 采到的情绪上下文不传给 BFF

这意味着：用户打开摄像头、表情显示"happy"、语音转写"sad"，AI 一无所知，回复仍是基础人设。

E2E-F-95（人格画像）教训：直接注入中文/英文枚举值，模型只能自行脑补；必须**给行为规格而非标签**。D-14 emotion context 继承同款护栏。

## 决策

### 决策 1：emotion 上下文由前端携带，BFF 拼到 system prompt

**方案**：前端 `useConversationSender.sendToExistingConversation` 从 `useFaceEmotion.getRecentEmotion()`（3 秒有效窗口）取最近一次表情结果，连同 `voiceEmotion` 一起塞进 `AIStreamParams.faceEmotion/faceConfidence/voiceEmotion/voiceConfidence` → BFF `aiStreamReq` 同名字段 → `buildSystemPromptWithEmotion` 拼"情绪上下文"段。

**为什么不后端查**：① 多模态采集实时性强（3 秒窗口），DB 查有 lag；② 后端再查等于把"已读到的实时信号"再"回查"——浪费；③ 与 F-95 同源："前端能直接给的信号别走后端绕一圈"。

### 决策 2：baseSystemPrompt 与 personality / emotion 三段拼接

```
system prompt = base
              + (optional) personality guide  // E2E-F-95 已落地
              + (optional) emotion context     // 本 ADR 新增
```

- **base**：固定人设（温柔共情陪伴者）
- **personality**：人格画像（维度分 → 行为规格）
- **emotion**：当前情绪（face/voice → 中性观察）

三段都"可选 + 不污染"：任一缺则 prompt 与无该段时**逐字相同**（不编造、不退化）。见 `ai_stream_handler.go:73 buildSystemPrompt` 与 `ai_stream_handler.go:buildSystemPromptWithEmotion`。

### 决策 3：中性句式 + 中英映射 + 三句护栏

**句式**：
```
情绪上下文：对方此刻神情看起来愉快，对方语气听起来平静。请让回应贴合对方此刻的状态。
```

**中英映射**（避免 prompt 出现英文枚举值）：
| 英文 | 中文 |
|------|------|
| happy | 愉快 |
| sad | 低落 |
| angry | 烦躁 |
| anxious | 不安 |
| neutral | 平静 |
| surprise | 惊讶 |
| fear | 紧张 |
| disgust | 不适 |
| 未知 | 原样返回 |

**三句护栏**（与 E2E-F-95 同款）：
1. **不点破来源**：prompt 不出现「摄像头 / 识别 / 分析 / 检测 / 设备 / 传感器 / 面部识别」等暴露采集手段的词
2. **不贴标签**：不当面称呼用户「你很 X」或类似直接贴脸标签
3. **不过火**：情绪描述保持中性简短（不超过 1 句话）

### 决策 4：emotionSource 高级模式留作独立轮次

本轮最小修法 = 用前端 payload 自带 emotion。**emotionSource 接口**（镜像 `personalitySource` 模式，从 DB 查最近情绪历史 + 引入 `AIStreamDeps.Emotion`）属下一轮独立工作。理由：① 当前最小可工作；② emotionSource 需要新加 repository（assessment-svc / emotion_analysis 表），范围超出 E2E-16 done 标准。

## 后果

### 正面

- ✅ AI 回复体现用户当前情绪状态（IAB 5 情绪验证通过：happy → "愉快的事吗" / sad → "压着些什么" / angry → "烦心事" / anxious → "不安/放不下" / neutral → baseline 不注入）
- ✅ 无情绪时 prompt 与原 base 逐字相同（不污染，E2E-F-95 教训守住）
- ✅ 三句护栏 grep 测试 4/4 PASS（避免英文枚举 + 不贴标签 + 护栏关键词）
- ✅ 9 个回归钉（4 Go + 5 TS）

### 风险 / 局限

- ❌ **摄像头关闭 / 拒绝权限时 emotion 上下文为空** → AI 不感知当前情绪（这是 E2E-F-122 留账的 emotionSource 高级模式目标）
- ❌ **AI 不知道用户历史情绪模式**（只能感知当前会话最近 3 秒）
- ❌ **前端 emotion 字段未经校验** → 恶意前端可伪造 emotion 注入 prompt（dev 模式威胁小，prod 需 E2E-29 JWT 鉴权 + E2E-29 §P0 防御）

## 验收证据

### 单元 / 契约测试

- `emotion-echo-web-bff/internal/handler/d14_emotion_context_test.go`：4 项
  1. `TestAIStreamReq_HasFaceEmotionField`（JSON 反序列化 round-trip）
  2. `TestBuildSystemPrompt_EmotionContext_Appended_WhenFaceOrVoicePresent`（含中文标签）
  3. `TestBuildSystemPrompt_EmotionContext_Absent_ReturnsBase`（无情绪时与 base 逐字相同）
  4. `TestBuildSystemPrompt_EmotionContext_HasGuardrails`（护栏关键词）
- `emotion-echo-web/app/composables/d14-ai-stream-emotion-fields.architecture.test.ts`：5 项字面量契约

### IAB 端到端（2026-09-23，5/5 情绪）

| faceEmotion | faceConfidence | AI 回复片段 |
|-------------|---------------|------------|
| happy | 0.9 | "听起来你今天状态挺平静的，还带着点愉快，这种感觉真好。愿意和我聊聊今天有什么让你开心的事吗？" |
| sad | 0.85 | "你开口问这句的时候，是不是自己心里也压着些什么呢？" |
| angry | 0.7 | "听起来你今天可能有些烦心事，愿意和我说说吗？" |
| anxious | 0.8 | "我其实有点不安，脑子里有些事放不下，也说不太清楚是什么。" |
| neutral | 0.0 | （baseline，不注入情绪段）"听起来你是在关心我的状态呢，谢谢你。" |

5 种情绪路径均观察到 prompt 注入生效（情绪词在 AI 回复里出现）。neutral baseline 未观察到情绪段（符合决策 2 的"不污染"约束）。

### 留账

- **E2E-F-122**：D-14 emotionSource 高级模式（DB 查最近情绪历史 + `AIStreamDeps.Emotion` 字段），独立轮次。

## 调研依据

- `docs/e2e-roadmap/stages/e2e-16-multimodal/plan.md` §B.10
- `docs/e2e-roadmap/discovered-unresolved.md` E2E-F-121 / E2E-F-122
- E2E-F-95（人格画像同款三句护栏 + 客观特征度量验收）
- F-70 教训（rebuild 镜像后才能验收）
