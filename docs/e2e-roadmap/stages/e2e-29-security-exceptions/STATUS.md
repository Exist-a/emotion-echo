# E2E-29 横切：异常与安全 — STATUS v4（**done** 收口版 + 2026-10-08 补验轮）

> 本文件是 Lane E 的**本轨进度事实源**（[parallel-tracks.md](../../../_meta/parallel-tracks.md) §五 指定路径）。
> 格式：已做 ✅ / 未做 ❌ 分列，**禁止美化**。下次 Lane E 会话开工前必读本文件 + [plan.md](plan.md) §0.2 开工复核清单。
> v1 = 纯建档轮（PR #168/#169，main `b6b1570`→`1396b59`）；**v2 增补：D-46/47/48 三项裁定落定 + L1（匿名 refresh 认证绕过）已修并运行时验证 + 环境归属实情（F18）**；**v3 = 收口（20/20 PASS + 第二方核对两轮）**；**v4 = 补验轮（2026-10-08）：补做 §6 要求的 IAB 实测 + `jwt-expiry.spec.ts` 加严 + 新登 E2E-F-207**。

## 状态

**✅ done（2026-10-08）** —— 20/20 测试点 PASS（0 FAIL / 0 BLOCKED / 0 N/A）；回归钉 `e2e/security-boundaries.spec.ts` **9/9 首跑绿**；5 个执行期 [M] 决策点**全部落定**（D-46 / D-47 / D-48 / M4 用户拍板，M5 由证据关闭）；账本本阶段名下 7 条（F-27 后续 / F-28 / F-182 / F-200 / F-201 / F-202 / F-203）**全部 ✅**，新发现 E2E-F-204 按约定**不回挂**；`e2e_stage_audit.py --all` **30 阶段 0 FAIL**；详档 [report.md](report.md)。
> 状态口径：**执行者不自行宣布 done** —— 本状态经 RUNBOOK §13.3 第二方核对（子代理独立复跑）后落定，核对结论见 report §8。

## 一、已做 ✅

| 项 | 证据 |
|----|------|
| **计划期调研（AGENTS §〇 功课 ①~④）** | 已读实现文件 9 个 + 测试文件 4 个 + 决策/ADR（architecture 决策 7/8/11/12/18、e2e-roadmap D-27/28/30/35、stage-109a/112）+ APISIX 官方文档 jwt-auth / cors / limit-count 三页 |
| **只读安全探针 6 条** | ① 匿名 `POST /api/v1/auth/refresh` → **200 + 24h JWT（user_id=1）**，该 token 经网关读 `/users/me` 返 `account=echo` ② 匿名访问受保护端点 → 401 ③ 直连 `:8894` 伪造 `X-User-Id: 2` → 200 `smoke_user` ④ 限流 70 次 = 60 非 429 + 10×429 ⑤ CORS 恶意 origin 无 `Access-Control-Allow-*` ⑥ reports `user_id` 不等 → 403，别名 `userId`/`id` → 200 且响应体逐字相同 |
| **建档产物** | [plan.md](plan.md)：20 测试点（组 A~F）+ 6 个 TDD 循环 L1~L6 + 5 个执行期 [M] 决策点 + §0.1 事实表 **18 项** + §0.2 开工复核 **7 项** |
| **账本 3 条新登** | `E2E-F-201`（匿名 refresh 认证绕过，**现已翻 ✅**）/ `E2E-F-202`（可信链默认 fail-open + 8894 暴露，含 dev overlay 补正）/ `E2E-F-203`（cookie 无 `SameSite`/`Secure` 而注释称 Lax + 前端 `jti` 契约后端零实现） |
| **决策裁定（3/5）** | **D-46** = M1 硬 401 ｜ **D-47** = M2 默认值分离 + 缺 CIDR 拒绝启动 + 收 8894（三件一起）｜ **D-48** = M3 双密钥并存窗口（须附 ADR）。均 2026-10-07 用户经 AskUserQuestion 拍板，已写入 [decisions.md](../../decisions.md) + plan §4 |
| **L1 已修（F-201 认证绕过）** | TDD RED 6 例（前 5 例实测 200 全红）→ GREEN 全绿（`auth_handler.go` 的 `refresh` 重写：无令牌/过期/签名不符/UserID==0 一律 401，有效令牌同一 user_id 续期）；`go test ./...` 全模块 rc=0 + `go vet` 干净；新增 `auth_refresh_guard_test.go` 6 例 |
| **L1 运行时前后对照（隔离容器）** | 不动在跑的栈：另起两个容器（`NACOS_ENABLED=false` + 独立端口 18894/18895，env 由运行容器转存后删）。**旧码 v0.1.32 匿名 refresh = `200 + Set-Cookie(token) + body.accessToken`；新码 v0.1.33 = `401 + 无 Set-Cookie + 无 accessToken`**；有效令牌（Bearer / Cookie）两条路径新旧均 200 ⇒ 修复为外科式。容器与临时 env 文件均已清理 |
| **镜像** | `emotion-echo/web-bff:v0.1.33` 已构建（`apps.yml` tag 已递增 + 注释）。**注意：运行中的栈仍跑 v0.1.32，修复尚未生效**（见 F18） |
| **§2.5 残留清理** | 用户批准后删除远端 `fix/f199-lipsync-pinyin` + `feat/f199-tts-speed-config`；远端现存仅 `origin/main`；本地已合并分支仅 main |
| **门禁** | `e2e_stage_audit.py --all` → 30 阶段 0 FAIL（多次）；`check_git_layout --strict` / `check_orphan_outputs` / `check_residual` / `check_soft_asserts` / `check_secrets` 全 GREEN |

