# Emotion-Echo-Web

> 情绪倾诉与心理健康助手前端应用

基于 Nuxt 3 + Vue 3 + Element Plus 构建的情绪倾诉 Web 应用，提供 AI 对话、情绪分析报表、心理测验、数字人语音等功能。

---

## 技术栈

| 层级 | 技术 |
|------|------|
| 框架 | Nuxt 3 |
| 语言 | Vue 3 + TypeScript |
| UI 组件 | Element Plus |
| 状态管理 | Pinia |
| 图表 | ECharts |
| 3D 数字人 | Three.js + Three-vrm |
| 本地数据库 | Dexie (IndexedDB) |

---

## 功能特性

- [x] 用户注册 / 登录 / 登出（JWT 认证）
- [x] 忘记密码（验证码重置）
- [x] AI 情绪疏导对话（流式输出，支持情绪标签）
- [x] 语音消息录制与播放（TTS 语音合成）
- [x] 3D 数字人展示与动作同步
- [x] 心理测验（SDS 等量表）
- [x] 情绪日报 / 周报 / 月报可视化报表
- [x] 用户中心与资料管理

---

## 快速开始

### 环境要求

- Node.js 18+
- npm / pnpm / yarn

### 1. 克隆与依赖

```bash
git clone <repo-url>
cd Emotion-Echo-Web
npm install
```

### 2. 配置

```bash
cp .env.example .env
```

按需修改 `.env`：
```env
NUXT_PUBLIC_API_BASE_URL=http://localhost:8080/api/v1
```

### 3. 开发启动

```bash
npm run dev
```

访问：`http://localhost:3000`

### 4. 生产构建

```bash
npm run build
npm run preview  # 预览构建结果
```

---

## 项目结构

```
.
├── app/
│   ├── assets/          # 静态资源（样式、3D模型、图标）
│   ├── components/      # Vue 组件
│   │   ├── charts/     # 图表组件
│   │   ├── digital-human/ # 数字人组件
│   │   └── voice/      # 语音相关组件
│   ├── composables/     # 组合式函数
│   ├── configs/        # 配置文件
│   ├── layouts/        # 布局
│   ├── middleware/     # 路由中间件（认证）
│   ├── pages/          # 页面路由
│   ├── plugins/        # Nuxt 插件
│   ├── stores/         # Pinia 状态管理
│   ├── types/          # TypeScript 类型
│   └── utils/          # 工具函数
├── public/             # 公开资源
├── docs/               # 文档
└── nuxt.config.ts      # Nuxt 配置
```

---

## 对接说明

### 后端 API

默认后端地址：`http://localhost:8080/api/v1`

环境变量：`NUXT_PUBLIC_API_BASE_URL`

### TTS 语音服务

默认 TTS 地址：`http://localhost:8003`

---

## 部署

### Docker 部署

```bash
docker build -t emotion-echo-web .
docker run -p 3000:3000 \
  -e NUXT_PUBLIC_API_BASE_URL=http://your-backend-api/api/v1 \
  emotion-echo-web
```

### 服务器部署

1. 构建：`npm run build`
2. 将 `.output` 目录上传到服务器
3. 配置反向代理（Nginx）
4. 启动：`node .output/server/index.mjs`

---

## 开发文档

- [`docs/DESIGN.md`](docs/DESIGN.md) — 设计文档

---

## License

MIT
