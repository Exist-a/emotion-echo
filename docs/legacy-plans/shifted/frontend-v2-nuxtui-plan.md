---
status: shifted
superseded-by: elementplus-to-native.md（最终方案：完全不下 UI 库）
original-path: .trae/documents/frontend-refactor/frontend-refactor-plan.md
original-date: 2026-07-15
migrated-at: 2026-09-03
round: 2-A
---

# Emotion-Echo 前端重构方案（v2.0）

> 日期：2026-07-15
> 状态：待评审
> 范围：`Emotion-Echo-Web/` 整体重构
> 核心目标：替换 Element Plus 为现代化组件库，统一设计语言，重构路由与视觉风格，但 **保留全部已有功能**

---

## 一、现状分析与重构动因

### 1.1 项目现状

`Emotion-Echo-Web` 是基于 **Vue 3 + Nuxt 4** 的前端工程，目前使用 **Element Plus**（@element-plus/nuxt）作为 UI 库。已实现功能：

| 模块 | 功能 | 状态 |
|------|------|------|
| 登录 | 登录、注册、忘记密码（4 步流程）、微信/QQ OAuth 占位 | ✅ |
| 聊天 | 会话列表、Markdown 流式 AI 对话、语音输入、面部情绪、3D 数字人、TTS | ✅ |
| 用户中心 | 资料修改、头像上传、行为图表 | ✅ |
| 设置 | 字体大小、主题切换（light/dark/auto） | ✅ |
| 统计 | 日/周/月/年报（折线/饼图）、意图分布 | ✅ |
| 心理测验 | 量表列表、答题、结果 | ✅ |

### 1.2 当前痛点

| 痛点 | 体现 |
|------|------|
| **视觉陈旧** | Element Plus 默认风格偏"传统中后台"，对一款情绪/陪伴类产品而言过于严肃、商务 |
| **设计语言割裂** | 大量页面用 `:deep(.el-*)` 暴力覆盖样式，全局 SCSS 变量映射到 Element CSS 变量非常笨重 |
| **包体积大** | Element Plus 全量约 ~43MB（npm-compare 数据），即使按需加载，首屏仍偏重 |
| **API 风格老旧** | `el-form` + `:rules` + `ref.validate()` 的选项式 API 不够直观，与项目主推的 Composition API 不匹配 |
| **暗色模式蹩脚** | 需手动写大量 `html.dark .xxx { ... !important }`，无法跟随主题切换 |
| **图标单一** | `@element-plus/icons-vue` 数量有限（约 200+），风格中性无个性 |
| **响应式不优雅** | 每个页面写两套 `.pc-xxx / .mobile-xxx` class，工作量大且易遗漏 |

### 1.3 重构核心目标

1. **替换 UI 库** → 用现代化、轻量、原生适配 Nuxt 4 的组件库
2. **统一设计语言** → 基于现有"沉静墨绿"色系，构建 Design Token 系统
3. **重构路由结构** → 嵌套更清晰、命名更规范、移动端与 PC 端同一套组件
4. **保留全部功能** → 不删任何业务逻辑，纯视觉与组件层替换
5. **提升可维护性** → 用 Tailwind + 设计令牌 + 语义化 class，减少魔法值与硬编码
6. **性能优化** → 首屏体积下降 ≥ 30%，暗色模式零样板代码

---

## 二、UI 库选型

### 2.1 候选库对比

调研了 2025–2026 年 Vue 3 主流组件库，核心数据如下：

| 组件库 | Stars | 周下载 | 体积 | 设计风格 | Nuxt 4 原生适配 | 中文生态 | 上手成本 |
|--------|-------|--------|------|----------|-----------------|----------|----------|
| **Nuxt UI v4** | 5.2k+ | 增长中 | 极小（按需） | 现代极简、语义化 Token | ✅ **官方旗舰** | 良好 | 低 |
| Naive UI | 17.6k | 26k | 36MB | 极简、TS 友好 | ⚠️ 需手动适配 | 优 | 低 |
| Element Plus | 26.4k | 高 | 43MB | 企业中后台、稳重 | ✅ 有 nuxt 模块 | 优 | 低 |
| Ant Design Vue | 21.1k | 高 | 78MB | 蚂蚁设计体系、商务 | ⚠️ 体积大 | 良 | 中 |
| Vuetify 3 | 40.7k | 636k | 61MB | Material Design | ✅ 有 nuxt 模块 | 中 | 中 |
| PrimeVue | 13.2k | 中 | 中 | 企业风、可双模式 | ⚠️ 配置复杂 | 中 | 高 |
| Shadcn-vue | 8k+ | 增长中 | 极小（复制粘贴） | 极简、Tailwind 原生 | ✅ 天然适配 | 中 | 中 |
| Arco Design Vue | 中 | 中 | 中 | 字节现代风 | ⚠️ 需适配 | 良 | 中 |

### 2.2 最终选择：**Nuxt UI v4** ⭐ 推荐

**核心理由：**

1. **官方旗舰，原生 Nuxt 4 适配** —— 同团队维护，与 Nuxt 4 的 SSR、auto-import、runtimeConfig 无缝集成
2. **技术栈前沿** —— 基于 **Reka UI**（无障碍 Headless 内核，WAI-ARIA 满分）+ **Tailwind CSS v4**（CSS-first 配置，构建提速 100x）+ **Tailwind Variants**（类型安全变体系统）
3. **设计语言贴合"情绪陪伴"产品调性** —— 默认极简、留白多、圆角现代、动画细腻；不像 Element Plus 那样"格子化、行政化"
4. **125+ 组件** —— 比 Element Plus 还多，且组件全部可访问性合规（键盘导航、ARIA、Focus 管理）
5. **设计令牌系统完美匹配现有色系** —— 现有项目已有 sage green 调色板（`#6E9387` / `#4F7569` / `#E8F0ED`），Nuxt UI 的 7 色语义系统可直接映射
6. **构建性能大幅提升** —— Tailwind v4 增量构建提速 100x，运行时提速 5x
7. **暗色模式零成本** —— 通过 CSS 变量自动切换，无需手写 `html.dark` 样式
8. **类型安全 + IDE 提示** —— 100% TypeScript，泛型组件 + Tailwind Variants 让 IDE 自动补全所有 props/slots

**对比表（决定性维度）：**

| 维度 | Element Plus（当前） | Nuxt UI v4（重构后） |
|------|---------------------|---------------------|
| 视觉风格 | 中后台、稳重、偏行政 | 现代极简、柔和、留白多 |
| 设计系统 | SCSS 变量 + 手动覆盖 | Tailwind v4 + 7 色语义令牌 |
| 暗色模式 | 手写 N 条 `.dark .xxx` | 一键切换、自动适配 |
| 组件数量 | ~70+ | ~125+ |
| TypeScript | 部分 | 全量、泛型组件 |
| 构建速度 | 普通 | 100x 提速（增量） |
| 包体积 | 43MB | 按需，小 50%+ |
| Nuxt 集成 | 第三方模块 | 官方原生 |
| 移动端 | 需手写响应式 | 内置响应式原语 |

### 2.3 备选方案：**Naive UI**（如团队偏好）

