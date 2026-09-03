---
status: planned
superseded-by: backlog；前端 ChatFile 组件未完整收口
original-path: .trae/documents/文件上传与消息扩展功能实施计划.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-C
---

# 文件上传与消息扩展功能实施计划

## 一、现状分析

### 1.1 后端现状
- ✅ 已有上传服务 `upload_service.go` 已实现，支持图片、文件、视频三种类型
- ✅ 上传处理器 `upload_handler.go` 已实现
- ✅ 路由已注册 `/upload/image`、`/upload/file`、`/upload/video`
- ⚠️ 消息模型 `Message` 已有 `ContentType` 字段，但只支持 `'text'` 和 `'audio'`

### 1.2 前端现状
- ⚠️ 聊天页面有附件按钮，但 `handleAttachment` 函数未实现
- ⚠️ API类型定义了 `MessageItem.contentType` 只有 `'text' | 'audio' | 'img'` 三种
- ⚠️ 消息显示只支持文本和语音消息
- ⚠️ 消息store的 `sendMessage` 函数只支持文本消息

---

## 二、核心原则

### 2.1 不改动现有部分
- 保持现有功能保持原样
- 只新增或扩展，不修改现有代码逻辑

---

## 三、实施步骤

### 阶段一：后端扩展

#### 1. 扩展消息模型（扩展内容）
**文件**: `internal/models/message.go`
- 扩展 `ContentType` 字段类型注释，添加新类型
- 不修改现有字段

#### 2. 扩展上传响应类型
**文件**: `internal/service/upload_service.go`
- 确认现有上传响应类型完整

#### 3. 后端文件类型校验
**文件**: `internal/service/upload_service.go`
- 确认现有校验逻辑（已有，但确保正确
- 可以优化：补充文件类型映射

---

### 阶段二：前端实现

#### 1. 扩展前端类型定义
**文件**: `app/types/api.ts`
- 扩展 `MessageItem.contentType` 添加新类型
- 新增上传相关类型定义

#### 2. 新增上传composable
**新建文件**: `app/composables/useFileUpload.ts`
- 文件选择逻辑
- 文件上传逻辑
- 文件校验逻辑
- 类型安全的 API 调用

#### 3. 扩展聊天页面
**文件**: `app/pages/chat/conversation/[id].vue`
- 实现 `handleAttachment` 函数
- 添加文件选择UI
- 添加文件上传状态
- 添加文件消息显示组件
- 上传后发送给 Kimi（带文件引用）

#### 4. 新增文件消息显示组件
**新建文件**: `app/components/ChatFile.vue`
- 图片消息显示
- 文件消息显示
- 视频消息显示
- 点击下载功能

#### 5. 扩展消息 store
**文件**: `app/stores/message.ts`
- 新增发送文件消息函数
- 保持现有 `sendMessage` 不变

#### 6. 扩展对话发送composable
**文件**: `app/composables/useConversationSender.ts`
- 新增带文件的消息发送逻辑
- 保持现有发送逻辑不变

---

## 四、详细实现详情

### 4.1 消息类型定义

#### 后端消息模型扩展
```go
// ContentType 可支持类型（向后兼容）
// 现有: text, audio
// 新增: image, file, video
```

#### 前端类型扩展
```typescript
// 新增上传响应类型
export interface UploadResult {
  url: string
  filename: string
  size: number
}

// 扩展消息类型
contentType: 'text' | 'audio' | 'image' | 'file' | 'video'
```

### 4.2 文件类型校验

#### 支持的文件类型
| 类型 | 扩展名 | 最大大小 |
|------|--------|----------|
| 图片 | .jpg, .jpeg, .png, .webp, .gif | 5MB |
| 文件 | * | 20MB |
| 视频 | .mp4, .avi, .mov, .wmv, .flv | 50MB |

#### 前后端都要校验
- 前端：上传前校验（用户体验）
- 后端：上传时校验（安全保障）

### 4.3 上传流程

```
用户点击附件按钮
    ↓
打开文件选择对话框
    ↓
选择文件（可多选？）
    ↓
前端校验（大小、类型）
    ↓
显示上传进度
    ↓
上传到后端
    ↓
接收上传结果
    ↓
显示文件消息
    ↓
发送给 Kimi（带文件引用）
    ↓
Kimi 处理返回
```

---

## 五、文件清单

### 修改文件（后端）
| 文件路径 | 操作 |
|---------|------|
| `internal/models/message.go` | 扩展类型注释 |

### 新增文件（前端）
| 文件路径 | 操作 |
|---------|------|
| `app/composables/useFileUpload.ts` | 新建 |
| `app/components/ChatFile.vue` | 新建 |

### 修改文件（前端）
| 文件路径 | 操作 |
|---------|------|
| `app/types/api.ts` | 扩展类型 |
| `app/pages/chat/conversation/[id].vue` | 实现上传 |
| `app/stores/message.ts` | 扩展功能 |
| `app/composables/useConversationSender.ts` | 扩展功能 |

---

## 六、风险与注意事项

### 6.1 向后兼容性
- ✅ 不修改现有功能
- ✅ 新增类型不会影响现有逻辑
- ✅ Kimi 模型能理解文件引用

### 6.2 安全性
- ✅ 前后端双重校验
- ✅ 文件大小限制
- ✅ 文件类型白名单

### 6.3 用户体验
- ✅ 上传进度显示
- ✅ 错误提示友好
- ✅ 支持取消上传
