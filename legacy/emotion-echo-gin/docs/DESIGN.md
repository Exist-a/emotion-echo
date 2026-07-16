# Emotion-Echo 后端设计文档

> 版本：v1.0
> 日期：2026-05-02
> 用途：多 Agent AI 系统对接指南

---

## 一、系统架构

### 1.1 技术栈

| 层级 | 技术 | 说明 |
|------|------|------|
| 语言 | Go 1.21+ | 后端服务 |
| Web 框架 | Gin | HTTP 路由和中间件 |
| ORM | GORM | 数据库操作 |
| 数据库 | PostgreSQL 14+ | 主数据存储 |
| 缓存 | Redis 7+ | Token 黑名单、限流、检查点 |
| AI 服务 | OpenAI/Kimi 兼容 API | LLM 对话能力 |
| 本地 AI | Qwen3-ASR / SenseVoice | 语音识别和情感识别（可选） |

### 1.2 整体架构

```
┌─────────────────────────────────────────────────────────────────┐
│                         客户端层                                  │
│  ┌──────────────┐    ┌──────────────┐    ┌──────────────┐       │
│  │   Web 前端   │    │  微信 H5     │    │  小程序      │       │
│  │  (Nuxt.js)  │    │              │    │  (预留)      │       │
│  └──────┬───────┘    └──────┬───────┘    └──────┬───────┘       │
└─────────┼───────────────────┼───────────────────┼───────────────┘
          │                   │                   │
          └───────────────────┼───────────────────┘
                              │ HTTP/HTTPS + SSE
┌─────────────────────────────────────────────────────────────────┐
│                         API 网关层                                │
│  ┌─────────────────────────────────────────────────────────────┐│
│  │                    /api/v1                                  ││
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐           ││
│  │  │  Auth   │ │  User   │ │Conversation│ │  AI    │           ││
│  │  └─────────┘ └─────────┘ └─────────┘ └─────────┘           ││
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐           ││
│  │  │ Survey  │ │ Report  │ │ Mental  │ │ Behavior│           ││
│  │  │         │ │         │ │ Health   │ │         │           ││
│  │  └─────────┘ └─────────┘ └─────────┘ └─────────┘           ││
│  └─────────────────────────────────────────────────────────────┘│
│                              │                                    │
│  ┌───────────────────────────┼───────────────────────────────┐ │
│  │                    Middleware                               │ │
│  │  JWT Auth │ CORS │ Rate Limit │ Logger │ Recovery         │ │
│  └───────────────────────────┼───────────────────────────────┘ │
└──────────────────────────────┼──────────────────────────────────┘
                               │
┌──────────────────────────────┼──────────────────────────────────┐
│                    业务逻辑层 (Service)                          │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐                │
│  │ AuthService │ │ UserService │ │ConvService  │                │
│  └─────────────┘ └─────────────┘ └─────────────┘                │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐                │
│  │ AIService   │ │SurveyService│ │ReportService│                │
│  └─────────────┘ └─────────────┘ └─────────────┘                │
│  ┌─────────────────────────────┐                                 │
│  │      MentalHealthService   │                                 │
│  └─────────────────────────────┘                                 │
└──────────────────────────────┼──────────────────────────────────┘
                               │
┌──────────────────────────────┼──────────────────────────────────┐
│                    工作流引擎层 (Workflow)                       │
│  ┌─────────────────────────────────────────────────────────────┐  │
│  │                    Graph Engine                            │  │
│  │  ┌────────────┐ ┌────────────┐ ┌────────────┐               │  │
│  │  │Sequential  │ │  Parallel  │ │Conditional │               │  │
│  │  │   Node     │ │   Node     │ │   Node     │               │  │
│  │  └────────────┘ └────────────┘ └────────────┘               │  │
│  │  ┌────────────┐ ┌────────────┐                                │  │
│  │  │  LLMNode   │ │  ToolNode  │                                │  │
│  │  └────────────┘ └────────────┘                                │  │
│  └─────────────────────────────────────────────────────────────┘  │
│  ┌─────────────────┐  ┌─────────────────┐                        │
│  │ Chat Workflow   │  │Assessment Workfl│                        │
│  │ (情绪分析+回复)  │  │ (多维度评估)    │                        │
│  └─────────────────┘  └─────────────────┘                        │
└──────────────────────────────┼──────────────────────────────────┘
                               │
┌──────────────────────────────┼──────────────────────────────────┐
│                    数据访问层 (Repository)                       │
│  ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐ ┌────────┐         │
│  │  User  │ │Conversation│ │Message │ │ Survey │ │ Redis  │         │
│  └────────┘ └────────┘ └────────┘ └────────┘ └────────┘         │
└──────────────────────────────┼──────────────────────────────────┘
                               │
┌──────────────────────────────┼──────────────────────────────────┐
│                         数据存储层                               │
│         ┌─────────────────┐        ┌─────────────────┐          │
│         │   PostgreSQL    │        │      Redis      │          │
│         │  (主数据库)      │        │   (缓存/会话)   │          │
│         └─────────────────┘        └─────────────────┘          │
└─────────────────────────────────────────────────────────────────┘
                               │
┌──────────────────────────────┼──────────────────────────────────┐
│                    外部 AI 服务层                                │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐           │
│  │ Kimi (Kimi)  │  │ OpenAI GPT   │  │ 本地 Whisper │           │
│  │              │  │              │  │  (可选)      │           │
│  └──────────────┘  └──────────────┘  └──────────────┘           │
└─────────────────────────────────────────────────────────────────┘
```

