---
stage: e2e-29
title: 横切：异常与安全（JWT 过期刷新 / IDOR / 限流 / CORS / 越权 / 密钥轮换 / 错误透出与留痕）
executed: 2026-10-07 ~ 2026-10-08
status: done
environment: dev 模式（基线栈重建：infra+apps+compose.dev.yml+--env-file .env.local+--profile dev；6 应用服务 + infra healthy；Nacos count=7；一次性容器 ExitCode 0）+ 宿主 nuxt dev :3000（主 worktree 源码）
---

# E2E-29 执行记录

## 1. 环境基线

- **启动命令**（RUNBOOK §2.1；本轮因原栈来自已删除 worktree 而**重建**）：
  ```bash
  cd deploy && docker compose -f docker-compose.infra.yml -f docker-compose.apps.yml \
    -f compose.dev.yml --env-file .env.local --profile dev up -d
  ```
- **容器状态**：6 应用服务（user/chat/analytics/assessment/ai/web-bff）+ infra（postgres/redis/kafka/nacos/etcd/minio/apisix）全部 healthy；`db-migrate` / `apisix-seed` / `kafka-init` / `minio-init` 均 **ExitCode 0**；模型服务（xtts/sensevoice/fer）保持既有运行态未重建。
- **Nacos 注册**：`count=7`（6 应用服务 + llm-service）。
- **声明的配置差异**（dev 专属，prod 语义不可由此推断）：
  - `BFF_DEV_RETURN_CODE=1`（验证码回显）、`BFF_TRUST_APISIX=true` + `BFF_APISIX_CIDRS=172.18.0.0/16`
  - **8894 宿主映射已移除**（E2E-29 D-47 ③）：BFF 仅容器网可达（`docker exec` 或经 :19080）
  - 前端：宿主 `nuxt dev --port 3000`（**主 worktree 源码**），非容器产物（F-196 铁律先验）
- **被验镜像（如实修正，见 §8 第二方核对发现 #1）**：**`emotion-echo/web-bff:v0.1.36`**（`docker inspect` 实证：`image=emotion-echo/web-bff:v0.1.36 created=2026-10-08T00:09:54Z`；含 L1/D-47/F-203/D-48 全部改动）。
  - **修正前的失真**：本报告初稿写 v0.1.36，但当时**运行容器实际是 v0.1.35**（v0.1.36 镜像已构建却未重建容器）。第二方核对机械证伪后，已 `up -d` 重建到 v0.1.36 并**在 v0.1.36 上复跑全部关键事实**（匿名 refresh 401 / login+受保护 200 / cookie `SameSite=Lax` / 宿主直连 8894 `code=000` / 回归钉 9/9），修正后结论不变。
  - **D-48 运行时窗口证据的来源（如实标注）**：窗口机制（新/旧 keyID token 都被网关接受、窗口态 BFF 接受旧密钥）是在**两个 v0.1.36 隔离探针容器**上实测的（探针 keyID `ee29-probe-*`，避免触碰真实密钥），**不是**主栈容器——主栈默认不开窗口（未配 `_PREV`）。
- **环境事实（如实记录，plan §0.1 F18）**：开工前 17 个后端容器由**另一个目录**（`Emotion-Echo-f198`，已非注册 worktree）的 compose 创建且**未加载 `compose.dev.yml`**（实测 `BFF_TRUST_APISIX=false`），且多数容器在开工前已 `Exited(127/255)` —— 故本轮**重建基线栈**后才做任何运行时结论。

## 2. 测试点结果