如果出于以下原因不愿使用 Nuxt UI v4（如对 Tailwind 不熟悉），可考虑 **Naive UI** 作为备选：

- 完全 TS，类型提示友好
- 主题用 JSON 配置，类似 SCSS 变量思路（迁移成本低）
- 极简现代风，体积小

但缺点：非 Nuxt 官方，需手动配置 Nuxt 插件；无 Tailwind 生态红利；默认风格仍偏企业。

---

## 三、技术栈迁移方案

### 3.1 依赖变更

**移除：**
```json
{
  "@element-plus/icons-vue": "^2.3.2",
  "@element-plus/nuxt": "^1.1.4",
  "element-plus": "^2.12.0"
}
```

**新增：**
```json
{
  "@nuxt/ui": "^4.0.0",
  "@iconify-json/lucide": "^1.2.0",
  "@iconify-json/mynaui": "^1.2.0",
  "tailwindcss": "^4.0.0",
  "@tailwindcss/vite": "^4.0.0",
  "tailwind-variants": "^0.3.0"
}
```

**保留（核心）：**
```json
{
  "nuxt": "^4.2.1",
  "@pinia/nuxt": "^0.11.3",
  "pinia": "^3.0.4",
  "echarts": "^6.0.0",
  "vue-echarts": "^8.0.1",
  "nuxt-echarts": "1.0.1",
  "@pixiv/three-vrm": "^3.5.2",
  "three": "^0.184.0",
  "@mediapipe/camera_utils": "^0.3.1675466862",
  "@mediapipe/face_mesh": "^0.4.1633559619",
  "dexie": "^4.3.0",
  "marked": "^18.0.3",
  "vue-dompurify-html": "^5.3.0",
  "js-sha256": "^0.11.1"
}
```

### 3.2 Nuxt 配置改造

**`nuxt.config.ts` 关键变更：**

```ts
export default defineNuxtConfig({
  modules: [
    '@nuxt/ui',          // 替换 @element-plus/nuxt
    '@pinia/nuxt',
    '@nuxtjs/device',
    'nuxt-echarts',
    '@nuxtjs/color-mode' // 替换手写主题切换
  ],

  css: ['~/assets/css/main.css'], // 移除 element-plus dark CSS

  ui: {
    colorMode: true  // 自动集成暗色模式
  },

  colorMode: {
    classSuffix: '',
    preference: 'system',
    fallback: 'light'
  },

  vite: {
    plugins: [tailwindcss()], // Tailwind v4 vite 插件
    build: {
      rollupOptions: {
        output: {
          manualChunks(id) {
            if (id.includes('echarts')) return 'echarts';
            if (id.includes('reka-ui')) return 'reka-ui';
            if (id.includes('node_modules')) return 'vendor';
          }
        }
      }
    }
  }
})
```

**`app/assets/css/main.css`（新增）：**

```css
@import "tailwindcss";
@import "@nuxt/ui";

/* 注入 Design Token */
@theme static {
  --color-primary-50:  #E8F0ED;
  --color-primary-100: #C5D8D1;
  --color-primary-200: #A4BFB7;
  --color-primary-300: #8DAEA4;
  --color-primary-400: #7DA094;
  --color-primary-500: #6E9387; /* 主色：沉静墨绿 */
  --color-primary-600: #5C8276;
  --color-primary-700: #4F7569; /* 深色 */
  --color-primary-800: #3F5D54;
  --color-primary-900: #2F4840;

  --color-secondary-500: #B97A6E; /* 暗玫 */

  --font-sans: "PingFang SC", "Microsoft YaHei", sans-serif;
  --font-serif: "STKaiti", "KaiTi", serif;
}
```

### 3.3 设计令牌迁移

将现有 `variables.scss` 的色系迁移到 Nuxt UI 的 `app.config.ts`：

```ts
// app.config.ts
export default defineAppConfig({
  ui: {
    colors: {
      primary: 'sage',        // 映射到上面的 primary 色阶
      secondary: 'rose',
      neutral: 'slate',
      success: 'emerald',
      info: 'sky',
      warning: 'amber',
      error: 'rose'
    },
    button: {
      defaultVariants: { color: 'primary', size: 'md' }
    }
  }
})
```

---

## 四、设计系统重建

> 本节不是"挑个调色板"那么简单。
> 我从 skill `frontend-design` 拿到的核心方法论是：**先有一个 thesis（设计论点），再决定色板、字体、布局，最后只做一次自我批判**。
> 下面是我作为这个项目"设计负责人"写下的判断书。

### 4.0 美学方向（Design Thesis）

#### 4.0.1 产品定位

Emotion-Echo 不是一个普通的聊天产品。它是：

- **品类**：情感陪伴 / 情绪支持类 AI
- **用户**：在国内寻求倾诉、想被"听见"的人群
- **功能关键词**：AI 对话、语音情绪、面部情绪、心理量表、3D 数字人
- **情感任务**：让用户感到"安全、被接纳、可被长期对话"

设计要回答的核心问题：**什么样的视觉语言，能让一个深夜焦虑的人愿意打开这个 App？**

#### 4.0.2 我拒绝的三个 AI 默认答案

调研 `frontend-design` skill 后，明确**不**走以下三种 AI 当前最容易输出的样式（不论这些样式本身好不好，它们都是默认而非选择）：

| 默认样式 | 为什么不适合本产品 |
|----------|---------------------|
| ① 米色 + 高对比衬线 + 陶土橙 | 偏向"工艺 / 美食 / 杂志"，与情绪陪伴无关 |
| ② 近黑 + 酸绿/朱红霓虹 | 偏向"赛博 / 游戏 / 工具"，会让用户紧张而非安心 |
| ③ 报纸 broadsheet（细线 + 0 圆角 + 多栏密排） | 偏向"严肃 / 资讯 / 报告"，与陪伴产品的柔和度冲突 |

#### 4.0.3 我的论点：**宋韵水墨（Song-era Ink Wash）**

> 把"被听见"这件事，翻译成**宋代山水画的留白**——画面的安静来自大面积的不说，少数几笔墨色被认真对待。

具体落地：

- **"玉"为主色**：现有 `沉静墨绿 #6E9387` 本就接近"青玉"，这是宋瓷釉色，不是绿色按钮色
- **"墨"为文字**：深墨 `#1F2329` 而不是纯黑 `#000`，是墨在宣纸上的浓度
- **"宣纸"为底**：偏暖的米白 `#F8F6F1`，但**不是** AI 默认的 `#F4F1EA` —— 我更偏冷一点，避免与所有米色页面撞脸
- **"朱砂"为稀有强调**：暗玫 `#B97A6E` 作为唯一暖色，承担"重要、情绪、未读"
- **"月白"为灰阶**：冷调浅灰 `#E8E8E0`，用于分割而非纯白

这个方向的合法性来自：
1. 项目已用"沉静墨绿"+ 楷体 + 登录页的唐诗，已经在往这个方向走了，我是在**收敛**而非"换方向"
2. 宋画的"计白当黑"（用空白表达）天然适合聊天产品 —— 文字越少越珍贵
3. 中文用户对楷体 + 留白有文化认同，海外用户看到会感到"东方禅意"，差异化明显

#### 4.0.4 Signature 元素：墨晕呼吸（Ink-bloom Breath）

