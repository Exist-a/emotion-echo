# Emotion-Echo · Stage 8c 量表结果查询（已完成 ✅）

> 2026-07-14：完成 assessment-svc 的"提交→查询"业务闭环。
> Stage 8c 是 Stage 8 的最后一块拼图。

## 🏆 战果

| 维度 | 数据 |
|------|------|
| 新增端点 | **2 个**（GetSurveyResult + ListMyResults）|
| Logic | 1 个文件（GetSurveyResultLogic 含 2 个方法）|
| Repo 新方法 | 2 个（GetResult + ListResultsByUser）|
| TDD 测试 | **5 个新增**（全部 PASS）|
| 编译 | ✅ |
| e2e 验证 | ✅ 完整闭环（提交→列表→详情→跨用户鉴权）|

## 🟢🟢🟢 端到端验证证据

```
=== 测试 1：提交 PHQ-9 ===
POST /api/v1/surveys/1/submit
Body: {"answers":{"q1":1,"q2":2,...},"durationSec":60}
→ {"resultId":2,"totalScore":6,"answered":5,"riskLevel":"high"} ✅

=== 测试 2：提交 GAD-7 ===
POST /api/v1/surveys/2/submit
→ {"resultId":3,"totalScore":8,"answered":4,"riskLevel":"high"} ✅

=== 测试 3：列出我的所有结果 ===
GET /api/v1/surveys/results
→ {"items":[
   {"resultId":3, "surveyId":2, "totalScore":8, "riskLevel":"high", "submittedAt":...},
   {"resultId":2, "surveyId":1, "totalScore":6, "riskLevel":"high", "submittedAt":...},
   {"resultId":1, "surveyId":1, "totalScore":15, "riskLevel":"high", "submittedAt":...}
  ],"total":3}
✅ 3 条按 submittedAt DESC 排序

=== 测试 4：user=1 查自己的 result=2 ===
GET /api/v1/surveys/results/2
→ {"resultId":2,"surveyId":1,"userId":1,"totalScore":6,
   "riskLevel":"high","durationSec":60,
   "answers":{"q1":1,"q2":2,"q3":0,"q4":1,"q5":2},
   "submittedAt":...} ✅

=== 测试 5：跨用户鉴权（user=99 查 user=1 的 result=2）===
GET /api/v1/surveys/results/2 (user=99 JWT)
→ HTTP 404 ✅ 鉴权拦截，不是自己的结果视为不存在

=== 测试 6：无 JWT 鉴权 ===
GET /api/v1/surveys/results/2 (无 Authorization)
→ HTTP 401 ✅
```

## 📁 改动文件清单

### 修改
- `emotion-echo-assessment-svc/internal/repository/survey_repository.go`
  - 接口加 `GetResult(ctx, resultID, userID)`
  - 接口加 `ListResultsByUser(ctx, userID, limit)`
  - InMemorySurveyRepo.GetResult（含 userID 鉴权）
  - InMemorySurveyRepo.ListResultsByUser
  - PostgresSurveyRepo.GetResult（WHERE id=? AND user_id=?）
  - PostgresSurveyRepo.ListResultsByUser
  - **ErrNotFound 文案更新**：`survey not found` → `survey result not found`
- `emotion-echo-assessment-svc/internal/types/types.go`
  - 新增 `GetSurveyResultReq`
  - 新增 `SurveyResultItem`（列表项）
  - 新增 `GetSurveyResultResp`（详情）
  - 新增 `ListMyResultsReq/Resp`
- `emotion-echo-assessment-svc/main.go`
  - 注册 `r.GET("/api/v1/surveys/results", ...)`
  - 注册 `r.GET("/api/v1/surveys/results/:resultId", ...)`

### 新增
- `emotion-echo-assessment-svc/internal/logic/getsurveyresultlogic.go`
  - `GetSurveyResult(req)` 方法
  - `ListMyResults(req)` 方法
- `emotion-echo-assessment-svc/internal/logic/survey_logic_test.go`（追加 5 个测试）
- `deploy/apisix/route-survey-results.json`（APISIX 路由配置，已 PUT 到 etcd）
- `docs/stage-8c-survey-results.md`（本文档）

## 🎯 API 设计

```
GET /api/v1/surveys/results
Authorization: Bearer <JWT>
→ 200 {"items":[...],"total":N}

GET /api/v1/surveys/results/:resultId
Authorization: Bearer <JWT>
→ 200 {
   "resultId":2,"surveyId":1,"userId":1,
   "totalScore":6,"riskLevel":"high","durationSec":60,
   "answers":{"q1":1,"q2":2,...},
   "submittedAt":1784040750856
  }
→ 404 （结果不存在 或 不是自己的）
→ 401 （无 JWT）
```

## 🎯 鉴权设计（关键洞察）

**问题**：用户 A 不能看用户 B 的量表结果。

**两层防御**：

| 层 | 实现 |
|----|------|
| **handler** | 调 `logic.GetSurveyResult(req)`，传 user_id 从 ctx 取出 |
| **logic** | 把 user_id 传给 `repo.GetResult(resultID, userID)` |
| **repo** | SQL: `WHERE id = ? AND user_id = ?`（双重过滤） |

```sql
-- InMemory
res := r.results[resultID]
if res.UserID != userID { return nil, nil }  // 跨用户视为不存在

-- Postgres  
err := db.Where("id = ? AND user_id = ?", resultID, userID).First(&res).Error
```