### 1.3 多 Agent 工作流架构

系统采用 DAG（有向无环图）引擎驱动 AI 工作流，支持多种节点类型：

```
┌─────────────────────────────────────────────────────────────────┐
│                      DAG 工作流引擎                              │
│                                                                 │
│  ┌─────────────┐     ┌─────────────┐     ┌─────────────┐        │
│  │  Sequential │────▶│  Parallel   │────▶│Conditional  │        │
│  │   Node      │     │   Node      │     │   Node      │        │
│  └─────────────┘     └──────┬──────┘     └──────┬──────┘        │
│                            │                    │               │
│                    ┌───────┴───────┐            │               │
│                    ▼               ▼            ▼               │
│             ┌───────────┐   ┌───────────┐   ┌───────────┐        │
│             │  LLMNode  │   │  LLMNode  │   │  LLMNode  │        │
│             │ (情绪分析) │   │ (关键词)  │   │ (摘要生成)│        │
│             └───────────┘   └───────────┘   └───────────┘        │
│                    │               │            │               │
│                    └───────┬───────┘            │               │
│                            ▼                    │               │
│                    ┌───────────────┐            │               │
│                    │  ToolNode    │◀───────────┘               │
│                    │ (数据存储)    │                            │
│                    └───────────────┘                            │
└─────────────────────────────────────────────────────────────────┘
```

**节点类型说明：**

| 节点类型 | 功能 | 使用场景 |
|---------|------|----------|
| SequentialNode | 顺序执行子节点 | 多步骤流程 |
| ParallelNode | 并行执行子节点 | 多数据源收集、多维度分析 |
| ConditionalNode | 条件分支 | 根据状态路由不同处理 |
| LLMNode | 调用大语言模型 | AI 分析、生成回复 |
| ToolNode | 调用工具函数 | 数据查询、计算、格式化 |

**工作流类型：**

| 工作流 | 用途 | 输入 | 输出 |
|--------|------|------|------|
| Chat Workflow | AI 对话 + 情绪分析 | 用户消息 | AI 回复 + 情绪标签 |
| Assessment Workflow | 心理健康评估 | 聊天记录 + 量表结果 | 六维评分 + 风险等级 + 建议 |

### 1.4 项目目录结构