**这个产品要被人记住的那一个东西**：每条 AI 消息气泡的左上角，有一道**不规则的水墨晕染**（用 CSS 渐变 + filter:blur 模拟墨在宣纸上的扩散），它会随消息流式打字而**慢慢洇开**，打字完成时定型。

```
┌─ ··········· ┐
│  ···水墨晕染··  │
│   墨色从一角起，  │
│   随打字渐开。   │
│   完成后定型。   │
└─────────────────┘
```

为什么是它：

- 把"被听见"做成视觉隐喻：墨从一角开始，意思是"你的话被听见了，然后它在我这里慢慢生长"
- 打字过程的渐开 = 思考过程外显
- 没有任何竞品做过这件事（因为它需要 CSS + 动画 + 内容状态深度绑定，门槛高，复制成本高）
- 与"宋韵水墨" thesis 一致

**实现策略（仅作技术占位，详细 Phase 5 落地）**：

```vue
<div class="ink-bubble" :data-state="streaming ? 'growing' : 'final'">
  <span class="ink-bloom" />
  <slot />
</div>

<style>
.ink-bloom {
  background: radial-gradient(
    circle at 0% 0%,
    rgba(110, 147, 135, 0.18) 0%,
    rgba(110, 147, 135, 0.08) 30%,
    transparent 60%
  );
  filter: blur(2px);
  transform-origin: 0 0;
  transition: transform 1.2s cubic-bezier(0.22, 1, 0.36, 1);
}
.ink-bubble[data-state="growing"] .ink-bloom { transform: scale(0.4); }
.ink-bubble[data-state="final"]    .ink-bloom { transform: scale(1); }
</style>
```

#### 4.0.5 把"boldness"花在一处

`spend your boldness in one place`——只让墨晕呼吸做"勇者"，其他地方**克制**：

- 卡片：低对比边框 `border-neutral-200/60`、极轻阴影
- 按钮：1px 边框 + 12px 圆角，不饱和
- 过渡：120-200ms ease-out，无弹跳
- 装饰：禁止用渐变、霓虹、emoji 装饰、玻璃拟态（除签名元素外）
- 数据图表：保留 ECharts，但默认配色全部用"墨色五阶"（从深墨到淡墨），不引彩虹调色板

### 4.1 限定 6 色调色板

不是 7 色 + N 个灰阶的 Tailwind 调色盘。我作为设计师就给你 6 个色，每个有名字、有来历、有分工：

| 中文名 | 英文 token | Hex | 角色 |
|--------|------------|------|------|
| 沉静墨绿 | `--jade` | `#6E9387` | 唯一品牌色。主按钮、链接、激活态、Signature 墨晕 |
| 深墨 | `--ink` | `#1F2329` | 文字、深色按钮、强调 |
| 宣纸 | `--paper` | `#F8F6F1` | 页面底色、卡片底色（**不是纯白**） |
| 月白 | `--moonwhite` | `#E8E8E0` | 描边、分割、Tag 底 |
| 朱砂 | `--cinnabar` | `#B97A6E` | 唯一暖色强调：未读、错误提示、情绪高亮 |
| 淡墨 | `--ash` | `#646A73` | 次级文字、placeholder |

**暗色模式：**

| 名 | Hex | 角色 |
|----|------|------|
| 暗夜 | `#1A1F26` | 页面底色（不是 `#000`） |
| 浅墨 | `#2C333D` | 卡片底色 |
| 寒玉 | `#A8C7BD` | jade 在暗色下的提亮版 |
| 残墨 | `#E5E6EB` | 主文字 |
| 残月 | `#8F959E` | 次级文字 |
| 暗朱 | `#C99086` | cinnabar 在暗色下的提亮版 |

**色板规则：**
- 全站只允许 6+6 这 12 个色值出现，其余一律派生
- 不准使用 Tailwind 默认的 `red-500`、`blue-500` 等饱和色（除图表数据可视化时由 ECharts 自行取色）
- 图表用 5 阶"墨色梯度"（`#1F2329 / #646A73 / #8F959E / #B4B9C0 / #D9DCE0`）做主色，cinnabar 留给"重要维度"

### 4.2 字体配对（有性格，不随手抓）

不写"用 sans-serif + serif"这种废话。具体到字族与下载方式：

| 角色 | 字体 | 来源 | 用法 |
|------|------|------|------|
| **Display** | **霞鹜文楷 LXGW WenKai** | GitHub 开源（已支持 CDN / self-host） | 页面 H1、欢迎语、楷体装饰、登录页诗句 |
| **Body** | **思源黑体 SC** Noto Sans SC | Google Fonts / Adobe | 正文、按钮、表单 |
| **Serif fallback** | **思源宋体 SC** Noto Serif SC | Google Fonts | 报表标题等需要"分量"的地方 |
| **Mono** | **JetBrains Mono** | Google Fonts | 代码块、技术信息、ID |
| **Brush（限定场景）** | 现项目自带的 `LuoGuoChengMaoBiXiaoXingJianTi-2.ttf` | 现 public/fonts/ | 仅在登录页 mask 与首页品牌签名用，**不外扩** |

**类型尺度（清晰、有意为之）：**

```
display-2xl  64px / 1.1   LXGW WenKai, -2% letter-spacing   // 登录页主标题
display-xl   48px / 1.15  LXGW WenKai, -1%                  // 报表大数字
display-lg   36px / 1.2   LXGW WenKai                       // 页面 H1
h1           28px / 1.3   LXGW WenKai                       // 区块标题
h2           22px / 1.4   Noto Sans SC, 600
h3           18px / 1.5   Noto Sans SC, 600
body         16px / 1.7   Noto Sans SC                      // 正文行高调到 1.7，给文字呼吸
body-sm      14px / 1.6   Noto Sans SC
caption      12px / 1.5   Noto Sans SC, 500
```

> 把正文行高从 1.5 提到 1.7 是个**有意识的小改动** —— 情绪文本需要呼吸感，太挤会让"被听见"变成"被压着"。

### 4.3 间距、圆角、阴影（少而准）

不写 Tailwind 默认全套，只声明本项目"非默认"的几条：

```css
--space: 4 / 8 / 12 / 16 / 24 / 32 / 48 / 64 / 96;  /* 默认 */
--space-mood: 80 120 160;                            /* 情绪留白：用于登录页、报表大数字 */

--radius-sm: 4px;    /* 极小元素：Tag、Badge */
--radius-md: 8px;    /* 按钮、输入框 */
--radius-lg: 12px;   /* 卡片、Dialog */
--radius-xl: 16px;   /* 强调卡片 */
--radius-2xl: 24px;  /* 登录页主卡片 */
--radius-pill: 9999px;  /* 仅用于"发送"按钮、Tag */
```

> **改条规则：默认圆角从 6px 升到 8px/12px**。原项目大量用 6-8px 太尖锐，与"陪伴感"相违；新设计更"现代+柔和"。

```css
--shadow-soft: 0 1px 2px rgba(31, 35, 41, 0.04), 0 2px 8px rgba(31, 35, 41, 0.04);
--shadow-lift: 0 4px 12px rgba(31, 35, 41, 0.06), 0 12px 24px rgba(31, 35, 41, 0.06);
--shadow-ink:  0 1px 0 rgba(31, 35, 41, 0.08);    /* "墨色 1px 投影"，用于卡片底部，模拟纸的厚度 */
```

