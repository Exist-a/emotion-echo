---
status: planned
priority: high
owner: TBD
created: 2026-09-16
last-refresh: 2026-09-16
type: known-issues-backlog
depends-on:
  - stage-109a-apisix-jwt-401-fix-2026-09-16.md (A7 修通)
  - stage-109b-ui-fixes-2026-09-16.md (7 个 UI bug 已修)
related-issues:
  - docs/plans/known-issues-backlog-2026-09-16.md (上一轮已知问题列表)
related-stages:
  - docs/plans/sprint-109b-end-to-end-chat-2026-09-16.md (Sprint 109b 计划)
---

# Known Issues Backlog 追加 — Sprint 109b (3 个未修问题)

> **目的**：本 stage 用户在浏览器实测后**没解决**、只落到 backlog 的 3 个 UI / 链路问题，作为 Sprint 110+ 的技术债登记。
>
> **注意**：用户明确指示"这些本次就不解决了"，所以本文件**不包含修复**，只描述现象、根因、复现步骤和建议修法。

---

## 一、起点

Sprint 109b 已修 7 个 UI 修复（详见 stage-109b-ui-fixes-2026-09-16.md）：

| # | Bug | 状态 |
|---|---|---|
| 1 | APISIX route 100 缺 OPTIONS (CORS preflight 404) | ✅ 已修 (seed.sh:487) |
| 2 | new.vue 与 [id].vue 按钮排布不一致 | ✅ 已修 (新 layout) |
| 3 | nav.vue 右上角 avatar "体" 字 | ✅ 已修 (删 avatar-link) |
| 4 | nav.vue 左下角 sidebar-footer "此刻，我在听" | ✅ 已修 (删 footer) |
| 5 | 数字人 emoji 按钮 (👁🔊) 难看 | ✅ 已修 (换 SVG icon) |
| 6 | AI 回复不存库 → 刷新丢失 | ✅ 已修 (BFF ai_stream_handler 加 saveAIMessage) |
| 7 | 没有开启摄像头入口 | ✅ 已修 (camera-btn + useFaceEmotion) |

修后浏览器 hard reload 验证：`hasSidebarFooter=false`、`hasAvatarLink=false`、`hasCameraBtn=true`、`hasDigitalHuman=true`。但用户在 `/chat/conversation/76` 实测时**又发现 3 个问题**未在本 sprint 处理。

---

## 二、新发现 3 个未解决问题

### Item 8 · A8: 浏览器点击发送后 AI 回复不渲染（前端 SSE 流 / store 链路未生效）

**现象**：浏览器登录 → /chat/conversation/new → 输入"你好，请用一句话介绍你自己" → 点击发送 → 页面 navigate 到 `/chat/conversation/:id`，**用户消息显示在 dialog**（`dialog-user` 渲染了 "你好，请用一句话介绍你自己"），**但 AI 回复不显示**（`hasAIBubble: false`）。

**curl 对照**：curl POST /api/v1/ai/stream 正常返回 SSE chunks + chat-svc DB 里 messages count: 2（user + assistant），证明**后端链路通的**。

**实测证据（来自 Sprint 109b hard reload 测试）**：
- `hasAIBubble: false`（截图 13-detail-ui-fixes.png body 内容只有"你好"，没有 AI 回复）
- BFF 日志：`[grpc-client] method=SendMessage` 只有 1 次（user 消息），**没有 ai-stream 调用**
- chat-svc 日志：Kafka 只 publish 了 `conversation.created` + `message.created`，**没有 ai 流请求**

**根因推测**：
1. **前端 SSE 流没真正发起**：BFF 看不到 `/api/v1/ai/stream` 调用。前端 `useAIStreamHandler.sendAIStream()` 在 `useConversationSender.sendToExistingConversation` 内调用（line 117），可能被 `isStreaming` guard 短路或 `streamAbortController` race condition
3. **前端 store `addMessage` 不触发响应式**：`messageStore.addMessage(tempAiMessage)` 用 `currentMessages.value.push(message)`（store/message.ts:157）。Vue3 ref 默认 deep reactive 但 push 不一定触发引用变化 → `computed(sortedMessages)` 不重算 → `[id].vue` 的 `v-for="item in data"` 不更新
4. **`v-memo` 缓存命中**：`[id].vue:8` `v-memo="[item.id, item.content, item.status, item.contentType]"` — 第一次 addMessage 时 `tempAiMessage.id='temp_ai_xxx'` 在 server 返回真实 ID 后 `updateMessage` 替换为真实 ID，但 v-memo 可能因 item.content  仍是空字符串误命中