```
Emotion-Echo-Gin/
├── cmd/server/main.go              # 服务入口
├── configs/
│   ├── config.yaml                 # 配置文件
│   └── config.example.yaml         # 配置模板
├── internal/
│   ├── config/config.go            # 配置解析
│   ├── database/
│   │   ├── postgres.go             # PostgreSQL 连接
│   │   └── redis.go                # Redis 连接
│   ├── handler/                    # HTTP 处理器
│   │   ├── auth_handler.go         # 认证
│   │   ├── user_handler.go         # 用户
│   │   ├── conversation_handler.go # 会话
│   │   ├── message_handler.go      # 消息
│   │   ├── ai_handler.go          # AI 对话
│   │   ├── survey_handler.go       # 测验
│   │   ├── report_handler.go       # 报表
│   │   ├── mental_health_handler.go# 心理健康
│   │   └── oauth_handler.go        # OAuth
│   ├── middleware/                 # 中间件
│   │   ├── auth.go                 # JWT 鉴权
│   │   ├── cors.go                 # 跨域
│   │   ├── rate_limit.go           # 限流
│   │   ├── logger.go               # 日志
│   │   └── recovery.go             # 错误恢复
│   ├── models/                     # 数据模型
│   │   ├── user.go
│   │   ├── conversation.go
│   │   ├── message.go
│   │   ├── survey.go
│   │   ├── survey_result.go
│   │   ├── emotion_analysis.go
│   │   └── mental_health.go
│   ├── repository/                 # 数据访问层
│   │   ├── user_repo.go
│   │   ├── conversation_repo.go
│   │   ├── message_repo.go
│   │   ├── survey_repo.go
│   │   ├── emotion_analysis_repo.go
│   │   └── redis_repo.go
│   ├── service/                    # 业务逻辑层
│   │   ├── auth_service.go
│   │   ├── user_service.go
│   │   ├── conversation_service.go
│   │   ├── message_service.go
│   │   ├── ai_service.go
│   │   ├── survey_service.go
│   │   ├── report_service.go
│   │   └── mental_health_service.go
│   ├── pkg/                        # 公共工具包
│   │   ├── errors/                 # 错误定义
│   │   ├── jwt/                    # JWT 工具
│   │   ├── llm/                    # LLM 客户端
│   │   ├── response/              # 统一响应
│   │   ├── nanoid/                # ID 生成
│   │   └── password/              # 密码哈希
│   ├── workflow/                   # 工作流引擎
│   │   ├── engine.go              # 工作流执行器
│   │   ├── state.go              # 工作流状态
│   │   ├── nodes.go              # 节点定义
│   │   ├── graph/                # DAG 引擎
│   │   │   ├── engine.go
│   │   │   ├── nodes.go
│   │   │   ├── state.go
│   │   │   └── checkpoint.go
│   │   ├── chat/                 # 对话工作流
│   │   │   ├── workflow.go
│   │   │   └── nodes.go
│   │   └── assessment/           # 评估工作流
│   │       ├── workflow.go
│   │       └── phases.go
│   ├── scheduler/                 # 定时任务
│   └── worker/                    # 异步 worker
├── migrations/                    # 数据库迁移
├── docs/                          # 文档
│   └── DESIGN.md                  # 本文档
└── server                         # 编译后的二进制
```

---

## 二、API 接口规范

### 2.1 基础信息

| 项目 | 值 |
|------|-----|
| Base URL | `http://localhost:8081/api/v1` |
| 协议 | HTTP（开发）/ HTTPS（生产） |
| 数据格式 | JSON |
| 字符编码 | UTF-8 |
| 时间格式 | ISO 8601 (`2026-04-19T08:36:16+08:00`) |
| 认证方式 | Bearer Token（JWT） |

### 2.2 响应格式

**成功响应：**
```json
{
  "code": 0,
  "message": "success",
  "data": { ... }
}
```

**错误响应：**
```json
{
  "code": 10001,
  "message": "参数错误：xxx",
  "data": null
}
```

### 2.3 错误码体系

| 错误码 | 含义 | 说明 |
|--------|------|------|
| 0 | 成功 | 请求成功 |
| 10001 | 参数错误 | 请求参数缺失或格式错误 |
| 10002 | Token 过期 | AccessToken 已过期，需刷新 |
| 10003 | Token 无效 | Token 格式错误或被篡改 |
| 20001 | 用户不存在 | |
| 20002 | 密码错误 | |
| 20003 | 验证码错误 | |
| 20004 | 用户已存在 | |
| 20005 | 验证码发送过于频繁 | |
| 30001 | 会话不存在 | |
| 30002 | 非会话所有者 | |
| 50001 | AI 服务错误 | |

### 2.4 认证机制

**双 Token 认证：**

```
┌──────────────┐         ┌──────────────┐         ┌──────────────┐
│  AccessToken │────────▶│   请求 API    │◀────────│ RefreshToken │
│  (15分钟)    │         │              │         │  (7天)       │
└──────────────┘         └──────────────┘         └──────────────┘
       │                                               │
       └────────────────── 存储位置 ───────────────────┘
         localStorage              HttpOnly Cookie
```

**请求头：**
```
Authorization: Bearer {accessToken}
Content-Type: application/json
```

### 2.5 接口列表

#### 认证模块（8个）

