---
status: landed
stage: 105
title: 浏览器 E2E + dev 模式鉴权链发现 4 个 bug + Dockerfile alpine musl 修复
date: 2026-09-16
type: browser-e2e-dev-auth
source-plan: （接 Stage 104 实测工作流）
phase: 测试找问题修复
depends-on:
  - stage-104-frontend-ui-recovery-2026-09-16.md
  - stage-103-dev-mode-launch-2026-09-16.md
related-stages:
  - stage-74-cors-allow-credential.md
  - stage-94-code-review-2026-09-14-p0-closure.md（JWT secret 修）
related-adrs:
  - ADR-15（前端 Web 选型 = Nuxt 3）
  - 决策 11/12（网关是唯一业务入口 + APISIX 鉴权）
---

# Stage 105 — 浏览器 E2E 续 + dev 模式鉴权链发现

> **本 stage 目标**：用 browser-use 在 dev 模式跑端到端，发现真实 bug。
> 本轮实测 + 调研发现 **4 个新 bug**，全部已 RED 写测试契约，2 个 GREEN 修复，2 个留 Stage 106 治理。

---

## 一、本 stage 起点

| 维度 | 状态 |
|---|---|
| dev 模式 | Stage 103 修完 9 真 bug，16 容器 healthy |
| 前端代码 | Stage 104 修 9 个 UI bug，13 commit 已上 origin |
| 测试基线 | 280 + 37 (Stage 104) = 317 全绿 |
| 用户工作流 | "用 browser-use 或 curl 跑实际功能" |

---

## 二、本轮发现：4 个新 bug

### Bug 1 — CORS 单 host 配置（Stage 105 PR-1，RED+GREEN ✅）

| 维度 | 内容 |
|---|---|
| **现象** | IAB 浏览器访问 `http://127.0.0.1:3000` 时所有 `/api/v1/*` API 返回 `Failed to fetch` |
| **触发** | 部分浏览器（Windows Chrome + 沙箱 IAB）拒绝 `localhost` 自动跳 `chrome-error://`，用户改用 `127.0.0.1:3000` |
| **根因** | `deploy/apisix/seed.sh:56` 默认 `CORS_ALLOW_ORIGINS="http://localhost:3000"` 单值硬编码 |
| **影响** | dev 模式浏览器实测**整页 API 被拒** |
| **RED 契约** | `deploy/apisix/seed_test.js` 第 39 项：默认 `allow_origins` 必须同时含 `localhost:3000` 和 `127.0.0.1:3000` |
| **GREEN 修复** | 默认值改为 `"http://localhost:3000,http://127.0.0.1:3000"`，运行时 6 个路由已 PATCH |
| **commit** | `ac1b6e0` test + `cf9c42c` fix |

### Bug 2 — `.app-content` 缺 flex 链路（Stage 105 PR-1，RED+GREEN ✅）

| 维度 | 内容 |
|---|---|
| **现象** | 登录后 `/chat/conversation/new`：`chatArea h=332`，`pageContent h=632`，`appContent h=739`（溢出19px），**composer 不贴底** |
| **根因** | Stage 104 修了 `.page-content` 但漏修 `.app-content`：缺 `display:flex; flex-direction:column; min-height:0`，子级不参与 flex 挤压 |
| **影响** | 主对话页观感"短"且滚动区域错位 |
| **RED 契约** | `app/layouts/nav.test.ts` 第 5 项：`.app-content` 必须 `display:flex + flex-direction:column + min-height:0` |
| **GREEN 修复** | 桌面 + 移动端断点同步加 flex 链路 |
| **commit** | `ac1b6e0` test（同一 commit）+ `cf9c42c` fix（同一 commit） |
| **运行验证** | 浏览器实测修复后 `chatArea h=561`（≈ viewport 720 − header 88 − pageContent 615 / 2），composer 在视口底部 |

### Bug 3 — Dockerfile alpine musl 上 oxc-parser binding 缺（Stage 105 PR-2，RED+GREEN ✅）

