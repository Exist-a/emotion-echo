---
status: landed
superseded-by: stage-22-ai-services-containerization.md + stage-25-final-landing.md
original-path: .trae/specs/digital-human-phase1/{spec,checklist,tasks}.md
original-date: 2026-06-XX
migrated-at: 2026-09-03
round: 2-A
note: 三份(spec/checklist/tasks)合并为一份
---

# 数字人功能 Phase 1（合并自 spec + checklist + tasks）

## 原始 spec.md

# 数字人功能 Phase 1 实现规范

## Why
需要在Emotion Echo项目中实现3D数字人功能，提供更丰富的用户交互体验。第一阶段重点实现基础渲染、拖拽定位和控制开关。

## What Changes

### Phase 1 核心功能
1. **3D数字人渲染** - 使用Three.js + @pixiv/three-vrm加载VRM模型
2. **拖拽定位** - 用户可自由拖动数字人到任意位置
3. **显示/语音开关** - 控制数字人显示和语音播放

### Phase 2 预留功能（延后实施）
1. **口型动画** - 根据XTTS返回的时间戳同步口型
2. **情绪动作** - 根据对话情绪展示预设动作

### 技术架构更新
1. **TTS服务配置** - 指向 localhost:8003
2. **Nuxt 4 适配** - 根据官方文档调整目录结构
3. **VRM支持** - 安装@pixiv/three-vrm

## Impact

### Affected Capabilities
- 前端：新增数字人组件、语音播放器
- 状态管理：新增DigitalHumanStore
- 样式：圆形数字人容器

### Affected Code
- `app/components/digital-human/` - 新增数字人组件
- `app/composables/useTTSPlayer.ts` - 新增语音播放逻辑
- `app/stores/digitalHuman.ts` - 新增状态管理
- `nuxt.config.ts` - TTS服务地址配置

---

## ADDED Requirements

### Requirement: 3D数字人渲染
系统**SHALL**提供在页面上渲染3D数字人的能力。

#### Scenario: 页面加载数字人
- **GIVEN** 用户打开聊天页面
- **WHEN** 页面加载完成
- **THEN** 数字人模型应显示在默认位置（右上角）

#### Scenario: 圆形容器显示
- **GIVEN** 数字人模型已加载
- **WHEN** 数字人显示开关为开启状态
- **THEN** 数字人应显示在圆形白色卡片容器内，容器有阴影效果

---

### Requirement: 拖拽定位
系统**SHALL**提供拖拽移动数字人位置的能力。

#### Scenario: 拖拽数字人
- **GIVEN** 数字人可见
- **WHEN** 用户鼠标按住数字人并拖动
- **THEN** 数字人应跟随鼠标移动到新位置
- **AND** 位置信息应保存到状态管理中

#### Scenario: 拖拽边界限制
- **GIVEN** 用户拖动数字人
- **WHEN** 数字人移动到视口边缘
- **THEN** 数字人应在视口边界内，不能拖出屏幕

---

### Requirement: 语音开关控制
系统**SHALL**提供控制语音播放的开关。

#### Scenario: 语音开关关闭时不播放
- **GIVEN** AI已返回语音数据
- **WHEN** 语音开关为关闭状态
- **THEN** 不应播放语音
- **AND** 语音数据仍可正常生成

#### Scenario: 语音开关开启时播放
- **GIVEN** AI已返回语音数据
- **WHEN** 语音开关为开启状态
- **THEN** 应播放语音

---

### Requirement: 显示开关控制
系统**SHALL**提供控制数字人显示的开关。

#### Scenario: 显示开关关闭时隐藏
- **WHEN** 显示开关为关闭状态
- **THEN** 数字人应完全隐藏
- **AND** 不占用页面空间

#### Scenario: 显示开关开启时显示
- **WHEN** 显示开关为开启状态
- **THEN** 数字人应显示在页面

---

## MODIFIED Requirements

### Requirement: TTS服务配置
**原配置**: `localhost:5002`
**新配置**: `localhost:8003`

系统**SHALL**将TTS服务地址配置为 `http://localhost:8003`，使用以下接口：
- `POST /tts` - 文本转语音
- `POST /tts_with_phonemes` - 文本转语音+时间戳（Phase 2使用）

---

## Technical Specifications

### 目录结构
```
Emotion-Echo-Web/
└── app/
    ├── assets/
    │   └── 3d-models/
    │       └── digital-human.vrm
    ├── components/
    │   └── digital-human/
    │       └── DigitalHuman.vue
    ├── composables/
    │   └── useTTSPlayer.ts
    └── stores/
        └── digitalHuman.ts
```

### 组件设计

#### DigitalHuman.vue
- **功能**: 渲染3D数字人、处理拖拽、显示控制
- **Props**:
  - `modelPath: string` - VRM模型路径
  - `visible: boolean` - 是否显示
  - `draggable: boolean` - 是否可拖拽