> 阴影里加了 `0 1px 0 rgba(...)`，模拟"宣纸漂浮在桌面上 1 像素" —— 是宋画装裱的暗示，绝大多数 UI 库不会这么用。

### 4.4 动效规范（克制，不炫技）

```
duration-fast:  120ms    /* hover、focus */
duration-base:  200ms    /* 状态切换 */
duration-slow:  400ms    /* 弹窗、Drawer */

ease:           cubic-bezier(0.22, 1, 0.36, 1)   /* 整体用"缓出"，无弹跳 */
ease-ink:       cubic-bezier(0.65, 0, 0.35, 1)    /* 仅墨晕呼吸用，慢进慢出 */

不允许：
- 弹跳（bounce）
- 视差滚动
- 多于 2 个元素同时进入动画
- 超过 500ms 的等待
```

### 4.5 自我批判（Critique）

按 `frontend-design` skill 的要求，写之前要**问自己**：这套设计是不是任何情绪类聊天产品都能用？

**测试方法：把品牌名遮住，看页面。**

- "Bumble 心碎治愈版"也能用这版吗？**能**。→ 这部分有通用性，OK
- "Notion 极简版"也能用吗？**不能**（因为我有墨晕呼吸、霞鹜文楷、朱砂强调）。→ 差异化成立
- 还有什么 AI 容易撞脸？**可能风险**：我的 宣纸色 `#F8F6F1` 接近 cream 默认，**补救**：所有"宣纸"色卡片必须叠加 0.3% 的噪点 SVG 纹理，破除"廉价米色"感

**再问一个**：除了"墨晕呼吸"以外的"勇者"在哪里？

- 字体选择霞鹜文楷（**勇者**：开源中文字体里不多见的"有温度的衬线"）
- 暗色模式背景用 `#1A1F26` 而不是 `#000`（**勇者**：让暗色也有"夜空"而非"无光"）
- 1px 墨色投影（**勇者**：几乎没人这么写 shadow）

够了。一处大勇 + 三处小勇，符合 skill "spend your boldness in one place" 的建议。

### 4.7 文案语气（Voice & Tone）

`frontend-design` skill 强调：**Words are design material, not decoration.**

Emotion-Echo 的语气要避开"工具感"，走向"人感"。下表是原项目文案与重写后文案的对比：

| 场景 | 原文案（太工具） | 重写后（有人味） |
|------|-----------------|------------------|
| 输入框 placeholder | "请输入想倾诉的内容，按发送按钮提交..." | "想说点什么？" |
| 欢迎语 | "你好啊，让我们开始聊天吧" | "在吗？我在听。" |
| 加载 | "加载中..." | "稍等，让我想想..." |
| 错误 | "获取日报失败" | "日报还没准备好，等会儿再试？" |
| 空状态 | "暂无数据" | "还没有数据，等你开始记录" |
| 删除确认 | "删除后将不可恢复!" | "这条对话会消失，你确定吗？" |
| 退出登录 | "是否退出登录？" | "要离开一会儿吗？" |
| 验证码发送 | "验证码已发送，请注意查收" | "验证码已送出，看看短信" |
| 提交成功 | "信息修改成功！" | "改好了" |
| 摄像头开启失败 | "摄像头开启失败" | "没能打开摄像头，先检查一下权限？" |
| 语音上传失败 | "语音上传失败" | "录音没传上去，再试一次？" |
| 答题提交 | "答题提交成功！" | "收到，让我看看..." |
| 心理测验标题 | "心理测验量表" | "认识自己的几个小问题" |
| 心理测验描述 | "我们将根据您的答题情况分析您的心理状况" | "花几分钟，更懂自己一点" |

**三条原则**：
1. **主语是"我/你"，不是"系统/用户"** —— 系统说"获取失败"，人不说"获取失败"，人问"你还好吗？"
2. **动词具体，不卖弄** —— "改好了" 比 "修改成功" 更短、更准确、更有温度
3. **失败也是对话** —— 不说"网络错误 500"，说"刚才信号不好，再试一次？"

### 4.6 落地：Nuxt UI + Tailwind v4 配置

```ts
// app.config.ts
export default defineAppConfig({
  ui: {
    colors: {
      primary: 'jade',     // 6+6 色板里的主色
      secondary: 'cinnabar',
      neutral: 'ash',
      success: 'jade',      // 成功也用 jade 系，不引入 emerald
      info: 'ink',
      warning: 'cinnabar',  // 警告也用 cinnabar，不引入 amber
      error: 'cinnabar'
    }
  }
})
```

```css
/* assets/css/main.css */
@import "tailwindcss";
@import "@nuxt/ui";

@theme static {
  --color-jade-50:  #F2F7F4;
  --color-jade-100: #E8F0ED;
  --color-jade-200: #C5D8D1;
  --color-jade-300: #A4BFB7;
  --color-jade-400: #8DAEA4;
  --color-jade-500: #6E9387;  /* 主色 */
  --color-jade-600: #5C8276;
  --color-jade-700: #4F7569;
  --color-jade-800: #3F5D54;
  --color-jade-900: #2F4840;

  --color-cinnabar-500: #B97A6E;
  --color-cinnabar-700: #94594E;

  --color-ink-500: #1F2329;
  --color-ink-700: #13171C;

  --color-paper:    #F8F6F1;
  --color-moonwhite: #E8E8E0;
  --color-ash-500:  #646A73;
  --color-ash-700:  #4A4F56;

  --font-display: "LXGW WenKai", "KaiTi", "STKaiti", serif;
  --font-sans:    "Noto Sans SC", -apple-system, BlinkMacSystemFont, sans-serif;
  --font-serif:   "Noto Serif SC", "Songti SC", serif;
  --font-mono:    "JetBrains Mono", "Fira Code", monospace;
}
```

---

## 五、组件库映射表（Element Plus → Nuxt UI）

> 这是重构的核心工作量表，逐个组件替换。

| Element Plus 组件 | Nuxt UI v4 对应 | 备注 |
|-------------------|----------------|------|
| `el-button` | `UButton` | API 几乎一致，添加 `variant`/`color` 语义 |
| `el-input` | `UInput` | 支持 `type="textarea"` |
| `el-form` + `el-form-item` + `:rules` | `UForm` + `UFormField` + `schema` | 改为 Schema 校验（valibot/zod），更 TS 友好 |
| `el-dialog` | `UModal` | 支持 `v-model:open` |
| `el-message-box` | `UModal` + `useConfirm` | 用 `useToast` 替代部分通知 |
| `el-notification` | `useToast` | 全局调用方式一致 |
| `el-message` | `useToast` | 同上 |
| `el-table` | `UTable` | 支持虚拟滚动 |
| `el-radio-group` + `el-radio` | `URadioGroup` + `URadio` | 或用 `USelect` 替代 |
| `el-radio-button` | `UTabs` 或 `UToggleGroup` | 主题切换、视图切换场景 |
| `el-checkbox` | `UCheckbox` | 一致 |
| `el-dropdown` + `el-dropdown-menu` | `UDropdownMenu` | 基于 Reka UI，键盘导航更好 |
| `el-menu` + `el-menu-item` + `el-sub-menu` | `UNavigationMenu` 或自定义 `UAside` | 重构侧边栏 |
| `el-steps` + `el-step` | 自定义 Stepper（基于 `UProgress` 或 `UBreadcrumb`） | 找回密码流程 |
| `el-date-picker` | `UCalendar` 或 `@internationalized/date` | 自定义日期范围选择 |
| `el-empty` | `UEmpty` | 一致 |
| `el-skeleton` | `USkeleton` | 一致 |
| `el-divider` | `UDivider` | 一致 |
| `el-icon` | `UIcon`（Iconify） | 内置 20 万+ 图标 |
| `el-avatar` | `UAvatar` | 一致 |
| `el-upload` | 自定义或 `UFileUpload`（待 Nuxt UI 后续完善） | 头像上传场景需手写 |
| `el-card` | `UCard` | 一致 |
| `el-tag` | `UBadge` | 语义更明确 |
| `@element-plus/icons-vue` | `@iconify-json/lucide` + `@iconify-json/mynaui` | 风格更现代 |