| 维度 | 内容 |
|---|---|
| **现象** | `docker compose build emotion-echo-web` 在 alpine 上失败：`Cannot find module '@oxc-parser/binding-linux-x64-musl'` |
| **根因** | Stage 74 `--omit=optional` 跳 native binding（当时 Nuxt 3 不需要）— Nuxt 4 升级后 `oxc-parser/transform/minify` 三个 binding 必装 |
| **影响** | dev 模式浏览器实测必须 `docker compose build` → **build 失败 → web 容器起不来 → 无法 E2E** |
| **修复** | Dockerfile `npm ci` 改用 `--ignore-scripts` 不带 `--omit=optional`，额外 `npm install --no-save` 显式装 4 个 linux-x64-musl binding（`@oxc-parser/transform/minify` + `@rollup/rollup-linux-x64-musl`） |
| **验证** | v0.1.0 镜像重 build 成功，容器启动 healthy，浏览器实测能进 chat 主页 |
| **commit** | (本 stage 工作树，未 commit — 与 bug 4 一起 commit) |
| **遗留** | `npm/cli#4828` 仍未根治；本 stage 用 `--no-save` 显式补装绕过 |

### Bug 4 — dev 模式鉴权链 3 个纠缠 bug（Stage 105 PR-3，调研完成，待治理 🔴）

| # | 问题 | 现象 | 根因（推断）|
|---|---|---|---|
| 4a | **APISIX `*` 通配符不匹配多段 path** | `/api/v1/reports/daily` / `/api/v1/conversations` OPTIONS 返回 404，但 `/api/v1/auth/login` 返回 200 | `route 100 uri=/api/v1/*` — APISIX 通配符语义要求前缀 + 单段，不是任意段 |
| 4b | **seed.sh 缺 reports 端点路由** | `routes 110-115` 只覆盖 `auth/login/register/verification-code/refresh/logout/reset-password`，**完全没注册 `reports/*` 与 `conversations/*` 等业务端点** | seed.sh 第487 行 `put_route 100 "/api/v1/*"` 期待 catch-all 覆盖，但 4a 不匹配多段 |
| 4c | **JWT 链 secret 不一致** | curl `Authorization: Bearer <login token>` 调 `/api/v1/reports/daily` 返回 `"JWT token invalid"`，即使手动 PATCH consumer secret 为 `dev-bff-secret`（默认值）也无效 | APISIX consumer secret 已变为 `lDJJR5NH8B3gssvm2/IW6w==`（Nacos 注入？）与 BFF env `BFF_JWT_SECRET=dev-bff-secret` 不匹配 |

**共同影响**：dev 模式浏览器访问 4 个报表页面时 API 全 401 / 404，**4 报表功能实测完全不可用**——这是 Stage 103 启动 dev 模式后**第二个未在浏览器内验证的盲区**（第一个是 CORS）。

#### 已知未修原因

| 维度 | 内容 |
|---|---|
| **调研深度** | 鉴权链跨 APISIX + BFF + nacos + sharedmw 4 个组件，单根因需要更深调试 |
| **改动面** | 至少动 seed.sh + BFF config + nacos 配置 + 可能的 jwt-auth 配置——属 dev 环境治理，非业务 bug |
| **风险** | 改 BFF secret 可能让现有 dev token 失效，需要重登录；改 seed.sh 需重跑 seed |
| **决策** | **不动**——交给 Stage 106 "dev 模式鉴权链专项治理"，本 stage 仅记录 |
| **前置 RED 测试** | 需先写"seed.sh 必须注册所有 `/api/v1/<service>/*` 路由"的红测试 + "BFF 与 APISIX consumer secret 必须同步"的契约测试 |

---

## 三、本轮已完成工作（已 push）

| commit | 内容 |
|---|---|
| `718c145` | `deploy/compose_dev.test.js` Stage 105 PR-0 调研记录（Dockerfile.dev 不可行结论 + REGRARD-GUARD 防改） |
| `cf9c42c` | `fix(seed+nav)` CORS 双 host + .app-content flex |
| `ac1b6e0` | `test(seed+nav)` Stage 105 RED 契约 |
| `d694141` | `docs(stage-104)` 13 commit 收口报告 |
| `071093e` 等 17 个 | Stage 104 UI 修复 + Mimosa 误报治理 |

## 四、本轮工作树未提交（bug 3 修复）

