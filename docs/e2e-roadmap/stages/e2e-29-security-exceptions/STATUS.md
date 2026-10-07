# E2E-29 横切：异常与安全 — STATUS v1（建档轮收工笔记）

> 本文件是 Lane E 的**本轨进度事实源**（[parallel-tracks.md](../../../_meta/parallel-tracks.md) §五 指定路径）。
> 格式：已做 ✅ / 未做 ❌ 分列，**禁止美化**。下次 Lane E 会话开工前必读本文件 + [plan.md](plan.md) §0.2 开工复核清单。

## 状态

**🚧 in-progress（建档完成，测试点未执行）** —— 2026-10-07 本轮只做了**计划期调研 + 建档**（PR #168，squash 后 main = `b6b1570`）。
阶段**未**开工执行：20 个测试点无一有结论，`report.md` / `screenshots/` / 回归钉均不存在（审计器对 e2e-29 报 A0 WARN "未开工，无 report.md"，符合 just-in-time 约定）。

## 一、已做 ✅

| 项 | 证据 |
|----|------|
| **计划期调研（AGENTS §〇 功课 ①~④）** | 已读实现文件 9 个（`web-bff/main.go:210-320`、`internal/auth/jwt.go`、`internal/handler/auth_handler.go:212-302`、`internal/handler/analytics_handler.go:47-72`、`internal/config/config.go:340-360`、`shared/pkg/middleware/gin_auth.go`、`apisix/seed.sh:300-660`、`docker-compose.apps.yml:654-723`、`compose.prod.yml:33-60`）；测试文件 4 个；ADR/决策（architecture 决策 7/8/11/12/18 + e2e-roadmap D-27/28/30/35 + stage-109a/112）；APISIX 官方文档 jwt-auth / cors / limit-count 三页 |
| **只读安全探针 6 条（dev mode，17 容器 healthy）** | ① 匿名 `POST /api/v1/auth/refresh` → **200 + 24h JWT（user_id=1）**，该 token 经网关读 `/users/me` 返 `account=echo` ② 匿名访问受保护端点 → 401 ③ 直连 `:8894` 伪造 `X-User-Id: 2` → 200 `smoke_user` ④ 限流 70 次 = 60 非 429 + 10×429 ⑤ CORS 恶意 origin 无 `Access-Control-Allow-*` ⑥ reports `user_id` 不等 → 403，别名 `userId`/`id` → 200 且响应体逐字相同 |
| **建档产物** | [plan.md](plan.md)：20 测试点（组 A~F）+ 6 个 TDD 循环 L1~L6 + 5 个执行期 [M] 决策点 M1~M5 + §0.1 事实表 17 项 + §0.2 开工复核 6 项 |
| **账本新登 3 条** | `E2E-F-201`（匿名 refresh 发 `user_id=1` 有效 JWT = 认证绕过）/ `E2E-F-202`（`BFF_TRUST_APISIX` 默认 `false` + 8894 暴露）/ `E2E-F-203`（cookie 无 `SameSite`/`Secure` 而注释称 Lax + 前端 `jti` 契约后端零实现） |
| **roadmap 同步** | 排期表 E2E-29 `pending → in-progress`；当前激活指针；详档表登记；front-matter `last-refresh` |
| **门禁** | `python scripts/e2e_stage_audit.py --all` → 30 阶段 0 FAIL（合并前后各跑一次）；`check_git_layout --strict` / `check_orphan_outputs` / `check_residual` / `check_soft_asserts` / `check_secrets` 全 GREEN；PR #168 与 main push（`b6b1570`）check-runs 无失败 |
| **本轮分支收口** | 本地 + 远端 `docs/e2e-29-plan` 已删（§2.5）；working tree 干净；`git branch --merged main` 仅 main |

## 二、未做 ❌

| 项 | 说明 |
|----|------|
| **20 个测试点** | 一条未执行（含 F-201 认证绕过**未修**——修法 = plan §2 组 A #1 / TDD L1） |
| **`report.md` / `screenshots/` / 回归钉 spec** | 均不存在（阶段未开工，非缺失） |
| **5 个 [M] 决策点** | M1（refresh 无令牌语义）/ M2（prod 信任链默认值）/ M3（密钥轮换形态）/ M4（备份生产化范围）/ M5（资源级越权归属）——**均未升级给用户裁定** |
| **JWT 密钥轮换机制（F-28）** | 未设计未落地；若落地方案触及认证/密钥属架构关键词 ⇒ 需 ADR + `architecture/decisions.md` |
| **备份/恢复生产化封装（F-27 follow-on）** | 未做（dev 演练口径已过，封装脚本/cron/异地均无） |
| **F-182 / F-200** | `smoke_bff_chat_grpc.sh` 契约 4/7 仍恒 401；摄像头 TypeError 兜底仍无 `error.name` 落痕、文案未分支化 |
| **§2.5 残留（非本阶段产出）** | 远端仍有 2 个**已合并**分支未删：`origin/fix/f199-lipsync-pinyin`（PR #166 squash 源）/ `origin/feat/f199-tts-speed-config`（PR #167 squash 源，`git diff main` = 0 行）。**未自行删除**（远端不可逆操作，已在 PR #168 请确认） |

## 三、环境基线（建档轮实测，供下次会话对照）

- `docker ps`：17 容器 healthy（postgres / redis / kafka / nacos / etcd / minio / apisix / 6 应用服务 / xtts / sensevoice / fer）
- `deploy/.env.local` 存在（内容未读未打印）；**无 `deploy/.devmode-session`**（双轨锁空闲）
- main = `b6b1570`；本会话**未启动/未停止任何容器**（环境是先前的运行态）

## 四、下次开工第一步（照 [plan.md](plan.md) §0.2）

1. 环境复核（RUNBOOK §2.1，带 `--env-file .env.local` + `--profile dev`）→ Nacos `count:6`、`db-migrate` ExitCode 0
2. **重跑 §0.1 的 F1/F3/F6/F9 探针**（禁止默认其仍坏或仍好）
3. 服务身份先验（F-196 铁律：`:3000` 到底是本地 dev server 还是 web 容器）
4. 从 TDD 循环 **L1（匿名 refresh 必须 401）** 起，先写 RED
