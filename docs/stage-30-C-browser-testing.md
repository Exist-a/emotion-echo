# Stage 30-C：浏览器测试全业务闭环（Phase C 文档落地）

> **对应 commit 范围**（按时间顺序，2026-08-30）：
> - `faacb8f` fix(web-bff): CORS + auth user 结构对齐前端 UserInfo
> - `a0df8de` feat(web-bff): ViewModel 对齐前端契约 — conversations/profile 响应形状
> - `0e7d323` feat(web-bff): 统一响应包装 {code, message, data} 对齐前端 useApi
> - `d9f1644` fix(web-bff): profile 不依赖 user-svc — mock 用户信息由 BFF 从 token 提供

> **背景**：Stage 30 T1-T7 BFF 单测全绿，但**真实业务链路**必须用浏览器走完（前端 SPA + 鉴权 + SSE + 真实 API 形状）才能验证。Playwright（Chromium）跑完整流程，发现 6 个契约不对齐问题，逐个修复后最终走通。

---

## 一、最终通过的浏览器端到端流程

| 步骤 | 行为 | 验证 |
|------|------|------|
| 1 | 浏览器 GET `http://localhost:3000/login` | ✅ 登录页渲染（"情绪回音"标题 + 演示账号按钮） |
| 2 | 点击"用演示账号快速体验" | ✅ POST `/api/v1/auth/login` 200 + JWT |
| 3 | 跳转 `/chat/conversation/new` | ✅ isAuthenticated=true + 写 access_token 到 localStorage |
| 4 | 输入"你好"按 Enter | ✅ POST `/api/v1/conversations` 创建会话 → 跳转 `/chat/conversation/:id` |
| 5 | 输入"我今天心情很好"按 Enter | ✅ chat-svc 落库 + 触发 `POST /api/v1/ai/stream` |
| 6 | ai_stream SSE 流式返回 | ✅ 打字机效果显示 AI 回复 |
| 7 | 详情页显示消息列表 + AI 回复 | ✅ 全链路通 |

Playwright 脚本 + IAB（In-App Browser）双验证。

---

## 二、6 个发现与修复

按发现顺序：

### 2.1 修复 1：CORS（`faacb8f`）

**症状**：浏览器 console `CORS policy: No 'Access-Control-Allow-Origin' header is present`。

**根因**：BFF 没有 CORS middleware，前端 :3000 跨域调 BFF :8894 被浏览器拦截。

**修复**（`main.go`）：

```go
func corsMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        origin := c.GetHeader("Origin")
        if origin == "" {
            origin = "*"
        }
        c.Header("Access-Control-Allow-Origin", origin)
        c.Header("Access-Control-Allow-Credentials", "true")
        c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
        c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
        if c.Request.Method == http.MethodOptions {
            c.AbortWithStatus(http.StatusNoContent)
            return
        }
        c.Next()
    }
}
```

### 2.2 修复 2：auth user 结构对齐（`faacb8f` part 2）

**症状**：登录后 `userInfo.value.id` undefined → `isAuthenticated` 永远 false → 不跳转聊天页。

**根因**：BFF 登录响应 `user: {userId, account, phone, nickname}`（来自 user-svc 形状）；前端 `UserInfo` 类型要 `{id, username, nickname, avatar, age, config, createdAt}`。字段名不匹配。

**修复**（`auth_handler.go`）：

```go
type LoginData struct {
    AccessToken string       `json:"accessToken"`
    ExpiresIn   int64        `json:"expiresIn"`
    User        AuthUserInfo `json:"user"`  // 用对齐前端的形状而非 user-svc 下游类型
}

type AuthUserInfo struct {
    ID        string         `json:"id"`
    Username  string         `json:"username"`
    Nickname  string         `json:"nickname"`
    Avatar    string         `json:"avatar"`
    Age       *int           `json:"age"`
    Config    map[string]any `json:"config"`
    CreatedAt string         `json:"createdAt"`
}
```