**为什么用 404 而不是 403**：
- 403 暴露资源存在性（攻击者可探测 ID）
- 404 一致地表达"不存在或无权访问"

## 🎯 TDD 测试覆盖（5 个新增）

### GetSurveyResultLogic
- ✅ `TestGetSurveyResultLogic_OwnResult_ReturnsDetail`（happy path）
- ✅ `TestGetSurveyResultLogic_OtherUserResult_ReturnsErrNotFound`（跨用户拦截）
- ✅ `TestGetSurveyResultLogic_NoUserID_Unauthorized`（无 user_id）
- ✅ `TestGetSurveyResultLogic_ZeroResultID_ValidationError`（ID=0）

### ListMyResultsLogic
- ✅ `TestListMyResultsLogic_OnlyReturnsOwnResults`（多用户隔离）

## 🎯 APISIX 路由配置

```json
{
  "id": "r-survey-results",
  "uri": "/api/v1/surveys/results*",
  "methods": ["GET"],
  "upstream_id": "assessment-svc",
  "plugins": {
    "jwt-auth":      { "key": "user_jwt" },
    "limit-count":   { "count": 60, "time_window": 60, "key": "X-User-Id" },
    "api-breaker":   { "unhealthy": {...}, "healthy": {...} }
  }
}
```

## 🎓 关键设计洞察

### 1. **user_id 鉴权下沉到 repo**

| 层 | 检查 |
|----|------|
| handler | 无（解析 path param）|
| logic | 检查 ctx 有 user_id，调 repo 时传入 |
| **repo** | **WHERE id=? AND user_id=?（核心防御）** |

即使 logic 有 bug 漏传 user_id，repo 也不会返回他人的数据。**defense in depth**。

### 2. **404 而非 403**

返回 403 暴露资源存在性；返回 404 不泄露。Stage 6 jwt-auth 已经处理"无 JWT"，但"有 JWT 但无权" 走 404 更安全。

### 3. **共享 ListMyResults 模式**

```go
func (l *GetSurveyResultLogic) ListMyResults(...) {
    // 直接调 ListResultsByUser(uid, limit)
}
```

按 user_id 列结果，所有权是查询的内置条件——无需额外鉴权。

### 4. **APISIX route uri 通配符**

```
"uri": "/api/v1/surveys/results*"
```

匹配 `/api/v1/surveys/results` 和 `/api/v1/surveys/results/123`（一个路由覆盖两个端点）。

## ⚠️ 过程中的坑

1. **APISIX 502 Bad Gateway 重启后恢复**
   - upstream 配置没变，但 APISIX 重启前可能缓存了 stale 配置
   - 解决：`docker restart emotion-echo-apisix`
2. **Bash / PowerShell 路径处理不同**
   - 第一次 `curl --data-binary @path` 用单反斜杠失败
   - 用双反斜杠 `\\\\` 解决

## 📊 项目进度

```
Phase 0 基础设施       ████████████████████ 100% ✅
Phase 1 微服务拆分      ████████████████████ 100% ✅
Phase 2 Kafka          ████████████████████ 100% ✅
Phase 3 LLM 接入       ████████████████████ 100% ✅
Phase 4 业务深化        ████████████████████ 100% ✅ ← 闭环完成
                       PHASE 4 业务深化：
                       - 量表 3 端点 ✅
                       - 量表结果 2 端点 ✅
                       - 完整闭环：列表→详情→提交→查询 ✅
                       - 跨用户鉴权 ✅
Phase 5 Nacos 删除     ████████████████████ 100% ✅
Phase 6 APISIX 安全    ████████████████████ 100% ✅
Phase 7 Gin 化迁移     ████████████████████ 100% ✅
Phase 8 legacy 迁移    ████████░░░░░░░░░░░░░░  40%
                       user-svc UpdateProfile ✅
                       assessment-svc 量表 5 端点 ✅ ← 闭环
                       其他 svc ⏳
Phase 9 K8s           ░░░░░░░░░░░░░░░░░░░░   0%
Phase 10 gRPC          ░░░░░░░░░░░░░░░░░░░░   0%
```

## 🎯 Phase 4 业务深化 100% 完成 🎉

量表业务的完整链路：

```
[客户端]  → [APISIX jwt-auth]
            ↓
       [APISIX limit-count] (60/min/user_id)
            ↓
       [APISIX api-breaker] (3 失败 → 熔断)
            ↓
       [assessment-svc] Gin handler
            ↓
       [logic 层] (鉴权 + 校验 + 业务规则)
            ↓
       [repo 层] (SQL WHERE id=? AND user_id=?)
            ↓
       [Postgres emotion_echo_assessment schema]
```

**业务能力**：
- 用户能浏览 3 个真实心理量表（PHQ-9 / GAD-7 / PSQI）
- 用户能提交作答，得到评分 + 风险等级
- 用户能查自己的历史结果
- **不能看别人的结果**（repo 防御 + 404 一致响应）

## 🎯 下一步候选

| 任务 | 工作量 | 价值 |
|------|--------|------|
| **K8s manifests**（5 svc 各 deployment + service）| 半天 | 生产演进 |
| **proto + gRPC**（ai-svc → llm-service）| 半天 | 跨语言契约 |
| **StreamChat**（ai-svc AI 流式对话）| 2h | 业务增强 |
| **Login**（user-svc 真实登录）| 4h | 用户系统 |
| **API 文档**（OpenAPI）| 半天 | 开发者体验 |

**推荐**：先做 **K8s manifests**——项目准备进入部署阶段。

要继续做哪个？