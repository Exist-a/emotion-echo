---
status: partial（代码 [A] 项全绿；浏览器 [V] 项 BLOCKED —— 环境 dev 客户端不 hydration）
stage: CL-07（前端半）
slug: cl-07-emotion-display
scope: 情绪实时回显（顶部状态条 / 消息融合角标 / 数字人情绪）
owner: Lane-CL-G（接手原 C 会话位）
created: 2026-10-16
---

# CL-07（前端半）· 执行记录（report）

> **结论：partial** —— 7/10 测试点 [A] 全绿（含全量 730 前端单测）；**T-8/T-9/T-10（浏览器 [V]）BLOCKED**：
> 本环境的 Nuxt **dev 客户端在 IAB webview 下不 hydration**（`#__nuxt.__vue_app__ === false`），
> 表单/点击走**原生提交**、Vue 处理器不执行 ⇒ 无法做真实浏览器验收。
> **对照实验证明与本改动无关**（见 §证据）：未改动的主目录（`main`）同一环境**同样不 hydration**。

## 起点

- 承接 `stages/cl-07-fusion-consumption/`（后端切片：融合产物 → 视图 → 报表 → proto/BFF 透传，**已 done 并合并 `bf401350`**）。
- 来源计划 `docs/plans/chat-emotion-realtime-display.md`（G1~G5）；**开工前提**：D 轨（CL-05）已 done 并释放 `pages/chat/conversation/[id].vue`（此前 D/C 争该页）⇒ 无并发持有者。
- 登记：PR [#242](https://github.com/Exist-a/emotion-echo/pull/242)（H 轨占位 + 握手）；worktree `../Emotion-Echo-cl07`（`pnpm install --frozen-lockfile` + `nuxt prepare`）。
- 栈：本轨已持 `:cl07` 栈（20 容器）；前端用**宿主 `pnpm dev`**（RUNBOOK §2：非容器预构建）。

## 实施（TDD）

| Phase | 改动 | 结果 |
|---|---|---|
| 2 | `app/composables/useCurrentEmotion.ts`（取数 + 护栏文案 + 诚实降级 + 消息级缓存/防 N+1） | ✅ 19 用例 |
| 3 | `app/components/emotion/CurrentEmotionBar.vue`（顶部「此刻的状态」条，`state=null` 整块不渲染） | ✅ 5 用例 |
| 4 | `[id].vue`：气泡**融合情绪角标**（消费消息级缓存；`order:-1` 显示在用户气泡左侧） | ✅ 接线完成 |
| 5 | `[id].vue`：数字人情绪打通（**方案 A**：流结束后立即拉 + 4.5s 补拉一次；`aiEmotion` 有值优先） | ✅ 接线完成 |
| — | `lib/apiRoutes.ts`：登记 `emotionFusedByMessage`（`/emotion/message/:messageId/fused`） | ✅ 契约测试绿 |
| 6 | 浏览器实测（IAB） | ❌ **BLOCKED**（见下） |

**关键设计取舍（明写）**：
1. **取数走"按消息"而非"按会话"**：BFF 的 `/emotion/conversation/:id` 返回的是**未融合**情绪（`emotion_analysis` 路径），融合产物是 `fused_emotions.message_id` 粒度 ⇒ 必须用 `/emotion/message/:id/fused`（CL-07 后端切片已把它接通）。
2. **`emotionTag` 不用**：它在全仓零渲染且是**单模态**标签；本半用融合数据，避免"拿单模态冒充融合"。
3. **数字人表达的是"用户当前情绪"**（共情镜像），不是 AI 自身情绪 —— 与 G1「用户当前情绪回显」一致。
4. **`new.vue` 未挂状态条**（**有意偏离**计划 §3.2）：新会话尚无消息 ⇒ 无 messageId 可锚 ⇒ 挂了恒空。理由：该页发首条消息后会跳转 `[id].vue`，状态条在那里出现。

## 证据

| 项 | 命令/动作 | 输出 |
|---|---|---|
| 目标测试 | `npx vitest run app/composables/useCurrentEmotion.test.ts app/components/emotion/CurrentEmotionBar.test.ts app/lib/apiRoutes.test.ts` | **3 文件 27 用例全过** |
| 全量前端 | `npx vitest run` | **86 文件 / 730 用例全过**（无回归） |
| 路由契约 | 同上（`apiRoutes.test.ts` 扫全仓字面量 ⊆ `API_ROUTES`） | PASS |
| 护栏钉 | `useCurrentEmotion.test.ts`：对 6 种情绪断言文案**不含**「识别/检测/摄像头/分析/监控」 | PASS |
| 诚实降级钉 | 纯 text ⇒ `modalityLabel='文本'`；含 face/voice ⇒ `'综合'` | PASS |
| 降级不阻塞钉 | 取数抛错 ⇒ 静默 `null`、**不抛**（增强非依赖） | PASS |

### T-8/T-9/T-10 BLOCKED 的证据（含对照实验）

1. 起宿主 `pnpm dev`（worktree 源码，`:3000`）→ SSR HTML 正常返回（`301 → /chat`，登录页可达）。
2. 浏览器（IAB）打开 `/login`：表单可见，但 **`document.querySelector('#__nuxt').__vue_app__ === false`** ⇒ 客户端 Vue **未挂载**。
3. 行为印证：点「登录」/回车 ⇒ **浏览器原生表单提交**（URL 变 `/login?`、输入被清空），`window.fetch` 钩子**未捕获任何应用请求** ⇒ Vue 处理器从未执行。
4. 客户端入口脚本 `/_nuxt/@vite/client`、`/_nuxt/.../entry.async.js` 均 **HTTP 200**（服务端无编译错误；dev 日志仅 WARN）。
5. **对照实验（决定性）**：以**未改动的主目录**（`main`，`D:/源码/Emotion-Echo`）在 `:3010` 起同样 dev ⇒ **`__vue_app__` 同样为 false**。⇒ **不是本改动引入**。
6. 根因**假设**（未证实）：Nuxt dev 的 `@fs` 模块 URL 内嵌**中文仓库路径**（`D:/源码/...`），该 IAB webview 未能执行该模块链。生产构建产物是哈希资源名、不含中文路径 ⇒ 预期可绕过（**未验证**：需 `pnpm build`，而项目记录过"web 构建 + 19 容器 ⇒ 内存击穿冻结"，本轮**未冒险**）。

## 结论

- **[A] 7/10 PASS**：取数/护栏/降级/缓存/组件/路由契约全部有测试且绿；全量 730 前端用例无回归。
- **[V] 3/10 BLOCKED**：浏览器验收因**环境 dev hydration 失效**无法执行（对照实验证明非本改动）。
- **阶段判 `partial`**（按 RUNBOOK §6：有未完成项不得判 done）。
- **建议补验路径（二选一）**：
  1. 在**仓库路径不含中文**的环境跑 `pnpm dev` + IAB；或
  2. 重建 `emotion-echo-web` 镜像（容器内 ASCII 路径）后按 `:cl07` 覆盖验证 —— 需先腾出内存（停部分容器）以免构建期冻结。

## 经验

1. **先做对照实验再归因**：hydration 失败第一时间怀疑自己的改动，用"未改动的主目录"同环境复现 ⇒ 一次排除。
2. **`pnpm dev` 别用 `| tail` 包**：会把输出缓冲、进程存活期间读不到日志（本轮先踩，重启才拿到真实日志）。
3. **dev 服务只监听 `[::1]`**：`curl 127.0.0.1` 会超时（假"服务没起"）；浏览器走 `localhost` 正常。
4. **IAB 的鉴权链**：HttpOnly `access_token` 必须由**应用自身登录流程**写入 Pinia（页面上下文手动 `fetch` 登录只落 cookie，客户端 store 仍空 ⇒ 下游 gRPC 缺 `x-user-id` 返 502）。这是"浏览器侧取数为何 502"的真因，与业务代码无关。
5. **前端取"融合"别走会话端点**：`/emotion/conversation/:id` 是未融合路径，融合按消息粒度。
