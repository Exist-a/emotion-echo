---
status: shifted
superseded-by: stage-7-gin-migration.md + 5 微服务架构（user/chat/ai/analytics/assessment）
original-path: .trae/documents/后端拆分规划.md
original-date: 2026-06-XX
migrated-at: 2026-09-03
round: 2-A
---

# Emotion-Echo-Gin 后端拆分规划

## 一、当前状态总览

### 1.1 Service 文件行数统计

| 文件 | 行数 | 状态 | 拆分优先级 |
|------|------|------|-----------|
| `auth_service.go` | 317 | ⚠️ 未拆分 | 🔴 高 |
| `oauth_service.go` | 301 | ⚠️ 未拆分 | 🔴 高 |
| `ai_stream.go` | 253 | ✅ 已拆分 | - |
| `user_behavior_service.go` | 254 | ⚠️ 未拆分 | 🟡 中 |
| `mental_health_service.go` | 250 | ⚠️ 未拆分 | 🟡 中 |
| `survey_service.go` | 232 | ⚠️ 未拆分 | 🟡 中 |
| `survey_scoring.go` | 215 | ⚠️ 未拆分 | 🟢 低 |
| `ai_service.go` | 181 | ✅ 已拆分 | - |
| `voice_service.go` | 180 | ⚠️ 未拆分 | 🟡 中 |
| `message_service.go` | 165 | ⚠️ 未拆分 | 🟢 低 |
| `conversation_service.go` | 152 | ⚠️ 未拆分 | 🟢 低 |

---

## 二、拆分计划

### Phase 1: 认证服务拆分 (auth_service.go)

**目标**: 将认证服务拆分为独立模块

**当前问题**:
- 混合了注册、登录、刷新Token、登出
- Token生成与验证逻辑混合
- 密码哈希与验证混合

**拆分方案**:

```
service/
├── auth_service.go           # 保留核心入口，委托给专用品
├── auth_types.go             # 认证相关类型定义
├── auth_token.go             # Token生成与刷新逻辑
├── auth_register.go          # 注册逻辑
├── auth_login.go             # 登录逻辑
└── auth_password.go          # 密码哈希与验证
```

**预计效果**:
- `auth_service.go`: 317行 → 120行 (-62%)
- 新增 4 个文件，每个约 50-80 行

---

### Phase 2: OAuth 服务拆分 (oauth_service.go)

**目标**: 将微信OAuth服务拆分为独立模块

**当前问题**:
- 微信API调用与业务逻辑混合
- HTTP客户端配置内嵌
- Token交换逻辑与用户创建逻辑混合

**拆分方案**:

```
service/
├── oauth_service.go          # 保留核心入口
├── oauth_wechat_types.go     # 微信OAuth类型定义
├── oauth_wechat_api.go       # 微信API调用
├── oauth_user_link.go        # 用户关联逻辑
└── oauth_token.go            # OAuth Token处理
```

**预计效果**:
- `oauth_service.go`: 301行 → 150行 (-50%)
- 新增 4 个文件，每个约 40-70 行

---

### Phase 3: 心理健康服务拆分 (mental_health_service.go)

**目标**: 拆分心理健康评估服务

**当前问题**:
- 评估计算与状态管理混合
- 定时任务与业务逻辑混合
- 多量表评分逻辑集中

**拆分方案**:

```
service/
├── mental_health_service.go  # 保留核心入口
├── mental_health_types.go    # 类型定义
├── mental_health_scheduler.go # 定时调度逻辑
├── mental_health_assessor.go # 评估计算逻辑
└── mental_health_notifier.go # 通知逻辑
```

**预计效果**:
- `mental_health_service.go`: 250行 → 100行 (-60%)
- 新增 4 个文件

---

### Phase 4: 语音服务拆分 (voice_service.go)

**目标**: 拆分语音处理服务

**当前问题**:
- 文件上传与转录混合
- 多种语音服务调用混杂
- 临时文件管理内嵌

**拆分方案**:

```
service/
├── voice_service.go          # 保留核心入口
├── voice_types.go            # 类型定义
├── voice_uploader.go         # 文件上传处理
├── voice_transcriber.go      # 转录服务调用
└── voice_cleanup.go          # 临时文件清理
```

**预计效果**:
- `voice_service.go`: 180行 → 80行 (-56%)
- 新增 4 个文件

---

### Phase 5: 其他服务优化