| # | 测试点 | 判定 | 结果 | 证据 | 备注 |
|---|--------|------|------|------|------|
| 1 | 匿名/无效令牌调 `/auth/refresh` 必须 401 | [A] | PASS | 修复前：`curl -X POST …/auth/refresh`（无 cookie/Authorization）→ `200` + `Set-Cookie: access_token=ey…` + body 含 `accessToken`，且该 token 经网关读 `/users/me` 返 `account=echo`；修复后（v0.1.33+）→ **401** `unauthorized: refresh requires a token`、**无 Set-Cookie**、**无 accessToken**。隔离容器前后对照（旧码 v0.1.32 vs 新码）：`200+token` → `401`。回归钉 `security-boundaries.spec.ts` #1 | D-46 硬 401；TDD `auth_refresh_guard_test.go` 6 例 |
| 2 | 有效令牌调 refresh 正常续期（同一 user_id） | [A] | PASS | 短 TTL 探针（TTL=2s 隔离容器）：未过期 refresh → `200`；TDD 断言续期后解析出**同一** user_id；回归钉 #1 同批 | — |
| 3 | 受保护端点匿名枚举（逐条，非抽样） | [A] | PASS | **27/27** 受保护端点（由前端 `apiRoutes.ts` 35 条减去 8 条白名单）匿名经网关 → 全部 **401**（`Missing JWT token in request`）；回归钉 #3 覆盖其中 7 条 | 清单来源=前端路由表（唯一来源） |
| 4 | 令牌类型隔离（双向） | [A] | PASS | reset token 当 access token → 网关 **401**、`/auth/refresh` **401**；access token 当 resetToken → `/auth/reset-password` **401** 且**原口令仍可登录**（未被改写）。回归钉 #4（含"原口令仍 200"断言） | TDD `auth_refresh_guard_test.go` 含 reset token 例 |
| 5 | 过期语义与续期路径（**2026-10-08 补验：IAB 实测**） | [A] | PASS | 短 TTL 探针：过期后 refresh → **401**；**签名有效但已过期**的 JWT 经网关受保护端点 → **401** `{"message":"failed to verify jwt"}`（**无 `code` 字段**）；未过期 refresh → 200。**IAB 真实浏览器补验**（browser-use）：过期令牌触发受保护请求 → 应用跳 `/login`（登录表单可见）、**零次 `/auth/refresh` 调用**（截图 `screenshots/expiry-redirect-to-login.png`）；`jwt-expiry.spec.ts` 加严为 2 用例 × 2 project = **4 passed** | **更正（2026-10-08）**：原写"前端 `code===10002` 续期分支由回归钉 #1 的 cookie 语义间接覆盖"**不成立**——实测该分支为**死代码**（全仓无任何后端下发 10002，见 §6 观察项 4 与账本 E2E-F-207）；真实行为是"过期即登出跳登录、不尝试续期"。plan §6 风险表要求的"IAB 实测真实过期场景"本轮**已补做** |
| 6 | logout 清除 cookie 语义 + 已知边界 | [A] | PASS | `POST /auth/logout` → `Set-Cookie: access_token=; Path=/; Max-Age=0; HttpOnly; SameSite=Lax`；回归钉 #6 断言清空 + SameSite | **已知边界（如实）**：无服务端黑名单 ⇒ 已签发 token 有效至 exp（D-46 已记录的另一议题） |
| 7 | reports 端点 IDOR 守卫 | [A] | PASS | A 的令牌 + `?user_id=<B>` → **403** `forbidden: user_id mismatch with authenticated user`；无 query → 200（用认证身份）；自己 id → 200；回归钉 #7/#8 | 实现 `analytics_handler.go:47-72` |
| 8 | 参数别名不构成越权 | [A] | PASS | `?userId=` / `?id=` / `?uid=` / `?user=` → 200 且**响应体 md5 与"不带 query"完全相同**（被忽略、无泄漏）；回归钉断言**逐字相同** | 守卫生效但语义是"字面参数名白名单"（已记录） |
| 9 | 资源级越权（跨用户读） | [A] | PASS | A 的令牌读 B 的会话消息 `GET /conversations/<B>/messages` → **403** `forbidden: conversation does not belong to current user`；A 的会话列表**不含** B 的会话；回归钉 #9/#10 | 下游 chat-svc 侧归属校验生效 |
| 10 | 越权写拒绝 | [A] | PASS | A 对 B 的会话 `PATCH` / `POST pin` / `POST messages` / `DELETE` → **全部 403**；对照：A 对自己的会话同四动作 → **200**；事后核验 B 的会话标题/isTop **未被改动**；回归钉 #9/#10 同批 | — |
| 11 | BFF 可信链（F-202） | [A]+[M] | PASS | **D-47 落地后**：宿主 `curl localhost:8894/health` → **code=000 拒连**（`docker inspect` → `ports=map[]`）；`docker exec` 内 `/health/ready` → 200；网关链路 login=200 / 匿名 refresh=401；fail-fast 两种危险形态 → **ExitCode=1** + 可操作日志，dev 形态对照 → running；回归钉 #11（宿主直连必须不可达） | [M] 已裁定 **D-47**；守卫 `check_bff_trust_chain.sh` 5/5 + 自检 3/3（两个负向对照） |
| 12 | 网关限流实测（两条链） | [A] | PASS | 白名单链（`/auth/login`）与 catch-all 链（`/users/me`）各打 70 次 → **第 61 次 429**（60 通过 + 10×429），带 `X-RateLimit-Limit/Remaining/Reset` | `policy=redis`（见 #14 的 Redis 键证据） |
| 13 | 限流拒绝码一致性（F-177 域） | [A] | PASS | 两条链的限流拒绝**均为 429**（响应体 `<title>429 Too Many Requests</title>`，非 openresty 裸 503） | 与 E2E-25 的 `rejected_code` 收口一致 |
| 14 | 登录锁定 + 跨实例共享 + Retry-After | [A] | PASS | 5 次错密码 → 401×5，第 6 次 → **423 Locked** `too many failed attempts`；锁定后正确密码仍 423；`LOGIN_LOCK_BACKEND=redis` + Redis 实键 `web-bff-auth:fails:<user>` 与 `plugin-limit-count:v1:/apisix/routes/110:…`（跨节点配额共享实证） | **观察项（如实）**：423 响应**无 Retry-After**；前端 `getRetryDelayMs` 优先读它、缺失则指数退避兜底 ⇒ 可用但可改进（本轮不修，理由：前端已有兜底且 423 语义清晰） |
| 15 | CORS origin 白名单双向 | [A] | PASS | 恶意 origin（`http://evil.example.com`）预检 → **0 个 `Access-Control-Allow-*`**；合法 origin → `ACAO: http://localhost:3000` 精确回显 + `ACAC: true`；回归钉 #15/#16 | — |
| 16 | `allow_headers` 面与预检时效 | [A] | PASS | 改后：`Access-Control-Allow-Headers: Content-Type,Authorization,X-Trace-Id`（**不含 X-User-Id**）、两条链 `Access-Control-Max-Age: 600`（白名单链原为默认 5）；`seed_test.js` +2 契约 → 77/0；回归钉断言不含 X-User-Id 且 Max-Age=600 | 依据：前端全仓**零处**发送 X-User-Id，且 APISIX 无条件覆盖该头 |
| 17 | JWT 密钥轮换机制（F-28） | [M] | PASS | **D-48 双密钥窗口**：TDD `jwt_multikey_test.go` 6 例 RED→GREEN；**运行时实测（真实网关 + 探针 keyID）**：新 keyID token → 200 / **旧 keyID token → 200（在途会话未断）** / 窗口态 BFF 的 refresh 接受旧密钥 → 200 / 无关密钥 → 401；`seed.sh` Step 2.6 双 consumer + 未配置 `_PREV` 时自动清理；`scripts/rotate_jwt_secret.sh`（status/plan/verify/finalize） | [M] 已裁定 **D-48**；ADR `adr-2026-10-jwt-key-rotation-dual-key` + 架构决策 41 |
| 18 | 备份/恢复生产化封装（F-27 后续） | [A]+[M] | PASS | 真演练：破坏前 messages=708 / conversations=532 / users=60 / ube=438 → 备份 349,975 字节（29 个 TABLE DATA，含 `pg_restore --list` 完整性校验）→ `DROP TABLE messages CASCADE`（连带 DROP 视图 `msg_summary_v`）→ 恢复 **rc=0**（errors=14 良性=14，全为分区表继承约束）→ **四项行数逐项一致**、视图连带恢复（708） | [M] 已裁定 **M4**（封装脚本 + dev 真演练）；守卫 `test_db_backup_restore_contract.sh` 9/9（含 3 行为负向 + 1 静态负向） |
| 19 | 错误透出与真因留痕（F-200） | [V]+[A] | PASS | 代码：TypeError 宽兜底**拆开**（"环境无 mediaDevices"→ 换浏览器/退出内嵌；"预览未挂载"→ 稍候再点）、消息统一带 `［原因：<error.name>］`、`reportClientError` 上报 `/api/v1/client-error`；TDD `useFaceEmotion.f200.test.ts` 4 例 RED→GREEN（+F-119 5 例，共 14/14）；**`[V]` 截图**：回归钉 #19 在 headless chromium 实点摄像头 → `screenshots/19-camera-error-attributable.png`（toast = 「摄像头组件未就绪（预览尚未挂载完成），请稍候再点一次［原因：TypeError］」，**截图已查看**） | 边界：无-mediaDevices 分支的文案由静态契约钉住，未在真实无该 API 的环境实点 |
| 20 | 收口与工具修复（含 F-182） | [A] | PASS | `smoke_bff_chat_grpc.sh` 由 4/7 FAIL → **9/9 全绿**（新增登录取 Bearer + 修字符串 id 解析）；静态守卫 `test_smoke_bff_chat_grpc_contract.sh` 5/5 接 CI；回归钉 spec **9/9 绿**（首跑即绿，见 §5）；`e2e_stage_audit.py --all` **0 FAIL**；账本对账见 §3 | — |

