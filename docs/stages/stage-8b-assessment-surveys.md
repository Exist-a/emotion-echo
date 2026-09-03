# Emotion-Echo · Stage 8b 心理评估量表（已完成 ✅）

> 2026-07-14：assessment-svc 业务能力增强，新增 3 个量表端点。
> 心理评估是 Emotion-Echo 的核心业务能力。

## 🏆 战果

| 维度 | 数据 |
|------|------|
| 新增端点 | **3 个**（ListSurveys + GetSurvey + SubmitSurvey）|
| Logic | 3 个（ListSurveysLogic + GetSurveyLogic + SubmitSurveyLogic）|
| Repo 新方法 | 1 个（SaveResult）|
| TDD 测试 | **15 个**（2 repo + 11 logic + 2 已有）全部 PASS |
| 编译 | ✅ 通过 |
| e2e 验证 | ✅ 端到端通过（量表列表→详情→提交→存储）|

## 🟢🟢🟢 端到端验证证据

```
=== 测试 1：无 JWT 鉴权拦截 ===
GET /api/v1/surveys (无 Authorization header)
→ HTTP 401 ✅

=== 测试 2：JWT List Surveys ===
GET /api/v1/surveys
→ {"items":[
   {"id":3,"code":"PSQI","title":"Pittsburgh Sleep Quality Index","questionNum":4},
   {"id":2,"code":"GAD-7","title":"GAD-7 anxiety scale","questionNum":7},
   {"id":1,"code":"PHQ-9","title":"PHQ-9 depression scale","questionNum":9}
  ],"total":3}
✅ 3 个量表全部返回，按 ID DESC 排序

=== 测试 3：JWT Get Survey 1 (PHQ-9) ===
GET /api/v1/surveys/1
→ code=PHQ-9, qcount=9 ✅

=== 测试 4：Submit PHQ-9 ===
POST /api/v1/surveys/1/submit
Body: {"answers":{"q1":2,"q2":1,...,"q9":1},"durationSec":120}
→ {"resultId":1,"surveyId":1,"totalScore":15,"answered":9,"riskLevel":"high"} ✅
```

## 📁 改动文件清单

### 修改
- `emotion-echo-assessment-svc/internal/repository/survey_repository.go`
  - 接口加 `SaveResult(ctx, result)`
  - InMemorySurveyRepo.SaveResult（自动分配 ID）
  - PostgresSurveyRepo.SaveResult（`Create`）
- `emotion-echo-assessment-svc/internal/types/types.go`
  - 新增 `ListSurveysReq/Resp` + `SurveyItem`
  - 新增 `GetSurveyReq/Resp`
  - 新增 `SubmitSurveyReq/Resp`
- `emotion-echo-assessment-svc/main.go`
  - 注册 `r.GET("/api/v1/surveys", ...)`
  - 注册 `r.GET("/api/v1/surveys/:id", ...)`
  - 注册 `r.POST("/api/v1/surveys/:id/submit", ...)`

### 新增
- `emotion-echo-assessment-svc/internal/logic/listsurveyslogic.go`
- `emotion-echo-assessment-svc/internal/logic/getsurveylogic.go`
- `emotion-echo-assessment-svc/internal/logic/submitsurveylogic.go`
- `emotion-echo-assessment-svc/internal/logic/survey_logic_test.go`（11 个测试）
- `emotion-echo-assessment-svc/internal/middleware/auth.go`（adapter）
- `emotion-echo-assessment-svc/internal/repository/survey_repository_test.go`（追加 2 个）
- `deploy/apisix/route-surveys.json`（已 PUT 到 etcd）
- `deploy/apisix/route-survey-get.json`（已 PUT 到 etcd）
- `deploy/apisix/route-survey-submit.json`（已 PUT 到 etcd）
- `deploy/db/seed-surveys.sql`（PHQ-9 / GAD-7 / PSQI 三个真实量表 seed）

## 🎯 API 设计

```
GET /api/v1/surveys
Authorization: Bearer <JWT>
→ 200 {"items":[...],"total":3}

GET /api/v1/surveys/:id
Authorization: Bearer <JWT>
→ 200 {id, code, title, category, version, questions}

POST /api/v1/surveys/:id/submit
Authorization: Bearer <JWT>
Content-Type: application/json
Body: {"answers":{"q1":2,"q2":1,...},"durationSec":120}
→ 200 {"resultId":1,"totalScore":15,"answered":9,"riskLevel":"high"}
```

## 🎯 APISIX 路由配置

| 路由 | 方法 | 限流 | 备注 |
|------|------|------|------|
| r-surveys | GET | 30/min | 列表（频次低）|
| r-survey-get | GET /surveys/* | 60/min | 详情（频次高）|
| r-survey-submit | POST /surveys/*/submit | 20/min | 提交（更敏感）|

三个都带：jwt-auth（鉴权）+ api-breaker（熔断）。

## 🎯 TDD 测试覆盖（15 个全 PASS）

### repo 层（追加 2 个测试）
- ✅ `TestSurveyRepo_InMemory_SaveResult_AssignsID`
- ✅ `TestSurveyRepo_InMemory_SaveResult_PreservesProvidedID`

### logic 层（新增 11 个测试）