## 二、未做 ❌

| 项 | 说明 |
|----|------|
| **20 个测试点** | 仅组 A #1（匿名 refresh 401）与本轮修复对应；**其余约 19 条一条未执行** |
| **`report.md` / `screenshots/` / 回归钉 spec** | 均不存在（`[V]` 类测试点无一取证） |
| **[M] 决策点** | **M4**（备份生产化范围）/ **M5**（资源级越权发现归属）**仍未升级给用户**；M1/M2/M3 已裁定（D-46/47/48） |
| **D-47（M2）实施** | 未做：prod overlay 默认值分离 / 非 dev 且 CIDR 空时 fail-fast / 收 8894 宿主映射。**运行时验证受 F18 阻塞**（要动运行栈） |
| **D-48（M3）实施** | 未做：BFF 双密钥 + `kid` + 验证 APISIX consumer 是否支持多凭据；**落地必须附 ADR + `architecture/decisions.md`** |
| **F-28 / F-27 follow-on** | 密钥轮换机制未落地（= D-48 待实施）；备份/恢复生产化封装（脚本/cron/异地）未做 |
| **F-182 / F-200 / F-203** | `smoke_bff_chat_grpc.sh` 契约 4/7 仍恒 401；摄像头 TypeError 兜底仍无 `error.name` 落痕、文案未分支化；cookie `SameSite` 与 `jti` 契约未修（TDD L4 未开） |
| **L2 / L3 / L4 / L5 / L6** | 全部未开工 |
| **运行栈未重建** | 修复与镜像 v0.1.33 均**未对运行环境生效**；F18 的栈归属问题未处置（按用户环境不宜擅动） |

## 三、环境基线（本轮实测，供下次会话对照）

**🔴 运行栈与 RUNBOOK §2.1 不一致（E2E-F-202 的成因，plan §0.1 F18）**：