**汇总：PASS 20 / FAIL 0 / BLOCKED 0 / N/A 0**

## 3. 发现与分类

| 发现 | 分类 | 处理 |
|------|------|------|
| 匿名 `POST /auth/refresh` 发放 `user_id=1` 有效 JWT（E2E-F-201，**认证绕过**） | 范围内 | 修复 = L1（PR #170）；账本 ✅ |
| BFF 可信链默认 fail-open + 8894 暴露（E2E-F-202） | 范围内 | 修复 = D-47（PR #171）；账本 ✅ |
| cookie 无 `SameSite`/`Secure` 而注释称 Lax（E2E-F-203） | 范围内 | 修复 = F-203（PR #172）；账本 ✅（该条另一半"前端 jti 契约后端零实现"**未修**，见 §6 观察项） |
| `smoke_bff_chat_grpc.sh` 恒 4/7 FAIL（E2E-F-182） | 范围内 | 修复 = F-182（PR #172）；账本 ✅ |
| 摄像头 TypeError 宽兜底 + 无真因留痕（E2E-F-200） | 范围内 | 修复 = F-200（本 PR）；账本 ✅ |
| JWT 密钥无轮换机制（E2E-F-28） | 范围内 | 实现 = D-48（PR #175）+ ADR；账本 ✅ |
| 无备份/恢复封装（E2E-F-27 后续） | 范围内 | 实现 = M4（PR #173）；账本 ✅ |
| **svc 的 SkyWalking tracer 初始化无重试**（OAP 未就绪 ⇒ 永久不上报；`docker restart` 即愈） | **范围外**（可观测性韧性） | 只记账：**E2E-F-204**（owner 按既有约定写为不回挂形态，防 A5 把新发现算到已 done 阶段头上） |
| 423 响应无 `Retry-After` | 范围内观察项 | **不修**（前端已有指数退避兜底；补头属可选项），已记录于 #14 备注 |
| 前端 `useApi.ts` 注释声称"后端要 jti 做黑名单/轮换校验"而 BFF 零 jti 逻辑 | 范围内观察项 | **不修**（纯注释漂移；`refresh` 已按 D-46 硬 401，jti 黑名单是另一议题），记录于 §6 |
| 运行栈归属异常（后端容器来自已删 worktree 且未带 dev overlay） | 环境 | 重建基线栈（plan §0.1 F18 + STATUS v3 记录） |