### 5.1 表单校验方案升级（重大变更）

**旧方案（el-form）：**
```vue
<el-form :rules="rules" :model="form" ref="formRef">
  <el-form-item label="账号" prop="username">
    <el-input v-model="form.username" />
  </el-form-item>
</el-form>
```

**新方案（UForm + valibot）：**
```vue
<script setup lang="ts">
import * as v from 'valibot'

const schema = v.object({
  username: v.pipe(v.string(), v.minLength(1, '请输入账号')),
  password: v.pipe(v.string(), v.regex(/^(?=.*[a-zA-Z])(?=.*\d).{6,18}$/, '6-18位含字母数字'))
})

const state = reactive({ username: '', password: '' })
</script>

<template>
  <UForm :schema="schema" :state="state" @submit="onSubmit">
    <UFormField label="账号" name="username">
      <UInput v-model="state.username" />
    </UFormField>
    <UButton type="submit">提交</UButton>
  </UForm>
</template>
```

**优势：** 完全 TS 类型推导、零样板、自动滚动到第一个错误字段。

---

## 六、路由重构（更合理、更清晰）

### 6.1 当前路由问题

1. **`/chat/conversation` 与 `/chat/conversation/:id` 混在一起**：列表页和详情页共用组件，逻辑耦合
2. **`/chat/user` 与 `/chat/setting` 是同级**：但与 `dashboard` 视觉层级关系不清
3. **忘记密码用 `child route` 但子页面用 `@changeActive` 事件通信**：父子通信别扭
4. **`/chat/dashboard` 中 `index.vue` 只是个壳**：没意义的多一层
5. **`/question` 独立于 `/chat`**：但实际属于登录后功能，被 auth 中间件覆盖不一致

### 6.2 重构后的路由结构

```
/                           → 重定向到 /chat（已登录）或 /login（未登录）
│
├── /login                  → 公开
│   ├── (default)           登录页（双栏卡片布局）
│   └── /forget             忘记密码流程（步骤条布局）
│       ├── /verify
│       ├── /reset
│       └── /success
│
├── /chat                   → 主应用壳（需登录，layout=app）
│   ├── /conversation       → 会话（侧边栏 + 内容区布局）
│   │   ├── (默认)          → 重定向到 /conversation/new
│   │   ├── /new
│   │   └── /:id
│   ├── /dashboard          → 统计
│   │   ├── /daily
│   │   ├── /weekly
│   │   ├── /monthly
│   │   └── /yearly
│   ├── /profile            → 用户中心（合并 user + 部分 setting）
│   └── /settings           → 设置
│
└── /assessment             → 心理测验（需登录，layout=app）
    ├── (default)           量表列表
    └── /:id                答题 / 结果
```

### 6.3 重构后的 `router.options.ts`（草案）

```ts
export default {
  routes: (_routes) => [
    {
      path: '/',
      name: 'root',
      redirect: { name: 'chat-conversations' }
    },

    // 公开
    {
      path: '/login',
      name: 'login',
      component: () => import('~/pages/auth/login.vue'),
      meta: { layout: 'auth', public: true }
    },
    {
      path: '/login/forget',
      component: () => import('~/pages/auth/forget.vue'),
      meta: { layout: 'auth', public: true },
      children: [
        { path: '', name: 'login-forget', redirect: { name: 'login-forget-verify' } },
        { path: 'verify', name: 'login-forget-verify', component: () => import('~/pages/auth/forget/verify.vue') },
        { path: 'reset',  name: 'login-forget-reset',  component: () => import('~/pages/auth/forget/reset.vue') },
        { path: 'done',   name: 'login-forget-done',   component: () => import('~/pages/auth/forget/done.vue') }
      ]
    },

    // 主应用（需登录）
    {
      path: '/chat',
      component: () => import('~/layouts/app.vue'),
      meta: { requiresAuth: true },
      children: [
        {
          path: 'conversation',
          name: 'chat-conversations',
          component: () => import('~/pages/chat/conversations.vue'),
          children: [
            { path: '', redirect: { name: 'chat-conversation-new' } },
            { path: 'new', name: 'chat-conversation-new', component: () => import('~/pages/chat/conversation/new.vue') },
            { path: ':id', name: 'chat-conversation-id',  component: () => import('~/pages/chat/conversation/[id].vue'), props: true }
          ]
        },
        {
          path: 'dashboard',
          name: 'chat-dashboard',
          component: () => import('~/pages/chat/dashboard.vue'),
          children: [
            { path: '',         redirect: { name: 'chat-dashboard-daily' } },
            { path: 'daily',    name: 'chat-dashboard-daily',    component: () => import('~/pages/chat/dashboard/daily.vue') },
            { path: 'weekly',   name: 'chat-dashboard-weekly',   component: () => import('~/pages/chat/dashboard/weekly.vue') },
            { path: 'monthly',  name: 'chat-dashboard-monthly',  component: () => import('~/pages/chat/dashboard/monthly.vue') },
            { path: 'yearly',   name: 'chat-dashboard-yearly',   component: () => import('~/pages/chat/dashboard/yearly.vue') }
          ]
        },
        {
          path: 'profile',
          name: 'chat-profile',
          component: () => import('~/pages/chat/profile.vue')
        },
        {
          path: 'settings',
          name: 'chat-settings',
          component: () => import('~/pages/chat/settings.vue')
        }
      ]
    },

    // 心理测验
    {
      path: '/assessment',
      meta: { requiresAuth: true, layout: 'app' },
      children: [
        { path: '',         name: 'assessment-list',  component: () => import('~/pages/assessment/index.vue') },
        { path: ':id',      name: 'assessment-take',  component: () => import('~/pages/assessment/[id].vue'), props: true }
      ]
    }
  ]
}
```

### 6.4 路由重构要点