- **Events**:
  - `@position-change` - 位置变更

#### useTTSPlayer.ts
- **功能**: 调用XTTS服务、处理音频播放
- **接口**:
  - `play(text: string, language: string)` - 播放语音
  - `pause()` - 暂停
  - `resume()` - 恢复
  - `stop()` - 停止
- **状态**:
  - `isPlaying: Ref<boolean>`
  - `currentTime: Ref<number>`

#### digitalHuman.ts (Pinia Store)
- **State**:
  - `visible: boolean`
  - `position: { x: number, y: number }`
  - `voiceEnabled: boolean`
- **Actions**:
  - `setPosition(x, y)`
  - `toggleVisible()`
  - `toggleVoice()`

### UI设计

#### 圆形容器样式
```css
.digital-human-container {
  width: 200px;
  height: 200px;
  border-radius: 50%;
  background: white;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.15);
  border: 3px solid #fff;
  overflow: hidden;
  position: absolute;
  cursor: move;
}
```

#### 控制按钮
- 显示开关：眼睛图标
- 语音开关：喇叭图标
- 悬停时显示按钮

### 依赖安装
```bash
npm install three @pixiv/three-vrm @types/three
```

---

## Implementation Notes

### VRM模型加载
使用 `@pixiv/three-vrm` 库加载VRM格式模型：
```typescript
import { GLTFLoader } from 'three/examples/jsm/loaders/GLTFLoader'
import { VRMLoaderPlugin } from '@pixiv/three-vrm'
```

### XTTS接口调用
```typescript
const response = await fetch('http://localhost:8003/tts', {
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify({
    text: '要转换的文本',
    language: 'zh-cn'
  })
})
```

---

## Phase 2 预留（口型动画）

Phase 2将实现口型动画功能，需要：
1. 使用 `/tts_with_phonemes` 接口获取时间戳
2. 根据phonemes时间戳驱动VRM BlendShape
3. 实现平滑的口型动画过渡

---

## 原始 checklist.md

# 数字人功能 Phase 1 检查清单

## 基础建设检查

- [x] **依赖安装**
  - [x] `npm install three @pixiv/three-vrm @types/three` 执行成功
  - [x] package.json中已添加依赖
  - [x] TypeScript类型定义完整

- [x] **组件结构**
  - [x] `app/components/digital-human/DigitalHuman.vue` 已创建
  - [x] 组件使用Vue 3 Composition API
  - [x] 已添加必要的props定义
  - [x] 已添加emits定义

- [x] **VRM加载**
  - [x] Three.js场景正确初始化
  - [x] VRM模型加载成功
  - [x] 模型渲染到canvas可见
  - [x] 加载过程有loading提示

- [x] **圆形容器**
  - [x] 容器为圆形（border-radius: 50%）
  - [x] 白色背景
  - [x] 阴影效果可见
  - [x] 3px白色边框
  - [x] overflow正确裁剪为圆形
  - [x] 默认位置在右上角

## 交互功能检查

- [x] **拖拽定位**
  - [x] 鼠标按下可拖动数字人
  - [x] 拖动时位置实时更新
  - [x] 释放后位置保持
  - [x] 拖动不能超出视口边界
  - [x] 位置状态已保存到Store

- [x] **状态管理Store**
  - [x] Pinia store正确创建
  - [x] visible状态正常工作
  - [x] position状态正常工作
  - [x] voiceEnabled状态正常工作
  - [x] toggleVisible action工作
  - [x] toggleVoice action工作
  - [x] setPosition action工作

- [x] **显示开关**
  - [x] 按钮图标显示正确
  - [x] 点击切换显示/隐藏
  - [x] 状态与Store同步
  - [x] 切换动画流畅

- [x] **语音开关**
  - [x] 按钮图标显示正确
  - [x] 点击切换开启/关闭
  - [x] 状态与Store同步
  - [x] 切换不影响数字人显示

## TTS集成检查

- [x] **TTS Player Composable**
  - [x] useTTSPlayer.ts已创建
  - [x] 调用localhost:8003/tts接口
  - [x] 正确解析返回的audio数据
  - [x] play接口正常工作
  - [x] pause接口正常工作
  - [x] resume接口正常工作
  - [x] stop接口正常工作

- [x] **配置更新**
  - [x] nuxt.config.ts中已添加TTS服务地址
  - [x] runtimeConfig正确配置
  - [x] 环境变量可读

## UI优化检查

- [x] **按钮交互**
  - [x] 鼠标悬停显示控制按钮
  - [x] 鼠标离开隐藏控制按钮
  - [x] 按钮样式美观
  - [x] 动画过渡流畅

- [x] **页面集成**
  - [x] 组件已添加到聊天页面
  - [x] 默认显示状态正确
  - [x] 位置正确
  - [x] 完整交互流程测试通过