## 4. 修复清单（TDD 记录）

| commit / PR | 内容 | 先行的失败测试 |
|-------------|------|---------------|
| PR #170（`fix/e2e-29-l1-anonymous-refresh`） | L1：`refresh` 无有效令牌一律 401 | `auth_refresh_guard_test.go` 6 例（RED 时 5 例实测 200） |
| PR #171（`fix/e2e-29-d47-trust-chain`） | D-47：fail-fast 自校验 + 收 8894 + prod 口径 + 守卫 | `auth_trust_test.go` 5 例（`ValidateAuthTrust` 未定义 → 编译失败即 RED） |
| PR #172（`fix/e2e-29-f203-f182`） | F-203 cookie 属性显式化 + F-182 smoke 修复 + 守卫 | `auth_cookie_attrs_test.go` 4 例；smoke 静态守卫负向对照 |
| PR #173（`fix/e2e-29-m4-db-backup`） | M4 备份/恢复封装 + #16 CORS 收口 | `test_db_backup_restore_contract.sh`（行为负向 3 例）；`seed_test.js` +2 契约 |
| PR #175（`feat/e2e-29-d48-jwt-rotation`） | D-48 双密钥窗口 + ADR + 账本修正 | `jwt_multikey_test.go` 6 例（`NewManagerMulti` 未定义 → RED） |
| 本 PR（收口） | F-200 错误透出 + 回归钉 spec + report/roadmap/STATUS/账本 | `useFaceEmotion.f200.test.ts` 4 例（4/4 红） |