| # | 方法 | 路径 | 说明 |
|---|------|------|------|
| 1 | POST | /auth/verification-code | 发送验证码 |
| 2 | POST | /auth/register | 用户注册 |
| 3 | POST | /auth/login | 用户登录 |
| 4 | POST | /auth/refresh | 刷新 Token |
| 5 | POST | /auth/logout | 登出 |
| 6 | POST | /auth/reset-password | 重置密码 |
| 7 | GET | /auth/oauth/wechat/url | 获取微信授权 URL（预留） |
| 8 | POST | /auth/oauth/wechat/login | 微信登录（预留） |

#### 用户模块（3个）

| # | 方法 | 路径 | 说明 |
|---|------|------|------|
| 9 | GET | /user/profile | 获取用户信息 |
| 10 | PUT | /user/profile | 更新用户信息 |
| 11 | POST | /user/avatar | 上传头像 |

#### 会话模块（5个）

| # | 方法 | 路径 | 说明 |
|---|------|------|------|
| 12 | GET | /conversations | 获取会话列表 |
| 13 | POST | /conversations | 创建会话 |
| 14 | PUT | /conversations/:id | 更新会话 |
| 15 | POST | /conversations/:id/pin | 置顶会话 |
| 16 | DELETE | /conversations/:id | 删除会话 |

#### 消息模块（2个）

| # | 方法 | 路径 | 说明 |
|---|------|------|------|
| 17 | POST | /conversations/:id/messages | 发送消息 |
| 18 | GET | /conversations/:id/messages | 获取消息列表 |

#### AI 模块（1个）

| # | 方法 | 路径 | 说明 |
|---|------|------|------|
| 19 | POST | /ai/stream | AI 流式对话（SSE） |

#### 测验模块（4个）

| # | 方法 | 路径 | 说明 |
|---|------|------|------|
| 20 | GET | /surveys | 获取量表列表 |
| 21 | GET | /surveys/:id | 获取量表详情 |
| 22 | POST | /surveys/:id/submit | 提交测验答案 |
| 23 | GET | /surveys/result/:resultId | 获取测验结果 |

#### 报表模块（2个）

| # | 方法 | 路径 | 说明 |
|---|------|------|------|
| 24 | GET | /reports/daily | 获取日报 |
| 25 | GET | /reports/trend | 获取情绪趋势 |

#### 心理健康模块（4个）

| # | 方法 | 路径 | 说明 |
|---|------|------|------|
| 26 | GET | /mental-health/assessment | 获取最新评估 |
| 27 | GET | /mental-health/history | 获取历史评估 |
| 28 | POST | /mental-health/trigger | 手动触发评估 |
| 29 | GET | /mental-health/trend | 获取评估趋势 |

### 2.6 AI 流式对话接口详解

**请求：**
```http
POST /ai/stream
Authorization: Bearer {accessToken}
Content-Type: application/json

{
  "conversationId": "conv_xxx",  // 可选，不传则自动创建会话
  "message": "我最近工作压力很大",
  "emotion": "sad",              // 可选，影响 AI 回复风格
  "model": "kimi",               // 可选，指定 AI 模型
  "shouldGenerateTitle": true    // 可选，是否为新会话生成标题
}
```

**响应：** SSE（Server-Sent Events）流式传输

**SSE 事件格式：**

| 事件 | 数据格式 | 说明 |
|------|----------|------|
| start | `{"type":"start","conversationId":"conv_xxx"}` | 流开始 |
| delta | `{"type":"delta","content":"你好"}` | 内容增量 |
| finish | `{"type":"finish","messageId":"msg_xxx"}` | 流结束 |
| error | `{"type":"error","code":50001,"message":"AI服务繁忙"}` | 错误 |

**智能标题生成：**

当 `shouldGenerateTitle` 为 `true` 或创建新会话时，系统会在 AI 回复完成后自动生成会话标题：

- 调用 LLM 根据首条用户消息生成不超过10个字符的标题
- 生成失败时降级为截取消息前10个字符
- 标题通过 `UpdateTitle` 方法存储到数据库

**多 AI 模型切换支持：**

| 模型标识 | 说明 | 配置key |
|----------|------|---------|
| kimi | Kimi (默认) | ai.kimi |
| openai | OpenAI GPT | ai.openai |
| local | 本地部署模型 | ai.local |

---

## 三、数据模型

### 3.1 用户表 (users)