## Phase 2 预留检查项（暂不检查）

以下功能将在Phase 2实施：
- [ ] **口型动画同步** - phonemes时间戳正确解析和驱动
- [ ] **情绪动作预设** - 根据情绪展示预设动作

## 整体检查

- [ ] **无Console错误**
  - [ ] 页面加载无错误
  - [ ] 控制台无警告
  - [ ] 网络请求无失败

- [ ] **代码规范**
  - [ ] ESLint检查通过
  - [ ] TypeScript类型正确
  - [ ] 注释完整（按需）
  - [ ] 符合项目代码风格

- [ ] **功能完整性**
  - [ ] 3D渲染正常工作
  - [ ] 拖拽定位正常工作
  - [ ] 显示开关正常工作
  - [ ] 语音开关正常工作

---

## 原始 tasks.md

# 数字人功能 Phase 1 任务清单

## 任务执行顺序

### 阶段一：基础建设

- [x] **Task 1**: 安装Three.js和VRM加载器依赖
  - 安装 `three`, `@pixiv/three-vrm`, `@types/three`
  - 验证安装成功

- [x] **Task 2**: 创建数字人组件基础结构
  - 创建目录 `app/components/digital-human/`
  - 创建基础 `DigitalHuman.vue` 组件框架
  - 配置Nuxt允许客户端3D渲染

- [x] **Task 3**: 实现VRM模型加载
  - 在DigitalHuman.vue中集成Three.js场景
  - 使用@pixiv/three-vrm加载VRM模型
  - 实现模型渲染到canvas

- [x] **Task 4**: 实现圆形容器UI
  - 创建圆形白色容器样式
  - 添加阴影效果
  - 设置默认位置（右上角）
  - 处理overflow裁剪为圆形

### 阶段二：交互功能

- [x] **Task 5**: 实现拖拽定位功能
  - 添加鼠标拖拽事件监听
  - 实现位置跟随逻辑
  - 添加边界限制（不能拖出视口）
  - 保存位置到状态管理

- [x] **Task 6**: 创建DigitalHumanStore状态管理
  - 创建Pinia store
  - 定义状态字段（visible, position, voiceEnabled）
  - 实现actions（setPosition, toggleVisible, toggleVoice）

- [x] **Task 7**: 实现显示开关功能
  - 添加显示/隐藏按钮
  - 按钮样式（眼睛图标）
  - 绑定DigitalHumanStore的visible状态
  - 实现切换逻辑

- [x] **Task 8**: 实现语音开关功能
  - 添加语音开关按钮
  - 按钮样式（喇叭图标）
  - 绑定DigitalHumanStore的voiceEnabled状态
  - 实现切换逻辑

### 阶段三：TTS集成

- [x] **Task 9**: 创建useTTSPlayer composable
  - 实现API调用（调用localhost:8003）
  - 实现音频播放功能
  - 提供播放控制接口（play, pause, resume, stop）

- [x] **Task 10**: 配置TTS服务地址
  - 在nuxt.config.ts中添加runtimeConfig
  - 设置TTS服务地址为 http://localhost:8003
  - 创建API调用composable

### 阶段四：UI优化

- [x] **Task 11**: 按钮悬停效果
  - 鼠标悬停显示控制按钮
  - 鼠标离开隐藏控制按钮
  - 优化动画过渡效果

- [x] **Task 12**: 集成到聊天页面
  - 将DigitalHuman组件添加到聊天页面
  - 确认默认位置和显示状态
  - 测试完整交互流程

---

## 任务依赖关系

```
Task 1 (依赖安装)
    ↓
Task 2 (基础框架) ← Task 3 (VRM加载)
    ↓                    ↓
Task 4 (圆形UI)      Task 5 (拖拽)
    ↓                    ↓
    └────────────────────┘
            ↓
    Task 6 (Store)
            ↓
    Task 7, Task 8 (开关)
            ↓
    Task 9, Task 10 (TTS)
            ↓
    Task 11, Task 12 (优化)
```

---

## Phase 2: 基础动画（已实施）

- [x] **Phase 2 Task 1**: 手臂自然放下姿势
  - 调整上臂旋转，让手臂自然下垂
  - 前臂稍微弯曲，更自然的姿势
  - 左右手臂对称

- [x] **Phase 2 Task 2**: 头部左右微动
  - 缓慢的左右晃动
  - 幅度适中，不显得夸张
  - 与动画循环集成

- [x] **Phase 2 Task 3**: 眨眼动画（5秒一次）
  - 定时触发眨眼
  - 眨眼持续约0.2秒
  - 使用BlendShape表情

---

## Phase 3 预留任务（待实施）

- 口型动画同步
- 情绪动作预设（高兴/生气/悲伤/思考）

---

## 验证标准

每个Task完成后应验证：
1. 功能正常工作
2. 无console错误
3. 符合UI设计规范
4. 代码符合项目风格