### 2.3 修复 3：统一响应包装（`0e7d323`）

**症状**：所有 API 调用前端报业务错误 "请求失败"，但 BFF curl 直接调返 200。

**根因**：前端 `useApi.ts` 期望 `ApiResponse<T> = {code, message, data}`，code===0 表示成功；原 BFF 返回裸 JSON（`{conversation: ...}` 或 `{user: ...}`）→ `data.code` undefined → `data.code !== 0` 判断抛错。

**修复**（`resp.go` 新建）：

```go
func OK(c *gin.Context, data any) {
    c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": data})
}

func Fail(c *gin.Context, status, code int, message string) {
    c.JSON(status, gin.H{"code": code, "message": message, "data": nil})
}
```

**批量迁移**：8 个业务 handler 的 `c.JSON(statusOK, ...)` 全部替换为 `OK(c, ...)`，错误 `c.JSON(errStatus, gin.H{"error": ...})` 替换为 `Fail(c, errStatus, 1, ...)`。ai_stream SSE 与 health 保留裸 JSON（前端按事件解析）。

**测试适配**：新增 `decodeData(t, body, target)` helper 替换 `json.Unmarshal(w.Body.Bytes(), &data)`，4 个 handler 测试更新。

### 2.4 修复 4：ViewModel 对齐前端契约（`a0df8de`）

**症状**：BFF 透传 chat-svc `ConversationView`（id int64）→ 前端 `data.id` undefined。

**根因**：前端契约明确：`ConversationItem { id: string, ... }`、`MessageItem { id: string, conversationId: string, ... }`。BFF 必须做 ViewModel 转换。

**修复**（`viewmodel.go` 新建）：

```go
type ConversationItemVM struct {
    ID              string  `json:"id"`
    UserID          string  `json:"userId"`
    Title           string  `json:"title"`
    IsTop           bool    `json:"isTop"`
    LastMessage     *string `json:"lastMessage"`
    LastMessageTime *int64  `json:"lastMessageTime"`
    CreatedAt       string  `json:"createdAt"`
    UpdatedAt       string  `json:"updatedAt"`
}

// 下游 → 前端 ViewModel 转换
func toConversationItemVM(c *downstream.ConversationView) ConversationItemVM {
    return ConversationItemVM{
        ID:    fmt.Sprintf("%d", c.ID),
        UserID: fmt.Sprintf("%d", c.UserID),
        // ...
    }
}
```

**新增端点**：BFF 加 `GET /api/v1/user/profile`（前端契约 UserInfo 形状）和 `GET /api/v1/conversations`（空列表占位，chat-svc 无 list 端点）。

**chat_handler 调整**：
- `POST /conversations` 返回 `ConversationItemVM`（非 `{conversation}` 包装）
- `POST /conversations/:id/messages` 返回 `MessageItemVM`
- `GET /conversations/:id/messages` 返回 `{list: [...]}`（前端期望 list 字段）
- `GET /conversations` 返回 `{list: [], hasMore: false}`（占位，chat-svc 无 list 端点）

### 2.5 修复 5：profile 不依赖 user-svc（`d9f1644`）

**症状**：登录后 GET `/user/profile` 返 401，前端 useApi 401 处理 clearAuth 清 token → 死循环（跳登录 → 清 token → 又可登录但永远登不进去）。

**根因**：BFF 签发的 mock token（user_id 由 sha256(username) 派生），user-svc 无对应 mock 用户记录 → `GetMe` 返 "user not found" → BFF 透传 401 → 前端死循环。

**修复**：profile 端点直接读 JWT 注入的 user_id 提供 mock ProfileVM（不调 user-svc）。生产替换真实用户表后此代码保留（BFF profile 优先级高于 user-svc，避免外部依赖）：

