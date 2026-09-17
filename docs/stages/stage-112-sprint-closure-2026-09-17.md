---
status: landed
priority: high
owner: zcode-bot
created: 2026-09-17
last-refresh: 2026-09-17
type: sprint-closure
related-stages:
  - docs/legacy-plans/landed/stage-112-bug-a-fixed-2026-09-17.md (Bug A 三层根因详录)
  - docs/stages/stage-109a-apisix-jwt-401-fix-2026-09-16.md
---

# Stage 112 — 主页分区扫描 + 多轮修复 sprint 收口（全部修通）

> **起点**：用户报告「忘记密码功能与现状不符」+「检查主页每个分区是否可点击、渲染正常」。
> **结果**：浏览器扫描发现 3 个真 bug + 深挖出 2 个隐藏 bug + 1 个文案错配，**全部按 TDD 修复**，
> 浏览器端到端 6/6 × 2 轮通过。

---

## 一、修复清单（5 bug + 1 文案）

### Bug A · dashboard / 我的空间 被踢回 /login（三层根因，见 landed 文档）

| 层 | 根因 | 修复 |
|---|---|---|
| 1 | APISIX **route 116 配置漂移**（手工创建、不在 seed.sh；jwt-auth 默认配置 ≠ route 100）→ `/reports/*` 走 116 被 401 | 删 route 116 + seed.sh Step 4.5 防漂移清理 |
| 2 | BFF `userIDQuery` 强制 `?user_id=` → 前端不发 → 400；**且 ?user_id 可查他人数据（IDOR）** | 回退认证身份（X-User-Id）+ 不一致返 403 |
| 3 | BFF downstream `InteractionDepth`/`FrequencyTrend`/`MentalAssessment` **漏 `withUserID(ctx)`** → analytics-svc 401 → "我的空间"被踢 | 三方法补 `withUserID(ctx)` |

### Bug B · `/question` layout 错配丢 sidebar
`pages/question/index.vue` 用 `layout: 'default'`，sidebar 在 `nav.vue` → 改 `layout: 'nav'`。

### Bug C · `/chat/user` 头像破图
`user.ts` fallback `/imgs/default-avatar.webp` 不存在 → 补资源文件 + 契约测试。

### 文案 · 忘记密码与 username-only 登录错配（方案 A）
`forget/verify.vue` 文案承诺"手机号或邮箱"但项目全用 username → 文案/placeholder/正则全改 username。

### 附带修复 · seed.sh 函数定义顺序
`_load_services` 调用 `log()` 时函数未定义 → `sh` 运行必挂（`log: command not found`）→ 把 `log/die` 前移。

---

## 二、提交记录

| # | commit | 内容 |
|---|---|---|
| 1 | ca68a6b | fix(B-B): question 页 layout 改 nav |
| 2 | f1dcf2a | fix(B-C): 补 default-avatar.webp |
| 3 | 7132bb7 | fix(A-方案A): forget-pwd 文案 username-only |
| 4 | d53f1f7 | fix(B-A): auth.global 中间件 client 兜底（探索期，保留） |
| 5 | 0d6d1cc | fix(B-A-ssr): SSR cookie 注入（探索期，保留） |
| 6 | d3ee175 | fix(B-A-v3): hasAccessToken 判定 + seed.sh 函数顺序 + jwt-auth 三路兜底 |
| 7 | （本次） | fix(B-A 根修): route 116 漂移清理 + userIDQuery 回退+IDOR 403 + downstream withUserID × 3 + docs |

> 4~6 为定位过程产物，最终根因见 7。

---

## 三、验收证据

### 单元测试（全绿）
- `go test ./...`（BFF 全包）✅
- 新增 8 用例：`analytics_handler_test.go` 5 个（userIDQuery fallback/403/兼容）+ `analytics_grpc_test.go` 3 个（metadata 注入，bufconn mock）
- 前端 10 用例：layout-uses-nav 2 + default-avatar 2 + username-only-copy 3 + auth-restore 3

### curl（经 APISIX，模拟前端调用）
- `/reports/daily`、`/reports/trend` 无 user_id + 认证头 → **200 + 真实数据**
- `/user-behavior/depth`、`/user-behavior/frequency` → **200**（修复前 401）
- 无认证 → 401（防线保留）；`?user_id=2` 越权 → **403**（新防线）

### 浏览器端到端（IAB，dev mode）
- 第一轮 6 链接（日报/周报/月报/年报/我的空间/设置）→ **6/6 不被踢回**
- 第二轮 3 链接复测 → **3/3 稳定**
- 截图：`gui-test-screenshots/stage-112-section-scan-2026-09-17/20-fixed-chat-user-no-kick.png`

---

## 四、防复发

1. **seed.sh Step 4.5**：幂等清理已知漂移路由 id（116）；注释明确"禁止手工 admin API 建路由"
2. **IDOR 防线**：userIDQuery 403 分支已被测试钉死
3. **metadata 注入**：3 个 downstream 方法的 x-user-id 注入已被测试钉死
4. 方法论沉淀（写进 landed 文档）：**先做对照实验再下结论**——`conversations 200 vs reports/daily 401` 一步定位漂移

---

## 五、遗留 / 后续建议

- **前端 401 处理策略**：`useApi.ts` 目前任何 401 都 `clearAuth + navigateTo('/login')`。若未来再有
  单个端点 401（配置问题），用户会被整体登出。建议：仅当 `code === 10002`（token 过期）才登出，
  其余 401 只 toast 报错。登记为 P2（未在本次改动，避免超范围）。
- **Playwright E2E**：把"登录后逐点 6 个 sidebar 链接不被踢回"写成 spec，进 CI 防回归（P2）。
- **APISIX route 审计**：可考虑 seed.sh 启动时列出所有 route id 与白名单比对告警（P3）。
