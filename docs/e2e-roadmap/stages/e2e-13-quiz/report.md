---
stage: e2e-13
title: 心理测验链路修复
executed: 2026-09-20
status: done
environment: dev 模式（17 容器 healthy，compose.apps.yml + .env.local）
---

# E2E-13 执行记录（report）

> 本轮性质：**全链路修复 + 端到端验证**。心理测验链路此前 100% 失败（三层契约错位 + 无种子数据），
> 本轮一次性修通全部 6 处错位 + 创建种子数据 + BFF 架构调整（gRPC→HTTP 绕过）+ 回归钉。

## 1. 环境基线

- 启动命令：`cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml -f compose.dev.yml --env-file .env.local up -d`
- 容器状态：17/17 healthy
- 声明的配置差异：无（`NUXT_PUBLIC_DISABLE_AUTH=false` 正常模式）
- 前端镜像：`emotion-echo/web:v0.1.0`，验收前已重建（E2E-F-70）
- BFF 镜像：`emotion-echo/web-bff:v0.1.18`，验收前已重建

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 |
|---|--------|------|------|------|
| 1 | 量表种子数据已就位（surveys 表 ≥ 2 行，PHQ-9 + GAD-7） | [A] | PASS | `SELECT count(*) FROM emotion_echo_assessment.surveys` → 2 行 |
| 2 | 种子数据结构正确（questions JSONB 含 q1~qN，与 scorer 一致） | [A] | PASS | API `GET /api/v1/surveys/1` → questions 数组 9 题，每题 id="q1"~"q9"，options 含 score |
| 3 | 列表页正常加载并显示量表卡片 | [A]+[V] | PASS | Playwright #3；`screenshots/01-quiz-list-1280x720.png` |
| 4 | 列表项显示标题、描述、题数 | [V] | PASS | Playwright #4；卡片含 title + "9 题"/"7 题" |
| 5 | 点击量表进入答题页，题目正确渲染 | [A]+[V] | PASS | Playwright #5；`screenshots/02-quiz-detail-1280x720.png`；9 个 `.question-block` |
| 6 | 答题后提交成功（不再 400） | [A] | PASS | Playwright #6；HTTP 200 + `riskLevel: "none"` |
| 7 | 提交 answers 格式为 map[string]int | [A] | PASS | Go 契约测试 `TestSurveyHandler_SubmitSurveyHTTP_PreservesAnswerKeys`；断言 body 含 `"q1"` 不含 `"1":` |
| 8 | 结果弹窗显示分数和风险等级 | [A]+[V] | PASS | Playwright #8；`screenshots/03-quiz-result-1280x720.png`；含"总分"+"等级" |
| 9 | 风险等级为中文可读文本 | [A] | PASS | Playwright #9；`riskLevelText` 匹配 `/正常\|轻度\|中度\|重度\|极重度/` |
| 10 | 刷新列表页后可再次答题 | [A] | PASS | Playwright #10；reload 后"开始答题"按钮可见可点击 |
| 11 | 结果查询接口返回正确数据 | [A] | PASS | Playwright #11；`GET /surveys/results/:id` → `riskLevel` + `totalScore` 非空 |
| 12 | 不存在的量表 ID 返回友好错误 | [A] | PASS | Playwright #12；`/question/99999` 无白屏；API 返回 404 |

汇总：**PASS 12 / FAIL 0 / BLOCKED 0 / N/A 0**

> `BLOCKED` = 0，未触发 RUNBOOK §4「BLOCKED > 1/3 不得判 done」。

### 截图清单（`screenshots/`）

| 文件 | 视口 | 覆盖 |
|------|------|------|
| `01-quiz-list-1280x720.png` | 1280×720 | #3（列表页，2 个量表卡片） |
| `02-quiz-detail-1280x720.png` | 1280×720 | #5（答题页，9 题 + 4 选项） |
| `03-quiz-result-1280x720.png` | 1280×720 | #8（结果弹窗，总分 + 等级） |

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| E2E-F-02：6 处契约错位（M1~M6） | 范围内 | 修复（PR #33）；账本 E2E-F-02 ✅ |
| E2E-F-03：种子数据不存在 | 范围内 | 修复（PR #33）；账本 E2E-F-03 ✅ |
| gRPC 转换丢失 JSONB 数据 | 范围内 | BFF survey 5 端点走 HTTP 绕过；ADR `adr-2026-09-survey-http-bypass.md` |
| questions map→数组顺序不确定 | 范围内 | BFF `getSurveyHTTP` 加 `sort.Strings(keys)` |
| BFF `getSurveyHTTP` 用 `c.JSON` 绕过响应包装 | 范围内（自身引入） | 改为 `OK(c, raw)` 保持 `{code,data,message}` 一致 |

## 4. 修复清单（TDD 记录）

| commit | 内容 | 先行的失败测试 |
|--------|------|---------------|
| `c64a5ba` (PR #33 squash) | 种子数据 + M1~M6 全部修复 + BFF HTTP 绕过 + ADR | Playwright #6-9 提交恒 400（M1）、结果等级恒空（M2）、列表取数失败（M3） |
| `c64a5ba` (同 commit) | Playwright quiz.spec.ts 18/18 + Go 契约测试 10/10 | 修复前 spec 全部 RED |
| 本轮追加 | questions 排序 + 镜像全量重建 | smoke §8 8 个 STALE |

## 5. 回归钉

- 新增 spec：`emotion-echo-web/e2e/quiz.spec.ts`（用例 9，首次运行结果：绿 18/18）
- Go 契约测试：`survey_handler_test.go` 新增 3 条（HTTP 绕过路径）
- 总计：Playwright 18/18 + Go 10/10 + vitest 427/427

## 6. 边界测试（补充验证）

| 边界 | 预期 | 实际 | 结果 |
|------|------|------|------|
| 空答案 `{}` | 400 | "answers cannot be empty" | ✅ |
| 答案数不足（5/9） | 400 | "requires exactly 9 answers, got 5" | ✅ |
| 分数超范围（5, max 3） | 400 | "must be 0-3, got 5" | ✅ |
| 负数分数（-1） | 400 | "must be 0-3, got -1" | ✅ |
| 不存在的量表 | 404 | "survey not found" | ✅ |
| 非法 JSON body | 400 | parse error | ✅ |
| 未登录无 token | 401 | "Missing JWT token" | ✅ |
| 旧格式 answers 数组 | 400 | "cannot unmarshal array into map" | ✅ |
| 无效 ID 格式 | 400 | "invalid survey id" | ✅ |
| 查别人的 result | 404 | "result not found"（权限隔离） | ✅ |

## 7. 待决策 / 升级项

- **gRPC survey 死代码**：BFF `assessmentGRPCClient.GetSurvey/SubmitSurvey/GetResult/ListResults` 不再被调用。短期可接受，长期需扩展 proto 或统一回 gRPC。
- **questions 排序**：已用 `sort.Strings` 修复，但依赖 key 格式（"q1"~"q9"）的字典序。若未来 key 不是 q+数字，需改为自然排序。

## 8. 收口自检

- [x] `git status` 干净
- [x] `git status -sb` main 与 origin/main 无 ahead/behind
- [x] `git branch --merged main` 除 main 外为空
- [x] `e2e_stage_audit.py --all` → 0 FAIL
- [x] smoke 24/24 PASS（镜像全量重建后）
- [x] TDD 门禁 GREEN
- [x] ADR 门禁 GREEN
