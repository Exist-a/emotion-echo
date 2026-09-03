---
status: landed
superseded-by: stage-35-production-hardening.md
original-path: .trae/documents/tts_optimization_plan.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-A
---

# XTTS 语速、音色调节及口型同步优化计划

## 问题分析

### 1. 语速调节
- **问题**：当前模型语速过快
- **解决方案**：XTTS支持`speed`参数（范围：0.5-2.0），需要在后端API中添加此参数

### 2. 音色调节
- **问题**：当前使用固定参考音频，需要支持更换音色
- **解决方案**：XTTS通过`speaker_wav`参数支持声音克隆，可提供多个参考音频选择

### 3. 口部运动时机
- **问题**：AI开始回复时嘴就开始动，应该在音频开始播放时才动
- **解决方案**：修改前端`useTTSPlayer.ts`，在第一个音频chunk播放时才启动口型动画

### 4. 表情持续时间
- **问题**：表情持续时间太短
- **解决方案**：修改前端`DigitalHuman.vue`中的表情动画时长参数

## 修改文件清单

### 后端修改

**文件**: `Emotion-Echo-LLM/XTTS/server.py`
- 在`TTSRequest`模型中添加`speed`参数
- 在`synthesize`调用中传入`speed`参数
- 在流式推理中也支持`speed`参数

### 前端修改

**文件**: `Emotion-Echo-Web/app/composables/useTTSPlayer.ts`
- 修改流式播放逻辑，在第一个chunk开始播放时触发口型动画
- 传递`speed`参数到后端

**文件**: `Emotion-Echo-Web/app/components/digital-human/DigitalHuman.vue`
- 增加表情持续时间参数
- 修改`setEmotion`方法的动画时长

**文件**: `Emotion-Echo-Web/app/composables/useDigitalHumanTTS.ts`
- 支持传递语速参数

## 实现步骤

1. **修改后端server.py**:
   - 添加`speed`参数到请求模型
   - 在`/tts`、`/tts_stream`、`/tts_with_phonemes`端点中支持speed参数

2. **修改前端useTTSPlayer.ts**:
   - 添加`playStream`方法中的口型动画触发逻辑
   - 在第一个音频chunk播放时调用口型同步

3. **修改前端DigitalHuman.vue**:
   - 调整表情持续时间（当前约1秒，建议改为2-3秒）

## 参数说明

| 参数 | 范围 | 默认值 | 说明 |
|------|------|--------|------|
| speed | 0.5-2.0 | 0.9 | 语速，0.5最慢，2.0最快 |
| emotionDuration | 1000-5000ms | 2000ms | 表情持续时间 |

## 风险评估

- 低风险：参数调整不影响核心逻辑
- 需要测试不同语速下的音频质量
- 表情时长调整可能需要用户反馈进行微调

## 测试要点

1. 测试不同语速（0.7, 0.9, 1.1）的音频效果
2. 验证口型动画是否在音频播放时才开始
3. 检查表情切换是否自然流畅