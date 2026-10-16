---
status: 9/10 PASS（[A] 7/7 全绿 + 浏览器 [V] T-8/T-9 PASS）；T-10 BLOCKED（既有账 CL-F-17 数字人 VRM 加载失败）
stage: CL-07（前端半）
slug: cl-07-emotion-display
scope: 情绪实时回显（顶部状态条 + 数字人情绪；原 G2 消息角标经用户裁定不做）
owner: Lane-CL-G（接手原 C 会话位）
created: 2026-10-16
updated: 2026-10-16（第二方核对 PASS + 按裁定移除 G2 + 文档纠偏）
---

# CL-07（前端半）· 执行记录（report）

> **结论：9/10 测试点 PASS** —— [A] 7/7；浏览器 [V] **T-8/T-9 PASS**（第二方核对子代理**独立复现**）；**T-10 BLOCKED**（数字人自身加载失败 = 既有账 **CL-F-17**，与本改动无关）。
> **未自行宣布 done**：第二方核对（RUNBOOK §6.7）已 **PASS**，最终 done 由用户/审阅裁定。

## 起点

- 承接 `stages/cl-07-fusion-consumption/`（后端切片：融合产物 → 视图 → 报表 → proto/BFF 透传，**已 done 并合并 `bf401350`**）。
- 来源计划 `docs/plans/chat-emotion-realtime-display.md`（G1~G5）；开工前提：D 轨（CL-05）已 done 并释放 `pages/chat/conversation/[id].vue`。
- 登记：PR [#242](https://github.com/Exist-a/emotion-echo/pull/242)（H 轨占位 + 握手，**已合并**）；阶段 PR [#243](https://github.com/Exist-a/emotion-echo/pull/243)。
- worktree `../Emotion-Echo-cl07`（`pnpm install --frozen-lockfile` + `nuxt prepare`）。

## 交付（按阶段）

| Phase | 内容 | 结果 |
|---|---|---|
| 2 | `app/composables/useCurrentEmotion.ts`：按消息取融合情绪 + 护栏文案 + **诚实降级**（无 face/voice ⇒ 标「文本」）+ 失败静默降级 | ✅ 15 用例 |
| 3 | `app/components/emotion/CurrentEmotionBar.vue`：顶部「此刻的状态」条；`state=null` 整块不渲染 | ✅ 5 用例 |
| 4 | ~~`[id].vue` 用户消息融合角标~~ | ❌ **用户裁定不做**（见下） |
| 5 | `[id].vue`：数字人情绪打通（**方案 A**：流结束后立即拉 + 4.5s 补拉一次；`aiEmotion` 有值优先） | ✅ 三处 `onFinish` 全接 |
| — | `lib/apiRoutes.ts`：登记 `/emotion/message/:messageId/fused` | ✅ 契约测试绿 |
| — | **BFF 缺陷修复**：`emotion_query_handler.go` 三处改 `session.WithRequestAuth(c)` | ✅ + 回归钉 |
| 6 | 浏览器实测（IAB · **生产构建**） | ✅ 见「浏览器实测」 |

## 用户裁定变更（2026-10-16，实测之后）

**G2 消息级融合角标不做** —— 用户看到实测效果后明确「消息左侧的角标不需要」。

- **已移除**：`[id].vue` 的角标 DOM/CSS、`v-memo` 中为角标加的依赖、`useCurrentEmotion` 的 `byMessage`/`ensureMessages`（含其 4 个用例）、旧角标回归钉。
- **新增反向回归钉** `cl07-emotion-display.architecture.test.ts`：断言页面**不得**再出现 `emotion-chip`/`byMessage`，且 `v-memo` 恰为原 4 项。
- **保留**：顶部状态条（G1/G3/G5）+ 数字人情绪（G4）。

## 证据

### 单元 / 契约级（[A] 7/7 全绿）

| 测试点 | 命令 | 结果 |
|---|---|---|
| T-1 模态解析 + 非多模态判定 | `npx vitest run app/composables/useCurrentEmotion.test.ts` | ✅（15 用例） |
| T-2 护栏文案不含来源词 | 同上（禁用词逐条断言） | ✅ |
| T-3 诚实降级（文本/综合） | 同上 | ✅ |
| T-4 取数失败静默不抛 | 同上 | ✅ |
| T-5 组件空态/有值 | `npx vitest run app/components/emotion/CurrentEmotionBar.test.ts` | ✅（5 用例） |
| T-6 路由登记契约 | `npx vitest run app/lib/apiRoutes.test.ts` | ✅（3 用例） |
| T-7 页面接线契约（含 G2 已移除的反向断言 + 三处 onFinish） | `npx vitest run app/pages/chat/conversation/cl07-emotion-display.architecture.test.ts` | ✅（6 用例） |
| 全量前端 | `npx vitest run` | ✅ **87 文件 / 732 用例** |
| BFF 全量 | `go build ./... && go test ./... -count=1`（`emotion-echo-web-bff`） | ✅ 全 `ok`（11 包） |

### 浏览器实测（IAB · 生产构建）—— [V] T-8/T-9 PASS，T-10 BLOCKED

**取数口径**：宿主 `pnpm dev` 在本环境 IAB 下**不 hydration**（`#__nuxt.__vue_app__ === false`；dev 的 `@fs` 模块 URL 内嵌中文仓库路径 `D:/源码/...`）。
**对照实验**：未改动的 `main` 同环境**同样不 hydration** ⇒ 非本改动。
**绕过（已验证）**：`pnpm build` 产生产物（哈希资源名、无中文路径）→ `PORT=3000 NUXT_PUBLIC_API_BASE_URL=http://localhost:19080/api/v1 node .output/server/index.mjs` ⇒ **成功 hydration**。

| 测试点 | 证据（DOM/截图；第二方核对已独立复现） |
|---|---|
| **T-8** 顶部状态条 | `.current-emotion-bar` 文本 = **「此刻的状态：开心 文本」**（`__vue_app__=true`，截图可见） |
| **T-9** 负向对照（纯文本不夸大） | 该消息 `available_modalities` = `["text"]`（线上 JSON 字符串形态）⇒ 模态标签「**文本**」而非「综合」 |
| T-10 数字人情绪 | ⛔ **BLOCKED** —— 页面显示「3D模型加载失败 / 重试」（既有账 **CL-F-17** VRM 竞态），无法观察表情；接线与架构钉已在码内，待 CL-F-17 修复后补验 |
| G2 角标 | **已移除**：`.emotion-chip` 计数 = **0**，源码零出现 |

### 本轮实测抓到并修复的真缺陷

1. **[高] BFF `/api/v1/emotion/*` 整族端点恒 502** —— `emotion_query_handler.go` 三个方法直接传 `c.Request.Context()`，**未包 `session.WithRequestAuth(c)`**（其余 10 个 handler 都包了）⇒ 下游 gRPC 缺 `x-user-id` metadata ⇒ ai-svc 返 `Unauthenticated`。
   - 端到端复现（同一 token）：`/conversations/*` → 200、`/reports/*` → 200，而 `/emotion/message/951/fused`、`/emotion/conversation/705` → **502 `missing x-user-id metadata`**。
   - **E2E-F-125 同型**；且它让本阶段功能**根本出不了数**（第一轮浏览器验证"状态条不出现"的真因）。
   - 修复：三处 `session.WithRequestAuth(c)`；回归钉 `emotion_query_handler_userid_test.go`（断言下游 ctx 带 user id、无头时不伪造身份；突变实验证明能红）。
2. **[中]（仅"G2 角标存在"期间成立）`[id].vue` 的 `v-memo` 依赖列表缺角标状态** —— `v-memo` 按固定依赖缓存子树，角标数据**异步**到达后**永不重渲染**。随 G2 被裁掉，该依赖已回退为原 4 项。**教训**：往 `v-memo` 子树里加异步数据必须同步加依赖（单测测不到，只有浏览器能暴露）。

### 环境口径（记录；非本改动）

- **dev 不 hydration**：见上（中文路径；对照实验证明非本改动）。
- **`emotion-echo-web` 镜像无法重建**：`package.json` 声明 `vue-router: ^5.4.0`，而 Dockerfile 的 `npm install` 走 `package-lock.json`（其中 `nuxt@4.2.2` 要求 `vue-router: ^4.6.3`）⇒ **ERESOLVE 失败**。宿主走 `pnpm-lock.yaml` 所以无碍。属既有缺陷，**未就地修**（改依赖版本影响面大，另议）。
- ⚠️ **因此当前跑的 `emotion-echo-web` 容器是旧镜像 `v0.1.9`**（容器内不含 CL-07）⇒ 直接看 `localhost:3000` 是**旧前端**；本阶段 [V] 验收是**生产构建产物**（`.output`）达成的。

## 第二方核对（RUNBOOK §6.7）

**独立子代理结论：PASS**（D1~D9 全过）。

- 每项声明均经**代码 + 突变实验**独立复现（护栏 4 条变红 / 降级 2 条 / 组件 1 条 / BFF 回归钉 FAIL / 页面钉 2 条变红；均还原干净）。
- **端到端自测**：`/emotion/message/951/fused` → **200**、`/emotion/conversation/705` → 200、`/reports/daily` → 200（对照）⇒ BFF 修复成立。
- **浏览器 [V] 自行复现**：它自己起 `.output` + IAB 实测 `__vue_app__=true`、状态条文本「此刻的状态：开心 文本」、`.emotion-chip`=0 ⇒ 与报告**逐字一致**；核对后已恢复容器。
- ERESOLVE 说法、T-10 未验的说法均**属实**。
- 其提出的**文档问题已全部整改**：本文件重写以消除前后矛盾；失效证据行、不存在的钉文件名、陈旧计数均已纠正；**架构钉已加强**为"三处 `onFinish` 均接 + `emotion-chip` 零出现 + `v-memo` 恰 4 项"。

## 结论

- **[A] 7/7 PASS**；**[V] T-8/T-9 PASS**（含第二方核对独立复现）；**T-10 BLOCKED**（CL-F-17，他项）。
- 交付物：对话页顶部「此刻的状态」+ 数字人情绪接线；**G2 角标按用户裁定不做**。
- 顺带修复 **BFF `/emotion/*` 整族 502**（阻塞本功能的高危缺陷，E2E-F-125 同型）。
- **未做**：T-10（待 CL-F-17）；`new.vue` 未挂状态条（**有意偏离**：新会话无消息 ⇒ 无锚点）。
- 落地计划仍在 `docs/plans/chat-emotion-realtime-display.md`（G2 已裁定不做，收口时同步状态）。

## 经验

1. **先做对照实验再归因**：hydration 失败先怀疑自己的改动，用"未改动的主目录"同环境复现 ⇒ 一次排除。
2. **`pnpm dev` 别用 `| tail` 包**：输出被缓冲，进程存活期间读不到日志。
3. **dev 服务只监听 `[::1]`**：`curl 127.0.0.1` 超时（假"服务没起"）。
4. **IAB 鉴权链**：HttpOnly `access_token` 必须由**应用自身登录流程**写入 Pinia；页面上下文手动 `fetch` 登录只落 cookie，客户端 store 仍空 ⇒ 下游 gRPC 缺 `x-user-id` 返 502。
5. **"unit 全绿但端到端恒不可用"是本仓反复模式**（E2E-F-125 同型）：handler 少包一层 `WithRequestAuth` 单测发现不了，**必须真走网关实测**。
6. **往 `v-memo` 子树里加异步数据，必须同步加依赖**（否则永不重渲染）。
7. **文档是叠加改出来的 ⇒ 会自相矛盾**：分多轮追加内容时必须回头清旧段（本轮第二方核对正是抓到这一点）。