- **后端 17 容器** healthy（postgres/redis/kafka/nacos/etcd/minio/apisix + 6 应用服务 + xtts/sensevoice/fer），但其容器标签 `com.docker.compose.project.config_files` 指向 **`D:\源码\Emotion-Echo-f198\deploy\{infra,apps}.yml`** —— 由**另一个仍在磁盘、已非注册 worktree 的目录**创建（`git worktree list` 只剩主目录）。
- **未加载 `compose.dev.yml`**：运行容器实测 `BFF_TRUST_APISIX=false`（apps.yml 默认值），启动日志 `[warn] TrustAPISIX=false; dev mode, any X-User-Id accepted (DO NOT use in prod)`；无 `BFF_APISIX_CIDRS`。⇒ 这是"宿主直连 8894 伪造成功"的直接原因。
- **`:3000` 不是容器**：`emotion-echo-web` 容器 `Exited (0) 2 days ago`；端口由**宿主 `node.exe`（PID 1800）跑 `nuxt dev --port 3000`** 提供，命令行指向**主 worktree** `D:\源码\Emotion-Echo\emotion-echo-web` ⇒ **前端是当前 main 代码，后端不是**（F-196 铁律的典型场景）。
- `deploy/.env.local` 存在（内容未读未打印，JWT secret 实测为真值 64-hex 而非占位符）；**无 `deploy/.devmode-session`**（双轨锁空闲）。
- main = `1396b59`（本轮 PR 合并后另变）；本会话**未停止/未重建任何容器**，仅临时起过 2 个隔离探针容器并已删除。
- 陷阱复现：`| tail` 会吃掉 `docker compose build` 的退出码（首次构建因服务名写成 `web-bff` 而实际失败，却报了 exit 0）——**构建后必须核对镜像 tag 是否真的出现**。

## 四、下次开工第一步（照 [plan.md](plan.md) §0.2）

1. **先定运行栈归属**（F18）：接管现栈还是重起基线栈。`container_name` 固定 ⇒ 直接 `up` 会冲突；建议确认前端 dev server 可中断后 `down` → 按 §2.1 全量起（`-f compose.dev.yml --env-file .env.local --profile dev`）→ 管理 `deploy/.devmode-session`。
2. 环境基线复核：Nacos `count:6`、`db-migrate` ExitCode 0、6 应用服务 healthy。
3. **重跑 F1/F3/F6/F9 探针**（禁止默认其仍坏或仍好）；F1 应随 v0.1.33 落地转 401（组 A #1 即可判 PASS 并留证）。
4. 服务身份先验（F-196）：确认 `:3000` 服务对象与代码版本，`[V]` 结论必须绑定被验对象。
5. 继续 TDD：**L2（令牌类型隔离）** 或直接按组推进；**D-47（M2）实施**（含运行时验证，需干净的基线栈）；D-48 落地时同步出 ADR。
6. 别忘了 [M] 决策点 **M4 / M5 需升级用户**。

## 五、补验轮（2026-10-08，done 后）

> 起因：收口轮未做 IAB（用户质询"进行 IAB 测试了吗"）。plan §6 风险表明确要求"L1 落地后**必须 IAB 实测**真实过期场景"，但该要求未进 §0.2 收口清单 ⇒ 收口轮漏做。本轮补做。

**已做 ✅**

| 项 | 证据 |
|----|------|
| **IAB 实测（browser-use）** | 演示账号登录 → 真实网关下用**签名有效但已过期**的 JWT 触发受保护请求 → 应用跳 `/login`（登录表单可见）、**零次 `/auth/refresh` 调用**；截图 `screenshots/expiry-redirect-to-login.png` |
| **curl 前后对照** | 有效令牌 → `/users/me` **200**；过期令牌 → **401** `{"message":"failed to verify jwt"}`（无 `code`）；过期令牌 → `/auth/refresh` **401**（Bearer + cookie 两条）；有效令牌 → `/auth/refresh` **200 + 新 token** |
| **HttpOnly 实测** | 页面 JS `document.cookie='access_token=…'` 被浏览器**拒绝**（同名 HttpOnly 已存在）⇒ 客户端无法覆盖/删除登录 cookie（安全正效应） |
| **`jwt-expiry.spec.ts` 加严** | 2 用例（过期冷启动 + 受保护接口返网关真实 401 体）× chromium/mobile = **4 passed**；新断言"零次 `/auth/refresh`" |
| **注释漂移修正** | `config.go:233`（`NewManager`→`NewManagerMulti`）、`useFaceEmotion.ts:133`（分支③措辞） |
| **新登账本** | **E2E-F-207**：前端 `code===10002` 续期分支为**死代码**（全仓无后端下发 10002）——owner = 决策门（需产品裁定是否实现滑动续期），**不回挂 E2E-29** |