| 改动 | 说明 |
|------|------|
| 抽出 `app.vue` 布局 | 把"侧边栏 + 顶栏 + 主区域"抽到 layouts，业务页面只关注内容 |
| 命名更规整 | `chat-conversations`（复数 = 列表），`chat-conversation-id`（带 id = 详情） |
| 合并 `user` → `profile` | 与行业惯例对齐（GitHub、Twitter 都用 Profile） |
| 删除 `dashboard/index.vue` 壳 | 改为 `dashboard.vue` 父布局组件，子路由直接挂载 |
| 忘记密码独立命名空间 | 路由都在 `/login/forget/*` 下，命名清晰 |
| 心理测验独立 | 业务独立，从 `/chat` 拆出，避免污染 |

---

## 七、页面重构方案（保留所有功能，仅视觉/结构升级）

### 7.1 登录页（`/login`）

**旧：** `app/pages/login/index.vue` 一文件包含 PC mask + 移动端表单双套代码。

**新：** 拆分为
- `app/pages/auth/login.vue`（容器）
- `app/components/auth/LoginCard.vue`（双栏卡片，PC 显示）
- `app/components/auth/LoginFormMobile.vue`（移动端表单）
- 用 Nuxt UI `UModal` 实现切换动画

**视觉：** 左侧大幅欢迎语（楷体）+ 右侧表单卡片，毛玻璃背景，圆角 16px，淡入动画。

### 7.2 忘记密码（`/login/forget/*`）

**旧：** `el-steps` + 自定义步骤条逻辑。

**新：**
- 顶部 `UBreadcrumb` 或自绘步骤指示器（3 个圆点 + 连接线）
- 中间表单 `UForm + UFormField + valibot`
- 步骤间通过 `useState('forget-flow', ...)` 共享状态，不再用 emit
- 路由命名：`verify / reset / done`（替换 `modify / success`）

### 7.3 聊天主区域（`/chat/conversation/:id`）

**这是整个产品的心脏。视觉设计完全围绕"宋韵水墨"thesis 展开。**

#### 布局骨架

```
┌──────────────────────────────────────────────────────────┐
│  ← 会话标题（霞鹜文楷）          ⌘ Esc 全屏 ⓘ 会话信息   │  ← 极简顶栏
├──────────────────────────────────────────────────────────┤
│                                                          │
│   （上方 80px 留白 —— "计白当黑"）                      │
│                                                          │
│   ┌─[ 墨晕 ]··········┐                                  │
│   │  AI · 1.2s ago     │  ← 极淡灰 meta                │
│   │                    │                                │
│   │  嗯，我听着呢。    │  ← LXGW WenKai, 行高 1.85     │
│   │  你想从哪里开始？  │    字号 17px, 墨色 1F2329       │
│   └────────────────────┘  ← 12px 圆角 + 1px 墨色 0.08 阴影 │
│                                                          │
│                       ┌─────────────────────────────┐   │
│                       │ 我最近总是睡不着      · Me │   │
│                       └─────────────────────────────┘   │
│                                                          │
│   （中间自动留白，AI 回答之间至少 32px 间距）             │
│                                                          │
├──────────────────────────────────────────────────────────┤
│  ⊕ 附件  😊  麦克风  📷 面部  ━━━━━━━━━━━━━━━━━━━  发送  │  ← 输入栏
└──────────────────────────────────────────────────────────┘
```

#### AI 消息气泡：**墨晕呼吸（Ink-bloom Breath）**

每条 AI 消息气泡的左上角内嵌一道**水墨晕染**，随流式打字进度洇开：

```vue
<!-- components/chat/MessageBubble.vue -->
<template>
  <article
    class="ink-bubble group"
    :data-state="status === 'streaming' ? 'growing' : 'final'"
  >
    <span class="ink-bloom" :style="bloomVars" />
    <header class="ink-meta">
      <span class="role">AI</span>
      <span class="dot">·</span>
      <time>{{ relativeTime }}</time>
    </header>
    <div class="ink-content" v-dompurify-html="html" />
  </article>
</template>

<style scoped>
.ink-bubble {
  position: relative;
  padding: 16px 20px 16px 24px;       /* 左侧多 8px 给墨晕 */
  background: #FFFFFF;                 /* 气泡白 */
  border: 1px solid rgba(31, 35, 41, 0.06);
  border-radius: 12px;
  box-shadow:
    0 1px 0 rgba(31, 35, 41, 0.08),   /* 1px 墨色底阴影 */
    0 2px 8px rgba(31, 35, 41, 0.04);
  max-width: 70%;
  font-family: "LXGW WenKai", "KaiTi", serif;
  font-size: 17px;
  line-height: 1.85;                   /* 比正文 1.7 还宽，给情绪文本呼吸 */
  color: var(--color-ink-500);
}

.ink-bloom {
  position: absolute;
  top: 0; left: 0;
  width: 56px; height: 56px;
  background: radial-gradient(
    circle at 0% 0%,
    rgba(110, 147, 135, 0.22) 0%,    /* 沉静墨绿 */
    rgba(110, 147, 135, 0.10) 35%,
    transparent 65%
  );
  filter: blur(2px);
  transform-origin: 0 0;
  transition: transform 1.2s cubic-bezier(0.65, 0, 0.35, 1),
              opacity 0.8s ease-out;
  pointer-events: none;
  border-top-left-radius: 12px;
}

/* 打字时墨晕收成 0.3，完成后绽开为 1 */
.ink-bubble[data-state="growing"] .ink-bloom { transform: scale(0.3); opacity: 0.6; }
.ink-bubble[data-state="final"]    .ink-bloom { transform: scale(1);   opacity: 1; }

html.dark .ink-bubble {
  background: #2C333D;                 /* 浅墨 */
  border-color: rgba(229, 230, 235, 0.08);
  color: var(--color-ink-dark);
}
</style>
```

**为什么这个设计成立**：
- 墨色从气泡左上角"长出来"——你的话被"听见了"，然后在 AI 这里慢慢洇开
- 打字过程渐开 = AI 思考过程的外显（用户能"看到"AI 在想）
- 完成时定型 = 句号感
- 暗色模式下墨晕自动适配（用 CSS 变量换色）
- 不依赖任何外部资源，纯 CSS，性能几乎零成本

#### 用户消息气泡

故意做得**克制**——这是 skill 说的"把 boldness 花在一处"：

```vue
<article class="user-bubble">
  {{ content }}
</article>

<style>
.user-bubble {
  padding: 12px 16px;
  background: var(--color-jade-500);   /* 沉静墨绿实色 */
  color: #F8F6F1;                      /* 宣纸色文字，保证对比度 */
  border-radius: 12px 12px 4px 12px;   /* 左下角切一个角 = 听觉"接住" */
  font-family: "Noto Sans SC", sans-serif;
  font-size: 16px;
  line-height: 1.7;
  max-width: 65%;
  /* 不用阴影 —— 墨色阴影是 AI 的特权 */
}
</style>
```

> 用户气泡故意**不**用墨晕、不用阴影，因为"用户在说话"这件事不该比"AI 在听"更显眼。这是隐喻：被听见的一方更有份量。

#### 输入栏 `ComposerBar.vue`