```sql
CREATE TABLE users (
  id BIGSERIAL PRIMARY KEY,
  username VARCHAR(64) UNIQUE NOT NULL,      -- 手机号或邮箱
  password_hash VARCHAR(255) NOT NULL,       -- bcrypt 哈希
  nickname VARCHAR(64) DEFAULT '用户',
  avatar VARCHAR(500) DEFAULT '/imgs/default-avatar.webp',
  age INTEGER CHECK (age >= 0 AND age <= 150),
  wechat_open_id VARCHAR(64),
  wechat_union_id VARCHAR(64),
  config JSONB DEFAULT '{}',                 -- 用户配置（字体大小、主题等）
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  deleted_at TIMESTAMP                       -- 软删除
);

CREATE INDEX idx_users_username ON users(username);
CREATE INDEX idx_users_wechat_open_id ON users(wechat_open_id);
```

**Go Model:**
```go
type User struct {
    ID            int64     `gorm:"primaryKey" json:"id,string"`
    Username      string    `gorm:"uniqueIndex;size:64;not null" json:"username"`
    PasswordHash  string    `gorm:"size:255;not null" json:"-"`
    Nickname      string    `gorm:"size:64;default:'用户'" json:"nickname"`
    Avatar        string    `gorm:"size:500;default:'/imgs/default-avatar.webp'" json:"avatar"`
    Age           *int      `json:"age,omitempty"`
    Config        UserConfig `gorm:"type:jsonb;default:'{}'" json:"config"`
    CreatedAt     time.Time `json:"createdAt"`
    UpdatedAt     time.Time `json:"updatedAt"`
}

type UserConfig struct {
    FontSize string `json:"fontSize,omitempty"`  // small/medium/large
    Theme    string `json:"theme,omitempty"`     // light/dark/auto
}
```

### 3.2 会话表 (conversations)

```sql
CREATE TABLE conversations (
  id VARCHAR(32) PRIMARY KEY,                -- nanoid 前缀 conv_
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  title VARCHAR(200) NOT NULL,
  is_top BOOLEAN DEFAULT FALSE,
  last_message_content TEXT,
  last_message_time TIMESTAMP,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_conversations_user_id ON conversations(user_id);
CREATE INDEX idx_conversations_updated_at ON conversations(updated_at DESC);
```

**Go Model:**
```go
type Conversation struct {
    ID                 string    `gorm:"primaryKey;size:32" json:"id"`
    UserID             int64     `gorm:"index;not null" json:"userId,string"`
    Title              string    `gorm:"size:200;not null" json:"title"`
    IsTop              bool      `gorm:"column:is_top;default:false" json:"isTop"`
    LastMessageContent string    `gorm:"column:last_message_content;type:text" json:"lastMessage,omitempty"`
    LastMessageTime    *int64    `gorm:"column:last_message_time" json:"lastMessageTime,omitempty"`
    CreatedAt          time.Time `json:"createdAt"`
    UpdatedAt          time.Time `json:"updatedAt"`
}
```

### 3.3 消息表 (messages)

```sql
CREATE TABLE messages (
  id VARCHAR(32) PRIMARY KEY,                -- nanoid 前缀 msg_
  conversation_id VARCHAR(32) NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
  sender VARCHAR(10) NOT NULL CHECK (sender IN ('user', 'ai')),
  content TEXT,
  content_type VARCHAR(10) DEFAULT 'text' CHECK (content_type IN ('text', 'audio', 'img')),
  emotion_tag VARCHAR(10) CHECK (emotion_tag IN ('sad', 'angry', 'anxious')),
  send_time BIGINT NOT NULL,                 -- 毫秒时间戳
  created_at BIGINT                           -- 秒时间戳
);

CREATE INDEX idx_messages_conversation_id ON messages(conversation_id);
CREATE INDEX idx_messages_send_time ON messages(send_time DESC);
```

**Go Model:**
```go
type Message struct {
    ID             string  `gorm:"primaryKey;size:32" json:"id"`
    ConversationID string  `gorm:"index;size:32;not null" json:"conversationId"`
    Sender         string  `gorm:"size:10;not null" json:"sender"`  // user/ai
    Content        string  `gorm:"type:text" json:"content"`
    ContentType    string  `gorm:"size:10;default:'text'" json:"contentType"`
    EmotionTag     *string `gorm:"size:10" json:"emotionTag,omitempty"`
    SendTime       int64   `gorm:"index;not null" json:"sendTime"`
    CreatedAt      int64   `json:"createdAt"`
}
```

### 3.4 情绪分析表 (emotion_analyses)