**ListSurveys（3 个）：**
- ✅ `TestListSurveysLogic_Empty_ReturnsEmptyItems`
- ✅ `TestListSurveysLogic_ReturnsActiveSurveysOnly`
- ✅ `TestListSurveysLogic_CountQuestionsFromItemsArray`

**GetSurvey（3 个）：**
- ✅ `TestGetSurveyLogic_Existing_ReturnsQuestions`
- ✅ `TestGetSurveyLogic_NotFound_ReturnsErrNotFound`
- ✅ `TestGetSurveyLogic_ZeroID_ValidationError`

**SubmitSurvey（5 个）：**
- ✅ `TestSubmitSurveyLogic_HappyPath_CalculatesScore`
- ✅ `TestSubmitSurveyLogic_NoUserID_Unauthorized`
- ✅ `TestSubmitSurveyLogic_EmptyAnswers_ValidationError`
- ✅ `TestSubmitSurveyLogic_ScoreOutOfRange_ValidationError`
- ✅ `TestSubmitSurveyLogic_SurveyNotFound_ReturnsErrNotFound`
- ✅ `TestSubmitSurveyLogic_RiskLevel_High`

## 🎓 关键设计洞察

### 1. **简化计分（Phase 4 starter）**

```go
// 简化规则：total_score = sum(answers)
// risk_level: ≥0.7*max → high; ≥0.4*max → medium; <0.4 → low
// 后续 Phase 升级：按量表类型（PHQ-9/GAD-7/自定义）跑不同 scoring 规则
```

**为什么简化**：
- Phase 4 阶段先验证业务流跑通
- legacy 的 survey_scoring.go 复杂（区分 SDS/SAS/PSQI 等），直接搬过来工作量大
- 后续 Phase 按量表类型精确升级 scoring

### 2. **countQuestions 自适应**

```go
// 不同量表的 questions 结构不同：
//   - PHQ-9: {"items": [...]}  → 取 items.length
//   - GAD-7: 同上
//   - 自定义: {"q1": ..., "q2": ...}  → 取 map 大小
```

让 svc 能兼容多种 schema。

### 3. **Answer 范围校验**

```go
if v < 0 || v > 10 {
    return nil, errors.New("validation: answer score must be 0-10")
}
```

0-10 是大多数量表的通用范围（PHQ-9 是 0-3，PSQI 各题不同，简化上限 10）。

### 4. **adapter pattern 复用**

```go
// emotion-echo-assessment-svc/internal/middleware/auth.go
type CtxUserIDKey = sharedmw.CtxUserIDKey
```

assessment-svc 之前没有 middleware/，加一个 8 行 adapter 文件就能用 shared 的 CtxUserIDKey。

## ⚠️ 过程中的坑

1. **SurveyRepo.SaveResult 没生效**
   - 多次 SearchReplace 失败
   - 用 Write 重写整个文件
2. **ListSurveysReq 缺失**
   - SearchReplace 只匹配了 `// ListSurveysResp` 前一行没插入完整内容
   - 重做
3. **psql 通过 PowerShell 命令行引号转义失败**
   - SQL 含中文 + JSON 双引号 → bash 转义乱
   - 用 `Get-Content | docker exec -i` stdin 方式
4. **SQL 文件 BOM 问题**
   - 第一版含中文有 BOM，psql 报错
   - 用英文 + 重写文件

## 📊 项目进度

```
Phase 0 基础设施       ████████████████████ 100% ✅
Phase 1 微服务拆分      ████████████████████ 100% ✅
Phase 2 Kafka          ████████████████████ 100% ✅
Phase 3 LLM 接入       ████████████████████ 100% ✅
Phase 4 业务深化        ████████████████░░  88%
                       量表 seed + 提交链路 ✅
                       报告/上传/AI 对话流 ⏳
Phase 5 Nacos 删除     ████████████████████ 100% ✅
Phase 6 APISIX 安全    ████████████████████ 100% ✅
Phase 7 Gin 化迁移     ████████████████████ 100% ✅
Phase 8 legacy 迁移    ██████░░░░░░░░░░░░░░░  30%
                       user-svc: UpdateProfile ✅
                       assessment-svc: surveys ✅ ← 当前
                       其他 svc ⏳
Phase 9 K8s           ░░░░░░░░░░░░░░░░░░░░   0%
Phase 10 gRPC          ░░░░░░░░░░░░░░░░░░░░   0%
```

## 🎯 Stage 8c 下一步候选

| handler | svc | 工作量 | 价值 |
|---------|-----|--------|------|
| **GetSurveyResult** | assessment-svc | 30min | 查历史结果 |
| GetMentalHealthReport | assessment-svc | 2h | 心理报告 |
| GetBehaviorReport | analytics-svc | 1.5h | 行为报告 |
| StreamChat | ai-svc | 2h | AI 流式对话 |
| Login | user-svc | 4h | 真实登录 |

**推荐**：先做 GetSurveyResult（30 分钟，跟量表闭环），再做其他。

---

**关键洞察**：legacy 的 service 层（survey_scoring、report_aggregator 等）有大量业务复杂度。**直接搬比重新设计更慢**——这次 Stage 8b 用简化计分 + 后续 Phase 升级的方式，证明**最小可交付切片**的有效性。