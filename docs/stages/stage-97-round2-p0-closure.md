---
status: planned
stage: 97
title: Round 2 Code Review P0 收口（基于工作区待提交草稿）
date: 2026-09-15
source-plan: code-review-2026-09-14-round-2.md
depends-on:
  - stage-94 (Round 1 P0 收口)
  - stage-95 (Round 2 收口报告)
  - stage-96 (Round 1 P1/P2 残余收口)
---

# Stage 97 — Round 2 P0 收口（基于工作区待提交草稿）

> **本报告对应 `code-review-2026-09-14-round-2.md` 10 个 P0 全部落地。**
> 工作区原已存在大量 Round 2 修复（working copy），但未 commit。本 Stage 工作 =
> **补缺 + 加测试 + 收口文档**。

## 1. P0 收口矩阵（10/10 ✅）

| # | 标题 | 工作区状态 | 落地证据 | 契约测试 |
|---|------|-----------|----------|----------|
| **P0-R2-1** | JWT 双存储（localStorage + 非 HttpOnly cookie）| ✅ 已修 | BFF `setAccessTokenCookie` HttpOnly；useApi 优先 cookie；APISIX seed `cookie: "access_token"` | `auth_handler_test.go` 4 用例 + `useApi.ts` cookie 路径 |
| **P0-R2-2** | useFaceEmotion 调 orphan `/face/emotion` | ✅ 已修 | 改调 `/multimodal/analyze`（form-data）| `useFaceEmotion.test.ts` 5 字面量契约 |
| **P0-R2-3** | llm-service HTTP 端零鉴权 | ✅ 已修 | CORS 白名单 `credentials=False`；`_check_http_api_key` dependency 挂 `/analyze` | `test_http_routes.py::TestHttpApiKey` 4 用例 |
| **P0-R2-4** | multimodal_handler 无 body size limit | ✅ 已修 | `http.MaxBytesReader(50 << 20)` | `multimodal_handler_test.go::BodyExceedsLimit` |
| **P0-R2-5** | env 名错配 `LLM_INTERNAL_API_KEY` vs `INTERNAL_API_KEY` | ✅ 已修 | ai-svc main.go + ai-api.yaml + deployment.yaml 全部改 `INTERNAL_API_KEY` | `config_override_test.go::TestEnvOverride_InternalAPIKey_P0R2_5` |
| **P0-R2-6** | `daily_emotion_by_modality_v` 缺 GRANT | ✅ 已修 | a004 第 54 行 GRANT；i005 视图存在 | `004_security_test.go::TestAnalyticsReaderRole_DailyEmotionByModalityV_P0R2_6` |
| **P0-R2-7** | messages.conversation_id FK 漂移 + 01/02 重叠 | ✅ 已修 | 02 line 79 FK + CASCADE；01 删 DDL 块 | `chat-svc/migrations/008_p0r27_ddl_drift_test.go` 2 用例 |
| **P0-R2-8** | web Dockerfile prod 阶段无 `USER node` | ✅ 已修 | line 30 `USER node` | `scripts/test_web_dockerfile_p0r28_r29.py` 字面量契约 |
| **P0-R2-9** | web Dockerfile `rm -f package-lock.json` | ✅ 已修 | `npm ci --omit=optional` 保留 lockfile | 同上 |
| **P0-R2-10** | dev compose 不引 env_file | ✅ 已修 | apps.yml 顶层 `x-env-file: &env-file [- .env.local]` + 7 个 svc 引用 | `scripts/test_compose_env_file_p0r210.py` 10 用例 |

## 2. 测试覆盖（新增 6 套，0 回归）

| 测试文件 | 范围 | 用例数 | 状态 |
|---------|------|--------|------|
| `emotion-echo-ai-svc/internal/handler/multimodal_handler_test.go` | body size limit | +1 | ✅ |
| `emotion-echo-ai-svc/internal/config/config_override_test.go` | INTERNAL_API_KEY env 注入 | +1 | ✅ |
| `emotion-echo-analytics-svc/migrations/004_security_test.go` | daily_emotion_by_modality_v GRANT | +1 | ✅ |
| `emotion-echo-chat-svc/migrations/008_p0r27_ddl_drift_test.go` | 01/02 DDL 漂移 + FK | +2 | ✅ |
| `emotion-echo-web-bff/internal/handler/auth_handler_test.go` | JWT cookie (login/logout/refresh) | +4 | ✅ |
| `emotion-echo-web/app/composables/useFaceEmotion.test.ts` | orphan 路径字面量契约 | 5 | ✅ |
| `emotion-llm-service/tests/unit/test_http_routes.py` | /analyze auth 4 用例 + dev mode | +4 | ✅ |
| `scripts/test_web_dockerfile_p0r28_r29.py` | Dockerfile 字面量契约 | 4 | ✅ |
| `scripts/test_compose_env_file_p0r210.py` | compose env_file 契约 | 10 | ✅ |

**总计 +32 测试用例，0 失败。**

### 2.1 全套测试结果

