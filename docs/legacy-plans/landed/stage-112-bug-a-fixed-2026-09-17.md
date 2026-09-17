---
status: landed
priority: high
created: 2026-09-17
last-refresh: 2026-09-17
sprint: 112
type: bug-fix
original-plan: docs/plans/stage-112-bug-a-deferred-2026-09-17.md (已在落地后删除)
related-stages:
  - docs/stages/stage-112-sprint-closure-2026-09-17.md
  - docs/stages/stage-109a-apisix-jwt-401-fix-2026-09-16.md (早期 jwt-auth 修复，本次发现其结论不完整)
---

# Stage 112 · Bug A（dashboard 被踢回 /login）— 已修复，真实根因三层

> **原判定（deferred 文档）**：以为根因是"APISIX 3.18.0 jwt-auth encrypt_fields 上游 bug + BFF 强依赖 X-User-Id"，
> 曾建议 Sprint 113 走「方向 A：BFF Set-Cookie」。
> **最终结论**：**三个独立根因叠加**，与 encrypt_fields 无关。全部按 TDD 修复并完成浏览器端到端验收（6/6 × 2 轮）。

---

## 一、真实根因（三层，逐层剥开）

### 层 1：APISIX route 116 配置漂移 → jwt-auth 401

**现象**：sidebar 点"日报/周报/月报/年报"→ URL 被改写成 `/login`（约 50% 概率）。

**排查**：APISIX 日志显示 `GET /api/v1/reports/daily → 401`。

**根因**：APISIX 中存在一条**手工 admin API 创建、不在 seed.sh 白名单**的路由 id=116
（`uri=/api/v1/reports/daily`）。APISIX 路由匹配"更具体的 URI 优先"，所以 `/reports/daily`
走 116 而不是 route 100 的 catch-all（`/api/v1/*`）。116 的 jwt-auth 配置与 route 100
不一致（默认 header 解析 vs 100 的 cookie+header），且从未随 seed 更新——**配置漂移**。

**修复**：
- 删除 route 116（`DELETE /apisix/admin/routes/116`）
- `deploy/apisix/seed.sh` Step 4.5 新增"清理已知漂移路由"步骤（幂等，防复发）
- 教训写入 seed.sh 注释：**任何路由必须经 seed.sh 幂等 PUT，禁止手工 admin API 建路由**

**验证**：删除后 `curl /api/v1/reports/daily`（header/cookie 两种方式）全部 200。

### 层 2：BFF `userIDQuery` 强制要求 query 参数 → 400 + IDOR 风险

**现象**：即使 auth 修好，4 个 dashboard 仍"加载失败"。

**根因**：前端 `fetchDailyReport` 调 `get('/reports/daily', { date })` **不带 user_id**，
而 BFF `userIDQuery` 强制要求 `?user_id=` → **400 validation: user_id is required**。

**附带发现（IDOR 越权）**：`?user_id=任意值` 会被原样透传给下游——已登录用户可以查
**任何其他用户**的行为报表。

**修复**（`emotion-echo-web-bff/internal/handler/analytics_handler.go`）：
```
userIDQuery 语义改为：
- 无 query user_id + 有 X-User-Id（APISIX 注入）→ 用认证身份（修复前端调用）
- 有 query user_id 且与认证身份不一致 → 403 forbidden（防 IDOR）
- 有 query user_id 且一致 / 无认证头（单测直连）→ 200（向后兼容）
- 两者都无 → 400（保留原契约错误）
```

**TDD**：`analytics_handler_test.go` 新增 5 个用例（RED 3 红 → GREEN 5/5）。

### 层 3：BFF downstream 3 个方法漏 `withUserID(ctx)` → analytics-svc 401

**现象**："我的空间"页（`/chat/user`）被踢回 /login；"设置"页跟着被踢（token 已被前一次 401 清掉）。

**根因**：`/chat/user` onMounted 并发调 3 个 user-behavior 端点。curl 实测：
- `day-night` → 200（`DayNightPattern` 有 `withUserID`）
- `depth` → **401 "downstream: analytics interactionDepth: unauthenticated: missing x-user-id metadata"**
- `frequency` → **401 "同 missing x-user-id metadata"**

`analytics_grpc.go` 中 `DailyReport` / `TrendReport` / `DayNightPattern` 三个方法包了
`withUserID(ctx)`，但 **`InteractionDepth` / `FrequencyTrend` / `MentalAssessment` 漏了**
→ gRPC metadata 无 x-user-id → analytics-svc 拦截器拒绝 → BFF 透传 401 → 前端
`useApi.ts` 的 401 分支 `clearAuth() + navigateTo('/login')`。

**修复**：三个方法补 `withUserID(ctx)`。

**TDD**：`analytics_grpc_test.go` 新增 3 个 metadata 断言用例（RED 3 红 → GREEN 5/5，
含 bufconn mock server 捕获 x-user-id）。

---

## 二、验收证据

### curl（经 APISIX :19080，模拟前端调用方式）
| 场景 | 结果 |
|---|---|
| `/reports/daily` 无 user_id + Authorization | **200** + 真实数据（34 条消息） |
| `/reports/trend` 无 user_id + Authorization | **200** + 真实数据 |
| `/reports/daily` + Cookie | 200 |
| `/reports/daily` 无认证 | 401（安全防线保留） |
| `/reports/daily?user_id=2` 认证 uid=1 | **403 forbidden**（IDOR 防线生效） |
| `/user-behavior/depth` | 200（修复前 401） |
| `/user-behavior/frequency` | 200（修复前 401） |

### 浏览器端到端（IAB，dev mode）
- **第一轮**：6 个 sidebar 链接（日报/周报/月报/年报/我的空间/设置）→ **6/6 不被踢回**
- **第二轮**：日报 + 我的空间 + 设置 → **3/3 稳定通过**
- 截图：`docs/evidence/gui-test-screenshots/stage-112-section-scan-2026-09-17/20-fixed-chat-user-no-kick.png`

### 单元测试
- `go test ./...`（BFF 全包）全绿
- 新增 8 个测试（5 handler + 3 downstream）

---

## 三、与原 deferred 文档的差异（教训）

| 原判断 | 实际情况 |
|---|---|
| 根因是 APISIX 3.18.0 encrypt_fields 上游 bug | 无关。signature 本地验算与 token 完全一致；真正问题是 route 漂移 |
| 根因是 BFF 强依赖 X-User-Id header | 只是最外层症状。X-User-Id 正常注入；是 route 116 提前 401 掐断了链路 |
| 建议 Sprint 113 改 BFF Set-Cookie | 不需要。jwt-auth 本身健康，**不需要**改动 token 传输架构 |
| 50% 概率复现 | 实际是"路径相关确定性失败"（/reports/* 走 116 必 401），概率感来自测试顺序 |

**方法论教训**：先做"对照实验"（同 token 测多个端点/多种传输方式）再下结论——
本次 `conversations 200 vs reports/daily 401` 的对照一步定位到 route 漂移。

---

## 四、防复发措施

1. `seed.sh` Step 4.5：清理已知漂移路由 id（当前 116），幂等可在每次 seed 后执行
2. seed.sh 注释明确规则：**禁止手工 admin API 建路由**（会产生漂移）
3. BFF `userIDQuery` 的 403 分支：防 IDOR，并已由测试钉死
4. downstream 三个方法的 metadata 注入已有测试钉死（防再漏）