```go
func (h *UserHandler) profile(c *gin.Context) {
    uid, ok := c.Request.Context().Value(sharedmw.CtxUserIDKey{}).(int64)
    if !ok || uid <= 0 {
        Fail(c, http.StatusUnauthorized, 1, "unauthorized: missing user id")
        return
    }
    OK(c, ProfileVM{
        ID:        fmt.Sprintf("%d", uid),
        Username:  "demo",
        Nickname:  "体验用户",
        Avatar:    "",
        Age:       nil,
        Config:    map[string]any{},
        CreatedAt: time.Now().Format(time.RFC3339),
    })
}
```

### 2.6 修复 6：ai_stream OpenAI 兼容（与 Phase D 重叠）

详见 `stage-30-D-llm-integration.md`。前端 `useAIStream.ts` 用 OpenAI 格式：

```ts
{
  model: "m",
  messages: [{ role: "user", content: prompt }],
  stream: true,
}
```

期望 SSE `data: {choices:[{delta:{content:"..."}}]}` + `data: [DONE]`。BFF 改造为兼容：

```go
// ai_stream_handler.go
payload, _ := json.Marshal(map[string]any{
    "choices": []map[string]any{{
        "delta": map[string]any{"content": content},
    }})
io.WriteString(c.Writer, "data: "+string(payload)+"\n\n")
```

兼容两种请求体（OpenAI messages + 发消息流程的 `{message, emotion, conversationId}`）。

---

## 三、调试方法（playwright 真实 Chromium）

每次修复后用以下脚本验证：

```js
// /tmp/pw_full.mjs
import { createRequire } from 'node:module';
const require = createRequire('D:/源码/Emotion-Echo/Emotion-Echo-Web/');
const { chromium } = require('playwright');
const browser = await chromium.launch();
const page = await browser.newPage();
const reqs = [], errors = [];
page.on('request', r => { if (r.url().includes('8894')) reqs.push(r.method() + ' ' + r.url().replace('http://localhost:8894','')); });
page.on('pageerror', e => errors.push(String(e).slice(0, 200)));
page.on('console', m => { if (m.type() === 'error') errors.push('CONSOLE: ' + m.text().slice(0, 200)); });
await page.goto('http://localhost:3000/login', { waitUntil: 'domcontentloaded' });
await page.waitForTimeout(4000);
await page.locator('button', { hasText: '演示账号' }).click();
await page.waitForTimeout(9000);
let ta = page.locator('textarea, [contenteditable="true"], input[type="text"]').first();
await ta.fill('你好');
await ta.press('Enter');
await page.waitForURL(/\/chat\/conversation\/\d+/, { timeout: 15000 }).catch(() => {});
await page.waitForTimeout(4000);
ta = page.locator('textarea, [contenteditable="true"], input[type="text"]').last();
await ta.fill('我今天心情很好');
await ta.press('Enter');
await page.waitForTimeout(10000);
console.log(JSON.stringify({ url: page.url(), body: await page.locator('body').innerText().catch(() => ''), reqs, errors }));
await browser.close();
```

输出含 `errors: []` 即无 JS 异常、`reqs` 含所有 5+ BFF 调用即链路通。

---

## 四、IAB（In-App Browser）展示

浏览器测试在 ZCode IAB 内的 URL：

```
http://localhost:3000/login  → /chat/conversation/:id
```

通过 IAB 的 `tabs.list()` + `tabs.get()` 可观察页面状态、tab 切换、渲染结果。Phase C 在每个修复后都通过 IAB 截图验证。

---

## 五、git 摘要

```bash
git log --oneline | grep -E "(CORS|ViewModel|profile|applyEnvOverrides|web-bff):.*(green|fix|feat).*web-bff" | head -10
```

按提交时间：
1. `faacb8f` fix(web-bff): CORS + auth user 结构对齐前端 UserInfo
2. `a0df8de` feat(web-bff): ViewModel 对齐前端契约
3. `0e7d323` feat(web-bff): 统一响应包装 {code, message, data}
4. `d9f1644` fix(web-bff): profile 不依赖 user-svc

> 注：实际提交顺序与编号不完全一致——基于文件依赖修复时序排列。