| 范围 | 结果 |
|------|------|
| ai-svc `go test ./...` | ✅ 全绿（含新增 2 用例）|
| analytics-svc `go test ./...` | ✅ 全绿（含新增 1 用例）|
| chat-svc `go test ./...` | ✅ 全绿（含新增 2 用例）|
| web-bff `go test ./...` | ✅ 全绿（含新增 4 用例）|
| llm-service pytest | ✅ 200 passed（含新增 4 用例）|
| web vitest | ✅ useFaceEmotion 5/5 |

## 3. PR 拆分（待 git add + commit，按 Round 2 §5 分组）

| PR | 范围 | 文件数 | 工作量 | commit message |
|---|------|--------|--------|----------------|
| **PR-1** | LLM 链路兜底 | 4 | S | `fix(ai-svc/llm-service): P0-R2-3 + P0-R2-5 — llm-service HTTP 鉴权 + env 名统一` |
| **PR-2** | multimodal DoS 防护 | 1 | S | `fix(ai-svc): P0-R2-4 — multimodal body size limit` |
| **PR-3** | DB 完整性（DDL 漂移 + GRANT + msg_summary_v 收敛）| ~12 | M | `fix(db): P0-R2-6 + P0-R2-7 — daily_emotion_by_modality_v GRANT + messages FK + 01/02 拆分` |
| **PR-4** | web 容器 USER + lockfile | 1 | S | `fix(web): P0-R2-8 + P0-R2-9 — USER node + npm ci lockfile` |
| **PR-5** | compose env_file 锚点 | 1 | S | `fix(deploy): P0-R2-10 — env_file 锚点 + 7 svc 注入 .env.local` |
| **PR-6** | JWT 双存储重写（cookie-only）| 4 | M | `fix(bff/web): P0-R2-1 — JWT HttpOnly cookie + APISIX jwt-auth cookie` |
| **PR-7** | useFaceEmotion orphan API | 1 | S | `fix(web): P0-R2-2 — useFaceEmotion 改 /multimodal/analyze` |
| **PR-8** | docs + Stage 97 收口 | |  | `docs(stage-97): Round 2 P0 收口报告` |
| **PR-9** | Round 1 P1/P2 + Round 2 P1/P2 顺手修复 | ~30 | L | `fix(round1/2-P1P2): DLQ sw8 + GRPC panic + nginx P1-R2-3 等` |

## 4. 改动文件统计

- 工作区总 diff: 76 文件 / 1024 插入 / 567 删除
- 涵盖域：ai-svc / analytics-svc / chat-svc / web-bff / llm-service / web /
  shared / deploy / charts / docs

## 5. Residuals（未做）

按 plan 文档归类：

1. **userInfo 仍存 localStorage**（同源 XSS 风险但非 token）—— user.ts:226-229
   - 风险低：userInfo 不含敏感 token，只含昵称/头像等公开字段
   - 修复建议：改为 cookie（user_info）+ 同步 BFF setAccessTokenCookie 模式
   - 不在本 P0 范围，下一 sprint 单独排期
2. **forgetPwdState.ts 把验证码 + 账号名写 localStorage** —— 风险高
   - 验证码理论上应只走 SMS/email，不进 XSS 可见存储
   - dev 模式保留（便于 e2e）；prod 必须走 SMS/email provider
   - 不在本 P0 范围，下一 sprint 单独排期
3. **基础镜像未 pin digest**（P2-R2-19）—— 仅 Dockerfile 注释提示，未实施
   - 真正落地要 CI sync digest 流程，工作量大
   - 不在本 Stage 范围
4. **Round 2 P1（17 项）/ P2（21 项）/ P3（5 项）** —— 工作区顺手做了部分，剩余排下一 sprint

## 6. 收口自检

- [x] `go test ./...` 全 4 个 svc + BFF 全绿
- [x] pytest 200 passed（含新增 4 用例）
- [x] vitest useFaceEmotion 5/5
- [x] 2 个 Python 字面量契约脚本 PASS
- [x] working copy 与已写测试一致
- [ ] git push + commit（按 PR 拆分执行）
- [ ] `code-review-2026-09-14-round-2.md` front-matter 改 `status: landed`
- [ ] roadmap / decisions.md 同步

## 7. 风险点（已识别）

| 风险 | 等级 | 缓解 |
|------|------|------|
| 9 个 PR 一次合并膨胀 diff | 高 | 按 §3 拆分，逐 PR 跑测试 |
| 改 schema DDL 后 dev 库缺视图 | 中 | migrate.sh 重跑 + smoke §契约 3 + 4 |
| web Dockerfile lockfile 触发 linux-musl 兼容（Stage 74 已知）| 中 | 已用 `--omit=optional` |
| JWT cookie-only 破坏现有 session | 中 | refresh_token 流程不变 |
| llm-service `/analyze` 鉴权后 dev 用户调试 AI 功能断 | 低 | dev compose 默认 key 空 → 跳过 |
| 大量顺手 P1/P2 改动混在 P0 PR | 高 | 拆分 PR-9 单独处理 |

> **调研依据 = 76 文件 diff stat + 9 个 P0 独立 Read 验证 + 6 套新增测试全绿 + Round 2 plan §5/§6 拆分**。