#### 5.1 survey_service.go + survey_scoring.go

**拆分方案**: 合并为一个模块

```
service/
├── survey_service.go         # 保留主入口
├── survey_types.go           # 类型定义
├── survey_scoring.go         # 评分逻辑（合并）
└── survey_questions.go       # 问卷题目管理
```

#### 5.2 message_service.go

**拆分方案**: 保持现状或拆分工具函数

```
service/
├── message_service.go       # 保持
├── message_types.go          # 类型定义
└── message_filter.go         # 消息过滤逻辑（可选）
```

#### 5.3 conversation_service.go

**拆分方案**: 保持现状或拆分分组逻辑

```
service/
├── conversation_service.go    # 保持
├── conversation_types.go      # 类型定义
└── conversation_grouper.go   # 会话分组逻辑（可选）
```

---

## 三、实施顺序

### 第一阶段（推荐立即执行）

| 序号 | 任务 | 工作量 | 风险 |
|------|------|--------|------|
| 1 | Phase 1: auth_service.go 拆分 | 高 | 中 |
| 2 | Phase 2: oauth_service.go 拆分 | 高 | 中 |

**原因**:
- 认证是核心模块，拆分后对其他模块影响小
- OAuth 相对独立，依赖少

### 第二阶段（建议执行）

| 序号 | 任务 | 工作量 | 风险 |
|------|------|--------|------|
| 3 | Phase 3: mental_health_service.go 拆分 | 中 | 低 |
| 4 | Phase 4: voice_service.go 拆分 | 中 | 低 |

### 第三阶段（可选）

| 序号 | 任务 | 工作量 | 风险 |
|------|------|--------|------|
| 5 | Phase 5: survey_service 优化 | 低 | 低 |
| 6 | Phase 5: message_service 优化 | 低 | 低 |
| 7 | Phase 5: conversation_service 优化 | 低 | 低 |

---

## 四、拆分原则

### 4.1 通用原则

1. **单一职责**: 每个文件只负责一个功能领域
2. **接口稳定**: 保持公开接口不变，对外暴露的方法不变
3. **依赖注入**: 通过构造函数注入依赖
4. **向后兼容**: 拆分后不影响现有调用方

### 4.2 文件命名规范

```
{domain}_{sub_domain}.go
例如:
- auth_service.go (主入口)
- auth_token.go (Token相关)
- auth_register.go (注册相关)
```

### 4.3 代码组织规范

```go
// auth_token.go

// ============ 类型定义 ============
type TokenRequest struct { ... }
type TokenResponse struct { ... }

// ============ 公共函数 ============
func GenerateToken(...) { ... }
func ValidateToken(...) { ... }

// ============ 私有辅助函数 ============
func hashToken(...) { ... }
```

---

## 五、预期成果

### 5.1 代码行数优化

| 指标 | 拆分前 | 拆分后 | 优化率 |
|------|--------|--------|--------|
| 最大单文件行数 | 317 (auth) | ~120 | -62% |
| Service 平均行数 | ~220 | ~100 | -55% |
| 总代码行数 | ~3500 | ~3200 | -8% |

### 5.2 架构改进

| 改进项 | 说明 |
|--------|------|
| 可维护性 | 每个模块独立，修改影响范围小 |
| 可测试性 | 独立函数更易单元测试 |
| 可扩展性 | 新增功能只需添加新文件 |
| 可读性 | 文件职责单一，代码更易理解 |

### 5.3 风险控制

| 风险 | 缓解措施 |
|------|----------|
| 接口不一致 | 保持公开方法签名不变 |
| 循环依赖 | 遵循依赖方向：主入口 → 专用品 |
| 性能下降 | 避免不必要的函数调用链 |
| 编译失败 | 每步拆分后立即编译验证 |

---

## 六、注意事项

1. **保持向后兼容**: 所有公开方法保持原有签名
2. **逐步实施**: 每完成一个文件拆分立即编译验证
3. **及时沟通**: 拆分过程中发现设计问题及时讨论
4. **代码审查**: 拆分完成后进行代码审查

---

## 七、后续优化方向

拆分完成后，可进一步考虑：

1. **领域驱动设计 (DDD)**: 将 service 层进一步重组为 domain 包
2. **事件驱动**: 引入事件总线解耦模块间通信
3. **中间件优化**: 提取公共中间件逻辑
4. **错误处理统一**: 建立统一的错误处理模式