```sql
CREATE TABLE emotion_analyses (
  id BIGSERIAL PRIMARY KEY,
  conversation_id VARCHAR(32) NOT NULL,
  user_id BIGINT NOT NULL,
  analyzed_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
  emotion_scores JSONB,                      -- {"sad": 0.6, "angry": 0.2, "anxious": 0.1}
  dominant_emotion VARCHAR(20),
  summary TEXT,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_ea_conversation_id ON emotion_analyses(conversation_id);
CREATE INDEX idx_ea_user_id ON emotion_analyses(user_id);
```

**Go Model:**
```go
type EmotionAnalysis struct {
    ID              int64            `gorm:"primaryKey" json:"-"`
    ConversationID  string           `gorm:"index;size:32;not null" json:"-"`
    UserID          int64            `gorm:"index;not null" json:"-"`
    AnalyzedAt      time.Time        `json:"analyzedAt"`
    EmotionScores   EmotionScoresMap `gorm:"type:jsonb" json:"emotionScores"`
    DominantEmotion string           `gorm:"size:20" json:"dominantEmotion"`
    Summary         string           `gorm:"type:text" json:"summary"`
    CreatedAt       time.Time        `json:"-"`
}

type EmotionScoresMap map[string]float64  // 情绪标签 -> 分数
```

### 3.5 心理测验表 (surveys)

```sql
CREATE TABLE surveys (
  id SERIAL PRIMARY KEY,
  title VARCHAR(200) NOT NULL,
  description TEXT,
  estimated_time VARCHAR(50),
  questions JSONB NOT NULL,                 -- 题目列表
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

**Go Model:**
```go
type Survey struct {
    ID            int               `gorm:"primaryKey" json:"id"`
    Title         string            `gorm:"size:200;not null" json:"title"`
    Description   string            `gorm:"type:text" json:"description"`
    EstimatedTime string            `gorm:"size:50" json:"estimatedTime"`
    Questions     []SurveyQuestion  `gorm:"type:jsonb" json:"questions"`
    CreatedAt     time.Time         `json:"createdAt"`
}

type SurveyQuestion struct {
    ID      int            `json:"id"`
    Title   string         `json:"title"`
    Type    string         `json:"type"`  // radio/checkbox/text
    Options []SurveyOption `json:"options"`
}