```vue
<template>
  <footer class="composer">
    <div class="composer-toolbar">
      <UButton icon="i-lucide-paperclip" variant="ghost" color="neutral" @click="attach" />
      <UButton icon="i-lucide-mic"        variant="ghost" :color="recording ? 'cinnabar' : 'neutral'" />
      <UButton icon="i-lucide-camera"     variant="ghost" :color="cameraOn ? 'jade' : 'neutral'" />
    </div>
    <UTextarea
      v-model="text"
      :rows="3"
      :maxlength="2000"
      placeholder="想说点什么？"
      autoresize
      class="composer-input"
    />
    <UButton
      :icon="streaming ? 'i-lucide-square' : 'i-lucide-arrow-up'"
      :color="streaming ? 'cinnabar' : 'jade'"
      variant="solid"
      shape="pill"
      :disabled="!text.trim() && !streaming"
      @click="onSend"
    />
  </footer>
</template>
```

样式要点：
- 圆角 16px（不再是 32px pill）
- 背景 `--color-paper`（宣纸色），不刺眼
- placeholder 改为"想说点什么？" —— 比"请输入想倾诉的内容"温柔 80%

#### 数字人 / 面部 / 语音组件

**保留三个组件的内部逻辑不动**（VRM 模型、MediaPipe、Recorder），但把它们的外观统一为 Nuxt UI 风格：
- 控制按钮改用 `UButton variant="ghost"` + Iconify 图标
- 浮窗背景从半透明黑 → 宣纸色 + 0.8 透明度
- 数字人加载失败时的"❌"表情去掉，改用 `<UIcon name="i-lucide-alert-triangle" />`

**功能保留：** 语音录制、面部情绪、文件上传、TTS 流播放、Markdown 渲染、表情解析、流式中断、滚动到底部。

### 7.4 新建会话（`/chat/conversation/new`）

**视觉：** 居中标题（楷体大字号，毛笔风） + 输入卡片，加载态用 `USkeleton`，空状态用 `UEmpty`。

### 7.5 用户中心（`/chat/profile`，原 `user`）

**结构：**
- 顶部：用户卡片（头像、昵称、ID）—— 用 `UCard` + `UAvatar`
- 中间：行为图表（昼夜模式饼图、对话频次折线、互动深度柱图）—— 保留 ECharts，但用 `UCard` 包装
- 底部：操作区（修改信息 / 退出登录 / 做心理测验）—— 用 `UButton` 列表或 `USettingsList`

**弹窗：** 修改信息用 `UModal`，表单用 `UForm + valibot`。

### 7.6 设置（`/chat/settings`）

**视觉：** 用 `USettingsGroup` 或自绘卡片列表。
- 字体大小选择：`UToggleGroup`（小/中/大）
- 主题选择：`URadioGroup`（浅色/深色/跟随系统）—— 同时改用 `@nuxtjs/color-mode`

### 7.7 统计报表（`/chat/dashboard/*`）

**结构：** 顶部 `UTabs` 切换日/周/月/年 + 下方统一图表卡片。

**日/周/月/年：**
- 日期选择：自定义日期选择器（基于 `@internationalized/date` + Nuxt UI `UCalendar`）
- 摘要卡片：`UCard` 内展示 summary + 统计数字
- 图表区：保留 `chartCard` 组件，内部用 `UTabs` 切换不同图表类型

**意图分布：** 新增 `UAccordion` 折叠展示，避免一次性渲染过多饼图。

### 7.8 心理测验（`/assessment`）

**列表页：** 改用 `UCard` 网格 + `UBadge` 状态标识，替代 `el-table`。

**答题页：**
- 顶部进度条 `UProgress`
- 题目卡片 `UCard`，单选 `URadioGroup`
- 提交按钮置底固定

**结果：** `UModal` 弹窗 + `UCard` 内容展示。

---

## 八、组件目录重组

### 8.1 新结构

```
app/
├── components/
│   ├── auth/                # 登录、注册、忘记密码
│   │   ├── LoginCard.vue
│   │   ├── ForgetStepIndicator.vue
│   │   └── OAuthButtons.vue
│   ├── chat/                # 聊天相关
│   │   ├── MessageBubble.vue        # 替代 [id].vue 内的 v-if 分支
│   │   ├── MessageList.vue          # 滚动容器
│   │   ├── ComposerBar.vue          # 输入条（语音、表情、附件）
│   │   ├── ConversationSidebar.vue  # 会话侧边栏
│   │   └── ConversationItem.vue     # 单条会话
│   ├── digital-human/       # 保留
│   │   └── DigitalHuman.vue
│   ├── face/                # 保留
│   │   └── FaceCamera.vue
│   ├── voice/               # 保留
│   │   ├── VoiceMessage.vue
│   │   └── VoiceRecorder.vue
│   ├── charts/              # ECharts 封装
│   │   ├── BaseChart.vue
│   │   ├── BarChart.vue
│   │   ├── LineChart.vue
│   │   ├── PieChart.vue
│   │   └── RadarChart.vue
│   ├── report/
│   │   ├── ChartsCard.vue
│   │   └── ReportSummary.vue
│   ├── layout/              # 布局
│   │   ├── AppShell.vue            # 侧边栏 + 顶栏
│   │   ├── AppSidebar.vue
│   │   ├── AppHeader.vue
│   │   └── AuthLayout.vue          # 登录专用布局
│   └── ui/                  # 通用业务组件
│       ├── EmptyState.vue
│       ├── PageHeader.vue
│       └── StatCard.vue
```

### 8.2 拆分原则

| 原则 | 说明 |
|------|------|
| **单一职责** | 一个组件只做一件事（如 `MessageBubble` 只渲染一条消息） |
| **业务组件 vs 通用组件** | `ui/` 是无业务依赖的纯展示组件，`chat/` 等是有业务语义的组件 |
| **页面级组件就近** | 强页面依赖的组件可放在 `pages/<page>/_components/`（Nuxt 4 支持） |

---

## 九、布局系统（替换 `default.vue` + `nav.vue`）

### 9.1 旧布局问题

- `nav.vue` 用 `el-menu` 实现侧边栏，移动端变水平导航条，但样式很丑
- 每个页面都要处理 `chat-page-container` / `chat-page-container-mobile`
- 没有统一的"页面壳"概念

### 9.2 新布局系统

**`app/layouts/default.vue`（重定向壳）：**
```vue
<template>
  <slot />
</template>
```

**`app/layouts/app.vue`（主应用壳）：**
```vue
<template>
  <div class="flex h-screen bg-neutral-50 dark:bg-neutral-950">
    <AppSidebar />
    <main class="flex-1 overflow-hidden">
      <slot />
    </main>
  </div>
</template>
```

**`app/layouts/auth.vue`（认证布局）：**
```vue
<template>
  <div class="min-h-screen flex items-center justify-center
              bg-gradient-to-br from-primary-50 to-primary-100
              dark:from-neutral-950 dark:to-neutral-900">
    <slot />
  </div>
</template>
```

### 9.3 侧边栏组件 `AppSidebar.vue`

```vue
<template>
  <UNavigationMenu
    orientation="vertical"
    :items="items"
    class="w-64 h-full border-r border-neutral-200 dark:border-neutral-800"
  />
</template>

<script setup lang="ts">
const items = [
  { label: '对话',   icon: 'i-lucide-message-circle',  to: '/chat/conversation' },
  { label: '统计',   icon: 'i-lucide-bar-chart-3',     to: '/chat/dashboard' },
  { label: '测验',   icon: 'i-lucide-clipboard-list',  to: '/assessment' },
  { label: '我的',   icon: 'i-lucide-user',            to: '/chat/profile' },
  { label: '设置',   icon: 'i-lucide-settings',        to: '/chat/settings' }
]
</script>
```