**复现路径**：
1. 浏览器 hard reload → http://localhost:3000/login?cb=$(date +%s)
2. 演示账号登录 → /chat/conversation/new
3. 输入"你好" → 点击发送
4. 观察 URL 变成 `/chat/conversation/:id`，但 page body 里只有 user 消息，没有 AI 回复
5. F12 → Network 看不到 `POST /api/v1/ai/stream` 请求

**建议修法（Sprint 110 范围）**：
1. 在 `[id].vue` 的 dialog v-for 上加 `:key="item.id + '-' + item.content"` 强制重渲染
2. `messageStore.addMessage` 改用 `currentMessages.value = [...currentMessages.value, message]` 触发引用变化
3. `v-memo` 改为 `:key="item.id"` (去除 content memo) 或加 `v-if="data.length" key="data.length"`
4. 加 console.log 调试：onDelta 触发次数、updateMessage 调用次数、computed 重算次数
5. 写 Playwright E2E spec（Sprint 110）— 用真浏览器点一次发送 + 等 5s + 断言 dialog 里有 .bubble-ai 元素

**工作量**：0.5-1 sprint（要 debug frontend 响应式 + 写回归测试）

---

### Item 9 · A9: 侧边栏"折叠会话列表"按钮 (`<aside>` 头部 `‹` 折叠箭头) 没对齐前置按钮

**现象**：`/chat/conversation/76` 实测，元素选择器 `button.icon-btn.fold-btn`，位置 `(x=312, y=134, width=30, height=30)`。从 user 提供的 DOM 信息（见用户消息 Element 1）：
- 元素 1 是个 `‹` 箭头（fold-btn），在 sidebar 顶部
- 但**这个箭头按钮的"后面"露出边了**——具体位置描述："侧边栏按钮的后面没有对齐前面，截图可以看到露出边了"

**推测根因**：
- `sidebar-header` flex 容器 + gap 8px（index.vue:264-269），但 `fold-btn` 之后可能还有别的元素（如 + 按钮）撑出 sidebar 宽度边界
- sidebar 的 width 是固定值，但里面的 flex children 总宽度超出 → 露出边
- 或 sidebar 滚动条出现（overflow: auto）让 fold-btn 看起来"露边"

**证据**：`emotion-echo-web/app/pages/chat/conversation/index.vue:263-269`
```scss
.sidebar-header {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 14px;
  flex-shrink: 0;
}
```

**建议修法**：
- 检查 sidebar-header 所有 children 总宽度 vs sidebar container width
- 加 `min-width: 0; flex-wrap: nowrap` 防止溢出
- 或用 `overflow: hidden` + flex-shrink: 1 让按钮自适应

**工作量**：0.1 sprint

---

### Item 10 · A10: 语音按钮样式不对（voice-btn 蓝色背景 + SVG icon 不对齐）

**现象**：user 提供的 Element 2/3：
- Element 2：voice-btn 内部 SVG icon (`rect x=9 y=3 width=6 height=12 rx=3` + `path M5 11a7 7 0 0 0 14 0` + `line x1=12 y1=18 x2=12 y2=22`)
- Element 3：voice-btn `<button class="voice-btn" aria-label="开始录音">`，**背景色 #5F8F7B (蓝绿色)**，位置 `(x=1139, y=618, width=20, height=31)` — 注意 **width=20 height=31 是个奇怪尺寸**（应该是 32x32 圆形按钮，但实测只有 20x31）

**推测根因**：
1. `voice-btn` CSS 在 `[id].vue:636-643` 用了 `position: relative` 但里面的 SVG `<rect>` 用了 `width="6" height="12"` 的 viewBox 24x24，SVG 实际渲染尺寸是 16x16 但 viewBox 坐标系统让 rect 看起来偏移
2. 实际尺寸 20x31 而非 32x32 → CSS `width: 32px; height: 32px` 没生效 → 可能被父级 `.composer-actions` 的 gap 或 padding 挤压
3. 颜色 `#5F8F7B` 是 primary 绿色（var(--ee-primary)），但根据 CSS 应该是 `var(--ee-primary-soft)` 浅色背景 + primary 主色 icon，**实测深绿色背景说明样式没读到 `--ee-primary-soft`**