## 5. 回归钉

- 新增 spec：`emotion-echo-web/e2e/security-boundaries.spec.ts`（**9 用例**）
  - 首次运行结果：**9 passed**（chromium；`BASE_URL=http://localhost:3000`）
  - 覆盖 #1 / #3 / #4 / #6 / #7+#8 / #9+#10 / #11 / #15+#16 / #19[V]
- 既有相关 spec 加严并保持绿：`jwt-expiry.spec.ts`（**2026-10-08 加严**：过期令牌冷启动 → 跳 `/login` + 登录表单可见 + **零次 `/auth/refresh`**；受保护接口返网关真实 401 体 → 登出跳登录 + 零次刷新；2 用例 × chromium/mobile = **4 passed**）、`multi-instance-smoke.spec.ts`（E2E-20 跨实例锁定）
- 单元层新增：`auth_refresh_guard_test.go`(6) / `auth_trust_test.go`(5) / `auth_cookie_attrs_test.go`(4) / `jwt_multikey_test.go`(6) / `useFaceEmotion.f200.test.ts`(4)
- 静态守卫（已接 CI `e2e-guards`）：`check_bff_trust_chain.sh`(5+3) / `test_smoke_bff_chat_grpc_contract.sh`(5) / `test_db_backup_restore_contract.sh`(9)；`seed_test.js` 77/0

## 6. 待决策 / 升级项

**执行期 [M] 决策点全部落定**：

| # | 决策 | 结论 | 来源 |
|---|------|------|------|
| M1 | `refresh` 无有效令牌语义 | **硬 401**（D-46） | 用户 2026-10-07 AskUserQuestion |
| M2 | prod 信任链默认值 | **默认值分离 + 缺 CIDR 拒绝启动 + 收 8894**（D-47） | 同上 |
| M3 | JWT 密钥轮换形态 | **双密钥并存窗口**（D-48） | 同上 |
| M4 | 备份生产化封装范围 | **封装脚本 + dev 真演练** | 用户 2026-10-07 AskUserQuestion |
| M5 | 资源级越权发现归属 | **由证据关闭**（实测未发现资源级越权：跨用户 5/5 → 403 + 同资源对照 200） | 证据落定，无需裁定 |