type SurveyOption struct {
    ID    int    `json:"id"`
    Text  string `json:"text"`
    Score int    `json:"score"`
}
```

### 3.6 测验结果表 (survey_results)

```sql
CREATE TABLE survey_results (
  id VARCHAR(32) PRIMARY KEY,                -- nanoid 前缀 res_
  user_id BIGINT NOT NULL,
  survey_id INT NOT NULL REFERENCES surveys(id),
  answers JSONB NOT NULL,                    -- 用户答案
  total_score INT NOT NULL,
  level VARCHAR(50),                         -- 轻度/中度/重度
  suggestion TEXT,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_sr_user_id ON survey_results(user_id);
```

### 3.7 心理健康评估表 (mental_health_assessments)

```sql
CREATE TABLE mental_health_assessments (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  
  -- 评估类型和周期
  assessment_type VARCHAR(20) NOT NULL,     -- daily/weekly/comprehensive
  period_start TIMESTAMP NOT NULL,
  period_end TIMESTAMP NOT NULL,
  
  -- 六维评分 (0-100，越高越健康)
  emotion_score INT,
  depression_score INT,
  anxiety_score INT,
  stress_score INT,
  sleep_score INT,
  social_score INT,
  
  -- 综合评估
  overall_score INT,
  risk_level VARCHAR(20) NOT NULL,           -- low/medium/high/critical
  risk_factors JSONB DEFAULT '[]',
  warning_flags JSONB DEFAULT '[]',
  
  -- 报告内容
  summary TEXT,
  suggestions JSONB DEFAULT '[]',
  
  -- 关联数据
  emotion_analysis_ids JSONB DEFAULT '[]',
  survey_result_ids JSONB DEFAULT '[]',
  
  -- 元数据
  is_notified BOOLEAN DEFAULT FALSE,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_mha_user_id ON mental_health_assessments(user_id);
CREATE INDEX idx_mha_user_type_created ON mental_health_assessments(user_id, assessment_type, created_at DESC);
```

**Go Model:**
```go
type MentalHealthAssessment struct {
    ID              int64        `gorm:"primaryKey" json:"id"`
    UserID          int64        `gorm:"index;not null" json:"-"`
    
    AssessmentType  string       `gorm:"size:20;not null" json:"assessmentType"`
    PeriodStart     time.Time    `json:"periodStart"`
    PeriodEnd       time.Time    `json:"periodEnd"`
    
    EmotionScore    int          `json:"emotionScore"`
    DepressionScore int          `json:"depressionScore"`
    AnxietyScore    int          `json:"anxietyScore"`
    StressScore     int          `json:"stressScore"`
    SleepScore      int          `json:"sleepScore"`
    SocialScore     int          `json:"socialScore"`
    
    OverallScore    int          `json:"overallScore"`
    RiskLevel       string       `gorm:"size:20;not null" json:"riskLevel"`
    RiskFactors     []string     `gorm:"type:jsonb" json:"riskFactors"`
    WarningFlags    []string     `gorm:"type:jsonb" json:"warningFlags"`
    
    Summary         string       `json:"summary"`
    Suggestions     []Suggestion `gorm:"type:jsonb" json:"suggestions"`
    
    CreatedAt       time.Time    `json:"createdAt"`
}

type Suggestion struct {
    Level     string   `json:"level"`      // immediate/short_term/long_term
    Category  string   `json:"category"`    // professional/self_help/lifestyle
    Title     string   `json:"title"`
    Content   string   `json:"content"`
    Actions   []string `json:"actions"`
}
```

### 3.8 Token 表 (refresh_tokens)

```sql
CREATE TABLE refresh_tokens (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  jti VARCHAR(64) UNIQUE NOT NULL,          -- JWT ID，用于 Token 轮换
  token_hash VARCHAR(255) NOT NULL,
  expires_at TIMESTAMP NOT NULL,
  created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_rt_user_id ON refresh_tokens(user_id);
CREATE INDEX idx_rt_jti ON refresh_tokens(jti);
```

---

## 四、多 AI 模型切换机制

### 4.1 配置结构

```yaml
ai:
  provider: "kimi"  # 当前使用的 AI 提供者
  
  kimi:
    api_key: "${KIMI_API_KEY}"
    base_url: "https://api.moonshot.cn/v1"
    model: "moonshot-v1-8k"
    timeout: 120
    max_tokens: 2000
    temperature: 0.7
  
  openai:
    api_key: "${OPENAI_API_KEY}"
    base_url: "https://api.openai.com/v1"
    model: "gpt-4o-mini"
    timeout: 120
    max_tokens: 2000
    temperature: 0.7
  
  local:
    base_url: "http://localhost:8000/v1"
    timeout: 60
```

### 4.2 切换流程

```
请求 POST /ai/stream
    │
    ▼
┌─────────────────────┐
│ 解析 model 参数      │
│ (可选，默认 kimi)    │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 从配置获取对应模型   │
│ LLM 实例            │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 调用 AI 对话        │
└──────────┬──────────┘
           │
           ▼
┌─────────────────────┐
│ 流式返回结果        │
└─────────────────────┘
```

### 4.3 新增 AI 模型接入

1. 在 `configs/config.yaml` 中添加新模型配置
2. 在 `internal/pkg/llm/client.go` 中实现新的 LLM 客户端
3. 在 `AI_SERVICE.StreamChat` 中添加新的模型路由逻辑

---

## 五、工作流引擎详解

### 5.1 Graph 引擎核心概念

```go
// Graph 有向无环图
type Graph struct {
    ID          string
    Nodes       map[string]Node
    Edges       map[string][]Edge
    Checkpointer Checkpointer  // 断点持久化
}

// Node 节点接口
type Node interface {
    GetID() string
    Execute(ctx context.Context, state State) (State, error)
}

// Edge 边
type Edge struct {
    To        string
    Condition EdgeCondition  // nil 表示无条件
}

// State 工作流状态
type State interface {
    Get(key string) (interface{}, bool)
    Set(key string, value interface{})
}
```

### 5.2 Chat 工作流（情绪分析 + AI 回复）

```
用户消息
    │
    ▼
┌──────────────────────────────────────┐
│         Chat Workflow                │
│                                      │
│  ┌──────────────┐    ┌────────────┐ │
│  │ EmotionAnalysis│───▶│PromptSelect│ │
│  │     Node      │    │   Node     │ │
│  └──────┬───────┘    └─────┬──────┘ │
│         │                  │        │
│         ▼                  ▼        │
│  ┌──────────────┐    ┌────────────┐ │
│  │state["emotion"]│  │state["sys"]│ │
│  └──────────────┘    └────────────┘ │
│                                      │
└──────────────────────────────────────┘
    │
    ▼
生成个性化 AI 回复
```

**节点说明：**

| 节点 | 输入 | 输出 |
|------|------|------|
| EmotionAnalysisNode | 用户消息 | emotion, confidence |
| PromptSelectorNode | emotion, confidence | system_prompt |

### 5.3 Assessment 工作流（心理健康评估）

```
触发评估
    │
    ▼
┌──────────────────────────────────────────────────────┐
│              Assessment Workflow                     │
│                                                       │
│  Phase 1: DataCollection                              │
│  ┌─────────────┐ ┌─────────────┐ ┌─────────────┐    │
│  │  聊天记录    │ │  量表结果   │ │  历史评估   │    │
│  └──────┬──────┘ └──────┬──────┘ └──────┬──────┘    │
│         └───────────────┼───────────────┘            │
│                         ▼                             │
│  Phase 2: DimensionAnalysis                          │
│  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────┐     │
│  │情绪分析 │ │抑郁风险 │ │焦虑风险 │ │压力指数 │     │
│  └────┬────┘ └────┬────┘ └────┬────┘ └────┬────┘     │
│       └───────────┼───────────┼───────────┘          │
│                   ▼                                   │
│  Phase 3: RiskAssessment                             │
│  ┌─────────────────────────────┐                     │
│  │    综合风险评估              │                     │
│  │  risk_level + risk_factors  │                     │
│  └─────────────┬───────────────┘                     │
│                ▼                                     │
│  Phase 4: Intervention                               │
│  ┌─────────────────────────────┐                     │
│  │    分级干预建议生成          │                     │
│  └─────────────┬───────────────┘                     │
│                ▼                                     │
│  Phase 5: ReportGeneration                           │
│  ┌─────────────────────────────┐                     │
│  │    评估报告生成              │                     │
│  └─────────────────────────────┘                     │
└──────────────────────────────────────────────────────┘
    │
    ▼
返回评估结果
```

---

## 六、配置说明

### 6.1 配置文件结构

```yaml
server:
  port: 8080
  mode: debug  # debug/release
  read_timeout: 60
  write_timeout: 120

database:
  postgres:
    host: localhost
    port: 5432
    user: postgres
    password: "${DB_PASSWORD}"
    dbname: emotion_echo
    sslmode: disable
    max_open_conns: 100
    max_idle_conns: 10
  
  redis:
    host: localhost
    port: 6379
    password: "${REDIS_PASSWORD}"
    db: 0

jwt:
  secret: "${JWT_SECRET}"
  access_token_expire: 15    # 分钟
  refresh_token_expire: 168  # 小时 (7天)

ai:
  provider: "kimi"
  kimi:
    api_key: "${KIMI_API_KEY}"
    base_url: "https://api.moonshot.cn/v1"
    model: "moonshot-v1-8k"
    timeout: 120
    max_tokens: 2000
    temperature: 0.7

oauth:
  wechat_app_id: "${WECHAT_APP_ID}"
  wechat_app_secret: "${WECHAT_APP_SECRET}"
  wechat_redirect_uri: "http://localhost:3000/auth/callback"

storage:
  type: local  # local/oss/s3
  local:
    path: ./uploads
  oss:
    endpoint: "${OSS_ENDPOINT}"
    access_key: "${OSS_ACCESS_KEY}"
    secret_key: "${OSS_SECRET_KEY}"
    bucket: "${OSS_BUCKET}"

rate_limit:
  enabled: true
  requests_per_second: 10
  burst: 20
```

---

## 七、部署说明

### 7.1 环境要求

- Go 1.21+
- PostgreSQL 14+
- Redis 7+

### 7.2 快速启动

```bash
# 1. 克隆代码
git clone <repo-url>
cd Emotion-Echo-Gin

# 2. 安装依赖
go mod download

# 3. 配置数据库
cp configs/config.example.yaml configs/config.yaml
# 编辑 config.yaml 填入数据库密码和 API Key

# 4. 启动服务
go run ./cmd/server/main.go

# 5. 验证服务
curl http://localhost:8080/health
```

### 7.3 Docker 部署

```bash
# 使用 docker-compose 启动
docker-compose up -d
```
