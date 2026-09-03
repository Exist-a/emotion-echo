---
status: landed
superseded-by: stage-24-endpoint-verification-and-bugfix.md + stage-25-final-landing.md
original-path: .trae/documents/phase3-implementation-plan.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-A
---

# Phase 3 实现计划：XTTS 集成与数字人口型/表情同步

## 一、现状分析

### 1.1 当前流程
```
用户发送文字 → 后端 AI 服务 → AI 流式回复 → 前端展示
```

### 1.2 需要新增的流程
```
AI 回复文字 → 过滤 Markdown/代码块 → 分批发送给 XTTS (8003)
  → 返回音频 + 时间戳 → 前端播放音频 + 驱动口型
AI 回复情绪标签 → 前端驱动表情
```

---

## 二、核心设计决策（已确认）

### 2.1 情绪标签
- 后端在 `finish` 事件中返回 AI 情绪标签
- 前端根据情绪标签驱动数字人表情

### 2.2 TTS 触发时机：分批调用 + 缓冲区
- 前端维护文本缓冲区
- 积累到一定量（30-50 字符）或遇到句号（。！？）时触发 TTS
- 平衡延迟和资源消耗

### 2.3 口型同步精度
- 使用字符级时间戳（MVP 版本）
- 简化口型映射方案

### 2.4 口型映射
- 简化方案：按元音或固定规则切换口型
- 不做复杂的声母/韵母映射

### 2.5 文本预处理（重要！）
发送给 TTS 前必须过滤：
- ❌ Markdown 语法（`*` `_` `` ` `` `>` `#` 等）
- ❌ 代码块内容（``` 包裹的内容）
- ❌ 链接 URL
- ❌ 多余空白字符
- ✅ 保留纯文本内容

---

## 三、技术方案

### 3.1 后端新增接口

#### 修改 `finish` 事件返回格式
```go
// 原有 finish 事件
{ "type": "finish", "messageId": "xxx" }

// 新增 emotion 字段
{ "type": "finish", "messageId": "xxx", "emotion": "happy" }
```

#### 新增 TTS 接口（可选，也可以直接调 XTTS 8003）
如果后端封装：
- `POST /api/tts/synthesize`
- 请求：`{ "text": "纯文本内容", "language": "zh-cn" }`
- 响应：`{ "audio": "base64", "phonemes": [...], "duration": 3.5 }`

### 3.2 前端文本预处理模块
```typescript
// utils/markdownToPlainText.ts
export function stripMarkdown(text: string): string {
  // 1. 移除代码块（```xxx```）
  // 2. 移除行内代码（`xxx`）
  // 3. 移除 Markdown 语法（* _ # > - [ ] ( ) 等）
  // 4. 移除 URLs
  // 5. 合并多余空白
  return plainText
}
```

### 3.3 前端 TTS Player 扩展
```typescript
// useTTSPlayer 扩展
interface TTSResult {
  audio: string;
  phonemes: Phoneme[];
  duration: number;
}

interface LipSyncState {
  text: string;
  startTime: number;
  phonemes: Phoneme[];
}

// 缓冲区管理
class TTSBuffer {
  private buffer: string = '';
  private readonly TRIGGER_THRESHOLD = 30; // 字符数
  private readonly PUNCTUATION = /[。！？]/;

  append(text: string): string | null; // 返回触发时积累的文本
  flush(): string; // 返回剩余文本
}
```

### 3.4 DigitalHuman 组件扩展
```typescript
// 新增方法
setEmotion(emotion: 'happy' | 'sad' | 'angry' | 'anxious' | 'neutral'): void
playLipAnimation(phonemes: Phoneme[], startTime: number): void
stopLipAnimation(): void

// 口型简化映射
const lipShapeMap: Record<string, 'aa' | 'ee' | 'ih' | 'oh' | 'ou'> = {
  'a': 'aa', 'o': 'oh', 'e': 'ee',
  'i': 'ih', 'u': 'ou', 'ü': 'ee',
  // 默认
}
```

---

## 四、实现步骤

### Step 1: 后端 - 修改 finish 事件
- [ ] 修改 `ai_service.go` 中的 `finish` 事件，添加 `emotion` 字段
- [ ] 确保返回 AI 回复内容的情绪标签

### Step 2: 前端 - 文本预处理模块
- [ ] 创建 `app/utils/stripMarkdown.ts`
- [ ] 实现 Markdown/代码块过滤逻辑
- [ ] 编写测试用例

### Step 3: 前端 - 扩展 useTTSPlayer
- [ ] 修改 `useTTSPlayer.ts`
- [ ] 添加缓冲区管理类
- [ ] 添加分批 TTS 调用逻辑
- [ ] 添加口型动画同步播放

### Step 4: 前端 - 扩展 DigitalHuman 组件
- [ ] 添加 `setEmotion()` 方法
- [ ] 添加口型动画驱动方法
- [ ] 集成表情和口型控制

### Step 5: 前端 - 集成到消息流程
- [ ] 修改 `messageStore.sendAIStream()`
- [ ] 在 `delta` 事件中累积文本并触发 TTS
- [ ] 在 `finish` 事件中获取情绪并设置表情
- [ ] 处理播放队列和时序

### Step 6: 测试与调优
- [ ] 单元测试
- [ ] 集成测试
- [ ] 性能优化

---

## 五、文件变更清单

### 后端 (Emotion-Echo-Gin)
| 文件 | 变更 |
|------|------|
| `internal/service/ai_service.go` | 修改 finish 事件返回格式 |

### 前端 (Emotion-Echo-Web)
| 文件 | 变更 |
|------|------|
| `app/utils/stripMarkdown.ts` | 新增 - Markdown 过滤工具 |
| `app/composables/useTTSPlayer.ts` | 修改 - 添加缓冲区管理、口型同步 |
| `app/components/digital-human/DigitalHuman.vue` | 修改 - 添加表情控制、口型动画 |
| `app/stores/message.ts` | 修改 - TTS 触发逻辑 |
| `app/types/api.ts` | 修改 - StreamChunk 类型添加 emotion |

---

## 六、风险评估

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| XTTS 延迟 | 语音播放延迟 | 预加载、分批调用 |
| 口型不同步 | 口型与语音不匹配 | MVP 阶段降低同步精度要求 |
| Markdown 过滤不完整 | TTS 读到奇怪内容 | 充分测试边界情况 |
| 性能问题 | 低配置设备卡顿 | 降级方案 |