**遗留观察项（本轮明确不修，理由已记录）**：
1. 423 响应无 `Retry-After`（前端有指数退避兜底）；
2. `useApi.ts` 的 `jti` 注释与 BFF 零 jti 实现不一致（注释漂移；jti 黑名单属另一议题）；
3. 越权"参数名白名单"式守卫（只认字面 `user_id`，别名被忽略）——现状无泄漏，但新增读别名的端点需同守。
4. **（2026-10-08 补验新增）前端"401 + `code===10002` → 自动续期"分支为死代码**：全仓无任何后端下发 `code:10002`（BFF 一律 `code:1`；网关返 `{"message":"failed to verify jwt"}` 无 code；shared 中间件返 `{"error":"unauthorized"}`），`refreshToken()` 仅被该分支调用 ⇒ 线上从未执行。**行为安全**（过期 → 登出跳登录），但"滑动续期"能力实为缺失。已登记账本 **E2E-F-207**。**→ 已解决（2026-10-08，D-49，PR #178）**：用户拍板**实现真正的滑动续期**（前端在令牌寿命 75% 处主动换新，零后端改动），删两处死分支；landed plan `docs/legacy-plans/landed/sliding-token-renewal.md`（§F 含执行期抓到的 2 个运行时问题）。
5. **（2026-10-08 补验新增）HttpOnly `access_token` 无法被页面 JS 覆盖/删除**：IAB 实测 `document.cookie = 'access_token=…'` 被浏览器拒绝（同名 HttpOnly 存在），`clearToken()` 的客户端 cookie 清除因此**无效**。**非安全缺陷**：显式登出走 `POST /auth/logout`（服务端 `Set-Cookie` 清除，生效）；`clearAuth()` 仅在 401 时触发，彼时令牌本已失效。记为边界。

## 7. 收口自检

- [x] git status 干净（收口 PR 内）
- [x] main 与 origin/main 无 ahead/behind
- [x] 无残留已合并分支（本地仅 main；远端仅 main 与本轮收口分支，合并后立即删除）
- [x] `e2e_stage_audit.py --all` → **30 阶段 0 FAIL**
- [x] 账本对账：本阶段名下 7 条（F-27 后续 / F-28 / F-182 / F-200 / F-201 / F-202 / F-203）**全部 ✅**；新发现 F-204 已登记且 owner 不回挂
- [x] 收口契约 §7 十一项：report ✅ / 截图 ✅（`screenshots/19-camera-error-attributable.png`，已查看）/ 回归钉 ✅（9/9 首跑绿）/ roadmap+plan 状态同步 ✅ / 账本 ✅ / 决策 ✅（D-46/47/48 + 架构决策 41）/ commit+push ✅ / 自检三连 ✅ / 账本对账 ✅ / 第二方核对（见 §8）/ 复读关键断言（证据列均带命令输出或 `文件:行号`）✅

## 8. 第二方核对（RUNBOOK §13.3；独立子代理，2026-10-08）

**核对方式**：另起独立子代理（明写"执行者自证不可信"），**只读 + 自己复跑**：不引用报告结论，
逐项执行 23 条断言（模板/账本/状态/机器门禁/运行时事实/失真排查）。**禁改仓库文件、禁重启容器、禁读 `.env.local`**。

**首轮结论：不通过**（1 项 FAIL + 1 项 WARN，其余 21 项 PASS）。