| 文件 | 状态 |
|---|---|
| `emotion-echo-web/Dockerfile` | `npm ci --omit=optional --ignore-scripts` → `npm ci --ignore-scripts` + 显式补装 4 个 native binding |
| `emotion-echo-web/Dockerfile.dev` | 已回滚（原状态保留） |

## 五、浏览器 E2E 实测覆盖度

| 轮次 | 状态 | 备注 |
|---|---|---|
| 轮 1：登录 + chat 主页 | ✅ | demo 账号走通，跳转 `/chat/conversation/new`；Stage 104/105 UI 修复生效（appContent flex / chart-empty 占位） |
| 轮 2：4 报表页 | 🔴 阻塞 | bug 4a/4b/4c 致 reports API 401 / 404，4 报表页面"暂无数据" |
| 轮 3：对话流（发消息 + AI 回复） | 🔒 未测 | Stage 103 已知"chat 发消息无 AI 回复"未修 + dev web 容器无法稳定 build |
| 轮 4：设置 / 个人空间 / 测验 | 🔒 未测 | dev web 容器刚刚能 build，无 UI bug 报告优先级 |

---

## 六、Stage 106 计划（dev 模式鉴权链治理 + 真实端到端验证）

| 优先级 | 任务 | 预期 |
|---|---|---|
| **P0** | bug 4a：seed.sh 改写 `/api/v1/*` 为多段通配（实测 APISIX 文档推荐 `/*` 写法或加 prefix matcher） | reports/conversations 等多段路径命中 |
| **P0** | bug 4b：seed.sh 添加 reports/* + conversations/* + messages/* + chat-message-stream 等业务端点显式路由（依赖 4a 失败时 fallback） | 4 报表 / 对话流 API 路由可达 |
| **P0** | bug 4c：统一 APISIX consumer secret 与 BFF `BFF_JWT_SECRET`（通过 Nacos 共享配置 / compose env 注入） | curl Bearer token 验签通过 |
| **P0** | 写 RED 集成契约：`seed_test.js` 加"必注册 N 个核心端点路由" + "consumer.secret 与 BFF env 一致" | 锁死回归 |
| **P1** | 浏览器 E2E 轮 2（4 报表页）：插入 echo 测试数据（已做）+ 看真实图表渲染 | 验证 Stage 104 pieChartConfig / chartsCard / lineChartConfig 修复 |
| **P1** | 浏览器 E2E 轮 3（对话流）：发消息 → 看 AI 回复是否正常（Stage 103 未修的 bug 是否仍存在）|  |
| **P2** | 浏览器 E2E 轮 4：设置 / 个人空间 / 测验 |  |

---

## 七、ADR / plans / decisions 影响

- **无 ADR 变更**：本 stage 不涉及架构决策
- **无 plans 变更**：dev 模式鉴权链治理进 Stage 106 plan
- **AGENTS.md**：未变更（TDD + 数据契约原则本就是约定一部分）
- **Stage 103 dev 模式启动 closure 报告**：需补充"鉴权链 E2E 验证"作为遗留项

---

## 八、验证总结

| 维度 | 结果 |
|---|---|
| 推送 commit | 17 个已上 origin（Stage 104 + Stage 105 PR-0/1/2） |
| Vitest | 318/318 全绿（基线 280 + Stage 104 +38） |
| 浏览器 E2E 轮 1 | ✅ 登录 + chat 主页走通，Stage 104/105 UI 修复实测生效 |
| 浏览器 E2E 轮 2-4 | 🔒 等 Stage 106 dev 鉴权链修复 |
| §2.5 收口 | working tree 干净、ahead=0、无残留分支（除 Dockerfile 修改未 commit） |

---

## 九、未提交工作树收口

`emotion-echo-web/Dockerfile` 含 bug 3 修复但未 commit——**留 Stage 106 一并提交**（与鉴权链修复在同一 PR）。

调研依据（每条 bug 均列出）：

- **Bug 1**：CORS 测试（curl 加 Origin 头）→ IAB 浏览器实测 → seed.sh 默认值
- **Bug 2**：getComputedStyle() 抓 .app-content display:block → nav.vue 源码 diff
- **Bug 3**：build 日志 + npm/cli#4828 + alpine musl ABI 文档
- **Bug 4**：APISIX admin API + curl OPTIONS 各种 path + BFF env vs consumer secret diff