**证据**：
- `emotion-echo-web/app/pages/chat/conversation/[id].vue:636-643`
```scss
.voice-btn {
  position: relative;
  color: var(--ee-primary);
  background: var(--ee-primary-soft);
  border-color: transparent;
}
```
- 实测 `Background: #5F8F7B` 是 `--ee-primary` 颜色而不是 `--ee-primary-soft`，说明 `.voice-btn` 的 `background: var(--ee-primary-soft)` 这条**被后面的 `&.recording` hover 状态覆盖了**，或者 CSS 选择器优先级问题
- 实测 `width=20, height=31` 是 SVG `viewBox="0 0 24 24"` 渲染出实际像素尺寸，但 `<svg width="16" height="16">` 应该限死 16x16。**说明 CSS `.voice-btn { width: 32px; height: 32px }` 没生效或被覆盖**

**建议修法**：
- 检查 `voice-btn` 的 `&.recording` 与 `&:hover` 状态，确认 background 优先级
- 检查父级 `.composer-actions` 是否有 `gap` 或 `padding` 挤压按钮尺寸
- 用 dev tools 强制刷新缓存（已知的 Nuxt 3 SSR hydration 缓存问题）
- 加 `box-shadow: 0 0 0 2px var(--ee-border); border: 0` 确保按钮有清晰边框

**工作量**：0.2 sprint

---

## 三、累计 backlog（叠加 known-issues-backlog-2026-09-16.md 的 A2-A6）

| Item | 标题 | 优先级 | Sprint |
|---|---|---|---|
| A2 | assessment-svc handler/logic 无单测 | high | Sprint 111 |
| A3 | reports E2E 无 Playwright | medium | Sprint 112 |
| A4 | chartData=[] 历史 bug 未实测复现 | high | Sprint 112 |
| A5 | OAP 9.x queryDuration bug | low | Sprint 115 |
| A6 | Stage 44 §四残余 (sw-oap / Nacos / 运维 SQL) | low | Sprint 115 |
| 顺手 1 | 字号调整 PATCH /users/me ×3 防抖 | low | 任意空闲 sprint |
| 顺手 2 | useApi 401 重试同 ts 双记录 | low | 任意空闲 sprint |
| **A8** | **AI 回复不渲染（前端 SSE/store 链路）** | **high** | **Sprint 110** |
| **A9** | **侧边栏 fold-btn 露出边** | **low** | **Sprint 116** |
| **A10** | **语音按钮样式不对（voice-btn 颜色 + 尺寸）** | **low** | **Sprint 116** |

**A8 优先级最高**：阻塞 chat 端到端可见性，必须 Sprint 110 修（修后立刻接 Sprint 109c 数据契约 smoke）

---

## 四、调研依据（AGENTS.md §〇 硬规则）

| 文件 | 用途 |
|---|---|
| `docs/stages/stage-109b-ui-fixes-2026-09-16.md` | Sprint 109b 已修 7 个 bug 报告 |
| `docs/stages/stage-109b-end-to-end-chat-2026-09-16/screenshots/13-detail-ui-fixes.png` | 用户实测的 [id].vue 截图（缺 AI 回复）|
| `docs/plans/known-issues-backlog-2026-09-16.md` | 上一轮已知问题（A2-A6） |
| `docs/plans/test-coverage-tracker-2026-09-16.md §一 E2E-2` | A8 阻塞端到端验证 |
| `emotion-echo-web/app/composables/useConversationSender.ts:117` | sendAIStream 调用点 |
| `emotion-echo-web/app/composables/useAIStreamHandler.ts:115` | fetch /ai/stream |
| `emotion-echo-web/app/stores/message.ts:156-165` | addMessage / updateMessage |
| `emotion-echo-web/app/pages/chat/conversation/[id].vue:8-43` | dialog v-for + v-memo |
| `emotion-echo-web/app/pages/chat/conversation/index.vue:263-269` | sidebar-header 布局 |

### 架构假设清单（写前对齐）

| 假设 | 验证 |
|---|---|
| A8 根因在 frontend (SSE/store/响应式) | ✅ 后端 curl + chat-svc DB 都通，前端 hasAIBubble=false |
| A8 优先级 high | ✅ 阻塞 chat 端到端 + Sprint 109c 数据契约 smoke |
| A9 + A10 是 CSS 细节，不影响功能 | ✅ 只是 UI 不对齐/样式不好看，聊天可继续 |
| A9 + A10 可在 Sprint 116 集中收口 | ✅ 单独 sprint 不划算 |

---

## 五、commit 计划

| # | Commit | 文件 |
|---|---|---|
| 1 | `docs(plans): known-issues-backlog 追加 Sprint 109b 3 个未修问题 (A8/A9/A10)` | `docs/plans/known-issues-backlog-append-sprint-109b-2026-09-16.md` (新建) |
| 2 | `docs(plans): known-issues-backlog-2026-09-16.md 加 A8/A9/A10 引用` | `docs/plans/known-issues-backlog-2026-09-16.md` |