| # | 判定 | 核对方实测摘要 |
|---|------|---------------|
| 1~5 | PASS | report 含 §10 全部章节；汇总行非占位符且**表格行数自己数得 20**；判定/结果列取值合法；证据列 `grep` 存在性措辞 = NONE；`[V]` 截图 60,963 字节非空 |
| 6~9 | PASS（#9 WARN） | 归属 E2E-29 的 7 条账本**全 ✅**；F-204 owner 只含主归属（A5 逻辑不触发）；三处 status 一致（done）；编号连续无跳号（1..206 无缺号）——**WARN：E2E-F-115/117/119 各 2 行（既有账本治理债 E2E-F-179，非本阶段引入）** |
| 10~12 | PASS | `audit --all` 退出码 0 / 30 阶段 0 FAIL；`--selftest` 通过；三个守卫 3/5/9 全过（含负向）；`seed_test.js` 77/0 |
| 13~20 | PASS | 匿名 refresh 401 无 cookie/token；6 条受保护端点匿名全 401；宿主 8894 拒连（`ports={"8894/tcp":null}`）而容器内 `/health/ready` 200；登录/登出 cookie 均 `SameSite=Lax`；CORS 恶意 origin 无 ACAO、合法 origin 无 `X-User-Id` 且 `Max-Age=600`；**自建两账号复跑越权：跨用户 5 动作全 403 + 同资源 5 动作全 200**；别名 `userId` 响应体 md5 逐字相同、`user_id` 403；回归钉 **9 passed** |
| 21 | **FAIL** | **环境基线失真**：报告称被验镜像 v0.1.36，实测容器为 **v0.1.35**（`Created=2026-10-07T22:58:13Z` 早于 v0.1.36 镜像 `23:35:38Z`；容器 env 无 D-48 的 `BFF_JWT_KEY_ID*`）⇒ "运行栈含 D-48 全部改动"不成立 |
| 22~23 | PASS | 无 N/A/BLOCKED 掩盖；证据列普遍含可复现命令或 `文件:行号`；未发现"只有单测却声称已验证"（#5 的前端分支如实标注为"间接覆盖"） |

**核对方另独立复读源码验证三条关键断言**（均成立）：`auth_handler.go:227-248` refresh 无回落默认身份；
`config.go:33` `ValidateAuthTrust` 存在且 `main.go:127` 调用；`useFaceEmotion.ts:13-20/100/119-130`
（mediaDevices 探测 + `reportClientError` + 两条独立前置条件分支）。

**FAIL 的处置（已闭环）**：
1. `docker compose … up -d emotion-echo-web-bff` **重建到 v0.1.36**（`image=…:v0.1.36 created=2026-10-08T00:09:54Z`）；
2. **在 v0.1.36 上复跑全部关键事实**：匿名 refresh **401** / login+受保护 **200** / cookie **`HttpOnly; SameSite=Lax`**（登出 `Max-Age=0; HttpOnly; SameSite=Lax`）/ 宿主直连 8894 **000** / 回归钉 **9/9**；
3. §1 环境基线改写为如实表述，并**明确标注 D-48 窗口证据来自隔离探针容器**（主栈默认不开窗口）。

**WARN 的处置**：E2E-F-115/117/119 重复行属既有账本治理债（E2E-F-179 已登记、owner 不回挂），**本阶段不修**（避免范围蔓延），已在 §8 如实记录。

**第二轮复检（同一独立核对方，保留其上下文，2026-10-08）**：结论 **`失真项复检：通过`**（5/5）：

| 复检项 | 判定 | 核对方实测摘要 |
|--------|------|---------------|
| 被验镜像 | PASS | `docker inspect` → `emotion-echo/web-bff:v0.1.36  Created=2026-10-08T00:09:54Z`；晚于镜像构建时刻 `2026-10-07T23:35:38Z` ⇒ 确为重建到 v0.1.36 |
| 匿名 refresh | PASS | `401` + `unauthorized: refresh requires a token`，无 `Set-Cookie`、无 `accessToken` |
| 登录 cookie | PASS | `Set-Cookie: access_token=…; Path=/; Max-Age=86400; HttpOnly; SameSite=Lax` |
| 宿主直连 8894 | PASS | `code=000`（exitcode=28 拒连） |
| §1 表述与实测一致性 | PASS | 报告 §1 与实测**逐字一致**，且如实标注修正前的失真、**明确限定 D-48 窗口证据来自隔离探针容器**（未把窗口态归到主栈），无夸大；§8 如实收录首轮 FAIL 与处置 |

**结论**：第二方核对**两轮完成**——首轮 1 FAIL + 1 WARN → 修正后第二轮 **PASS**（FAIL 已闭环，WARN 为既有治理债并如实登记）。E2E-29 判 **done**。