**未做 ❌ / 边界**

| 项 | 说明 |
|----|------|
| **E2E-F-207 的修复** | 未修（属产品取舍：是否实现滑动续期；当前"过期即登出跳登录"行为**安全**） |
| **plan §0.2 清单补线** | 已补（"改变前端用户可见行为的修复必须 IAB 实测真实场景"），防复发 |

**环境**：dev 栈运行中（web-bff `v0.1.36`，其余 18 容器 healthy）；`:3000` 由宿主 `nuxt dev`（主 worktree）服务；本轮注册并释放了 `deploy/.devmode-session` 锁。

## 六、E2E-F-207 follow-up（2026-10-08，PR #178）

**用户裁定（AskUserQuestion）**：① E2E-F-207 → **实现真正的滑动续期**；② 会话吊销策略 → 保持自动续期方向；③ 其余范围内观察项 / 既有债 → **先进文档，等下一轮**。

**已做 ✅**：前端滑动续期落地（`app/lib/tokenRenewal.ts` 16 单测 + `plugins/init.ts` 接线 + 删两处 `code===10002` 死分支 + `refreshAccessToken` 导出）；回归钉 `e2e/token-renewal.spec.ts`（2 用例 × 2 project）；决策 **D-49**；账本 **E2E-F-207 → ✅**；landed plan `docs/legacy-plans/landed/sliding-token-renewal.md`。
**验收**：vitest 全仓 **657 passed** / `tsc` 干净 / Playwright `token-renewal`+`jwt-expiry` **8 passed** / IAB 实测 `renewNow()` → `/auth/refresh` **200** + 新令牌 `exp-iat=86400`。
**执行期抓到 2 个只在运行时/端到端暴露的问题**：`isAuthenticated`（还要求 userInfo.id）挡住排程；SPA 登录后不排程（插件只跑一次）——均已修（无条件调用 + `watch(token)` 重排）。

**未做 ❌**：其余范围内观察项（423 `Retry-After` / `useApi.ts:159` jti 注释漂移 / IDOR 字面参数名白名单）与既有债（账本重复行 E2E-F-115/117/119 = E2E-F-179）**按用户指示留待下一轮**，本轮只进文档。**→ 已全部解决（2026-10-08 遗留项跟进轮，见 §七）**。


## 七、遗留项跟进轮（2026-10-08，done 后）

**用户指示**：解决上一阶段（E2E-29）的遗留任务。**结论：§六 未做 ❌ 列出的 4 项全部解决**，E2E-29 遗留清零。

| # | 遗留项 | 状态 | 落地 |
|---|--------|------|------|
| 1 | 423 `Retry-After` | ✅ | `authlock.LoginLockStore.RetryAfter` + handler 设头（423 与触发锁定的 401）；运行时 `Retry-After: 900` |
| 2 | `useApi.ts` jti 注释漂移 | ✅ | 注释如实化 + 4 处陈旧 plan 路径修正 |
| 3 | IDOR 字面参数名白名单 | ✅ | `userIDQuery` 身份别名同守（数字别名不符 → 403）；回归钉 9/9 |
| 4 | 账本重复行 E2E-F-115/117/119 | ✅ | 删 I 段重复 3 行 + F-119 裁定（保留 H 段）；`E2E-F-179 → ✅`；audit 0 FAIL |

**镜像**：`emotion-echo/web-bff:v0.1.37`（承接遗留项 1+3；`Created=2026-10-08T02:21:27Z`），其余 18 容器沿用 E2E-29 基线。
**门禁**：go build/vet/test 全绿；audit --all 0 FAIL；4 个仓库守卫 GREEN；vitest 657 / tsc 0 / Playwright `security-boundaries` 9/9。
**详档**：[report.md](report.md) §9。**本轨收工三查**：STATUS 已写（本节）→ `deploy/.devmode-session` 已删 → §2.5 三连自检（见收口）。
