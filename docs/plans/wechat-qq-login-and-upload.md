---
status: planned
superseded-by: 尚未完整实施；前端 ChatFile 组件在 backlog
original-path: .trae/documents/微信QQ登录和文件上传实施计划.md
original-date: 2026-07-XX
migrated-at: 2026-09-03
round: 2-C
---

# 微信/QQ登录 + 通用文件上传 功能实施计划

## 一、现状分析

### 1.1 微信登录 - 已完成 ✅
- **代码已实现**：
  - [oauth_handler.go](file:///d:\源码\Emotion-Echo\Emotion-Echo-Gin\internal\handler\oauth_handler.go) - OAuth 处理器
  - [oauth_service.go](file:///d:\源码\Emotion-Echo\Emotion-Echo-Gin\internal\service\oauth_service.go) - 微信 OAuth Service
  - [oauth_wechat_api.go](file:///d:\源码\Emotion-Echo\Emotion-Echo-Gin\internal\service\oauth_wechat_api.go) - 微信 API 调用
  - [oauth_types.go](file:///d:\源码\Emotion-Echo\Emotion-Echo-Gin\internal\service\oauth_types.go) - 微信登录相关类型
- **路由已注册**：`GET /api/v1/auth/oauth/wechat/url`、`POST /api/v1/auth/oauth/wechat/login`
- **数据模型已支持**：User 模型有 `WechatOpenID`、`WechatUnionID` 字段
- **配置已支持**：config.example.yaml 有 `oauth.wechat_app_id` 等配置

### 1.2 QQ登录 - 未实现 ❌
- 无任何 QQ OAuth 相关代码
- User 模型无 QQ 相关字段
- 配置文件无 QQ 相关配置

### 1.3 文件上传 - 部分实现 ⚠️
- **已实现**：
  - 头像上传（[user_handler.go:67-138](file:///d:\源码\Emotion-Echo\Emotion-Echo-Gin\internal\handler\user_handler.go#L67-138)）
  - 语音上传（[voice_handler.go](file:///d:\源码\Emotion-Echo\Emotion-Echo-Gin\internal\handler\voice_handler.go)）
- **未实现**：通用文件上传功能（用于聊天中的图片、文件等）

---

## 二、实施计划

### 任务一：QQ 登录功能

#### 1. 数据模型扩展
**文件**：`internal/models/user.go`
- 新增字段：
  - `QQOpenID string` - QQ OpenID
  - `QQUnionID string` - QQ UnionID（如果 QQ 支持）

#### 2. QQ OAuth 类型定义
**文件**：`internal/service/oauth_types.go`（追加）
```go
// QQ OAuth 请求/响应类型
type GetQQAuthURLRequest struct{ ... }
type GetQQAuthURLResponse struct{ ... }
type QQLoginRequest struct{ ... }
type QQLoginResponse struct{ ... }
type QQUserInfo struct{ ... }
type QQTokenResponse struct{ ... }
```

#### 3. QQ API 服务
**新建文件**：`internal/service/oauth_qq_api.go`
- 实现内容：
  - `GetAuthURL()` - 获取 QQ 授权 URL
  - `ExchangeCodeForToken()` - 通过 code 换取 access_token
  - `GetUserInfo()` - 获取用户信息

#### 4. QQ OAuth Service
**新建文件**：`internal/service/oauth_qq_service.go`
- 实现内容：
  - `GetAuthURL()` - 获取授权 URL
  - `LoginByCode()` - 通过授权码登录，创建/更新用户，生成 JWT

#### 5. QQ OAuth Handler
**文件**：`internal/handler/oauth_handler.go`（追加方法）
- 新增方法：
  - `GetQQAuthURL()` - `GET /auth/oauth/qq/url`
  - `QQLogin()` - `POST /auth/oauth/qq/login`

#### 6. 路由注册
**文件**：`internal/router/router.go`
- 在公开接口组新增：
  ```go
  auth.GET("/oauth/qq/url", r.oauthHandler.GetQQAuthURL)
  auth.POST("/oauth/qq/login", r.oauthHandler.QQLogin)
  ```

#### 7. 配置文件更新
**文件**：`configs/config.example.yaml`
```yaml
oauth:
  qq_app_id: ''
  qq_app_key: ''
  qq_redirect_uri: 'http://localhost:3000/auth/callback'
```

#### 8. 配置结构更新
**文件**：`internal/config/config.go`
- 在 OAuthConfig 结构体中新增：
  ```go
  QQAppID     string
  QQAppKey    string
  QQRedirectURI string
  ```

#### 9. 依赖注入更新
**文件**：`cmd/server/main.go`
- 创建 QQOAuthService 实例
- 注入到 OAuthHandler

---

### 任务二：通用文件上传功能

#### 1. 文件上传 Service
**新建文件**：`internal/service/upload_service.go`
- 实现内容：
  - `UploadImage()` - 通用图片上传（支持 jpg/png/webp/gif，最大 5MB）
  - `UploadFile()` - 通用文件上传（支持任意类型，最大 20MB）
  - `UploadVideo()` - 视频上传（支持 mp4/avi，最大 50MB）
  - 文件存储策略：本地存储 / OSS（阿里云OSS配置扩展）
  - 统一返回文件访问 URL

#### 2. 文件上传 Handler
**新建文件**：`internal/handler/upload_handler.go`
- 实现内容：
  - `UploadImage()` - `POST /upload/image`
  - `UploadFile()` - `POST /upload/file`
  - `UploadVideo()` - `POST /upload/video`
- 统一错误处理、文件大小校验、类型校验

#### 3. 路由注册
**文件**：`internal/router/router.go`
- 在需要认证的接口组新增：
  ```go
  upload := authorized.Group("/upload")
  {
      upload.POST("/image", r.uploadHandler.UploadImage)
      upload.POST("/file", r.uploadHandler.UploadFile)
      upload.POST("/video", r.uploadHandler.UploadVideo)
  }
  ```

#### 4. 存储配置扩展
**文件**：`configs/config.example.yaml`
```yaml
storage:
  type: local  # local | oss
  local:
    path: ./uploads
    base_url: http://localhost:8080/uploads
  oss:
    endpoint: ''
    access_key_id: ''
    access_key_secret: ''
    bucket: ''
    base_url: ''
```

#### 5. 依赖注入更新
**文件**：`cmd/server/main.go`
- 创建 UploadService 实例
- 创建 UploadHandler 实例

---

## 三、实施顺序

```
第一阶段：QQ 登录
1.1 更新 User 模型（添加 QQOpenID 字段）
1.2 配置文件和配置结构更新
1.3 创建 QQ API 服务 (oauth_qq_api.go)
1.4 创建 QQ OAuth Service (oauth_qq_service.go)
1.5 OAuth Handler 添加 QQ 方法
1.6 路由注册 QQ 登录接口
1.7 依赖注入更新
1.8 单元测试

第二阶段：通用文件上传
2.1 创建 Upload Service (upload_service.go)
2.2 创建 Upload Handler (upload_handler.go)
2.3 路由注册上传接口
2.4 依赖注入更新
2.5 单元测试
```

---

## 四、关键文件清单

### 新建文件
| 文件路径 | 说明 |
|---------|------|
| `internal/service/oauth_qq_api.go` | QQ API 服务 |
| `internal/service/oauth_qq_service.go` | QQ OAuth 业务逻辑 |
| `internal/service/upload_service.go` | 通用文件上传服务 |
| `internal/handler/upload_handler.go` | 文件上传处理器 |

### 修改文件
| 文件路径 | 修改内容 |
|---------|---------|
| `internal/models/user.go` | 添加 QQOpenID、QQUnionID 字段 |
| `internal/handler/oauth_handler.go` | 添加 QQ 登录方法 |
| `internal/router/router.go` | 注册 QQ 登录和文件上传路由 |
| `internal/config/config.go` | 添加 QQ OAuth 配置 |
| `configs/config.example.yaml` | 添加 QQ OAuth 和 OSS 配置 |
| `cmd/server/main.go` | 依赖注入更新 |

---

## 五、技术要点

### QQ OAuth 2.0 流程
1. 前端调用 `GET /api/v1/auth/oauth/qq/url?redirectUri=xxx`
2. 后端返回 QQ 授权页面 URL
3. 用户授权后，QQ 回调到前端，前端获取 code
4. 前端调用 `POST /api/v1/auth/oauth/qq/login` 带 code
5. 后端用 code 换取 access_token，再用 token 获取用户信息
6. 后端创建/更新用户，返回 JWT

### 文件上传安全
- 文件大小限制（防止 DOS）
- 文件类型白名单（扩展名 + 魔数校验）
- 文件名随机化（防止覆盖和路径遍历）
- 存储隔离（上传目录与代码目录分离）

---

## 六、待用户确认事项

1. **QQ 登录**：是否需要支持 PC 端和移动端两种scope？
2. **文件存储**：目前先用本地存储，后续是否需要支持阿里云 OSS？
3. **文件上传**：是否有其他特定的文件类型需要支持？