---

## 十、暗色模式重构（零样板）

### 10.1 旧方案（手写 N 条规则）

```scss
html.dark {
  .chart-wrapper { background-color: #1a1a1a !important; }
  .summary-card { background-color: #1a1a1a !important; }
  .setting-item { background-color: #1a1a1a !important; }
  /* ... 几百行 */
}
```

### 10.2 新方案（基于 Token + Tailwind）

- 组件中 **不写具体颜色**，全部用语义化 class（`bg-default`, `text-muted`, `border-default`）
- `@nuxtjs/color-mode` 自动给 `<html>` 加 `light` / `dark` class
- Nuxt UI 的 Tailwind Variants 自动响应暗色

```vue
<!-- 重构前 -->
<div style="background: #fff; color: #333;">

<!-- 重构后 -->
<div class="bg-default text-highlighted">
```

---

## 十一、实施步骤（按 TDD 节奏）

> 项目遵循 `AGENTS.md` 的 **TDD 强制约束**，每一步都必须先写测试/迁移校验、再写实现。

### Phase 1：基础设施搭建（不破坏现有）

1. 安装 Nuxt UI v4 + Tailwind v4（保留 Element Plus 不删）
2. 配置 `tailwind.config` + `app.config.ts` 设计令牌
3. 写一个 `composables/useTheme.ts` 桥接旧 `userConfig.theme` 与 Nuxt UI color-mode
4. **测试：** 写主题切换的 unit test，确保切换不影响 Element Plus 现有样式

### Phase 2：路由重构（先空壳）

1. 重写 `router.options.ts`
2. 新建空页面文件（如 `pages/chat/conversations.vue`）
3. 在旧页面加 `definePageMeta({ redirect: '/chat/conversations' })` 兼容
4. **测试：** E2E 跑通所有旧 URL 都跳到新 URL

### Phase 3：布局与公共组件

1. 抽出 `AppShell`、`AppSidebar`、`AppHeader`
2. 复用 `chartCard`、`charts/*`（不依赖 Element Plus）
3. **测试：** 写组件渲染测试（vitest + @vue/test-utils），snapshot 旧 UI 不变

### Phase 4：登录 / 忘记密码迁移

1. 迁移 `LoginCard` → Nuxt UI
2. 迁移 `ForgetStepIndicator`
3. 删除 `el-form` / `el-input` / `el-button`，替换为 `UForm` / `UInput` / `UButton`
4. **测试：** 跑表单校验单测，确保验证规则等价

### Phase 5：聊天主区域

1. 抽出 `MessageBubble` 组件
2. 迁移输入栏 `ComposerBar`
3. 迁移 `ConversationSidebar`
4. 数字人 / 面部 / 语音组件**保留**（它们不依赖 Element Plus）
5. **测试：** 跑流式消息单测，确保打字机、停止、中断行为不变

### Phase 6：用户中心 / 设置 / 报表 / 测验

1. 逐页迁移，每页先写"前后视觉对比"截图
2. **测试：** E2E（Playwright）覆盖关键路径：登录 → 发起会话 → 收到回复 → 提交问卷 → 查看结果

### Phase 7：清理与优化

1. 删除 `@element-plus/nuxt` 依赖
2. 删除所有 `el-*` / `:deep(.el-*)` 残留
3. 删除 `global.scss` 中的 `html.dark .xxx` 大量规则
4. 重新打包，分析 chunk 体积

### Phase 8：回归与发布

1. 全量测试 `pnpm test`
2. Lighthouse 跑一遍，目标 Performance ≥ 90
3. 暗黑模式 / 移动端回归
4. README 更新、CHANGELOG 撰写

---

## 十二、风险与对策

| 风险 | 影响 | 对策 |
|------|------|------|
| Nuxt UI v4 较新，部分高级组件（如复杂日期选择）需自写 | 工作量 +10% | 用 `@internationalized/date` 自行封装 |
| 表单校验从 options 改 schema，开发习惯迁移 | 学习成本 | 提供封装 `composables/useFormSchema.ts` 减小改动 |
| 暗色模式自动切换可能与原 `userConfig.theme` 冲突 | 状态不同步 | 用 `useTheme` 桥接，persistence 走 color-mode |
| Tailwind v4 与 SCSS 共存 | 构建复杂度 | 渐进迁移，业务组件不再写 `<style lang="scss">`，改用 Tailwind class |
| 路由重构可能影响现有 deep link / 书签 | 用户体验 | 提供 `legacyRedirects` 兼容旧 URL |
| 数字人 / 面部 / 语音组件视觉需同步换风格 | 设计不统一 | 这三个组件 UI 部分重制为 Nuxt UI 按钮，但内部逻辑不动 |
| 包体积首次反而可能变大（双组件库共存期） | 短期体验 | Phase 7 彻底移除 Element Plus 后会显著下降 |

---

## 十三、迁移 Checklist（高层）

- [ ] **Phase 1** 基础设施（Nuxt UI + Tailwind + Token）
- [ ] **Phase 2** 路由重构（兼容旧 URL）
- [ ] **Phase 3** 布局 / 公共组件
- [ ] **Phase 4** 登录 / 忘记密码
- [ ] **Phase 5** 聊天主区域
- [ ] **Phase 6** 用户中心 / 设置 / 报表 / 测验
- [ ] **Phase 7** 清理 Element Plus
- [ ] **Phase 8** 测试 / 回归 / 发布

每阶段完成后必须满足：
- `pnpm test` 全绿
- `pnpm lint` 通过
- Lighthouse Performance ≥ 90
- 移动端 3 个尺寸（375 / 768 / 1280）回归无破损
- 暗色模式回归无破损

---

## 十四、预期收益

| 指标 | 当前 | 重构后 | 改善 |
|------|------|--------|------|
| 首屏 JS 体积 | ~1.2 MB | ≤ 700 KB | -42% |
| Lighthouse Performance | ~75 | ≥ 92 | +17 |
| 暗色模式切换 | 需手写 ~300 行 | 0 行 | -100% |
| 组件平均代码量 | ~150 行 | ~80 行 | -47% |
| 新增页面工作量 | ~2 天 | ~0.5 天 | -75% |
| 设计一致性 | 50% | 95% | +45pt |
| TypeScript 类型完整性 | 70% | 95% | +25pt |

---

## 十五、附录：参考资源

- [Nuxt UI v4 官方文档](https://ui4.nuxt.com)
- [Reka UI 组件库](https://reka-ui.com)
- [Tailwind CSS v4 升级指南](https://tailwindcss.com/docs/upgrade-guide)
- [Tailwind Variants](https://www.tailwind-variants.org)
- [valibot Schema 校验](https://valibot.dev)
- [@nuxtjs/color-mode](https://color-mode.nuxtjs.org)
- [@internationalized/date](https://react-spectrum.adobe.com/internationalized/date/)

---

> 评审通过后，按 Phase 1 → Phase 8 顺序执行；每阶段产出可独立部署的中间版本，便于渐进发布与回滚。