---
status: landed
superseded-by: stage-34-multimodal-fusion.md
original-path: .trae/documents/multimodal-emotion-feature/backend-plan.md
original-date: 2026-06-XX
migrated-at: 2026-09-03
round: 2-A
---

# Emotion-Echo 多模态情绪功能 - 后端实现计划

## 一、需求分析

根据设计文档 `multimodal-emotion-feature/plan.md`，后端需要实现以下功能：

| 功能点 | 说明 | 优先级 |
|--------|------|--------|
| 面部情绪分析客户端 | 调用 FER 服务进行面部情绪识别 | P0 |
| 面部情绪API接口 | 接收前端发送的面部关键点，返回情绪分析结果 | P0 |
| Redis缓存集成 | 临时存储面部情绪结果（TTL=3秒） | P0 |
| 多模态情绪融合 | 融合面部(0.5)、语音(0.3)、文字(0.2)情绪 | P0 |
| 请求结构扩展 | 在 StreamRequest 中添加面部情绪字段 | P0 |

---

## 二、修改文件清单

### 2.1 新增文件

| 文件路径 | 说明 |
|----------|------|
| `internal/pkg/llm/face_emotion.go` | 面部情绪分析客户端，调用 FER 服务 |
| `internal/handler/face_handler.go` | 面部情绪API接口处理器 |
| `internal/service/face_service.go` | 面部情绪服务逻辑 |

### 2.2 修改文件

| 文件路径 | 修改内容 |
|----------|----------|
| `internal/service/ai_types.go` | StreamRequest 新增 `FaceEmotion` 和 `FaceConfidence` 字段 |
| `internal/service/ai_emotion.go` | 新增多模态情绪融合逻辑 |
| `internal/router/router.go` | 添加 `/api/face/emotion` 路由 |
| `config.yaml` | 添加 FER 服务配置 |

---

## 三、详细设计

### 3.1 新增：面部情绪分析客户端

**文件**: `internal/pkg/llm/face_emotion.go`

```go
// FaceEmotionClient 面部情绪分析客户端
type FaceEmotionClient struct {
    httpClient *http.Client
    baseURL    string
}

// FaceEmotionResult 面部情绪分析结果
type FaceEmotionResult struct {
    Emotion     string  `json:"emotion"`
    Confidence  float64 `json:"confidence"`
    RawEmotion  string  `json:"raw_emotion,omitempty"`
    ProcessedAt int64   `json:"processed_at"`
}

// AnalyzeFaceEmotion 分析面部情绪（接收图片文件）
func (c *FaceEmotionClient) AnalyzeFaceEmotion(ctx context.Context, imageData []byte) (*FaceEmotionResult, error)
```

### 3.2 新增：面部情绪服务

**文件**: `internal/service/face_service.go`

```go
// FaceService 面部情绪服务
type FaceService struct {
    client    *llm.FaceEmotionClient
    redisRepo *repository.RedisRepository
    cfg       *config.Config
}

// AnalyzeAndCache 分析面部情绪并缓存到 Redis
func (s *FaceService) AnalyzeAndCache(ctx context.Context, userID int64, sessionID string, imageData []byte) (*llm.FaceEmotionResult, error)

// GetCachedEmotion 获取缓存的面部情绪（3秒内有效）
func (s *FaceService) GetCachedEmotion(ctx context.Context, userID int64, sessionID string) (*llm.FaceEmotionResult, error)
```

### 3.3 新增：面部情绪API处理器

**文件**: `internal/handler/face_handler.go`

```go
// FaceHandler 面部情绪处理接口
type FaceHandler struct {
    faceService *service.FaceService
}

// AnalyzeEmotion POST /api/face/emotion
func (h *FaceHandler) AnalyzeEmotion(c *gin.Context)
```

### 3.4 修改：StreamRequest 结构

**文件**: `internal/service/ai_types.go`

```go
type StreamRequest struct {
    ConversationID      string  `json:"conversationId,omitempty"`
    Message            string  `json:"message" binding:"required"`
    Emotion            string  `json:"emotion,omitempty"`          // 现有：文字情绪
    VoiceEmotion       string  `json:"voiceEmotion,omitempty"`     // 现有：语音情绪
    FaceEmotion        string  `json:"faceEmotion,omitempty"`      // 新增：面部情绪
    FaceConfidence     float64 `json:"faceConfidence,omitempty"`   // 新增：面部情绪置信度
    Model              string  `json:"model,omitempty"`
    ShouldGenerateTitle bool   `json:"shouldGenerateTitle,omitempty"`
}
```

### 3.5 修改：多模态情绪融合

**文件**: `internal/service/ai_emotion.go`

新增方法：
```go
// FuseEmotions 多模态情绪融合
// 权重：面部(0.5)、语音(0.3)、文字(0.2)
func (s *AIService) FuseEmotions(faceEmotion, voiceEmotion, textEmotion string, 
    faceConfidence, voiceConfidence, textConfidence float64) (string, float64)
```

### 3.6 修改：路由

**文件**: `internal/router/router.go`

```go
// 面部情绪路由
r.POST("/api/face/emotion", handler.NewFaceHandler(faceService).AnalyzeEmotion)
```

### 3.7 修改：配置文件

**文件**: `config.yaml`

```yaml
ai:
  # 原有配置...
  face:
    enabled: true
    fer_base_url: "http://localhost:8004"  # FER服务地址
    redis_ttl: 3                           # Redis缓存时间（秒）
    weights:
      face: 0.5
      voice: 0.3
      text: 0.2
```

---

## 四、多模态融合算法

### 4.1 融合规则

```
1. 归一化各情绪置信度到 0-1 范围
2. 计算加权得分:
   FinalScore[emotion] = 
     Face(0.5) * faceScore[emotion] + 
     Voice(0.3) * voiceScore[emotion] + 
     Text(0.2) * textScore[emotion]
3. 取 FinalScore 最高的作为最终情绪
```

### 4.2 缺失处理

| 缺失项 | 调整后权重 |
|--------|-----------|
| 面部不可用 | Voice(0.6) + Text(0.4) |
| 语音不可用 | Face(0.6) + Text(0.4) |
| 文字不可用 | Face(0.7) + Voice(0.3) |
| 全部不可用 | 返回 neutral, confidence = 0.5 |

---

## 五、部署与集成

### 5.1 FER 服务配置

| 项目 | 值 |
|------|-----|
| 服务地址 | http://localhost:8004 |
| API端点 | POST /analyze |
| 请求格式 | multipart/form-data (image file) |
| 响应格式 | `{"emotion": "happy", "confidence": 0.85}` |

### 5.2 启动顺序

1. 启动 Redis
2. 启动 FER 服务 (`python server.py --port 8004`)
3. 启动 Go 后端服务

---

## 六、风险评估

| 风险 | 描述 | 缓解措施 |
|------|------|----------|
| FER服务不可用 | 面部情绪分析失败 | 降级为仅使用语音+文字情绪 |
| Redis不可用 | 缓存失败 | 直接使用实时分析结果，不依赖缓存 |
| 网络延迟 | 影响实时性 | 设置合理超时时间（10秒） |
| 情绪冲突 | 多模态情绪不一致 | 按权重融合，置信度加权 |

---

## 七、测试验证

### 7.1 单元测试

- 面部情绪客户端测试
- 多模态融合算法测试
- Redis缓存读写测试

### 7.2 集成测试

- 面部情绪API接口测试
- 流式对话携带面部情绪测试
- 多模态融合结果验证